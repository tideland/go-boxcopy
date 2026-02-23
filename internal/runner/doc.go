package runner

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

// Package runner provides the Run function for a single boxcopy pass.
//
// It loads config, decrypts passwords, optionally dry-runs or performs
// a full copy: safety confirmation, target cleanup, parallel mailbox copy.
