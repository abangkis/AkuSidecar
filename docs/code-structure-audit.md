# Product source structure audit — 2026-10-25

## Status synchronization (2026-11-03)

The original audit and its validation below are historical. Current source is
reviewed through `de95c58` (empty-candidate completion; observation preparation and vision handoff retained), including per-run candidate evaluation/completion, then acquisition planning orchestration, then automatic completion of the remaining 58 HTTP adapters, then Timeline composition HTTP adaptation, vision-evaluation HTTP adapters, model-usage HTTP adapters, Inbox read HTTP adapters, Inbox selection-review HTTP adapters, AI-feedback HTTP adapters, semantic correction HTTP adapters, development X HTTP controls, capture telemetry rotation/shutdown compatibility, HTTP reload-action separation, capture-owner path normalization, HTTP bootstrap and the router responsibility map, HTTP media-recapture entry points, HTTP update/session lifecycle, Engine passive media evidence, Library removal/Forget, FullReset, AI feedback, selection corrections, vision-lane routing/coverage, local acquisition policy, capture rounds and the Store privacy-test correction, reasoning results, process recovery, observation contracts, session admission and AI detection/media provenance, Engine/HTTP media-recapture, Engine/HTTP Bridge command, Bridge status and diagnostics feature separation; see the [roadmap status index](roadmap-status.md)
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
| HTTP Bridge reload actions | `6ea303c` | Identical characterization passes 45 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,393/50 skip and dev 3,237/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-bridge-actions-20261102/validation.json), [review](../runtime/qa/http-bridge-actions-review-20261102/review.json) and [commit readback](../runtime/qa/http-bridge-actions-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four private Server methods preserve route predicates/order/normalization, request context, auth/decode/error order, UI relay versus worker guard, action output, empty 204, wait bound, delivery/cancellation priority and in-memory effects before write failure. Heartbeat, shared helpers, Engine/Store state and locks, owner-path fix, protocol and guards unchanged. CL-3.3/CL-3 partial; operational acceptance open |

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
bootstrap extraction candidate. At the bootstrap checkpoint, delegation left 49 retain
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

The heartbeat/reload review retained heartbeat in server.go and selected four
reload-action HTTP handlers. The separately approved extraction is implemented,
tested and reviewed as `6ea303c`; see the [reload-action result](cl1-cleanup-candidates.md#cl-33-http-bridge-reload-actions-result-2026-11-02).
The earlier owner-path guard correction remains `b482ba2`. State and locks
remain in Engine/transport, with shared infrastructure retained.

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
acceptance remains open. At this admission-path checkpoint, the post-admission rotation scenario was
still unverified. The later [telemetry rotation result](cl1-cleanup-candidates.md#capture-telemetry-rotation-result-2026-11-03)
records its separate fix and offline tests; neither guard repair establishes
all owner-rotation or live-source behavior.

This is a guard correction, not a sixth HTTP extraction. At that guard checkpoint, the router-map counts
remained 49 retained arms, 57 deferred arms and 30 retained server.go declarations.
The central Engine checkpoint remains 52 retained declarations. CL-3.3 and CL-3
remain partial; no comprehensive or live-source acceptance is established.

### Reload-action boundary follow-up

The heartbeat/reload review retained heartbeat in server.go and selected four
reload-action HTTP handlers. The separately approved extraction is implemented,
tested and reviewed as `6ea303c`; see the [reload-action result](cl1-cleanup-candidates.md#cl-33-http-bridge-reload-actions-result-2026-11-02).
The earlier owner-path guard correction remains `b482ba2`. State and locks
remain in Engine/transport, with shared infrastructure retained.

## HTTP Bridge reload actions result (2026-11-02)

Four HTTP adapters are committed as `6ea303c` in
`internal/httpapi/bridge_actions.go`: requestBridgeReload, nextBridgeAction,
acceptBridgeAction and bridgeActionStatus. Their bodies are identical except
indentation; the only server.go change replaces their 59 body lines with four
delegations. Router predicates/order/normalization and the remainder are exact.
Heartbeat, security/CORS/JSON/error helpers, the owner-path guard correction,
Engine/Store state and locks, protocol and guards are unchanged.

Characterization was added first and frozen byte-identically across the move.
It covers split/non-split lifecycle, replay and active-action conflicts, output
fields/metadata, request validation and existing trailing-JSON behavior, routing,
UI relay versus worker authentication/owner fencing, empty 204/header behavior,
caller context and response-write failures without undoing in-memory state.
Cancellation before token authentication retains 401; the bounded long-poll
fixture allows that legitimate race or empty 204. A separate deterministic Engine
control checks pending delivery before cancellation. Unproduced generic error
branches and expiry timing remain covered by exact body/Engine equivalence;
no clock, interface or production test seam was introduced.

Identical characterization passes 45 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,393/50 skip and dev 3,237/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts. Counts include parents/subtests and repeated tag selections, not
unique independent cases. Review reverified source/log/artifact and committed
blob identity; this sync reuses evidence without product test/build reruns.
Implemented/tested and scoped review accepted; not activated. Operational
acceptance remains open. The then-unverified post-admission rotation scenario
now has a separate offline fix/result below.
See [validation](../runtime/qa/http-bridge-actions-20261102/validation.json), [review](../runtime/qa/http-bridge-actions-review-20261102/review.json) and
[commit readback](../runtime/qa/http-bridge-actions-review-20261102/commit-readback.json). Publication readbacks belong under
runtime/qa/http-bridge-actions-roadmap-sync-20261102.

Current central-router snapshot: 106 top-level switch arms, not endpoint count;
54 retained arms comprise 28 existing delegations and 26 thin/control/heartbeat
arms. The 52 deferred arms comprise nine native/dev/telemetry arms and 43 product
domain arms. Relative to the bootstrap checkpoint (49 retain/57 defer), four
reload arms became delegations and heartbeat was explicitly reclassified retain.
All 30 server.go function/method declarations and 52 central Engine declarations
remain retained. Six bounded HTTP extractions are established; the owner-path
guard correction is a separate correctness increment. Deferred groups are not
an automatic extraction queue. Keep central-file splitting stopped unless a
concrete ownership, coupling or testability problem justifies another boundary.
CL-3.3 and CL-3 remain partial; this is not whole-package or operational acceptance.

## Capture telemetry rotation result (2026-11-03)

Implemented as `237dbd1`: admitted capture-surface HTTP telemetry rechecks the
owner key under `splitCapture.mu` and retains that mutex through the original
batch mutation loop. Body decoding and response I/O remain outside the lock.
Retired-key requests return 409 without mutation; valid current-key cleanup
continues during `Replacing`, without a replacement-blocking runtime lease.
The review-discovered shutdown deadline regression is also fixed: a listening
server can force-close HTTP connections on deadline while transport cleanup
retains its existing ownership and finishes later.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `237dbd1`, telemetry fencing plus coupled shutdown compatibility; no new protocol/schema or feature extraction |
| Tested | Matching 14-package selection: default 2,419 pass/50 skip; dev 3,263 pass/62 skip; zero failures. Controlled concurrency/shutdown selection repeated 20 times: 540 passing parent/subtest results per mode. Four Windows amd64 tag combinations and 16 artifacts pass |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted; live rotation, source qualification and whole-package acceptance remain open |

Review rechecked the unchanged mutation-loop order and all recorded source/test,
configuration and 16 binary hashes. This synchronization reuses matching
validation rather than rerunning product tests/builds. No race detector or real
native observer/browser trial was run. Trusted internal native-trace emitters
remain independent; the new fence covers stale HTTP mutations, not a global
native-trace drain. See the [detailed result](cl1-cleanup-candidates.md#capture-telemetry-rotation-result-2026-11-03)
for managed evidence retrieval and review limits.

Router-map counts remain 106 arms: 54 retained and 52 deferred; six bounded
HTTP extractions. The 30 server.go declarations and 52 central Engine
declarations remain retained. CL-3.3 and CL-3 remain partial.

### Development X follow-up

The then-proposed read-only review and separately approved extraction are now
complete; see the development X result below.

## HTTP development X controls result (2026-11-03)

Four Server adapters are committed as `1814c9b` in
`internal/httpapi/x_development.go`: startXDaily, stopXDaily, startXManual and
stopXManual. Their 56 original body lines retain exact behavior; only indentation
and a local `ctx := r.Context()` binding differ. The router replaces those bodies
with four delegations; predicates/order/normalization and its remainder are exact.
Development/epoch admission, daily/manual consent, opt-out versus stop semantics,
error mappings, status envelopes and effect order remain distinct. Engine, Store,
shared security, consent decoding and telemetry are unchanged.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `1814c9b`, four development X HTTP adapters; seventh bounded HTTP extraction |
| Tested | Frozen characterization: 123 passing default/dev parent/subtest results before and after. Matching 14-package selection: default 2,542 pass/50 skip; dev 3,386 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted; successful native-start HTTP journeys, broader source and whole-package operational acceptance remain separate |

Review rechecked raw log summaries, original method bodies/router remainder,
source/test/configuration, helper and all 16 binary hashes. Matching validation
is reused without product test/build reruns. Context forwarding and branches
unproduced by the isolated HTTP fixture retain structural proof; no new public
factory seam or real native/browser startup was introduced. See the
[detailed result](cl1-cleanup-candidates.md#cl-33-http-development-x-controls-result-2026-11-03),
[validation](../runtime/qa/http-x-development-20261103/validation.json),
[review](../runtime/qa/http-x-development-review-20261103/review.json) and
[publication readback](../runtime/qa/http-x-development-review-20261103/final-readback.json).

Current router snapshot: 106 top-level arms, not endpoint count; 58 retained
(32 delegations and 26 thin/control/heartbeat arms) and 48 deferred (five
native/dev/telemetry and 43 product-domain arms). All 30 server.go declarations
and 52 central Engine declarations remain retained. CL-3.3 and CL-3 remain partial.

### Semantic correction follow-up

The then-proposed review and separately approved extraction are now complete;
see the semantic corrections result below.

## HTTP semantic corrections result (2026-11-03)

Three Server adapters are committed as `3c5e61a` in
`internal/httpapi/semantic_corrections.go`: semanticEventSuggestions,
correctSemanticEvent and undoSemanticCorrection. Their 36 original body lines
move unchanged except indentation and a local `ctx := r.Context()` binding.
Router-normalized `p` is passed unchanged; predicates, order, the intervening
Bridge passive-media-evidence arm and the whole router remainder are exact.
Engine operation locking, Store transactions/constraints/deltas/commit/cleanup,
shared security, decoder behavior, protocol, guards and cancellation are unchanged.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `3c5e61a`, three semantic correction HTTP adapters; eighth bounded HTTP extraction |
| Tested | Frozen characterization: 75 passing default/dev parent/subtest results before and after. Matching 14-package selection: default 2,617 pass/50 skip; dev 3,461 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted; broader source and whole-package operational acceptance remain open |

The isolated SQLite/Engine fixture covers suggestions null/empty shapes and
public fields, correction/undo output and parsing, caller contexts, transaction
rollback, ignored post-commit cleanup errors, committed effects after response
write failure, and real EnforceRetention eviction followed by undo/re-correction.
Review rechecked raw log summaries, source/helper/tag-graph identities, body and
router equivalence, and all 16 binary hashes. Matching validation is reused
without product test/build reruns. No new production test seam, concurrency,
race-detector run, activation or live trial was introduced. See the
[detailed result](cl1-cleanup-candidates.md#cl-33-http-semantic-corrections-result-2026-11-03),
[validation](../runtime/qa/http-semantic-corrections-20261103/validation.json),
[review](../runtime/qa/http-semantic-corrections-review-20261103/review.json) and
[publication readback](../runtime/qa/http-semantic-corrections-review-20261103/final-readback.json).

Current router snapshot: 106 top-level arms, not endpoint count; 61 retained
(35 delegations and 26 thin/control/heartbeat arms) and 45 deferred (five
native/dev/telemetry and 40 product-domain arms). All 30 server.go declarations
and 52 central Engine declarations remain retained. CL-3.3 and CL-3 remain partial.

### AI feedback follow-up

The then-proposed review and separately approved extraction are now complete;
see the AI feedback result below.

## HTTP AI feedback result (2026-11-03)

Three Server adapters are committed as `a3718ff` in
`internal/httpapi/ai_feedback.go`: addAIFeedback, aiFeedbackHistory and
undoAIFeedback. Their 31 original body lines are unchanged except indentation
and a local `ctx := r.Context()` binding. Router-normalized path, predicates,
dispatch order and the whole router remainder are exact. Engine/Store state,
operation/mu locks, cancellation, shared JSON/security helpers, protocol and
guards retain their existing ownership and behavior.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `a3718ff`, three AI-feedback HTTP adapters; ninth bounded HTTP extraction |
| Tested | Frozen characterization: 80 passing default/dev parent/subtest results before and after. Matching 14-package selection: default 2,697 pass/50 skip; dev 3,541 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted; broader source and whole-package operational acceptance remain open |

The isolated SQLite/Engine fixture covers target/scope/DTO contracts, append-only
supersedes/clear history, matching-account isolation, routing/parsing and failure
order, durable feedback after failed unsure handoff, independent review context,
same-session replacement and undo without cancelling a running review. Other-session
and media-provenance cancellation isolation retain the unchanged Engine regression
coverage. No real provider/native/browser call or new production test seam is
introduced. Review rechecked source/helper/raw-log/tag-graph and 16 binary identities;
matching evidence is reused without product test/build reruns. See the
[detailed result](cl1-cleanup-candidates.md#cl-33-http-ai-feedback-result-2026-11-03),
[validation](../runtime/qa/http-ai-feedback-20261103/validation.json),
[review](../runtime/qa/http-ai-feedback-review-20261103/review.json) and
[publication readback](../runtime/qa/http-ai-feedback-review-20261103/final-readback.json).

Current router snapshot: 106 top-level arms, not endpoint count; 64 retained
(38 delegations and 26 thin/control/heartbeat arms) and 42 deferred (five
native/dev/telemetry and 37 product-domain arms). All 30 server.go declarations
and 52 central Engine declarations remain retained. CL-3.3 and CL-3 remain partial.

### Selection-review follow-up

The proposed selection-review extraction is complete as `3f4a66d`; see the
Inbox selection-review result below.

## HTTP Inbox selection review result (2026-11-03)

Three Server adapters are committed as `3f4a66d` in
`internal/httpapi/selection_review.go`: correctSelection, reevaluateFailedRun
and undoSelectionCorrection. Their 42 original body lines are unchanged except
indentation and a local `ctx := r.Context()` binding. Router predicates, dispatch
order and normalized path are unchanged; the router remainder is exact. Lock,
transaction, worker and cancellation ownership remains in Engine/Store. No
protocol or guard is changed, and no production test seam is introduced.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `3f4a66d`, three Inbox selection-review HTTP adapters; tenth bounded HTTP extraction |
| Tested | Identical characterization payload and pass selection: 82 default/dev parent/subtest results before and after. Matching 14-package regression selection: default 2,779 pass/50 skip; dev 3,623 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted and code committed; whole-package, CL-3.3/CL-3 and operational acceptance remain open |

The isolated SQLite/real-Engine fixture uses a gated fake provider. It covers
201 correction/item, 202 run and 200 undo envelopes; header/field contracts;
route/security/ID parsing; JSON validation before admission; untrimmed candidate
lookup, duplicate fields and first-document decoding; ignored retry/undo bodies;
active-session priority; cancellation/deadline/store/SQL failures; duplicate and
repeated undo; safe replay guards; partial effects after commit and response-write
failure. Retry reuses durable capture without planning or a new capture command,
and its background context survives request cancellation. Existing Engine lock,
effect-order and AI/media-provenance cancellation coverage remains unchanged.

Review independently matched the baseline server to its committed source, verified
the exact moved feature and router remainder, and checked source/test/helper,
raw-log, tag-graph and all 16 PE/amd64 artifact identities. Passing evidence was
reused without product test/build reruns for this review or publication. No new
race-detector run, real provider/native/browser trial, activation or configuration
change is claimed. Evidence:
[validation](../runtime/qa/http-selection-review-20261103/validation.json),
[extraction proof](../runtime/qa/http-selection-review-20261103/extraction-proof.json),
[review](../runtime/qa/http-selection-review-review-20261103/review.json) and
[commit/publication readback](../runtime/qa/http-selection-review-review-20261103/final-readback.json).
Original receipt fields describe their capture time and remain unchanged.

Current router snapshot: 106 top-level arms, not endpoint count; 67 retained
(41 feature delegations and 26 thin/control/heartbeat arms) and 39 deferred
(five native/dev/telemetry and 34 product-domain arms). All 30 server.go functions
and 52 central Engine functions/methods remain retained. CL-3.3 and CL-3 remain
partial. Deferred groups are not an automatic extraction queue.

### Inbox read follow-up

The proposed Inbox read extraction is complete as `ce57e79`; see the Inbox
read result below.

## HTTP Inbox read result (2026-11-03)

Two Server adapters are committed as `ce57e79` in
`internal/httpapi/inbox.go`: inboxSessions and inboxRunTrace. Their 28 original
body lines are unchanged except indentation and a local `ctx := r.Context()`
binding. The trace adapter receives the already-normalized path. Router predicates,
dispatch order and the router remainder are exact. Engine/Store retain their
aggregation, projection, state and cancellation ownership; protocol and guards
are unchanged. No new interface, cache, transaction or production test seam is added.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `ce57e79`, two Inbox read HTTP adapters; eleventh bounded HTTP extraction |
| Tested | Identical characterization payload and pass selection: 70 default/dev parent/subtest results before and after. Matching 14-package regression selection: default 2,849 pass/50 skip; dev 3,693 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted and code committed; whole-package, CL-3.3/CL-3 and operational acceptance remain open |

The isolated SQLite/real-Engine fixture covers list defaults and newest-first
pagination, clamp/invalid/overflow/duplicate query behavior, empty arrays and
offsets beyond the total; trace stage normalization and five legal filters;
global counts versus filtered total/page; opaque candidate/correction/undo
projection and hidden evidence identities; active-session reads; security,
preflight, normalized paths and standard-mux redirects; ID/stage/lookup error
precedence; ignored GET bodies; caller cancellation/deadline, closed Store,
corrupt observations, late query failure and response-write error. Durable row
counts stay unchanged during successful and failed reads. Existing Store
canonical-identity, aggregation and privacy regression coverage remains unchanged.

The initial draft redirect expectation was corrected against local Go 1.26.10
net/http source before extraction. The frozen test uses a separate standard mux
as the redirect-code reference and checks the exact normalized Location, absent
API epoch and unchanged state. This is a test correction, not a router change.

Review reverified the committed baseline, exact moved feature, router remainder,
frozen characterization, pre-existing Go/config identities, raw logs and summaries,
four test graphs and all 16 PE/amd64 artifact identities. Passing evidence is
reused without product test/build reruns for review, documentation or publication.
No new race-detector run, live provider/native/browser trial, activation or
configuration change is claimed. Evidence:
[validation](../runtime/qa/http-inbox-20261103/validation.json),
[extraction proof](../runtime/qa/http-inbox-20261103/extraction-proof.json),
[review](../runtime/qa/http-inbox-review-20261103/review.json) and
[commit/publication readback](../runtime/qa/http-inbox-review-20261103/final-readback.json).
Original receipt fields describe their capture time and remain unchanged.

Current router snapshot: 106 top-level arms, not endpoint count; 69 retained
(43 feature delegations and 26 thin/control/heartbeat arms) and 37 deferred
(five native/dev/telemetry and 32 product-domain arms). All 30 server.go functions
and 52 central Engine functions/methods remain retained. CL-3.3 and CL-3 remain
partial. Deferred groups are not an automatic extraction queue.

### Model-usage follow-up

The proposed model-usage extraction is complete as `af8d722`; see the model
usage result below.

## HTTP model usage result (2026-11-03)

Two Server adapters are committed as `af8d722` in
`internal/httpapi/model_usage.go`: sessionModelUsage and aggregateModelUsage.
Their 21 original body lines are unchanged except indentation and a local
`ctx := r.Context()` binding. The session adapter receives the router-normalized
path. Router predicates, dispatch order and the router remainder are exact.
Engine/Store retain ledger reads, grouping, totals, time-window selection and
state/cancellation ownership; protocol and guards remain unchanged. No new
interface, cache, transaction or production test seam is added.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `af8d722`, two model-usage HTTP adapters; twelfth bounded HTTP extraction |
| Tested | Identical characterization payload and pass selection: 74 default/dev parent/subtest results before and after. Matching 14-package regression selection: default 2,923 pass/50 skip; dev 3,767 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted and code committed; whole-package, CL-3.3/CL-3 and operational acceptance remain open |

The isolated SQLite/real-Engine fixture covers session versus aggregate DTOs,
four/six-category order, empty entry arrays, omitted scope-specific fields and
nullable versus zero token counters. Complete/partial/unavailable coverage,
receipt metadata/timing, chronological session entries, aggregate grouping and
async Living Topics are preserved without double counting cached/reasoning
breakouts. Time fixtures distinguish session-created membership from standalone
topic-receipt membership; timestamps are bounded by the request rather than
fixed snapshots. HTTP default/clamp/allowlist and duplicate-query order,
ID/lookup/window error precedence, missing session, ignored GET body, security,
preflight, standard-mux redirects, cancellation/deadline, closed Store, late
query and response-write failures retain their existing behavior. Durable row
counts remain unchanged during successful and failed reads.

Review reverified the committed baseline, exact moved feature, router remainder,
frozen characterization, pre-existing Go/config identities, raw logs and summaries,
four test graphs and all 16 PE/amd64 artifact identities. Passing evidence is
reused without product test/build reruns for review, documentation or publication.
No new race-detector run, live provider/native/browser trial, activation or
configuration change is claimed. Evidence:
[validation](../runtime/qa/http-model-usage-20261103/validation.json),
[extraction proof](../runtime/qa/http-model-usage-20261103/extraction-proof.json),
[review](../runtime/qa/http-model-usage-review-20261103/review.json) and
[commit/publication readback](../runtime/qa/http-model-usage-review-20261103/final-readback.json).
Original receipt fields describe their capture time and remain unchanged.

Current router snapshot: 106 top-level arms, not endpoint count; 71 retained
(45 feature delegations and 26 thin/control/heartbeat arms) and 35 deferred
(five native/dev/telemetry and 30 product-domain arms). All 30 server.go functions
and 52 central Engine functions/methods remain retained. CL-3.3 and CL-3 remain
partial. Deferred groups are not an automatic extraction queue.

### Vision-evaluation follow-up

The proposed vision-evaluation extraction is complete as `cf534ec`; see the
vision evaluation result below.

## HTTP vision evaluation result (2026-11-03)

Two Server adapters are committed as `cf534ec` in
`internal/httpapi/vision_evaluation.go`: visionEvaluationJobs and
retryVisionEvaluation. Their 22 original body lines are unchanged except
indentation and a local `ctx := r.Context()` binding. Both receive the
router-normalized path. Router predicates, dispatch order and the router
remainder are exact. Engine/Store retain queue state, capacity, promotion and
cancellation ownership; protocol and guards remain unchanged. No new interface,
transaction, worker or production test seam is added.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `cf534ec`, two vision-evaluation HTTP adapters; thirteenth bounded HTTP extraction |
| Tested | Identical characterization payload and pass selection: 56 default/dev parent/subtest results before and after. Matching 14-package regression selection: default 2,979 pass/50 skip; dev 3,823 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted and code committed; whole-package, CL-3.3/CL-3 and operational acceptance remain open |

The isolated SQLite/real-Engine fixture uses the existing Store clock injection
and a counting offline provider. It covers run/source list filtering, FIFO order,
DTO/manual labels, empty arrays, malformed candidate tolerance and unknown-field
projection; failed-only retry, active-session behavior, source-wide capacity
across runs and older/tied deferred priority; exact field reset/retention;
independently committed peer promotion after a target SQL or zero-row failure;
route/security/preflight/standard-mux behavior, ignored bodies, ID validation,
cancellation/deadline, closed Store and response-write failure. Provider, capture
command and reasoning-invocation counters remain zero. Existing Store queue and
Engine vision-lane regression coverage remains unchanged.

The current product path remains a media-only queue/manual review based on
metadata. HTTP 202 records requeue acceptance; it does not establish a running
evaluator or successful pixel analysis. This cleanup adds no model invocation.

Review reverified the committed baseline, exact moved feature, router remainder,
frozen characterization, pre-existing Go/config identities, raw logs and summaries,
four test graphs and all 16 PE/amd64 artifact identities. Passing evidence is
reused without product test/build reruns for review, documentation or publication.
No new race-detector run, live provider/native/browser trial, activation or
configuration change is claimed. Evidence:
[validation](../runtime/qa/http-vision-evaluation-20261103/validation.json),
[extraction proof](../runtime/qa/http-vision-evaluation-20261103/extraction-proof.json),
[review](../runtime/qa/http-vision-evaluation-review-20261103/review.json) and
[commit/publication readback](../runtime/qa/http-vision-evaluation-review-20261103/final-readback.json).
Original receipt fields describe their capture time and remain unchanged.

Current router snapshot: 106 top-level arms, not endpoint count; 73 retained
(47 feature delegations and 26 thin/control/heartbeat arms) and 33 deferred
(five native/dev/telemetry and 28 product-domain arms). All 30 server.go functions
and 52 central Engine functions/methods remain retained. CL-3.3 and CL-3 remain
partial. Deferred groups are not an automatic extraction queue.

### Timeline follow-up

The proposed Timeline response extraction is complete as `3cf90b4`; see the
Timeline composition result below.

## HTTP Timeline composition result (2026-11-03)

One Server adapter is committed as `3cf90b4` in `internal/httpapi/timeline.go`:
timeline. Its 19 original body lines are unchanged except indentation and a local
`ctx := r.Context()` binding. Route predicate, dispatch order and router remainder
are exact. The ordered calls remain Timeline, LatestTimelineCheck,
TimelineBatchSummaries and AutoUpdateStatus, followed by the original response.
Engine/Store retain selection, projection, summaries, expiry and state ownership;
protocol and guards remain unchanged. No new interface, cache, transaction,
concurrency or production test seam is added.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `3cf90b4`, one Timeline response-composition adapter; fourteenth bounded HTTP extraction |
| Tested | Identical characterization payload and pass selection: 46 default/dev parent/subtest results before and after. Matching 14-package regression selection: default 3,025 pass/50 skip; dev 3,869 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the test and produce 16 artifacts |
| Activated | Not performed for this increment |
| Acceptance | Scoped source/test review accepted and code committed; whole-package, CL-3.3/CL-3 and operational acceptance remain open |

The isolated SQLite/real-Engine fixture uses the existing Store clock injection.
It covers the four-part envelope, headers and mode-dependent null/empty item
arrays; API defaults versus configured Timeline capacity; clamp/invalid/overflow
and duplicate query behavior; collapsed/hide/show-all pagination and summaries;
unpaged latest-check/batch scope; bounded personal-memory projection; hidden
prepared/expired batches; exact and idempotent expiry with Auto Update off;
errors before and after expiry, queue-vacancy failure, context/deadline/closed
Store, security/preflight/standard-mux behavior, ignored body and response-write
failure. Existing Store selection, lifecycle and projection coverage remains.

AutoUpdateStatus remains last and can persist expiry and queue vacancy even
with Auto Update off. A failed vacancy write leaves expiry committed; a later
read does not retry that write if no new batch expires. This existing behavior
is characterized and retained, not repaired by this extraction. Earlier errors
do not reach expiry; later errors and response-write failures do not roll back
already committed effects. The new HTTP fixture starts no scheduler or live
capture and uses only an offline deterministic provider.

Review reverified the committed baseline, exact moved feature, router remainder,
frozen characterization, pre-existing Go/config identities, raw logs and summaries,
four test graphs and all 16 PE/amd64 artifact identities. Passing evidence is
reused without product test/build reruns for review, documentation or publication.
No new race-detector run, live provider/native/browser trial, activation or
configuration change is claimed. Evidence:
[validation](../runtime/qa/http-timeline-20261103/validation.json),
[extraction proof](../runtime/qa/http-timeline-20261103/extraction-proof.json),
[review](../runtime/qa/http-timeline-review-20261103/review.json) and
[commit/publication readback](../runtime/qa/http-timeline-review-20261103/final-readback.json).
Original receipt fields describe their capture time and remain unchanged.

Current router snapshot: 106 top-level arms, not endpoint count; 74 retained
(48 feature delegations and 26 thin/control/heartbeat arms) and 32 deferred
(five native/dev/telemetry and 27 product-domain arms). All 30 server.go functions
and 52 central Engine functions/methods remain retained. CL-3.3 and CL-3 remain
partial. Deferred groups are not an automatic extraction queue.

## Automatic HTTP feature separation result (2026-11-03)

The owner expanded the previously incremental approval into an automatic run to
finish the remaining HTTP feature separation. This supersedes the pending
content-context-only proposal for this bounded workstream; it does not authorize
frontend, Engine, Store, runtime, configuration or protocol changes.

Code commit: `9da6ed8`. All 58 remaining inline top-level router adapters move
into 24 feature file boundaries. The 32 formerly deferred arms and 26 previously
retained thin/control/heartbeat arms are now delegated. All 106 top-level switch
arms delegate to Server feature handlers; this is an arm count, not endpoint
count. The earlier split-capture dispatch remains unchanged. The 935 original
body lines retain exact statements and comments except indentation and a local
request-context binding. The exclusive feedbackTraceID helper moves verbatim
with Timeline feedback.

server.go now has 623 lines and 29 declarations; its composition, HTTP lifecycle,
dispatch, shared security/authentication and serialization helpers remain. The
original file had 1,521 lines and 30 declarations. Existing feature declarations
are unchanged except imports. All other tracked Go files, Engine/Store behavior,
user configuration and unrelated work remain preserved. No new interface,
transaction, cache, scheduler, worker, state owner or compatibility change is
introduced.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `9da6ed8`: 58 remaining adapters in 24 feature boundaries; central HTTP handler separation is source-complete |
| Tested | Identical frozen characterization payload and pass names: 89 default/dev parent/subtest results before and after; 14-package regression (13 tested, Bridge builder compile-only): default 3,114 pass/50 skip, development 3,958 pass/62 skip, zero failures; four Windows amd64 tag combinations include the new test and produce 16 artifacts |
| Activated | Not performed; no restart, provider/browser trial or runtime/configuration change |
| Acceptance | Scoped source/test review accepted; central-router handler separation complete. Broader HTTP infrastructure, CL-3, repository integration and operational acceptance remain separate/open |

The new isolated SQLite/real-Engine fixture adds validation/guard precedence,
strict path/query parsing, public read envelopes, development/runtime-control
gates, request-context forwarding, native-reader/control cancellation, feedback
trace headers, content-context feedback/append-only undo, diagnostics attachment
and error behavior, plus effects retained after feedback/full-reset response
write failure. Existing successful onboarding/calibration/settings/credential,
Library/Living Topic/retention, Auto Update, Bridge owner/rotation/heartbeat and
native-reader tests remain included. The new fixture uses an offline provider;
no live provider or browser trial is claimed.

Reconstruction verifies every moved body, all predicates and dispatch order,
every pre-existing feature declaration and the entire router remainder, including
middleware and lifecycle. Heartbeat retains its lock through response, telemetry
retains authenticated batch serialization and partial effects, runtime shutdown
is scheduled only after successful response writing, and feedback timing/logging
order is unchanged. Characterization fixture mistakes were corrected before
production extraction; they do not represent production fixes or relaxed guards.

Evidence: [validation](../runtime/qa/http-features-20261103/validation.json),
[exact extraction proof](../runtime/qa/http-features-20261103/extraction-proof.json),
[scoped review](../runtime/qa/http-features-20261103/review.json), and
[commit/publication readback](../runtime/qa/http-features-20261103/final-readback.json).
Original validation fields sourceHead/sourceDirty/committed describe capture
time and are not overwritten by later commits/publication. No race-detector or
runtime acceptance result is inferred from ordinary regression/Windows builds.

### Completion boundary and next decision

The automatic HTTP handler separation requested by the owner is complete.
There is no remaining central inline feature-handler queue. Shared infrastructure
and existing feature subrouters remain intentional owners rather than deletion
or mandatory micro-extraction candidates. This closes this HTTP source boundary,
not the entire CL-3 or comprehensive cleanup roadmap. A future read-only CL-3
closure review can classify retained infrastructure and genuinely deferred
application boundaries before proposing any further implementation. Activation
and operational qualification still require their own authorized task.

## Engine acquisition execution result (2026-11-03)

The owner separately approved the acquisition-planning orchestration extraction
after the read-only CL-3 status review. Code `8027599` moves the original
95-line planning block from session_execution.go into executeAcquisitionPlanning
in internal/engine/acquisition_execution.go. The caller invokes it at the same
position, after continuity/vision handling and before Knowledge/candidate
evaluation. Only return tuples and caller control-flow adaptation change:
successful follow-up returns true/nil, an error returns false/original error,
and continuation into evaluation returns false/nil. The bool is used only when
the error is nil; a committed queue followed by receipt failure remains an error.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `8027599`: one acquisition orchestration helper; session_execution.go is 347 lines with six existing declarations; acquisition_execution.go is 111 lines with one helper |
| Tested | Frozen characterization payload and pass names match before/after: 230 default/dev parent/subtest results, comprising the new 14-scenario model-path test plus retained policy/daily-gate/telemetry/fallback suites. Matching 14-package regression (13 tested, Bridge builder compile-only): default 3,129 pass/50 skip; dev 3,973 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the new test and produce 16 artifacts |
| Activated | Not performed; no runtime restart, provider/browser trial or configuration change |
| Acceptance | Scoped source/test review accepted and code committed; broader Engine package, CL-3 and operational acceptance remain open. Central HTTP handler separation remains complete |

The daily-source query still executes before the planning gates. Local/model
decisions, bounded continuation, configured profile dispatch, stage persistence,
background telemetry, queue insertion and receipt persistence retain their order.
No lock is introduced or moved. Caller context, original raw error identities
and wrapping, shared Coverage aliases, protocol and guards remain unchanged.
Telemetry failure remains nonfatal; cancellation/deadline/authentication errors
do not become incomplete-response fallback. Partial planner output is
never used to queue a follow-up. A receipt failure after queue commit preserves
the queued work and stops evaluation with the original error.

The new characterization test exercises the existing process entry point using
real isolated SQLite and an offline provider. Disposable triggers observe
stage/telemetry/queue/receipt/evaluation order and inject stage/queue/receipt/
telemetry failures. It also covers finish/follow-up, continuation-not-ready,
incomplete fallback, raw provider error identity, joined cancellation/deadline
and caller cancellation. Existing policy, daily issuance/forged-coverage,
planning-disabled/single-round and capture-telemetry tests remain in the selection.

Publication review reverified the committed baseline, reverse-adapted block,
exact caller remainder, unchanged unrelated Go/config files, raw log counts and
pass-name selection, four tag graphs, and all 16 PE/amd64 artifact identities.
No product tests/builds were repeated for review or publication because their
relevant inputs are unchanged. No new race-detector or live-source result is
claimed. The 52 central Engine declarations remain intentionally retained;
this helper does not reopen that facade checkpoint or close the whole package.

Evidence: [validation](../runtime/qa/acquisition-execution-20261103/validation.json),
[extraction proof](../runtime/qa/acquisition-execution-20261103/extraction-proof.json),
[publication review](../runtime/qa/acquisition-execution-review-20261103/review.json)
and [commit/publication readback](../runtime/qa/acquisition-execution-review-20261103/final-readback.json).
Original sourceHead/sourceDirty/committed receipt fields describe capture time
and remain unchanged; the later readback binds the validated payload to commits.

### Candidate evaluation follow-up

The proposed per-run evaluation/completion extraction is now implemented as
`ed35d82`; see the result below.

## Engine candidate evaluation result (2026-11-03)

The owner approved one candidate-evaluation/run-completion extraction after
read-only analysis, then authorized review, roadmap synchronization and
publication. Code `ed35d82` moves 69 original body lines into
evaluateCandidatesAndCompleteRun in internal/engine/candidate_evaluation.go.
The moved statements/comments are exact; the helper only adds a terminal nil
return after the unchanged added timing. The caller delegates at the original
position and retains startNext and its original error return.

| Dimension | Current evidence |
| --- | --- |
| Implemented | `ed35d82`: one per-run evaluation/completion helper; session_execution.go is 280 lines with six existing declarations; candidate_evaluation.go is 84 lines with one helper |
| Tested | Frozen characterization payload and pass names match before/after: 88 default/dev parent/subtest results. Matching 14-package regression (13 tested, Bridge builder compile-only): default 3,159 pass/50 skip; dev 4,003 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the new test and produce 16 artifacts |
| Activated | Not performed; no runtime restart, provider/browser trial or configuration change |
| Acceptance | Scoped source/test review accepted and code committed. Broader Engine package, CL-3 and operational acceptance remain open; central HTTP handler separation remains source-complete |

The sequence remains source-local Knowledge (limit 200), evaluation stage,
configured profile dispatch, background telemetry/evaluated timing, source URL
binding and evidence validation, preference projection, prior evidence/events,
continuity exceptions and protected selection, selected timing, Timeline mapping,
CompleteRun transaction and added timing. startNext stays in process, so the
existing schedule lock, session finalization and worker/error-recovery owners
do not move. No new lock, transaction, worker, interface or state owner is added.
Caller/background contexts, raw error identity/wrapping, Coverage aliases,
protocol and guards remain unchanged.

The real-SQLite/offline-provider characterization observes stage, telemetry,
timing, preference, assessment/item writes and completion order. It covers
Knowledge/stage/provider/validation/preference/lookup/completion failures,
best-effort telemetry/timing, caller cancellation, source-scoped prior delivery
and event selection, material updates/contradictions, changed/cooldown resurfacing,
weak-evidence rejection, routed evaluation profile and the 200-item Knowledge
cap/order. A held schedule lock proves run completion precedes advancement;
cancellation and finalization failure after commit preserve run output/timing.
Existing reasoning-result, continuity/event, preference and progressive/full-wait
tests remain in the characterization selection.

Preference/cache effects preceding a later failure remain committed. CompleteRun
write failure rolls back its assessment/item/run transaction while earlier
telemetry/timing/cache effects survive; no advancement follows that error.
Advancement failure after successful completion does not roll back completed
run output. These existing partial-error contracts are characterized, not fixed.

Publication review reverified the committed baseline, exact moved block and
caller remainder, frozen source/test bytes, raw log counts and matching pass
names, four tag graphs and all 16 PE/amd64 artifact identities. Passing tests
and builds were reused for review/publication because inputs were unchanged.
No new race-detector or live-source qualification is claimed. All other tracked
Go/config files are preserved. The 52 central Engine declarations and 106
delegated top-level HTTP arms remain unchanged; this closes only this seam.

Evidence: [validation](../runtime/qa/candidate-evaluation-20261103/validation.json),
[extraction proof](../runtime/qa/candidate-evaluation-20261103/extraction-proof.json),
[publication review](../runtime/qa/candidate-evaluation-review-20261103/review.json)
and [commit/publication readback](../runtime/qa/candidate-evaluation-review-20261103/final-readback.json).
Original sourceHead/sourceDirty/committed receipt fields describe capture time
and remain unchanged; the later readback binds the payload to commits.

### Vision handoff follow-up completed

The proposed read-only review is complete. Retain the compact vision handoff
inside Engine.process: Store already owns queue capacity, deferred promotion,
deduplication and persistence; vision_lane.go owns pure routing/projection.
Another Engine helper would add indirection without separating an owner.
The owner approved characterization tests instead of a production extraction.

## Vision lane persistence characterization result (2026-11-03)

The owner approved review, roadmap synchronization, commit and publication of
this test-only increment to origin/main at https://github.com/abangkis/AkuSidecar.git.
The preceding published checkpoint is 7b312ff. Test commit 70ea120 adds one file;
production remains at the preceding candidate-evaluation source ed35d82.

| Dimension | Evidence and scope |
| --- | --- |
| Implemented | 70ea120: vision_lane_persistence_test.go adds three test roots with six scenarios (nine parent/subtest results). No production extraction; the vision handoff remains in Engine.process |
| Tested | Matching frozen test/source/log/artifact hashes reviewed. Focused Store/Engine characterization: 94 pass in each default/dev mode. Fourteen-package regression (13 tested; Bridge builder compile-only): default 3,168 pass/50 skip; dev 4,012 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the new test and produce 16 PE/amd64 artifacts |
| Activated | Not performed; no runtime, configuration, restart or live trial change |
| Acceptance | Scoped characterization review accepted with no findings. Retain decision recorded; CL-3 remains partial. Broader package, integration and operational acceptance remain open |

The tests exercise real Engine.process and Store methods with a deterministic
provider. Fixture SQL initializes historical state and injects INSERT failures;
it does not replace the queue operations under test. The assertions establish:

- Earlier enqueues survive a later insertion failure; failure stops before a
  success receipt, text reasoning, assessments, Timeline delivery or follow-up.
- A deferred job in another session/run is promoted before insertion and keeps
  that promotion if the new insert fails. Its occupied source-wide capacity
  makes an earlier successful new insert deferred when the second insert fails.
- Cross-run deduplication preserves the complete historical row, including owner,
  retry metadata and JSON bytes/storage types. The new run gets no duplicate job
  and a zero-count run-scoped receipt while the source-wide summary retains the
  retrying job. Mixed text reasoning observes the durable receipt first; media-only
  completion still reports input candidate count rather than newly inserted jobs.

All 1,447 pre-existing tracked files were hash-identical at validation and review,
including production, protocol, guards, previous tests and the already dirty
user configuration. Only this new test and four approved roadmap documents are
committed by this publication. Existing .tmp/ and experiments/ remain excluded.
The new results extend the previous candidate-evaluation and vision suites;
they do not replace their historical before/after evidence. No race-detector or
independent summary-query failure injection is claimed.

Evidence: [validation](../runtime/qa/vision-lane-persistence-20261103/validation.json),
[scoped review](../runtime/qa/vision-lane-persistence-review-20261103/review.json),
and [commit/publication readback](../runtime/qa/vision-lane-persistence-review-20261103/final-readback.json).
These are local ignored QA receipts. Original sourceHead/sourceDirty/committed
fields describe capture time and remain unchanged; the later readback binds
the frozen tested payload to commits and the verified publication target.

### Observation preparation and empty-candidate follow-up completed

Read-only review retained observation preparation in Engine.process: accepted
observation loading, merge/durable coverage, follow-up yield and continuity
classification/filtering form one ordered orchestration path. Pure helpers
already have capture_rounds.go and continuity.go owners. Vision handoff remains
retained under the previous decision. The owner separately approved extracting
only empty-candidate run completion after characterization on the original code.

## Empty-candidate completion result (2026-11-03)

The owner approved review, roadmap synchronization, commit and publication of
this bounded increment to origin/main at https://github.com/abangkis/AkuSidecar.git.
The preceding published code/roadmap checkpoint is 367ef89, including vision
persistence characterization 70ea120. Production/test code is now de95c58.

| Dimension | Evidence and scope |
| --- | --- |
| Implemented | de95c58: 27 original body lines moved into completeRunWithoutCandidates in empty_candidate_completion.go (42 lines); session_execution.go is 255 lines with its six existing declarations. Candidate-count predicate and startNext remain in process |
| Tested | Frozen characterization payload and pass names match before/after: 218 default/dev parent/subtest results each. Three new roots cover 16 scenarios (19 parent/subtest results). Matching 14-package regression (13 tested; Bridge builder compile-only): default 3,187 pass/50 skip; dev 4,031 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the new test and produce 16 PE/amd64 artifacts |
| Activated | Not performed; no runtime, configuration, restart or live trial change |
| Acceptance | Scoped source/test review accepted with no findings. Observation preparation and vision handoff retained; empty-candidate completion separated. CL-3 remains partial; broader package, integration and operational acceptance remain open |

The helper preserves Engine receiver and Store authority; no service, interface,
new state owner, lock or transaction was introduced. The caller keeps scheduling.
The original effect order remains: create a missing bypass receipt, select the
existing summary, CompleteRun with caller context, then best-effort added timing
with context.Background(). CompleteRun errors remain unwrapped; bypass errors
retain their existing prefix; timing errors remain ignored. Existing receipt
maps and present null values are not replaced. Media-only summary still counts
input candidates and takes precedence over the resurfaced summary.

Characterization uses real process/Store operations with a deterministic provider;
fixture SQL initializes only disposable history and failure/effect probes. It
checks singular/plural summaries, combined resurface/media input, map/null receipt
preservation, second-round yield, ordered durable side effects, bypass/completion
failure, ignored timing failure, caller cancellation, the schedule lock and
post-commit cancellation/finalization failure without losing committed output.
Existing nonempty candidate, capture-round and vision tests remain in the suite.

Review independently compares the moved body and caller remainder with Git source
367ef89 and verifies frozen source/test/log/graph/helper/artifact hashes. The test
payload passed before production changed and remains byte-identical afterward.
All 1,447 other previously tracked files were unchanged at validation and review,
including existing tests, protocol, guards and the already dirty user config.
Only the three approved Engine source/test files and four roadmap documents are
committed by this publication; .tmp/ and experiments/ remain excluded. The 52
retained central Engine declarations and 106 delegated top-level HTTP arms are
unchanged. No race-detector, real-provider or broader runtime acceptance is claimed.

Evidence: [validation](../runtime/qa/empty-candidate-completion-20261103/validation.json),
[extraction proof](../runtime/qa/empty-candidate-completion-20261103/extraction-proof.json),
[scoped review](../runtime/qa/empty-candidate-completion-review-20261103/review.json),
and [commit/publication readback](../runtime/qa/empty-candidate-completion-review-20261103/final-readback.json).
These are local ignored QA receipts. Original sourceHead/sourceDirty/committed
fields describe capture time and remain unchanged; the later readback binds the
frozen tested payload to commits and the verified publication target.

### Proposed next cleanup analysis

Read-only CL-3.2 closure review of Engine read-model and enrichment ownership:
trace retained Session, Run, Timeline and Inbox read entry points to Store and
map enrichment calls to their existing feature owners. Classify actual mixed
responsibilities versus required facade/delegation, using current consumers and
tests. Produce an explicit remaining/retained map and at most one justified
next seam with characterization requirements; a retain-only outcome is valid.
Do not repeat central-file splitting or the completed process extractions.
Preserve Store authority, lock/context/cancellation, effect/error order, protocol
and guards. No implementation, configuration change, activation or trial is
authorized by this proposal.
