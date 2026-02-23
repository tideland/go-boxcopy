package config

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

// EncryptPassword encrypts a password using AES-GCM with the given key.
// Returns a base64-encoded string.
func EncryptPassword(password, key string) (string, error) {
	keyHash := deriveKey(key)

	block, err := aes.NewCipher(keyHash)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptPassword decrypts a password that was encrypted with EncryptPassword.
func DecryptPassword(encrypted, key string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decode password: %w", err)
	}

	keyHash := deriveKey(key)

	block, err := aes.NewCipher(keyHash)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt password: %w", err)
	}

	return string(plaintext), nil
}

// deriveKey derives a 32-byte AES key from a passphrase using SHA-256.
func deriveKey(passphrase string) []byte {
	hash := sha256.Sum256([]byte(passphrase))
	return hash[:]
}

// DecryptMailboxPasswords decrypts all mailbox passwords in the config.
func (c *Config) DecryptMailboxPasswords(key string) error {
	for i := range c.Mailboxes {
		sourcePass, err := DecryptPassword(c.Mailboxes[i].SourcePassword, key)
		if err != nil {
			return fmt.Errorf("mailbox %s: failed to decrypt source password: %w",
				c.Mailboxes[i].Name, err)
		}
		c.Mailboxes[i].SourcePassword = sourcePass

		targetPass, err := DecryptPassword(c.Mailboxes[i].TargetPassword, key)
		if err != nil {
			return fmt.Errorf("mailbox %s: failed to decrypt target password: %w",
				c.Mailboxes[i].Name, err)
		}
		c.Mailboxes[i].TargetPassword = targetPass
	}
	return nil
}
