package mailbox

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"fmt"
	"log/slog"
	"sort"

	"tideland.dev/go/boxcopy/internal/imap"
)

// copyFolders copies folder structure from source to target.
func (m *Mailbox) copyFolders() error {
	m.mu.RLock()
	connected := m.sourceClient != nil && m.targetClient != nil
	m.mu.RUnlock()
	if !connected {
		return fmt.Errorf("not connected")
	}

	m.logger.Debug("copying folder structure")

	// Get folders from source.
	sourceFolders, err := m.sourceClient.ListFolders()
	if err != nil {
		return fmt.Errorf("failed to list source folders: %w", err)
	}

	// Get folders from target.
	targetFolders, err := m.targetClient.ListFolders()
	if err != nil {
		return fmt.Errorf("failed to list target folders: %w", err)
	}

	// Build target folder set for quick lookup.
	targetSet := make(map[string]bool)
	for _, f := range targetFolders {
		targetSet[f.Name] = true
	}

	// Create missing folders on target.
	// Sort by name to ensure parent folders are created first.
	sort.Slice(sourceFolders, func(i, j int) bool {
		return sourceFolders[i].Name < sourceFolders[j].Name
	})

	for _, folder := range sourceFolders {
		if folder.IsNoSelect() {
			continue
		}
		if !targetSet[folder.Name] {
			m.logger.Debug("creating folder on target", slog.String("folder", folder.Name))
			if err := m.targetClient.CreateFolder(folder.Name); err != nil {
				m.logger.Warn("failed to create folder",
					slog.String("folder", folder.Name),
					slog.Any("error", err))
				// Continue with other folders.
			} else {
				m.addFoldersCreated(1)
			}
		}
	}

	m.mu.RLock()
	created := m.foldersCreated
	m.mu.RUnlock()

	m.logger.Debug("folder copy completed",
		slog.Int("source_folders", len(sourceFolders)),
		slog.Int64("folders_created", created))

	return nil
}

// copyFolder copies a single folder's messages from source to target.
func (m *Mailbox) copyFolder(folderName string) error {
	m.mu.RLock()
	connected := m.sourceClient != nil && m.targetClient != nil
	m.mu.RUnlock()
	if !connected {
		return fmt.Errorf("not connected")
	}

	// Select folder on source.
	sourceInfo, err := m.sourceClient.Select(folderName)
	if err != nil {
		return fmt.Errorf("failed to select source folder %s: %w", folderName, err)
	}

	// Ensure the folder exists on target.
	if err := m.targetClient.EnsureFolder(folderName); err != nil {
		return fmt.Errorf("failed to ensure target folder %s: %w", folderName, err)
	}

	// Skip message copy for empty source folders — FETCH 1:* is invalid with 0 messages.
	if sourceInfo.NumMessages == 0 {
		m.logger.Debug("folder is empty, skipping message copy", slog.String("folder", folderName))
		return nil
	}

	_, err = m.targetClient.Select(folderName)
	if err != nil {
		return fmt.Errorf("failed to select target folder %s: %w", folderName, err)
	}

	// Check UIDValidity — if changed, reset sync state.
	if m.state.SetUIDValidity(m.config.Name, folderName, sourceInfo.UIDValidity) {
		m.logger.Warn("UIDValidity changed, resetting state",
			slog.String("folder", folderName))
	}

	// Get all messages from source (metadata only).
	messages, err := m.sourceClient.FetchAll(imap.DefaultFetchOptions())
	if err != nil {
		return fmt.Errorf("failed to fetch messages from %s: %w", folderName, err)
	}

	if len(messages) == 0 {
		m.logger.Debug("no messages in folder", slog.String("folder", folderName))
		return nil
	}

	// Filter out already copied messages.
	allUIDs := make([]uint32, len(messages))
	for i, msg := range messages {
		allUIDs[i] = msg.UID
	}
	unsyncedUIDs := m.state.GetUnsyncedUIDs(m.config.Name, folderName, allUIDs)

	if len(unsyncedUIDs) == 0 {
		m.logger.Debug("all messages already copied",
			slog.String("folder", folderName),
			slog.Int("total", len(messages)))
		m.addSkipped(int64(len(messages)))
		return nil
	}

	m.logger.Info("copying messages",
		slog.String("folder", folderName),
		slog.Int("total", len(messages)),
		slog.Int("to_copy", len(unsyncedUIDs)))

	// Fetch and copy messages in batches to avoid high memory usage.
	// Use a channel pipeline to overlap fetching and copying.
	type batch struct {
		msgs []imap.Message
		err  error
	}
	// Buffer allows one batch to be fetched while another is processed.
	batchChan := make(chan batch, 2)

	// Fetcher Goroutine
	go func() {
		defer close(batchChan)
		const batchSize = 50
		total := len(unsyncedUIDs)

		for start := 0; start < total; start += batchSize {
			// Check context cancellation
			if m.ctx.Err() != nil {
				return
			}

			end := start + batchSize
			if end > total {
				end = total
			}

			batchUIDs := unsyncedUIDs[start:end]
			msgs, err := m.sourceClient.FetchByUID(batchUIDs, imap.FullFetchOptions())

			batchChan <- batch{msgs: msgs, err: err}

			if err != nil {
				return
			}
		}
	}()

	// Copier Loop
	total := len(unsyncedUIDs)
	nextMilestone := m.progress
	currentIdx := 0
	var copyErr error

	for b := range batchChan {
		if b.err != nil {
			m.logger.Warn("batch fetch failed", slog.Any("error", b.err))
			if copyErr == nil {
				copyErr = fmt.Errorf("fetch failed: %w", b.err)
			}
			// Drain channel if needed, but loop continues to consume remaining batches if any?
			// Fetcher stops on error, so channel will close.
			continue
		}

		for _, msg := range b.msgs {
			// Check context in copier loop too
			if m.ctx.Err() != nil {
				if copyErr == nil {
					copyErr = m.ctx.Err()
				}
				break
			}

			if err := m.copyMessage(folderName, &msg); err != nil {
				m.logger.Warn("failed to copy message",
					slog.String("folder", folderName),
					slog.Uint64("uid", uint64(msg.UID)),
					slog.Any("error", err))
				// Continue with other messages.
			}

			currentIdx++

			// Progress milestone logging.
			if total > 0 && nextMilestone > 0 && nextMilestone <= 100 {
				pct := currentIdx * 100 / total
				if pct >= nextMilestone {
					m.logger.Info("copy progress",
						slog.String("folder", folderName),
						slog.Int("percent", pct),
						slog.Int("done", currentIdx),
						slog.Int("total", total))
					nextMilestone += m.progress
				}
			}
		}
	}

	if copyErr != nil {
		return copyErr
	}

	// Update last sync time for folder.
	m.state.UpdateLastSync(m.config.Name, folderName)

	return nil
}

// copyMessage copies a single message to the target.
func (m *Mailbox) copyMessage(folderName string, msg *imap.Message) error {
	// Reject messages with invalid UID (0 is not valid per IMAP spec).
	if msg.UID == 0 {
		return fmt.Errorf("message has invalid UID 0 in folder %s", folderName)
	}

	// Skip if already copied (double check).
	if m.state.IsSynced(m.config.Name, folderName, msg.UID) {
		m.addSkipped(1)
		return nil
	}

	// Skip deleted messages.
	if msg.IsDeleted() {
		m.logger.Debug("skipping deleted message",
			slog.String("folder", folderName),
			slog.Uint64("uid", uint64(msg.UID)))
		m.addSkipped(1)
		return nil
	}

	// Apply throttle if configured.
	if m.throttle != nil {
		if err := m.throttle.Process(m.ctx, func() error {
			return m.appendMessage(folderName, msg)
		}); err != nil {
			return err
		}
	} else {
		if err := m.appendMessage(folderName, msg); err != nil {
			return err
		}
	}

	return nil
}

// appendMessage appends a message to the target folder and records the mapping.
func (m *Mailbox) appendMessage(folderName string, msg *imap.Message) error {
	targetMsg := &imap.Message{
		Flags:        msg.Flags,
		InternalDate: msg.InternalDate,
		Body:         msg.Body,
	}

	targetUID, err := m.targetClient.Append(folderName, targetMsg)
	if err != nil {
		return fmt.Errorf("failed to append message UID %d: %w", msg.UID, err)
	}

	// Mark as copied and record source→target UID mapping.
	m.state.MarkSynced(m.config.Name, folderName, msg.UID)
	m.state.SetUIDMapping(m.config.Name, folderName, msg.UID, targetUID)

	m.addCopied(1, int64(len(msg.Body)))

	m.logger.Debug("message copied",
		slog.String("folder", folderName),
		slog.Uint64("uid", uint64(msg.UID)),
		slog.Int("size", len(msg.Body)))

	return nil
}
