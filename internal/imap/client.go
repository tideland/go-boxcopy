package imap

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"tideland.dev/go/boxcopy/internal/config"
)

// Client wraps an IMAP client connection with additional state.
type Client struct {
	mu       sync.Mutex
	client   *imapclient.Client
	server   config.ServerConfig
	user     string
	selected string
	logger   *slog.Logger
	done     chan struct{} // closed by Close() to stop the keepalive goroutine
}

// Options configures client behavior.
type Options struct {
	Logger    *slog.Logger
	TLSConfig *tls.Config

	// KeepaliveInterval sets how often a NOOP command is sent to keep the
	// connection alive during idle periods. Zero disables keepalive.
	KeepaliveInterval time.Duration
}

// DefaultOptions returns default client options.
func DefaultOptions() *Options {
	return &Options{
		Logger: slog.Default(),
	}
}

// Dial connects to an IMAP server and authenticates.
func Dial(server config.ServerConfig, user, password string, opts *Options) (*Client, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	logger := opts.Logger.With(
		slog.String("host", server.Host),
		slog.Int("port", server.Port),
		slog.String("user", user),
	)

	addr := fmt.Sprintf("%s:%d", server.Host, server.Port)
	logger.Debug("connecting to IMAP server")

	var client *imapclient.Client
	var err error

	tlsConfig := opts.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			ServerName: server.Host,
		}
	}

	if server.TLS {
		client, err = imapclient.DialTLS(addr, &imapclient.Options{
			TLSConfig: tlsConfig,
		})
	} else {
		client, err = imapclient.DialInsecure(addr, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	logger.Debug("logging in")

	if err := client.Login(user, password).Wait(); err != nil {
		client.Close() //nolint:errcheck
		return nil, fmt.Errorf("failed to login as %s: %w", user, err)
	}

	logger.Info("connected and authenticated")

	c := &Client{
		client: client,
		server: server,
		user:   user,
		logger: logger,
		done:   make(chan struct{}),
	}
	if opts.KeepaliveInterval > 0 {
		c.startKeepalive(opts.KeepaliveInterval)
	}
	return c, nil
}

// startKeepalive launches a background goroutine that sends a NOOP command
// every interval to prevent the server from closing an idle connection.
func (c *Client) startKeepalive(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.Noop(); err != nil {
					c.logger.Warn("keepalive NOOP failed, stopping keepalive",
						slog.Any("error", err))
					return
				}
			case <-c.done:
				return
			}
		}
	}()
}

// Close closes the IMAP connection gracefully.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil
	}

	// Signal the keepalive goroutine to stop before tearing down the connection.
	if c.done != nil {
		close(c.done)
		c.done = nil
	}

	c.logger.Debug("logging out")

	if err := c.client.Logout().Wait(); err != nil {
		c.logger.Warn("logout failed", slog.Any("error", err))
	}

	if err := c.client.Close(); err != nil {
		return fmt.Errorf("failed to close connection: %w", err)
	}

	c.client = nil
	c.logger.Info("connection closed")
	return nil
}

// Server returns the server configuration.
func (c *Client) Server() config.ServerConfig {
	return c.server
}

// User returns the authenticated user.
func (c *Client) User() string {
	return c.user
}

// Selected returns the currently selected mailbox name, or empty if none.
func (c *Client) Selected() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.selected
}

// Select selects a mailbox for subsequent operations.
func (c *Client) Select(mailbox string) (*MailboxInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("selecting mailbox", slog.String("mailbox", mailbox))

	data, err := c.client.Select(mailbox, nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to select mailbox %s: %w", mailbox, err)
	}

	c.selected = mailbox

	return &MailboxInfo{
		Name:        mailbox,
		NumMessages: data.NumMessages,
		UIDValidity: data.UIDValidity,
		UIDNext:     uint32(data.UIDNext),
		Flags:       convertFlags(data.Flags),
	}, nil
}

// SelectReadOnly selects a mailbox in read-only mode (IMAP EXAMINE command),
// preventing the server from changing \Recent flags on the source.
func (c *Client) SelectReadOnly(mailbox string) (*MailboxInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("examining mailbox read-only", slog.String("mailbox", mailbox))

	data, err := c.client.Select(mailbox, &imap.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return nil, fmt.Errorf("failed to examine mailbox %s: %w", mailbox, err)
	}

	c.selected = mailbox

	return &MailboxInfo{
		Name:        mailbox,
		NumMessages: data.NumMessages,
		UIDValidity: data.UIDValidity,
		UIDNext:     uint32(data.UIDNext),
		Flags:       convertFlags(data.Flags),
	}, nil
}

// Unselect deselects the currently selected mailbox, returning to the
// authenticated state. This must be called before deleting folders on
// servers that reject DELETE while a mailbox is selected.
func (c *Client) Unselect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.selected == "" {
		return nil
	}

	if err := c.client.Unselect().Wait(); err != nil {
		return fmt.Errorf("failed to unselect mailbox: %w", err)
	}

	c.selected = ""
	return nil
}

// Noop sends a NOOP command to keep the connection alive.
func (c *Client) Noop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client == nil {
		return nil
	}
	if err := c.client.Noop().Wait(); err != nil {
		return fmt.Errorf("NOOP failed: %w", err)
	}
	return nil
}

// MailboxInfo contains information about a selected mailbox.
type MailboxInfo struct {
	Name        string
	NumMessages uint32
	UIDValidity uint32
	UIDNext     uint32
	Flags       []string
}

// convertFlags converts imap.Flag slice to string slice.
func convertFlags(flags []imap.Flag) []string {
	result := make([]string, len(flags))
	for i, f := range flags {
		result[i] = string(f)
	}
	return result
}

// convertToFlags converts string slice to imap.Flag slice.
func convertToFlags(flags []string) []imap.Flag {
	result := make([]imap.Flag, len(flags))
	for i, f := range flags {
		result[i] = imap.Flag(f)
	}
	return result
}
