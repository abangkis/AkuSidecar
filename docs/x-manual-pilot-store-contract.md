# X manual pilot: durable claim and result contract

Implementation boundary: 2026-10-24. Schema compatibility barrier: 29 to 30.

The Store contract is independent of browser observations and broker routing.
A development-only trusted Go issuer now connects private startup validation to
Store authority. No native provider is registered in main, and a successful QA
report or a public Boolean cannot create Store authority. Validation of this
connection opens only synthetic test databases, never the user's database.

## Authority and durable fencing

A process-local opaque permit binds one Store instance, database incarnation,
session, X headless run, attempt/epoch, owner generation and reader/collector pair.
The session fence and operation receipts persist in `meta`. Permit pointers and
monotonic deadlines do not persist. Reopening or copying a database does not
restore a permit. Serialization and copied permit values cannot mint authority.

`IssueSyntheticManualPilot` exists only with `auth_session_dev`; it requires
explicit factory opt-in and synthetic owner/pair references. Existing account
checkpoint records are incompatible and are rejected without relabeling them.
Ordinary production issuance remains unavailable.

## Private Engine and startup composition

The development issuer accepts a trusted, typed Go validation port, rather than
JSON or diagnostic flags. Engine supplies that port from the already validated
manual startup vector: trusted recipe provenance, matching account/login evidence
for both roles, exact pair/attempt/policy/consent and full process/context/document
bindings. The Store receives a SHA-256 digest of this binding, not account IDs or
raw browser evidence. Offline vectors cannot be promoted into this path; tests
exercise the trusted recipe using invented source responses and owners.

The issued deadline is the earlier of the original start plus 60 seconds and the
validated vector's explicit limit. Issuance validates before SQL and again after
SQL; an observed invalidation during publication closes the permit. Broker stop
also revokes the Store permit. SQL, backend capture and post-commit scheduling
execute outside broker/controller locks and atomic-effect callbacks. Existing
trusted owner state/inspection checks retain their earlier broker/controller
locking; this increment does not claim to move all owner-port I/O out of locks.

The private Engine connection requires a confirmed Store claim before capture.
It binds that claim to the runtime ticket and exact owner, checks the observation,
then persists through `CommitManualPilotResult`. The ordinary and pilot paths
share the post-persistence continuation. Its scheduling/delivery error is returned
separately from the durable Store outcome, so a committed result cannot become an
automatic capture retry. Receipt reconciliation remains available after expiry
or invalidation without granting another capture.

This composition remains private, development-only and default-off. There is no
new CLI flag, HTTP endpoint, native factory registration, session transfer or
main-dev activation. Synthetic acceptance establishes the wiring contract; it
does not establish native X execution or production enrollment.

Generic claim, observation save, failure and lease recovery cannot mutate pilot
work around its fence. This includes unstamped follow-up commands and other runs
in the protected session. Cancellation closes the persisted authorization in the
same transaction as cancellation. Ordinary unprotected sessions retain their
existing behavior.

## Transaction protocol

1. `ClaimManualPilot` validates the live permit and exact persisted queued command,
   then atomically records the command claim and an operation receipt. A dispatch
   handle is returned only after a confirmed claim commit. Repeating the same
   claim reports the prior outcome without granting another dispatch. If the
   original invocation reconciles an ambiguous commit through its matching
   receipt, it may return that first handle with `AlreadyCommitted`; a later
   replay returns no handle. Dispatch still rechecks the live controller deadline.
2. `CommitManualPilotResult` requires that handle, matching claim ownership and an
   X observation. It uses the existing observation transaction, including
   `observations.command_id` uniqueness, and commits the result and receipt
   together. The caller's observation is not modified.
3. Both transactions check authority again immediately before COMMIT. The absolute
   deadline is the original attempt start plus at most 60 seconds, including
   startup time. Equality with the deadline is rejected. The decision time is
   recorded; SQLite COMMIT may finish after that decision or deadline. This is the
   approved pre-COMMIT policy, not a guarantee about disk completion latency.
4. Each follow-up needs a new claim under the still-live original permit. Claiming
   or saving a result never extends the attempt. After revocation or expiry,
   queued work remains fenced and uncertain claimed work is not recaptured.

Browser/owner checks happen outside Store transactions. The private Engine
connector binds its trusted startup evidence and original deadline to the
development issuer; it keeps browser and controller locks out of SQL transactions.
Registering and qualifying that connector with actual native X remains separate
acceptance work.

## Outcomes and reconciliation

| Outcome | Meaning and caller action |
| --- | --- |
| `Committed` | The transaction committed; persistence succeeded. |
| `AlreadyCommitted` | A matching receipt proves commit. Never redispatch or ingest from the outcome alone; only the original claim invocation can return its first dispatch handle. |
| `RejectedNoMutation` | This operation made no committed mutation; a prior operation may still exist. |
| `Indeterminate` | Commit was attempted but its outcome could not be proved; reconcile, never recapture automatically. |

Operation IDs are immutable bindings to kind, scope, command and payload digest.
Reusing one with different content is rejected. Matching result receipts are
checked before live-authority expiry checks, so expiry does not erase knowledge
of a committed result. `LookupManualPilotOp` can read receipts after reopening
without issuing dispatch authority. A missing receipt after a commit error is
insufficient evidence to retry capture. Persistence outcomes must remain separate
from errors in later scheduling or response delivery.

## Upgrade, downgrade and validation

The 29-to-30 migration changes only the schema compatibility marker, preserving
queues, observations, account checkpoint data and fences. Schema 30 marks new
authorization semantics even though no new table is needed. A schema-29 binary
must refuse the database; downgrading by rewriting the marker is unsupported.

Run `scripts/test-manual-pilot-store.ps1`. It runs the Store suite in default,
development, fixture and combined builds. It also builds a real schema-29
consumer from pinned local commit `28fe840e91d75b184702949d51b0622b3a364f2b`,
creates queued/claimed synthetic work, upgrades that same database, and verifies
the old binary refuses it without changing its bytes. No fetch is performed.
All temporary databases and binaries stay under a unique project QA root and are
removed after success; evidence logs remain. This does not test native issuance,
session transfer, production Store enrollment or user-runtime activation.

Acceptance run `408dd7eece0644aeaccf386524319c60` passed the complete Store
suite in all four build modes and all four historical-consumer compatibility
checks. It includes eleven manual-pilot top-level tests across the development
suite, with regressions for original-start expiry at the final transaction gate,
revocation/cancellation ordering, follow-ups, generic bypass, copied/reopened
authority, operation conflicts and ambiguous commits. The untagged persisted-fence
and migration tests also passed in normal and fixture-only builds. Successful-run
temporary databases, pinned source and binaries were removed.
Evidence: `runtime/qa/auth-session-sidecar/manual-pilot-store-408dd7eece0644aeaccf386524319c60.log.txt`.

Run `scripts/test-manual-pilot-engine-store.ps1` for the synthetic Engine/startup
connection and ordinary post-observation scheduling regressions. It compiles the
actual Engine production sources, explicitly excludes browser test files and
package TestMain, and runs only named accountless tests with project-owned
temporary databases. The successful Store run above remains evidence for its
unchanged schema migration and historical-consumer barrier.

The Engine connection passed run `b48eb725ea2f415d958a4a08deeaf759`: nineteen
synthetic integration cases and four ordinary Engine continuation regressions.
The cases include the actual deterministic Engine reasoning continuation,
development opt-in, strict startup rejection, source/owner drift, expiry and
cancellation during capture, follow-ups under the original deadline, issuance
invalidation, committed-result delivery failure and restart receipt readback.
Store and backend probes confirm those operations run outside broker/controller
locks. This does not extend that claim to the inherited owner-inspection path.

The current Store issuance refactor passed its complete four-build suite and
historical-binary compatibility checks in run
`0a19c64d0f554fb8a5e071fea294c91c`. The unchanged non-Store main/broker/Coordinator
fixture passed all ten actual-main scenarios, four build variants, early native
provider rejection and absent config/database sentinels in run
`8d59d1ce1276459f9a69f2ad33ce88e4`. All successful-run temporary roots were removed.
Evidence logs remain under `runtime/qa/auth-session-sidecar/` with these run IDs.
