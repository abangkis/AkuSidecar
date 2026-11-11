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
15 seconds. Playlist bodies are limited to 64 KiB and each media resource to
12 MiB. The per-resource allowance covers the measured 9,606,473-byte Matt video
fragment; it does not increase the session allowance below.
One cumulative 64 MiB budget covers audio, video, playlists, quality switches,
retries, and seeks for the whole session. Switching quality never resets it.
This is a byte allowance for application reads, not a precise wire-traffic or
decoded-memory measurement. Repeated seeking or long, high-bitrate videos can
exhaust it and trigger native fallback. Required audio failure also stops video.

## Optional bounded UHD pilot

The same production Timeline caller can admit an optional UHD level. Normal
sessions retain 12 MiB per resource and 64 MiB cumulative reads. Before the HLS
engine sees a UHD level, the generic admission module requires positively known
3840×2160 dimensions, a frame rate greater than zero and no higher than 30 fps,
explicit SDR, peak `BANDWIDTH` no higher than 35 Mbps, AVC plus AAC, and an actual
MediaCapabilities result with `supported` and `smooth` both true. Unknown
capabilities exclude UHD. A `powerEfficient: false` result alone does not exclude
an otherwise suitable level. 4K60, HDR, DCI 4K and other codec families are outside
this pilot; valid lower variants remain available when UHD is excluded. The pilot
also requires a non-UHD fallback variant, including after the engine filters
unsupported codecs. UHD-only masters use native-post fallback; they do not start
UHD under Auto on a small display or introduce an additional waiting UI state.

Preflight checks the candidate video playlist and its exact audio group before
any large video resource is allowed. Video segments must be no longer than six
seconds and the whole video no longer than 20 seconds. Audio alone may round up
by 50 ms per segment and in total, accommodating AAC frame boundaries. Preflight
tries at most four metadata-eligible candidates within one shared five-second
deadline and deduplicates exact playlist reads. Source authorization and playlist
validation run first. All preflight reads consume the same cumulative allowance;
cancellation prevents late admission. A qualified playlist cannot change between
preflight and the engine's read.

Only an exact, admitted UHD main-video segment request receives a 32 MiB resource
limit and upgrades that session's existing cumulative allowance to 128 MiB. This
adds allowance without resetting bytes already consumed, and happens at most
once. Shared audio, HD declarations reusing the playlist, and initialization
segments cannot trigger the upgrade. Audio retains its normal resource limit.
The pilot uses a six-second back buffer; normal playback retains ten seconds.
The 64 KiB playlist limit, 15-second timeout, source trust rules, no credentials,
no redirects and whole-session audio-failure fallback remain in force.

Auto's future level ceiling follows the rendered video size multiplied by the
actual device pixel ratio. Resize, fullscreen and DPR changes update only
`autoLevelCapping`, avoiding the built-in size/FPS controllers' eager buffer
flush. Manual selection still uses conservative `loadLevel` and can choose an
admitted UHD level independently of the Auto display ceiling. Observers and
listeners are removed on disposal. In a sparse quality ladder, UHD is not the
nearest-covering choice for a smaller display: Auto opens its UHD ceiling only
when the actual rendered size times DPR reaches UHD. This is a short UHD pilot, not a data-quota
policy for arbitrary long 4K videos or proof of 4K60/HDR compatibility.

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
