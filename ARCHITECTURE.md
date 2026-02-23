# BoxCopy Architecture

This document describes the technical architecture, design decisions, and implementation details of Tideland Go BoxCopy.

## Overview

BoxCopy performs a one-time, one-way copy of IMAP mailboxes from a source server to a target server. Multiple mailboxes are copied in parallel via a worker pool; messages within each mailbox are rate-limited via a throttle.

```
┌─────────────────────────────────────────────┐
│              boxcopy copy --perform         │
│  (confirmation → cleanup → copy)            │
└──────────────────┬──────────────────────────┘
                   │
       ┌───────────┼───────────┐
       ▼           ▼           ▼
   ┌────────┐  ┌────────┐  ┌────────┐
   │Mailbox │  │Mailbox │  │Mailbox │   ← worker.WorkerPool
   │Copy 1  │  │Copy 2  │  │Copy N  │     (max_connections)
   └────┬───┘  └────┬───┘  └────┬───┘
        │           │           │
        └───────────┼───────────┘
                    ▼
            ┌──────────────┐
            │   Throttle   │   ← wait.Throttle
            │(msg/sec rate)│     (messages_per_second)
            └──────────────┘
```

## Package Structure

```
boxcopy/
├── cmd/boxcopy/       # CLI entry point
├── internal/
│   ├── config/        # TOML configuration parsing
│   ├── runner/        # Copy orchestration: confirmation, cleanup, copy
│   ├── mailbox/       # Mailbox copy implementation
│   ├── imap/          # IMAP client wrapper
│   └── state/         # Persistence, copy state
```

### Package Responsibilities

| Package | Responsibility |
|---------|----------------|
| `config` | Parse TOML config, validate settings, decrypt credentials |
| `runner` | Safety confirmation, target cleanup, parallel copy orchestration |
| `mailbox` | Per-mailbox copy logic: folders, messages, flags, progress |
| `imap` | IMAP client wrapper, folder/message operations, expunge |
| `state` | Persist copy state, UID mappings, restart recovery |

## Dependencies

### Tideland Libraries

| Library | Purpose |
|---------|---------|
| [Go TOML](https://tideland.dev/go/toml) | Configuration parsing |
| [Go Worker](https://tideland.dev/go/worker) | Parallel mailbox processing |
| [Go Wait](https://tideland.dev/go/wait) | Per-message rate throttling |
| [Go Asserts](https://tideland.dev/go/asserts) | Unit testing |

### Third-Party Libraries

| Library | Purpose |
|---------|---------|
| [go-imap](https://github.com/emersion/go-imap) | IMAP client (IMAP4rev1) |
| Standard `flag` | CLI argument parsing |
| Standard `log/slog` | Structured logging |

## Concurrency Model

### Mailbox Worker Pool

Multiple mailboxes are copied in parallel using `worker.WorkerPool`:

- Pool size = `max_connections` from `[copy_parameters]`
- Each mailbox runs as a task in the pool
- Round-robin task distribution across workers
- `worker.WaitForTasks` waits for all mailboxes to finish

```go
pool, _ := worker.NewWorkerPool(cfg.CopyParam.MaxConnections, worker.DefaultConfig())
for _, mbConfig := range cfg.Mailboxes {
    worker.Enqueue(pool, func() error { return mb.Copy() })
}
worker.WaitForTasks(pool, 24*time.Hour)
```

### Message Throttle

Within each mailbox, messages are rate-limited using `wait.Throttle`:

- Rate = `messages_per_second` from `[copy_parameters]`
- Single shared throttle across all mailboxes
- Applied per-message via `throttle.Process(ctx, task)`

```go
throttle := wait.NewThrottle(wait.Limit(cfg.CopyParam.MessagesPerSecond), burst)
throttle.Process(ctx, func() error { return appendMessage(...) })
```

## Copy Flow

### 1. Dry-Run (default)

Without `--perform`, BoxCopy only loads config and logs what would be copied. No connections are made.

### 2. --perform Flow

```
1. Safety confirmation
   └─ Print: source, target, mailbox list, warning
   └─ Require user to type YES

2. Target cleanup
   └─ For each mailbox:
       └─ Connect to target
       └─ List all folders
       └─ For each selectable folder:
           └─ SELECT folder
           └─ STORE all messages as \Deleted
           └─ EXPUNGE
   └─ Abort entirely if any folder cannot be cleaned

3. State reset
   └─ Clear saved copy state for all mailboxes

4. Parallel copy (WorkerPool)
   └─ For each mailbox (in parallel):
       └─ Connect to source + target
       └─ Copy folder structure
       └─ For each folder:
           └─ Fetch messages from source
           └─ For each uncopied message (throttled):
               └─ Append to target
               └─ Record source→target UID mapping
           └─ Log progress at milestones
       └─ Disconnect

5. Save state
```

## Error Handling

### Target Cleanup

Target cleanup is a hard prerequisite. If any folder cannot be cleaned (server error, permission error), the entire operation is aborted before any copying begins. This prevents partial copies to partially-cleaned targets.

### Per-Folder Copy Errors

If a folder copy fails, the tool attempts to reconnect to the target and continues with the next folder. If reconnection fails, the mailbox copy is aborted.

### Per-Message Copy Errors

Failed messages are logged and skipped; the copy continues with the next message.

### Logging

- All errors logged with context (mailbox, folder, UID)
- Progress logged at configurable percentage milestones
- Structured logging with slog

## State and Recovery

The state file (`~/.boxcopy/state.dat`) tracks:
- Which message UIDs have been copied per folder
- Source→target UID mapping for each copied message
- UIDValidity for detecting mailbox rebuilds
- Last copy timestamp per folder and mailbox

Since the target is cleaned at the start of each run, resumption of interrupted copies is not supported.

## Configuration

Configuration uses Tideland Go TOML (read-only parser):

```go
doc, err := toml.Parse("config.toml")
host, _ := doc.GetString("source.host")
port, _ := doc.GetInt("source.port")
mailboxes, _ := doc.TableArray("mailbox")
```

## Security

### Configuration File

- Config file MUST have mode 0600 (owner read/write only)
- Insecure permissions cause the tool to refuse startup

### Credential Storage

- All passwords MUST be encrypted (unencrypted passwords rejected)
- Encryption uses AES-GCM with SHA-256 key derivation
- Stored as base64-encoded strings
- Encryption key MUST be provided via `-k <key>` flag
- No environment variable fallback (prevents accidental exposure)

### Key Management

- Encryption key managed by administrator
- Same key used for `encrypt-password` and `copy` commands
- Key not stored anywhere — must be provided at runtime

### IMAP Connections

- TLS enabled by default (port 993)
- Non-TLS connections require explicit configuration

## Testing

- Unit tests using Tideland Go Asserts
- Race detector enabled (`go test -race`)
- Coverage reports via `make coverage`

## Code Style

All Go files use the Tideland header:

```go
package name

// Copyright (C) 2026 Frank Mueller / Tideland
//
// All rights reserved. Use of this source code is governed
// by the new BSD license.
```
