# Daily X concurrency in ordinary development

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
derivation authority. Legacy X claims, dispatch and reader borrowing fail closed.
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
