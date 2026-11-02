# Product source structure audit — 2026-10-25

## Status synchronization (2026-11-02)

The original audit and its validation below are historical. Current source is
reviewed through `b482ba2`, including capture-owner path normalization, HTTP bootstrap and the router responsibility map, HTTP media-recapture entry points, HTTP update/session lifecycle, Engine passive media evidence, Library removal/Forget, FullReset, AI feedback, selection corrections, vision-lane routing/coverage, local acquisition policy, capture rounds and the Store privacy-test correction, reasoning results, process recovery, observation contracts, session admission and AI detection/media provenance, Engine/HTTP media-recapture, Engine/HTTP Bridge command, Bridge status and diagnostics feature separation; see the [roadmap status index](roadmap-status.md)
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
| Engine capture rounds and privacy-test validation detour | `683b1e8` | Capture-round characterization passes 59 default/dev results before/after relocation; privacy test passes 20 repetitions per build after deterministic legacy failure; fresh 13-package selection (12 tested, Bridge builder compile-only) passes default 1,705/48 skip and dev 2,549/60 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [final validation](../runtime/qa/store-tombstone-assertion-20261102/validation.json), [review](../runtime/qa/engine-capture-rounds-review-20261102/review.json) and [commit readback](../runtime/qa/engine-capture-rounds-review-20261102/final-readback.json) verify combined source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed/published. Eight free helpers and Engine remainder byte-identical; callers, telemetry/coverage/alias/reconciliation/continuation semantics, locks, contexts, protocol and guards retained. Exact independent HMAC test vectors replace the probabilistic assertion; production privacy behavior unchanged. CL-3 remains partial; operational acceptance remains open |
| Engine local acquisition policy | `f16f11a` | Identical characterization passes 206 default/dev parent/subtest results before/after relocation; matching 13-package selection (12 tested, Bridge builder compile-only) passes default 1,911/48 skip and dev 2,755/60 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-acquisition-policy-20261102/validation.json), [review](../runtime/qa/engine-acquisition-policy-review-20261102/review.json) and [commit readback](../runtime/qa/engine-acquisition-policy-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four free functions retain exact signatures/bodies/comments/reasons; Engine remainder, caller priority, scalar readers, continuation, source descriptors, Store authority, queue/receipt/error order, protocol and guards unchanged. CL-3 remains partial; operational acceptance remains open |
| Engine vision-lane routing and coverage | `25c3339` | Identical characterization passes 83 default/dev parent/subtest results before/after relocation; matching 13-package selection (12 tested, Bridge builder compile-only) passes default 1,994/48 skip and dev 2,838/60 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-vision-lane-20261102/validation.json), [review](../runtime/qa/engine-vision-lane-review-20261102/review.json) and [commit readback](../runtime/qa/engine-vision-lane-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed/published. Two free functions retain exact signatures/bodies; Engine remainder, caller, source predicates, Coverage/nested aliasing, queue capacity, Store ownership, context/error/side-effect order, protocol and guards unchanged. Current manual_review availability is preserved. CL-3 remains partial; operational acceptance remains open |
| Engine selection-correction workflow | `f3d46b8` | Identical characterization passes 32 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,048/50 skip and dev 2,892/62 skip, zero failures; EventEngine added to prior scope; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-selection-corrections-20261102/validation.json), [review](../runtime/qa/engine-selection-corrections-review-20261102/review.json) and [commit readback](../runtime/qa/engine-selection-corrections-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Two Engine methods retain exact signatures/bodies; Engine remainder, caller/Store/background ownership, operation/mu scopes, partial errors, durable/enrichment/cancellation order, protocol and guards unchanged. Undo retains same-session AI cancellation only and post-commit cleanup error behavior. CL-3 remains partial; operational acceptance remains open |
| Engine AI-feedback workflow | `6a1ce33` | Identical characterization passes 58 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,106/50 skip and dev 2,950/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-ai-feedback-20261102/validation.json), [review](../runtime/qa/engine-ai-feedback-review-20261102/review.json) and [commit readback](../runtime/qa/engine-ai-feedback-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three Engine methods retain exact signatures/bodies; Add/Undo operation locks, lock-free history, durable-write/read/handoff order, review failures logged after commit while saved feedback is returned, append-only clear, cancellation/context behavior, Store/domain/HTTP ownership, protocol and guards unchanged. CL-3 remains partial; operational acceptance remains open |
| Engine FullReset orchestration | `da019dd` | Identical characterization passes 23 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,129/50 skip and dev 2,973/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-full-reset-20261102/validation.json), [review](../runtime/qa/engine-full-reset-review-20261102/review.json) and [commit/publication readback](../runtime/qa/engine-full-reset-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed and code published. One Engine method retains exact signature/body; operation/mu scopes, cancellation before admission/errors, default/provider resolution, candidate disposal, Store authority and post-commit capture/headless/swap order unchanged. CL-3 remains partial; operational acceptance remains open |
| Engine Library removal/Forget workflow | `a19bf86` | Identical characterization passes 28 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,157/50 skip and dev 3,001/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-library-removal-20261102/validation.json), [review](../runtime/qa/engine-library-removal-review-20261102/review.json) and [commit/publication readback](../runtime/qa/engine-library-removal-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; code committed/published. Two exact Engine methods retain topic snapshot before Store mutation, sorted/background queue handoff, per-topic error logging/continuation, caller return/error/context semantics, absence of entry-level locks, Store privacy authority, protocol, guards and AI/provenance cancellation. CL-3 remains partial; operational acceptance remains open |
| Engine passive X media evidence | `2bdc0f1` | Identical characterization passes 20 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,177/50 skip and dev 3,021/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-passive-media-20261102/validation.json), [review](../runtime/qa/engine-passive-media-review-20261102/review.json) and [commit readback](../runtime/qa/engine-passive-media-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Exact method moved to existing media_recapture.go with no import changes; development gate, operation lock, Store result/errors, conditional Timeline/provenance handoff, background contexts, unlock-before-terminal-cleanup order, protocol and guards retained. Central-file checkpoint retains 52 declarations; CL-3 remains partial and operational acceptance open |
| HTTP update/session lifecycle | `d32c065` | Identical characterization passes 53 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,230/50 skip and dev 3,074/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-session-updates-20261102/validation.json), [review](../runtime/qa/http-session-updates-review-20261102/review.json) and [commit readback](../runtime/qa/http-session-updates-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four Server handlers preserve router predicates/order/normalization, request contexts, shared loopback/origin security and progress projection, raw start output, asymmetric errors and durable cancel/read/write order. Engine, Store, protocol, guards and cancellation unchanged. CL-3 remains partial; operational acceptance open |
| HTTP media-recapture status/enqueue/passive evidence | `90e3a7f` | Identical characterization passes 42 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,272/50 skip and dev 3,116/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-media-recapture-20261102/validation.json), [review](../runtime/qa/http-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/http-media-recapture-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three Server handlers preserve router predicates/order/normalization, distinct ID parsing, private status projection, UI versus Bridge security, authentication before decode, unfenced passive relay, validation/defaults/errors, caller context and durable effects before response. Legacy Facebook branch, Engine/Store, Bridge claim/result handlers, protocol, guards and cancellation unchanged. CL-3 remains partial; operational acceptance open |
| HTTP bootstrap composition | `97bbe51` | Identical characterization passes 25 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,297/50 skip and dev 3,141/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-bootstrap-20261102/validation.json), [review](../runtime/qa/http-bootstrap-review-20261102/review.json) and [commit readback](../runtime/qa/http-bootstrap-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. One Server method preserves route predicates/order/normalization, shared loopback/origin security, request context, complete output/progress projection, ten ordered fallible data calls, first-error mapping, token initialization, first-run calibration, prepared-batch expiry and durable effects before response-write failure. No new interface, transaction, cache or concurrency; Engine/Store, protocol and guards unchanged. CL-3.3 and CL-3 remain partial; activation and operational acceptance open |
| HTTP capture-owner path guard | `b482ba2` | Regression added before the fix reproduced unauthorized action acceptance and telemetry persistence for trailing/encoded slash forms. The unchanged regression payload then passed default/dev; matching 14-package selection (13 tested, Bridge builder compile-only) passed default 2,348/50 skip and dev 3,192/62 skip, zero failures; four Windows amd64 build-tag graphs include the new tests and 16 artifacts pass; source and artifact identities reverified before commit, without new test/build runs | Not performed | Scoped admission-path fix reviewed and committed. One classifier assignment now trims exactly one trailing slash like dispatch; missing/stale keys return 409 before accept/event mutation. Valid workers, contract/token priority, UI relay and non-split behavior remain. No handler move or wire change. Post-admission rotation is separate and untested; CL-3.3/CL-3 partial; operational acceptance open |

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
| Observation rounds and capture continuation | [capture rounds](../internal/engine/capture_rounds.go) owns accumulated evidence, latest-round/durable coverage, yield and continuation derivation; source descriptors, reconciliation, caller decisions and payload/Store guards remain unchanged | `683b1e8` |
| Local acquisition decisions | [acquisition policy](../internal/engine/acquisition_policy.go) owns structured-fallback/local-frontier completion, guarded X follow-up and the frontier wrapper; caller priority/effects, scalar readers, descriptors, continuation, Store and guards remain unchanged | `f16f11a` |
| Vision-lane candidate routing and coverage | [vision lane](../internal/engine/vision_lane.go) owns media-only candidate separation and exact summary coverage; source predicates, shallow aliases, bounded Store queue, manual_review availability and caller effects remain unchanged | `25c3339` |
| Selection correction and undo | [selection corrections](../internal/engine/selection_corrections.go) owns correction/enrichment and undo/cancellation orchestration; Engine receivers, operation/mu scopes, partial errors, Store transaction boundaries and exact cancellation targets remain unchanged | `f3d46b8` |
| AI-feedback write, undo and history | [AI feedback](../internal/engine/ai_feedback.go) owns feedback-to-Deep handoff and Store delegations; mutation locks, lock-free history, durable/logging/error order, append-only output and existing cancellation behavior remain unchanged | `6a1ce33` |
| FullReset orchestration | [FullReset](../internal/engine/full_reset.go) owns cancellation/default/provider/Store reset and post-commit runtime transitions; exact method, operation/mu scopes, error/cleanup order and Store authority retained | `da019dd` |
| Library removal and permanent Forget | [Library removal](../internal/engine/library_removal.go) owns topic snapshot, Store privacy mutation and background topic handoff; exact methods, error/continuation/context behavior and absence of entry-level locks retained | `a19bf86` |
| Passive X media evidence | [Media recapture](../internal/engine/media_recapture.go) also owns ApplyPassiveXMediaEvidence; exact Store/enrichment/cleanup order, Engine operation ownership, contexts, return/errors and guards retained | `2bdc0f1` |

The Engine facade intentionally retains dependency/state ownership, `New`,
locks, protocol constants/action anchors and suitable short delegations. The
goal is clear responsibilities, not an empty `engine.go` or removal of every
wrapper. The declined provider-selection micro-extraction remains declined.

| Remaining review area | Current question and boundary |
| --- | --- |
| Typed coverage readers | Retain the existing three scalar readers for now. Production consumers are in acquisition_policy.go; relocating them merely to shorten Engine is not the next priority and is not approved |
| Product privacy/reset workflows | FullReset and Library removal/Forget boundaries are implemented/tested/reviewed. Retain ResetLearning and CleanDatabase with their existing operation/Store admission contracts; no additional privacy/reset file split is recommended at this checkpoint |
| Facade and adjacent runtime modules | The central-file checkpoint below retains all 52 remaining declarations after passive-evidence colocation. Existing capture/daily/native modules are not automatically obsolete or complete; deeper runtime/QA work also belongs to CL-5, not wholesale Engine rewriting |

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
Capture-round state preparation now belongs to `capture_rounds.go` as
`683b1e8`, with eight exact helpers and unchanged Engine remainder/callers.
The same commit corrects only the probabilistic Store privacy-test assertion;
fresh full default/dev runs and 16 Windows artifacts verify the combined payload.
Local acquisition decisions now belong to `acquisition_policy.go` as `f16f11a`: four
free functions retain exact signatures, bodies, comments and reasons; source
predicates, bounded follow-up, model fallback, protocol, guards and caller ownership
remain unchanged. Vision-lane routing/coverage now belongs to `vision_lane.go`
as `25c3339`, preserving exact helper bodies, Coverage/nested aliases, source
predicates, queue/receipt/evaluation order and existing manual_review availability.
Selection correction/undo now belongs to `selection_corrections.go` as `f3d46b8`,
with two identical Engine methods, operation/mu locks, contexts, durable/enrichment/
error order and existing cancellation targets retained. AI feedback now belongs
to `ai_feedback.go` as `6a1ce33`, with three identical Engine methods, mutation
locks, lock-free history, persistence/review gates and error/cancellation behavior
retained. FullReset now belongs to `full_reset.go` as `da019dd`, preserving
cancellation/admission, candidate disposal/swap, Store reset and capture authority
effects. Library removal/Forget now belongs to `library_removal.go` as `a19bf86`,
with exact snapshot/mutation/background queue and partial-error contracts. The
central Engine checkpoint retains all 52 remaining declarations after passive
evidence colocation in the existing media-recapture module as `2bdc0f1`.
The scalar readers remain in place for now.
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

## Engine facade stopping checkpoint (2026-11-02)

Goal: leave Engine as the stable owner of dependencies, locks and application
entry points, while feature modules own substantive workflows. The source
review covered 53 declarations at `a19bf86`; the one selected colocation is now
implemented at `2bdc0f1`. All 52 remaining declarations are retained. This does
not certify every file in the Engine package or close CL-3.2.
The [inventory](../runtime/qa/engine-passive-media-roadmap-sync-20261102/engine-facade-inventory.json) binds each name, line,
classification and decision to the current source hash. Classification: 52 retain,
zero pending central-file moves. These counts are an inventory, not cleanup progress.

| Group | Count | Decision and reason |
| --- | ---: | --- |
| Constructor and identity/dependency access | 4 | Retain New, resolver injection, Epoch and ProviderName with Engine state ownership |
| Store facade delegates | 31 | Retain stable query/mutation entry points; Store already owns domain transactions/read models and the wrappers add no competing policy |
| Capture/scheduling adapters and scalar coverage readers | 5 | Retain capturePayload, launch and three scalar readers; builder/process/policy owners already exist and relocation adds no useful seam |
| Short lock-protected Store delegates | 3 | Retain semantic correction/undo and ResetLearning; operation ownership stays visible and Store remains authoritative |
| Onboarding admission | 1 | Retain CompleteOnboarding: one coherent operation-locked readiness-before-completion gateway. Provider readiness/probes and durable onboarding already have separate owners; another file alone does not separate those responsibilities |
| Bridge heartbeat/reload entry points | 5 | Retain RecordHeartbeat and four reload methods. They coordinate Engine-owned heartbeat/origin/access state with existing BridgeStatus, ReloadActions, command recovery and scheduler owners; no new state owner/service is justified here |
| Vision read projection | 1 | Retain VisionEvaluationJobs: source-scoped list derived from the existing run; no new policy object |
| Failed-run retry admission | 1 | Retain ReevaluateFailedRun: operation gate, Store retry then existing process launcher. Do not introduce a separate retry worker or alter post-commit launch order |
| Database cleanup admission | 1 | Retain CleanDatabase: operation gate and active-recapture guard before Store cleanup. No new cleanup policy or transaction owner |

Focused source evidence: [Engine](../internal/engine/engine.go),
[provider readiness](../internal/engine/provider_readiness.go),
[Bridge status](../internal/engine/bridge_status.go),
[reload lifecycle](../internal/engine/reload_actions.go),
[media recapture](../internal/engine/media_recapture.go),
[passive evidence Store](../internal/store/recapture.go),
[provenance handoff](../internal/engine/media_provenance.go) and
[terminal cleanup](../internal/engine/capture_runtime.go).

Historical pre-move requirements below explain the boundary. The method and
[focused tests](../internal/engine/passive_media_test.go) are now implemented and
validated as `2bdc0f1`; this synchronization reuses that evidence:

1. Non-nil developmentSources, including an empty map, rejects before Store,
   operation lock and deferred cleanup. Preserve exact zero/false/error output.
2. Ordinary calls hold operation through Store apply and conditional Timeline
   read/provenance handoff. Store error and unchanged/no-op paths skip enrichment;
   preserve the original result/updated/error tuple without new normalization.
3. Successful Store commit precedes Timeline reload and provenance queue. A
   Timeline reload error is ignored, retaining the successful Store result.
   Inspector unavailable/queue failure/active-worker cases retain existing
   helper behavior and do not change the returned Store success.
4. Caller context reaches Store apply and Timeline reload. Provenance and deferred
   terminal cleanup keep their existing background contexts. operation unlock
   occurs before terminal cleanup because of defer order; cleanup still runs
   after ordinary failures/no-op/cancellation. No new drain/cancellation, AI
   cancellation or observer/identity/trusted-media guard change is proposed.
5. Reuse existing Store passive-media validation/idempotence and HTTP Bridge
   authentication tests; add focused Engine ordering/error/cleanup tests with
   disposable real Store data and controlled Inspector/capture fixtures. There
   was no direct Engine test at the earlier checkpoint; the new suite now covers
   these paths. Exact relocation, default/dev regressions and four-tag Windows
   compilation passed. No new product test/build or live-source activity ran
   for this documentation synchronization.

Stopping rule: that one colocation is now implemented. Retain the central facade
until a concrete coupling, testability or behavior problem justifies more work.
Do not create onboarding/heartbeat/retry/cleanup files just to reduce line count.
The central-file map is now checkpointed; this alone does not complete
adjacent Engine/runtime/HTTP work, CL-3.2, CL-3 or CL-5/CL-7/CL-8 acceptance.
No remaining methods are classified as removable legacy code by this review.
At that checkpoint, the next proposal was the read-only HTTP router
responsibility map. That analysis and its separately approved bootstrap
extraction are now recorded in the
[bootstrap/router-map result](cl1-cleanup-candidates.md#cl-33-http-bootstrap-and-router-map-result-2026-11-02).
The map does not authorize extraction of deferred groups.

## HTTP update and session boundary (2026-11-02)

The approved HTTP update/session extraction is committed as `d32c06562fd30b8a20c9e781e886faacfc4a8711`.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| HTTP update/session lifecycle | `d32c065` | Identical characterization passes 53 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,230/50 skip and dev 3,074/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-session-updates-20261102/validation.json), [review](../runtime/qa/http-session-updates-review-20261102/review.json) and [commit readback](../runtime/qa/http-session-updates-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four Server handlers preserve router predicates/order/normalization, request contexts, shared loopback/origin security and progress projection, raw start output, asymmetric errors and durable cancel/read/write order. Engine, Store, protocol, guards and cancellation unchanged. CL-3 remains partial; operational acceptance open |

Four private Server methods now live in internal/httpapi/session_updates.go:
startUpdate, activeSession, sessionDetail and cancelSession. The router replaces
only four inline bodies with delegation, preserving predicates and their order;
detail/cancel receive the already-normalized path. Body equivalence allows only
one-level deindentation and method-local r.Context binding. Shared api/security,
readJSON/writeJSON/error helpers and sessionProgressProjection remain in place;
bootstrap still uses the projection. No Engine interface, new admission owner,
Store transaction, protocol or guard is introduced.

Characterization was added first and frozen identically before/after. It covers
reserved active-path precedence, path.Base aliases and trailing slash behavior,
method mismatch, nil active session, polling projection/nonmutation, full raw
start output, MIME/unknown/type/trailing-JSON behavior, loopback/origin guards
without Bridge-token requirements, epoch/cache headers and OPTIONS, caller
cancellation before work/after decode, distinct 404/409/500 mappings, cancellation
rollback, durable start/cancel followed by read error, and response-write failure
without rollback. Local httptest requests and disposable real Engine/Store data
need no live capture or native observer. Initial two fault fixtures were corrected
to supply valid intent, preserving production validation; their failed attempt
is retained separately from the final passing logs.

Identical characterization passes 53 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,230/50 skip and dev 3,074/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts. Counts include parent/subtest results and repeated tag selections,
not unique independent cases. Source/log/artifact and committed-blob identities
were checked during review. This sync reuses that evidence without product test/
build reruns. Activation and operational acceptance remain open; CL-3 remains
partial. Publication readbacks belong under runtime/qa/http-session-updates-roadmap-sync-20261102.

The central Engine stopping checkpoint remains unchanged: 52 declarations retained,
with no new central-file move proposed. The roadmap header and detailed index
now agree on the reviewed source commit. This does not close all Engine/runtime,
HTTP capture, CL-3 or integration/package/source-acceptance work.

At that checkpoint, the next proposal was the read-only HTTP router
responsibility map. That analysis and its separately approved bootstrap
extraction are now recorded in the
[bootstrap/router-map result](cl1-cleanup-candidates.md#cl-33-http-bootstrap-and-router-map-result-2026-11-02).
The map does not authorize extraction of deferred groups.

## HTTP media-recapture entry boundary (2026-11-02)

Status, enqueue and passive-evidence handlers now belong to
`internal/httpapi/media_recapture.go` as `90e3a7f`. See the
[result](cl1-cleanup-candidates.md#cl-33-http-media-recapture-entry-result-2026-11-02)
and [validation](../runtime/qa/http-media-recapture-20261102/validation.json), [review](../runtime/qa/http-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/http-media-recapture-review-20261102/commit-readback.json).
Identical characterization passes 42 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,272/50 skip and dev 3,116/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts. Source/body/router equivalence and existing evidence are reused,
not rerun by this sync. Privacy projection, UI/Bridge security and unfenced relay,
error/context/durable effect order, protocol and guards are preserved. Scoped
review accepted; activation and operational acceptance remain open. CL-3 remains partial.

At that checkpoint, the next proposal was the read-only HTTP router
responsibility map. That analysis and its separately approved bootstrap
extraction are now recorded in the
[bootstrap/router-map result](cl1-cleanup-candidates.md#cl-33-http-bootstrap-and-router-map-result-2026-11-02).
The map does not authorize extraction of deferred groups.

## HTTP bootstrap and router responsibility map (2026-11-02)

Bootstrap composition is committed as `97bbe51` in
`internal/httpapi/bootstrap.go`, with one delegation in `server.go`.
Characterization was added first and frozen unchanged across the move. It covers
the response envelope/empty shapes, metadata, active-session projection and
Timeline capacity; UI/security/method/path/preflight behavior; repeated first-run
calibration and prepared-batch expiry; representative first-error/partial-effect
cases; cancelled/deadline contexts and response-write failure. Eight failure
stages are injected; the two other ordered calls retain exact body equivalence.
Token initialization, calibration creation and expiry can write state, so this
endpoint is not treated as a pure read. No new transaction/rollback, concurrency,
cache, interface or test seam was introduced.

Identical characterization passes 25 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,297/50 skip and dev 3,141/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts. Counts include parents/subtests and repeated tag selections, not
unique independent cases. Review reuses matching evidence without product
test/build reruns. The initial capacity fixture was corrected to use the custom
profile; a dev setup attempt hit sandbox cache access denial and passed with
the same payload outside the sandbox. Those attempts remain separate from the
final passing receipts. Implemented/tested and scoped review accepted; activation
and operational acceptance remain open. CL-3.3 and CL-3 remain partial.

See [validation](../runtime/qa/http-bootstrap-20261102/validation.json), [review](../runtime/qa/http-bootstrap-review-20261102/review.json) and
[commit readback](../runtime/qa/http-bootstrap-review-20261102/commit-readback.json). Publication readbacks belong under
runtime/qa/http-bootstrap-roadmap-sync-20261102.

The baseline map covered 106 top-level router switch arms, not 106 endpoints
(some arms contain multiple methods/subroutes): 48 retain, 57 defer and one
bootstrap extraction candidate. Bootstrap is now delegated, leaving 49 retain
(24 already-delegated feature arms and 25 thin/control/gate arms), 57 defer and
no automatically approved extraction candidate. The 30 remaining server.go
function/method declarations stay with Server lifecycle, dispatch, security/CORS,
JSON/error handling and shared projection/validation ownership.

| Deferred group | Top-level switch arms | Reason for deferral |
| --- | ---: | --- |
| Native/dev probes and capture telemetry | 9 | Native/runtime ownership, gates and trace lifetime need a dedicated review |
| Product calibration | 4 | Preserve first-run workflow and domain ownership |
| Settings/providers/credentials | 5 | Preserve secure persistence, validation order and partial effects; keep Settings cohesive |
| Selection correction/re-evaluation | 3 | Engine owns cancellation and execution |
| Timeline/content context | 4 | Existing view helpers and Store/Engine query authority remain |
| Living Topics | 5 | Prefix arms contain additional subroutes with distinct product contracts |
| Library queries/privacy/actions | 9 | Preserve privacy projection, tombstones and distinct Remove/Forget semantics |
| Auto Update lifecycle/quota | 4 | Scheduler and quota authority remain in Engine |
| Keep/read-later/preference/AI feedback | 6 | Distinct persistence, handoff and cancellation semantics |
| Semantic corrections | 3 | Product correction contracts need separate consumer analysis |
| Bridge heartbeat/reload actions | 5 | Shared Engine/transport authority and long-poll cancellation need a bounded review |

No deletion candidate was established. Defer means retain pending a separately
approved ownership/consumer decision, not obsolete or a mandatory move queue.
The stopping rule is to retain infrastructure/shared helpers and thin facades;
extract only a whole use case with a concrete coupling/testability benefit,
never merely to shorten the file. After each bounded result, stop and assess
the remaining evidence. The map and bootstrap close this selected analysis and
extraction, not the entire HTTP package, CL-3.3, CL-3 or source/runtime acceptance.
The central Engine checkpoint remains at 52 retained declarations.

The read-only heartbeat/reload review identified two responsibilities: retain
heartbeat in server.go, because it coordinates readiness, source authority,
recovery, scheduler wake and reload observation; the four request/next/accept/
detail handlers form one proposed reload-action HTTP boundary. A normalization
guard gap found during that review was fixed separately as `b482ba2`.
The next implementation proposal is characterization first, then four private
Server methods in `internal/httpapi/bridge_actions.go`. It requires separate
approval. Preserve route order/normalization, shared authentication/security,
Engine and split-transport state/lock ownership, raw errors/statuses, delivery
priority, request cancellation and in-memory side effects before write failure.
Keep heartbeat, shared helpers, protocol and other refactors unchanged.
The current continuation refines that proposal read-only; it grants no new
implementation, runtime/configuration change, activation or live trial.

## Capture-owner admission guard result (2026-11-02)

Implemented as `b482ba2`: `splitCaptureOwnedRoute` now applies exactly the
same single-suffix `strings.TrimSuffix(r.URL.Path, "/")` normalization as
dispatch. This closes the accept and capture-surface telemetry aliases while
preserving existing UI relay exclusions, auth order, current-worker and non-split
behavior. No Engine/Store change, new lock, handler extraction or protocol change.

Regression added before the fix reproduced unauthorized action acceptance and telemetry persistence for trailing/encoded slash forms. The unchanged regression payload then passed default/dev; matching 14-package selection (13 tested, Bridge builder compile-only) passed default 2,348/50 skip and dev 3,192/62 skip, zero failures; four Windows amd64 build-tag graphs include the new tests and 16 artifacts pass. Counts include parent/subtest results and repeated tag selections,
not unique independent cases. Full security + ServeMux + API recorder fixtures
proved the mutations before the fix and rejection without mutation afterwards;
canonical, trailing slash, encoded separator/slash and query forms are covered.
Two independent read-only boundary/review passes found no blocking candidate
issue. The source and 16 artifact hashes still match the recorded validation;
this synchronization reuses evidence rather than rerunning tests/builds.

Managed standalone evidence is outside Git. Retrieve it with
read_codex_security_artifact using targetPath=<this checkout root>,
storage=`persistent`, path=`artifacts/guard-path-normalization-20261102/validation.json`.
Receipt content SHA256: `96f513711e04c529aaa95d575905cd378a486c73249369fcf5b7642848364702`.
Later commit/documentation/publication readbacks use the same collection and
the paths `artifacts/guard-path-normalization-20261102/commit-readback.json` and
`artifacts/guard-path-normalization-20261102/publication.json`. Raw validation
fields such as committed=false describe the earlier capture time; retain them.
The regression source is [split_capture_owner_guard_test.go](../internal/httpapi/split_capture_owner_guard_test.go).

Implemented/tested and scoped review accepted; not activated. Operational
acceptance remains open. A separate statically noted telemetry request admitted
before owner rotation has not been dynamically tested or changed; do not infer
closure of all owner-rotation behavior from this admission-path repair.

This is a guard correction, not a sixth HTTP extraction. The router-map counts
remain 49 retained arms, 57 deferred arms and 30 retained server.go declarations.
The central Engine checkpoint remains 52 retained declarations. CL-3.3 and CL-3
remain partial; no comprehensive or live-source acceptance is established.

### Next reload-action boundary

The read-only heartbeat/reload review identified two responsibilities: retain
heartbeat in server.go, because it coordinates readiness, source authority,
recovery, scheduler wake and reload observation; the four request/next/accept/
detail handlers form one proposed reload-action HTTP boundary. A normalization
guard gap found during that review was fixed separately as `b482ba2`.
The next implementation proposal is characterization first, then four private
Server methods in `internal/httpapi/bridge_actions.go`. It requires separate
approval. Preserve route order/normalization, shared authentication/security,
Engine and split-transport state/lock ownership, raw errors/statuses, delivery
priority, request cancellation and in-memory side effects before write failure.
Keep heartbeat, shared helpers, protocol and other refactors unchanged.
The current continuation refines that proposal read-only; it grants no new
implementation, runtime/configuration change, activation or live trial.
