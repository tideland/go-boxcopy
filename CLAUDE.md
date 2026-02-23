# Tideland Go BoxCopy

## Project Goal

- **Tideland Go BoxCopy** is an OSS CLI tool for performing a one-time, one-way copy of IMAP mailboxes from a source server to a target server.
- It facilitates mail server migrations: copy all mailboxes from server A to server B, then switch the MX DNS entry.
- The number of users is < 50; some mailboxes contain mails of > 20 years.
- The configuration file format is TOML.
- Dry-run by default; use `--perform` for the actual copy.
- Before copying, all existing messages on the target are deleted (clean migration).

## Code Style

### File Header

All Go files must use the Tideland header style:

```go
package name

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.
```

## Technical Constraints

- As a Tideland software it will be developed in Go.
- The own package name is tideland.dev/go/boxcopy.
- Via redirection all Tideland Go software is managed on GitHub as github.com/tideland/go-....
- So BoxCopy will be stored at github.com/tideland/go-boxcopy.
- It is important to use the following Tideland libraries wherever it makes sense:
  - Go Asserts simplifies testing in a flexible and natural way
  - Go TOML reads and processes TOML files
  - Go Wait allows throttling events (closures) at a configurable rate
  - Go Worker provides a pool of workers for parallel task processing
- Other libraries for CLI, IMAP should be commonly used 3rd party libraries.
- Logging should be done by the standard Go library log/slog.
- No daemon, no signals, no PID file — one-shot execution.

## Current Architecture

### CLI Commands

```
boxcopy copy -k <key>              # Dry-run: show what would be copied
boxcopy copy -k <key> --perform    # Actual copy (confirmation + cleanup + copy)
boxcopy encrypt-password -k <key>  # Encrypt password for config
boxcopy init                       # Generate initial config at ~/.boxcopy/config.toml
```

### --perform Flow

1. Safety confirmation: print summary, require user to type `YES`
2. Target cleanup: expunge all messages from all folders on target
3. Clear copy state
4. Parallel copy: WorkerPool (one task per mailbox, max_connections workers)
5. Per-message throttling via wait.Throttle (messages_per_second)
6. Save state

### Package Structure

```
boxcopy/
├── cmd/boxcopy/       # CLI entry point
├── internal/
│   ├── config/        # TOML configuration, encryption
│   ├── runner/        # Copy orchestration
│   ├── mailbox/       # Per-mailbox copy logic
│   ├── imap/          # IMAP client wrapper
│   └── state/         # Copy state persistence
```

### Configuration Sections

- `[general]`: state_file, log_level, progress
- `[copy_parameters]`: messages_per_second, max_connections
- `[source]`, `[target]`: host, port, tls
- `[[mailbox]]`: name, source_user, source_password, target_user, target_password

### Key Types

- `config.GeneralConfig` (field `cfg.General`)
- `config.CopyParameters` (field `cfg.CopyParam`)
- `runner.Options`: ConfigPath, EncryptionKey, Perform, Logger, Input
- `mailbox.Options`: Logger, Throttle, Context, Progress
- `mailbox.StatusInfo`: Name, Status, MessagesCopied, MessagesSkipped, BytesCopied, ...

## Libraries

### Tideland Libraries

| Purpose | Library |
|---------|---------|
| Configuration | Go TOML |
| Task Processing | Go Worker |
| Rate Limiting | Go Wait (Throttle) |
| Testing | Go Asserts |

### 3rd Party Libraries

| Purpose | Library |
|---------|---------|
| CLI | Standard `flag` |
| IMAP | `emersion/go-imap/v2` |
