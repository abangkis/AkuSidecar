# Daily X concurrency in ordinary development

## Current delivery direction: preserve the stable development route (2026-10-28)

Concurrency must be adapted to the existing development lifecycle. The daily
entry points remain `scripts/build-dev.ps1`, `scripts/restart-dev.ps1` and
`runtime/dev/aku-sidecar.exe`. Sealed experiment directories are validation and
historical rollback evidence; switching the service/native-host to a new sealed
directory for each rebuild is superseded as the daily delivery mechanism.

The integration sequence is:

1. **Ordinary toolchain compatibility, offline first.** Build development
   capabilities with `auth_session_dev`, stage the complete `.next` code bundle,
   preserve ordinary registered arguments, and verify configuration, schema and
   hashes before replacement. Config/schema diagnostics must not initialize the
   optional capture composition. Candidate preparation does not overwrite the
   active broker, UI assets, worker or native transports. Promotion rolls back
   the whole code bundle if a move fails. Mode defaults remain exclusive.
2. **Adapt optional ownership and X routing.** Keep the existing scheduler,
   ordinary non-X Browser admission, parser, reasoning, Store and Timeline.
   Change only the X capture adapter. Main-profile transport/persistent ownership
   is an explicit startup composition, not process consent; its compatibility
   with ordinary operations must be proved separately. Do not broaden the
   legacy Facebook hybrid exception or reopen refresh/rotation qualification.
3. **One approved handoff, then ordinary daily restarts.** A service still
   pointing at a sealed experiment must be migrated once through the existing
   approval workflow. Normal rebuilds then retain the stable executable and
   native-host paths. Require graceful release and source-specific acceptance,
   including two X batches/reader overlap, before claiming daily readiness.

Compilation/staging does not grant session derivation. Config selects the
backend, process consent authorizes derivation, and each batch retains its
separate account/ownership/Store authority and 60-second ceiling. There is no
silent fallback from a failed authorized route. Changing ownership composition
still requires restart; hot switching remains deferred.

Returning to exclusive must also clear/reset `xReaderSource: main` according to
the existing config contract. Neither build nor restart rewrites those fields,
source settings, login, database contents or Auto Update.

The first correction is implemented and validated synthetically only. It does
not activate a runtime, inspect the user database or transfer a real session.
See the [ordinary activation procedure](x-daily-development-activation.md).

## Target and selected policy (2026-10-27)

The next approved integration adds the existing main profile as a borrowed reader
through [the main-profile connection](x-main-profile-connection-design.md).
The dedicated-reader implementation and its prepared package remain intact;
they do not establish main-profile support or authorize runtime activation.

The MVP is ordinary, repeated X capture in AkuBrowser main dev, with the native
reader usable during collection and results entering the existing Timeline.
An isolated QA package is supporting evidence, not the delivered user journey.
Only X changes backend. Other sources, existing effective reasoning provider,
settings, database, pacing, challenge pause and Auto Update remain ordinary.

The user approved derivation authorization for the lifetime of the Sidecar
process, with a maximum of 60 seconds for each batch. Restart expires that
authorization; saved configuration is a mode selection, not an account proof,
cookie receipt or replayable Store permit. Each batch must obtain its own
verified account/login/ownership evidence and run-scoped claim/result authority.
The original attempt clock cannot reset inside a batch. A new normal batch may
derive only under a still-valid process authorization after verified cleanup.

## Three delivery increments

1. **Restart-required ordinary composition.** Add `capture.xMode`, either
   `exclusive` (omitted/default) or `concurrent`. Validate supported Windows
   `auth_session_dev` development selection before Store/provider/UI access.
   Use normal config, port, Store, effective settings/provider and UI, without
   requiring a fresh QA plan or database. Attach X routing before resume,
   HTTP serving and scheduling. A mode change takes effect only in a new
   gracefully restarted process. No hot switch is promised by this increment.
2. **Daily native batches.** Lease the explicitly selected dedicated signed-in
   X reader profile with no simultaneous profile owner. Keep the reader alive
   across disposable collectors. Route ordinary manual and scheduled X work
   through the existing parser/worker/Coordinator and per-batch Store permits.
   Preserve command budgets and source settings instead of manufacturing a
   one-command QA session. Wire process authorization, repeated derivation,
   60-second batch expiry, source Stop and verified cleanup. Do not silently
   fall back, retry a failed derivation or reuse a consumed batch permit.
3. **Daily-use acceptance and activation.** First validate offline/synthetic
   boundaries. After separate operational authorization, prove two batches in
   one normal app session, reader interaction during capture, normal Timeline
   results, Stop blocking subsequent X batches, and graceful config rollback
   from concurrent to exclusive with user data/provider/preferences preserved.

Full refresh/rotation qualification and hot switching remain deferred. Account
mismatch, observed logout, challenge, owner loss or uncertain cleanup pauses X
and requires explicit recovery. No automated challenge bypass is introduced.

## Increment 1 boundary

The mode is read once with the ordinary typed config. Unsupported/non-dev and
production selections fail before normal Store startup. Manual/QA selectors
cannot be mixed with daily concurrency. The normal development port and existing
database are permitted; no QA namespace or `OpenDevelopmentQADatabase` is used.

This increment owns no second reader, collector, profile lease, native factory,
cookie derivation or batch permission. The existing ordinary capture process
remains the sole current profile owner for non-X work. Ownership transfer to
the persistent reader belongs to increment 2.

Until that adapter is connected, status reports selected `concurrent`, effective
`unavailable`, state `not-ready`, reason `batch-adapter-required`, and no granted
derivation authority. Legacy X capture claims and dispatch fail closed.
Dedicated reader borrowing also remains gated. In supported concurrent/main
composition, ordinary Open source/native reader actions instead borrow the exact
retained main Browser through the existing Bridge/broker path, independently of
batch consent. They grant no capture authority; missing/stale ownership rejects
the action. See the [ordinary action boundary](x-main-profile-connection-design.md#ordinary-x-window-actions-offline-2026-10-28).
Mixed-source updates continue eligible non-X sources without rewriting saved
source choices or Auto Update. Queued X runs encountered after restart receive
an explicit readiness failure so later non-X work is not stranded. Closing or
stopping the selection never silently changes backends; restart with exclusive
configuration is the rollback path.

Illustrative config change, not an instruction to edit the live config:

```json
{"capture":{"xMode":"concurrent"}}
```

The rest of the existing capture/config fields must be retained. No runtime
restart, registry change, login access, user-database access or session transfer
is authorized by this design, implementation or its offline tests.

## Increment 2: persistent process reader and repeated ordinary batches

The Windows/dev composition now has a separate `aku.x.daily-owner.v1` private
protocol. A process owns one reader; each strictly increasing batch generation
gets a new collector/profile and one consumed derivation grant. Successful
collector cleanup does not close the reader. Per-batch authority remains bounded
by the original seed start plus 60 seconds; neither capture, cleanup nor later
commands can restart that clock. Another batch is a new generation, not a reset
of the existing manual pilot's one-use fields. Protocol replay state is bounded.

Normal manual and scheduled `startSession` calls keep their ordinary intent,
delivery, budget, source settings and downstream Timeline pipeline. The session
freezes X to headless while keeping each non-X driver's ordinary selection.
Only trusted daily startup can issue a run-scoped permit. All legacy X claims
remain fenced; unrelated non-X runs in the same session remain usable. The
legacy manual permit still fences its entire session. Old binaries conservatively
retain that larger fence because the existing metadata prefix is unchanged.
No schema migration or user-database operation occurred in this implementation.

### Process consent and stop

Configuration installs a private factory, not consent. The new same-origin,
epoch-checked `POST /api/development/x-daily/start` accepts exactly:

```json
{"acknowledgeLimitations":true,"allowProcessSessionDerivation":true}
```

That grant permits repeated verified batches only in this running process.
Startup consumes its attempt even on failure. Saved config, old request bodies,
another epoch and process restart cannot restore it. The existing manual start
body cannot grant process consent. Status is available in `xCapture` under
`GET /api/collection/runtime`; before consent it reports
`process-consent-required` and `derivationAuthorized:false`.

`POST /api/development/x-daily/stop` with `{}` revokes derivation, cancels and
joins startup/batch cleanup, and blocks later batches. It preserves the owned
reader until whole-process shutdown. Failure, account mismatch, challenge or
uncertain cleanup also latches admission closed. Process teardown retains a
partially created owner and uncertainty until cleanup is verified. A separate
worker drain covers the final scheduling continuation before database release;
capture completion alone is not idle. Closed first-failure phase and cleanup
state remain separate in the status projection.

### Startup diagnostics and bounded progress (2026-10-28)

`BeginBatch` preserves a private unwrap-able operational error and exposes only
the closed `xCapture.startupDiagnostic` projection: `stage`, `condition`, `cause`,
and optional catalog-only `origin` / `brokerReason`. Substages distinguish
admission, reader check, fresh collector creation/identity/lease, batch-begin
RPC/decode/receipt, reader/collector binding, seed RPC/decode/receipt, broker
creation/start, Managers, runtime and Store admission. Receipt predicates are
reported separately; an unknown exception is never used as a public label.

The worker records its first operational failure before cancellation and cleanup.
A subsequent Stop or cleanup error cannot replace that diagnostic. Cleanup
pending/uncertainty and revoked authority remain separate status fields. The
terminal `x_daily_startup` log uses the same closed projection. Private errors
retain `errors.Is` / `errors.As` support but their arbitrary message, paths,
account values, cookies and raw replies are never formatted into this output.

Main-profile startup progress crosses the borrowed Node/control/Go path only
for correlated startup inspection, inventory and acquisition RPCs. Each pending
RPC admits at most 32 exact catalog-validated progress DTOs, each at most 512
bytes. Unknown/duplicate/extra fields, malformed envelopes, retired IDs and
post-terminal events are dropped. Progress remains diagnostic only: it cannot
grant Store/account/ownership authority, extend a deadline or replace the first
terminal cause. Existing ordinary Browser actions retain their protocol.

The shared telemetry library integration and relocation are deferred until the
user identifies that repository. This increment uses existing local diagnostics
and logging; it does not create a replacement telemetry library. Offline evidence
does not identify the earlier live failure retrospectively or authorize another
session transfer, restart or activation.

### Explicit activation resources and MVP limit

An explicitly selected `capture.xReaderProfile` must be an existing canonical,
dedicated signed-in X profile, distinct from the ordinary capture profile. It is
leased exclusively. No existing reader is adopted, no profile/login is copied,
and reuse of the user's current main login is not claimed. The ordinary profile
continues serving non-X work. Selecting this reader for real use remains an
explicit deployment/resource decision, outside offline implementation authority.

The binary directory must contain pinned Node (`node/` or the existing
`headless-worker/`) and `x-daily-owner/manifest.json` with its sealed
`x-daily-owner.cjs`. `buildNativeDailyOwnerPackage` in
`scripts/build-x-native-owner.mjs` builds that distinct entry without running it.
The normal capture Chrome image is pinned before process start. The daily-only
post preparer supports the normal main port; manual QA origin restrictions are
unchanged. Native-host/helper and physical foreground requirements still apply.

The initial daily MVP preserves the existing native worker's **one acquisition
round per X batch**. Command scroll/item budgets and normal evaluation remain
intact. Trusted Store commit writes a `round_one_v1` acquisition receipt; only
that receipt together with the daily issuance record suppresses optional
follow-up capture. Generic observation coverage cannot select this policy.
Non-X and exclusive-mode acquisition behavior is unchanged. Multi-round native
continuation is deferred rather than queued into an unsupported collector.

### Offline acceptance

Run `scripts/test-x-daily-development.ps1`. It exercises the sealed Node package,
bounded generations/replay, synthetic Windows child ownership, normal Engine
commands and SQLite persistence, repeated batches, Stop/cleanup, startup races,
late owners, durable recovery and the four build modes. Fake child processes
deliberately fail account qualification; their announcements cannot grant real
capture authority. This proves offline composition, not native account capture
or foreground acceptance. The live daily-use acceptance in increment 3 remains
separately authorized; no actual transfer or main-dev activation was run.

Preparation for increment 3 is documented in
[Daily X main-dev activation and rollback](x-daily-development-activation.md).
The distinct daily package retains the original ordinary config root/database
and service arguments, and leaves reader selection and process consent pending.
