# Shared daily concurrency core: X and Instagram

## Scope and compatibility

Daily concurrency is one replacement mechanism with source adapters. The
[shared-foundation progress report](shared-capture-foundation-progress.md) maps
the remaining lifecycle and windowed/native parity work to the convergence
roadmap. Instagram
does not receive a copy of the X pipeline. The ordinary Run, Observation,
deduplication, reasoning and Timeline pipeline remains shared. Existing X
configuration, endpoints, saved profile permission and acquisition receipts keep
their compatibility behavior.

The current convergence increment is offline. It does not access a signed-in
profile, derive a real session, migrate a user database, activate a runtime or
change Auto Update. The default Instagram route remains Browser. An explicitly
configured native trial and any earlier activation do not qualify its account
evidence recipe; a new handoff or live probe still needs separate authorization.

## Responsibilities

The shared core owns source-bound admission, per-profile saved permission,
generation fencing, cancellation/join, the original batch deadline, collector
lifetime, first-cause diagnostics and independent cleanup. Reader lifetime is
separate from a disposable batch. Closing a collector or temporary Home target
does not authorize closing a user reader. A later response cannot release an
unverified target, commit after expiry or revive a revoked generation.

Browser capture and native collector retirement have different resource owners.
The Bridge journal reconciles the requested lease's records, not whichever
lease currently occupies the managed-window singleton. A verified retired lease
must not starve pending Browser commands. A still-live tab can be closed only
with matching ownership, generation and source policy; an independently verified
absence can retire a record without granting authority over a live target. A
different current lease and user readers must remain untouched.

The source adapter owns native/Home URL policy, account/login/session evidence,
the transfer manifest and source-specific capture/media operations. It provides
evidence to the shared verifier rather than granting authority by itself. X keeps
its qualified data limits and existing native path. Instagram reuses its existing
collector/parser and structured media pipeline, with a source-bound temporary
page capability for media recovery.

The selector chooses the backend per source through the existing scheduling and
capture composition. Selecting a source must not borrow another source's owner,
permit, saved preference, evidence, target or cleanup receipt. Native Instagram
cannot become available merely because its synthetic adapter passes tests.

## Authority and Store

Permission remains scoped to source and reader profile. X permission does not
authorize Instagram; no stronger persistent account-enrollment policy is added
merely to generalize the core. Each batch still verifies fresh account/login
evidence and same-account continuity across the reader and collector.

The existing private Store permit machinery remains the authority for claim and
commit. Source, run, owner generation, revision and deadline are checked at the
transaction boundary. Distinct source state within one update session must not
overwrite an earlier source's durable receipt. A completed X run must remain
inspectable after Instagram starts, and a live Instagram permit must not allow
ordinary or X mutation of its guarded run.

The approved schema 30 to 31 transition is a semantic downgrade fence: schema 30
binaries only enforce the earlier X-specific guard and must not open a database
that may contain Instagram authority. Existing X record encoding remains
compatible with the new code. This does not authorize changing schema metadata
back to 30. A future operational rollback must use a compatible binary or an
explicitly approved pre-upgrade database restoration; source-only rollback is
insufficient after a user database has migrated. This increment tests synthetic
databases only.

## Instagram qualification boundary

The native provider implementation is now described in the
[native source provider contract](native-source-provider-contract.md). Its
private main-browser composition, proof/claim path and sealed packaging exist;
the live IG recipe remains qualification-required. The synthetic-only adapter
checkpoint below records the preceding increment, not native acceptance.

An authenticated-looking feed, an inbox/edit-account link, or cookie presence
alone does not establish account identity. A native Instagram adapter needs a
qualified bounded source for account/login evidence and a source-specific
session-data manifest. The X cookie manifest must never be extended implicitly
or reused against another domain. Raw credentials, cookies, response bodies and
account identifiers must not enter public status, diagnostics or logs.

Synthetic evidence and transfer transports exercise the implementation boundary;
they do not qualify a real Instagram source or authorize session transfer. Any
future native experiment needs concrete source/profile, evidence read and
transfer limits before execution. Full refresh/rotation qualification and a
shared reservation redesign remain deferred rather than new prerequisites for
the shared-core extraction.

## Implemented selector and ports

`capture.instagramMode` accepts `browser` (the default) or `concurrent` and is
read at startup. Existing `capture.xMode`, `xReaderSource` and `xReaderProfile`
remain compatible. Selecting concurrent Instagram does not register a native
factory or grant permission; its unavailable provider is reported rather than
falling through to X authority. No active configuration is edited here.

Both source routes use `dailyController` and the same
`sourceDailyOwner`/`sourceDailyBatch` ports. Existing X entry points delegate to
that controller; they are compatibility wrappers, not a second orchestration.
`SourceCaptureStatus(source)` projects readiness independently per source.
Trusted offline composition can attach the synthetic Instagram adapter, start it
through `StartInstagramDaily`, and revoke it independently with
`RevokeInstagramDailyPermission`. These Go methods do not create a native IG
factory or a public endpoint accepting fixture evidence.

Store retains its private permit, claim and commit model. X keeps the original
state and preference key encoding; IG uses a distinct source state suffix and
`instagram_daily_permission_v1`. The source-specific acquisition marker remains
backed by a private issuance record. Ordinary capture cannot select this policy
by adding coverage fields.

The worker exports `createSourceConcurrentAdapter`. Its current evidence domain
is explicitly `synthetic-fixture`, and its capability says
`nativeQualified: false`. It calls the existing asset loader and
`captureIsolated` collector, including the existing IG media recovery. Collector
pages carry source/profile/generation/attempt bindings; close receipts must
match those bindings. Cleanup of these temporary media targets is separate from
the enclosing collector's process cleanup. A late-created page retains its
cleanup obligation after cancellation without reviving capture or publication.

The Go Engine/Store adapter and JavaScript collector seam have separate
synthetic boundary tests. Their combined coverage is not a native
reader-to-collector end-to-end result.

## Offline acceptance

Regression coverage must demonstrate:

- X keeps its existing routing, permission, claim/commit, failure and cleanup
  behavior while using the shared orchestration.
- A second source reaches Engine, Store and the existing collector seam through
  the same core, including X and Instagram runs in one update session.
- Source/account mismatch, expired evidence/deadline, stale completion, revoked
  permission and cross-source permit/receipt use cannot authorize capture or
  commit.
- Temporary Instagram media targets stay in the collector's ownership and are
  closed independently without retiring the reader or another source's targets.
- Fresh/cold-open schema 31 and synthetic 30 to 31 upgrade preserve guarded
  state; an old schema-30 binary refuses the newer database.
- Configuration defaults preserve current X behavior and ordinary Instagram
  Browser capture; an unavailable native adapter cannot manufacture readiness.

## Implementation validation checkpoint (2026-10-29)

The following offline checks passed using project-local temporary directories
and cached dependencies (`GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`):

- Tagged integration: `go test -tags auth_session_dev ./internal/engine
  ./internal/store ./internal/config ./cmd/akusidecar -run
  'TestSourceDaily|TestInstagram|TestXDaily|TestManualPilot|TestCapture|TestHybrid'
  -count=1`.
- Full tagged Store and configuration suites: `go test -tags auth_session_dev
  ./internal/store ./internal/config`.
- Default-build Engine/HTTP regressions and main compilation: `go test
  ./internal/engine ./internal/httpapi ./cmd/akusidecar -run
  'TestXDaily|TestCapture|TestHybrid|TestInstagram' -count=1`.
- Combined `auth_session_dev auth_session_fixture` Engine/Store/main
  compilation, with `-run '^$'` (compile only).
- JavaScript source-adapter suite: 18 tests, including the existing IG media
  recovery and late target cleanup. Related worker, freshness, X bootstrap and
  package-builder regressions also passed.
- Development-package verifier: 41 tests. Historical schema-30 and new
  schema-31 package metadata remain readable; unsupported versions are rejected.
- Store code compiled from immutable pre-change commit `c336d21` refused a
  synthetic schema-31 database and preserved its schema marker and sentinel.
  This is an old-source Store test, not an executed old full-runtime binary.
  Current Store tests cover cold open, semantic 30-to-31 migration, and source
  grant/permit isolation.

Mixed-source coverage includes X-to-IG and IG-to-X runs, source/profile/account
binding, proof revision and freshness rejection, cancellation, independent
revocation, retained cleanup uncertainty and predecessor cleanup barriers.
Changing the selector back to Browser reconciles closed durable permits; a
still-protected permit rejects startup rather than bypassing the fence.
Ordinary runs without a daily permit remain ordinary.

The main-dev configuration and runtime were not changed. No native IG readiness,
session transfer or native end-to-end result is inferred from this coverage.
The legacy PowerShell Store runner's schema expectations were updated and its
syntax checked; that entire runner was not executed in this increment.

## Post-capture reasoning failures (2026-10-30)

Capture authority and reasoning completion have different lifetimes. A daily
collector may finish, commit its observation, and close its permit before the
model finishes evaluating that observation. The retained Store fence must still
reject new capture claims/results, but must not prevent recording a subsequent
reasoning failure.

`FailCapturedRunReasoning` is a negative-only transition. For protected daily
runs it requires the same run, session, source and permit attempt, a completed
capture command, an accepted observation and its committed result receipt. It
rejects active capture commands, changed stages and existing terminal outcomes.
A closed or expired capture permit does not invalidate that historical receipt.
The generic `FailRun` guard remains unchanged.

An ordinary provider failure terminates that run and allows the next source to
proceed. The existing quota policy additionally stops untouched queued peers;
`FailQueuedAfterReasoningFailure` proves the failed anchor and refuses peers
with capture work, observations or their own authority fence. Neither operation
renews a permit, retries a model request or changes remembered source consent.

Synthetic regressions cover X and Instagram in both orders, terminal failure,
accepted evidence preservation, peer continuation, capture-window intent
release after worker drain, a subsequent successful batch, and quota stopping
before peer capture. These checks do not establish that Instagram's native
identity endpoint accepts the current request. That separate HTTP 400 remains
under investigation through a bounded reader-only diagnostic.
