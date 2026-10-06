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
| Freshness | Source adapter wake/reveal policies are executed by Bridge, including X wake/reveal observations. | Worker explicitly reports Bridge freshness qualification as unverified. | Headless capture is not proof of Bridge freshness parity. |
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

Selectors, adapters and structured-media resolvers were already shared. This consolidation does not merge media transport/hydration, quality retry controllers, frontier ownership, profile leases, lifecycle, scheduler, or freshness wake/reveal. Headless quality remains `unverified` and Bridge freshness qualification is still not implemented in the worker.

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
