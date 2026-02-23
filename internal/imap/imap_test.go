package imap

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"testing"
	"time"

	"tideland.dev/go/asserts/verify"
)

// TestAddressString tests the Address.String method.
func TestAddressString(t *testing.T) {
	tests := []struct {
		name     string
		address  Address
		expected string
	}{
		{
			name: "full address with name",
			address: Address{
				Name:    "John Doe",
				Mailbox: "john",
				Host:    "example.com",
			},
			expected: "John Doe <john@example.com>",
		},
		{
			name: "address without name",
			address: Address{
				Mailbox: "john",
				Host:    "example.com",
			},
			expected: "john@example.com",
		},
		{
			name: "empty name",
			address: Address{
				Name:    "",
				Mailbox: "info",
				Host:    "test.org",
			},
			expected: "info@test.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.address.String()
			verify.Equal(t, result, tt.expected)
		})
	}
}

// TestMessageHasFlag tests the Message.HasFlag method.
func TestMessageHasFlag(t *testing.T) {
	msg := Message{
		Flags: []string{FlagSeen, FlagFlagged},
	}

	verify.True(t, msg.HasFlag(FlagSeen))
	verify.True(t, msg.HasFlag(FlagFlagged))
	verify.False(t, msg.HasFlag(FlagDeleted))
	verify.False(t, msg.HasFlag(FlagAnswered))
}

// TestMessageIsSeen tests the Message.IsSeen method.
func TestMessageIsSeen(t *testing.T) {
	seenMsg := Message{Flags: []string{FlagSeen}}
	unseenMsg := Message{Flags: []string{}}

	verify.True(t, seenMsg.IsSeen())
	verify.False(t, unseenMsg.IsSeen())
}

// TestMessageIsDeleted tests the Message.IsDeleted method.
func TestMessageIsDeleted(t *testing.T) {
	deletedMsg := Message{Flags: []string{FlagDeleted}}
	normalMsg := Message{Flags: []string{FlagSeen}}

	verify.True(t, deletedMsg.IsDeleted())
	verify.False(t, normalMsg.IsDeleted())
}

// TestMessageIsFlagged tests the Message.IsFlagged method.
func TestMessageIsFlagged(t *testing.T) {
	flaggedMsg := Message{Flags: []string{FlagFlagged}}
	normalMsg := Message{Flags: []string{}}

	verify.True(t, flaggedMsg.IsFlagged())
	verify.False(t, normalMsg.IsFlagged())
}

// TestFolderHasAttribute tests the Folder.HasAttribute method.
func TestFolderHasAttribute(t *testing.T) {
	folder := Folder{
		Name:       "INBOX",
		Attributes: []string{"\\HasChildren", "\\Subscribed"},
	}

	verify.True(t, folder.HasAttribute("\\HasChildren"))
	verify.True(t, folder.HasAttribute("\\Subscribed"))
	verify.False(t, folder.HasAttribute("\\Noselect"))
}

// TestFolderIsNoSelect tests the Folder.IsNoSelect method.
func TestFolderIsNoSelect(t *testing.T) {
	selectableFolder := Folder{
		Name:       "INBOX",
		Attributes: []string{},
	}
	noSelectFolder := Folder{
		Name:       "Folder",
		Attributes: []string{"\\Noselect"},
	}
	nonExistentFolder := Folder{
		Name:       "Ghost",
		Attributes: []string{"\\NonExistent"},
	}

	verify.False(t, selectableFolder.IsNoSelect())
	verify.True(t, noSelectFolder.IsNoSelect())
	verify.True(t, nonExistentFolder.IsNoSelect())
}

// TestFolderHasChildren tests the Folder.HasChildren method.
func TestFolderHasChildren(t *testing.T) {
	parentFolder := Folder{
		Name:       "Parent",
		Attributes: []string{"\\HasChildren"},
	}
	leafFolder := Folder{
		Name:       "Leaf",
		Attributes: []string{"\\HasNoChildren"},
	}

	verify.True(t, parentFolder.HasChildren())
	verify.False(t, leafFolder.HasChildren())
}

// TestDefaultFetchOptions tests DefaultFetchOptions function.
func TestDefaultFetchOptions(t *testing.T) {
	opts := DefaultFetchOptions()

	verify.True(t, opts.UID)
	verify.True(t, opts.Flags)
	verify.True(t, opts.InternalDate)
	verify.True(t, opts.Size)
	verify.True(t, opts.Envelope)
	verify.False(t, opts.Body)
}

// TestFullFetchOptions tests FullFetchOptions function.
func TestFullFetchOptions(t *testing.T) {
	opts := FullFetchOptions()

	verify.True(t, opts.UID)
	verify.True(t, opts.Flags)
	verify.True(t, opts.InternalDate)
	verify.True(t, opts.Size)
	verify.True(t, opts.Envelope)
	verify.True(t, opts.Body)
	verify.True(t, opts.BodyPeek)
}

// TestConvertFlags tests flag conversion functions.
func TestConvertFlags(t *testing.T) {
	t.Run("string to imap flags", func(t *testing.T) {
		stringFlags := []string{FlagSeen, FlagFlagged}
		imapFlags := convertToFlags(stringFlags)

		verify.Equal(t, len(imapFlags), 2)
	})

	t.Run("empty flags", func(t *testing.T) {
		stringFlags := []string{}
		imapFlags := convertToFlags(stringFlags)

		verify.Equal(t, len(imapFlags), 0)
	})
}

// TestFirstString tests the firstString helper function.
func TestFirstString(t *testing.T) {
	verify.Equal(t, firstString([]string{"first", "second"}), "first")
	verify.Equal(t, firstString([]string{"only"}), "only")
	verify.Equal(t, firstString([]string{}), "")
	verify.Equal(t, firstString(nil), "")
}

// TestMailboxInfo tests MailboxInfo struct.
func TestMailboxInfo(t *testing.T) {
	info := MailboxInfo{
		Name:        "INBOX",
		NumMessages: 42,
		UIDValidity: 12345,
		UIDNext:     100,
		Flags:       []string{FlagSeen, FlagDeleted},
	}

	verify.Equal(t, info.Name, "INBOX")
	verify.Equal(t, info.NumMessages, uint32(42))
	verify.Equal(t, info.UIDValidity, uint32(12345))
	verify.Equal(t, info.UIDNext, uint32(100))
	verify.Equal(t, len(info.Flags), 2)
}

// TestEnvelope tests Envelope struct.
func TestEnvelope(t *testing.T) {
	now := time.Now()
	env := Envelope{
		Date:      now,
		Subject:   "Test Subject",
		MessageID: "<test@example.com>",
		From: []Address{
			{Name: "Sender", Mailbox: "sender", Host: "example.com"},
		},
		To: []Address{
			{Name: "Recipient", Mailbox: "recipient", Host: "example.com"},
		},
	}

	verify.Equal(t, env.Subject, "Test Subject")
	verify.Equal(t, env.MessageID, "<test@example.com>")
	verify.Equal(t, len(env.From), 1)
	verify.Equal(t, len(env.To), 1)
	verify.Equal(t, env.From[0].String(), "Sender <sender@example.com>")
}

// TestFolderStatusInfo tests FolderStatusInfo struct.
func TestFolderStatusInfo(t *testing.T) {
	info := FolderStatusInfo{
		Name:        "INBOX",
		NumMessages: 100,
		NumUnseen:   5,
		UIDNext:     101,
		UIDValidity: 67890,
	}

	verify.Equal(t, info.Name, "INBOX")
	verify.Equal(t, info.NumMessages, uint32(100))
	verify.Equal(t, info.NumUnseen, uint32(5))
	verify.Equal(t, info.UIDNext, uint32(101))
	verify.Equal(t, info.UIDValidity, uint32(67890))
}
