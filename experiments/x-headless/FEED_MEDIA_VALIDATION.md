# X headless feed/media validation — 2026-10-01, Asia/Jakarta

Three Home-feed batches and one targeted video-post test used the original
registered capture profile and Google Chrome 154.0.8037.58. AkuSidecar was stopped
and restored through Supervisor for each test. No login/cookie copy, foreground
capture fallback, extension reload, production integration, commit, or push.

## Results

All feed batches used a 1280×900 viewport, at most 30 posts, 12 scroll steps of
720 pixels, 30-second readiness ceiling, and one 750-ms passive media hydration
observation per sample. Each finished at its scroll limit, not the end of feed.

| Batch / WIB | Posts / unique IDs | Image observations / decoded | Video posters / frames ready | Collapsed texts | Capture duration |
| --- | --- | --- | --- | --- | --- |
| Home 1 / 02:42 | 14 / 14 | 10 / 10 | 0 / 0* | 9 | 27.525 s |
| Home 2 / 02:45 | 14 / 14 | 7 / 7 | 6 / 6 | 10 | 27.617 s |
| Home 3 / 02:47 | 16 / 16 | 5 / 5 | 1 / 1 | 10 | 27.763 s |
| Targeted video / 02:47 | 2 / 2 | 1 / 1 | 2 / 1 | 0 | 3.559 s |

*Home 1 used a narrower DOM video selector. After seeing video responses without
exported video records, the PoC selector was broadened to all nonquoted `video`
elements inside the post. Do not interpret its zero as an absence of videos or
compare that video yield directly against the later batches. Production files
were not changed. Home 2/3 used the same updated extractor.

### Verified

- Every sampling round observed the signed-in account UI; every test's document
  response included HTTP 200. No detected challenge or login request.
- Every feed batch added new post identities after scrolling, reaching 8640 px.
  The 44 exported feed posts were distinct across all three batches; per-batch
  repeated observations were deduplicated. This is sample uniqueness, not proof
  of complete coverage or stable recommendations across reloads.
- All 44 feed posts had author, text, parseable timestamp, and native permalink
  matching their ID. No future timestamp beyond the one-minute tolerance.
- All 22 image observations across Home batches had `complete=true` and positive
  natural dimensions. URLs were restricted to the adapter's allowed media hosts.
- Home 2 exposed six video records with blob sources, metadata and decoded
  frames. Home 3 exposed one frame-ready video and observed automatic playback.
- The targeted primary post was captured and its video had decoded frames,
  `paused=false`, and advancing nonzero playback time. A second observed video
  had a verified poster response but was not yet metadata/frame-ready at the
  sample. No playback button was clicked.
- Poster HTTP verification and video response bytes were observed through CDP;
  no HTTP >=400 response was observed among the recorded media responses.
  This does not count all network failures or prove a complete video download.
- All four lifecycle receipts verified an empty stopped process tree, profile
  release before launch and after capture, healthy service restoration, and
  Bridge compatibility. A final Supervisor snapshot remained healthy/running.
- `node smoke.mjs` passed with the new instrumentation. `node verify-reports.mjs`
  independently checked these four private reports and corresponding lifecycle
  receipts. Syntax checks passed. Captured post bodies/identities stay in ignored
  `runs/`; this document only contains aggregate results.

### Remaining gaps

- **29/44 feed texts were collapsed** behind Show more. DOM text is not full
  content; this PoC deliberately does not click expansion controls.
- Three feed records expected an image under the adapter's broader contract but
  exported no `image` URL. This may involve additional media roots/previews or
  overlapping video/image expectations. It is an unresolved mapping gap, not a
  proven count of three missing image files.
- Quoted-post bodies are not exported. One or more quote/repost relationships
  were observed, but relationship/attachment parity is not established.
- Nine missing-identity rejection observations occurred in each feed batch.
  Those observations are not necessarily distinct posts; their nature was not
  attributed. No ambiguous-identity or missing-author rejection was recorded.
- Blob-backed videos render/play, but the PoC does not resolve a complete
  downloadable stream or attach the production structured media evidence.
- No contemporary normal-collector baseline was run. No parity claim for
  quality, latency, CPU/RAM, or source freshness. Post timestamps are timestamps,
  not independent proof that the feed is the newest available feed.
- OS focus/Z-order events were not instrumented. Headless capture was requested;
  normal UI restoration is intentionally visible. No measured zero-blinking claim.

Conclusion: authenticated scrolling, basic image decoding, and video rendering /
automatic playback work in this headless sample. Full content/media capture and
production readiness remain open; the remaining work is primarily PoC extraction
and evidence completeness rather than demonstrated inability to render headless.

## Reproduce

From this experiment package:

```powershell
node shared-profile-test.mjs --max-posts 30 --max-scrolls 12 --wait-ms 30000
node video-test.mjs
node audit.mjs 4
node verify-reports.mjs
```

The first two commands intentionally stop/test/restore the registered service.
Use only when that interruption is intended. The last two read local artifacts;
audit writes a sanitized summary beside each ignored capture JSON.
`verify-reports.mjs` is the acceptance check for this suite (latest three feed
batches plus one targeted video run), not a generic test for arbitrary histories.
