package config

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

// Package config handles TOML configuration parsing and management
// using Tideland Go TOML.
//
// Configuration includes:
//   - Source and target IMAP server settings
//   - User/mailbox definitions with encrypted credentials
//   - Throttling settings (messages/time, connections)
//   - Daemon settings (PID file, logging)
