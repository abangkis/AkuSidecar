# SharedDOMEngine stage 01 baseline

Two isolated Chromium runs produced byte-identical results across 13 synthetic
X DOM cases. Both consumers loaded `x-dom-v24` from AkuBridge `465b1e5`. The
fixture report records SHA-256 hashes of the evaluated source files and the
Windowed readiness gate.

| Fixture | Windowed | Headless |
| --- | --- | --- |
| Legacy timestamp permalink | `feed_ready`, one identified post | `feed_ready`, same post |
| Wrapped permalink without timestamp | `feed_ready`, one identified post | `feed_ready`, zero posts, `missing_identity` |
| Legacy and wrapped posts with a placeholder | `feed_ready`, two posts and one invalid placeholder | `feed_ready`, legacy post only, two `missing_identity` rejections |
| Legacy nested quote | Main and quote identities stay separate | Same main and quote identities |
| Wrapped nested quote | Main and quote identities stay separate | Main post rejected as `missing_identity` |
| Media-only legacy post | One post with media and an empty body | Same |
| Visible wrapped placeholder | `feed_hydrating`, capture gate closed | `feed_ready`, zero posts, `missing_identity` |
| Hydration before and after | `feed_hydrating` → `feed_ready`, then one identified post | `feed_ready` in both samples, zero posts, `missing_identity` |
| Two distinct primary timestamp links | One capture block marked `conflicting`, with quality verdict `complete` | Zero posts, `ambiguous_identity` |
| Generic empty DOM without a recognized source marker | `selector_mismatch` with `feed_shell_unhydrated` | `selector_mismatch` |
| Login and challenge | `login_required` and `challenge_required` respectively | Same |

The wrapper-only gap is reproducible at the in-page capture boundary. Headless
derives primary identity from timestamp anchors and can report `feed_ready`
before it has accepted a post. The conflict fixture establishes a separate
baseline. Windowed emits the block with the first permalink and preserves the
conflict diagnosis in its identity comparison. Headless rejects the candidate.
This observation does not establish whether Sidecar would store the conflicting
Windowed block.

## Reproduce

Run from the workspace root with an installed Chromium or Chrome executable.

```powershell
node AkuSidecar/scripts/shared-dom-x-baseline.mjs 'C:/Program Files/Google/Chrome/Application/chrome.exe'
```

An optional second argument after the browser path writes a new JSON report
instead of printing it. The runner loads the Windowed registered feed scripts,
invokes its readiness and collect-visible message handlers, and applies the
service-worker readiness predicate before collection. For Headless it invokes
the X worker's `XHeadlessPoC.collect()` using the executable source assets
selected by its loader. Both paths see the same sanitized HTML in a real DOM
with the same viewport and a synthetic X route. The runner replaces the DOM
before the second consumer reads it. Hydration is represented by consecutive
fixture states.

The runner uses a disposable browser profile, local HTML, a restrictive content
policy, and blocked DNS. It uses a single snapshot with no scroll or page
mutation controls in the fixtures. The baseline covers in-page DOM readiness,
extraction, and candidate decisions. It does not cover live navigation,
extension transport, full worker orchestration, or Store admission.
