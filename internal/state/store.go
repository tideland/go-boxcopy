package state

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// stateFile is the JSON structure for persistence.
type stateFile struct {
	Version   int                      `json:"version"`
	UpdatedAt time.Time                `json:"updated_at"`
	Mailboxes map[string]*MailboxState `json:"mailboxes"`
}

const currentVersion = 1

// Load loads state from a file. If the file doesn't exist, returns empty state.
// Any leftover .tmp file from a previous interrupted save is removed first.
func Load(path string) (*State, error) {
	s := New(path)

	// Remove any stale temp file left by a previously interrupted save.
	// Errors are intentionally ignored — the .tmp file is not authoritative.
	os.Remove(path + ".tmp") //nolint:errcheck

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty state.
			return s, nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var sf stateFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse state file: %w", err)
	}

	// Handle version migrations if needed.
	if sf.Version > currentVersion {
		return nil, fmt.Errorf("state file version %d is newer than supported version %d",
			sf.Version, currentVersion)
	}

	// Initialize UIDSets that may have been serialized as null.
	for _, ms := range sf.Mailboxes {
		if ms.Folders == nil {
			ms.Folders = make(map[string]*FolderState)
		}
		for _, fs := range ms.Folders {
			if fs.SyncedUIDs == nil {
				fs.SyncedUIDs = make(UIDSet)
			}
			if fs.UIDMap == nil {
				fs.UIDMap = make(map[uint32]uint32)
			}
		}
	}

	s.mailboxes = sf.Mailboxes
	if s.mailboxes == nil {
		s.mailboxes = make(map[string]*MailboxState)
	}

	return s, nil
}

// Save saves the state to the configured file path.
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.saveUnlocked()
}

// saveUnlocked saves without acquiring the lock (caller must hold lock).
func (s *State) saveUnlocked() error {
	sf := stateFile{
		Version:   currentVersion,
		UpdatedAt: time.Now(),
		Mailboxes: s.mailboxes,
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize state: %w", err)
	}

	// Ensure directory exists.
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write to temp file first for atomic update.
	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	// Rename temp file to actual file (atomic on most systems).
	if err := os.Rename(tempPath, s.path); err != nil {
		os.Remove(tempPath) //nolint:errcheck // Clean up temp file on failure.
		return fmt.Errorf("failed to save state file: %w", err)
	}

	s.dirty = false
	return nil
}

// SaveIfDirty saves the state only if there are unsaved changes.
func (s *State) SaveIfDirty() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.dirty {
		return nil
	}

	return s.saveUnlocked()
}

// MarshalJSON implements json.Marshaler for UIDSet.
func (u UIDSet) MarshalJSON() ([]byte, error) {
	uids := u.ToSlice()
	slices.Sort(uids)
	return json.Marshal(uids)
}

// UnmarshalJSON implements json.Unmarshaler for UIDSet.
func (u *UIDSet) UnmarshalJSON(data []byte) error {
	var uids []uint32
	if err := json.Unmarshal(data, &uids); err != nil {
		return err
	}

	*u = make(UIDSet, len(uids))
	for _, uid := range uids {
		(*u)[uid] = struct{}{}
	}
	return nil
}

// Stats returns statistics about the state.
type Stats struct {
	MailboxCount   int
	FolderCount    int
	TotalSyncedUIDs int
	LastUpdated    time.Time
}

// Stats returns statistics about the current state.
func (s *State) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := Stats{
		MailboxCount: len(s.mailboxes),
	}

	var latestSync time.Time
	for _, ms := range s.mailboxes {
		stats.FolderCount += len(ms.Folders)
		for _, fs := range ms.Folders {
			stats.TotalSyncedUIDs += len(fs.SyncedUIDs)
			if fs.LastSync.After(latestSync) {
				latestSync = fs.LastSync
			}
		}
	}
	stats.LastUpdated = latestSync

	return stats
}

// Export exports the state as a formatted string for debugging.
func (s *State) Export() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sf := stateFile{
		Version:   currentVersion,
		UpdatedAt: time.Now(),
		Mailboxes: s.mailboxes,
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Sprintf("error: %v", err)
	}
	return string(data)
}
