package config

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"testing"

	"tideland.dev/go/asserts/verify"
)

// TestLoadStringMinimal tests loading a minimal valid configuration.
func TestLoadStringMinimal(t *testing.T) {
	toml := `
[source]
host = "imap.source.com"

[target]
host = "imap.target.com"

[[mailbox]]
name = "user1"
source_user = "user1@source.com"
source_password = "secret1"
target_user = "user1@target.com"
target_password = "secret2"
`
	cfg, err := LoadString(toml)
	verify.NoError(t, err)
	verify.NotNil(t, cfg)

	// Check source server defaults.
	verify.Equal(t, cfg.Source.Host, "imap.source.com")
	verify.Equal(t, cfg.Source.Port, 993)
	verify.True(t, cfg.Source.TLS)

	// Check target server defaults.
	verify.Equal(t, cfg.Target.Host, "imap.target.com")
	verify.Equal(t, cfg.Target.Port, 993)
	verify.True(t, cfg.Target.TLS)

	// Check general defaults.
	verify.Equal(t, cfg.General.LogLevel, "info")
	verify.Equal(t, cfg.General.Progress, 10)

	// Check copy_parameters defaults.
	verify.Equal(t, cfg.CopyParam.MessagesPerSecond, 10)
	verify.Equal(t, cfg.CopyParam.MaxConnections, 5)

	// Check mailbox.
	verify.Equal(t, len(cfg.Mailboxes), 1)
	verify.Equal(t, cfg.Mailboxes[0].Name, "user1")
	verify.Equal(t, cfg.Mailboxes[0].SourceUser, "user1@source.com")
	verify.Equal(t, cfg.Mailboxes[0].SourcePassword, "secret1")
	verify.Equal(t, cfg.Mailboxes[0].TargetUser, "user1@target.com")
	verify.Equal(t, cfg.Mailboxes[0].TargetPassword, "secret2")
}

// TestLoadStringComplete tests loading a complete configuration.
func TestLoadStringComplete(t *testing.T) {
	toml := `
[general]
state_file = "/var/lib/boxcopy/state.dat"
log_level = "debug"
progress = 25

[copy_parameters]
messages_per_second = 20
max_connections = 10

[source]
host = "imap.source.com"
port = 143
tls = false

[target]
host = "imap.target.com"
port = 993
tls = true

[[mailbox]]
name = "user1"
source_user = "user1@source.com"
source_password = "secret1"
target_user = "user1@target.com"
target_password = "secret2"

[[mailbox]]
name = "user2"
source_user = "user2@source.com"
source_password = "secret3"
target_user = "user2@target.com"
target_password = "secret4"
`
	cfg, err := LoadString(toml)
	verify.NoError(t, err)
	verify.NotNil(t, cfg)

	// Check general settings.
	verify.Equal(t, cfg.General.StateFile, "/var/lib/boxcopy/state.dat")
	verify.Equal(t, cfg.General.LogLevel, "debug")
	verify.Equal(t, cfg.General.Progress, 25)

	// Check copy_parameters settings.
	verify.Equal(t, cfg.CopyParam.MessagesPerSecond, 20)
	verify.Equal(t, cfg.CopyParam.MaxConnections, 10)

	// Check source server.
	verify.Equal(t, cfg.Source.Host, "imap.source.com")
	verify.Equal(t, cfg.Source.Port, 143)
	verify.False(t, cfg.Source.TLS)

	// Check target server.
	verify.Equal(t, cfg.Target.Host, "imap.target.com")
	verify.Equal(t, cfg.Target.Port, 993)
	verify.True(t, cfg.Target.TLS)

	// Check mailboxes.
	verify.Equal(t, len(cfg.Mailboxes), 2)
	verify.Equal(t, cfg.Mailboxes[0].Name, "user1")
	verify.Equal(t, cfg.Mailboxes[1].Name, "user2")
}

// TestLoadStringMissingSource tests error on missing source section.
func TestLoadStringMissingSource(t *testing.T) {
	toml := `
[target]
host = "imap.target.com"

[[mailbox]]
name = "user1"
source_user = "user1@source.com"
source_password = "secret1"
target_user = "user1@target.com"
target_password = "secret2"
`
	_, err := LoadString(toml)
	verify.Error(t, err)
	verify.ErrorContains(t, err, "source")
}

// TestLoadStringMissingTarget tests error on missing target section.
func TestLoadStringMissingTarget(t *testing.T) {
	toml := `
[source]
host = "imap.source.com"

[[mailbox]]
name = "user1"
source_user = "user1@source.com"
source_password = "secret1"
target_user = "user1@target.com"
target_password = "secret2"
`
	_, err := LoadString(toml)
	verify.Error(t, err)
	verify.ErrorContains(t, err, "target")
}

// TestLoadStringMissingMailbox tests error on missing mailbox section.
func TestLoadStringMissingMailbox(t *testing.T) {
	toml := `
[source]
host = "imap.source.com"

[target]
host = "imap.target.com"
`
	_, err := LoadString(toml)
	verify.Error(t, err)
	verify.ErrorContains(t, err, "mailbox")
}

// TestLoadStringMissingMailboxFields tests error on missing mailbox fields.
func TestLoadStringMissingMailboxFields(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		missing string
	}{
		{
			name: "missing name",
			toml: `
[source]
host = "imap.source.com"
[target]
host = "imap.target.com"
[[mailbox]]
source_user = "user1@source.com"
source_password = "secret1"
target_user = "user1@target.com"
target_password = "secret2"
`,
			missing: "name",
		},
		{
			name: "missing source_user",
			toml: `
[source]
host = "imap.source.com"
[target]
host = "imap.target.com"
[[mailbox]]
name = "user1"
source_password = "secret1"
target_user = "user1@target.com"
target_password = "secret2"
`,
			missing: "source_user",
		},
		{
			name: "missing source_password",
			toml: `
[source]
host = "imap.source.com"
[target]
host = "imap.target.com"
[[mailbox]]
name = "user1"
source_user = "user1@source.com"
target_user = "user1@target.com"
target_password = "secret2"
`,
			missing: "source_password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadString(tt.toml)
			verify.Error(t, err)
			verify.ErrorContains(t, err, tt.missing)
		})
	}
}

// TestConfigValidate tests configuration validation.
func TestConfigValidate(t *testing.T) {
	validConfig := func() *Config {
		return &Config{
			General: GeneralConfig{
				StateFile: "/var/lib/boxcopy/state.dat",
				LogLevel:  "info",
				Progress:  10,
			},
			CopyParam: CopyParameters{
				MessagesPerSecond: 10,
				MaxConnections:    5,
			},
			Source: ServerConfig{
				Host: "imap.source.com",
				Port: 993,
				TLS:  true,
			},
			Target: ServerConfig{
				Host: "imap.target.com",
				Port: 993,
				TLS:  true,
			},
			Mailboxes: []MailboxConfig{
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

	t.Run("valid config", func(t *testing.T) {
		cfg := validConfig()
		verify.NoError(t, cfg.Validate())
	})

	t.Run("empty source host", func(t *testing.T) {
		cfg := validConfig()
		cfg.Source.Host = ""
		verify.Error(t, cfg.Validate())
	})

	t.Run("empty target host", func(t *testing.T) {
		cfg := validConfig()
		cfg.Target.Host = ""
		verify.Error(t, cfg.Validate())
	})

	t.Run("no mailboxes", func(t *testing.T) {
		cfg := validConfig()
		cfg.Mailboxes = nil
		verify.Error(t, cfg.Validate())
	})

	t.Run("invalid messages_per_second", func(t *testing.T) {
		cfg := validConfig()
		cfg.CopyParam.MessagesPerSecond = 0
		verify.Error(t, cfg.Validate())
	})

	t.Run("invalid max_connections", func(t *testing.T) {
		cfg := validConfig()
		cfg.CopyParam.MaxConnections = 0
		verify.Error(t, cfg.Validate())
	})

	t.Run("invalid log_level", func(t *testing.T) {
		cfg := validConfig()
		cfg.General.LogLevel = "invalid"
		verify.Error(t, cfg.Validate())
	})
}
