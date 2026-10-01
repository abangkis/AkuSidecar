# Browser and headless collection roadmap

Status: architecture approved; phase 1 committed; runtime/session/media ownership
and reader guard committed as `b376438`; split-action leases committed as
`86cd0c3`. Source/login-window tracking is implemented in the paired Sidecar and
Bridge checkpoint. Local candidate implementations for phases 2c-5 now exist,
but acceptance is incomplete. Do not install or release this candidate yet.
Owner: AkuSidecar integration, with source extraction shared with AkuBridge.

## Product contract

The user selects browser or headless collection in Settings and can switch back.
Browser remains the default, including when older settings omit the new field.
Headless is opt-in and does not replace the current collector.

`collectionMode` will be `browser` or `headless`. Existing `captureVisibility`
remains a browser-only policy, with its value preserved while headless is active.
Quiet is not headless. Settings must show the requested mode, effective mode,
pending transition and actionable failure separately.

Both drivers submit `domain.Observation` through the existing engine acceptance,
quality validation, reconciliation, reasoning and storage pipeline. A driver must
not write Timeline records directly or bypass durable command admission.

## Invariants and scope boundaries

- Exactly one managed capture process may own the authenticated capture profile.
  Use the configured capture Chrome executable in both modes; retain the separate
  UI Chrome for Testing profile. Do not clone profiles or synchronize cookies.
- A run pins its driver and runtime generation. Switching modes never changes a
  run halfway through acquisition or its follow-up round. Fence late callbacks.
- Stop new acquisition admission before a transition; drain active capture and
  interactive ownership before releasing the profile. Coordinate manual updates,
  auto-update, queued work, cancellation, restart recovery and media recapture.
- Replacing capture must not shut down Sidecar or UI. Own only managed processes;
  never terminate arbitrary user Chrome windows or an active native reader.
- No silent visible-browser fallback when headless is selected. A failed
  transition may restore the previous healthy owner, but effective mode and the
  failure must be shown explicitly. Never claim the requested mode is active.
- Login/session expiry and explicit Open native post may need a visible window.
  Use an interactive profile lease and resume the selected mode after release.
  Switching modes alone must not require a new login.
- Source support is explicit and gated by evidence. X and Facebook are initial
  candidates, not blanket production-ready claims. Unsupported sources fail with
  a clear capability status before a partial session is accidentally created.
- Findings from a production extractor run inside the headless PoC do not prove
  a defect in the installed browser collector. Shared extractor changes require
  browser-mode regression evidence; retain headless-specific fixes in that path
  until their common contract is proven.

## Code ownership

| Area | Responsibility |
| --- | --- |
| `internal/collection/request.go` | Acquisition intent and command-preparation contract; no browser execution |
| `internal/collection/bridge/` | Existing Bridge payload preparation; asynchronous transport remains intact |
| `internal/collection/coordinator.go` | Local candidate for driver routing, capability/readiness checks, admission and pinned run ownership |
| `internal/collection/headless/` | Local candidate for the packaged Node/CDP worker and common observation submission; production continuation remains incomplete |
| `internal/captureruntime/` | Managed Chrome lifecycle, profile ownership, generation and leases; local handoff/recovery implementation awaits final acceptance |
| `internal/engine/`, `internal/store/` | Durable commands/runs, result validation, reasoning and common data persistence |
| `internal/httpapi/` and embedded UI | Settings contract, transition status, source capabilities and error presentation |
| AkuBridge source runtime | Reusable extraction logic with separate Bridge/CDP host facilities |
| `experiments/x-headless/` | Fixtures and live comparison harness; not a product runtime dependency |

Do not add an unused driver registry or a Settings toggle ahead of a working
runtime owner. Phase 1 deliberately defines only preparation: the existing
collector is asynchronous, so a synchronous `Collect()` wrapper would hide its
durable ownership boundary. The complete driver execution interface belongs in
phase 2. `internal/capture` already handles reconciliation and remains separate
from Chrome process lifecycle.

## Phases and acceptance gates

### 1. Preserve the existing collection contract

Scope: introduce typed acquisition intent and a Bridge command builder; route
the existing engine payload helper through it. Keep all payload fields and their
JSON meaning, including null continuation and first-round mutation policies.

Gate: first and follow-up wire fixtures pass; existing engine acquisition,
follow-up and observation tests pass; application compiles. No settings/schema,
Chrome lifecycle, extractor, transport protocol or mode-default change.

### 2. Introduce coordinated runtime ownership

Scope: separate capture lifetime from UI/application lifetime; define the
execution driver boundary around persisted commands and accepted observations.
Add admission/drain, pinned driver/generation and ownership checks. Route
readiness through the selected driver's capabilities instead of assuming Bridge
readiness universally. Keep only browser active while proving this boundary.
Headless requires independent UI/capture ownership. For launches without the
existing split-profile architecture, define a supported transition or report the
mode as unavailable; do not close a combined UI/capture process or copy its
signed-in profile as an implicit migration.

Gate: intentional capture replacement preserves UI; a second profile owner is
rejected; queued/follow-up commands cannot be claimed by another driver; stale
results are rejected; active work and readers delay transition; cancellation,
shutdown and restart preserve accepted observations. Define timeout and failed
handoff recovery before adding headless. Any persisted ownership changes require
an explicit backward-compatible schema migration and restart tests.

Implementation checkpoints (all are required to complete phase 2):

- **2a: process-owner foundation.** Adopt the current split capture process in
  `captureruntime.Manager`. Distinguish intentional replacement from unexpected
  exit; wait for owned-tree cleanup, block replacements while leases exist, and
  expose a process-local generation fence. No replacement endpoint is exposed.
- **2b: collection admission and durable fencing.** Attach leases to actual runs,
  including follow-up rounds and media recapture. Pin driver/generation in durable
  command ownership; integrate claim/result rejection, cancellation, restart
  recovery, readiness and automatic updates. A local generation counter alone is
  not sufficient to authenticate or fence a persisted command.
- **2c: interactive ownership and replacement integration.** Hold leases through
  native-reader lifetime, not merely through dispatch of an open action. Rebind
  split transport, containment and reader callbacks to the new process. Define
  bounded transition contexts, failure presentation and verified-owner recovery.
  Do not call Replace from product code before 2b and 2c gates pass.

### 3. Productize the headless driver behind an internal gate

Scope: use the configured capture Chrome through CDP; adapt the common source
core to host facilities for navigation, delay, hover and media resolution. Return
the existing Observation schema through authenticated/fenced ingestion.

Decide and record the CDP implementation first: native Go or a bounded packaged
Node worker. Reusing the PoC's Node worker requires versioned distribution,
provenance and cleanup; never depend on a user's PATH Node. Do not add another
Chromium distribution. The PoC wrapper that stops the entire Sidecar is excluded.

Gate: same-profile authentication, owned-process cleanup and timeout/cancellation
work; headless commands cannot be claimed by Bridge; extractor identity and media
evidence preserve unknown/unresolved states. Use the existing X/Facebook evidence
as the baseline, then close missing cases rather than rerunning all experiments.
Facebook native-ID aliases and unresolved video evidence remain open gates.

### 4. Expose opt-in Settings and interactive handoff

Scope: persist `collectionMode`, migrate old settings to browser, implement
validation and requested/effective/pending/error status across API and UI. Apply
at a documented safe boundary. Preserve captureVisibility selection. Implement
explicit login/Open native post leases and resume the selected mode afterward.

Gate: old settings behave identically; browser -> headless -> browser succeeds
without restarting UI or losing authentication; changing Settings during capture
does not mix drivers; unsupported source/mode combinations are explained; failed
startup reports actual effective mode; auto-update and media recapture respect
ownership. Session-expiry interaction resumes safely without unsolicited visible
collection. Do not offer a selectable mode that cannot complete its handoff.

### 5. Validate opt-in parity and package readiness

Scope: bounded read-only comparison on supported sources and release/runtime
verification. Compare explicit native identity, text, timestamps, media ownership,
pagination/coverage and duplicate behavior. Identical text/author is insufficient
to establish native-ID equivalence.

Gate: validate empty and media-only posts, long text, multiple images, video,
shared posts, follow-up capture, authentication challenge, worker failure and
mode-switch recovery. Distinguish fixture, live-headless and live-browser evidence.
Record remaining gaps and supported capabilities. Confirm background collection
visibility through live Windows observation; do not promise zero blinking merely
because DOM capture passed. Browser remains default after this gate.

## Progress ledger

| Phase | Status | Evidence / next action |
| --- | --- | --- |
| 1 | Complete; committed as `a263bcc` | Typed Request, Bridge Builder, engine integration and two wire fixtures; full engine tests and application build pass |
| 2a-2b | Foundation complete and committed; final integrated validation pending | Process ownership, durable driver/generation fencing, session/media/action leases, cancellation drain and native-reader/source lifetime tracking; commits `b376438`, `86cd0c3`, `d9b735a` and paired Bridge `8541337` |
| 2c | Local candidate; acceptance incomplete | Credential rotation, callback rebinding, interactive borrowing and verified recovery exist. Resolve popup TOCTOU and prove real handoff/recovery before activation |
| 3 | Local candidate; acceptance incomplete | Dependency-free Node/CDP X/Facebook worker and Observation mapping exist. Fix production payload/budget names and follow-up frontier; restage the init fix and repeat ownership smoke |
| 4 | Local candidate; acceptance incomplete | Settings/API/UI persist browser/headless and show requested/effective/pending/failure. Validate the complete switching, login/native reader, auto-update and recapture journey |
| 5 | Packaging helper implemented; product validation incomplete | Official Node archive pin, worker/license staging and builder integration exist; helper fixture tests pass. Full package build, authenticated parity and Windows visibility evidence remain |

The table describes code availability separately from acceptance. Candidate
Settings/driver wiring was implemented before the 2c gate passed; this is an open
roadmap sequencing deviation, not evidence that phases 3-4 are accepted. Keep the
candidate out of installed/released runtimes until the ownership gate is closed.

### Resume order after the local candidate checkpoint

1. Complete production payload/budget handling and follow-up frontier ownership,
   including source interleaving and media-recapture invalidation.
2. Resolve the cross-driver error-contract test mismatch; restage the init fix
   and rerun isolated profile exclusivity/reuse smoke plus focused integration
   checks. Do not treat earlier successful suites as validation of later edits.
3. Finish scoped review and close popup/handoff/recovery acceptance. A live
   successful transition alone does not eliminate the known TOCTOU race.
4. Validate rendered Settings and native-reader/login journeys, then bounded
   authenticated X/Facebook parity and background-window visibility.
5. Build and verify complete local packages. Record supported capabilities and
   remaining source limitations before declaring any phase complete.

Snapshot commits preserve unfinished candidate work; they do not authorize
installation, runtime restart, push or publication and do not imply green tests.

### Phase 1 validation (2026-10-01)

- `go test ./internal/collection/... ./internal/engine -count=1`: passed. Engine
  tests include the new first/follow-up wire fixtures and existing acquisition,
  observation, progressive wait and follow-up behavior. Collection packages
  compile and are exercised through the engine contract tests.
- `go build -buildvcs=false -o build/collection-contract-check.exe ./cmd/akusidecar`:
  passed; generated binary is a local verification artifact, not installed or run.
  The initial normal build exited successfully with a denied external module
  stat-cache write; the build without VCS metadata completed without that warning.
- `gofmt -l` on changed Go files: empty; tracked diff whitespace check: passed.
- Settings, storage schema, Bridge wire payload, extraction, command transport and
  Chrome startup behavior remain unchanged. Existing PoC files were preserved.
- No live capture, installed runtime restart, commit, push or deployment performed.

### Phase 2a implementation boundary (2026-10-01)

- Split startup returns a managed capture owner to main. Intentional replacement
  does not end that manager; unexpected capture exit preserves the existing
  application shutdown policy. Combined UI/capture launches are unchanged.
- Replacement uses the existing `appshell.Window.CloseForRetry` ownership drain,
  not root exit alone. Failed cleanup retains the old owner and blocks admission.
  Failed/cancelled launch blocks admission and further launch attempts because
  ownership may be unverified; there is no silent fallback.
- Manager leases are process-local building blocks. Current source dispatch and
  readers do not yet acquire them, and generation checks are not yet connected to
  persisted commands. Product code therefore does not call Replace yet.
- Tests cover first/second lease drain, idempotent release, intentional replacement,
  late-generation rejection, root exit before owned-tree drain, cleanup/launch
  failure, cancellation before/during cleanup and launch, unexpected exit and
  shutdown. These are controlled-process tests, not live Chrome handoff evidence.
- `go test ./internal/captureruntime ./internal/collection/... ./internal/engine
  ./internal/httpapi ./cmd/akusidecar -count=1`: passed, including all 11 manager
  tests and the existing engine/API/app-entrypoint suites.
- `go build -buildvcs=false -o build/capture-runtime-check.exe ./cmd/akusidecar`:
  passed. The binary was not installed or executed.
- Race-detector execution is unavailable here: CGO is disabled and no GCC is on
  PATH. No compiler installation is needed for this checkpoint.
- First-stage commit `a263bcc` was explicitly authorized after phase 1. Phase 2a
  remains local work; existing PoC files are preserved. No push, installation,
  installed runtime restart or live browser capture has been performed.

### Next implementation checkpoint: 2b and 2c

Baseline inspection confirmed `startNext` admits initial acquisition and
`AcceptObservation` can admit the next source before reasoning finishes. A lease
must therefore cover durable session/run ownership, not only a synchronous
capture callback. Follow-up commands are queued later by reasoning; release on
the first observation would permit a driver change between rounds.

The split action transport tracks action completion and reader foreground
preparation, not the full lifetime of a native reader window. Action completion
is not evidence that the reader released its profile. Add explicit reader
lifetime tracking before connecting any replacement trigger. Existing callback
closures also point to the original containment instance and must be rebound.

### Phase 2b and native-reader guard (2026-10-01)

- Managed split startup attaches capture ownership to the engine before starting
  automatic updates. Existing durable active session/recapture leases are adopted
  before the manager is published to callbacks. Combined UI/capture launches keep
  the legacy unowned payload behavior.
- Session admission acquires ownership before writing a session. It holds through
  queued acquisition, both follow-up rounds, progressive-source scheduling and
  reasoning. Only durable terminal state plus worker drain releases the lease.
  Cancellation alone does not release an active reasoning worker's ownership.
- Managed command payloads persist an additive `captureRuntime` object with
  driver, Sidecar epoch and generation. Claim rejects stale commands before
  dispatch; observation admission checks that persisted owner before saving.
  Stale queued work follows the existing failed-command/fallback path rather than
  replaying an accepted capture. Legacy unstamped commands are admitted only in
  the initial managed generation. No database schema version change is required.
- Media recapture acquires its own lease and atomically persists the owner stamp
  with job creation. Claim, acceptance and failure check ownership; terminal jobs
  release their lease, including passive enrichment that completes an active job.
- Replacement checks a process readiness guard before terminating the old owner.
  Windows containment remembers explicitly bound reader HWNDs independently of
  consumed/expired foreground capability. Native existence, not visibility or
  action completion, controls release. Reused live HWNDs conservatively block;
  tracked live readers are bounded to 32. No native foreground behavior changes.
- Passed full tests for captureruntime, collection, engine, store, HTTP API and
  app entrypoint; passed targeted appshell reader/containment tests and application
  build. Added controlled tests for session cancellation/worker drain, unchanged
  owner across real follow-up workflow, stale-epoch dispatch/result rejection,
  malformed generations, unavailable runtime admission, media ownership/duplicate
  admission and native reader existence. No live Chrome handoff was performed.
- At that checkpoint phase 2 remained incomplete: pending/claimed split actions
  still needed admission leases; sign-in windows need full-lifetime protection;
  replacement must rotate split instance credentials and rebind containment and
  reader callbacks. Recovery after failed startup also needs verified ownership
  cleanup. Do not wire a product replacement trigger until these gates pass.

Complete these remaining ownership boundaries with targeted tests before opening
a mode switch. Do not retrofit a settings-only change around them.

### Split-action lease checkpoint (2026-10-01)

- Previous runtime/session/recapture/reader foundation committed as `b376438`,
  excluding `experiments/`. No push or runtime installation performed.
- Split action admission acquires a manager lease before enqueue. Startup adopts
  existing queued/claimed actions before publishing the runtime to admission.
- An unclaimed cancelled/timed-out request removes its action and releases the
  lease. A claimed request remains detached in its original queue slot until a
  late result arrives or the transport closes. It cannot be claimed again.
- Late results, including failure results, release ownership and remove detached
  actions. Explicit completion state rejects duplicates even after a waiting UI
  consumes the result channel. Unknown claimed outcomes stay unknown and block
  replacement; expiry alone never proves browser work has stopped.
- Detached actions count toward the existing 32-slot limit. Transport closure
  releases pending leases; the native reader-lifetime guard separately protects
  reader windows after action completion. Login-window lifetime and rebinding
  still need implementation. No replacement endpoint or headless setting exists.
- Added controlled API tests for client cancellation before/after claim, request
  timeout, successful/failed late results, duplicate result after consumption,
  existing-action adoption, unavailable runtime admission and transport shutdown.
  Production action timeout remains 115 seconds; tests use a shorter internal
  transport timeout. This is not live authenticated browser handoff evidence.
- Validation: full `go test ./internal/httpapi ./internal/captureruntime
  ./internal/engine ./cmd/akusidecar -count=1` passed, including seven new action
  lease tests; application build and diff whitespace check passed. New split-action
  checkpoint committed as `86cd0c3`; installed runtime has not been restarted.

### Source/login-window lifetime checkpoint (2026-10-01)

- Scope: protect the explicitly opened split source/permission window before
  source navigation, using a paired optional Sidecar/Bridge handshake. Sidecar
  records a uniquely marked native HWND in the capture job without foreground
  writes or reader foreground privileges. Keep the original browser default.
- The next-action response advertises native source tracking only when a native
  callback exists. New Bridge creates a local marker window, requests one-use
  authenticated preparation for its claimed `open_source` action, then navigates
  the same tab to permission/feed. Binding failure closes only that new marker
  window; navigation failure after binding preserves its tracked lifetime.
- Source windows share the bounded native lifetime guard with readers and block
  replacement while their HWND still exists, including when minimized. Tracking
  does not expire when the action acknowledges or its HTTP request finishes.
- Legacy Bridge/Sidecar combinations preserve direct source opening. They do not
  prove login-window tracking and cannot qualify for safe runtime handoff.
  Site-created popup windows and transport credential/callback rebinding still
  require evidence before phase 2 can pass. No mode switch is exposed.
- This paired checkpoint is committed separately in AkuSidecar and AkuBridge.
  No live authenticated capture, installation, runtime restart or headless
  activation is part of this checkpoint.
- Validation: Sidecar API/runtime/engine/entrypoint suites and targeted native
  containment/lifetime tests passed; Sidecar application build passed. Bridge
  syntax checks and 16 targeted tests passed, including negotiated/legacy source
  handshake, ordering, failed-binding cleanup and post-binding navigation failure.
  Additional API cases cover native capability advertisement, marker access,
  claimed/type/instance fencing, consumed/completed intent and failed-binding replay.

### Capability and popup readiness checkpoint (2026-10-01)

- Source/login-window checkpoint committed as Sidecar `d9b735a` and Bridge
  `8541337`. This next checkpoint remains local and uncommitted.
- Bridge declares version 1 of source-window tracking at authenticated split
  bootstrap. Legacy/unknown versions keep browser operation compatible but do
  not pass transport readiness for replacement. An unprepared successful source
  result records an unverified outcome; a reconnect cannot erase that uncertainty.
- Main registers an immutable transport-readiness callback on the manager. Its
  refusal restores the prior state without terminating the owner or increasing
  its generation. No product replacement trigger is exposed.
- Windows replacement readiness enumerates owned top-level browser windows,
  including hidden/minimized windows, without foreground property writes. It
  requires one capture-host marker and rejects additional Chrome windows even
  after their parent closes. Other user Chrome processes are outside ownership;
  known non-browser native helper windows are ignored. Unknown owned classes,
  failed enumeration, missing/ambiguous host markers fail closed.
- This is a conservative preflight, not atomic popup protection. A popup can
  appear after enumeration; quiescence and the final cleanup/transition boundary
  still need integrated evidence. A missing marker may conservatively block a
  healthy runtime. This guard currently describes browser capture, not headless.
- Transport rotation/rebinding, selected-driver readiness and verified recovery
  remain pending. Installed Chrome has not been restarted or tested live.
- Validation: full API/runtime/engine/entrypoint suites, targeted appshell
  containment/lifetime/popup tests and the application build passed. Sixteen
  Bridge tests and its client syntax check passed. New cases cover legacy/unknown
  capability versions, downgrade on reconnect, persistent unverified outcomes,
  verified preparation, closed transport, immutable manager gate and hidden,
  orphaned, unowned, missing-host and ambiguous-host windows. This is fixture and
  controlled-process evidence; it is not a live native popup handoff test.

### Local integration candidate and stop checkpoint (2026-10-01)

- Uncommitted candidate adds requested/effective/pending Settings status,
  browser/headless coordination, generation-pinned execution, interactive
  browser borrowing, split credential rotation/rebinding and verified recovery.
  The engine submits worker observations through its existing acceptance path.
- Dependency-free Node/CDP worker supports bounded X/Facebook extraction and
  canonical Observation IDs with explicit partial quality/video uncertainty.
  AkuBrowser builders now stage a checksum-pinned official Node runtime,
  worker sources and licenses beside Sidecar. Packaging helper fixture tests
  passed; a full package build and installed-runtime validation have not run.
- Coordinator/domain tests and three Settings-state JavaScript tests passed.
  The earlier focused Go suites passed before the latest integration changes.
  Latest runtime and HTTP suites passed, but the new engine cross-driver test
  failed because rejection returns a plain error rather than the expected stale
  owner sentinel. The rejection itself occurred; final suites are not green.
- Real Node/Go-owned Chrome smoke used an isolated test profile, without source
  navigation. It failed at init because the worker omitted bridgePath from its
  validated options. That source fix exists, but staged assets still predate it;
  restage and rerun the profile-exclusivity/reuse smoke before claiming success.
- Important remaining implementation: production payload names/budgets and
  acquisition-round continuation/frontier handling. Current worker still reads
  PoC-style maxScrolls/waitMs and navigates home for every capture. Do not claim
  production follow-up parity until this is fixed and tested, including source
  interleaving and recapture invalidation.
- Remaining acceptance: finish scoped integration review; final Go/JS checks;
  full packaging; rendered Settings/native-reader journey; authenticated X/FB
  parity; live profile handoff/recovery and native popup protection. Window
  enumeration is still a conservative preflight with a known TOCTOU gap, not
  proof of atomic popup safety. The product handoff acceptance gate is open.
- No installation, installed-runtime restart, authenticated live collection,
  new commit or push occurred. Existing experiments remain excluded. Work was
  stopped because the active 1,000,000-token goal budget was exhausted; agents
  were interrupted. Preserve all local edits when resuming.

Update this ledger with exact validation and unresolved gaps after each phase.
A phase is complete only when its acceptance gate passes. Changes to scope or
invariants must be recorded here before implementation. This roadmap does not
authorize commit, push, installation, runtime restart or release publication.
