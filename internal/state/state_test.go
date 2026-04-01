package state

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"tideland.dev/go/asserts/verify"
)

// TestNewState tests creating a new state.
func TestNewState(t *testing.T) {
	s := New("/tmp/test.json")

	verify.Equal(t, s.Path(), "/tmp/test.json")
	verify.False(t, s.IsDirty())
	verify.Equal(t, len(s.Mailboxes()), 0)
}

// TestGetMailbox tests getting and creating mailbox state.
func TestGetMailbox(t *testing.T) {
	s := New("/tmp/test.json")

	ms := s.GetMailbox("user1")
	verify.NotNil(t, ms)
	verify.Equal(t, ms.Name, "user1")
	verify.True(t, s.IsDirty())

	// Getting same mailbox again should return same instance.
	ms2 := s.GetMailbox("user1")
	verify.Equal(t, ms, ms2)

	// Check mailbox list.
	names := s.Mailboxes()
	verify.Length(t, names, 1)
	verify.Equal(t, names[0], "user1")
}

// TestGetFolder tests getting and creating folder state.
func TestGetFolder(t *testing.T) {
	s := New("/tmp/test.json")

	fs := s.GetFolder("user1", "INBOX")
	verify.NotNil(t, fs)
	verify.Equal(t, fs.Name, "INBOX")

	// Getting same folder again should return same instance.
	fs2 := s.GetFolder("user1", "INBOX")
	verify.Equal(t, fs, fs2)

	// Check folder list.
	folders := s.Folders("user1")
	verify.Length(t, folders, 1)
	verify.Equal(t, folders[0], "INBOX")

	// Non-existent mailbox should return no folders.
	nonExistentFolders := s.Folders("nonexistent")
	verify.Length(t, nonExistentFolders, 0)
}

// TestUIDValidity tests UIDValidity tracking.
func TestUIDValidity(t *testing.T) {
	s := New("/tmp/test.json")

	// Initial set should not indicate change.
	changed := s.SetUIDValidity("user1", "INBOX", 12345)
	verify.False(t, changed)
	verify.Equal(t, s.GetUIDValidity("user1", "INBOX"), uint32(12345))

	// Same value should not indicate change.
	changed = s.SetUIDValidity("user1", "INBOX", 12345)
	verify.False(t, changed)

	// Different value should indicate change.
	changed = s.SetUIDValidity("user1", "INBOX", 67890)
	verify.True(t, changed)
	verify.Equal(t, s.GetUIDValidity("user1", "INBOX"), uint32(67890))

	// Non-existent should return 0.
	verify.Equal(t, s.GetUIDValidity("nonexistent", "INBOX"), uint32(0))
}

// TestUIDValidityResetsState tests that UIDValidity change resets sync state.
func TestUIDValidityResetsState(t *testing.T) {
	s := New("/tmp/test.json")

	// Set up initial state.
	s.SetUIDValidity("user1", "INBOX", 12345)
	s.MarkSynced("user1", "INBOX", 100)
	s.MarkSynced("user1", "INBOX", 200)

	verify.Equal(t, s.GetSyncedCount("user1", "INBOX"), 2)
	verify.True(t, s.IsSynced("user1", "INBOX", 100))

	// Change UIDValidity - should reset state.
	changed := s.SetUIDValidity("user1", "INBOX", 67890)
	verify.True(t, changed)
	verify.Equal(t, s.GetSyncedCount("user1", "INBOX"), 0)
	verify.False(t, s.IsSynced("user1", "INBOX", 100))
}

// TestMarkSynced tests marking UIDs as synced.
func TestMarkSynced(t *testing.T) {
	s := New("/tmp/test.json")

	s.MarkSynced("user1", "INBOX", 100)
	verify.True(t, s.IsSynced("user1", "INBOX", 100))
	verify.False(t, s.IsSynced("user1", "INBOX", 200))
	verify.Equal(t, s.GetHighestSyncedUID("user1", "INBOX"), uint32(100))

	s.MarkSynced("user1", "INBOX", 200)
	verify.True(t, s.IsSynced("user1", "INBOX", 200))
	verify.Equal(t, s.GetHighestSyncedUID("user1", "INBOX"), uint32(200))

	// Lower UID shouldn't change highest.
	s.MarkSynced("user1", "INBOX", 50)
	verify.Equal(t, s.GetHighestSyncedUID("user1", "INBOX"), uint32(200))
}

// TestMarkMultipleSynced tests marking multiple UIDs at once.
func TestMarkMultipleSynced(t *testing.T) {
	s := New("/tmp/test.json")

	s.MarkMultipleSynced("user1", "INBOX", []uint32{100, 200, 300})
	verify.True(t, s.IsSynced("user1", "INBOX", 100))
	verify.True(t, s.IsSynced("user1", "INBOX", 200))
	verify.True(t, s.IsSynced("user1", "INBOX", 300))
	verify.Equal(t, s.GetSyncedCount("user1", "INBOX"), 3)
	verify.Equal(t, s.GetHighestSyncedUID("user1", "INBOX"), uint32(300))

	// Empty slice should be no-op.
	s.MarkMultipleSynced("user1", "INBOX", []uint32{})
	verify.Equal(t, s.GetSyncedCount("user1", "INBOX"), 3)
}

// TestGetUnsyncedUIDs tests filtering unsynced UIDs.
func TestGetUnsyncedUIDs(t *testing.T) {
	s := New("/tmp/test.json")

	s.MarkMultipleSynced("user1", "INBOX", []uint32{100, 200, 300})

	allUIDs := []uint32{100, 200, 300, 400, 500}
	unsynced := s.GetUnsyncedUIDs("user1", "INBOX", allUIDs)

	verify.Length(t, unsynced, 2)
	verify.True(t, contains(unsynced, 400))
	verify.True(t, contains(unsynced, 500))
	verify.False(t, contains(unsynced, 100))

	// Non-existent mailbox should return all UIDs.
	unsynced = s.GetUnsyncedUIDs("nonexistent", "INBOX", allUIDs)
	verify.Equal(t, len(unsynced), 5)
}

// TestLastSync tests last sync timestamp tracking.
func TestLastSync(t *testing.T) {
	s := New("/tmp/test.json")

	// Initial should be zero.
	lastSync := s.GetLastSync("user1", "INBOX")
	verify.True(t, lastSync.IsZero())

	// Update and verify.
	before := time.Now()
	s.UpdateLastSync("user1", "INBOX")
	after := time.Now()

	lastSync = s.GetLastSync("user1", "INBOX")
	verify.Between(t, lastSync, before, after)

	// Check mailbox sync count.
	ms := s.GetMailbox("user1")
	verify.Equal(t, ms.SyncCount, int64(1))
}

// TestClearFolder tests clearing folder state.
func TestClearFolder(t *testing.T) {
	s := New("/tmp/test.json")

	s.SetUIDValidity("user1", "INBOX", 12345)
	s.MarkMultipleSynced("user1", "INBOX", []uint32{100, 200, 300})
	s.UpdateLastSync("user1", "INBOX")

	verify.Equal(t, s.GetSyncedCount("user1", "INBOX"), 3)

	s.ClearFolder("user1", "INBOX")

	verify.Equal(t, s.GetSyncedCount("user1", "INBOX"), 0)
	verify.Equal(t, s.GetUIDValidity("user1", "INBOX"), uint32(0))
	verify.True(t, s.GetLastSync("user1", "INBOX").IsZero())
}

// TestUpdateMailboxLastSync tests that UpdateMailboxLastSync updates the mailbox
// timestamp without creating a phantom folder entry (fix 1.4).
func TestUpdateMailboxLastSync(t *testing.T) {
	s := New("/tmp/test.json")

	before := time.Now()
	s.UpdateMailboxLastSync("user1")
	after := time.Now()

	// Mailbox LastSync and SyncCount should be updated.
	ms := s.GetMailbox("user1")
	verify.Between(t, ms.LastSync, before, after)
	verify.Equal(t, ms.SyncCount, int64(1))

	// No folder entries must have been created.
	folders := s.Folders("user1")
	verify.Length(t, folders, 0)
}

// TestClearMailbox tests clearing entire mailbox state.
func TestClearMailbox(t *testing.T) {
	s := New("/tmp/test.json")

	s.MarkSynced("user1", "INBOX", 100)
	s.MarkSynced("user1", "Sent", 200)

	verify.Length(t, s.Folders("user1"), 2)

	s.ClearMailbox("user1")

	verify.Length(t, s.Folders("user1"), 0)
}

// TestUIDSet tests UIDSet methods.
func TestUIDSet(t *testing.T) {
	u := make(UIDSet)

	u.Add(100)
	u.Add(200)
	u.Add(300)

	verify.Equal(t, u.Len(), 3)
	verify.True(t, u.Contains(100))
	verify.True(t, u.Contains(200))
	verify.False(t, u.Contains(400))

	u.Remove(200)
	verify.Equal(t, u.Len(), 2)
	verify.False(t, u.Contains(200))

	slice := u.ToSlice()
	verify.Length(t, slice, 2)
}

// TestSaveLoad tests state persistence.
func TestSaveLoad(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "state.json")

	// Create and populate state.
	s := New(path)
	s.SetUIDValidity("user1", "INBOX", 12345)
	s.MarkMultipleSynced("user1", "INBOX", []uint32{100, 200, 300})
	s.UpdateLastSync("user1", "INBOX")

	// Save.
	err := s.Save()
	verify.NoError(t, err)
	verify.False(t, s.IsDirty())

	// Load into new state.
	s2, err := Load(path)
	verify.NoError(t, err)

	// Verify loaded state.
	verify.Equal(t, s2.GetUIDValidity("user1", "INBOX"), uint32(12345))
	verify.Equal(t, s2.GetSyncedCount("user1", "INBOX"), 3)
	verify.True(t, s2.IsSynced("user1", "INBOX", 100))
	verify.True(t, s2.IsSynced("user1", "INBOX", 200))
	verify.True(t, s2.IsSynced("user1", "INBOX", 300))
}

// TestLoadNonExistent tests loading from non-existent file.
func TestLoadNonExistent(t *testing.T) {
	s, err := Load("/tmp/nonexistent_state_file_12345.json")
	verify.NoError(t, err)
	verify.NotNil(t, s)
	verify.Length(t, s.Mailboxes(), 0)
}

// TestSaveIfDirty tests conditional save.
func TestSaveIfDirty(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "state.json")

	s := New(path)

	// Not dirty, shouldn't create file.
	err := s.SaveIfDirty()
	verify.NoError(t, err)
	_, err = os.Stat(path)
	verify.True(t, os.IsNotExist(err))

	// Make dirty and save.
	s.MarkSynced("user1", "INBOX", 100)
	err = s.SaveIfDirty()
	verify.NoError(t, err)
	_, err = os.Stat(path)
	verify.NoError(t, err)
}

// TestStats tests state statistics.
func TestStats(t *testing.T) {
	s := New("/tmp/test.json")

	s.MarkMultipleSynced("user1", "INBOX", []uint32{1, 2, 3})
	s.MarkMultipleSynced("user1", "Sent", []uint32{4, 5})
	s.MarkMultipleSynced("user2", "INBOX", []uint32{6, 7, 8, 9})
	s.UpdateLastSync("user2", "INBOX")

	stats := s.Stats()

	verify.Equal(t, stats.MailboxCount, 2)
	verify.Equal(t, stats.FolderCount, 3)
	verify.Equal(t, stats.TotalSyncedUIDs, 9)
	verify.False(t, stats.LastUpdated.IsZero())
}

// TestExport tests state export.
func TestExport(t *testing.T) {
	s := New("/tmp/test.json")
	s.MarkSynced("user1", "INBOX", 100)

	export := s.Export()
	verify.Positive(t, len(export))
	verify.True(t, stringContains(export, "user1"))
	verify.True(t, stringContains(export, "INBOX"))
}

// Helper functions

func contains(slice []uint32, val uint32) bool {
	return slices.Contains(slice, val)
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
