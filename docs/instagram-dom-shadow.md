# Instagram SharedDOMEngine shadow probe (Headless Stage 03)

The new `adapters/instagram-dom-shadow.js` binding feeds normalized, bounded
Instagram DOM observations into the unchanged SharedDOMEngine browser artifact.
Windowed MV3 registration, generic source injection and ordinary Headless asset
loading carry the same engine and probe. The probe exposes only `assess`; capture
identity and admission continue through their existing paths.

## Observation boundary

The source binding reuses the actual Instagram adapter's native identity,
author, caption and media-root policies. It excludes nested article/candidate and
comment evidence from the primary root. Native links must agree; conflicting
claims are rejected by the existing source-independent evaluator. No selectors or
Instagram identifiers are added to the engine.

Sampling is bounded to 64 candidates, 100 queried nodes and eight distinct native
claims per candidate. A failed/bounded candidate remains diagnostic evidence.
Access failures retain closed codes; an absent feed stays `empty_unverified`.
Samples are independent and unsettled, so they do not claim temporal DOM drift.
A valid candidate can yield an engine `ready` diagnostic while the existing
readiness policy still reports loading. Controllers continue using that existing
policy; the shadow result cannot authorize capture, recovery or native admission.

The probe reads DOM only. It does not expand text, acquire media, resolve playback,
click, scroll, fetch, navigate, inspect accounts or write storage. Its failure is
isolated from existing readiness/capture. Only closed diagnostic codes, bounded
counts and rejection codes are projected into observation coverage; raw text,
identities and locators are not emitted as shadow telemetry.

## Offline evidence

`scripts/shared-dom-instagram-shadow.mjs` runs the real Windowed message/readiness/
capture entry points and the actual Headless collector in separate disposable
Chrome profiles, using identical synthetic HTML fixtures. CSP blocks subresources,
DNS resolution is disabled, and no signed-in browser/profile is used.

The 23 cases cover article and permalink-ancestor layouts, missing native link,
offscreen post, media-only content, video poster, missing author/content,
conflicting primary links, nested article and comment isolation, loading,
unverified empty feed, login, challenge, node/claim/candidate limits and a throwing
probe. Both loaders' engine/probe SHA-256 values must agree.

Each consumer runs with and without shadow assets. After excluding the additive
diagnostic and timing fields, its complete existing capture/readiness result must
remain equal. The two consumers' shadow diagnostics must agree; their existing
capture decisions are not forced to match each other. MutationObserver and effect
traps verify zero shadow mutations, clicks, scrolling or JavaScript network calls.

The current report is project-local at
`build/instagram-dom-shadow/verification.json`. Scratch HTML/browser profiles are
removed after each run; no extra native package or backup is created. Run:

```powershell
node scripts/shared-dom-instagram-shadow.mjs 'C:/Program Files/Google/Chrome/Application/chrome.exe' build/instagram-dom-shadow/verification.json
```

This increment is source implementation and offline shadow qualification only.
It does not complete all of Headless Stage 03, onboard LinkedIn/Facebook, migrate
Instagram capture identity, qualify native authentication or prove live behavior.
Publication, development activation and live capture remain separate actions.
