# Product source structure audit — 2026-10-25

## Status synchronization (2026-11-01)

The original audit and its validation below are historical. Current source is
reviewed through `8dc3f5e`, including executable preparation `c303ed4`; see the [roadmap status index](roadmap-status.md)
for commit-to-receipt mappings. Cleanup is not provider qualification.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Unused wrappers | Removed in `f348b9b` | Scoped cleanup validation | Not recorded | Bounded removal; no whole-repository dead-code claim |
| Native factory / QA separation | `29ec2b4` | Equivalence and production package checks; later relevant provenance tests 148/148 | Not recorded | Production factory no longer belongs to the fixture workflow |
| Standalone owner harness | `cd8fd60` | Staging regression and Windows compile | Not recorded | Production transient-error dependency included; guards unchanged |
| Production shared helpers | `2ba01bb` | Source equivalence, production dependency graphs and Windows checks | Not recorded | Callers moved out of fixture-folder helpers |
| Engine work, scheduler, execution and finalization | `202ecbb`, `056b53b`, `d897130`, `ae1a535` | Respective equivalence/regression receipts; finalization adds seven Store checks and eight Windows artifacts | Not recorded | Scoped refactors reviewed; `e.mu`, schedule lock, start-once/wake, side effects and error contracts retained |
| Unused deletion alias | `21ac9ad` | G10 remainder equivalence, full Store/HTTP regressions and eight Windows artifacts | Not performed | Bounded internal alias removal; tombstone/recapture guards retained |
| Reasoning config decoder | `d82f53b` | Exact decoder relocation; corrected repository smoke-test context; default/dev config/application suites each 46 pass, two skips, zero failures; eight Windows artifacts | Not performed | Both supported config shapes and strict validation retained; original baseline failure kept as historical evidence |
| Provider candidate failure cleanup | `7d00d9c` | Six failure scenarios reproduced before the fix; default 526 pass/38 skip, dev 1,199 pass/48 skip, zero failures; eight Windows artifacts | Not performed | Candidate disposed on planning failure; original errors and success ownership preserved |
| Settings execution file boundary | `02e063e` | Unchanged 120-line method and remaining source; same default/dev pass/skip counts; four build-tag graphs and eight Windows artifacts | Not performed | First E01 file boundary reviewed/committed; deeper Settings decomposition remains proposed |
| Pure profile-map merge | `aea3867` | Characterization and merge/validation tests pass; the initial unrelated cancellation failure remains historical; latest combined result below passes | Not performed | Precedence and independent map allocation preserved; normalization remains in the caller |
| Reader cancellation validation detour | `edb246d` | Deterministic regression and 300 stress passes; recorded default 547 pass/38 skip, dev 1,220 pass/48 skip, zero failures; three profile contracts, four build-tag graphs and eight Windows artifacts | Not performed | Existing waiter cancellation takes precedence over stopped reporting; new closed-transport requests remain rejected; protocol and guards retained |
| Executable-change preparation | `c303ed4` | 20 characterization results pass before/after extraction; exact helper/caller equivalence. Initial native-pilot failures remain historical; latest combined evidence below passes | Not performed | Scoped helper/test review complete and committed; operation/work lock scopes, no-op and error contracts, discovery before persistence and activation before provider swap retained |
| Offline Node startup timing detour | `8dc3f5e` | Latest default 568 pass/38 skip, dev 1,241 pass/48 skip, zero failures; 98 Node tests, 200 repeated integration iterations, four build-tag graphs and eight Windows artifacts; [review](../runtime/qa/settings-executable-review-20261101/review.json) matches source/log/artifact and staging hashes | Not performed | Scoped fixture/test correction committed; original clock, timestamps, protocol, document, cancellation, freshness and deadline guards retained. This bounded fixture scheduling does not establish universal clock synchronization or live-source acceptance |

No code change, activation or trial is performed by this status update. Existing
test receipts are reused at their verified source/artifact scope, not rerun.

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

The [comprehensive cleanup roadmap](code-cleanup-roadmap.md) now organizes
consumer/compatibility inventory, bounded retirement, responsibility extraction
and package/integration acceptance as CL-1 through CL-8. This audit remains the
historical finding record; the roadmap preserves completed work and requires
approval of each concrete implementation increment.

The original audit measured `app.js` at approximately 9,900 lines, `engine.go`
at 3,123, the native probe driver at 2,439, and Bridge `service-worker.js` at
2,524. These are historical measurements, not current counts: production
factory/helper and Engine extractions have since changed the source layout.
Size identifies review targets, not evidence that code is removable.
Follow-up extraction should move complete
features with their tests: Timeline rendering, Settings orchestration, then
probe mode dispatch. Avoid moving all pages at once or hiding authority behind a
generic helper merely to reduce line count.

The earlier fixture-factory coupling has been addressed by `29ec2b4` and
`2ba01bb`; it is no longer the outstanding blocker stated by this audit.
The backend Settings orchestration file boundary is now established in
`settings_execution.go`, and the profile-map merge now belongs to the pure
`settings_profiles.go` helper. Remaining validation, persistence and runtime
effects still remain in the same method. Remaining candidates include feature-level
Timeline/frontend Settings orchestration, deeper backend Settings decomposition,
probe mode dispatch, HTTP capture-service boundaries and a broader inventory of
legacy/compatibility consumers. They are proposals requiring separate bounded
analysis/approval, not a deletion list or authorization to continue refactoring.
Native IG qualification and operational acceptance remain open for their own
document-continuity reasons, not because the production factory is still in QA.

## Validation

The following results belong to the original 2026-10-25 audit. Later cleanup
validation is linked in the 2026-11-01 matrix and status index above.

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
