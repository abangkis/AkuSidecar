# Timeline scrolling investigation — 2026-11-14

## Purpose and status

Record four bounded investigations into intermittent Timeline scrolling hesitation
so future work can resume without repeating completed comparisons or treating
unproven explanations as confirmed causes. The investigation remains open.

This is a historical observation record, not a performance benchmark, remediation
plan approval, or claim that scrolling has been fixed. Dates use Asia/Jakarta;
raw evidence timestamps retain their original UTC representation.

## Scope and environment

- Tests used ordinary Google Chrome through its extension. The dedicated
  AkuBrowser window could not be qualified through native Computer Use.
- The live comparisons retained 12 Timeline cards and used the same first three
  posts listed below. Viewport dimensions varied between checks and are recorded
  separately; results are not cross-check frame-rate benchmarks.
- No application source code or Settings were changed. Source/Brief presentation
  changes were temporary and restored. The isolated fixture was separate from
  the daily application.
- No application trace recorder, Performance recording, frame sampler, or
  persistent telemetry was enabled. Temporary rendering overlays were disabled
  after inspection.
- Existing unrelated working-tree changes were preserved.

Reference posts:

1. Derya: <https://x.com/DeryaTR_/status/2121540354537320655>
2. 晨空: <https://x.com/don_tech1/status/2121060480714469728>
3. uefaeuro: <https://www.instagram.com/reel/DfsE4j3txh4/>

## Findings at a glance

| Check | Verified result | Remaining uncertainty | SI stage status |
| --- | --- | --- | --- |
| 01 — Input methods | Wheel-style input and Page Down/Page Up moved the same retained segment; the owner reported manual scrollbar dragging felt smooth. | Wheel versus Page Down perceived smoothness, hardware latency, and reproduction of the intermittent symptom were not established. | active |
| 02 — Source versus Brief | Same-post comparison completed; the owner reported both felt the same. Presentation and reading anchor were restored. | This does not exclude media/card complexity on another workload or quantify GPU savings. | closed |
| 03 — Positioning animations | Isolated A/B replay retained identical workload and settled control geometry. | The owner did not observe the replay; no perceptual or performance improvement was established. | active |
| 04 — Rendering overlays | Selected stills showed localized control repaint and frame-rate estimates; overlays were removed. | The actual hesitation was not independently confirmed during these samples; dropped frames and root cause were not quantified. | active |

## 01 — Compare input methods

At viewport 1920 × 945, wheel-style input through the extension moved two viewport
pages to scrollY 1890 and returned to zero. Page Down moved 826 pixels per step
(826, then 1652); two Page Up inputs returned to zero. Card count, card heights,
and document height remained stable.

Automated scrollbar dragging selected navigation text rather than the scrollbar
thumb. This was an automation targeting failure and is not evidence of a broken
application scrollbar. The selection was cleared. The owner performed the manual
drag and reported: **“terasa lancar”**.

Functional input response was verified. Equal animation smoothness, physical
mouse-wheel latency, and an input-specific cause were not verified. Console
warning/error output was empty. Evidence for this check is in the current chat's
2026-11-14 tool observations and owner feedback; no standalone result file was
saved.

## 02 — Compare Source and Brief

The same three posts were compared at viewport 1920 × 889 with 12 retained cards.

| Post | Source height (px) | Brief height (px) | DOM descendants in either presentation |
| --- | ---: | ---: | ---: |
| Derya | 869 | 398 | 82 |
| 晨空 | 428 | 423 | 68 |
| uefaeuro | 794 | 348 | 78 |

Source displayed five images/avatars across these cards; Brief displayed none.
DOM descendant counts remained unchanged: Brief hides the media presentation
rather than removing the entire alternate DOM representation.

Both comparisons ended at the matching third-post lower boundary, not an identical
absolute scroll position: Source scrollY 2036 and Brief scrollY 1113, with the
third card bottom at viewport coordinate 889. The owner reported:
**“Terasa sama”**.

All three cards were restored to Source with their original heights. The previous
reading anchor was restored to scrollY 1902, with the second card top at -215
pixels. Console warning/error output was empty.

This closes the bounded comparison only. It does not establish that media cannot
cause hesitation elsewhere, that Brief reduces GPU work, or that a fix is needed
in either presentation.

Evidence: `C:/WorkspaceCodex/AkuWorkspace/SharedTemp/source-brief-scroll-20261114-1794669986266/`
contains `source.jpg` and `brief.jpg`.

## 03 — Compare positioning animations in an isolated fixture

The fixture used 76 hash-matching copied product JavaScript/CSS assets, including
the real Timeline rendering, context, and frame-task-queue functions. It contained
12 synthetic cards with heights 869/428/794 pixels repeated four times and eight
loaded static SVG posters. Page height was 9388 pixels in both modes.

- Mode A retained the product's positional transitions.
- Mode B removed the targeted top/left/bottom/transform transitions while
  preserving opacity, visibility, and color transitions, position calculations,
  passive scroll handling, and frame coalescing.

Both modes completed the same ten-waypoint programmatic scroll sweep, spaced
700 ms apart. Card HTML, heights, and settled control geometry were identical.
Console warning/error output was empty. Asset verification found no differences
between the 76 copied assets and their checked source inputs at test time.

The baseline already removes the Back to top bottom transition while following
a batch boundary; its return movement is a separate behavior. Other observed
positional transitions included AI Signals top/left and Related context transform.

The owner reported **“Belum sempat mengamati”**. Technical equivalence therefore
does not establish smoother scrolling, better FPS, lower GPU cost, or improved
hardware latency. Coverage excludes open drawers, background refresh, and active
video playback. The fixture made no application API, model, Settings, or storage
calls; its CSP blocked network connections. The fixture server and test tab were
closed after testing.

Evidence: `C:/WorkspaceCodex/AkuWorkspace/SharedTemp/side-control-positioning-20261114/`
contains `README.md`, `browser-results.json`, `asset-manifest.json`, `verify.mjs`,
the fixture/server sources, copied assets, and `mode-a-on.jpg` / `mode-b-off.jpg`.
The saved fixture URL/port is historical, not a running endpoint.

## 04 — Observe Frame rendering stats and Paint flashing

The owner manually enabled the two Chrome DevTools Rendering overlays. Docked
DevTools reduced the viewport to 1365 × 889. The Timeline retained 12 cards.

| Selected still | scrollY | HUD frame-rate estimate | HUD GPU memory used | Paint observation |
| --- | ---: | ---: | ---: | --- |
| Down | 889 | 115.2 fps | 14.7 MB | Green rectangles around AI Signals |
| Down | 1778 | 108.0 fps | 9.1 MB | Green rectangle around Related context |
| Up | 889 | 98.4 fps | 13.3 MB | No broad green repaint visible |

GPU raster was on. Blue/yellow frame-chart segments were visible; yellow denotes
partially presented frames in Chrome's overlay. Dropped-frame counts were not
quantified. These estimates are selected still snapshots, not an average, a trace,
or a benchmark. The display refresh target was not established. HUD memory values
do not establish total GPU load or saturation.

No full-card or full-viewport green repaint was visible in these selected stills.
This does not exclude repaint or stalls at other moments. Localized control repaint
is a candidate area for investigation, not an established cause. The intermittent
hesitation was not independently confirmed during these samples.

The owner confirmed both overlay checkboxes were off. The final screenshot showed
no HUD; viewport 1920 × 889 and scrollY 0 were restored before the test tab closed.
Closing DevTools itself was not independently verified.

Evidence: `C:/WorkspaceCodex/AkuWorkspace/SharedTemp/timeline-rendering-overlays-20261114-1794671759153/`
contains `observations.json`, `scroll-ai-signals.jpg`,
`scroll-related-context.jpg`, and `overlays-off.jpg`.

Overlay interpretation reference:
[Chrome DevTools rendering performance documentation](https://developer.chrome.com/docs/devtools/rendering/performance).

## Roadmap application receipt

The owner approved the summary through SI's human web-review flow. Readback
verified status `applied`, decision `approve`, and matching proposal/decision/
receipt digests.

- Registry: `si-local`
- Project: `5e026b8f-dcda-26f6-d364-a71485f84b8d` (AkuBrowser)
- Board: `b020b48a-b807-43b6-8ddc-49732ba42d56`
- Proposal: `a3a3fa37-3ec1-4b7a-9a8f-33815ed5f643`
- Applied board revision: **122 → 127**
- Track: `46841f3a-9a53-4017-97cc-4e0394fdb422`, **active**

| Stage | Node ID | Applied state |
| --- | --- | --- |
| 01 — Input methods | `2c9a1064-afbb-48c8-a5cf-9122581c07ab` | active |
| 02 — Source/Brief | `e7bdcecd-e89d-468f-9e15-ab9f812c90c7` | closed |
| 03 — Positioning animations | `05685dc6-1b95-4abc-b93f-985c98a77b35` | active |
| 04 — Rendering overlays | `299a6d83-14b8-45ae-9417-ce4b01fe4b80` | active |

Detailed evidence and limitations were recorded in the state-change reasons/history
of the existing nodes. No duplicate stages were added. These are historical applied
states; read the current SI board before future tracking changes.

## How to revisit

1. Read the current SI board and record the active application revision, Chrome
   version, viewport, display refresh target, retained post identities, and whether
   background updates or video playback are active. Do not assume this historical
   workload still represents the current runtime.
2. Complete the owner-observed A/B positioning-animation comparison in the isolated
   fixture. Preserve the same workload and distinguish more direct control motion
   from smoother page scrolling. Verify saved asset provenance before reusing it.
3. Complete the perceived wheel/Page Down/scrollbar comparison on the same segment,
   explicitly recording whether the hesitation actually occurs for each input.
4. If the symptom recurs, observe rendering overlays during that occurrence and
   correlate visible paint/frame behavior with it. Disable both overlays afterward.
   Do not treat these historical snapshots as reproduction of the symptom.
5. Propose a narrowly scoped correction only after evidence identifies a plausible
   mechanism. Application changes, Settings changes, trace recording, and runtime
   activation require their own authorization.

The immediate recommendation is step 2, not an application change. No follow-up
execution is authorized merely by saving this record.

## Related implementation records

- [Timeline background refresh](timeline-background-refresh.md) describes the
  unchanged-response guard and fetch → stage → apply contract. Its implementation
  does not by itself establish the cause of the remaining hesitation.
- [Diagnostics and alpha.4 telemetry adoption](diagnostics-telemetry-alpha4-adoption.md)
  records the separate diagnostics delivery; this scrolling investigation did not
  enable its exporter.

Raw evidence remains in local SharedTemp directories and is not bundled into this
document. It may be removed independently; the tables and limitations above retain
the essential findings if those files are no longer available.
