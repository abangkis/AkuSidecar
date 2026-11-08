# SharedDOMEngine stage 03 shadow diagnostics

AkuBridge Windowed X and the general Headless X worker load the same generated
`shared-dom-engine/browser-runtime.js` and the same `adapters/x-dom-shadow.js`
evidence binding. The runtime SHA-256 is
`abd93e9e6658befb465376d81208c79a5ae5564ce25589d89612739e8f526731`.
The package verifier includes both files in the extension runtime closure.

Windowed evaluates the candidates from its existing readiness probe and
forwards the diagnostic into observation coverage. Headless evaluates the
candidates from its existing snapshot and projects the last diagnostic into
coverage. The worker also includes it in its existing empty-capture diagnostic
payload. That payload is currently worker-local evidence at the failure
boundary because the Go client drops its `diagnostics` field. Both paths emit
closed codes and bounded counts. The observer adds no navigation, clicks,
scrolls, or Store calls. Each sample is independent, so this stage reports
current evidence and leaves repeated-sample DOM drift confirmation for later.

## Offline comparison

The runner uses separate disposable Chrome profiles and the real registered
Windowed assets and Headless executable assets. Each profile reads the same
synthetic fixtures after the document is ready. All 13 fixtures produced
identical shadow codes, counts, and rejections across modes. The 26 shadow
evaluations produced zero observed DOM mutations. Removing the additive shadow
fields reproduces every legacy fixture result in the verified stage 01 report.

| Fixture | Shared shadow result | Legacy capture result |
| --- | --- | --- |
| Legacy, legacy quote, media only | `ready` | One post in each mode |
| Wrapped without time, wrapped quote, hydrated wrapper | `ready` | One Windowed post. Headless rejects it as `missing_identity`. |
| Mixed layout | `ready`, two accepted, one rejected | Two Windowed posts and one Headless post |
| Placeholder | `no_valid_candidates`, missing required author | Windowed waits. Headless reports `feed_ready` and produces zero posts. |
| Loading placeholder | `hydrating` | Zero posts in both modes |
| Conflicting primary identity | `no_valid_candidates`, `identity_conflict` | Windowed produces one conflicting block. Headless rejects it. |
| Empty, login, challenge | `empty_unverified`, `login_required`, `challenge_required` | Zero posts in both modes |

The baseline report SHA-256 is
`60f30a5dd2ad11c80d0d27e14576cc0d5f1ad12ce584dfff6085fb75dd36f964`.
The broader Headless worker suite had nine X reply and GraphQL fixture failures,
all reproduced against the unchanged AkuBridge and AkuSidecar commits. The
new bounded diagnostic projection test passed. The offline scope covers DOM
classification and observation projection. It leaves live capture and Store
admission unverified.

Run from the workspace root with the verified Stage 01 report and a fresh
output path.

```powershell
node AkuSidecar/scripts/shared-dom-x-baseline.mjs 'C:/Program Files/Google/Chrome/Application/chrome.exe' AkuSidecar/build/shared-dom-x-shadow-stage03-isolated.json AkuSidecar/build/shared-dom-x-baseline-stage01-verified.json
```
