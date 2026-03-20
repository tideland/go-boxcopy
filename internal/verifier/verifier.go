package verifier

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"fmt"
	"log/slog"
	"sort"

	"tideland.dev/go/boxcopy/internal/config"
	"tideland.dev/go/boxcopy/internal/imap"
)

// FolderResult holds the comparison result for a single folder.
type FolderResult struct {
	Name     string
	SrcCount uint32
	TgtCount uint32
	SrcSize  uint64 // total RFC822 bytes on source
	TgtSize  uint64 // total RFC822 bytes on target
}

// OK returns true if message count and total size both match.
func (r FolderResult) OK() bool {
	return r.SrcCount == r.TgtCount && r.SrcSize == r.TgtSize
}

// MailboxResult holds the verification result for one mailbox.
type MailboxResult struct {
	Name           string
	SrcUser        string
	TgtUser        string
	MissingFolders []string // present on source, absent on target
	ExtraFolders   []string // present on target, absent on source
	Folders        []FolderResult
	Err            error
}

// OK returns true if there are no errors, no missing folders, and every
// common folder's count and size match.
func (r *MailboxResult) OK() bool {
	if r.Err != nil || len(r.MissingFolders) > 0 {
		return false
	}
	for _, fr := range r.Folders {
		if !fr.OK() {
			return false
		}
	}
	return true
}

// Options configures the verifier.
type Options struct {
	ConfigPath    string
	EncryptionKey string
	Logger        *slog.Logger
}

// Verify connects to each source/target mailbox pair defined in the config and
// compares folder structure, per-folder message counts, and total per-folder
// RFC822 sizes. All IMAP connections are read-only; no data is modified.
func Verify(opts *Options) ([]MailboxResult, error) {
	if opts.ConfigPath == "" {
		return nil, fmt.Errorf("config path is required")
	}
	if opts.EncryptionKey == "" {
		return nil, fmt.Errorf("encryption key is required")
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if err := cfg.DecryptMailboxPasswords(opts.EncryptionKey); err != nil {
		return nil, fmt.Errorf("failed to decrypt passwords: %w", err)
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var results []MailboxResult
	for _, mb := range cfg.Mailboxes {
		results = append(results, verifyMailbox(mb, cfg, logger))
	}
	return results, nil
}

func verifyMailbox(mb config.MailboxConfig, cfg *config.Config, logger *slog.Logger) (result MailboxResult) {
	result = MailboxResult{
		Name:    mb.Name,
		SrcUser: mb.SourceUser,
		TgtUser: mb.TargetUser,
	}

	imapOpts := &imap.Options{Logger: logger}

	src, err := imap.Dial(cfg.Source, mb.SourceUser, mb.SourcePassword, imapOpts)
	if err != nil {
		result.Err = fmt.Errorf("source connect failed: %w", err)
		return result
	}
	defer func() {
		if err := src.Close(); err != nil && result.Err == nil {
			result.Err = fmt.Errorf("failed to close source connection: %w", err)
		}
	}()

	tgt, err := imap.Dial(cfg.Target, mb.TargetUser, mb.TargetPassword, imapOpts)
	if err != nil {
		result.Err = fmt.Errorf("target connect failed: %w", err)
		return result
	}
	defer func() {
		if err := tgt.Close(); err != nil && result.Err == nil {
			result.Err = fmt.Errorf("failed to close target connection: %w", err)
		}
	}()

	srcFolders, err := src.ListFolders()
	if err != nil {
		result.Err = fmt.Errorf("failed to list source folders: %w", err)
		return result
	}
	tgtFolders, err := tgt.ListFolders()
	if err != nil {
		result.Err = fmt.Errorf("failed to list target folders: %w", err)
		return result
	}

	// Build maps of selectable folder names to message counts.
	srcMap := make(map[string]uint32)
	tgtMap := make(map[string]uint32)

	for _, f := range srcFolders {
		if !f.IsNoSelect() {
			count := uint32(0)
			if f.NumMessages != nil {
				count = *f.NumMessages
			}
			srcMap[f.Name] = count
		}
	}
	for _, f := range tgtFolders {
		if !f.IsNoSelect() {
			count := uint32(0)
			if f.NumMessages != nil {
				count = *f.NumMessages
			}
			tgtMap[f.Name] = count
		}
	}

	// Identify missing and extra folders.
	for name := range srcMap {
		if _, ok := tgtMap[name]; !ok {
			result.MissingFolders = append(result.MissingFolders, name)
		}
	}
	for name := range tgtMap {
		if _, ok := srcMap[name]; !ok {
			result.ExtraFolders = append(result.ExtraFolders, name)
		}
	}
	sort.Strings(result.MissingFolders)
	sort.Strings(result.ExtraFolders)

	// For folders present on both sides, compare count and total size.
	common := make([]string, 0, len(srcMap))
	for name := range srcMap {
		if _, ok := tgtMap[name]; ok {
			common = append(common, name)
		}
	}
	sort.Strings(common)

	for _, name := range common {
		fr := FolderResult{
			Name:     name,
			SrcCount: srcMap[name],
			TgtCount: tgtMap[name],
		}

		srcSize, err := folderTotalSize(src, name)
		if err != nil {
			logger.Warn("failed to fetch source folder sizes",
				slog.String("mailbox", mb.Name),
				slog.String("folder", name),
				slog.Any("error", err))
		}
		fr.SrcSize = srcSize

		tgtSize, err := folderTotalSize(tgt, name)
		if err != nil {
			logger.Warn("failed to fetch target folder sizes",
				slog.String("mailbox", mb.Name),
				slog.String("folder", name),
				slog.Any("error", err))
		}
		fr.TgtSize = tgtSize

		result.Folders = append(result.Folders, fr)
	}

	return result
}

// folderTotalSize selects a folder read-only and returns the sum of all
// message RFC822 sizes. Returns 0 immediately for empty folders.
func folderTotalSize(c *imap.Client, name string) (uint64, error) {
	info, err := c.SelectReadOnly(name)
	if err != nil {
		return 0, err
	}
	if info.NumMessages == 0 {
		return 0, nil
	}
	return c.FetchAllSizes()
}
