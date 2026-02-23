package main

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"tideland.dev/go/boxcopy/internal/config"
	"tideland.dev/go/boxcopy/internal/runner"
)

const (
	version = "0.1.0"

	defaultConfigPath = "~/.boxcopy/config.toml"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "-h", "--help", "help":
		usage()
	case "-v", "--version", "version":
		fmt.Printf("Tideland Go BoxCopy version %s\n", version)
	default:
		var err error
		switch cmd {
		case "copy":
			err = cmdCopy(cmdArgs)
		case "encrypt-password":
			err = cmdEncryptPassword(cmdArgs)
		case "init":
			err = cmdInit(cmdArgs)
		default:
			fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
			usage()
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Tideland Go BoxCopy - IMAP Mailbox Copy

Usage: boxcopy <command> [options]

Commands:
  init              Generate initial configuration file
  copy              Copy mailboxes (requires -k)
  encrypt-password  Encrypt a password for config (requires -k)
  help              Show this help message

Copy Options:
  -c, --config <path>     Path to configuration file (default: %s)
  -k, --key <key>         Encryption key (required)
  --perform               Perform the actual copy (default is dry-run)
  --log-level <level>     Log level: debug, info, warn, error

Init Options:
  -o, --output <path>     Output path (default: %s)
  -f, --force             Overwrite existing file

Encrypt Password Options:
  -k, --key <key>         Encryption key (required)

Security:
  - Config file must have mode 0600 (owner read/write only)
  - All passwords in config must be encrypted
  - Encryption key (-k) is required for copy and encrypt-password

Workflow:
  1. boxcopy init                        Generate config at default location
  2. Choose and remember an encryption key
  3. boxcopy encrypt-password -k <key>  Encrypt each password, add to config
  4. boxcopy copy -k <key>              Dry-run: review what would be copied
  5. boxcopy copy -k <key> --perform    Actual copy (asks for confirmation)

Examples:
  boxcopy init
  boxcopy init -o /etc/boxcopy.toml
  boxcopy encrypt-password -k mykey
  boxcopy copy -c myconfig.toml -k mykey
  boxcopy copy -c myconfig.toml -k mykey --perform

`, defaultConfigPath, defaultConfigPath)
}

// cmdCopy runs a copy cycle.
func cmdCopy(args []string) error {
	fs := flag.NewFlagSet("copy", flag.ExitOnError)
	cfgPath := fs.String("c", defaultConfigPath, "Path to configuration file")
	fs.StringVar(cfgPath, "config", defaultConfigPath, "Path to configuration file")
	key := fs.String("k", "", "Encryption key for passwords (required)")
	fs.StringVar(key, "key", "", "Encryption key for passwords (required)")
	perform := fs.Bool("perform", false, "Perform the actual copy (default is dry-run)")
	logLvl := fs.String("log-level", "", "Log level (debug, info, warn, error)")
	fs.Parse(args) //nolint:errcheck

	if *key == "" {
		return fmt.Errorf("encryption key is required: use -k <key>")
	}

	logger := setupLogger(*logLvl)

	return runner.Run(&runner.Options{
		ConfigPath:    expandPath(*cfgPath),
		EncryptionKey: *key,
		Perform:       *perform,
		Logger:        logger,
	})
}

// cmdEncryptPassword encrypts a password.
func cmdEncryptPassword(args []string) error {
	fs := flag.NewFlagSet("encrypt-password", flag.ExitOnError)
	key := fs.String("k", "", "Encryption key (required)")
	fs.StringVar(key, "key", "", "Encryption key (required)")
	fs.Parse(args) //nolint:errcheck

	if *key == "" {
		return fmt.Errorf("encryption key is required: use -k <key>")
	}

	// Read password.
	fmt.Print("Enter password to encrypt: ")
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read password: %w", err)
	}
	password = strings.TrimSpace(password)

	if password == "" {
		return fmt.Errorf("password is required")
	}

	// Encrypt.
	encrypted, err := config.EncryptPassword(password, *key)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	fmt.Printf("\nEncrypted password:\n%s\n", encrypted)
	fmt.Println("\nAdd this to your config file as source_password or target_password.")

	return nil
}

// cmdInit generates an initial configuration file.
func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	output := fs.String("o", defaultConfigPath, "Output path")
	fs.StringVar(output, "output", defaultConfigPath, "Output path")
	force := fs.Bool("f", false, "Overwrite existing file")
	fs.BoolVar(force, "force", false, "Overwrite existing file")
	fs.Parse(args) //nolint:errcheck

	outPath := expandPath(*output)

	// Check if file already exists.
	if _, err := os.Stat(outPath); err == nil && !*force {
		return fmt.Errorf("config file already exists: %s (use -f to overwrite)", outPath)
	}

	// Ensure directory exists.
	dir := filepath.Dir(outPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Write config template with secure permissions (mode 0600).
	if err := os.WriteFile(outPath, []byte(configTemplate), 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Configuration file created: %s (mode 0600)\n", outPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit the config file with your IMAP server details")
	fmt.Println("  2. Use 'boxcopy encrypt-password -k <key>' to encrypt passwords")
	fmt.Println("  3. Run 'boxcopy copy -k <key>' for a dry-run")
	fmt.Println("  4. Run 'boxcopy copy -k <key> --perform' to start the actual copy")

	return nil
}

// configTemplate is the initial configuration template.
const configTemplate = `# BoxCopy Configuration
#
# SECURITY NOTES:
# - This file must have mode 0600 (owner read/write only)
# - All passwords MUST be encrypted using: boxcopy encrypt-password -k <key>
# - The same key must be used for the copy command

# General settings
[general]
state_file = "~/.boxcopy/state.dat"
log_level = "info"   # debug, info, warn, error
progress = 10        # Log progress every N% (0 = disable, 10 = every 10%, 25 = every 25%)

# Copy parameters
[copy_parameters]
messages_per_second = 10  # Rate limit for message copy
max_connections = 5       # Max concurrent IMAP connections (one per mailbox)

# Source IMAP server (copy FROM)
[source]
host = "imap.source.example.com"
port = 993
tls = true

# Target IMAP server (copy TO)
[target]
host = "imap.target.example.com"
port = 993
tls = true

# Mailbox configurations
# Add one [[mailbox]] section per user to copy
# Passwords MUST be encrypted: boxcopy encrypt-password -k <key>

[[mailbox]]
name = "john.doe"
source_user = "john.doe@source.example.com"
source_password = "PASTE_ENCRYPTED_PASSWORD_HERE"
target_user = "john.doe@target.example.com"
target_password = "PASTE_ENCRYPTED_PASSWORD_HERE"

# [[mailbox]]
# name = "jane.doe"
# source_user = "jane.doe@source.example.com"
# source_password = "PASTE_ENCRYPTED_PASSWORD_HERE"
# target_user = "jane.doe@target.example.com"
# target_password = "PASTE_ENCRYPTED_PASSWORD_HERE"
`

// setupLogger creates a logger based on log level string.
func setupLogger(logLvl string) *slog.Logger {
	level := slog.LevelInfo

	switch strings.ToLower(logLvl) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)
	return slog.New(handler)
}

// expandPath expands ~ to user home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}
