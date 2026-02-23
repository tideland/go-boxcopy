# Changelog

All notable changes to Tideland Go BoxCopy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial project structure
- TOML configuration parsing (internal/config)
- IMAP client wrapper (internal/imap)
- Mailbox copy implementation (internal/mailbox)
- Copy state persistence (internal/state)
- Copy runner with safety confirmation, target cleanup, parallel copy (internal/runner)
- CLI commands (cmd/boxcopy): copy, encrypt-password, init
- Dry-run mode by default; --perform flag for actual copy
- Single safety confirmation prompt before any destructive action
- Target cleanup before copy (expunge all messages on target)
- WorkerPool for parallel mailbox copying
- Throttle for per-message rate limiting
- Progress logging at configurable percentage milestones
- BSD-3-Clause license
