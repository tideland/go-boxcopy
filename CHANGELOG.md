# Changelog

All notable changes to Tideland Go BoxCopy will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.2.0]

### Added

- `boxcopy verify` command for post-copy validation: compares folder structure,
  per-folder message counts, and total RFC822 sizes between source and target
  (read-only, no message bodies downloaded)
- `docs/NEXTSTEPS.md`: prioritised analysis of bugs and improvements across
  four tiers (correctness, robustness, code quality, test coverage)
- `state.UpdateMailboxLastSync`: dedicated method that updates the mailbox-level
  last-sync timestamp and sync count without creating a phantom empty-name folder
  entry in the state file
- `config.Validate()` now checks each `[[mailbox]]` entry for non-empty `name`,
  `source_user`, `source_password`, `target_user`, and `target_password`

### Fixed

- **Folder-copy errors silently ignored** (`internal/mailbox/mailbox.go`):
  `copy()` accumulated folder-level failures in a counter and now returns an
  error when any folder fails, instead of always returning `nil`
- **Message-copy errors silently ignored** (`internal/mailbox/copy.go`):
  individual `appendMessage` failures are counted; a non-zero count is logged
  at `Error` level and causes `copyFolder` to return an error
- **`progress = 0` did not disable progress logging** (`internal/mailbox`):
  removed the `<= 0 → 10` clamp in `New()`; `m.progress > 0` is now checked
  inside the milestone block so zero correctly disables logging
- **`UpdateLastSync("", …)` created phantom folder entry** (`internal/state`):
  replaced the call-site with the new `UpdateMailboxLastSync` method
- **`"warning"` rejected by config validator** (`internal/config`):
  `isValidLogLevel` now accepts `"warning"` in addition to `"warn"`, consistent
  with what the log-level switches already handled
- **Verifier skipped config validation** (`internal/verifier`):
  `Verify()` now calls `cfg.Validate()` after loading config, matching the
  behaviour of `runner.Run()`
- **Log level in config file was ignored** (`internal/verifier`):
  logger resolution now follows the order: test-provided logger →
  `ExplicitLogLevel` CLI value → `cfg.General.LogLevel` from the config file
- **Stale `.tmp` state file left on hard crash** (`internal/state`):
  `state.Load()` removes any pre-existing `.tmp` file before reading state,
  preventing confusion after an unclean shutdown
- **Duplicated IMAP fetch-item parsing loop** (`internal/imap/message.go`):
  extracted `parseFetchMessage`, `buildFetchOpts`, and `drainFetchCmd` helpers;
  `FetchByUID` and `FetchAll` now share the same parsing path
- **`FetchAll` and `FetchAllSizes` used sequence-number sets** (`internal/imap`):
  both methods now use a UID range (`1:*`) which is stable against concurrent
  expunges, consistent with the rest of the IMAP client
- **`UIDSet` serialised in non-deterministic order** (`internal/state`):
  `MarshalJSON` now sorts the UID slice before encoding, making the state file
  deterministic across saves
- **Close errors discarded on login failure** (`internal/imap/client.go`):
  `Dial()` now logs a warning when closing the raw connection after a failed
  login, rather than silently suppressing the error
- **Close errors discarded on connect cleanup** (`internal/mailbox/mailbox.go`):
  `connect()` logs a warning when closing the already-open source connection
  after a target-connection failure
- **`Disconnect()` swallowed disconnect errors** (`internal/mailbox/mailbox.go`):
  public `Disconnect()` now returns the error from the internal `disconnect()`
  instead of discarding it
- **Deferred `disconnect()` error lost on successful copy**
  (`internal/mailbox/mailbox.go`): `copy()` uses a named return value so a
  disconnect error is propagated when the copy itself succeeded
- **Deferred `worker.Stop()` error lost** (`internal/runner/runner.go`):
  `performCopy()` uses a named return value so a pool-stop error is propagated
  when the copy itself succeeded

### Changed

- **Misleading comment in `copy.go`** (fix 3.2): replaced uncertain drain-channel
  comment with a precise explanation of the fetcher-close behaviour
- **Misleading lock comment in `mailbox.go`** (fix 3.3): replaced contradictory
  comment with "Copy stats under lock to avoid holding the lock during logging"

## [0.1.0] — Initial release

### Added

- Initial project structure
- TOML configuration parsing (`internal/config`)
- IMAP client wrapper (`internal/imap`)
- Mailbox copy implementation (`internal/mailbox`)
- Copy state persistence (`internal/state`)
- Copy runner with safety confirmation, target cleanup, parallel copy
  (`internal/runner`)
- CLI commands (`cmd/boxcopy`): `copy`, `encrypt-password`, `init`
- Dry-run mode by default; `--perform` flag for actual copy
- Single safety confirmation prompt before any destructive action
- Target cleanup before copy (expunge all messages, delete non-INBOX folders)
- `WorkerPool` for parallel mailbox copying
- `Throttle` for per-message rate limiting
- Progress logging at configurable percentage milestones
- BSD-3-Clause license
