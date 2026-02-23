package state

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"maps"
	"sync"
	"time"
)

// State manages sync state for all mailboxes.
type State struct {
	mu        sync.RWMutex
	path      string
	mailboxes map[string]*MailboxState
	dirty     bool
}

// MailboxState holds sync state for a single mailbox (user account).
type MailboxState struct {
	Name      string                  `json:"name"`
	Folders   map[string]*FolderState `json:"folders"`
	LastSync  time.Time               `json:"last_sync"`
	SyncCount int64                   `json:"sync_count"`
}

// FolderState holds sync state for a single folder within a mailbox.
type FolderState struct {
	Name             string            `json:"name"`
	UIDValidity      uint32            `json:"uid_validity"`
	UIDNext          uint32            `json:"uid_next"`
	LastSync         time.Time         `json:"last_sync"`
	SyncedUIDs       UIDSet            `json:"synced_uids"`
	HighestUID       uint32            `json:"highest_uid"`
	MessageCount     uint32            `json:"message_count"`
	UIDMap           map[uint32]uint32 `json:"uid_map"` // source UID → target UID
}

// UIDSet is a set of message UIDs for efficient lookup.
type UIDSet map[uint32]struct{}

// New creates a new State instance.
func New(path string) *State {
	return &State{
		path:      path,
		mailboxes: make(map[string]*MailboxState),
	}
}

// Path returns the state file path.
func (s *State) Path() string {
	return s.path
}

// IsDirty returns true if the state has unsaved changes.
func (s *State) IsDirty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.dirty
}

// GetMailbox returns the state for a mailbox, creating it if necessary.
func (s *State) GetMailbox(name string) *MailboxState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateMailbox(name)
}

// getOrCreateMailbox returns or creates a MailboxState. Must be called with s.mu held.
func (s *State) getOrCreateMailbox(name string) *MailboxState {
	if ms, ok := s.mailboxes[name]; ok {
		return ms
	}

	ms := &MailboxState{
		Name:    name,
		Folders: make(map[string]*FolderState),
	}
	s.mailboxes[name] = ms
	s.dirty = true
	return ms
}

// GetFolder returns the state for a folder, creating it if necessary.
func (s *State) GetFolder(mailbox, folder string) *FolderState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getOrCreateFolder(mailbox, folder)
}

// getOrCreateFolder returns or creates a FolderState. Must be called with s.mu held.
func (s *State) getOrCreateFolder(mailbox, folder string) *FolderState {
	ms := s.getOrCreateMailbox(mailbox)

	if fs, ok := ms.Folders[folder]; ok {
		return fs
	}

	fs := &FolderState{
		Name:       folder,
		SyncedUIDs: make(UIDSet),
		UIDMap:     make(map[uint32]uint32),
	}
	ms.Folders[folder] = fs
	s.dirty = true
	return fs
}

// Mailboxes returns all mailbox names.
func (s *State) Mailboxes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.mailboxes))
	for name := range s.mailboxes {
		names = append(names, name)
	}
	return names
}

// Folders returns all folder names for a mailbox.
func (s *State) Folders(mailbox string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return nil
	}

	names := make([]string, 0, len(ms.Folders))
	for name := range ms.Folders {
		names = append(names, name)
	}
	return names
}

// SetUIDValidity sets the UIDValidity for a folder.
// Returns true if the value changed (indicating mailbox was rebuilt).
func (s *State) SetUIDValidity(mailbox, folder string, validity uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	fs := s.getOrCreateFolder(mailbox, folder)

	if fs.UIDValidity == validity {
		return false
	}

	changed := fs.UIDValidity != 0 && fs.UIDValidity != validity
	fs.UIDValidity = validity

	if changed {
		// UIDValidity changed - mailbox was rebuilt, reset sync state.
		fs.SyncedUIDs = make(UIDSet)
		fs.HighestUID = 0
		fs.LastSync = time.Time{}
	}

	s.dirty = true
	return changed
}

// GetUIDValidity returns the stored UIDValidity for a folder.
func (s *State) GetUIDValidity(mailbox, folder string) uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return 0
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return 0
	}
	return fs.UIDValidity
}

// MarkSynced marks a message UID as synced.
// Returns false without modifying state if uid is 0 (invalid per IMAP spec).
func (s *State) MarkSynced(mailbox, folder string, uid uint32) bool {
	if uid == 0 {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fs := s.getOrCreateFolder(mailbox, folder)
	fs.SyncedUIDs.Add(uid)
	if uid > fs.HighestUID {
		fs.HighestUID = uid
	}
	s.dirty = true
	return true
}

// MarkMultipleSynced marks multiple message UIDs as synced.
// UIDs with value 0 are silently skipped (invalid per IMAP spec).
func (s *State) MarkMultipleSynced(mailbox, folder string, uids []uint32) {
	if len(uids) == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fs := s.getOrCreateFolder(mailbox, folder)
	for _, uid := range uids {
		if uid == 0 {
			continue
		}
		fs.SyncedUIDs.Add(uid)
		if uid > fs.HighestUID {
			fs.HighestUID = uid
		}
	}
	s.dirty = true
}

// SetUIDMapping records the source→target UID mapping for a synced message.
func (s *State) SetUIDMapping(mailbox, folder string, sourceUID, targetUID uint32) {
	if sourceUID == 0 || targetUID == 0 {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fs := s.getOrCreateFolder(mailbox, folder)
	if fs.UIDMap == nil {
		fs.UIDMap = make(map[uint32]uint32)
	}
	fs.UIDMap[sourceUID] = targetUID
	s.dirty = true
}

// GetTargetUID returns the target UID for a given source UID, and whether it was found.
func (s *State) GetTargetUID(mailbox, folder string, sourceUID uint32) (uint32, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return 0, false
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return 0, false
	}
	targetUID, ok := fs.UIDMap[sourceUID]
	return targetUID, ok
}

// GetUIDMap returns a copy of the source→target UID map for a folder.
func (s *State) GetUIDMap(mailbox, folder string) map[uint32]uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return nil
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return nil
	}
	result := make(map[uint32]uint32, len(fs.UIDMap))
	maps.Copy(result, fs.UIDMap)
	return result
}

// IsSynced checks if a message UID has been synced.
func (s *State) IsSynced(mailbox, folder string, uid uint32) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return false
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return false
	}
	return fs.SyncedUIDs.Contains(uid)
}

// GetUnsyncedUIDs returns UIDs that haven't been synced yet.
func (s *State) GetUnsyncedUIDs(mailbox, folder string, allUIDs []uint32) []uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return allUIDs
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return allUIDs
	}

	unsynced := make([]uint32, 0, len(allUIDs))
	for _, uid := range allUIDs {
		if !fs.SyncedUIDs.Contains(uid) {
			unsynced = append(unsynced, uid)
		}
	}
	return unsynced
}

// GetHighestSyncedUID returns the highest synced UID for a folder.
func (s *State) GetHighestSyncedUID(mailbox, folder string) uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return 0
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return 0
	}
	return fs.HighestUID
}

// GetSyncedCount returns the number of synced messages for a folder.
func (s *State) GetSyncedCount(mailbox, folder string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return 0
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return 0
	}
	return len(fs.SyncedUIDs)
}

// UpdateLastSync updates the last sync timestamp for a folder.
func (s *State) UpdateLastSync(mailbox, folder string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fs := s.getOrCreateFolder(mailbox, folder)
	now := time.Now()
	fs.LastSync = now

	if ms, ok := s.mailboxes[mailbox]; ok {
		ms.LastSync = now
		ms.SyncCount++
	}
	s.dirty = true
}

// GetLastSync returns the last sync timestamp for a folder.
func (s *State) GetLastSync(mailbox, folder string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return time.Time{}
	}
	fs, ok := ms.Folders[folder]
	if !ok {
		return time.Time{}
	}
	return fs.LastSync
}

// ClearFolder resets the sync state for a folder.
func (s *State) ClearFolder(mailbox, folder string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ms, ok := s.mailboxes[mailbox]
	if !ok {
		return
	}

	if fs, ok := ms.Folders[folder]; ok {
		fs.SyncedUIDs = make(UIDSet)
		fs.UIDMap = make(map[uint32]uint32)
		fs.HighestUID = 0
		fs.LastSync = time.Time{}
		fs.UIDValidity = 0
		s.dirty = true
	}
}

// ClearMailbox resets the sync state for an entire mailbox.
func (s *State) ClearMailbox(mailbox string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mailboxes[mailbox]; ok {
		s.mailboxes[mailbox] = &MailboxState{
			Name:    mailbox,
			Folders: make(map[string]*FolderState),
		}
		s.dirty = true
	}
}

// UIDSet methods

// Add adds a UID to the set.
func (u UIDSet) Add(uid uint32) {
	u[uid] = struct{}{}
}

// Remove removes a UID from the set.
func (u UIDSet) Remove(uid uint32) {
	delete(u, uid)
}

// Contains checks if a UID is in the set.
func (u UIDSet) Contains(uid uint32) bool {
	_, ok := u[uid]
	return ok
}

// Len returns the number of UIDs in the set.
func (u UIDSet) Len() int {
	return len(u)
}

// ToSlice converts the set to a slice.
func (u UIDSet) ToSlice() []uint32 {
	uids := make([]uint32, 0, len(u))
	for uid := range u {
		uids = append(uids, uid)
	}
	return uids
}
