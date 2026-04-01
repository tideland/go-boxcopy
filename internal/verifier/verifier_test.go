package verifier

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"errors"
	"fmt"
	"os"
	"testing"

	"tideland.dev/go/asserts/verify"

	"tideland.dev/go/boxcopy/internal/config"
)

// TestFolderResultOK tests the FolderResult.OK method.
func TestFolderResultOK(t *testing.T) {
	t.Run("matching count and size", func(t *testing.T) {
		fr := FolderResult{Name: "INBOX", SrcCount: 10, TgtCount: 10, SrcSize: 1024, TgtSize: 1024}
		verify.True(t, fr.OK())
	})

	t.Run("count mismatch", func(t *testing.T) {
		fr := FolderResult{Name: "INBOX", SrcCount: 10, TgtCount: 9, SrcSize: 1024, TgtSize: 1024}
		verify.False(t, fr.OK())
	})

	t.Run("size mismatch", func(t *testing.T) {
		fr := FolderResult{Name: "INBOX", SrcCount: 10, TgtCount: 10, SrcSize: 1024, TgtSize: 900}
		verify.False(t, fr.OK())
	})

	t.Run("both count and size mismatch", func(t *testing.T) {
		fr := FolderResult{Name: "INBOX", SrcCount: 10, TgtCount: 8, SrcSize: 1024, TgtSize: 800}
		verify.False(t, fr.OK())
	})

	t.Run("empty folder matches", func(t *testing.T) {
		fr := FolderResult{Name: "Drafts", SrcCount: 0, TgtCount: 0, SrcSize: 0, TgtSize: 0}
		verify.True(t, fr.OK())
	})
}

// TestMailboxResultOK tests the MailboxResult.OK method.
func TestMailboxResultOK(t *testing.T) {
	t.Run("all folders match, no error", func(t *testing.T) {
		r := MailboxResult{
			Name: "user1",
			Folders: []FolderResult{
				{Name: "INBOX", SrcCount: 5, TgtCount: 5, SrcSize: 500, TgtSize: 500},
				{Name: "Sent", SrcCount: 3, TgtCount: 3, SrcSize: 300, TgtSize: 300},
			},
		}
		verify.True(t, r.OK())
	})

	t.Run("connection error", func(t *testing.T) {
		r := MailboxResult{
			Name: "user1",
			Err:  errors.New("source connect failed: connection refused"),
		}
		verify.False(t, r.OK())
	})

	t.Run("missing folder on target", func(t *testing.T) {
		r := MailboxResult{
			Name:           "user1",
			MissingFolders: []string{"Archive"},
			Folders: []FolderResult{
				{Name: "INBOX", SrcCount: 5, TgtCount: 5, SrcSize: 500, TgtSize: 500},
			},
		}
		verify.False(t, r.OK())
	})

	t.Run("folder count mismatch", func(t *testing.T) {
		r := MailboxResult{
			Name: "user1",
			Folders: []FolderResult{
				{Name: "INBOX", SrcCount: 5, TgtCount: 4, SrcSize: 500, TgtSize: 400},
			},
		}
		verify.False(t, r.OK())
	})

	t.Run("extra folders on target do not fail", func(t *testing.T) {
		r := MailboxResult{
			Name:         "user1",
			ExtraFolders: []string{"NewFolder"},
			Folders: []FolderResult{
				{Name: "INBOX", SrcCount: 5, TgtCount: 5, SrcSize: 500, TgtSize: 500},
			},
		}
		verify.True(t, r.OK())
	})

	t.Run("no folders", func(t *testing.T) {
		r := MailboxResult{Name: "user1"}
		verify.True(t, r.OK())
	})
}

// TestVerifyMissingConfigPath tests that Verify returns an error for an empty config path.
func TestVerifyMissingConfigPath(t *testing.T) {
	_, err := Verify(&Options{
		ConfigPath:    "",
		EncryptionKey: "somekey",
	})
	verify.NotNil(t, err)
}

// TestVerifyMissingEncryptionKey tests that Verify returns an error for an empty key.
func TestVerifyMissingEncryptionKey(t *testing.T) {
	_, err := Verify(&Options{
		ConfigPath:    "/some/path/config.toml",
		EncryptionKey: "",
	})
	verify.NotNil(t, err)
}

// TestVerifyNonExistentConfig tests that Verify returns an error for a missing config file.
func TestVerifyNonExistentConfig(t *testing.T) {
	_, err := Verify(&Options{
		ConfigPath:    "/nonexistent/path/config.toml",
		EncryptionKey: "somekey",
	})
	verify.NotNil(t, err)
}

// TestVerifyRejectsInvalidConfig tests that Verify validates the config and returns
// an error for invalid settings (fix 2.1: verifier previously skipped Validate()).
func TestVerifyRejectsInvalidConfig(t *testing.T) {
	// Build a minimal valid TOML config with an invalid value.
	toml := `
[source]
host = "imap.src.com"

[target]
host = "imap.tgt.com"

[general]
log_level = "verbose"

[[mailbox]]
name = "u"
source_user = "u@src"
source_password = "x"
target_user = "u@tgt"
target_password = "y"
`
	f, err := os.CreateTemp("", "boxcopy_verify_test*.toml")
	verify.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	fmt.Fprint(f, toml)
	f.Close()

	_, err = Verify(&Options{
		ConfigPath:    f.Name(),
		EncryptionKey: "testkey",
	})
	verify.Error(t, err)
	verify.ErrorContains(t, err, "invalid config")
}

// TestVerifyLogLevelFromConfig tests that an empty ExplicitLogLevel falls back to the
// config's log_level without error, and that config loading succeeds (fix 2.6).
func TestVerifyLogLevelFromConfig(t *testing.T) {
	encKey := "testkey"
	srcPwd, err := config.EncryptPassword("p", encKey)
	verify.NoError(t, err)
	tgtPwd, err := config.EncryptPassword("p", encKey)
	verify.NoError(t, err)

	toml := fmt.Sprintf(`
[source]
host = "127.0.0.1"
port = 19993
tls = false
[target]
host = "127.0.0.1"
port = 19994
tls = false
[general]
log_level = "debug"
[[mailbox]]
name = "u"
source_user = "u@src"
source_password = %q
target_user = "u@tgt"
target_password = %q
`, srcPwd, tgtPwd)

	f, err := os.CreateTemp("", "boxcopy_verify_loglevel*.toml")
	verify.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	fmt.Fprint(f, toml)
	f.Close()

	// No ExplicitLogLevel → falls back to config's "debug".
	// Verify itself must return nil (config is valid); per-mailbox connect errors
	// are stored in MailboxResult.Err, not returned as a function error.
	results, err := Verify(&Options{ConfigPath: f.Name(), EncryptionKey: encKey})
	verify.NoError(t, err)
	verify.Positive(t, len(results))
	// Connect will fail (no server on those ports), but that is a per-mailbox error.
	verify.NotNil(t, results[0].Err)
}
