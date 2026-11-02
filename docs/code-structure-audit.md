# Product source structure audit — 2026-10-25

## Status synchronization (2026-11-02)

The original audit and its validation below are historical. Current source is
reviewed through `eb05ac8`, including Engine reasoning results, process recovery, observation contracts, session admission and AI detection/media provenance, Engine/HTTP media-recapture, Engine/HTTP Bridge command, Bridge status and diagnostics feature separation; see the [roadmap status index](roadmap-status.md)
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
| Offline Node startup timing detour | `8dc3f5e` | Recorded default 568 pass/38 skip, dev 1,241 pass/48 skip, zero failures; 98 Node tests, 200 repeated integration iterations, four build-tag graphs and eight Windows artifacts; [review](../runtime/qa/settings-executable-review-20261101/review.json) matches source/log/artifact and staging hashes | Not performed | Scoped fixture/test correction committed; original clock, timestamps, protocol, document, cancellation, freshness and deadline guards retained. This bounded fixture scheduling does not establish universal clock synchronization or live-source acceptance |
| Diagnostics export feature | `1068e35` | Identical characterization passes 17 results before/after relocation; recorded default 585 pass/38 skip, dev 1,258 pass/48 skip, zero failures; four build-tag graphs and eight Windows artifacts; [review](../runtime/qa/diagnostics-export-review-20261101/review.json) verifies unchanged declarations/remainder and source/log/artifact hashes | Not performed | Scoped source/test review complete and committed; ten declarations moved together, receiver, Store ownership, read/error order, JSON/protocol/guard behavior retained; default-range zero usage remains existing behavior |
| Bridge status/compatibility feature | `7b2d0d8` | Identical characterization passes 38 results before/after relocation; recorded default 623 pass/38 skip, dev 1,296 pass/48 skip, zero failures; four build-tag graphs and eight Windows artifacts; [review](../runtime/qa/bridge-status-review-20261101/review.json) verifies source/log/artifact hashes and identity-checker anchors | Not performed | Scoped source/test review complete and committed; nine functions/two types moved unchanged; constants, action catalog, full read-lock scope, heartbeat pointer guard and revocation/recovery/reload/scheduler order retained. This is Sidecar CL-3.2, not extension CL-6 completion |
| HTTP Bridge command feature | `7cb712b` | Identical characterization passes 53 results before/after relocation; recorded default 676 pass/38 skip, dev 1,349 pass/48 skip, zero failures; four HTTP build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/bridge-commands-review-20261102/review.json) and [commit readback](../runtime/qa/bridge-commands-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review complete and committed; four Server methods in `bridge_commands.go`, with route predicates/normalization, context, authentication, protocol, guards, status/error mappings and native trace order retained. This closes one CL-3.3 feature boundary, not the whole stage |
| HTTP Bridge media-recapture feature | `be9fcd2` | Identical characterization passes 54 results before/after relocation; recorded default 730 pass/38 skip, dev 1,403 pass/48 skip, zero failures; four HTTP build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/bridge-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/bridge-media-recapture-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review complete and committed; three Server methods in `bridge_media_recapture.go`; route/context/authentication/protocol/guards and error/effect order unchanged. Queued/claimed failure acceptance, stale-claim side effect and commit-before-response behavior characterized; CL-3 remains partial |
| Engine media-recapture feature | `c614512` | Identical characterization passes 34 default/35 dev results before/after relocation; recorded default 764 pass/38 skip, dev 1,438 pass/48 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/engine-media-recapture-review-20261102/final-readback.json) verify method/remainder equivalence and source/log/artifact/commit identity | Not performed | Scoped source/test review complete and committed; ten methods in `media_recapture.go` retain receiver, locks, context, primary-error and side-effect order, protocol and guards. Store, capture runtime and HTTP unchanged; CL-3 remains partial |
| Engine Bridge command feature | `3e13434` | Identical characterization passes 37 results in both default/dev before and after relocation; recorded default 801 pass/38 skip, dev 1,475 pass/48 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-bridge-commands-review-20261102/review.json) and [commit readback](../runtime/qa/engine-bridge-commands-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Seven methods in Engine `bridge_commands.go` retain receiver, lock/context scopes, recovery and persistence/scheduling order, errors, protocol and guards. Shared normalization/validation, Store, capture runtime, heartbeat and scheduler unchanged; CL-3 remains partial |
| Engine media-provenance feature | `8d7f7b6` | Identical characterization passes 24 results in both default/dev before/after relocation; six-package Inspector/Store/Engine/HTTP/config/application suites pass default 1,214/43 skip and dev 2,040/55 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-media-provenance-review-20261102/review.json) and [commit readback](../runtime/qa/engine-media-provenance-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Five Engine methods in `media_provenance.go` retain receiver, locks, request/background contexts, queue/worker/result order, protocol and guards. Exactly two gofmt-only alignment changes remain in `engine.go`; Store, Inspector, worker algorithm and authority unchanged; CL-3 remains partial |
| Engine AI-detection feature | `b84a62d` | Identical characterization passes 32 results in both default/dev before/after relocation; seven-package Detector/Inspector/Store/Engine/HTTP/config/application suites pass default 1,270/45 skip and dev 2,096/57 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-ai-detection-review-20261102/review.json) and [commit readback](../runtime/qa/engine-ai-detection-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Five methods in `ai_detection.go` retain receiver, locks, request/background contexts, admission/profile/job/result order, errors, protocol, guards and AI/media-provenance cancellation. Only standalone setter alignment normalized; Engine remainder byte-identical; CL-3 remains partial |
| Engine session-admission feature | `4bf0731` | Identical characterization passes 31 default/34 dev parent/subtest results before/after relocation; seven-package suites pass default 1,301/45 skip and dev 2,130/57 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-session-admission-review-20261102/review.json) and [commit readback](../runtime/qa/engine-session-admission-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Three Engine methods and one error helper in `session_admission.go` retain signatures/bodies, caller/receiver ownership, lock/context/error/effect order, source/policy/lease behavior, protocol, guards and AI/provenance cancellation; Engine remainder byte-identical; CL-3 remains partial |
| Engine observation contracts | `d9c3025` | Identical characterization passes 61 default/74 dev parent/subtest results before/after relocation; eight-package suites pass default 1,409/45 skip and dev 2,251/57 skip, zero failures; four build-tag graphs and 12 Windows amd64 artifacts; [validation](../runtime/qa/engine-observations-20261102/validation.json), [review](../runtime/qa/engine-observations-review-20261102/review.json) and [commit readback](../runtime/qa/engine-observations-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three free functions move unchanged into existing observations.go; reconciliation, five production callers, normalization/validation/error order, native identity rejection, protocol and guards retained. CL-3 remains partial; operational acceptance remains open |
| Engine process-error and usage-limit recovery | `5b39fa4` | Identical characterization passes 53 default/dev parent/subtest results before/after relocation; nine-package suites pass default 1,564/48 skip and dev 2,406/60 skip, zero failures; four build-tag graphs and 12 Windows amd64 artifacts; [validation](../runtime/qa/engine-process-recovery-20261102/validation.json), [review](../runtime/qa/engine-process-recovery-review-20261102/review.json) and [commit readback](../runtime/qa/engine-process-recovery-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Seven declarations in process_recovery.go retain three Engine receivers, exact bodies and Engine remainder, locks, caller contexts, persistence/error/effect order, accepted-capture/queued-peer guards, protocol and nonblocking wake. CL-3 remains partial; operational acceptance remains open |
| Engine reasoning-result acceptance and Timeline mapping | `eb05ac8` | Identical characterization passes 52 default/dev parent/subtest results before/after relocation; ten-package suites pass default 1,622/48 skip and dev 2,464/60 skip, zero failures; four build-tag graphs and 12 Windows amd64 artifacts; [validation](../runtime/qa/engine-reasoning-result-20261102/validation.json), [review](../runtime/qa/engine-reasoning-result-review-20261102/review.json) and [commit readback](../runtime/qa/engine-reasoning-result-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three free helpers in reasoning_result.go retain exact signatures/bodies; Engine remainder identical except unused fmt import removal. Caller binding/validation/selection/completion order, error/coverage semantics, Store ownership, protocol and guards unchanged. CL-3 remains partial; operational acceptance remains open |

No code change, activation or trial is performed by this status update. Existing
test receipts are reused at their verified source/artifact scope, not rerun.

## Current Engine responsibility map (2026-11-02)

These are bounded implemented/tested/reviewed source changes. Earlier increments
retain their own recorded validation scope; this map does not rerun or enlarge
those receipts and does not imply activation, live-source acceptance or CL-3 closure.

| Completed boundary | Current ownership | Relevant code commits |
| --- | --- | --- |
| Work and Engine lifecycle | [work state](../internal/engine/work_lifecycle.go) tracks active/pending/cancelled work; [Engine lifecycle](../internal/engine/engine_lifecycle.go) retains launch, cancellation, shutdown and idle/readiness orchestration under existing locks | `202ecbb` |
| Scheduler lifecycle | [scheduler lifecycle](../internal/engine/scheduler_lifecycle.go) owns start-once, stop, wake and tick-loop mechanics; automatic policy remains separate | `056b53b` |
| Session execution/finalization | [execution](../internal/engine/session_execution.go) owns process/advance/accepted-observation/resume orchestration; [finalization](../internal/engine/session_finalization.go) retains the caller-held schedule lock and effect/error order | `d897130`, `ae1a535` |
| Settings execution | [save orchestration](../internal/engine/settings_execution.go), [profile merge](../internal/engine/settings_profiles.go) and [executable preparation](../internal/engine/settings_executable.go); candidate cleanup preserves error and provider ownership | `7d00d9c`, `02e063e`, `aea3867`, `c303ed4` |
| Diagnostics/status | [diagnostics export](../internal/engine/diagnostics_export.go) and [Bridge status](../internal/engine/bridge_status.go) separate output/compatibility work; constants/action catalog and heartbeat effects remain anchored | `1068e35`, `7b2d0d8` |
| Capture command/recapture use cases | [Bridge commands](../internal/engine/bridge_commands.go) and [media recapture](../internal/engine/media_recapture.go) own their orchestration while reusing Store/Manager and shared guards | `3e13434`, `c614512` |
| Background provenance/AI enrichment | [media provenance](../internal/engine/media_provenance.go) and [AI detection](../internal/engine/ai_detection.go) retain worker contexts, durable result order and shared cancellation | `8d7f7b6`, `b84a62d` |
| Session admission/creation | [admission](../internal/engine/session_admission.go) owns visible policy, source admission, lease acquisition/durable creation and handoff to scheduling | `4bf0731` |
| Shared observation contracts | [observations](../internal/engine/observations.go) owns normalization, validation and evidence-shape checks; five callers retain their own normalization/validation order and native identity guards | `d9c3025` |
| Process-error and usage-limit recovery | [process recovery](../internal/engine/process_recovery.go) owns failure projection, durable failure/queued-peer stop and pause/restore decisions; caller contexts/locks, Store authority and scheduler lifecycle remain unchanged | `5b39fa4` |
| Reasoning-result acceptance and Timeline mapping | [reasoning result](../internal/engine/reasoning_result.go) owns captured URL binding, assessment/evidence validation and selected item mapping; process keeps selection/effect order, Store assigns persistence fields and guards remain unchanged | `eb05ac8` |

The Engine facade intentionally retains dependency/state ownership, `New`,
locks, protocol constants/action anchors and suitable short delegations. The
goal is clear responsibilities, not an empty `engine.go` or removal of every
wrapper. The declined provider-selection micro-extraction remains declined.

| Remaining review area | Current question and boundary |
| --- | --- |
| Observation rounds and capture continuation | Next read-only proposal: map mergeObservations, continuationFrom, snapshotContinuationAnchors, continuationAnchors and shared helper consumers; preserve latest-round telemetry, accumulated evidence, readiness/overlap/anchor contracts and follow-up consumers before choosing a boundary |
| Vision/acquisition policy helpers | Vision routing/coverage and acquisition completion decisions remain separate review areas; keep source-specific evidence and acquisition guards |
| Product correction/feedback/privacy/reset workflows | Selection correction, AI/semantic feedback, library removal/forgetting, reset and cleanup coordinate multiple domain effects; inspect one complete workflow at a time while retaining privacy/retention and operation locks |
| Facade and adjacent runtime modules | Classify each remaining delegation as retain or useful boundary. Existing capture/daily/native modules are not automatically obsolete or complete; deeper runtime/QA work also belongs to CL-5, not wholesale Engine rewriting |

CL-3 remains partial until its responsibility map and remaining boundary decisions
are reviewed at their proper scope. Full packaging/consumer closure and final
integration/source sign-off remain CL-7/CL-8 work; operational source qualification
and activation have separate evidence gates. No cleanup percentage is asserted.

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
Feature extraction should move complete responsibilities with their tests,
such as Timeline rendering, frontend Settings orchestration or probe mode
dispatch; the immediate proposal is recorded below. Avoid moving all pages at once or hiding authority behind a
generic helper merely to reduce line count.

The earlier fixture-factory coupling has been addressed by `29ec2b4` and
`2ba01bb`; it is no longer the outstanding blocker stated by this audit.
The backend Settings orchestration boundary is established in
`settings_execution.go`; profile-map merge and executable preparation have their
own focused files. The provider-selection micro-extraction was declined: keep
the remaining 98-line `SaveSettings` cohesive rather than adding another wrapper.
The complete diagnostics export feature now belongs to `diagnostics_export.go`;
Bridge status/compatibility and its public types belong to `bridge_status.go`,
with constants/actions and heartbeat/recovery/scheduler effects retained in
Engine. HTTP Bridge command handlers belong to `bridge_commands.go`; the three
media-recapture handlers belong to `bridge_media_recapture.go`, retaining routing
and authority contracts. Ten Engine media-recapture methods now belong to
`media_recapture.go`. Seven Engine Bridge command methods now belong to
`internal/engine/bridge_commands.go`, retaining admission/fencing, Store effects,
recovery, follow-on scheduling and error/context contracts. Shared normalizers,
validation, capture-runtime, heartbeat/scheduler consumers and state ownership
remain unchanged. Five Engine media-provenance methods now belong to
`media_provenance.go`, retaining inspector runtime/resume, queue/worker/result
order, e.mu scopes, result persistence after cancellation and best-effort behavior.
Five AI-detection methods now belong to `ai_detection.go`, preserving fast/deep
candidate/profile/job/result orchestration, e.mu scopes, result contexts and
global AI/media-provenance cancellation. Session admission now belongs to
`session_admission.go`, retaining guard/source/lease/cancellation order, durable
session behavior and operation release before scheduling. Shared observation
contracts now belong to existing `observations.go` as `d9c3025`; normalization,
validation, evidence rules and all five caller files remain unchanged.
Process-error/usage-limit recovery now belongs to `process_recovery.go` as
`5b39fa4`, with seven exact declarations, unchanged Engine remainder and
three caller files. Reasoning-result acceptance and Timeline mapping now
belong to `reasoning_result.go` as `eb05ac8`, retaining three exact helper
bodies and the caller's binding/validation/selection/completion order.
Next propose read-only analysis of merging observations across rounds and
deriving capture continuation, retaining latest-round telemetry, accumulated
evidence, source overlap/readiness rules, protocol, guards and caller ownership.
No further implementation is approved here.
No new service layer is proposed. Other remaining
candidates include feature-level Timeline/frontend Settings orchestration,
probe mode dispatch, HTTP capture-service boundaries and broader legacy/consumer
review. They require separate bounded analysis/approval, not a deletion list or
authorization to continue refactoring.
Native IG qualification and operational acceptance remain open for their own
document-continuity reasons, not because the production factory is still in QA.

## Validation

The following results belong to the original 2026-10-25 audit. Later cleanup
validation is linked in the 2026-11-02 matrix and status index above.

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
