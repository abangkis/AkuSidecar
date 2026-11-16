# Facebook extractor comparison

Validated 2026-10-01, within the existing experimental package. AkuBridge and
production Sidecar source/settings were not changed. Captures were not ingested.

## Scope

The harness navigates three saved native Facebook URLs using registered Google
Chrome headless, the same authenticated capture profile and a 1280x900 viewport.
For each DOM it runs: production extractor before PoC, PoC with its bounded hover,
then production extractor again. Production scripts are read directly from the
checkout and execute in a separate CDP isolated world. The production MAIN-world
structured-video resolver and deferred-delivery content-script path are reused.
Only listener registration and bounded extension timers are emulated. This is not
the installed Bridge service worker, a headed browser comparison, end-to-end
Sidecar collection, or a performance benchmark. Sequential observations can hydrate.

Native Timeline target selection occurs before Sidecar shutdown. The existing
wrapper refuses active Inbox sessions and rechecks after selection; it releases
the profile and restores Sidecar/Bridge in finally. The lifecycle receipt verifies
healthy restoration, compatible Bridge, and no leftover headless profile owner.

## Live results: identity-only matching is unsafe

Private immutable evidence:
`runs/2026-10-01T06-21-59-671Z-29440-facebook-comparison/capture.json`
and `runs/shared-profile-2026-10-01T06-21-57-983Z.json`.
Capture SHA-256:
`c0ddcff3d8863d40e0c69b1241780e97dd3d50f96fdf86395f67f646c2bd5168`.

The first harness revision joined the first record by native post ID only. Its raw
`paired` and text-equality fields are **not valid parity claims**: all three joins
picked the same unrelated 141-character body under three distinct target IDs.
Those raw reports remain unchanged as negative evidence. The updated verifier
assesses author/text binding against the saved target, considers every same-ID
record, and rejects the misleading pairs.

| Target selected from Timeline | PoC record matching saved author and exact text | Media on that record | Production extractor retained that target |
| --- | --- | --- | --- |
| Long text | 746 characters | 1 image | No |
| Multi-image | 988 characters | 2 images | No |
| Video candidate | 54 characters | 1 image; video expected, stream unresolved | No |
| Shared post | No saved native target available | Untested live | Untested live |

Each live case produced two PoC records with the same ID but different author and
text. Exactly one matches the saved target author and complete text. The production
extractor produced one same-ID record whose author/text do not match that baseline;
the result was unchanged in the post-PoC diagnostic pass. All cases retained signed-in
UI evidence. Three PoC targets were verified, three identities were ambiguous, and
**zero valid production/PoC pairs** were established.

Source inspection explains why these results cannot be accepted: Facebook canonical
permalink selection reads descendant anchors; author and text are selected separately.
Production deduplication then keeps the first admitted record for a platform ID.
The captured data demonstrate the mixed bindings and lost target; precise live DOM
ownership/visibility tracing remains necessary before assigning a complete root cause.
The experiment's comparison guard does not repair either extractor's candidate binding.

One correctly bound live PoC record exposes `native_structured_story` time from an
exact-ID Story JSON match whose scan did not reach its bounds (`bounded:false`).
The other two target records expose labelled relative estimates. This extends the
earlier fixture-only JSON evidence by one live observation, not universal exact-time
coverage or independent validation of Facebook's timestamp semantics.

## Fixture results

`node facebook-comparison-smoke.mjs` passes in real headless Chrome using local
synthetic HTML with CDN network requests blocked. The fixture-only hostname shim
allows localhost; it is never included in live production-script injection.

- An expanded post over 4,000 characters demonstrates the production text cap while
  PoC preserves the longer body; both restore the collapsed state.
- Two 240x120 images produce identical exact URL sets. An earlier 160-pixel-wide
  fixture was rejected by the production geometry policy; size filtering is not parity.
- The production structured/deferred path enriches a synthetic video with one
  playback URL; PoC retains only poster evidence. This is not a live video success.
- Shared-post relationship classification works in the fixture; PoC still explicitly
  reports that the original shared body is not separately extracted.
- A same-ID record with foreign author/text cannot pass the baseline binding guard.

## Reproduction and next boundary

Run from this package:

```powershell
node facebook-comparison-smoke.mjs
node shared-profile-test.mjs --source facebook --facebook-compare --wait-ms 30000 --max-scrolls 0
node facebook-comparison-verify.mjs runs/2026-10-01T06-21-59-671Z-29440-facebook-comparison/capture.json runs/shared-profile-2026-10-01T06-21-57-983Z.json
```

The live command temporarily closes registered Sidecar/UI/capture. The verifier
only reads the saved artifacts and emits aggregate evidence. The latest harness
rejects unverified bindings rather than claiming successful pairs; the saved live
capture predates that guard but is re-assessed by the verifier and covered by its
same-ID regression fixture. No redundant live rerun is needed to reinterpret those
unchanged capture inputs.

Before migration, resolve primary-container ownership and bind author/text/media to
the same native identity before deduplication. Then repeat same-post extraction
comparison, followed separately by installed headed/headless lifecycle testing.
Live video playback, shared-post detail, and zero OS blinking remain unproven.
