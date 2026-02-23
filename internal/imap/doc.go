package imap

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

// Package imap wraps the emersion/go-imap client library.
//
// Provides:
//   - Connection management (source and target servers)
//   - Folder operations (list, create, delete)
//   - Message operations (fetch, append, delete, flag sync)
//   - Credential encryption/decryption helpers
