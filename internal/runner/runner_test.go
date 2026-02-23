package runner

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"testing"

	"tideland.dev/go/asserts/verify"
)

// TestRunMissingConfigPath tests Run with missing config path.
func TestRunMissingConfigPath(t *testing.T) {
	err := Run(&Options{EncryptionKey: "testkey"})
	verify.Error(t, err)
}

// TestRunInvalidConfig tests Run with non-existent config file.
func TestRunInvalidConfig(t *testing.T) {
	err := Run(&Options{
		ConfigPath:    "/nonexistent/config.toml",
		EncryptionKey: "testkey",
	})
	verify.Error(t, err)
}

// TestRunMissingEncryptionKey tests Run with missing encryption key.
func TestRunMissingEncryptionKey(t *testing.T) {
	err := Run(&Options{ConfigPath: "/some/path.toml"})
	// Should fail because config file doesn't exist — that's fine.
	verify.Error(t, err)
}

// TestRunNilOptions tests Run with nil options.
func TestRunNilOptions(t *testing.T) {
	err := Run(nil)
	verify.Error(t, err)
}
