package config

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tideland.dev/go/toml"
)

// Config represents the complete BoxCopy configuration.
type Config struct {
	General   GeneralConfig
	CopyParam CopyParameters
	Source    ServerConfig
	Target    ServerConfig
	Mailboxes []MailboxConfig
}

// GeneralConfig holds general boxcopy settings.
type GeneralConfig struct {
	StateFile string
	LogLevel  string
	Progress  int
}

// CopyParameters holds copy throttling settings.
type CopyParameters struct {
	MessagesPerSecond int
	MaxConnections    int
}

// ServerConfig holds IMAP server connection settings.
type ServerConfig struct {
	Host string
	Port int
	TLS  bool
}

// MailboxConfig holds configuration for a single mailbox copy.
type MailboxConfig struct {
	Name           string
	SourceUser     string
	SourcePassword string
	TargetUser     string
	TargetPassword string
}

// RequiredFileMode is the required permission mode for config files.
const RequiredFileMode = 0600

// Load reads and parses a configuration file.
// The file must have mode 0600 (owner read/write only) for security.
func Load(path string) (*Config, error) {
	// Check file permissions.
	if err := CheckFilePermissions(path); err != nil {
		return nil, err
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve config path: %w", err)
	}

	doc, err := toml.Parse(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return loadFromDocument(doc, filepath.Dir(absPath))
}

// CheckFilePermissions verifies the file has mode 0600.
func CheckFilePermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat config file: %w", err)
	}

	mode := info.Mode().Perm()
	if mode != RequiredFileMode {
		return fmt.Errorf("config file %s has insecure permissions %04o, must be %04o",
			path, mode, RequiredFileMode)
	}

	return nil
}

// LoadString parses configuration from a string.
func LoadString(content string) (*Config, error) {
	doc, err := toml.ParseString(content, "config")
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return loadFromDocument(doc, "")
}

// loadFromDocument extracts configuration from a parsed TOML document.
// cfgDir is the directory of the config file, used to resolve relative paths.
func loadFromDocument(doc *toml.Document, cfgDir string) (*Config, error) {
	cfg := &Config{}

	// Load general section.
	if err := loadGeneralConfig(doc, cfg, cfgDir); err != nil {
		return nil, err
	}

	// Load copy_parameters section.
	if err := loadCopyParameters(doc, cfg); err != nil {
		return nil, err
	}

	// Load source server section.
	if err := loadServerConfig(doc, "source", &cfg.Source); err != nil {
		return nil, err
	}

	// Load target server section.
	if err := loadServerConfig(doc, "target", &cfg.Target); err != nil {
		return nil, err
	}

	// Load mailbox table array.
	if err := loadMailboxConfigs(doc, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadGeneralConfig loads the [general] section.
// cfgDir is the directory of the config file; relative paths are resolved against it.
func loadGeneralConfig(doc *toml.Document, cfg *Config, cfgDir string) error {
	section, err := doc.Section("general")
	if err != nil {
		// General section is optional, use defaults.
		cfg.General = defaultGeneralConfig()
		return nil
	}

	stateFile, err := section.GetString("state_file")
	if err != nil {
		stateFile = defaultStateFile()
	}
	cfg.General.StateFile = resolvePath(stateFile, cfgDir)

	logLevel, err := section.GetString("log_level")
	if err != nil {
		logLevel = "info"
	}
	cfg.General.LogLevel = logLevel

	progress, err := section.GetInt("progress")
	if err != nil {
		progress = 10
	}
	cfg.General.Progress = int(progress)

	return nil
}

// loadCopyParameters loads the [copy_parameters] section.
func loadCopyParameters(doc *toml.Document, cfg *Config) error {
	section, err := doc.Section("copy_parameters")
	if err != nil {
		// Section is optional, use defaults.
		cfg.CopyParam = defaultCopyParameters()
		return nil
	}

	mps, err := section.GetInt("messages_per_second")
	if err != nil {
		mps = 10
	}
	cfg.CopyParam.MessagesPerSecond = int(mps)

	maxConn, err := section.GetInt("max_connections")
	if err != nil {
		maxConn = 5
	}
	cfg.CopyParam.MaxConnections = int(maxConn)

	return nil
}

// loadServerConfig loads a server section ([source] or [target]).
func loadServerConfig(doc *toml.Document, name string, server *ServerConfig) error {
	section, err := doc.Section(name)
	if err != nil {
		return fmt.Errorf("missing required section [%s]", name)
	}

	host, err := section.GetString("host")
	if err != nil {
		return fmt.Errorf("[%s] missing required field 'host'", name)
	}
	server.Host = host

	port, err := section.GetInt("port")
	if err != nil {
		port = 993 // Default IMAPS port.
	}
	server.Port = int(port)

	tls, err := section.GetBool("tls")
	if err != nil {
		tls = true // Default to TLS enabled.
	}
	server.TLS = tls

	return nil
}

// loadMailboxConfigs loads the [[mailbox]] table array.
func loadMailboxConfigs(doc *toml.Document, cfg *Config) error {
	sections, err := doc.TableArray("mailbox")
	if err != nil {
		return fmt.Errorf("missing required section [[mailbox]]")
	}

	if len(sections) == 0 {
		return fmt.Errorf("at least one [[mailbox]] entry is required")
	}

	cfg.Mailboxes = make([]MailboxConfig, 0, len(sections))

	for i, section := range sections {
		mb := MailboxConfig{}

		name, err := section.GetString("name")
		if err != nil {
			return fmt.Errorf("[[mailbox]] entry %d: missing required field 'name'", i)
		}
		mb.Name = name

		sourceUser, err := section.GetString("source_user")
		if err != nil {
			return fmt.Errorf("[[mailbox]] entry %d: missing required field 'source_user'", i)
		}
		mb.SourceUser = sourceUser

		sourcePassword, err := section.GetString("source_password")
		if err != nil {
			return fmt.Errorf("[[mailbox]] entry %d: missing required field 'source_password'", i)
		}
		mb.SourcePassword = sourcePassword

		targetUser, err := section.GetString("target_user")
		if err != nil {
			return fmt.Errorf("[[mailbox]] entry %d: missing required field 'target_user'", i)
		}
		mb.TargetUser = targetUser

		targetPassword, err := section.GetString("target_password")
		if err != nil {
			return fmt.Errorf("[[mailbox]] entry %d: missing required field 'target_password'", i)
		}
		mb.TargetPassword = targetPassword

		cfg.Mailboxes = append(cfg.Mailboxes, mb)
	}

	return nil
}

// defaultGeneralConfig returns default general configuration.
func defaultGeneralConfig() GeneralConfig {
	return GeneralConfig{
		StateFile: expandPath(defaultStateFile()),
		LogLevel:  "info",
		Progress:  10,
	}
}

// defaultCopyParameters returns default copy parameters.
func defaultCopyParameters() CopyParameters {
	return CopyParameters{
		MessagesPerSecond: 10,
		MaxConnections:    5,
	}
}

// defaultStateFile returns the default state file path.
func defaultStateFile() string {
	return "~/.boxcopy/state.dat"
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// resolvePath expands ~ and makes relative paths absolute using cfgDir as base.
// This ensures paths in the config work correctly regardless of working directory.
func resolvePath(path, cfgDir string) string {
	path = expandPath(path)
	if filepath.IsAbs(path) {
		return path
	}
	if cfgDir == "" {
		return path
	}
	return filepath.Join(cfgDir, path)
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.Source.Host == "" {
		return fmt.Errorf("source server host is required")
	}
	if c.Target.Host == "" {
		return fmt.Errorf("target server host is required")
	}
	if len(c.Mailboxes) == 0 {
		return fmt.Errorf("at least one mailbox is required")
	}
	if c.CopyParam.MessagesPerSecond < 1 {
		return fmt.Errorf("messages_per_second must be at least 1")
	}
	if c.CopyParam.MaxConnections < 1 {
		return fmt.Errorf("max_connections must be at least 1")
	}
	if !isValidLogLevel(c.General.LogLevel) {
		return fmt.Errorf("invalid log_level: %s (must be debug, info, warn, or error)", c.General.LogLevel)
	}
	return nil
}

// isValidLogLevel checks if the log level is valid.
func isValidLogLevel(level string) bool {
	switch strings.ToLower(level) {
	case "debug", "info", "warn", "error":
		return true
	default:
		return false
	}
}
