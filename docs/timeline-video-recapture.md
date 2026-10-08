# Timeline video recapture

Capture can establish that a post contains a video while retaining only its
poster or blob-stream metadata. This is `unresolved_video`, distinct from
`missing_media` (no captured media) and `playback_error` (a saved inline URL failed).

The Timeline shows **Recapture video** for explicit video evidence without a
source-trusted inline playback URL. Rendering does not start a capture. Clicking
the button requests one background capture of the same native post through the
existing collector, ownership, and source-access controls. It never silently
opens a foreground window or closes user windows. The existing foreground option
still requires an unavailable background attempt with the same reason.

The API accepts `reason: "unresolved_video"` for X, Instagram, LinkedIn, and
Facebook. Image-only posts and posts already containing valid inline playback
do not qualify. Recapture succeeds only when the requested native post supplies
a source-trusted inline video URL; images and posters alone remain unavailable.
An empty unsuccessful capture preserves the saved poster and video expectation.
This verifies captured URL evidence, not decoder playback or future CDN validity.

X now declares playback-error recovery, using the existing serial, bounded UI
queue. Recovery requires progressive MP4 under the trusted `video.twimg.com`
paths. An error recovery must replace the failed URL rather than reuse it.

In the shared X resolver's explicit MP4 mode, the bounded scan collects posters
and progressive variants before applying the media output count. Matched videos
take output slots first, using the same Tweet ownership and media asset identity
checks. HLS variants do not consume MP4 output slots. The resolver's default
non-MP4 behavior is preserved.

Validation covers UI click/poll behavior, API-to-store recapture, poster-only
results, unsafe/foreign media, reason and foreground gates, and the shared
resolver in Bridge and headless. Sidecar must be rebuilt/restarted to activate
the embedded UI and staged worker. Reload AkuBridge for extension-side changes.
