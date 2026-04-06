package runner

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"tideland.dev/go/asserts/verify"

	"tideland.dev/go/boxcopy/internal/config"
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

// TestDryRun tests the dryRun function directly with a valid config.
func TestDryRun(t *testing.T) {
	cfg := validTestConfig()
	err := dryRun(cfg, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	verify.NoError(t, err)
}

// TestConfirmCopyYES tests that confirmCopy succeeds when user types YES.
func TestConfirmCopyYES(t *testing.T) {
	err := confirmCopy(validTestConfig(), strings.NewReader("YES\n"))
	verify.NoError(t, err)
}

// TestConfirmCopyDeclined tests that confirmCopy fails for any answer other than YES.
func TestConfirmCopyDeclined(t *testing.T) {
	for _, answer := range []string{"no\n", "NO\n", "yes\n", "\n", "NOPE\n", "Yes\n"} {
		err := confirmCopy(validTestConfig(), strings.NewReader(answer))
		verify.Error(t, err)
		verify.ErrorContains(t, err, "cancelled")
	}
}

// writeTempConfig writes a minimal valid TOML config with encrypted passwords
// to a temp file and returns the file path. The file is removed at test cleanup.
func writeTempConfig(t *testing.T, key string) string {
	t.Helper()
	srcPwd, err := config.EncryptPassword("sourcepwd", key)
	verify.NoError(t, err)
	tgtPwd, err := config.EncryptPassword("targetpwd", key)
	verify.NoError(t, err)

	toml := fmt.Sprintf(`
[source]
host = "imap.source.com"

[target]
host = "imap.target.com"

[[mailbox]]
name = "user1"
source_user = "user1@source.com"
source_password = %q
target_user = "user1@target.com"
target_password = %q
`, srcPwd, tgtPwd)

	f, err := os.CreateTemp("", "boxcopy*.toml")
	verify.NoError(t, err)
	t.Cleanup(func() {
		if err := os.Remove(f.Name()); err != nil {
			t.Errorf("failed to remove temp file: %v", err)
		}
	})
	_, err = f.WriteString(toml)
	verify.NoError(t, err)
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}
	return f.Name()
}

// TestRunDryRunFromFile tests a full dry-run through Run() using a temp config file
// with properly encrypted passwords — no IMAP connection is made.
func TestRunDryRunFromFile(t *testing.T) {
	key := "testkey"
	err := Run(&Options{
		ConfigPath:    writeTempConfig(t, key),
		EncryptionKey: key,
		Perform:       false,
	})
	verify.NoError(t, err)
}

// TestRunPerformDeclined tests that Run with Perform=true stops at the
// confirmation prompt when the user does not type YES.
func TestRunPerformDeclined(t *testing.T) {
	key := "testkey"
	err := Run(&Options{
		ConfigPath:    writeTempConfig(t, key),
		EncryptionKey: key,
		Perform:       true,
		Input:         strings.NewReader("NO\n"),
	})
	verify.Error(t, err)
	verify.ErrorContains(t, err, "cancelled")
}

// validTestConfig returns a minimal valid *config.Config for use in unit tests.
func validTestConfig() *config.Config {
	return &config.Config{
		General: config.GeneralConfig{
			StateFile: "/tmp/boxcopy_test_state.dat",
			LogLevel:  "info",
			Progress:  10,
		},
		CopyParam: config.CopyParameters{
			MessagesPerSecond: 10,
			MaxConnections:    5,
		},
		Source: config.ServerConfig{
			Host: "imap.source.com",
			Port: 993,
			TLS:  true,
		},
		Target: config.ServerConfig{
			Host: "imap.target.com",
			Port: 993,
			TLS:  true,
		},
		Mailboxes: []config.MailboxConfig{
			{
				Name:           "user1",
				SourceUser:     "user1@source.com",
				SourcePassword: "secret1",
				TargetUser:     "user1@target.com",
				TargetPassword: "secret2",
			},
		},
	}
}
