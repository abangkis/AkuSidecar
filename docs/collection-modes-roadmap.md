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

Approved amendment (2026-10-02): Quiet browser collection may use hidden Chrome
targets created as machine collectors from the beginning. Explicit interactive
source/login/reader windows and Adaptive/foreground behavior remain separate and
preserved. Update Quiet's single/multiple-window promises with the implementation;
do not silently reinterpret existing ordinary tabs as disposable hidden targets.
This amendment does not establish source parity or authorize installation/release.

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
- Hidden collectors must use the managed profile's default browser context and
  an immutable allowlist of created machine targets. Never adopt an interactive
  target into that list. Retire collector targets/sessions before the static host;
  preserve legacy ordinary tabs whose interactive ownership cannot be fenced.
- A headed capture's private CDP transport belongs to the persistent process
  owner, not to an extraction worker. Worker timeout/cancellation must not close
  the root pipe, invoke Browser.close or kill the Job while interactive windows
  survive. Pipe disconnection can close Chrome; retain it through natural exit.

## Code ownership

| Area | Responsibility |
| --- | --- |
| `internal/collection/request.go` | Acquisition intent and command-preparation contract; no browser execution |
| `internal/collection/bridge/` | Existing Bridge payload preparation; asynchronous transport remains intact |
| `internal/collection/coordinator.go` | Local candidate for driver routing, capability/readiness checks, admission and pinned run ownership |
| `internal/collection/headless/` | Packaged Node/CDP worker and common observation submission; production continuation fixtures pass, authenticated parity remains open |
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
- **2d: approved Quiet machine surfaces.** First prove hidden-target support and
  private owner-held transport in the pinned Chrome, with disposable static
  fixtures. Then separate collector backend from browser/headless presentation,
  preserving durable admission, generation fencing and source capabilities.
  Require hidden default-context/cookie continuity, no tab/window promotion,
  independent interactive-window survival, worker-failure containment, natural
  owner/Job drain and automatic same-profile return. Update Quiet UI wording and
  browser regressions together. Experimental protocol support is a capability
  gate, not an assumed guarantee or an excuse to drop Adaptive/source parity.

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
| 2c | Scoped retirement implemented; integrated acceptance incomplete | Natural-exit ownership and automatic retry pass isolated Windows popup smoke. Finish authenticated Bridge retirement and integrated handoff/recovery |
| 3 | Local candidate; focused contract/ownership checks pass, acceptance incomplete | Production Bridge fields and per-source continuation/frontier handling implemented. Worker fixtures 6/6 and isolated real Node/CDP/Job/profile exclusivity/reuse smoke pass; authenticated parity, freshness qualification and source gaps remain |
| 4 | Rendered Settings fixture passes; integrated acceptance incomplete | Real UI/API/coordinator with fake processes proves switching, reload persistence, visibility restoration and unsupported-source rejection. Native reader/login, auto-update and recapture journey remain |
| 5 | Complete local tuple build and verifier pass; product validation incomplete | Official Node pin, worker/license staging, 411-file payload hashes and candidate probe pass. Packaged Chrome/worker popup smoke passes; authenticated parity, end-to-end Bridge and Windows visibility remain |

The table describes code availability separately from acceptance. Candidate
Settings/driver wiring was implemented before the 2c gate passed; this is an open
roadmap sequencing deviation, not evidence that phases 3-4 are accepted. Keep the
candidate out of installed/released runtimes until the ownership gate is closed.

### Resume order after the local candidate checkpoint

1. Build and verify complete local packages with the committed Bridge scoped
   retirement. Retain the passing payload/frontier, permissions, rendered
   Settings and isolated Windows ownership evidence.
2. Close authenticated end-to-end Bridge handoff/recovery under the approved
   live-switch and auto-return contract.
3. Validate native-reader/login journeys, then
   bounded authenticated X/Facebook parity and background-window visibility.
4. Record supported capabilities and
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

### Post-checkpoint continuation and required product decision (2026-10-01)

- Checkpoint commits: Sidecar `8967dff` preserves the unfinished integration and
  updated roadmap; AkuBrowser `479d11b` adds pinned worker packaging. Neither was
  pushed or installed. The following continuation remains local until committed.
- Worker now accepts production Bridge payload/budget fields; uses per-source
  CDP tabs; preserves source frontier across interleaved captures; verifies
  round-2 URL, scroll and anchors; invalidates continuation on fresh navigation
  or recapture; rejects unverifiable continuation explicitly. Freshness policy
  remains `not_verified` in coverage, not a claim of live browser parity.
- Worker fixtures pass 6/6 and all worker module syntax checks pass. Staging was
  refreshed. Real Node/CDP/Windows Job smoke passes with an isolated profile:
  concurrent owner rejection, verified tree cleanup and profile reuse. WMI
  enumeration was denied in sandbox; the smoke passed outside sandbox. It did
  not use the installed profile, navigate to social feeds or open visible Chrome.
- Cross-driver claim/recapture errors now wrap the common stale-owner sentinel.
  Fresh browser heartbeat refreshes retained headless source authorization and
  clears revoked/incompatible access; a new regression test passes. Login and
  challenge failures preserve their codes and are not automatic-retry promises.
- Final focused suites passed for runtime, collection, domain, engine, HTTP and
  app entrypoint after these changes. Three collection Settings-state JavaScript
  tests, app syntax and diff whitespace checks pass. Earlier application build
  passed; no full package or rendered/live UI/source validation is claimed.
- A scoped lifecycle review confirms the unresolved safety race. Replace checks
  readiness before CloseForRetry; CloseForRetry sends WM_CLOSE to all owned
  top-level windows and can kill the Job after eight seconds. New native popups
  are not fenced by Go admission mutexes. Recover also needs the same live-headed
  safety policy; extra snapshots or a successful smoke do not prove atomicity.
- Product decision required: either preserve live switching/automatic return to
  headless and extend the headed browser's cooperation/quiescence design, or
  approve a reduced contract: headless at cold startup, browser sticky after
  interactive promotion until the user closes the application. The latter
  changes the approved resume-after-interaction behavior and requires a startup
  and source-consent design; it is not implemented or silently selected here.
- Keep the candidate out of installed/released runtimes. Rendered journeys,
  authenticated X/Facebook parity, complete packaging and visibility evidence
  still follow closure of the handoff gate. No additional profile/runtime action
  is authorized merely by this documentation checkpoint.

### Approved live-switch contract and scoped retirement (2026-10-01)

- The user selected immediate browser/headless switching and automatic return
  to headless after interaction. Cold-start-only or sticky browser mode is not
  the approved contract. Continuation changes above were committed as `284bfcc`.
- CDP renderer freezing and new-target pause do not establish a native popup
  creation barrier. Destructive WM_CLOSE-all or Job termination therefore cannot
  implement a safe headed handoff. A fail-closed guard now protects that boundary.
- The implementation direction is scoped retirement: an authenticated Bridge
  command removes only verified background capture tabs and its exact host tab;
  source, login and reader windows are preserved. Sidecar waits for natural
  Chrome exit and verified zero owned processes before launching the next mode.
  A popup race or timeout retains the old owner and a pending mode request;
  it never authorizes a forced close or profile reuse. Explicit application quit
  remains separate from collection-mode handoff.
- This protocol and lifecycle change are under implementation. Its acceptance
  requires non-destructive timeout/retry tests, preservation of independent
  interactive windows, and integrated Windows handoff evidence. Settings,
  authenticated X/Facebook parity, full packaging and visibility gates remain
  open until their respective evidence is recorded.
- The pinned Windows c2patool binary exists at Sidecar's project-local
  `runtime/dev/c2patool.exe` with the manifest SHA-256. Full packaging can use
  that explicit input without provisioning a binary outside the workspace.
- Go lifecycle implementation now binds scoped retirement to initial and
  replacement capture windows. Recovery applies readiness before cleanup;
  retirement timeouts retain the owner and retry automatically. Natural root
  exit retains the Windows Job handle until zero active processes is verified;
  it does not kill surviving children or close a kill-on-close handle early.
- The internal HTTP action is negotiated explicitly, admitted only while the
  manager is replacing a browser owner with no active leases, and omitted from
  public UI action admission. Claimed requests survive cancellation without
  replay; late ACK and completed rejection behavior are covered by tests.
- Focused appshell/runtime/collection/domain/engine/HTTP/entrypoint suites pass.
  The full package attempt reached launcher verification and exposed strict
  decoding of the new `headlessWorkerPath` field. Launcher now recognizes the
  optional adjacent worker directory and requires declared worker payloads;
  its full Go suite passes. The package builder still needs a final rerun.
- Actual independent-popup Windows smoke has not run. Bridge tab-retirement
  integration is the current remaining implementation delta, followed by full
  packaging, rendered journeys and authenticated source parity. This checkpoint
  does not establish zero blinking, popup safety in live Chrome, or release
  readiness. Installed runtime/profile remain untouched.

### Isolated popup and rendered Settings checkpoint (2026-10-02)

- Sidecar `3511147`, AkuBrowser `036d839` and Bridge `b7d7340` preserve scoped
  Go retirement, launcher worker-manifest support and capability negotiation.
  The following tests add acceptance evidence without changing production code.
- Opt-in Windows smoke passes (12.88 seconds) with project-local pinned Node,
  configured Chrome and a disposable profile. An independent minimized popup
  is created after Manager readiness, inside the host-close callback. Exact host
  removal times out without killing the popup, root or Job. Coordinator retry
  subsequently starts real headless generation 2 on the same profile after the
  popup is closed separately and the old owner exits naturally.
- This smoke substitutes an exact CDP host-target close for the authenticated
  Bridge/server callback. It proves the native ownership boundary for this popup
  scenario; it does not prove all native dialogs or end-to-end Bridge handoff.
  The sandbox denied loopback CDP; the unchanged smoke passed outside sandbox.
- The real rendered Settings UI/API/coordinator fixture passes (100.63 seconds).
  The browser/headless/browser/headless journey, reload persistence, restored
  browser visibility controls and unsupported LinkedIn rejection were exercised.
  Final assertions verify headless, retained quiet visibility, X/Facebook source
  settings and at least four generations. Capture processes and source consent
  are simulated; native reader, authentication and live parity are not covered.
- Evidence screenshots are local artifacts under
  `build/collection-ui-fixture-20261002/`; the final assertion run uses
  `build/collection-ui-final-20261002/`. Opt-in fixtures skip in ordinary suites.
- Bridge implementation is committed as `19719a5`. Both bootstrap and action
  envelope must advertise the internal close capability. Host identity is checked
  before managed retirement and again after result ACK; close polling waits for
  completion to prevent a terminal next-action response preempting host removal.
  Only exact tracked feed/placeholder tabs in minimized unfocused windows are
  retired. Transient, reader, source and navigated tabs are preserved. Failed ACK
  or host removal does not establish profile release. Focused tests pass 74/74;
  service-worker/client/runtime syntax checks and diff whitespace checks pass.
- Chrome has no atomic conditional tab removal. Navigation or user promotion
  between the final re-read and `tabs.remove` remains a residual race; repeated
  checks are not an atomic safety guarantee. End-to-end acceptance stays open.
- Remaining gates: authenticated end-to-end Bridge retirement, full package rerun and
  verifier, integrated native reader/login and recovery, authenticated X/Facebook
  parity including native IDs/media/coverage, and Windows visibility evidence.
  No installed runtime/profile, push or release was changed.

### Complete local package checkpoint (2026-10-02)

- Full installed-app tuple builder and independent builder verifier both report
  `status: ok`. Candidate directory:
  `AkuBrowser/build/headless-handoff-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Its exact source tuple is Browser `036d839`, Sidecar `a3a148a`, Bridge `19719a5`.
  Sidecar is recorded dirty because untracked experiments remain preserved;
  `-AllowDirty` was used only for this local candidate.
- The payload contains 411 files / 588,711,115 bytes. Verifier checks the complete
  declared payload hashes, adjacent pinned Node/worker and both licenses, Chrome
  executable pin, Bridge identity, schema 29 provenance and Sidecar candidate
  probe. Launcher verify-only also succeeds; it does not launch the application.
- SHA-256: launcher
  `3e1223555c51273206c0b9fb77d414d5e28f3dd26e920d46a67b5a47401aef0e`;
  Sidecar
  `34329997ff32cd3aef46c4472f2fe9f54eb4e160e84fcd16ec4b9d21c5ebcd05`.
- Isolated Windows popup smoke passes again (13.71 seconds) using this package's
  Chrome, Node, worker and Bridge paths. The changed binary inputs justify the
  rerun. It retains the existing smoke's CDP callback substitution and does not
  close the authenticated end-to-end Bridge gate.
- Package metadata inherits the existing 0.9.0 release tuple labels. Successful
  packaging does not establish headless feature release readiness. No installer,
  installed-runtime launch/restart, profile access, push or publication occurred.

### End-to-end preparation and runtime boundary (2026-10-02)

- Read-only preflight identifies the active listener as the registered dev runtime
  (`runtime/dev/aku-sidecar.exe`, PID 2704), not an installed candidate. Supervisor
  reports healthy/running, no operator hold; Inbox has no active sessions. The
  registered capture profile has exactly one Google Chrome owner (PID 8372), and
  Bridge is healthy/compatible. These are observations, not a stop authorization.
- An alternative listener on `127.0.0.2:11122` bound successfully. Scoped Chrome
  resolver mapping to preserve the localhost origin did not produce a matching
  fixture request within the bounded probe: the expected target URL was present
  with an empty title and the isolated listener observed zero matching requests.
  The cause remains unverified. No DNS, extension allowlist or running service was
  changed; failed probe code is retained only as an ignored build artifact.
- `scripts/test-bridge-headless-handoff.mjs` prepares a controlled operator-only
  test. Default invocation performs read-only preflight. That mode and its syntax
  check pass. The explicit `--allow-runtime-stop` branch requires authorization:
  it verifies the candidate, rechecks idle Inbox and original ownership, stops the
  registered service through Supervisor, runs a disposable-profile smoke on the
  original port, and restores/verifies the original service in `finally`.
- End-to-end fixture execution requires a user decision because it temporarily
  closes the active product UI/capture runtime. It does not install the candidate,
  alter registration/settings, copy cookies or select the user's profile for the
  static Bridge smoke. Authenticated source parity remains a separate evidence
  gate after the static protocol/lifecycle smoke.

### Authorized static Bridge smoke and startup diagnosis (2026-10-02)

- The user approved controlled stop/test/restore after the fixture was ready.
  The wrapper and Windows-only `TestBridgeHeadlessHandoffWindowsSmoke` fixture
  were reviewed; default-skip compilation passes with project-local caches and
  temporary directories. The fixture binds the original loopback port before
  creating Chrome, uses an isolated store/profile and trusts the exact packaged
  extension identity. It would verify the real close ACK before transport rotation
  and direct Manager replacement, not an automatic Coordinator trigger.
- First opt-in run failed after 60.17 seconds waiting for authenticated capability
  bootstrap. It did not reach host retirement or profile handoff. Receipt:
  `build/bridge-handoff-dec61495-5ee7-4319-9549-b57513660e94/receipt.json`.
- A single diagnostic repeat added a direct HTTP host-page preflight, request
  counters and safe CDP booleans. It failed after 48.71 seconds with
  `host_http_hits=0`, `host_target=true`, `host_title=false`,
  `host_fragment=true`, `bridge_worker=true`, `bootstrap_requests=0`,
  `bootstrap_status=0`, `bootstrap_accepted=false`, `cdp=ok`. The direct HTTP
  preflight returns the expected page; its hit is excluded from Chrome counters.
  Receipt: `build/bridge-handoff-853137c6-3cdd-43dd-9d84-71106585a3e6/receipt.json`.
- Both runs restored the original registered service and verified healthy runtime,
  compatible Bridge and exactly one original-profile Chrome owner. A separate
  read-only preflight after the first restoration also confirmed zero active
  Inbox sessions. No candidate installation, registration change or authenticated
  source capture was performed.
- The failure is narrowed to initial host navigation: the packaged Bridge worker
  exists, but Chrome never requests the host document, so the capability handshake
  cannot begin. The navigation cause remains unverified; neither the host-close
  protocol nor end-to-end handoff is accepted. Do not repeat without a causal
  startup correction or new diagnostic evidence. The earlier popup smoke covers
  native lifecycle only and cannot fill this gap.

### Renderer startup diagnostic checkpoint (2026-10-02)

- A bounded read-only CDP diagnostic attaches to the disposable capture-host
  target and reports sanitized document, frame and extension state. It neither
  navigates/reloads the target nor changes production launch flags. Default-skip
  compilation, JavaScript syntax and diff whitespace checks pass.
- One authorized stop/test/restore run fails at capability bootstrap after
  48.80 seconds. Receipt:
  `build/bridge-handoff-ea285e40-1901-4999-9549-1361c656231c/receipt.json`.
  The original runtime is restored and its health, compatible Bridge and single
  original-profile Chrome owner are verified by the wrapper.
- Diagnostic: host HTTP hits 0; target found and attached; renderer document
  state `complete`, expected frame/title/host marker false, error class
  `unexpected_document`, unreachable false. Packaged extension worker exists,
  but its isolated world/content-script marker is absent from this document.
  Bootstrap requests remain 0. Target URL presence therefore does not establish
  navigation to the expected document. No handoff acceptance is claimed.
- Network counters are explicitly late observations: zero events after diagnostic
  attachment cannot prove that no earlier request or network failure occurred.
- An upstream CEF Windows minimized-startup issue identifies native occlusion as
  a possible hypothesis, not a verified cause in this Chrome build:
  https://github.com/chromiumembedded/cef/issues/3638 . A separate disposable
  loopback navigation comparison completed in 13.04 seconds: baseline and
  `--disable-features=CalculateNativeWinOcclusion` both observed 0 page hits and
  `loaded=false`. The probe completed, but the occlusion flag did not resolve
  this failure. It did not stop the original runtime or change its profile/settings.
- The user subsequently approved hidden Quiet collectors while retaining
  interactive and Adaptive windows. Pinned-Chrome hidden-target and private-pipe
  capability probes are the next bounded steps; no production backend change has
  been accepted or shipped on the basis of upstream source alone.

### Pinned-Chrome hidden-target capability checkpoint (2026-10-02)

- The isolated random-port diagnostic completes in 1.64 seconds using the
  existing full package tuple and reports actual `Chrome/152.0.7977.54`.
  No original runtime stop, authenticated profile or production configuration
  change is involved. The fixture's PASS means its diagnostic completed.
- Hidden targets created in a dedicated child browser session are supported;
  the default browser context is retained. The hidden fixture receives one HTTP
  request and reaches the expected complete document. Its native-window lookup
  returns a protocol error, whose exact reason was not classified in this run.
- Explicit Page.navigate of the known disposable host receives one HTTP request
  and reaches the complete host document. Natural startup had zero requests,
  an expected target URL but empty frame/history URLs. Initial document evaluation
  failed, so its exact initial document class remains unverified. This establishes
  an effective explicit-navigation path, not the underlying Chromium cause or
  authenticated Bridge bootstrap.
- A harmless fixture cookie seeded by the host is visible in the hidden target:
  default-context sharing passes without copying cookies. This is not live
  X/Facebook authentication evidence. Exact hidden disposal and child-session
  detachment pass while the root test-only WebSocket remains connected.
- After the run, the helper makes opaque-document cookie reads exception-safe,
  classifies window errors without raw text, and labels extension presence as
  `anyExtensionWorker` rather than claiming exact Bridge identity. Syntax passes;
  these diagnostic refinements have not received another native run.
- Go-owned private-pipe lifetime, worker-failure containment, interactive-window
  preservation and production collection integration remain separate gates.
  The loopback debugging endpoint exists only in this disposable probe.

### Go-owned Windows pipe capability checkpoint (2026-10-02)

- The first disposable private-pipe probe establishes Browser.getVersion as
  `Chrome/152.0.7977.54`, but fails after 0.32 seconds because an exact
  `about:blank` app target is unavailable. It closes no ambiguous target and
  cleans up only its synthetic owned Job. Natural shutdown remains unproven
  by that first run.
- The corrected fixture uses its own random-port `/pipe-host` URL and a bounded
  two-second wait for one exact page target. The second native run passes in
  0.38 seconds: pipe roundtrip, exact target closure, clean natural root exit and
  whole-Job zero are verified while both Go parent pipe ends remain open.
- The Windows transport uses inherited child-only anonymous pipe handles and
  `--remote-debugging-io-pipes`; it exposes no HTTP debugging port. No signed-in
  profile, original runtime interruption or global configuration change occurs.
- This is a transport/lifecycle capability proof. It does not yet prove the
  production Window API, hidden-collector integration, independent interactive
  window retention, worker-failure recovery or authenticated Bridge handoff.

### Managed private transport and static Bridge handoff checkpoint (2026-10-02)

- The opt-in managed Window foundation now exposes `LaunchOptions.PrivateCDP`
  and `Window.CaptureProtocol().Call`. It requires minimized capture ownership,
  rejects caller debugging switches, and uses Windows inherited anonymous pipes
  without an HTTP debug port. Ordinary launches retain their previous behavior.
  Product main/Quiet routing does not enable this option yet.
- Transport calls have bounded frames/requests/pending work. Cancellation leaves
  the owner-held connection open; late responses cannot resolve a newer request.
  The extraction backend must review these bounds before large script/results
  are passed through this bootstrap-oriented foundation.
- Focused transport/options tests and Linux cross-compilation pass. The managed
  Window native smoke passes in 0.45 seconds with Chrome/152.0.7977.54: exact
  target closure, natural root exit and empty Job are verified while parent pipe
  endpoints remain held until drain. Exceptional startup cleanup remains
  fail-closed: package-owned quarantine retains pipe and Job references until
  application exit when startup cleanup cannot be verified. A retention unit test
  passes; these error paths have not received native fault injection.
- The real Bridge fixture now explicitly navigates only the uniquely identified
  static host after its bindings exist. A first corrected run passes bootstrap
  and host-close ACK but fails its final assertion after 68.07 seconds:
  `Manager.watch` had consumed the single Window.Done result. This was a fixture
  assertion defect, not evidence that ownership remained live. Receipt:
  `build/bridge-handoff-c41f9ec0-6cd4-4f0d-a7e0-f21d56105e95/receipt.json`.
- After changing the fixture to repeatable CloseForRetry ownership readback,
  the authorized stop/test/restore smoke passes in 9.38 seconds. Authenticated
  Bridge bootstrap, real close ACK, natural owner drain and headless generation
  2 Ready on the same disposable profile are verified. Receipt:
  `build/bridge-handoff-120669ea-5960-4dce-8724-af7265790fc2/receipt.json`.
  Both runs restore the original healthy runtime, compatible Bridge and exactly
  one original-profile Chrome owner. No installation or registration change.
- This closes the static Bridge bootstrap/direct-Manager handoff smoke gap.
  It does not close phase 2d: hidden collector routing, independent interactive
  survival, extraction-worker failure containment, automatic Coordinator return,
  Quiet UI/source parity and authenticated X/Facebook comparisons remain open.
  The existing full package tuple supplies Chrome/Bridge/Node to these source
  fixtures; it does not contain the new managed transport binary yet.

### Quiet hidden backend implementation checkpoint (2026-10-02)

- Local code now binds a generation-specific browser Quiet backend to the Go-owned
  private protocol. X/Facebook in both Quiet policies use hidden targets created
  from birth; Adaptive and other sources retain Bridge. Browser ownership and
  captureRuntime stamps remain unchanged; captureCollector is persisted separately.
- Initial and replacement headed hosts use exact-target explicit navigation after
  bindings exist. Hidden source targets have independent flattened sessions and
  fixed 1280x900 page metrics in the default authenticated context. The broker
  rejects worker browser/target-management commands and tracks only created IDs.
- A borrowed worker owns Node only and forwards source-scoped CDP requests to Go.
  Its RPC replies bypass the capture command queue. Worker timeout/protocol failure
  stops Node and retires only hidden targets; root pipe and capture Job stay owned.
  Transport bounds are now 16 MiB response frames and 8 MiB requests, covering
  bounded extractor injection/results without making the stream unbounded.
- Durable routes fence claims before mutation, filter each consumers pending work,
  pin followups/media, preserve legacy Bridge commands, and fail closed on unknown
  or corrupt stamps. Quiet source authorization is checked again before capture.
  Per-route split dispatch acknowledges the internal pump while other Bridge
  actions and browser source coverage remain intact.
- Required host-only retirement is newly negotiated by Bridge. Go retires hidden
  targets before requesting hostOnly:true; the Bridge client skips ordinary-tab
  cleanup and revalidates/closes only the authenticated static host. Older Bridge
  capabilities block handoff explicitly. Legacy unset retirement stays unchanged.
  Untracked/legacy ordinary windows can therefore keep profile release pending.
- Full store/engine/httpapi suites passed at the routing checkpoint (8.433s/3.841s/
  2.599s); later host-only and corrupt-route focused checks pass. Collection,
  appshell and main checks pass; Node worker/collection-mode tests pass 12/12.
  Bridge split-client negotiation/legacy tests pass 23/23. UI Quiet labels now
  describe hidden X/Facebook only when that backend is configured; rendered
  validation for these new labels remains open.
- The isolated native worker-failure smoke passes in 8.86s with pinned Chrome152:
  two hidden targets, shared fixture cookie, independent minimized interactive
  window survival, retained root pipe, exact hidden cleanup and natural owner
  drain after the last ordinary window closes. Actual pinned Node RPC extraction
  with fake CDP passes in 8.523s, preserving IDs, partial quality and unresolved
  media. Neither fixture establishes authenticated source parity.
- New full package validation and automatic native borrow/return smoke are next.
  End-to-end login/reader-close auto-return, source parity, focus measurements and
  release/install acceptance remain open. No installed runtime/settings changed.

### Packaged Quiet and native handoff checkpoint (2026-10-02)

- Implementation commits: Sidecar `f8b3f2a` and `266f33c`, Bridge `e8136f7`.
  Exact owned-target disposal now polls for up to three seconds before child
  session detachment. Delayed disappearance and cancellation/retry tests pass
  (`0.104s`). No ordinary target is adopted during polling.
- Full local candidate:
  `AkuBrowser/build/quiet-hidden-verified-20261002/AkuBrowser-0.9.0-windows-x64-installed-app`.
  Builder and tuple verifier pass: 412 files / 588,821,744 bytes, Chrome
  152.0.7977.54. Sidecar binary SHA-256:
  `cc4ac7513c0004c1f83b7dde199c1b4ddebcf039c5c741aa29c80466dea608ea`.
  Source identities: Sidecar `266f33c`, Bridge `e8136f7`, Browser `036d839`.
  Untracked experiments were excluded from commits. Later test/documentation
  changes do not alter this candidate payload.
- Rendered isolated Settings fixture passes in `82.23s`: hidden Quiet wording,
  browser -> headless -> browser -> headless, browser-only visibility control,
  and headless selection retained after reload. Fake process/collector owners
  do not establish native handoff or source parity.
- Real static Bridge smoke now proves the initial browser -> headless handoff:
  negotiated host-only retirement, successful close ACK, hidden target disposal
  and natural owned-tree/profile drain. The former one-shot target listing raced
  Chrome's asynchronous destruction; bounded polling corrected that cause.
- The extended generation-3 interactive-window/auto-return fixture still fails
  its safety assertion. CDP window lookup reports another `about:blank` page in
  the fixture window, both after CDP target creation and Bridge windows API
  creation. Its provenance and native association remain unverified. The fixture
  refuses WM_CLOSE on a window containing an unknown page; production retention
  rules are unchanged. Closing only the fixture tab did not clear the tracked
  HWND. This does not establish completed auto-return acceptance.
- Latest diagnostic:
  `AkuSidecar/build/bridge-handoff-8ca282de-7993-4dd9-adf8-d195db2ffdaa`.
  Smoke fails in `5.54s` after the first handoff proof, with original Supervisor
  runtime restored and Bridge compatible. Every controlled run restored the
  original runtime. No candidate installation or settings replacement occurred.
- Next: identify that blank target without adopting or closing an ordinary tab;
  prove an isolated native reader/source window lifetime and automatic return
  after closure. Then finish authenticated X/Facebook parity, integrated login/
  native reader journeys, visibility and release gates. Authenticated profile
  collection remains separate from approved disposable static stop/test/restore.
  The new one-million-token segment is nearing its ceiling; continued diagnosis
  requires a budget/scope decision.

### Bounded blank-target provenance diagnostic (2026-10-02)

- A fixture-only protocol recorder now records successful hidden-target creation
  responses directly; it never adopts IDs from target inventories.
- The additional `about:blank` page does not match any recorded machine target
  in the borrowed browser generation. Receipt:
  `AkuSidecar/build/bridge-handoff-89fdcdce-8e71-430c-9aaa-371d9fc7739d`.
  Diagnostic fails its safety assertion in `5.53s`, after initial real Bridge
  handoff succeeds; original runtime restoration is verified. This rules out
  treating the unknown page as a Quiet collector or excluding it from safety
  checks on that basis. Its origin remains open.
- Next investigation should distinguish pages already present after browser
  re-entry from pages created with the interactive window. Do not close/adopt
  the unknown target or weaken production native lifetime checks. No production
  code or candidate payload changed in this diagnostic continuation.
- A subsequent before/after inventory proves the unknown blank target was not
  present before interactive-window creation. Receipt:
  `AkuSidecar/build/bridge-handoff-1f2afc62-fbc1-4b2e-a763-68c5fbc0c7ae`;
  diagnostic fails safely in `5.49s`, original runtime restored. This narrows the
  next investigation to window creation/CDP target representation. Compare the
  exact newly created Chrome window's actual tabs against its CDP page targets
  before interpreting either inventory as permission to close a native window.

Update this ledger with exact validation and unresolved gaps after each phase.
A phase is complete only when its acceptance gate passes. Changes to scope or
invariants must be recorded here before implementation. This roadmap does not
authorize commit, push, installation, runtime restart or release publication.
