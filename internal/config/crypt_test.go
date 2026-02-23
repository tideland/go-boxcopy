package config

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"testing"

	"tideland.dev/go/asserts/verify"
)

// TestEncryptDecryptPassword tests the encryption and decryption of passwords.
func TestEncryptDecryptPassword(t *testing.T) {
	password := "mysecretpassword"
	key := "encryptionkey123"

	encrypted, err := EncryptPassword(password, key)
	verify.NoError(t, err)
	verify.Different(t, encrypted, password)

	decrypted, err := DecryptPassword(encrypted, key)
	verify.NoError(t, err)
	verify.Equal(t, decrypted, password)
}

// TestEncryptPasswordDifferentResults tests that encryption produces different
// results each time (due to random nonce).
func TestEncryptPasswordDifferentResults(t *testing.T) {
	password := "mysecretpassword"
	key := "encryptionkey123"

	encrypted1, err := EncryptPassword(password, key)
	verify.NoError(t, err)

	encrypted2, err := EncryptPassword(password, key)
	verify.NoError(t, err)

	// Different encryptions should produce different ciphertexts.
	verify.Different(t, encrypted1, encrypted2)

	// But both should decrypt to the same password.
	decrypted1, err := DecryptPassword(encrypted1, key)
	verify.NoError(t, err)
	verify.Equal(t, decrypted1, password)

	decrypted2, err := DecryptPassword(encrypted2, key)
	verify.NoError(t, err)
	verify.Equal(t, decrypted2, password)
}

// TestDecryptPasswordWrongKey tests decryption with wrong key fails.
func TestDecryptPasswordWrongKey(t *testing.T) {
	password := "mysecretpassword"
	key := "encryptionkey123"
	wrongKey := "wrongkey456"

	encrypted, err := EncryptPassword(password, key)
	verify.NoError(t, err)

	_, err = DecryptPassword(encrypted, wrongKey)
	verify.Error(t, err)
}

// TestDecryptPasswordInvalidBase64 tests decryption with invalid base64.
func TestDecryptPasswordInvalidBase64(t *testing.T) {
	invalid := "not-valid-base64!!!"
	key := "anykey"

	_, err := DecryptPassword(invalid, key)
	verify.Error(t, err)
}

// TestDecryptMailboxPasswords tests decrypting all mailbox passwords.
func TestDecryptMailboxPasswords(t *testing.T) {
	key := "testkey"

	// Encrypt passwords.
	sourcePass, err := EncryptPassword("sourcepass", key)
	verify.NoError(t, err)
	targetPass, err := EncryptPassword("targetpass", key)
	verify.NoError(t, err)

	cfg := &Config{
		Mailboxes: []MailboxConfig{
			{
				Name:           "user1",
				SourceUser:     "user1@source.com",
				SourcePassword: sourcePass,
				TargetUser:     "user1@target.com",
				TargetPassword: targetPass,
			},
		},
	}

	err = cfg.DecryptMailboxPasswords(key)
	verify.NoError(t, err)
	verify.Equal(t, cfg.Mailboxes[0].SourcePassword, "sourcepass")
	verify.Equal(t, cfg.Mailboxes[0].TargetPassword, "targetpass")
}

// TestDecryptMailboxPasswordsWrongKey tests that wrong key fails.
func TestDecryptMailboxPasswordsWrongKey(t *testing.T) {
	key := "testkey"
	wrongKey := "wrongkey"

	sourcePass, err := EncryptPassword("sourcepass", key)
	verify.NoError(t, err)

	cfg := &Config{
		Mailboxes: []MailboxConfig{
			{
				Name:           "user1",
				SourcePassword: sourcePass,
				TargetPassword: "plaintext",
			},
		},
	}

	err = cfg.DecryptMailboxPasswords(wrongKey)
	verify.Error(t, err)
}

// TestEncryptPasswordEmptyValues tests edge cases with empty values.
func TestEncryptPasswordEmptyValues(t *testing.T) {
	// Empty password should still work.
	encrypted, err := EncryptPassword("", "key")
	verify.NoError(t, err)

	decrypted, err := DecryptPassword(encrypted, "key")
	verify.NoError(t, err)
	verify.Equal(t, decrypted, "")

	// Empty key should still work (produces deterministic key from hash).
	encrypted2, err := EncryptPassword("password", "")
	verify.NoError(t, err)

	decrypted2, err := DecryptPassword(encrypted2, "")
	verify.NoError(t, err)
	verify.Equal(t, decrypted2, "password")
}
