# X manual pilot: durable claim and result contract

For the current acceptance boundary and prepared capture QA, see
[X concurrency dev readiness](concurrency-dev-readiness.md). The prepared QA now
includes shared pacing/challenge admission; native execution remains unapproved.

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

## Actual Chrome capture with synthetic Store admission (2026-10-25)

The dual-tag Windows adapter now reads a post from actual Chrome DOM and returns
the production observation format through the shared Engine claim/result path.
It uses a dedicated capture RPC lane; control/owner inspection remains responsive
while the synthetic feed response is held. Capture opens a separate target in the
collector browser and keeps the startup identity target/document unchanged.
There is no caller-selected URL, source or script. The compiled loopback DOM
extractor supplies the actual text, author, permalink and post ID to production
`observation.mjs`; Go does not manufacture the captured post.

The Go reply is bounded to 140 KiB, rejects unknown fields, matches every frozen
startup pin, requires a distinct capture target and X source, and validates one
snapshot with at most twenty canonical post identities. Node bounds its
observation to 128 KiB, post text to 8,192 characters and author to 256 characters.
Capture/abort use separate lanes; retiring capture closes its dedicated target.

A separate test-only admission capability requires fresh Job-owned Chrome/Node
processes, distinct physical profile directories, verified directory handles and
the helper's explicit process gate. It issues a synthetic permit bound to the
full owner hash; it never changes offline provenance into native authority. The
native issuer still rejects the same offline vector. Synthetic issuance accepts
an optional earlier `ValidUntil`; it cannot extend the original 60-second limit.

Run `scripts/test-manual-pilot-chrome-store.ps1` for offline boundaries, tag gates,
compilation and disabled-entry verification. Add `-VerifyChrome` to execute the
fresh-profile loopback acceptance. No retained QA profile or native X operation
is included. The executable includes actual Engine production sources and only
the explicit local Chrome test/helper files; browser package TestMain and native
retained-profile tests are excluded.

Acceptance run `3a894c544d154329862cd0a778c05c92` passed five Node checks, four
tag gates, disabled-entry skip and two actual Chrome scenarios in 4.33 seconds:

- A held feed request overlaps an additional reader window. Startup/bootstrap
  pins and collector PID/generation remain unchanged. Real DOM evidence commits
  once, deterministic Engine reasoning completes, replay does not recapture, and
  closing the final reader closes admission.
- Cancellation during held capture rejects the late real DOM result with zero
  observations committed.

All four role Jobs/process trees were verified empty. The helper removed its
owned profiles, databases and test binary; the evidence log remains at
`runtime/qa/auth-session-sidecar/manual-pilot-chrome-store-3a894c544d154329862cd0a778c05c92.log.txt`.
This is actual Chrome execution with synthetic accounts/posts and Store permits.
It does not qualify native X selectors/network evidence, activate a main provider,
access a user's database, or authorize a real-session transfer.

Post-change regression evidence: `61390719cad04b3d9623ac7f2d697226` passed the
nineteen Engine/Store integration cases and four ordinary continuation tests;
`5c8a17242b7d4a3d8860532ed7c80bef` passed all four Store builds, shorter-deadline
cases and historical schema-29 consumer checks; `46d4d1b2d90841dbaf35fdae6a95a589`
passed all ten actual-main fixture scenarios and four build variants. Successful
regression roots were removed. Their logs remain under the same QA evidence root.

## Prepared native X capture through the existing worker (2026-10-25)

The native owner protocol now has private `manual_capture` and
`manual_capture_cancel` operations. A matching collector handle, frozen startup
pins, request correlation and original deadline are required. The Windows backend
routes capture through its dedicated RPC lane. The Engine composition constructs
the native backend from the same owner ports that prepared and acquired startup
evidence, then uses the existing guarded Store claim/result path. There is no
main, CLI or HTTP registration.

Capture calls production `sourceAssets` and `captureIsolated`, including the
AkuBridge X adapter, vendored X extractor and normal observation mapper. A new
target belongs to the already owned collector browser context; it cannot replace
the startup account-evidence target. The source-specific worker controls DOM
extraction and canonical X navigation. Source, arbitrary scripts, raw CDP commands
and public QA authority flags cannot be selected by a capture request.

The startup transport retains its original fixed-expression restrictions. A
separate private worker capability permits the operations needed for capture and
installs header/cookie observation before navigation. Authentication-cookie writes
or ambiguous header coverage reject capture. This capability cannot publish an
account identity receipt. Native cookie/target behavior still requires separately
authorized QA; offline success is not native account/session qualification.

The initial MVP capture supports acquisition round one only, with bounded scroll,
post count and hydration settings. Unsupported continuation/round-two requests
are explicitly rejected rather than changed into a new first round. Each native
capture uses an isolated frontier map; the ordinary worker's persistent frontier
behavior remains unchanged. Capture stops at the original attempt deadline and
retires only its own target. There is no new seed, retry, renewal or fallback.

Observation JSON is limited to 512 KiB; the complete escaped response envelope is
limited to 768 KiB. The Go adapter validates schema, correlation, exact pins,
distinct capture target, observation digest and X observation before allowing the
existing Store result transaction. Failure does not trigger another capture.

Use `scripts/test-x-manual-native-capture.ps1` for the accountless Node/Go tests.
The synthetic transport executes the actual parser/worker in a VM-backed DOM/CDP
environment, then passes its output through Node stdio, Engine, Store and
deterministic reasoning. No browser or profile is launched, no X request or session
transfer runs, and only newly created QA databases are opened. Prepared-source
fingerprints include the new native module and the worker/parser code assets.

Acceptance run `4c710fc29a2343d6a461895764bf015b` passed 68 Node checks, six Go
result scenarios and four tag gates. The Go test uses the actual owner-port
constructor, prepare/acquire transitions and `bindXNativePilotRuntime`; it then
executes the production parser over synthetic CDP/DOM through Node stdio. Valid
evidence commits and completes deterministic reasoning. Wrong owner, target,
source, digest and expiry reject the result and never recapture on replay.
Known missing HTTP/ExtraInfo coverage is rejected; non-HTTP WebSocket events do
not become HTTP response obligations. Live cross-target/service-worker coverage
remains unqualified.

Post-change regression runs `e0ecaf81c6a74df2bf971188fb97225a` and
`8f86940d29bf474f9c31ddeded22c8ad` passed the nineteen Engine/Store cases, four
ordinary continuation regressions and ten actual-main scenarios/four builds.
Store/schema code was unchanged, so the preceding Store compatibility evidence
remains applicable.

Preparation-only run `578e6bcdaecb4672b270fb7bc1f1ff3f` passed 278 startup/network
Node checks plus 32 owner-port checks, standalone compilation and disabled-entry
verification before profile access. Its plan retained `executionApproved=false`
and `defaultOff=true`, with 75 source fingerprints including 25 worker/parser
assets. This validates the existing startup QA preparation, not a native capture
execution through Engine/Store. A dedicated capture QA entry and fresh explicit
one-shot authorization remain required before a live test. Logs/provenance remain
under `runtime/qa/auth-session-sidecar/`; temporary databases, test binaries and
preparation roots from this increment were removed.
