# LinkedIn SharedDOMEngine shadow probe (Headless Stage 03)

The same `AkuBridge/adapters/linkedin-dom-shadow.js` source binding runs beside
the existing Windowed readiness/capture and ordinary Headless collector. It
passes normalized evidence to the unchanged SharedDOMEngine browser artifact.
The source candidate is Bridge 0.9.6 / `source-adapters-v131`, with thin worker
registration entry v5 and matching Sidecar/AkuBrowser identity. This is an
offline source increment; activation and live qualification are separate.

## Evidence and authority

The binding reuses the actual LinkedIn adapter's native-ID normalization,
permalink policy, exact freshness identity pair, author selectors, declared text
root, media-root policy and trusted media hosts. It records all primary claims
instead of selecting the first link when claims conflict. Nested candidates,
comments and embedded repost evidence cannot supply primary fields or identity.
Native `activity`, `ugcPost` and `share` claims remain distinct.

Text outside the declared authored-text root and document shells without an
owned trusted preview remain unverified in this diagnostic. Existing capture
fallbacks and attachment extraction retain their behavior. A diagnostic `ready`
does not establish complete media, decoded content, playback, account proof or
capture admission. A single sample never proves temporal drift or an empty feed.

Sampling is bounded to 64 candidates, 100 queried/traversed nodes per read and
eight distinct native claims per candidate. Telemetry contains only closed codes,
bounded counts and rejection codes. The probe does not recover permalinks,
expand text, click, scroll, navigate, fetch, inspect accounts or write storage.
Failures are isolated from capture and readiness. The shared evaluator is
unchanged; selectors, identifiers and registration stay outside it.

## Equivalent offline fixtures

`scripts/shared-dom-shadow-fixture.mjs` is the shared runner used by the existing
Instagram and new LinkedIn fixture corpora. It executes actual Windowed message,
readiness and capture entry points and the actual Headless collector in separate
disposable Chrome profiles. CSP blocks subresources and DNS resolution is
disabled. Fixtures are synthetic, with no signed-in or live source profile.

The 45 LinkedIn cases cover native/wrapper/action cards, all supported native ID
kinds, root attributes and public activity anchors, conflicting/foreign identity,
offscreen content, owned image/poster evidence, placeholders, unknown content,
nested/comment/repost isolation, login/challenge/loading, unknown-empty state,
sampling bounds and a throwing probe. Both consumers load the same engine/probe
hashes. Their complete baseline output must remain identical with shadow disabled,
after excluding only shadow and timing fields. Final Headless projection is
separately checked for diagnostic allowlisting and unchanged capture output.

One route-boundary difference is intentional: Headless rejects an unsupported
hostname before selecting a source adapter and emits no source probe diagnostic;
Windowed receives an explicit source and emits `wrong_route`. All other corpus
diagnostics agree. Mutation/effect traps assert zero added shadow mutations,
clicks, scrolls or JavaScript network calls. The retained 23 Instagram cases verify
the shared runner extraction independently.

```powershell
node scripts/shared-dom-linkedin-shadow.mjs 'C:/Program Files/Google/Chrome/Application/chrome.exe' build/linkedin-dom-shadow/verification.json
```

Project-local evidence is under `build/linkedin-dom-shadow/`. No separate native
package is required. C1 quality/media decisions remain intact. Native account
qualification, capture-decision migration, LinkedIn continuation/live media under
Stage 05, Facebook routing and parent-stage acceptance retain their own scope.

## Permalink recovery blocker diagnostics

The source hardening passed 666 Bridge tests and 365 worker tests (seven optional
worker cases skipped). The final projector/error-context refinements also passed
the 11 targeted diagnostic regressions. Isolated real-DOM verification passed
45 LinkedIn and 23 Instagram fixtures. These are offline results, not runtime
activation or live identity qualification.

The shared LinkedIn adapter now adds a bounded `blockerDiagnostic` only when
the existing visible-menu/dialog guard already rejects permalink recovery.
Windowed sends `linkedinRecoveryBlockers` in `observation.coverage`, with
`snapshotIndex` on each record. The typed snapshot payload follows the Sidecar
JSON contract. The ordinary Headless collector retains these records in its
internal snapshot, including candidates rejected for missing identity. The
native observation and empty-capture error projections allowlist the same
metadata. The Go endpoint regression checks acceptance and storage readback.

Each record uses the closed `existing_surface` code, visible menu/dialog counts
capped at 32, and at most four samples. Each consumer retains at most eight
records per projection. Samples contain only closed selector/reference codes
and boolean-or-null observations: positive area, viewport intersection, display,
CSS visibility and opacity. `hiddenAttribute`, `ariaHidden` and `inert` are
separate facts on the surface itself. `ancestors` inspects at most 16 ancestors,
excluding the surface, and separately aggregates display/visibility/opacity and
the three semantic attributes. Truncated or unreadable tails remain unknown
unless a positive observation was already established. These observations do
not treat `aria-hidden` or `inert` as proof of CSS invisibility.

`aria.labelledBy`, `aria.controls` and `aria.owns` independently compare references
with the current trigger/surface. Each is `matches`, `different`, `absent` or
`unknown`. A matching label cannot conceal a conflicting controls reference;
`different` does not claim that the referenced element is another post's trigger.
Neither result establishes ownership or authorizes cleanup. Raw text, labels,
exception messages, URLs and DOM IDs are never copied.

Each record carries `phase:pre_recovery_guard`, `snapshotIndex` and `candidateIndex`.
Indices are bounded to 0..1,000,000 or null when unavailable. Candidate position
refers to the consumer's discovery list, not a persistent post or DOM identifier.
Windowed supplies its current snapshot index. The native raw collector leaves
that index unknown; the observation/error projection supplies the actual capture
snapshot index. This context orders observations within a capture; it does not
prove that the same surface persisted across snapshots or documents.

The guard keeps its original menu-first short circuit. When menus already block
recovery, `dialogCount:null` means dialogs were not checked; it is not zero.
`complete:false` also covers skipped dialog checks, truncated samples/counts and
unavailable diagnostic reads. Unknown visibility remains null. Diagnostic reads
cannot dismiss a surface, recover a permalink, admit an unidentified candidate,
relax login/loading, or alter capture deadlines. Guard visibility and potentially
contradictory style/ancestor observations are reported separately, allowing a
later investigation without changing the current blocking decision.

The seven blocker cases cover visible menus, visible dialogs, ARIA-hidden/inert
ancestors, hidden nonblocking dialogs, conflicting ARIA references and an
ancestor with `opacity:0`. The hidden-dialog fixture has a valid post owner and
asserts an actual recovery-button click in both consumers; it cannot pass merely
by failing owner discovery before the guard.

Windowed and Headless receive identical fixtures. Ordinary blocker data agrees
after native context projection. The transparent-ancestor fixture deliberately
records the existing operational difference: Windowed considers the surface
visible and blocks, while Headless rejects the ancestor's visibility and attempts
recovery. This diagnostic increment preserves both predicates and does not
resolve that difference. Shadow results and each consumer's capture output remain
unchanged relative to its own shadow-disabled baseline.
Unit regressions also cover throwing reads, count/sample limits, stale-owner
protection, private-data stripping, invalid telemetry and native empty-capture
projection. This source change has not been built, activated or live-qualified.

Local evidence for the hardening is retained in
`build/linkedin-blocker-hardening-parity.json`,
`build/linkedin-blocker-hardening-instagram.json`,
`build/linkedin-blocker-hardening-bridge-tests.log` and
`build/linkedin-blocker-hardening-native-tests.log`. Shadow effect counters cover
the passive probe only; fixture recovery-button clicks are separately reported
and are existing recovery behavior, not added diagnostic interactions.
