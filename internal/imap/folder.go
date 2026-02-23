package imap

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"fmt"
	"log/slog"

	"github.com/emersion/go-imap/v2"
)

// Folder represents an IMAP mailbox/folder.
type Folder struct {
	Name       string
	Delimiter  string
	Attributes []string
	NumMessages *uint32
	NumUnseen   *uint32
}

// ListFolders returns all folders/mailboxes on the server.
func (c *Client) ListFolders() ([]Folder, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("listing folders")

	// List all folders with status information.
	listCmd := c.client.List("", "*", &imap.ListOptions{
		ReturnStatus: &imap.StatusOptions{
			NumMessages: true,
			NumUnseen:   true,
		},
	})

	var folders []Folder
	for {
		data := listCmd.Next()
		if data == nil {
			break
		}

		folder := Folder{
			Name:       data.Mailbox,
			Delimiter:  string(data.Delim),
			Attributes: make([]string, len(data.Attrs)),
		}

		for i, attr := range data.Attrs {
			folder.Attributes[i] = string(attr)
		}

		if data.Status != nil {
			folder.NumMessages = data.Status.NumMessages
			folder.NumUnseen = data.Status.NumUnseen
		}

		folders = append(folders, folder)
	}

	if err := listCmd.Close(); err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}

	c.logger.Debug("folders listed", slog.Int("count", len(folders)))
	return folders, nil
}

// CreateFolder creates a new folder/mailbox.
func (c *Client) CreateFolder(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("creating folder", slog.String("name", name))

	if err := c.client.Create(name, nil).Wait(); err != nil {
		return fmt.Errorf("failed to create folder %s: %w", name, err)
	}

	c.logger.Info("folder created", slog.String("name", name))
	return nil
}

// DeleteFolder deletes a folder/mailbox.
func (c *Client) DeleteFolder(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("deleting folder", slog.String("name", name))

	if err := c.client.Delete(name).Wait(); err != nil {
		return fmt.Errorf("failed to delete folder %s: %w", name, err)
	}

	c.logger.Info("folder deleted", slog.String("name", name))
	return nil
}

// RenameFolder renames a folder/mailbox.
func (c *Client) RenameFolder(oldName, newName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("renaming folder",
		slog.String("from", oldName),
		slog.String("to", newName))

	if err := c.client.Rename(oldName, newName).Wait(); err != nil {
		return fmt.Errorf("failed to rename folder %s to %s: %w", oldName, newName, err)
	}

	c.logger.Info("folder renamed",
		slog.String("from", oldName),
		slog.String("to", newName))
	return nil
}

// FolderStatus returns status information for a folder without selecting it.
func (c *Client) FolderStatus(name string) (*FolderStatusInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Debug("getting folder status", slog.String("name", name))

	data, err := c.client.Status(name, &imap.StatusOptions{
		NumMessages: true,
		NumUnseen:   true,
		UIDNext:     true,
		UIDValidity: true,
	}).Wait()

	if err != nil {
		return nil, fmt.Errorf("failed to get status for folder %s: %w", name, err)
	}

	info := &FolderStatusInfo{
		Name: data.Mailbox,
	}

	if data.NumMessages != nil {
		info.NumMessages = *data.NumMessages
	}
	if data.NumUnseen != nil {
		info.NumUnseen = *data.NumUnseen
	}
	if data.UIDNext != 0 {
		info.UIDNext = uint32(data.UIDNext)
	}
	if data.UIDValidity == 0 {
		return nil, fmt.Errorf("server returned invalid UIDValidity 0 for folder %s", name)
	}
	info.UIDValidity = data.UIDValidity

	return info, nil
}

// FolderStatusInfo contains status information for a folder.
type FolderStatusInfo struct {
	Name        string
	NumMessages uint32
	NumUnseen   uint32
	UIDNext     uint32
	UIDValidity uint32
}

// FolderExists checks if a folder exists.
func (c *Client) FolderExists(name string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	listCmd := c.client.List("", name, nil)

	found := false
	for {
		data := listCmd.Next()
		if data == nil {
			break
		}
		if data.Mailbox == name {
			found = true
		}
	}

	if err := listCmd.Close(); err != nil {
		return false, fmt.Errorf("failed to check folder existence: %w", err)
	}

	return found, nil
}

// EnsureFolder creates a folder if it doesn't exist.
func (c *Client) EnsureFolder(name string) error {
	exists, err := c.FolderExists(name)
	if err != nil {
		return err
	}

	if !exists {
		return c.CreateFolder(name)
	}

	return nil
}

// HasAttribute checks if a folder has a specific attribute.
func (f *Folder) HasAttribute(attr string) bool {
	for _, a := range f.Attributes {
		if a == attr {
			return true
		}
	}
	return false
}

// IsNoSelect returns true if the folder cannot be selected (container only).
func (f *Folder) IsNoSelect() bool {
	return f.HasAttribute("\\Noselect") || f.HasAttribute("\\NonExistent")
}

// HasChildren returns true if the folder has child folders.
func (f *Folder) HasChildren() bool {
	return f.HasAttribute("\\HasChildren")
}
