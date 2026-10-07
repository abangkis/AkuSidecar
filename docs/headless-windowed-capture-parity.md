# Headless and windowed capture comparison

Source audit: 5 October 2026. Windowed refers to AkuBridge's visible-browser capture pipeline; native reader display alone is not a capture pipeline. Quiet/hidden browser is a separate backend. This comparison focuses on the X text incident and the shared capture boundaries, not a live parity certification for every source.

## Shared contracts and separate controllers

Headless injects AkuBridge's source-adapter runtime and adapters. X uses the same `x-dom-v22` selector, text/quote parser, expansion policy (12 attempts at 40 ms), and expected-media rules. Headless does not execute the complete `AkuBridge/content-script.js` controller. It supplies separate discovery, admission, hydration, navigation, recovery and serialization in its worker.

| Capability | Windowed AkuBridge | Headless worker | Difference / reuse boundary |
| --- | --- | --- | --- |
| Text extraction and expansion | `content-script.js` calls `expandSourceContent` before extracting each block and records expansion state; restorable adapters restore their controls. | `vendor/x-extract.js` uses the X adapter policy, excludes quoted controls and expands only on the matching native detail route. Feed text that remains collapsed is marked partial. | Adapter selectors/policy are shared; click/wait orchestration is duplicated. Windowed inline expansion is not equivalent to headless detail navigation. |
| Full X text recovery | The examined Bridge path expands the visible block. Its `recoverPermalinks` hook recovers identity URLs, not full text through a second page. | `x-text-recovery.mjs` navigates an owned temporary page, checks exact identity and accepts only longer, resolved text. | This is an additional headless capability with its own latency budget and failure modes. |
| Primary and quoted identity | Shared X adapter extracts timestamp permalink, quoted fields and reply semantics. | Adds exactly-one-own-permalink admission and `XHeadlessQuoteIdentity` evidence/probing. | Reuse adapter parsers; do not remove headless identity safeguards in pursuit of parity. |
| Media | Adapter media rules plus Bridge's media/quality recovery and persistent evidence integrations. | Shared selectors/expected kinds, worker DOM observations and separate structured-media resolution. | Shared selectors do not establish identical media recovery or admission. |
| Quality and retry | `content-script.js` evaluates per-block quality and retries within `qualityRetryBudget` and operation deadline. | Observation keeps explicit limitations; X text recovery has its own bounded retry and outcome cache. | Quality pipelines and diagnostic schemas differ. |
| Freshness | Source adapter wake/reveal policies are executed by Bridge, including X wake/reveal observations. | X initial headless feed capture now probes the shared pending control, activates it once when allowed, and qualifies primary identity/content changes. Other sources remain unverified. | Headless qualification covers visible X controls and within-capture changes; it does not establish global feed freshness or full Bridge wake parity. |
| Feed continuation | Bridge tracks frontier and anchor matching in its tab. | Worker tracks per-source frontier, validates continuation anchors and bounds scroll rounds. Temporary text recovery leaves retained feed navigation alone. | Similar concepts, separate state and controllers. |
| User window ownership | Captures the existing browser surface under Bridge permissions. | Owns headless pages; native-reader ownership can block profile access until explicit user closure. | Browser lifecycle must remain backend-specific; no silent closure of user windows. |

Evidence: `AkuBridge/adapters/x-adapter.js` (`contentExpansion`, quote/identity/media/freshness contracts); `AkuBridge/content-script.js` (`recoverPermalinks`, `expandSourceContent`, `evaluateBlockQuality`, continuation); `internal/collection/headless/worker/vendor/x-extract.js`; `capture.mjs`; `observation.mjs`; `x-text-recovery.mjs`.

## Reported truncated post and scoped fix

The Darek Gusto post `2106867448746021278` was stored with 276 characters, `requires_permalink_capture`, `text_may_be_collapsed` and `permalink_capture_timeout`. Timeline rendered all stored text. Its Show more threshold (420 characters / 6 logical lines) did not apply; UI expansion cannot recover missing source text.

Before this fix, recovery allowed 3 targets, 3 seconds per target and 6 seconds total, caching the first failure. It did poll for hydration within that attempt, but did not retry navigation after a transient failure. The recorded timeout alone does not identify the failed stage.

The fix retains 3 distinct targets and caching, permits at most two attempts per target only for timeout or capture failure, and uses an 8-second per-target / 18-second shared recovery ceiling. The first attempt gets at most half the target budget. Neither attempt can exceed the enclosing capture deadline. Identity mismatch, login/challenge/unavailable and unresolved collapse are not retried. A settled temporary-page setup rejection can be retried; a still-pending setup is reused to avoid leaking owned pages.

`captureQuality.textRecovery` now records each attempt's stage (`target_setup`, `navigation`, `document_ready`, `asset_injection`, `text_collection` or `deadline`), outcome, limitation, duration and total elapsed time. Original feed media/quotes/engagement remain unchanged. Incomplete text remains explicitly partial if recovery fails. The observation cap of 4,000 code points remains unchanged.

The increased ceiling can add up to 12 seconds to recovery compared with the old shared ceiling, subject to the existing capture deadline. It does not retroactively change stored posts. Activation and recapture of the reported post are separate runtime validation steps.

## Implemented shared boundary

`AkuBridge/capture-primitives.js` is the single implementation for structured text, strict X permalink canonicalization, expansion-label normalization, quote-safe content expansion/restoration, text completeness, text-replacement acceptance, and primary/quote identity decisions. Bridge loads it before its source scripts; the headless worker injects the same hashed asset before its adapters. `capture-primitives.mjs` evaluates that asset in an isolated Node VM for pure recovery rules; it does not maintain a second copy.

Expansion re-resolves the text root after DOM replacement, ignores quote controls, and aborts on route changes or the host deadline. Windowed retains inline expansion and restoration policy; headless retains its matching-detail-route restriction and detect-only policy. Headless recovery continues to use an owned temporary page without changing the retained feed.

The additive `textCompleteness` contract is `unknown`, `no_collapse_observed`, `expanded`, `recovery_verified`, `truncated`, or `partial`. No collapse control is an observation, not a universal completeness guarantee. Legacy `textStatus` and presentation expansion labels are projected for compatibility. Text replacement still requires exact identity, a resolved candidate, and more code points than the stored excerpt. Failed recovery preserves the original feed evidence.

Primary identity is evaluated through the shared helper, with `identityComparison` recording the legacy/shared decision and agreement. This is comparison metadata; windowed admission is not switched to the worker's exactly-one-permalink gate. Quote decisions share explicit relation/conflict handling, but worker MAIN-world traversal and host-specific evidence acquisition stay separate. Bridge initially compares its observed quote identity without adding MAIN-world traversal or navigation.

Selectors, adapters and structured-media resolvers were already shared. This consolidation does not merge media transport/hydration, quality retry controllers, frontier ownership, profile leases, lifecycle, or scheduler. Headless quality remains `unverified`. The later X freshness phase below reuses pending-control discovery while retaining a separate headless qualification controller.

Both updated components must be staged together: the worker now requires the shared Bridge asset. Deploy/reload/restart and paired authenticated runtime validation remain separate from source-level tests. Old captured posts require recapture; these helpers cannot reconstruct absent text retroactively.

The extension version remains `0.9.2`; runtime revision/build identity moves to `source-adapters-v112` in Bridge and Sidecar's expected identity. This also changes the content-script reinjection guard so an already-injected v111 controller is replaced rather than silently reused.

## Deferred UX consideration: manually opened reader tabs

Incident reported and clarified by the user on 6 October 2026 (Asia/Jakarta): the native reader window contained a tab the user opened manually. Clicking "Tutup jendela post & lanjutkan auto update" caused a brief UI refresh but left the window open. The banner displayed `native reader window contains an unverified page; close it manually`, which was easy to overlook.

The user expected the button to close the entire reader window, including manually opened tabs. Current behavior instead rejects closure when any page in that window is absent from `readerTargets`. This guard already existed in commit `7aeb7c7`; the v112 consolidation did not change the closure path. Collection remains blocked until the reader is released.

User decision: leave current behavior unchanged for now and retain this incident for future consideration. Potential future work, not yet approved: make the blocking page and error more visible, clarify the button's scope, or offer an explicit confirmation to close all tabs in an ownership-verified native reader window. Any such flow must preserve unrelated user windows and verify profile release before resuming headless collection.

## Native post navigation when Windows rejects foreground activation

On 6 October 2026, a trusted native-post click completed profile handoff and reader preparation but the helper reported `windows_activation_rejected`, with the UI still foreground and the reader visible and not minimized. The previous completion path waited for successful foreground activation before dispatching post navigation, leaving the prepared reader at its local marker and blocking headless profile access.

The authorized fix separates foreground verification from one-shot navigation. Only the authenticated direct-reader path can fall back after that exact Windows rejection, with explicit matching focus/visibility diagnostics, a live context, an unexpired binding, and a revalidated marker tab in the original owned window. Changed pages/windows, cancelled or expired requests, missing diagnostics, transport failures and other rejection reasons remain rejected. Normal activation still verifies foreground before navigation.

Fallback navigation reports `foreground: manual_required` and asks the user to select the reader window manually. It does not report successful foreground verification or release the reader's profile ownership. Runtime restart/reload and real Windows activation validation remain separate from source-level regression tests.

## Bounded Darek text validation, 7 October 2026

The user authorized updating the parity QA harness to accept an idle, ready
headless coordinator without a windowed Bridge heartbeat. Browser mode retains
its heartbeat and source-access checks. Both modes still require a healthy
Supervisor, exactly one owner matching the registered Chromium executable,
zero active leases, no native-reader ownership and no pending transition.
Restoration verifies the original effective backend rather than treating an
absent MV3 heartbeat as failed headless restoration.

At 14:24 WIB, the current source worker captured X post
`x:status:2106867448746021278` at its stored canonical Darek permalink. One exact
identity matched; its 431-character primary text matched the user's native-post
screenshot after whitespace normalization, including the final Claude paragraph.
This is a supplementary comparison against the historical screenshot, not a
fresh paired windowed/headless parity claim. The older Timeline record was not
rewritten. The worker explicitly shut down, released the profile, and Supervisor
restored a healthy headless owner. The receipt is
`build/darek-validation-20261007-receipt.json` (local QA artifact).

The owner subsequently opened two native posts and confirmed closing them with
the close-and-resume button. Logs show both navigation requests completing at
14:25:51 and 14:26:00 WIB, with reuse on the second request, followed by profile
release at 14:26:04. The coordinator returned to ready headless with zero leases
and `nativeReaderBlocked: false`.

Immediate batch preparation remained guarded by the existing generation
allowance. No allowance or scheduling setting was changed. After the allowance
became available, scheduler session `session_d9968e0ef48fe6f1e2c966ea398e4ae0`
started at 14:34:11 and completed at 14:35:00 WIB, with X, Instagram and LinkedIn
runs completed without errors. Headless remained ready with zero active leases.
The read-only observer receipt is `build/reader-close-resume-live-20261007.json`.
The deferred manual-tab closure policy remains unchanged.

## X headless freshness phase 1, 7 October 2026

Initial X feed capture already navigates to `/home`; this change does not add
another reload. After hydration, the worker executes the hashed Bridge
`source-freshness-runtime.js` probe. Its additive `activatePending` helper
reuses the adapter's label, visibility and feed/quote exclusion rules and checks
the exact page URL and mutation deadline immediately before clicking.

Only round 1 on an owned `headless_worker` X home page can reveal, and only with
`reveal_if_present`, `wake_and_reveal` and `sameTabMutationAllowed: true`.
Activation is attempted at most once. The phase has a five-second ceiling and
reserves one second of the enclosing capture budget plus time for a final route
check. Explicit native targets, round 2 and Quiet browser mode do not activate
the control. Round 2 retains its original frontier and anchor contract.

After activation, `headless-freshness.mjs` compares direct primary posts with matching
native status IDs and canonical permalinks against the initial snapshot. A new
primary ID or changed normalized primary text qualifies; reordered posts,
engagement counts, author/relative-time labels and quote-only changes do not.
Qualification is scoped to this capture, not historical novelty, chronological
newestness or a guarantee that X exposed all available content.

`coverage.freshness` retains `requestedPolicy` and `workerStatus`, adds `status`,
probe outcome, pending label, bounded added/changed primary counts and elapsed
time. `checked_no_pending` means no supported visible pending control was found.
`verified` means qualifying primary evidence was observed after activation.
`pending_not_revealed`, `preserved`, `not_applicable`, `unavailable`,
`reveal_failed` and `reveal_unverified` preserve the reason rather than claiming
success. `activationAttempts` counts requests; `activationCount` is zero or one
for known outcomes and null when a transport failure leaves the click unknown.
Unverified reveals retain the original snapshot. Route changes, login,
challenges and unavailable-source states fail the capture instead of admitting
the old feed as a successful reveal.

Scheduler cadence, generation allowance, profile leases, user-window ownership,
cross-session deduplication and reasoning cooldown remain unchanged. Instagram,
Facebook and LinkedIn still report freshness as `not_verified` in this worker.
The controller and shared asset are included in runtime provenance and normal
worker staging. Controller, shared DOM-helper and capture/frontier integration
fixtures verify the implemented contract. The runtime qualification below
additionally covers actual feed probes; activation of a naturally appearing
pending control has not yet been observed on the authenticated account.

Validation: 137 worker tests and 395 Bridge tests passed, including the worker's
isolated real-Chromium playback regression. Bridge package verification passed.
Canonical staging produced `build/freshness-x-phase1-20261007/headless-worker`
with the new controller's SHA-256 matching source. This initial source validation
preceded the runtime activation described below.

## Adapter-driven generic headless controller

The headless orchestration now lives in `headless-freshness.mjs` for every source.
There is no X host, status-ID parser or source branch in that controller. Its
deadline, one-attempt activation, route checks, polling, primary-content
comparison and diagnostic outcomes are reused through one implementation.

Each adapter's optional `freshness.headless` contract declares `enabled`, a
version, `matchesFeedURL(url)` and `primaryIdentity(post)`. X owns the `/home`
restriction and exact native-ID/permalink agreement in its adapter. The registry
requires route and identity methods plus reveal support before accepting an
enabled contract. Windowed wake/reveal policy and controller remain unchanged.

`source-freshness-contract.mjs` evaluates the actual provenance-hashed Bridge
adapter asset in a bounded Node VM and exposes only its pure contract methods.
It does not keep a second copy of platform URL or identity rules. The browser
activation helper checks the same adapter's feed-route policy and capability,
plus exact current URL and deadline before the click. Worker staging includes
the shared runtime after each source adapter and hashes both generic modules.

| Source | Headless freshness contract | Existing windowed reveal support |
| --- | --- | --- |
| X | Enabled; same initial-feed behavior as phase 1 | Supported |
| LinkedIn | Explicitly disabled pending source validation | Supported |
| Instagram | Explicitly disabled | Unsupported |
| Facebook | Explicitly disabled | Unsupported |

A disabled, missing, malformed or source-mismatched contract reports
`not_verified` / `source_contract_unsupported` before freshness page evaluation
or mutation. For supported sources, explicit native targets remain
`not_applicable`, later rounds remain `preserved`, and Quiet browser mode remains
`not_applicable`. The generic diagnostic schema is
`aku.headless-source-freshness.v1`; legacy `workerStatus` remains available, with
`source` and `adapterFreshnessVersion` recording the chosen contract.

Adding headless reveal support for another source requires its own verified
route/identity contract and QA; a windowed `revealSupported: true` alone does not
enable it. No additional source was enabled by this refactor. Scheduler,
allowance, leases and user-window ownership retain their existing behavior.

Refactor validation: 148 worker tests and 397 Bridge tests passed, including
explicit-target, frontier, disabled-source capture and synthetic non-X contract
regressions. Bridge package verification passed. Canonical staging produced
`build/freshness-generic-20261007/headless-worker`; its controller and contract
loader hashes match source. Importing the staged worker confirmed X enabled and
the other three adapters disabled. Runtime qualification is recorded below.

## Runtime activation and authenticated qualification

On 7 October 2026, the user authorized restart, extension reload, commit/push
and live testing. The development worker was rebuilt and promoted through
Supervisor at idle. Bridge reload used the product maintenance action with a
temporary Browser transition, verified heartbeat build
`aku-bridge-0.9.2-source-adapters-v112` at 16:02:26 WIB, and restored Headless
ready with zero leases. No persistent collection-mode change was retained.

The first authenticated worker test exposed missing backend identity on the
owned Chromium adapter: capture succeeded, but X freshness was skipped as
`backend_unsupported`. `chrome.mjs` now declares `headless_worker` on its owned
browser handle. A real-Chromium regression verifies that the controller reaches
its probe through that handle. The worker was rebuilt and restarted before
repeating account qualification; 149 worker tests and 397 Bridge tests pass.

The repeat, 16:06:58-16:07:53 WIB, used the running staged worker and production X
round-1 reveal policy. X feed capture returned `checked_no_pending` with an
observed probe and three unique native IDs. No supported pending control was
visible, so no click was attempted. Round 2 returned `preserved`, continued
from scroll 675 to 1350 and captured five unique IDs. The explicit Darek target
returned `not_applicable` / `explicit_target`, exactly one matching primary
identity and 431 characters. It did not rewrite the existing Timeline record.

LinkedIn and Instagram feed captures succeeded with two unique native IDs each
and freshness `not_verified`, as required by their disabled contracts. Facebook
succeeded initially, returned `empty_unverified` on the repeat, then succeeded
in one bounded source-only recheck with one native ID and freshness
`not_verified`. That observation alone does not establish the cause of the
intermittent empty capture; no Facebook adapter recovery was changed.

Each operator test explicitly shut down its worker, confirmed zero owners of
the test profile before restoration, and restored Supervisor-owned Headless
ready with zero leases. QA did not invoke reasoning or write feed observations
into Timeline. Local, ignored receipts are
`build/freshness-reload-20261007-receipt.json`,
`build/freshness-live-20261007-receipt.json` and
`build/freshness-facebook-recheck-20261007-receipt.json`.
Real-account reveal-click qualification remains pending natural availability of
the supported control; fixture activation/identity regressions remain the
evidence for that branch.

## Acquisition telemetry handoff

The engine combines candidate evidence across acquisition rounds but exposes
only the latest round's capture telemetry at the top of `coverage`. Raw round
coverage remains in `coverage.rounds`. Durable pipeline receipts, including
follow-up yield, are retained without overwriting current capture fields or
filling missing latest fields from an earlier round.

Headless now emits the existing windowed keys `performedScrolls` and
`scrollStopReason`, while retaining its legacy `stopReason`. Scroll counts
describe observed movement, using the windowed two-pixel threshold. No movement
ends bounded collection; unavailable position leaves the count unknown. The
normal stop reasons are `not_requested`, `budget_exhausted`, `no_movement` and
`deadline`, with explicit reasons for evidence limits and unavailable position.
A deadline already reached between snapshots retains accepted partial evidence;
an in-flight collection deadline retains the existing failure path.

Planning telemetry is nullable: absent or malformed counts, frontier signals,
and readiness are JSON `null`, rather than false or zero. Native anchor values
remain private; only a validated count reaches the planner. The optional
`frontier.continuationReady` takes precedence over deriving readiness from a
legacy anchor array. Explicit false or unknown readiness prevents continuation
dispatch. Headless retains a frontier only with observed position and native
anchors, and reports missing height or viewport as an unknown candidate signal.
Its overall quality remains partial/unverified; this change does not assert
windowed quality parity or enable the complete-quality local follow-up gate.

Regression coverage follows capture acceptance through the model planner,
persisted continuation, adjacent capture and final evaluation for both backend
telemetry shapes. It checks that both rounds' candidates survive, latest
telemetry is retained, acquisition remains bounded to two rounds, and round two
uses `preserve_frontier` / `detect_only` without further freshness activation.
There is no scheduler, allowance, source-enablement or user-window policy change.

Validation: 149 worker tests pass (one real-Chromium test skipped without its
explicit executable setting). Engine and reasoning suites pass except
`TestHybridFacebookRecaptureAdmitsBrowserDispatchesBridgeAndCleansSurface`, whose
cleanup-generation mismatch also reproduces against pre-change HEAD through an
isolated Go overlay. Authenticated account qualification of the telemetry patch
remains pending runtime activation and subsequent user updates.
