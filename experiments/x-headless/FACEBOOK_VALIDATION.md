# Facebook headless PoC validation

Validated 2026-10-01 (Asia/Jakarta) in the existing experimental package. Production source files and application settings were not changed; results are not imported into Sidecar.

`--source facebook` selects the existing `facebook-dom-v19` adapter. The collector reuses discovery/admission, canonical permalink, author, text expansion policy, relative timestamp estimation, engagement, media roots, allowed host suffixes and Facebook scroll tuning. It exports structured text, image decode readiness, video-poster metadata, evidence limits and rejection diagnostics. The launcher/lifecycle wrapper is shared with X; X defaults and fixtures remain valid.

## Measured live results

Both runs used the original registered Google Chrome executable and capture profile, headless, at 1280×900. No login or cookie copy was needed. Sidecar was temporarily stopped through the existing authorized wrapper and restored after every capture.

| Batch | Accepted post observations | Images / decoded / HTTP 200 | Known timestamps | Sampling rounds | Maximum scroll | Capture duration |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| First | 1 | 5 / 5 / 5 | 0 | 6 | 6621 px | 42.692 s |
| Second | 2 | 2 / 2 / 2 | 1 | 5 | 4799 px | 12.284 s |

The two runs contain three distinct accepted post IDs. All samples exposed signed-in account controls and no login requirement, detected challenge or source outage. Each accepted record has an author, canonical Facebook permalink and adapter-derived platform ID. Both runs stopped after three rounds without a new accepted identity, not at the end of the feed. The first run's longer duration is not a controlled performance result.

The verifier read captured image URLs, decoded dimensions and matching CDP response metadata; it independently checked lifecycle receipts for released profile ownership, healthy restored Sidecar and compatible Bridge. The private reports and receipts are kept under ignored `runs/`. Run `node facebook-verify.mjs` to inspect aggregate results without printing account names or post bodies.

## Fixture and regression checks

`node facebook-smoke.mjs` passed in real headless Chrome with synthetic content: canonical IDs/URLs, nested/non-feed admission rejection, text expansion with paragraphs/emoji and subsequent restoration, relative timestamp estimation, avatar exclusion, trusted image observation, unloaded video poster, login and site-outage detection. Synthetic CDN requests are blocked during the fixture. `node smoke.mjs` passed the existing X fixture after shared-launcher/source selection changes.

## Limits

- Native permalink exposure is the main current gap: 9 and 5 missing-identity observations were rejected across the two runs. These counts include repeated observations, not distinct lost posts. No IDs are invented from text or unrelated photo assets.
- Two accepted post observations have no timestamp. Estimated timestamps are labelled and unknown is preserved; timestamp completeness is not a pass claim.
- No text expansion or video-poster record occurred in these live samples; their paths are fixture-verified only. A lack of observed videos is not evidence that Facebook's feed has no videos.
- Full video playback/download, structured Facebook state/response resolvers, and shared-post body separation are not implemented in this first PoC.
- No normal-collector parity, complete-feed coverage, stable recommendation order, or zero-OS-blinking claim is made. The experimental stop/start handoff still closes and restores the product UI.

Reproduce with `node shared-profile-test.mjs --source facebook --max-posts 20 --max-scrolls 12 --wait-ms 60000` while Inbox is idle. The wrapper refuses to interrupt an active Inbox session. Next scoped work is native permalink recovery and timestamp evidence before comparing Facebook capture coverage with production.
