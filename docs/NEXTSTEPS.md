# BoxCopy – Next Steps

Prioritized list of bugs, gaps, and improvements identified by static analysis of the
codebase (2026-04-01). Items are grouped by urgency; within each group they are ordered
by impact.

---

## Priority 1 – Bugs / Correctness

These items can cause silent data loss or incorrect behaviour and should be fixed before
the tool is used in production.

### 1.1 `copy()` swallows folder-copy errors — always returns `nil`

**File:** `internal/mailbox/mailbox.go`, `copy()`, lines 343-384
When `copyFolder()` fails the code logs a warning, disconnects, reconnects and
continues with the next folder. No error is accumulated and the function always
returns `nil`. The caller in `runner.go` therefore never counts the mailbox as
failed, and the final copy report says "success" even when whole folders were not
copied.

**Fix:** Add a `folderErrors int` counter in the loop. After all folders are
processed, return a wrapped error if `folderErrors > 0`.

---

### 1.2 `copyMessage` errors are swallowed — partial copies treated as complete

**File:** `internal/mailbox/copy.go`, `copyFolder()`, lines 210-216
Individual message copy failures are logged as warnings only. `copyErr` is set only
for batch-fetch failures and context cancellation, never for `appendMessage`
failures. A folder where 500 of 1 000 messages failed to append reports "copy
completed" with no error.

**Fix:** Accumulate message-level errors in a counter; return an error from
`copyFolder` (or at least log a summary) when the count is non-zero.

---

### 1.3 `progress = 0` does not disable progress logging

**File:** `internal/mailbox/mailbox.go`, `New()`, lines 121-123
The config template documents `progress = 0` as "disable", but the constructor
clamps any value `<= 0` to `10`, so zero silently becomes every-10-percent
logging.

**Fix:** Remove the clamp. Check `m.progress > 0` inside the milestone block in
`copyFolder` (line 221) instead of clamping at construction time.

---

### 1.4 `UpdateLastSync("")` creates a phantom folder entry in state

**File:** `internal/mailbox/mailbox.go`, `copy()`, line 379
`m.state.UpdateLastSync(m.config.Name, "")` is called with an empty folder name
after a full-mailbox copy completes. `state.UpdateLastSync` calls
`getOrCreateFolder(mailbox, "")`, which writes a `FolderState` with `Name: ""`
to the persisted JSON. This is misleading noise in the state file and could
interfere with future state inspection tools.

**Fix:** Either add a dedicated `UpdateMailboxLastSync(mailbox string)` method that
does not touch folder state, or guard `getOrCreateFolder` against empty folder names.

---

### 1.5 `isValidLogLevel` rejects `"warning"` but `setupLogger` accepts it

**File:** `internal/config/config.go`, `isValidLogLevel()`, line 343
`Validate()` → `isValidLogLevel()` only accepts `"warn"`.  `main.go`'s
`setupLogger` also accepts `"warning"`. A config file containing
`log_level = "warning"` fails validation even though the equivalent CLI flag
works fine.

**Fix:** Add `"warning"` as an accepted value in `isValidLogLevel`.

---

## Priority 2 – Robustness / Usability

These items do not cause immediate data loss but reduce reliability or make the tool
harder to operate correctly.

### 2.1 Verifier does not validate config before connecting

**File:** `internal/verifier/verifier.go`, `Verify()`, lines 74-91
`runner.Run()` calls `cfg.Validate()` after loading config, but `verifier.Verify()`
does not. A config with `messages_per_second = 0` or an unknown `log_level` passes
silently into the verifier run.

**Fix:** Add `cfg.Validate()` call in `Verify()` after `config.Load()`, consistent
with `runner.Run()`.

---

### 2.2 No graceful shutdown on SIGINT / SIGTERM

**File:** `internal/runner/runner.go`, `performCopy()`, line 167
The copy context is `context.Background()` with no cancellation. Pressing Ctrl+C
kills the process immediately. An in-progress folder copy cannot save its partial
state, and the worker pool goroutines are leaked from the OS perspective (though
they are cleaned up by process exit).

**Fix:** Use `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`
and pass the resulting context to the mailbox worker. Add a `defer cancel()` call.
State is already saved atomically via temp-file rename, so graceful cancellation
will leave the state consistent up to the last completed folder.

---

### 2.3 `cleanTargets` runs serially — slow for many mailboxes

**File:** `internal/runner/runner.go`, `cleanTargets()`, lines 253-265
Target cleanup opens one IMAP connection per mailbox in sequence. With 50 mailboxes
and large folders, this can add significant wall-clock time before copying even
starts.

**Fix:** Parallelise cleanup using the same `worker.WorkerPool` pattern used for
copying. Cap parallelism at `cfg.CopyParam.MaxConnections`.

---

### 2.4 No IMAP connection timeout

**File:** `internal/imap/client.go`, `Dial()`
`imap.Dial` uses no dial or I/O timeout. If the IMAP server is unreachable (e.g.
wrong host name, firewall drop), the connect call blocks until the OS TCP timeout
— typically minutes — blocking a worker slot.

**Fix:** Pass a `net.Dialer` with `Timeout: 30 * time.Second` (or a value from
config) when constructing the IMAP client.

---

### 2.5 No IMAP keepalive — long copies risk server-side idle timeout

**File:** `internal/imap/client.go`
For large mailboxes at 10 messages/second a full copy can take hours. Many IMAP
servers close idle connections after 30 minutes. There is no periodic `NOOP`
command to keep the connection alive between message appends.

**Fix:** Start a background goroutine per connection that sends `NOOP` every
5 minutes (or make the interval configurable). Cancel it when the connection is
closed.

---

### 2.6 `--log-level` CLI flag does not override config's `log_level`

**File:** `cmd/boxcopy/main.go`, `cmdCopy()` / `cmdVerify()`
The logger is created from the `--log-level` CLI flag, defaulting to `"info"` when
the flag is absent. The config's `log_level` field is loaded and validated but
never applied to the logger. A user who sets `log_level = "debug"` in config
gets `"info"` logging unless they also pass `--log-level debug` on the command line.

**Fix:** In `cmdCopy()` and `cmdVerify()`, fall back to `cfg.General.LogLevel` when
the `--log-level` flag is empty, after loading the config.

---

### 2.7 Hard-coded 24-hour copy timeout

**File:** `internal/runner/runner.go`, line 202
`worker.WaitForTasks(pool, 24*time.Hour)` is a magic number. A very large
migration (20+ years of mail for 50 users) could legitimately exceed 24 hours.

**Fix:** Add an optional `Timeout` field to `runner.Options` (defaulting to
`24 * time.Hour`). Alternatively, expose it as a `[copy_parameters]` config key
`max_copy_hours`.

---

### 2.8 Orphaned `.tmp` state file on hard crash

**File:** `internal/state/store.go`, `saveUnlocked()`, lines 100-108
If the process is killed between `os.WriteFile(tempPath, ...)` and the subsequent
`os.Rename`, the file `<state>.dat.tmp` is left on disk indefinitely. It does not
affect correctness (the next run reads only `.dat`), but it is surprising to users
and grows stale.

**Fix:** In `state.Load()`, detect and remove a stale `.tmp` file before
proceeding.

---

## Priority 3 – Code Quality / Maintainability

### 3.1 `FetchByUID` and `FetchAll` duplicate message-parsing logic

**File:** `internal/imap/message.go`, lines 141-196 and 233-291
Both functions contain an identical inner loop that maps `imapclient.FetchItemData*`
types to the local `Message` struct. Any future change to message parsing must be
applied in two places.

**Fix:** Extract a `parseFetchMessage(msg *imapclient.FetchMessageBuffer, opts *FetchOptions) (Message, error)`
helper and call it from both functions.

---

### 3.2 Misleading comment in `copy.go`

**File:** `internal/mailbox/copy.go`, line 196
The comment `// Drain channel if needed, but loop continues to consume remaining
batches if any?` is both uncertain and inaccurate. The fetcher closes the channel on
any error, so the `range batchChan` loop will terminate naturally — no explicit
draining is needed or performed.

**Fix:** Replace with a precise explanation: `// Fetcher closed the channel after the
error; the range loop will exit on the next iteration.`

---

### 3.3 Misleading lock comment in `mailbox.go`

**File:** `internal/mailbox/mailbox.go`, lines 373-374
`// We are reading protected fields here, so we need lock or use local vars. / But
logging is safe to do inside lock or copy to locals.` — the comment is internally
contradictory and doesn't explain the actual pattern (copy to locals, log after
unlock).

**Fix:** Replace with: `// Copy stats under lock to avoid holding the lock during
logging.`

---

### 3.4 `UIDSet` serialises in non-deterministic order

**File:** `internal/state/store.go`, `MarshalJSON()`, line 128
`UIDSet.ToSlice()` iterates over a map, producing a different element order on each
run. The state file therefore changes unnecessarily on every save, making diffs
noisy and slightly harder to audit.

**Fix:** Sort the slice in `MarshalJSON` before passing it to `json.Marshal`.

---

### 3.5 `config.Validate()` does not check individual mailbox fields

**File:** `internal/config/config.go`, `Validate()`, lines 318-338
Validation checks for at least one mailbox and valid server hosts, but does not
verify that each mailbox has a non-empty `name`, `source_user`, `target_user`,
`source_password`, or `target_password`. A config with blank credentials passes
validation and fails only at IMAP login time.

**Fix:** Add a loop inside `Validate()` that checks required per-mailbox fields.

---

### 3.6 `imap.Fetch` used with sequence set in `FetchAll` / `FetchAllSizes`

**File:** `internal/imap/message.go`, `FetchAll()` and `FetchAllSizes()`
Both methods use a sequence-number set (`imap.SeqSet{1:*}`) rather than a UID set.
Sequence numbers change when messages are expunged, so a concurrent expunge on the
server between SELECT and FETCH can cause mismatched results. This is unlikely in
practice (source is read-only during copy) but is a theoretical correctness hazard.

**Fix:** Use `client.Fetch` with a `imap.UIDSet` (UID range `1:*`) and set
`FetchOptions.UID = true`, consistent with the rest of the codebase.

---

## Priority 4 – Test Coverage

### 4.1 `progress = 0` disable behaviour has no test

Add a unit test for `New()` with `Options{Progress: 0}` that verifies no milestone
log entries are produced during a simulated folder copy.

---

### 4.2 No test for folder-copy error recovery (disconnect + reconnect)

`copy()` in `mailbox.go` has a reconnect path on folder failure. There is no test
exercising this path (the reconnect attempt and the continuation to the next folder).

---

### 4.3 No test for state file corruption or version mismatch

`state.Load()` handles the case where the version in the file is newer than the
binary supports, but this path is untested. Add a test that writes a state file with
`"version": 999` and verifies the error message.

---

### 4.4 No test for `cleanTargetMailbox`

The target-cleanup logic (expunge + folder deletion) is entirely untested. Consider
adding a test using an in-process IMAP server mock (e.g., `emersion/go-imap`'s
test server) or, at minimum, table-driven unit tests for the folder-deletion sort
order.

---

### 4.5 `TestRunDryRunFromFile` and `TestRunPerformDeclined` duplicate config setup

Both tests build the same TOML config string. Extract a `writeTempConfig(t, key)
string` helper to reduce duplication and make it easier to add more file-based
runner tests.

---

## Summary Table

| # | Area | Severity | Effort |
|---|------|----------|--------|
| 1.1 | `copy()` silently ignores folder errors | Critical | Small |
| 1.2 | `copyMessage` errors not aggregated | Critical | Small |
| 1.3 | `progress=0` does not disable logging | Bug | Trivial |
| 1.4 | `UpdateLastSync("")` phantom state entry | Bug | Small |
| 1.5 | `"warning"` rejected by config validator | Bug | Trivial |
| 2.1 | Verifier skips config validation | Important | Trivial |
| 2.2 | No signal handling for graceful shutdown | Important | Small |
| 2.3 | Serial target cleanup | Performance | Small |
| 2.4 | No IMAP connection timeout | Robustness | Small |
| 2.5 | No IMAP keepalive | Robustness | Medium |
| 2.6 | CLI `--log-level` doesn't override config | Usability | Small |
| 2.7 | Hard-coded 24 h copy timeout | Usability | Trivial |
| 2.8 | Orphaned `.tmp` state file on crash | Robustness | Trivial |
| 3.1 | Duplicated fetch message-parsing loop | Quality | Small |
| 3.2 | Misleading drain-channel comment | Quality | Trivial |
| 3.3 | Misleading lock comment | Quality | Trivial |
| 3.4 | Non-deterministic `UIDSet` JSON order | Quality | Trivial |
| 3.5 | Per-mailbox fields not validated | Quality | Small |
| 3.6 | Sequence-set fetch instead of UID-set | Quality | Small |
| 4.1–4.5 | Missing tests | Testing | Small–Medium |
