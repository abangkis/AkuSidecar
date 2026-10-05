# Timeline scroll timing trace (development)

This diagnostic is opt-in and disabled on startup. It does not change collection mode or collect source content.

1. Open Settings → Diagnostics → Trace scroll for 30 seconds.
2. The app opens Timeline. Scroll through the sections that feel uneven for 30 seconds. Avoid switching apps during recording; hiding the page stops the trace.
3. Recording stops automatically, or use Stop trace. The report is uploaded to `/api/diagnostics/ui-performance`.
4. Codex can read the latest report from that endpoint. Download last trace saves the same timing report manually, including when the upload fails.

The endpoint is available only in development. It holds one report in runtime memory, replaces it with the next recording and loses it on restart. There is no database write or automatic file export. Uploaded JSON rejects unknown fields, arbitrary names and oversized reports. Events and frame samples are capped and dropped samples are counted.

The report contains requestAnimationFrame gaps, long tasks, long animation frames, layout shifts, image load timing and synchronous costs for Timeline rendering and scroll callbacks. Unsupported observer APIs are explicitly marked unavailable. Script URLs, DOM nodes, attribution, post text, media URLs and credentials are never retained.

Frame callback gaps are scheduling measurements, not rendered FPS. Long animation frame events expose main-thread timing; they do not establish GPU, compositor, paint or image decode costs. Layout shifts with recent input are retained separately. This tool identifies timing correlations; diagnosing remaining jank requires a recording of the real user interaction. The trace itself adds bounded observer and sampling overhead. It performs no per-frame UI updates.
