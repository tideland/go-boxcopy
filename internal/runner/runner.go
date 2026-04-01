package runner

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"tideland.dev/go/wait"
	"tideland.dev/go/worker"

	"tideland.dev/go/boxcopy/internal/config"
	"tideland.dev/go/boxcopy/internal/imap"
	"tideland.dev/go/boxcopy/internal/mailbox"
	"tideland.dev/go/boxcopy/internal/state"
)

// Options configures the copy run.
type Options struct {
	// ConfigPath is the path to the configuration file.
	ConfigPath string

	// EncryptionKey for decrypting passwords.
	EncryptionKey string

	// Perform enables the actual copy. Without it, a dry-run is performed.
	Perform bool

	// Logger overrides the logger (used in tests). When nil the runner creates
	// its own logger based on ExplicitLogLevel / config log_level.
	Logger *slog.Logger

	// ExplicitLogLevel is the raw --log-level CLI value. If empty and Logger
	// is nil, the runner falls back to the config's log_level after loading it.
	ExplicitLogLevel string

	// MaxCopyDuration caps how long the parallel copy phase may run.
	// Zero defaults to 24 hours.
	MaxCopyDuration time.Duration

	// Context for the copy operation. When nil, a signal-aware context that
	// cancels on SIGINT/SIGTERM is created automatically.
	Context context.Context

	// Input overrides stdin for the safety prompt (used in tests).
	Input io.Reader
}

// Run loads config, optionally dry-runs or performs the full copy with
// safety confirmation, target cleanup, and parallel mailbox processing.
func Run(opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}

	if opts.ConfigPath == "" {
		return fmt.Errorf("config path is required")
	}

	if opts.EncryptionKey == "" {
		return fmt.Errorf("encryption key is required (-k flag)")
	}

	// Load and validate configuration.
	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if err := cfg.DecryptMailboxPasswords(opts.EncryptionKey); err != nil {
		return fmt.Errorf("failed to decrypt passwords: %w", err)
	}

	// Resolve logger: test-provided logger takes precedence; otherwise build
	// one from the effective log level (CLI flag > config file).
	logger := opts.Logger
	if logger == nil {
		levelStr := opts.ExplicitLogLevel
		if levelStr == "" {
			levelStr = cfg.General.LogLevel
		}
		logger = newLoggerForLevel(levelStr)
	}
	logger = logger.With(slog.String("component", "boxcopy"))

	if !opts.Perform {
		return dryRun(cfg, logger)
	}

	return performCopy(cfg, opts, logger)
}

// newLoggerForLevel creates a text logger writing to stderr at the given level.
func newLoggerForLevel(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// dryRun shows what would be copied without making any changes.
func dryRun(cfg *config.Config, logger *slog.Logger) error {
	logger.Info("dry-run mode: no changes will be made (use --perform to copy)")
	logger.Info("configuration loaded",
		slog.String("source", cfg.Source.Host),
		slog.String("target", cfg.Target.Host),
		slog.Int("mailboxes", len(cfg.Mailboxes)),
		slog.Int("max_connections", cfg.CopyParam.MaxConnections),
		slog.Int("messages_per_second", cfg.CopyParam.MessagesPerSecond),
	)
	for _, mb := range cfg.Mailboxes {
		logger.Info("would copy mailbox",
			slog.String("name", mb.Name),
			slog.String("source_user", mb.SourceUser),
			slog.String("target_user", mb.TargetUser),
		)
	}
	logger.Info("dry-run complete: run with --perform to start the actual copy")
	return nil
}

// performCopy runs the full copy: safety confirmation → target cleanup → copy.
func performCopy(cfg *config.Config, opts *Options, logger *slog.Logger) error {
	// Determine input source for confirmation prompt.
	input := opts.Input
	if input == nil {
		input = os.Stdin
	}

	// Step 1: Safety confirmation.
	if err := confirmCopy(cfg, input); err != nil {
		return err
	}
	logger.Info("copy confirmed by user")

	// Set up a context that cancels on SIGINT/SIGTERM for graceful shutdown.
	// If the caller provides a context (e.g. in tests), use that instead.
	ctx := opts.Context
	if ctx == nil {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
	}

	// Step 2: Target cleanup — connect to each target mailbox in parallel,
	// expunge all messages, then delete all non-INBOX folders.
	logger.Info("cleaning target mailboxes before copy")
	if err := cleanTargets(cfg, logger); err != nil {
		return fmt.Errorf("target cleanup failed: %w", err)
	}
	logger.Info("target cleanup completed")

	// Step 3: Load or create sync state, cleared fresh for --perform.
	syncState, err := state.Load(cfg.General.StateFile)
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	for _, mb := range cfg.Mailboxes {
		syncState.ClearMailbox(mb.Name)
	}
	if err := syncState.Save(); err != nil {
		return fmt.Errorf("failed to save cleared state: %w", err)
	}

	// Step 4: Copy all mailboxes in parallel using a WorkerPool.
	logger.Info("starting copy",
		slog.Int("mailboxes", len(cfg.Mailboxes)),
		slog.Int("max_connections", cfg.CopyParam.MaxConnections),
	)

	throttle := wait.NewThrottle(
		wait.Limit(cfg.CopyParam.MessagesPerSecond),
		cfg.CopyParam.MessagesPerSecond,
	)

	pool, err := worker.NewWorkerPool(
		cfg.CopyParam.MaxConnections,
		worker.DefaultConfig(),
	)
	if err != nil {
		return fmt.Errorf("failed to create worker pool: %w", err)
	}
	defer worker.Stop(pool) //nolint:errcheck

	var (
		mu         sync.Mutex
		copyErrors int
	)

	for _, mbConfig := range cfg.Mailboxes {
		mbConfig := mbConfig // capture for closure
		mbLogger := logger.With(slog.String("mailbox", mbConfig.Name))

		if err := worker.Enqueue(pool, func() error {
			mb := mailbox.New(
				mbConfig,
				cfg.Source,
				cfg.Target,
				syncState,
				&mailbox.Options{
					Logger:   logger,
					Throttle: throttle,
					Context:  ctx,
					Progress: cfg.General.Progress,
				},
			)

			if err := mb.Copy(); err != nil {
				mbLogger.Error("mailbox copy failed", slog.Any("error", err))
				mu.Lock()
				copyErrors++
				mu.Unlock()
				return err
			}
			mbLogger.Info("mailbox copy completed")
			return nil
		}); err != nil {
			return fmt.Errorf("failed to enqueue mailbox %s: %w", mbConfig.Name, err)
		}
	}

	// Resolve copy timeout: use caller-supplied value or default to 24 hours.
	copyTimeout := opts.MaxCopyDuration
	if copyTimeout <= 0 {
		copyTimeout = 24 * time.Hour
	}

	// Wait for all mailboxes to finish.
	if err := worker.WaitForTasks(pool, copyTimeout); err != nil {
		return fmt.Errorf("copy timed out after %v: %w", copyTimeout, err)
	}

	// Always save state, even if some mailboxes failed.
	if err := syncState.Save(); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	mu.Lock()
	errs := copyErrors
	mu.Unlock()

	if errs > 0 {
		return fmt.Errorf("%d mailbox(es) failed to copy", errs)
	}

	logger.Info("copy completed", slog.Int("mailboxes", len(cfg.Mailboxes)))
	return nil
}

// confirmCopy prints a summary and asks the user to confirm. Returns an error
// if the user declines.
func confirmCopy(cfg *config.Config, input io.Reader) error {
	fmt.Printf("\nBoxCopy - IMAP mailbox copy\n")
	fmt.Printf("  Source : %s\n", cfg.Source.Host)
	fmt.Printf("  Target : %s\n", cfg.Target.Host)
	fmt.Printf("  Mailboxes (%d):\n", len(cfg.Mailboxes))
	for _, mb := range cfg.Mailboxes {
		fmt.Printf("    - %s  (%s → %s)\n", mb.Name, mb.SourceUser, mb.TargetUser)
	}
	fmt.Printf("\nWARNING: All existing messages on the target server will be permanently\n")
	fmt.Printf("         deleted before copying begins.\n\n")
	fmt.Printf("Type YES to continue: ")

	reader := bufio.NewReader(input)
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to read confirmation: %w", err)
	}
	answer = strings.TrimSpace(answer)

	if answer != "YES" {
		return fmt.Errorf("copy cancelled")
	}

	return nil
}

// cleanTargets connects to each target mailbox in parallel (up to MaxConnections
// concurrent workers), deletes all messages in all folders, and expunges.
func cleanTargets(cfg *config.Config, logger *slog.Logger) error {
	pool, err := worker.NewWorkerPool(cfg.CopyParam.MaxConnections, worker.DefaultConfig())
	if err != nil {
		return fmt.Errorf("failed to create cleanup worker pool: %w", err)
	}
	defer worker.Stop(pool) //nolint:errcheck

	var (
		mu          sync.Mutex
		cleanErrors int
	)

	for _, mbConfig := range cfg.Mailboxes {
		mbConfig := mbConfig // capture for closure
		mbLogger := logger.With(slog.String("mailbox", mbConfig.Name))

		if err := worker.Enqueue(pool, func() error {
			mbLogger.Info("cleaning target mailbox")
			if err := cleanTargetMailbox(mbConfig, cfg.Target, mbLogger); err != nil {
				mbLogger.Error("failed to clean target mailbox", slog.Any("error", err))
				mu.Lock()
				cleanErrors++
				mu.Unlock()
				return err
			}
			mbLogger.Info("target mailbox cleaned")
			return nil
		}); err != nil {
			return fmt.Errorf("failed to enqueue cleanup for mailbox %s: %w", mbConfig.Name, err)
		}
	}

	if err := worker.WaitForTasks(pool, 30*time.Minute); err != nil {
		return fmt.Errorf("target cleanup timed out: %w", err)
	}

	mu.Lock()
	errs := cleanErrors
	mu.Unlock()

	if errs > 0 {
		return fmt.Errorf("%d mailbox(es) failed to clean", errs)
	}
	return nil
}

// cleanTargetMailbox connects to a single target mailbox and expunges all
// messages from all selectable folders.
func cleanTargetMailbox(mbConfig config.MailboxConfig, targetServer config.ServerConfig, logger *slog.Logger) (retErr error) {
	client, err := imap.Dial(
		targetServer,
		mbConfig.TargetUser,
		mbConfig.TargetPassword,
		&imap.Options{Logger: logger},
	)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close client: %w", err)
		}
	}()

	folders, err := client.ListFolders()
	if err != nil {
		return fmt.Errorf("failed to list folders: %w", err)
	}

	for _, folder := range folders {
		if folder.IsNoSelect() {
			continue
		}

		info, err := client.Select(folder.Name)
		if err != nil {
			return fmt.Errorf("failed to select folder %q: %w", folder.Name, err)
		}

		if info.NumMessages == 0 {
			logger.Debug("folder already empty", slog.String("folder", folder.Name))
			continue
		}

		logger.Debug("expunging folder",
			slog.String("folder", folder.Name),
			slog.Uint64("messages", uint64(info.NumMessages)))

		// Mark all messages as deleted using UID 1:*.
		allUIDs, err := client.FetchAll(&imap.FetchOptions{UID: true})
		if err != nil {
			return fmt.Errorf("failed to fetch UIDs in folder %q: %w", folder.Name, err)
		}

		uids := make([]uint32, 0, len(allUIDs))
		for _, msg := range allUIDs {
			uids = append(uids, msg.UID)
		}

		if len(uids) > 0 {
			if err := client.MarkDeleted(uids); err != nil {
				return fmt.Errorf("failed to mark messages deleted in folder %q: %w", folder.Name, err)
			}
		}

		if err := client.Expunge(); err != nil {
			return fmt.Errorf("failed to expunge folder %q: %w", folder.Name, err)
		}

		logger.Info("folder expunged",
			slog.String("folder", folder.Name),
			slog.Int("messages", len(uids)))
	}

	// Deselect any currently selected mailbox before deleting folders.
	// Some servers drop the connection if DELETE is issued while a
	// mailbox is still selected.
	if err := client.Unselect(); err != nil {
		return fmt.Errorf("failed to unselect mailbox before folder deletion: %w", err)
	}

	// Delete all folders except INBOX.
	// Sort descending by name so children always come before their parents
	// (e.g. "INBOX.Sent.Sub" before "INBOX.Sent" before "INBOX").
	sorted := make([]imap.Folder, len(folders))
	copy(sorted, folders)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name > sorted[j].Name
	})
	for _, folder := range sorted {
		// INBOX cannot be deleted on most servers; skip it.
		if strings.EqualFold(folder.Name, "INBOX") {
			continue
		}
		if err := client.DeleteFolder(folder.Name); err != nil {
			return fmt.Errorf("failed to delete folder %q: %w", folder.Name, err)
		}
		logger.Info("folder deleted", slog.String("folder", folder.Name))
	}

	return nil
}
