# Tideland Go BoxCopy

[![GitHub release](https://img.shields.io/github/release/tideland/go-boxcopy.svg)](https://github.com/tideland/go-boxcopy)
[![GitHub license](https://img.shields.io/badge/license-New%20BSD-blue.svg)](https://raw.githubusercontent.com/tideland/go-boxcopy/master/LICENSE)
[![Go Module](https://img.shields.io/github/go-mod/go-version/tideland/go-boxcopy)](https://github.com/tideland/go-boxcopy/blob/master/go.mod)
[![GoDoc](https://godoc.org/tideland.dev/go/boxcopy?status.svg)](https://pkg.go.dev/mod/tideland.dev/go/boxcopy?tab=packages)
![Workflow](https://github.com/tideland/go-boxcopy/actions/workflows/build.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/tideland/go-boxcopy)](https://goreportcard.com/report/tideland.dev/go/boxcopy)

## Overview

Tideland Go BoxCopy is an open-source CLI tool for copying IMAP mailboxes from one server to another. It facilitates mail server migrations by performing a clean, one-time copy of all mailboxes from a source server to a target server.

## Features

- One-way copy from source to target server (clean migration)
- Safe by default: dry-run mode without `--perform`
- Single confirmation prompt before any destructive action
- Target cleanup before copy: all existing messages on target are removed first
- Initial en-bloc copy (folders first, then all messages)
- Full flag copying (read/unread, starred, deleted, etc.)
- Configurable throttling (messages per second, concurrent connections)
- Mandatory encrypted credentials with administrator-managed key
- Secure config file permissions (mode 0600 required)
- Progress logging at configurable percentage milestones
- Clean copy: Target is always cleaned first to ensure consistency

## Installation

```bash
go install tideland.dev/go/boxcopy/cmd/boxcopy@latest
```

Or build from source:

```bash
git clone https://github.com/tideland/go-boxcopy.git
cd go-boxcopy
make install
```

## Workflow

```
1. boxcopy init                        # Generate config at ~/.boxcopy/config.toml
2. vim ~/.boxcopy/config.toml          # Edit with your server details
3. boxcopy encrypt-password -k <key>  # Encrypt each mailbox password
4. boxcopy copy -k <key>              # Dry-run: review what would be copied
5. boxcopy copy -k <key> --perform    # Actual copy (asks for confirmation)
6. boxcopy verify -k <key>            # Verify: compare folder structure and sizes
```

## Usage

**Important**: All command options come after the command name.

```bash
# Dry-run (no changes made)
boxcopy copy -k <key>

# Actual copy (asks for confirmation, then cleans target and copies)
boxcopy copy -k <key> --perform

# With custom config
boxcopy copy -c /path/to/config.toml -k <key> --perform

# Helpers
boxcopy init                         # Generate initial config
boxcopy init -o /etc/boxcopy.toml    # Generate at specific path
boxcopy encrypt-password -k <key>    # Encrypt password for config
```

## What --perform does

1. Prints a summary of source, target, and mailboxes
2. Asks for confirmation: type `YES` to continue
3. Connects to each target mailbox and expunges all existing messages
4. Copies all folders and messages from source to target
5. Applies rate limiting (messages per second) and parallel connections

If target cleanup fails for any mailbox, the entire operation is aborted — no partial copies.

## Security Requirements

### Config File Permissions

- Config file MUST have mode 0600 (owner read/write only)
- `boxcopy init` creates the file with correct permissions
- Tool refuses to start if permissions are insecure

### Password Encryption

- All passwords MUST be encrypted (plaintext passwords rejected)
- Use `boxcopy encrypt-password -k <key>` to encrypt
- Encrypted passwords are base64-encoded strings

### Encryption Key

- The key is required for `copy` and `encrypt-password` commands
- Same key must be used for encryption and decryption
- Key is passed via `-k <key>` flag (no environment variable)
- Administrator is responsible for key management and secure storage

## Configuration

BoxCopy uses TOML configuration. Default location: `~/.boxcopy/config.toml`

```toml
# Config file must have mode 0600

[general]
state_file = "~/.boxcopy/state.dat"
log_level = "info"   # debug, info, warn, error
progress = 10        # Log progress every N% (0 = disable)

[copy_parameters]
messages_per_second = 10  # Rate limit for message copy
max_connections = 5       # Max concurrent mailbox connections

[source]
host = "imap.source-server.com"
port = 993
tls = true

[target]
host = "imap.target-server.com"
port = 993
tls = true

# All passwords must be encrypted
[[mailbox]]
name = "user1"
source_user = "user1@source.com"
source_password = "SGVsbG8gV29ybGQ..."
target_user = "user1@target.com"
target_password = "SGVsbG8gV29ybGQ..."

[[mailbox]]
name = "user2"
source_user = "user2@source.com"
source_password = "SGVsbG8gV29ybGQ..."
target_user = "user2@target.com"
target_password = "SGVsbG8gV29ybGQ..."
```

### Encrypting Passwords

Use the built-in helper (same key for all passwords and the copy command):

```bash
boxcopy encrypt-password -k mykey
Enter password to encrypt: ********

Encrypted password:
SGVsbG8gV29ybGQ...

Add this to your config file as source_password or target_password.
```

## Verification

After copying, run the built-in `verify` command to confirm the migration completed correctly. Both connections are strictly read-only — no data is modified on either server.

```bash
boxcopy verify -c ~/.boxcopy/config.toml -k <key>
```

The command compares, for each mailbox, the folder structure (names), per-folder message counts, and total per-folder RFC822 sizes between source and target, then prints a per-folder PASS/FAIL table with an overall result. No message bodies or headers are downloaded.

Counting messages and summing their sizes is sufficient to detect missing or truncated messages without fetching any content. If a message is absent or was corrupted in transit, the totals will diverge.

**Note:** both servers remain live during verification. Any mail event between the end of the copy and the verify run — a new arrival, a spam filter moving a message to Trash, an auto-expunge — will appear as a discrepancy even if the copy itself was correct. Run `verify` immediately after `copy --perform`, before mail clients reconnect or server-side filters trigger.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - Technical architecture and design
- [CHANGELOG.md](CHANGELOG.md) - Version history

## License

BSD-3-Clause - see [LICENSE](LICENSE)

## Author

Frank Mueller - Germany - https://themue.dev
