package state

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

// Package state manages sync state persistence.
//
// Uses simple ASCII file format (pure Go, no C dependencies).
//
// Tracks:
//   - Last sync timestamps per mailbox/folder
//   - Message UIDs already synced
//   - Incomplete sync recovery data
//   - Sync session IDs (using Tideland Go UUID)
