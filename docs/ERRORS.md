# Errors and Warnings

This document lists user-facing errors (critical) and warnings (non‑critical) emitted by boxcopy, grouped by meaning with short guidance on how to avoid or resolve them. Use this when preparing runs or troubleshooting failures.

---

## Errors (must be fixed before retry)

1. "config path is required"
   - Meaning: No config file path provided to the runner.
   - Avoidance: Provide -c/--config or use default (~/.boxcopy/config.toml). Run `boxcopy init` to create the template.

2. "encryption key is required (-k flag)" / "encryption key is required: use -k <key>"
   - Meaning: copy/verify/encrypt-password needs the encryption key.
   - Avoidance: Supply the same key used to encrypt passwords with -k. Keep keys secret.

3. "config file ... has insecure permissions" (mode != 0600)
   - Meaning: Config file permissions are too permissive.
   - Avoidance: Restrict the file: `chmod 600 config.toml`. Use `boxcopy init` which writes with 0600.

4. "failed to load config: ..." or "invalid config: ..."
   - Meaning: TOML parse errors, missing required sections/fields, or invalid values.
   - Avoidance: Validate config against template; ensure required [[mailbox]] entries and required fields.

5. "failed to decrypt passwords: ..." / "failed to decrypt password"
   - Meaning: The provided -k key cannot decrypt encrypted passwords (wrong key or corrupted value).
   - Avoidance: Use the exact key used at encryption. Re-encrypt with a known key if necessary.

6. IMAP connection/authentication failures: "failed to connect", "failed to login as <user>"
   - Meaning: Network, TLS, host/port, or credential problems when dialing/authenticating to source/target.
   - Avoidance: Verify host/port/TLS, test connectivity (telnet/openssl), confirm credentials, firewall rules, and reachable network.

7. "failed to select folder <name>" / "failed to list folders"
   - Meaning: Server refused SELECT/LIST (permissions, folder missing, or transient server error).
   - Avoidance: Verify account permissions and folder names. Use `verify` in dry-run to inspect remote folder lists.

8. "failed to append message UID %d: ..." / "append failed"
   - Meaning: Target server rejected APPEND (size, flags, server policy) or network error during upload.
   - Avoidance: Ensure target server accepts APPEND, check message sizes and allowed flags; verify target disk/quota.

9. "append succeeded but server returned no valid UID" / "server returned invalid UID 0"
   - Meaning: Server returned unexpected response; server may be incompatible or buggy.
   - Avoidance: Test with a small manual append; consult server logs/maintainer; consider using a different target or IMAP client settings.

10. "failed to expunge folder <name>: ..." / "expunge failed"
    - Meaning: Failure during target cleanup (cannot permanently remove messages).
    - Avoidance: Verify account has permission to EXPUNGE; ensure no server-side restrictions; run with appropriate account.

11. "target cleanup failed: ..." or "%d mailbox(es) failed to clean"
    - Meaning: One or more mailboxes could not be cleaned; by design the run aborts to avoid partial copy.
    - Avoidance: Fix per-mailbox errors (authentication, permissions) before re-running. Use dry-run first.

12. "failed to create worker pool" / "failed to stop worker pool"
    - Meaning: System resource limits or invalid pool size prevented worker pool creation or shutdown.
    - Avoidance: Ensure MaxConnections is a reasonable positive integer and system has capacity (file descriptors, OS limits).

13. "copy timed out after <duration>: ..." / "copy timed out"
    - Meaning: Parallel copy phase exceeded MaxCopyDuration.
    - Avoidance: Increase --max-copy-duration (Options.MaxCopyDuration) or reduce workload (fewer mailboxes, adjust max_connections/messages_per_second).

14. State persistence errors: "failed to save state: ..." / "failed to write state file"
    - Meaning: Disk permissions, full disk, or path issues when writing state file.
    - Avoidance: Ensure StateFile path is writable, directory exists or is creatable, and has correct permissions. Check disk space.

15. State file version incompatibility: "state file version X is newer than supported"
    - Meaning: On-load the state file has a newer version than the code supports.
    - Avoidance: Use compatible boxcopy version or migrate state using provided tooling; keep versions aligned.

16. "message has invalid UID 0" / "server returned invalid UID 0"
    - Meaning: IMAP server returned UID 0 which is invalid per spec.
    - Avoidance: Check server behavior; skip or report to server admin. Do not rely on servers that violate UID rules.

17. "mailbox copy failed" / "%d mailbox(es) failed to copy"
    - Meaning: One or more mailboxes experienced fatal errors during copy.
    - Avoidance: Inspect per-mailbox logs (slog entries), fix connectivity/permission issues, and re-run. Use smaller batches to isolate.

18. CLI usage errors (unknown command / missing required flag)
    - Meaning: Provided command or flags are incorrect.
    - Avoidance: Run `boxcopy help` and follow usage. Supply required flags (-k for copy/verify/encrypt-password).

---

## Warnings (informational / non‑fatal)

1. "failed to close connection" / "failed to close source connection during cleanup"
   - Meaning: Close/Logout returned an error, often network-related; run continues.
   - Avoidance: Usually transient; re-run if problems persist. Check network stability.

2. "folder already empty" (during cleanup)
   - Meaning: No action required; informational.
   - Avoidance: None. Normal if target folder is already empty.

3. "folder expunged" / "folder deleted" (info messages during cleanup)
   - Meaning: Informational: cleanup completed for that folder.
   - Avoidance: N/A.

4. "failed to create folder" (during folder creation)
   - Meaning: Target folder creation failed for one folder; copy continues for others.
   - Avoidance: Check target account permissions and folder naming constraints; create missing parents manually if necessary.

5. "batch fetch failed" / "fetch failed"
   - Meaning: A fetch batch failed; fetcher may stop that stream, copier will continue with available data or abort folder after retries.
   - Avoidance: Reduce batch size, check source server stability, inspect network.

6. "failed to copy message" (per-message) — counted and reported
   - Meaning: Individual message append or processing failed; other messages continue.
   - Avoidance: Inspect message (size/flags) and server limits; re-run after fixing problematic messages or accept skips.

7. "UIDValidity changed, resetting state"
   - Meaning: Source mailbox was rebuilt; sync state reset to avoid UID collisions.
   - Avoidance: This is expected if mailbox was recreated. Avoid rebuilding source mailboxes during copy runs.

8. "all messages already copied" / skipped messages
   - Meaning: State indicates message already copied; informational.
   - Avoidance: Normal on retries. To force re-copy, clear state for mailbox (state.ClearMailbox) or remove state file.

9. "keepalive NOOP failed, stopping keepalive"
   - Meaning: Keepalive encountered persistent failures and stopped; connection may still be usable.
   - Avoidance: Check network and server stability; increase keepalive interval or disable if problematic.

10. Rate-limit skips (broadcast semantics or throttle delays)
    - Meaning: If consumers are slow, broadcast semantics may drop items; throttle enforces pacing.
    - Avoidance: Tune messages_per_second and max_connections to match target capacity; increase buffers if applicable.

11. "progress" and other informational logs
    - Meaning: Periodic info on percent complete; no action required.
    - Avoidance: N/A. Disable by setting progress=0 in config.

---

## Troubleshooting checklist (quick)

- Ensure config file exists and is mode 0600.
- Use `boxcopy encrypt-password -k <key>` to produce encrypted passwords and store them in config.
- Test IMAP connectivity manually (telnet/openssl) and verify credentials.
- Run dry-run (`boxcopy copy -k <key>`) before `--perform`.
- If a mailbox fails, inspect logs shown on stderr (slog) and rerun for that mailbox after fixes.
- Ensure StateFile path is writable and has secure permissions.
- Adjust copy parameters: reduce max_connections or messages_per_second if servers fail under load.
