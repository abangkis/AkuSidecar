# Product source structure audit — 2026-10-25

## Scope and method

Read-only inventory covers tracked source in AkuSidecar, AkuBridge and AkuBrowser.
Focused semantic review covers native capture admission, private transport,
retained-QA composition and embedded page navigation. This is not a claim that
every function in all three repositories received a line-by-line audit.

Run `runtime/dev/headless-worker/node.exe scripts/audit-source-structure.mjs` to
reproduce file sizes, exact duplicates and unreferenced local page-function
candidates. The initial tracked inventory counted 923 Sidecar, 151 Bridge and
68 Browser source files (tests included; new untracked implementation excluded).
No byte-identical source files over 100 bytes were found. Profiles, databases,
build output, dependency trees and unrelated experiments are excluded.

## Findings addressed

| Finding | Correction and responsibility |
| --- | --- |
| Private pilot bypassed ordinary pacing and persistent challenge pause | Store owns one transactional claim boundary and checks pause again before result acceptance. Engine passes policy and handles cancellation. |
| Optional pacing could allow ordinary work to overlap a pilot | Pilot occupancy is mandatory independently of the spacing toggle; other sources and Auto Update preferences are unchanged. |
| Native capture errors were processed as startup failures | Closed capture challenge categories survive both Node and Windows transport; startup evidence parsing remains independent. |
| Capture observation envelope could exceed the metadata reply cap | Capture-only output capacity is bounded separately; the 64 KiB startup control limit stays intact. |
| Failure completion timestamps bypassed the injected Store clock | Capture failure history uses `Store.Now`, so backoff and tests share one clock. |
| A failed challenge-metadata lookup exposed a raw database error instead of the existing closed category | The source-pause guard retains `capture pause lookup failed`; full Store regression coverage checks this contract. |
| Startup QA only proved an in-memory result | Shared startup recipe accepts a separate capture continuation that compiles actual Engine/Store sources into the prepared test. No duplicated login/seed recipe. |
| Page panel, navigation and accessibility mappings were repeated | `page-navigation.js` owns the page registry, normalization, visibility and focus. `app.js` retains loading, teardown and orchestration. |
| Onboarding/calibration omitted the Living Topics panel from their hide lists | Setup-page transitions now use the same registry and hide every ordinary page, including Living Topics. |
| Historical blockers read like current work | `concurrency-dev-readiness.md` is the current checkpoint; older roadmap entries are explicitly historical. |

Removed residue consists of duplicated page toggles/heading maps and stale current
status prose. No whole source file was deleted merely because it was old or had a
similar name. Fixture, compatibility and migration tests remain referenced by
canonical helpers and are retained. No unused local named function was found by
the narrow `app.js` reference scan; this is not whole-program dead-code proof.

## Boundaries to preserve

| Component | Responsibility |
| --- | --- |
| AkuBrowser | Packaging, launcher, deployment manifest |
| AkuBridge | DOM/source capture and browser extension transport |
| Sidecar page modules | Render state and user intent, never account authority |
| Engine / Coordinator | Dispatch, owner leases, cancellation and delivery |
| Store | Durable claim, pause, fencing, commit and replay semantics |
| Native startup owner | Account proof, fixed document/owner binding and private transport |
| QA helper | Explicit gates, source pinning, disposable DB and owned cleanup |

## Remaining structural debt

`app.js` is still approximately 9,900 lines, `engine.go` 3,123, the native probe
driver 2,439, and Bridge `service-worker.js` 2,524. Size identifies review targets,
not evidence that code is removable. Follow-up extraction should move complete
features with their tests: Timeline rendering, Settings orchestration, then
probe mode dispatch. Avoid moving all pages at once or hiding authority behind a
generic helper merely to reduce line count.

The main-dev provider remains unfinished because Windows/profile ownership is
currently tied to a fixture factory. Promoting that lifecycle is substantive
implementation, not deleting tags or adding a setting. Native QA acceptance and
explicit activation remain separate from this cleanup.

## Validation

- Full Store suite passed in default, development, fixture and combined builds.
- All 226 frontend tests passed, including the new page registry regressions.
- The full HTTP API suite passed with loopback fixture access; its embedded
  navigation contract now checks the extracted module.
- Native parser/worker offline acceptance passed eight Go result scenarios and
  four build-tag gates, including both challenge categories.
- Engine/Store synthetic acceptance passed 19 integration cases and four
  ordinary continuation regressions.
- Ordinary capture/reader/Auto Update regression selection passed.
- Default and combined fixture Sidecar builds passed.
- Retained capture preparation compiled successfully and the disabled test entry
  skipped before profile access.
- A later, separately authorized one-shot retained QA attempt rejected at the
  joint startup evidence deadline, before capture or QA database creation.
  Cleanup was verified, with no retry. See the current readiness checkpoint.
