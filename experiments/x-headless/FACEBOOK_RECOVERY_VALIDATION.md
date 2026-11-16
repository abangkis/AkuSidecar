# Facebook native identity and time recovery

Validated 2026-10-01 (Asia/Jakarta), in the existing experimental package only. Production source and settings are unchanged.

## Changes

- Up to four timestamp/header anchors per snapshot can be hovered through CDP in the owned headless tab, with a 650 ms wait per anchor. This does not move the OS cursor or activate a desktop window. No menu, engagement or publishing control is clicked.
- Hover candidates prioritize timestamp links, preserve the `aria-posinset` wrapper/primary-article distinction, and exclude deeper articles. A tooltip is used only through its anchor's explicit `aria-describedby` association and matching native permalink; arbitrary floating tooltips are not read as post evidence.
- Native timezone-qualified `time[datetime]`, `data-utime`, and relative header text are supported. Relative estimates carry `timestampEstimated=true`; future/contradictory time evidence is not accepted as an exact timestamp.
- An optional bounded already-loaded JSON Story reader accepts `creation_time` only through an exact native post URL/ID, or an equal opaque Story ID for its timestamp section. It scans at most 256 JSON script elements, parses at most 48 relevant scripts, limits each to 512 KB, total parsed data to 4 MB and traversal to 20,000 objects/depth 40. No response replay, cookie reading, raw JSON export, or direct Facebook API request is used. Bounded/conflicting resolution remains unknown.
- Missing-identity diagnostics export sanitized URL shapes and query-key names, not names, post bodies or query values.

## Live evidence

A bounded feed test recovered two previously unavailable native post permalinks after headless hover. Both were emitted by the existing adapter as `post_anchor`; no identity was derived from text or a photo ID. Remaining candidates without an accepted permalink were still rejected. That run returned to a healthy Sidecar and compatible Bridge with released profile ownership.

Two post IDs from the original Facebook PoC, both previously missing timestamps, were then opened on their known native detail URLs. In the final paired run both exact target IDs were captured with timestamps from relative text. Both are estimates, not exact publication times. All samples retained signed-in account controls without detected login/challenge; each lifecycle receipt confirms profile release and healthy Sidecar/Bridge restoration.

Earlier renders of those detail pages exposed one or zero usable timestamps. The final 2/2 result therefore establishes successful bounded recovery, not stable completeness across Facebook renders. The native-Unix and JSON Story paths passed local fixtures but did not establish exact timestamp recovery in these live cases. Tooltip association was not observed in the successful live examples.

Private evidence: `runs/facebook-recovery-validation.json`, its before/after reports and lifecycle receipts; the live hover run is `runs/2026-10-01T05-37-16-965Z-7808/capture.json`. `node facebook-recovery-verify.mjs` checks saved paired IDs, estimate labels, authenticated samples and receipts. `node facebook-recovery-test.mjs` performs the two native-detail captures with controlled Sidecar stop/restore.

## Local checks and remaining limits

The real headless fixture verifies native hover hydration, scoped tooltip estimates, native Unix time, primary-article wrappers, nested-post exclusion, exact Story binding, unrelated Stories, time conflicts, expansion/restoration, admission and login/outage detection. X regression also passed.

Facebook timestamp/identity completeness remains unproven; JSON evidence is only accepted when explicit bindings survive its bounds. Shared-post bodies, full video acquisition, production parity, zero OS blinking and an uninterrupted production browser lifecycle remain separate work. An active Inbox session correctly prevented a test shutdown; the test resumed after it completed.
