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

The 38 LinkedIn cases cover native/wrapper/action cards, all supported native ID
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
