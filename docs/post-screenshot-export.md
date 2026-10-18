# Full post screenshots

Each expanded Timeline post has a persistent vertical `⋮` menu beside the
presentation toggle. The menu contains **Screenshot full post**. A modal previews
the generated PNG, with **Save PNG** and **Copy image**. Multiple parts have
individual numbered downloads and a part selector; copying copies the selected
part. Clipboard denial leaves the PNG download available and displays an error.
Escape/outside-click dismiss the menu; Escape/Close dismiss the preview and abort
pending work. Text expansion, media playback and Timeline scroll are not changed.

The export uses captured source evidence, independent of the current Brief/Source
toggle and collapsed text. It includes source identity, full captured text, quote,
ordered media and attachment previews. Carousels become vertical media; videos use
posters, never playback or a newly captured video frame. Controls, AI assessments,
Related Context and private memory are excluded. Missing images remain explicitly
marked. Text absent from the stored capture cannot be reconstructed by exporting.

## Local compositor

`POST /api/post-screenshots` accepts only `{timelineId, theme}` (`dark` or `light`).
The server reads the persisted Timeline item rather than accepting HTML or caller
supplied post content. Existing loopback/origin checks apply. One export runs at a
time; an overlapping request returns 429. Nothing is written to Timeline or Store.

The renderer uses the packaged, checksum-pinned Node binary and a discovered local
Chromium executable. Each invocation starts a hidden headless browser with a new
profile beneath `runtime/post-screenshots/export-*`, a private debugging pipe and
no extension, cookie jar, account login or capture-session reuse. It is started
inside the existing owned-process boundary; Windows assigns the Job before Node
executes, and the owned tree is drained before removing its files. An unverified
startup/cleanup retains the exact private root rather than deleting a live profile.
No background browser is kept between exports. Missing local dependencies return
an actionable export failure; this feature never installs a runtime or signs in.

The server retrieves only raster images from the existing social-media CDN host
families, without browser cookies, Authorization or Referer. It bypasses ambient
proxies, validates every redirect and pins connections to resolved public IPs.
SVG, HTML, private addresses and arbitrary attachment targets are not fetched.
The generated document contains escaped captured strings, trusted app CSS and
embedded data images. Its CSP forbids scripts/navigation resources, and the target
blocks external network requests. Image decoding finishes before measuring height.

## Completeness and resource limits

The output width is 800 pixels. Height is measured after layout; up to 8,192 pixels
fits in one PNG. Taller documents are captured as contiguous numbered slices, with
no vertical gaps. All captured media positions remain represented. Media may have
an unavailable placeholder after the download limits (64 assets, 8 MiB each,
32 MiB total, 30-second phase deadline); authentication is never used as a fallback.
Individual decoded raster images are limited to 32 million pixels.

The entire operation has a 90-second deadline. A document taller than 262,144 pixels
or output above 40 MiB of base64 fails as a whole; it never returns a partial export
as complete. These are explicit safety bounds, not an unlimited single-image
promise. PNGs contain the captured post presentation, not source-authenticated
screenshots of the native social site.

## Regression commands

From the repository root:

```powershell
.\scripts\test-post-screenshot.ps1
.\scripts\test-post-screenshot.ps1 -VerifyChrome
```

The first runs Node presentation/interaction tests and HTTP/media regressions.
The second also launches only fresh offline Chrome fixtures using synthetic text
and raster images. It validates actual PNG dimensions, long-post segmentation and
image pixels. Logs and synthetic visual artifacts remain under the emitted
`SCREENSHOT_QA_ROOT`. Build caches and temporary files are project-local; module
downloads are disabled. No real authentication, user runtime, registry or paused
session-integration setting is involved.

The Chrome fixture defaults to the existing Windows Chrome installation; pass
`-ChromePath` to select another fixture executable. This test override does not
change product browser discovery or save configuration. The actual application
uses its existing pinned Chromium discovery paths/environment.

Validated on 2026-10-18: seven focused Node tests, seven Go tests (including four
Chrome subcases), the broader web suite (215 tests), the full HTTP package and the
ordinary Sidecar build passed. Chrome checked whole-post output, contiguous
multipart output with decoded PNG dimensions and synthetic image pixels,
desktop/mobile menu/preview/close, compact toolbar, hidden single-part navigation,
and canceled-renderer cleanup. The UI driver uses the real menu/preview module
with a synthetic response; HTTP storage tests and compositor tests separately
exercise their respective boundaries. It does not prove an activated live product
journey or overwrite the user's operating-system clipboard. Clipboard success and
denial are covered through injected browser APIs. Six protected runtime/config
hashes remained unchanged. Final fixture evidence:
`runtime/qa/post-screenshot/e97bba62b53943309f5659a3dd908fac/`.
