# Instagram video recovery

Inline playback stores a source-owned MP4 URL, not a local video copy. Instagram
CDN URLs can expire after capture. A poster with a native-post link remains the
fallback when playback or capture cannot recover the video.

## Playback recovery

A playback error after the user's Play action queues a background recapture.
The UI waits for the update session, capture leases, another recapture, native
reader ownership, calibration, and runtime readiness before dispatching it.
The existing two-second runtime poll and UI state transitions drain the queue.

The queue holds at most 64 post/URL requests and processes one job at a time.
It deduplicates the failed URL, drops requests whose Timeline evidence has been
removed or refreshed, and allows a newly refreshed URL to have its own attempt.
The queue is scoped to the current UI page; reloading it clears pending requests.
Recovery does not automatically play the refreshed video or close user windows.

## Headless capture retry and diagnostics

An Instagram post with an expected video and no playback URL can receive one
additional resolver attempt after an empty exact-candidate `no_match` result.
There are at most four retried candidates per resolver invocation, with a short
hydration wait inside the existing capture deadline. If the first resolver
explicitly reports a scan bound, retry may use its existing larger safe limits.
Foreign candidates, ambiguous identity, unsafe media, and unavailable resolvers
do not qualify for retry. Exact native identity and the CDN allowlist still apply.

Per-post evidence and snapshot coverage retain bounded resolver counters and
attempt/retry outcomes. Unknown counters stay null; known zero stays zero.
Diagnostics do not retain source JSON, signed media URLs, or exception payloads.
This does not certify that a captured URL will remain playable indefinitely.

Validation uses the actual UI recovery functions with a controlled API transport,
queue concurrency cases, and the headless resolver/observation regression suites.
Runtime activation requires the normal Sidecar rebuild and restart.
