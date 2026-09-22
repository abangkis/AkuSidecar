# Database health, orphan cleanup, and retention

Status: implemented maintenance boundary (2026-09-22).

SQLite writer connections use connection-time `foreign_keys(1)` and a 5-second
busy timeout, including replacement connections. Opening a writer verifies FK
enforcement. `PRAGMA foreign_keys` is connection-local; a schema declaration or
one startup PRAGMA is insufficient. Deleting a session with enforcement off
deletes only that parent and leaves children behind; cascade does not run.

## Inspection and retention

`GET /api/database/health` and bootstrap's `databaseHealth` expose `quick_check`,
FK enforcement, FK-violation count, unique disconnected-row count, affected
relationships, repair eligibility, and an issue fingerprint. They do not expose
row content. The fingerprint hashes the affected contents and relationship
identities, so a changed issue invalidates a previous cleanup decision. Unrelated
healthy writes do not create new issue identities.

Inspection also checks current operational last-run pointers and Personal Memory
child references that intentionally have no SQL FK. Historical audit references
(for example content-context feedback) are not current ownership and are not
classified as orphan data. Personal Memory issues require manual repair.

Every retention invocation reserves the SQLite writer and performs preflight in
the deletion transaction. Nonzero issues or disabled FK enforcement return
`ErrDatabaseMaintenanceRequired` before deletion. Learning projection, eligible
deletions, and postflight share that transaction. A non-pristine postflight rolls
the transaction back. New engine updates are gated on health as well. Reads remain
available. A repairable issue does not abort application startup: the HTTP/UI
surface stays available for inspection and the explicit cleanup decision while
retention and new updates remain paused. No scheduler retention call silently
repairs existing damage.

Only expired terminal sessions are eligible. Prepared unread sessions are
protected. Storage pressure reclaims free pages but cannot delete a young session;
`RetentionResult.storagePressure` reports remaining pressure. The storage size
still measures the whole database, not just event memory.

## User action and cleanup

The UI opens a maintenance dialog once per issue fingerprint, with **Back up and
clean**, **Clean without backup**, and **Later**. Deduplication survives refresh in
local storage. Settings → Database can reopen the action. Later leaves the data
and maintenance gate in place. Nonrepairable issues show the manual-repair state
and disable deletion actions.

Bootstrap inspects health immediately. Visible UI health refreshes run on a
separate 60-second cadence, skip overlapping scans and cleanup, and pause while
the document is hidden. Routine 15-second batch-status polling does not inspect
the database. Retention still performs its own immediate transactional checks;
the UI notices a pending issue on the next health refresh or bootstrap.

`POST /api/database/cleanup` requires the explicit payload:

```json
{"confirmed": true, "fingerprint": "value from current inspection", "backup": true}
```

Cleanup refuses active sessions and active media recapture, serializes against
engine operations, reserves the SQLite writer, and rechecks the fingerprint and
health inside its transaction. It projects durable preference learning before
deleting. Only explicitly allowlisted operational table/parent relationships are
eligible; unknown FK damage and durable Memory damage fail closed. Deferred FK
checking permits connected damaged rows to be deleted in one transaction; FK
enforcement remains on. Postflight must have zero FK and application-reference
issues and successful quick-check before commit. Personal Memory and the durable
learning ledger are not deletion targets.

Backup uses SQLite `VACUUM INTO` to make a consistent copy beside the database in
`backups/pre-orphan-cleanup-<timestamp>.db`, then reopens that copy read-only,
checks structural integrity and confirms the same issue fingerprint. Unlike the
full-reset verifier, repair backup verification must allow the preexisting orphan
data to be preserved. Failed backup verification prevents deletion. The response
returns health, removed operational row counts (including cascades), and backup
path when requested. A failed subsequent repair may leave a valid backup file.

After successful cleanup, the next update or retention automatically passes the
health gate. Normal retention reclaims physical free pages separately; cleanup
does not promise a smaller database file or reset product settings. The dev
database may be cleaned without backup when explicitly authorized.

## Remaining lifecycle work

This change closes the integrity gap and young-session storage eviction. It does
not decouple Timeline/feedback from sessions, split durable versus operational
storage budgets, add independent TTLs for transient payload classes, or add durable
retention receipts. Those require separate lifecycle migrations and policy work;
do not infer their completion from zero FK violations. A pristine health result
means the inspected invariants hold, not recovery of already deleted parent data.
