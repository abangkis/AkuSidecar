# Timeline HLS component

The Timeline uses one HLS component for primary and quoted video. Source adapters
admit media and supply resource policies; the player does not contain source
names, CDN hosts, post identities, or asset-path rules.

## Responsibilities

- `timeline-hls-player.js` owns the hls.js session, Auto/manual quality state,
  readiness, errors, and disposal.
- `timeline-hls-loader.js` owns streaming fetch, cancellation, timing statistics,
  request limits, and a shared session byte budget.
- `timeline-hls-policies.js` maps captured source media to `{ url, policy }`.
- `x-hls-policy.js` admits matching X poster/master assets and confines subsequent
  requests to the same trusted video asset. Its current policy supports bounded,
  unencrypted VOD fMP4 playlists, including separate audio.
- `timeline-video-quality.js` renders the source-independent quality selector.
  `timeline-media.js` connects that control to the session and native fallback.

The official full ESM distribution is vendored locally. Version, archive
integrity, distribution checksum, and license are recorded in
`internal/httpapi/web/vendor/README.md`. Playback has no runtime library-CDN
dependency. Workers are disabled to preserve the existing CSP.

## Session contract

```js
const session = attachTimelineHls(video, {
  url,
  policy,
  onState({ qualities, selected, active, pending }) {
    // Render selected mode separately from the quality currently playing.
  },
  onError(error) {
    // The session has already been torn down. Offer the native-post fallback.
  },
});

await session.ready;
session.setQuality("1"); // A quality ID returned in qualities.
session.setQuality("auto");
session.dispose();
```

`ready` resolves when manifest quality options are available, not when all media
has downloaded. Quality IDs are opaque and belong to this session. Manual
selection persists until Auto is chosen. The conservative `loadLevel` API applies
the selection as new fragments load; previously buffered quality continues until
then, and the control reports the pending selection separately. It does not flush
buffered audio/video, recreate the video element, or reset playback time. The
forward buffer target is six seconds, with segment-duration tolerance; quality
changes are not instantaneous. Disposal is
idempotent and aborts outstanding requests; late events cannot update the UI.

A policy must implement `authorizeRequest(context)` and
`validatePlaylist(text, context)`. Authorization returns an allowed absolute URL
or throws, and runs before every request. Playlist validation runs before text
reaches the HLS engine. A new source needs its own explicit policy and registry
admission. Its production network destinations must also satisfy the server CSP;
the generic player does not add destinations or infer trust from a file suffix.

## Bounds and fallback

Requests omit credentials and referrers, reject redirects, and time out after
15 seconds. Playlist bodies are limited to 64 KiB and media resources to 8 MiB.
One cumulative 64 MiB budget covers audio, video, playlists, quality switches,
retries, and seeks for the whole session. Switching quality never resets it.
This is a byte allowance for application reads, not a precise wire-traffic or
decoded-memory measurement. Repeated seeking or long, high-bitrate videos can
exhaust it and trigger native fallback. Required audio failure also stops video.

## Verification

Unit tests cover source policies, immutable vendor identity, bounded fetch,
concurrent budget use, cancellation, and player/control lifecycle. Chromium
fixtures exercise real fMP4 audio/video bytes, manual low/high/Auto transitions,
actual Auto upshift and downshift under throttling, seeking, and disposal. A
separate injected non-X policy exercises the same generic player. Quoted-card
tests verify that the shared media renderer receives quote-owned media.

Browser fixtures require `AKUSIDECAR_HLS_FIXTURE_ROOT`,
`AKUSIDECAR_HLS_FIXTURE_CHROME`, `AKUSIDECAR_HLS_CDN_CAPTURE_ROOT`, and
`AKUSIDECAR_HLS_QUALITY_CAPTURE_ROOT`. Without these, browser scenarios skip;
ordinary Node success alone does not establish browser playback acceptance.
