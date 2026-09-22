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

## Durable Timeline ownership (schema 27)

Timeline posts, More/Less feedback, AI assessments, media-provenance assessments,
and semantic reports do not belong to an operational session or run. Their
`session_id`/`run_id` values are historical provenance snapshots without parent
foreign keys. AI feedback likewise remains durable. Missing operational parents
are valid for these rows and are not cleanup targets. The Timeline API exposes
`originSessionAvailable` and `originRunAvailable`; retained IDs do not promise
that the original diagnostics still exist.

Before a session is deleted, a database trigger snapshots its actual status,
presentation order/time, and prepared-batch visibility onto its Timeline rows.
Before a run is deleted, another trigger retains the bounded displayed evidence
block from its observations. Raw observations, capture commands, invocations,
calibration, and other operational children still cascade normally. Existing
evidence overrides remain Timeline-owned. Unknown legacy origins stay
`unavailable`; migration never fabricates a parent or a completed status.

More/Less writes and preference learning use the retained Timeline assessment;
the feedback and learning-ledger entry commit together. Completed/partial origin
snapshots permit the same More/Keep/Read-later memory actions after retention.
Missing legacy evidence is not reconstructed. Hidden expired batches remain
hidden when their operational batch record disappears, and live prepared batches
retain their existing protection and reveal behavior.

Migration from 26 to 27 rebuilds the five affected tables on a reserved connection
in one transaction, preserving data, extra columns, indexes, and trigger
definitions. Foreign keys are disabled only on that migration connection outside
the transaction and restored before normal operation; the migration checks that
no new FK violations appeared. The schema marker and lifecycle triggers commit
atomically. Existing unrelated damage stays available to the maintenance UI.
Full Reset explicitly deletes durable Timeline rows; ordinary session retention
does not. Durable Timeline/feedback currently have no automatic age eviction.

## Finite-inbox observation (schema 28)

Schema 28 records the moment a card actually enters the visible Timeline.
User-triggered cards receive `presented_at` when their durable card is published;
prepared automatic cards receive it only when their batch is revealed. Migration
backfills visible schema-27 cards from batch reveal time, then session completion,
then card creation time. Hidden prepared or expired batches remain unpresented
and fail safe as protected.

Every retention invocation now writes one bounded aggregate receipt in
`timeline_retention_receipts`. At most 128 receipts are retained. Receipts contain
no card ids or content and run in `observe` mode: **no Timeline card is deleted**.
The canary policy protects cards for 14 days after presentation, starts routine
expiry eligibility at 30 days, and previews soft boundaries of 500 cards and
10 MiB of logical Timeline-card payload. Cards aged 14–30 days are candidates
only when a soft boundary is exceeded. Hidden cards and visible cards without a
valid presentation timestamp remain protected.

`GET /api/timeline/storage` returns current aggregate usage and eligibility,
including protected, eligible, routine-expired, hidden, and missing-presentation
counts. It reports effective and allocated whole-database bytes separately from
the Timeline logical payload. `wouldRemove*` is a dry-run estimate, not a cleanup
promise, and `needsAttention` means eligible candidates are insufficient to get
under the preview boundaries. The endpoint is read-only.

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

These changes close the integrity gap, young-session storage eviction, Timeline
and feedback operational ownership, and add finite-inbox measurement plus durable
dry-run receipts. They do not activate Timeline deletion, split every durable
category into a separately enforced budget, or add independent TTLs for every
transient payload class. Do not infer those behaviors from zero FK violations or
from a dry-run candidate count. A pristine health result means the inspected
invariants hold, not recovery of already deleted parent data.
