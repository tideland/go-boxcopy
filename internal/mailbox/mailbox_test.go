package mailbox

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"testing"

	"tideland.dev/go/asserts/verify"

	"tideland.dev/go/boxcopy/internal/config"
	"tideland.dev/go/boxcopy/internal/state"
)

// TestStatusString tests Status.String method.
func TestStatusString(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusIdle, "idle"},
		{StatusConnecting, "connecting"},
		{StatusCopying, "copying"},
		{StatusDisconnecting, "disconnecting"},
		{StatusError, "error"},
		{StatusStopped, "stopped"},
		{Status(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			verify.Equal(t, tt.status.String(), tt.expected)
		})
	}
}

// TestDefaultOptions tests DefaultOptions function.
func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	verify.NotNil(t, opts)
	verify.NotNil(t, opts.Logger)
}

// TestNewMailbox tests creating a new mailbox.
func TestNewMailbox(t *testing.T) {
	mbConfig := config.MailboxConfig{
		Name:           "testuser",
		SourceUser:     "test@source.com",
		SourcePassword: "sourcepass",
		TargetUser:     "test@target.com",
		TargetPassword: "targetpass",
	}

	sourceServer := config.ServerConfig{
		Host: "imap.source.com",
		Port: 993,
		TLS:  true,
	}

	targetServer := config.ServerConfig{
		Host: "imap.target.com",
		Port: 993,
		TLS:  true,
	}

	syncState := state.New("/tmp/test_state.json")

	mb := New(mbConfig, sourceServer, targetServer, syncState, nil)
	verify.NotNil(t, mb)
	verify.Equal(t, mb.Name(), "testuser")
}

// TestMailboxStatus tests getting mailbox status.
func TestMailboxStatus(t *testing.T) {
	mbConfig := config.MailboxConfig{
		Name:           "statustest",
		SourceUser:     "test@source.com",
		SourcePassword: "sourcepass",
		TargetUser:     "test@target.com",
		TargetPassword: "targetpass",
	}

	sourceServer := config.ServerConfig{
		Host: "imap.source.com",
		Port: 993,
		TLS:  true,
	}

	targetServer := config.ServerConfig{
		Host: "imap.target.com",
		Port: 993,
		TLS:  true,
	}

	syncState := state.New("/tmp/test_state.json")

	mb := New(mbConfig, sourceServer, targetServer, syncState, nil)

	status := mb.Status()
	verify.Equal(t, status.Name, "statustest")
	verify.Equal(t, status.Status, StatusIdle)
	verify.False(t, status.IsConnected)
	verify.Equal(t, status.SyncCount, int64(0))
}

// TestStatusInfo tests StatusInfo struct.
func TestStatusInfo(t *testing.T) {
	info := StatusInfo{
		Name:            "testmailbox",
		Status:          StatusCopying,
		SyncCount:       5,
		MessagesCopied:  100,
		MessagesSkipped: 10,
		BytesCopied:     1024000,
		IsConnected:     true,
	}

	verify.Equal(t, info.Name, "testmailbox")
	verify.Equal(t, info.Status, StatusCopying)
	verify.Equal(t, info.SyncCount, int64(5))
	verify.Equal(t, info.MessagesCopied, int64(100))
	verify.Equal(t, info.MessagesSkipped, int64(10))
	verify.Equal(t, info.BytesCopied, int64(1024000))
	verify.True(t, info.IsConnected)
}

// TestMailboxWithCustomOptions tests creating mailbox with custom options.
func TestMailboxWithCustomOptions(t *testing.T) {
	mbConfig := config.MailboxConfig{
		Name:           "customopts",
		SourceUser:     "test@source.com",
		SourcePassword: "sourcepass",
		TargetUser:     "test@target.com",
		TargetPassword: "targetpass",
	}

	sourceServer := config.ServerConfig{
		Host: "imap.source.com",
		Port: 993,
		TLS:  true,
	}

	targetServer := config.ServerConfig{
		Host: "imap.target.com",
		Port: 993,
		TLS:  true,
	}

	syncState := state.New("/tmp/test_state.json")

	opts := &Options{
		Logger: nil, // Will use default.
	}

	mb := New(mbConfig, sourceServer, targetServer, syncState, opts)
	verify.NotNil(t, mb)
	verify.Equal(t, mb.Name(), "customopts")
}

// TestMailboxConnectWithoutServer tests that connect fails gracefully
// when no server is available.
func TestMailboxConnectWithoutServer(t *testing.T) {
	mbConfig := config.MailboxConfig{
		Name:           "noserver",
		SourceUser:     "test@source.com",
		SourcePassword: "sourcepass",
		TargetUser:     "test@target.com",
		TargetPassword: "targetpass",
	}

	// Use localhost with unlikely ports.
	sourceServer := config.ServerConfig{
		Host: "127.0.0.1",
		Port: 19993,
		TLS:  true,
	}

	targetServer := config.ServerConfig{
		Host: "127.0.0.1",
		Port: 19994,
		TLS:  true,
	}

	syncState := state.New("/tmp/test_state.json")

	mb := New(mbConfig, sourceServer, targetServer, syncState, nil)

	// Connect should fail since no server is running.
	err := mb.Connect()
	verify.Error(t, err)

	// Status should show error.
	status := mb.Status()
	verify.Equal(t, status.Status, StatusError)
	verify.NotNil(t, status.LastError)
}
