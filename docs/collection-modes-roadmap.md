# Browser and headless collection roadmap

Status: architecture approved; phase 1 committed; phase 2a/2b implemented locally,
with native-reader lifetime guard added. Phase 2c handoff integration is pending.
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
| `internal/collection/coordinator.go` (planned) | Driver routing, capability/readiness checks, admission and pinned run ownership |
| `internal/collection/headless/` (planned) | CDP acquisition worker and common observation submission |
| `internal/captureruntime/` (planned) | Managed Chrome lifecycle, profile ownership, generation, interactive leases and recovery |
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
| 2 | In progress: session/follow-up/recapture leases and owner fences wired; 2c pending | Native reader guard implemented; pending interactive actions, sign-in lifetime and transport/containment rebinding remain before replacement is exposed |
| 3 | PoC evidence only | `experiments/x-headless`; resolve product worker dependency and source gaps |
| 4 | Not started | No collectionMode setting or user-visible switch exists yet |
| 5 | Not started | Product parity and packaging require phases 2-4 |

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
- Phase 2 remains incomplete: pending/claimed split actions need admission leases
  that survive client cancellation; sign-in windows need full-lifetime protection;
  replacement must rotate split instance credentials and rebind containment and
  reader callbacks. Recovery after failed startup also needs verified ownership
  cleanup. Do not wire a product replacement trigger until these gates pass.

Complete these remaining ownership boundaries with targeted tests before opening
a mode switch. Do not retrofit a settings-only change around them.

Update this ledger with exact validation and unresolved gaps after each phase.
A phase is complete only when its acceptance gate passes. Changes to scope or
invariants must be recorded here before implementation. This roadmap does not
authorize commit, push, installation, runtime restart or release publication.
