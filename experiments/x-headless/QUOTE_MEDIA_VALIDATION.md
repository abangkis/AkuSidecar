# Quoted media validation

Validated 2026-10-01 (Asia/Jakarta), confined to the independent headless PoC.

`extract.js` passes its bounded media reader into AkuBridge's `extractQuotedPost`. The reader uses the adapter's image selectors, video roots, expected-kind detector and trusted media hosts. Primary media excludes the quoted root; quoted media belongs to that root only. Avatar/emoji images are excluded. Duplicate poster URLs within one scope are consolidated. Output is capped at 20 media observations per scope.

Images report decode readiness and natural dimensions. Actual video elements report metadata readiness, dimensions, playback state and source kind; their poster URL is still only a poster observation. Standalone poster images report image decode readiness. `loadedMeaning` makes these different readiness meanings explicit.

Each scope has `mediaEvidence`: observed URLs remain partial observations; expected kinds without an accepted URL are `missing_expected_url`; no observed media is not a claim that no source media exists. A video stream is not resolved or downloaded by this package.

## Validation

The real headless Chrome fixture passed image/poster scope separation, poster deduplication, trusted-host filtering, unloaded media, expected media without an accepted URL, and the earlier text/quote/browser cleanup checks. Synthetic media requests were blocked so the fixture did not fetch external images.

Three previously observed quote posts were captured on their native permalinks using the registered capture profile:

| Case | Observed quoted media | Readiness and HTTP evidence |
| --- | --- | --- |
| 1 | One video poster | Video metadata ready; poster response HTTP 200. Adapter also reported an expected image kind without an accepted image URL. |
| 2 | No media observed | No expected media kind exposed in the quoted DOM. |
| 3 | One image | Decoded with natural dimensions; HTTP 200. |

All three primary identities matched their target URLs; authenticated UI remained present with no login requirement or challenge. The existing adapter obtained a quote permalink in one detail view; two quote permalinks remained unknown. Earlier observations did not supply parent permalinks for cross-run comparison. Unidentified quote roots must not be merged into independently identified source posts. Fixture quote identities are explicit and separately verified.

`node verify-quote-media.mjs` independently verified the saved media evidence and lifecycle receipts: no browser owner remained after capture, Sidecar returned healthy and Bridge compatible after every run. Private captures and receipts remain in ignored `runs/`; the aggregate is `runs/quote-media-validation.json`.

Reproduce live with `node quote-media-test.mjs` (temporarily stops/restores registered Sidecar). Verify existing evidence with `node verify-quote-media.mjs`; run the isolated fixture with `node smoke.mjs`.

At this media-validation stage the remaining gaps were quote permalink recovery in detail views, unresolved expected media kinds, full video acquisition, collector parity and lifecycle design that avoids routine stop/start. See `QUOTE_IDENTITY_VALIDATION.md` for subsequent recovery of the two unknown quote identities. No measured zero-blinking claim is made.
