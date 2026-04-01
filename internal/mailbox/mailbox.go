package mailbox

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"tideland.dev/go/wait"

	"tideland.dev/go/boxcopy/internal/config"
	"tideland.dev/go/boxcopy/internal/imap"
	"tideland.dev/go/boxcopy/internal/state"
)

// Status represents the current status of a mailbox.
type Status int

const (
	StatusIdle Status = iota
	StatusConnecting
	StatusCopying
	StatusDisconnecting
	StatusError
	StatusStopped
)

// String returns a string representation of the status.
func (s Status) String() string {
	switch s {
	case StatusIdle:
		return "idle"
	case StatusConnecting:
		return "connecting"
	case StatusCopying:
		return "copying"
	case StatusDisconnecting:
		return "disconnecting"
	case StatusError:
		return "error"
	case StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// Mailbox manages copying of a single mailbox from source to target.
type Mailbox struct {
	mu           sync.RWMutex
	config       config.MailboxConfig
	sourceServer config.ServerConfig
	targetServer config.ServerConfig
	state        *state.State
	logger       *slog.Logger
	throttle     *wait.Throttle
	ctx          context.Context
	progress     int

	sourceClient *imap.Client
	targetClient *imap.Client

	status    Status
	lastError error
	lastSync  time.Time
	syncCount int64

	// Statistics
	foldersCreated  int64
	messagesCopied  int64
	messagesSkipped int64
	bytesCopied     int64
}

// Options configures mailbox behavior.
type Options struct {
	Logger   *slog.Logger
	Throttle *wait.Throttle
	Context  context.Context
	Progress int
}

// DefaultOptions returns default mailbox options.
func DefaultOptions() *Options {
	return &Options{
		Logger:   slog.Default(),
		Progress: 10,
	}
}

// New creates a new Mailbox.
func New(
	mbConfig config.MailboxConfig,
	sourceServer config.ServerConfig,
	targetServer config.ServerConfig,
	syncState *state.State,
	opts *Options,
) *Mailbox {
	if opts == nil {
		opts = DefaultOptions()
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger = logger.With(slog.String("mailbox", mbConfig.Name))

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	progress := opts.Progress

	return &Mailbox{
		config:       mbConfig,
		sourceServer: sourceServer,
		targetServer: targetServer,
		state:        syncState,
		logger:       logger,
		throttle:     opts.Throttle,
		ctx:          ctx,
		progress:     progress,
		status:       StatusIdle,
	}
}

// Name returns the mailbox name.
func (m *Mailbox) Name() string {
	return m.config.Name
}

// Connect establishes connections to source and target IMAP servers.
func (m *Mailbox) Connect() error {
	return m.connect()
}

// Disconnect closes connections to both IMAP servers.
func (m *Mailbox) Disconnect() {
	m.disconnect() //nolint:errcheck
}

// Copy performs a full copy (folders and messages) from source to target.
func (m *Mailbox) Copy() error {
	return m.copy()
}

// Status returns the current mailbox status.
func (m *Mailbox) Status() StatusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return StatusInfo{
		Name:            m.config.Name,
		Status:          m.status,
		LastError:       m.lastError,
		LastSync:        m.lastSync,
		SyncCount:       m.syncCount,
		FoldersCreated:  m.foldersCreated,
		MessagesCopied:  m.messagesCopied,
		MessagesSkipped: m.messagesSkipped,
		BytesCopied:     m.bytesCopied,
		IsConnected:     m.sourceClient != nil && m.targetClient != nil,
	}
}

// setStatus safely sets the mailbox status.
func (m *Mailbox) setStatus(s Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status = s
}

// setError safely sets the last error and updates status to StatusError.
func (m *Mailbox) setError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = err
	m.status = StatusError
}

// addCopied safely updates copied statistics.
func (m *Mailbox) addCopied(count int64, bytes int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messagesCopied += count
	m.bytesCopied += bytes
}

// addSkipped safely updates skipped statistics.
func (m *Mailbox) addSkipped(count int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messagesSkipped += count
}

// addFoldersCreated safely updates folders created statistics.
func (m *Mailbox) addFoldersCreated(count int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.foldersCreated += count
}

// StatusInfo contains detailed status information.
type StatusInfo struct {
	Name            string
	Status          Status
	LastError       error
	LastSync        time.Time
	SyncCount       int64
	FoldersCreated  int64
	MessagesCopied  int64
	MessagesSkipped int64
	BytesCopied     int64
	IsConnected     bool
}

// Internal methods

func (m *Mailbox) connect() error {
	m.mu.RLock()
	alreadyConnected := m.sourceClient != nil && m.targetClient != nil
	m.mu.RUnlock()

	if alreadyConnected {
		return nil // Already connected.
	}

	m.setStatus(StatusConnecting)
	m.logger.Debug("connecting to IMAP servers")

	// Connect to source.
	sourceClient, err := imap.Dial(
		m.sourceServer,
		m.config.SourceUser,
		m.config.SourcePassword,
		&imap.Options{Logger: m.logger.With(slog.String("server", "source"))},
	)
	if err != nil {
		m.setError(fmt.Errorf("source connection failed: %w", err))
		return m.lastError
	}

	m.mu.Lock()
	m.sourceClient = sourceClient
	m.mu.Unlock()

	// Connect to target.
	targetClient, err := imap.Dial(
		m.targetServer,
		m.config.TargetUser,
		m.config.TargetPassword,
		&imap.Options{Logger: m.logger.With(slog.String("server", "target"))},
	)
	if err != nil {
		m.sourceClient.Close() //nolint:errcheck
		m.mu.Lock()
		m.sourceClient = nil
		m.mu.Unlock()
		m.setError(fmt.Errorf("target connection failed: %w", err))
		return m.lastError
	}

	m.mu.Lock()
	m.targetClient = targetClient
	m.mu.Unlock()

	m.setStatus(StatusIdle)
	m.logger.Info("connected to both IMAP servers")
	return nil
}

func (m *Mailbox) disconnect() error {
	m.setStatus(StatusDisconnecting)
	m.logger.Debug("disconnecting from IMAP servers")

	var errs []error

	m.mu.Lock()
	sourceClient := m.sourceClient
	targetClient := m.targetClient
	m.sourceClient = nil
	m.targetClient = nil
	m.mu.Unlock()

	if sourceClient != nil {
		if err := sourceClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("source disconnect: %w", err))
		}
	}

	if targetClient != nil {
		if err := targetClient.Close(); err != nil {
			errs = append(errs, fmt.Errorf("target disconnect: %w", err))
		}
	}

	m.setStatus(StatusIdle)

	if len(errs) > 0 {
		m.setError(errs[0])
		return m.lastError
	}

	m.logger.Info("disconnected from IMAP servers")
	return nil
}

func (m *Mailbox) copy() error {
	// Ensure connected.
	if err := m.connect(); err != nil {
		return err
	}
	defer m.disconnect() //nolint:errcheck

	m.setStatus(StatusCopying)
	m.logger.Info("starting full copy")

	// Copy folders first.
	if err := m.copyFolders(); err != nil {
		m.setError(err)
		return err
	}

	// Get list of folders to copy. connect() above ensures sourceClient is non-nil.
	folders, err := m.sourceClient.ListFolders()
	if err != nil {
		m.setError(fmt.Errorf("failed to list folders: %w", err))
		return m.lastError
	}

	// Copy each folder, accumulating errors so all folders are attempted.
	var folderErrors int
	for _, folder := range folders {
		if folder.IsNoSelect() {
			continue
		}
		m.logger.Info("copying folder", slog.String("folder", folder.Name))
		if err := m.copyFolder(folder.Name); err != nil {
			m.logger.Warn("folder copy failed",
				slog.String("folder", folder.Name),
				slog.Any("error", err))
			folderErrors++
			// The target connection may be broken; reconnect before next folder.
			if disconnErr := m.disconnect(); disconnErr != nil {
				m.logger.Warn("error while disconnecting before reconnect",
					slog.Any("error", disconnErr))
			}
			if reconnErr := m.connect(); reconnErr != nil {
				m.logger.Error("failed to reconnect to target server, aborting folder copy",
					slog.Any("error", reconnErr))
				break
			}
		} else {
			m.logger.Info("folder copy completed", slog.String("folder", folder.Name))
		}
	}

	m.setStatus(StatusIdle)

	m.mu.Lock()
	m.lastSync = time.Now()
	m.syncCount++
	// Copy stats under lock to avoid holding the lock during logging.
	copied := m.messagesCopied
	skipped := m.messagesSkipped
	m.mu.Unlock()

	m.state.UpdateMailboxLastSync(m.config.Name)
	m.logger.Info("full copy completed",
		slog.Int64("messages_copied", copied),
		slog.Int64("messages_skipped", skipped))

	if folderErrors > 0 {
		return fmt.Errorf("%d folder(s) failed to copy", folderErrors)
	}
	return nil
}
