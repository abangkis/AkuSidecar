# Roadmap status and evidence index

Status synchronized: 2026-11-03 (Asia/Jakarta).
Reviewed local source: AkuSidecar `f27e91b` (reviewed automatic Store feature separation; 95 declarations/15 feature files; after Store Timeline read-model `bee6a9f`, after empty-candidate completion `de95c58`, after vision persistence `70ea120`; per-run candidate evaluation/completion, after acquisition planning orchestration, after automatic remaining HTTP handler separation, after Timeline composition HTTP adaptation, vision-evaluation HTTP adapters `cf534ec`, model-usage HTTP adapters `af8d722`, Inbox read HTTP adapters `ce57e79`, Inbox selection-review HTTP adapters `3f4a66d`, after AI-feedback HTTP adapters `a3718ff`, after semantic correction HTTP adapters `3c5e61a`, development X HTTP controls `1814c9b`, capture telemetry rotation/shutdown compatibility `237dbd1`, HTTP Bridge reload actions `6ea303c`, capture-owner path guard `b482ba2`, HTTP bootstrap/router map `97bbe51`, HTTP media-recapture entry points `90e3a7f`, HTTP update/session lifecycle `d32c065`, Engine passive media evidence `2bdc0f1`, Library removal/Forget `a19bf86`, FullReset `da019dd`, AI feedback `6a1ce33` and selection corrections `f3d46b8`, vision-lane routing/coverage `25c3339`, local acquisition policy `f16f11a`, capture-round separation and deterministic Store privacy test `683b1e8`, reasoning result `eb05ac8`, process recovery `5b39fa4`, observation contracts `d9c3025`, session admission `4bf0731`, AI detection `b84a62d`, media provenance `8d7f7b6`, Engine Bridge commands `3e13434`, Engine media recapture `c614512`, HTTP recapture `be9fcd2`, HTTP commands `7cb712b`, Bridge status `7b2d0d8` and diagnostics export `1068e35`),
AkuBrowser `3a3c043`, AkuBridge `3676f5c`.
This is a documentation checkpoint, not a runtime inspection or release.
The preceding published checkpoint is `6b9b3a0`, including automatic Store
feature separation `f27e91b`. The owner accepted the read-only CL-3 closure
review, then authorized documentation synchronization, commit and push to
origin/main at https://github.com/abangkis/AkuSidecar.git. The prior Store
commit/publication readback below retains its historical identity; this update
records source closure without changing code, runtime or trial state.
Publication does not establish runtime activation or operational acceptance.
The Browser worktree contains pre-existing build/cache changes; those changes
are outside this synchronization and are not evidence of a released tuple.

Current decision: source CL-3 is closed following the owner-accepted read-only
responsibility-map review at Sidecar `6b9b3a0` (code through `f27e91b`). See
[CL-3 source closure](code-cleanup-roadmap.md#cl-3-source-closure-2026-11-03) for the exit-criteria assessment,
retained responsibilities and explicit deferral triggers. Earlier partial/open
statements below describe their original checkpoints and are superseded only
for this backend source gate. Activation and operational acceptance are not
established by this closure; other cleanup stages retain their own status.

## Status vocabulary

| Dimension | Meaning | Does not establish |
| --- | --- | --- |
| Implemented | Code exists at the named commit, or an explicitly identified historical contract records implementation | Test success, deployment or provider qualification |
| Tested | The named source/artifact passed the stated offline, synthetic or local checks | A current running binary, a clean-machine install or real-provider acceptance |
| Activated | A dated receipt identifies the artifact used in a running process | Activation of later commits, continuous current health or a clean source build |
| Acceptance | The stated source, product or operational gate has reviewed evidence and scope; source acceptance does not establish activation | Acceptance of another source, mode, release or broader reliability claim |

`Not recorded` means evidence was not established here, not proof that an
operation never happened. `Pending`, `frozen` and `deferred` preserve the existing
scope decisions. Test/build success never upgrades activation or acceptance.
Historical sections in linked documents retain their original evidence scope;
this checkpoint takes precedence for current status wording.

## Collection and source sub-roadmaps

| Workstream | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Daily native X | Existing daily controller/owner and `ab2dc40` diagnostics | Existing lifecycle suites and dated real-trial receipts | Historical `41ecd1c` daily-MVP receipt and later `ab2dc40` trial artifact; current process not inspected | Selected main-dev MVP accepted on 2026-10-29; one later X-only repeat completed on 2026-11-01. Not full rotation or mixed X/IG qualification |
| X/IG convergence stages 1-3 | Shared cleanup/deadlines, ownership-aware dispatch, pre-claim expiry, readiness and capability projection exist in source | Offline contracts and mixed-source synthetic coverage recorded in [foundation progress](shared-capture-foundation-progress.md) | IG diagnostic artifact reached `32b8552`; a complete paired Sidecar/Bridge rollout of every convergence change is not certified by that receipt | Contract/baseline boundary covered; repeated real mixed-source acceptance remains pending |
| IG bootstrap and document diagnostics, stage 4 | Shared bootstrap verifier, continuous observer and method/caller diagnostic in `32b8552` | Historical Node/Go suites; accountless Chrome 156 fixture captured both History methods and retained invalidation | One IG-only trial used the recorded `32b8552` artifact; provenance says `sourceDirty=true` | Failed: reader `historyApi` invalidation during `readiness`; native daily IG remains unqualified; no caller attribution in the final receipt |
| Daily acceptance and obsolete-path retirement, stage 5 | Existing daily machinery and bounded cleanup extractions; not a wholesale retirement | Synthetic/repeated-batch checks exist | No activation of the 2026-11-01/02 cleanup commits recorded | Repeated real X/IG batches, restart/cancel/consent/reader acceptance remain pending |
| Broader browser/headless parity | Existing Manager/Coordinator, driver and handoff implementation | Historical component, source-parity and selected Windows evidence | Historical selected-mode evidence only | General integrated handoff, reader/login and full media/package parity are not all closed; do not reset the accepted X MVP |
| Shared capture reservation | Proposed only | Conservative predecessor-drain tests are not reservation tests | Not recorded | Deferred; not another gate for the accepted X MVP |

Detailed scopes: [collection roadmap](collection-modes-roadmap.md),
[X/IG convergence](x-ig-convergence-roadmap.md),
[IG evidence and diagnostics](instagram-bootstrap-daily-proof.md), and
[reservation proposal](shared-capture-reservation-plan.md).

## Cleanup and modularization

Execution proposal: [comprehensive code cleanup roadmap](code-cleanup-roadmap.md).
Its CL-0 preserves the eight completed increments below. CL-1 through CL-8 cover
consumer inventory, proven obsolete-path retirement, modularization, packaging
and final source acceptance. Creating the roadmap does not authorize code
changes or operational work. CL-1 has now delivered a
[consumer/build/package inventory](cl1-consumer-inventory.md) and
[32-boundary candidate ledger](cl1-cleanup-candidates.md). Read-only metadata
and reference checks are recorded separately from product testing. The first
CL-2 proposal was the unused `DeleteMemoryItem` alias only; it has since been
separately approved, removed, validated and committed with the roadmap as `21ac9ad`. Its
[result record](cl1-cleanup-candidates.md#cl-2-g10-result-2026-11-01) separates
implementation/testing from commit, activation and operational acceptance.
G06 was subsequently approved and relocated without behavior changes. Its
separately approved smoke-test context correction resolves the confirmed baseline
failure without changing repository config or guards. Both changes were reviewed
and committed as `d82f53b`; latest default/dev regressions and Windows compilation pass. See the
[G06 result](cl1-cleanup-candidates.md#g06-result-2026-11-01).
E01 then added candidate cleanup and the unchanged Settings execution file
boundary, reviewed and committed as `7d00d9c` and `02e063e`. The
[E01 result](cl1-cleanup-candidates.md#e01-candidate-cleanup-and-settings-execution-2026-11-01)
records their distinct source parents, passing evidence and remaining scope.
The later profile merge `aea3867` and cancellation validation detour `edb246d`
are also reviewed/committed. Their
[result](cl1-cleanup-candidates.md#e01-profile-merge-and-cancellation-validation-detour-2026-11-01)
preserves the original non-green receipt and records the latest combined green
suite without treating a source cleanup as runtime acceptance.
External-consumer and offline Browser dependency gaps remain explicit.

Executable-change preparation is now extracted in `c303ed4`; the independently
approved offline Node timing correction is committed as `8dc3f5e`. Their
[review and result](cl1-cleanup-candidates.md#e01-executable-preparation-and-node-fixture-validation-detour-2026-11-01)
maps the earlier failing receipt to the latest combined passing payload. CL-3
remains partial; neither commit establishes runtime activation or acceptance.

CL-3.2 now separates the complete diagnostics export feature as `1068e35`.
Its [result](cl1-cleanup-candidates.md#cl-32-diagnostics-export-feature-result-2026-11-01)
records characterization before the move, unchanged declarations and its
recorded combined passing suites. The provider-selection micro-extraction proposal was
subsequently declined: keep the existing 98-line `SaveSettings` cohesive rather
than adding another single-caller wrapper. No provider-selection helper was added.

CL-3.2 also separates Bridge status/compatibility as `7b2d0d8`, with unchanged
constants, action catalog, lock scopes and heartbeat side effects. Its
[result](cl1-cleanup-candidates.md#cl-32-bridge-status-feature-result-2026-11-01)
records its characterization/regression/Windows evidence. This is a
Sidecar backend increment, not completion of CL-6 extension responsibility work.

CL-3.3 now separates the four HTTP Bridge command handlers as `7cb712b` in
`bridge_commands.go`. Its
[result](cl1-cleanup-candidates.md#cl-33-http-bridge-command-feature-result-2026-11-02)
maps the pre-commit validation to the reviewed commit. Route predicates and
normalization remain in `server.go`; existing context, authentication, protocol,
guards and claim/trace/result ordering are unchanged. This completes one HTTP
feature boundary, not all HTTP capture work or CL-3.

CL-3.3 also separates Bridge media-recapture claim, observation and failure as
`be9fcd2` in `bridge_media_recapture.go`. Its
[result](cl1-cleanup-candidates.md#cl-33-http-bridge-media-recapture-feature-result-2026-11-02)
records the identical passing characterization payload, current regression/build
evidence and reviewed commit. Claim retains 404/409/204/200 outcomes; POST Engine
errors retain 400 and successful results 200. Failure still accepts queued or
claimed jobs, while stale-owner claim can fail the job. Engine/Store effects,
authentication and guards remain unchanged. CL-3 remains partial.

CL-3.2 now separates ten Engine media-recapture methods as `c614512` in
`media_recapture.go`. Its
[result](cl1-cleanup-candidates.md#cl-32-engine-media-recapture-feature-result-2026-11-02)
records default/development characterization before and after the unchanged
method move, current regression/Windows evidence and the reviewed commit.
Existing locks, request/background contexts, error precedence and effect order
are retained. Store and shared capture-runtime ownership remain unchanged;
CL-3 and operational acceptance are not closed by this file boundary.

CL-3.2 also separates seven Engine Bridge command methods as `3e13434` in
`internal/engine/bridge_commands.go`. Its [result](cl1-cleanup-candidates.md#cl-32-engine-bridge-command-feature-result-2026-11-02) maps the
original validation receipt to reviewed committed blobs. The identical default/
dev characterization payload passes 37 results before/after; latest combined
suites pass 801/1,475 results with 38/48 skips and zero failures. Four Engine
build-tag graphs and 12 Windows artifacts pass. Receiver, locks, contexts,
recovery before polling, durable results before cancellation/follow-on work,
protocol and guards are unchanged. Implemented/tested/scoped review are complete;
activation and operational acceptance remain unestablished. CL-3 is still partial.

CL-3.2 now separates five Engine media-provenance methods as `8d7f7b6` in
`media_provenance.go`. Its [result](cl1-cleanup-candidates.md#cl-32-engine-media-provenance-feature-result-2026-11-02) maps the original validation
to reviewed committed blobs. The identical characterization payload passes 24
results in default/dev before and after relocation. Latest regressions cover
six packages, adding Inspector/Store to Engine/HTTP/config/application: default
1,214 pass/43 skip, dev 2,040 pass/55 skip, zero failures. Four Engine build-tag
graphs and 12 Windows artifacts pass. Request/background contexts, queue before
active-worker check, e.mu registration/retirement, persistence after cancellation
and best-effort primary-result semantics remain unchanged. Implemented/tested/
scoped review are complete; activation and operational acceptance remain open.

CL-3.2 also separates five Engine AI-detection methods as `b84a62d` in
`ai_detection.go`. Its [result](cl1-cleanup-candidates.md#cl-32-engine-ai-detection-feature-result-2026-11-02) maps the original validation to
reviewed committed blobs. The identical default/dev characterization payload
passes 32 results before/after. Seven-package Detector/Inspector/Store/Engine/
HTTP/config/application regressions pass default 1,270/45 skip and dev 2,096/57
skip, zero failures; four build-tag graphs and 12 Windows artifacts pass. Worker
locks/contexts, same-session cancellation before settings reads, profile and job/
assessment order, usage-limit handling and media-provenance cancellation remain
unchanged. Implemented/tested/scoped review are complete; activation and
operational acceptance remain open. CL-3 is still partial.

CL-3.2 now separates session admission as `4bf0731` in `session_admission.go`.
Its [result](cl1-cleanup-candidates.md#cl-32-engine-session-admission-feature-result-2026-11-02) maps the original receipt to reviewed committed blobs.
Three Engine methods and one shared error helper move unchanged, with exact
Engine remainder. Identical characterization passes 31 default/34 dev results
before/after; seven-package regressions pass 1,301/2,130 results with 45/57 skips,
zero failures, four build-tag graphs and 12 Windows artifacts. Lock/context/
guard/error order, AI/provenance cancellation and durable ownership before
operation release and scheduling remain unchanged. Implemented/tested/scoped
review are complete; activation and operational acceptance remain open.

CL-3.2 now separates three observation functions as `d9c3025` in existing
`observations.go`; the [result](cl1-cleanup-candidates.md#cl-32-engine-observation-contracts-result-2026-11-02)
records exact caller/body equivalence and [validation](../runtime/qa/engine-observations-20261102/validation.json), [review](../runtime/qa/engine-observations-review-20261102/review.json) and [commit readback](../runtime/qa/engine-observations-review-20261102/final-readback.json). Characterization passes
61/74 default/dev before/after; eight-package regressions pass 1,409/2,251
results with 45/57 skips, zero failures; four build-tag graphs and 12 Windows
artifacts pass. Domain is added to the preceding seven-package scope. Native
decoding still rejects missing identity without normalization. Implemented,
tested and scoped review accepted; activation and operational acceptance open.

The process-error/usage-limit recovery feature was subsequently approved,
characterized before relocation and committed as `5b39fa4` in
`process_recovery.go`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-process-recovery-result-2026-11-02)
and [validation](../runtime/qa/engine-process-recovery-20261102/validation.json), [review](../runtime/qa/engine-process-recovery-review-20261102/review.json) and [commit readback](../runtime/qa/engine-process-recovery-review-20261102/final-readback.json). All seven declarations, three Engine receivers and the Engine
remainder are identical; the three caller files remain unchanged.
Characterization passes 53 results in both default/dev before/after; nine-package
regressions pass 1,564/2,406 results with 48/60 skips and zero failures. Reasoning
is added to the preceding eight-package scope. Four build-tag graphs and 12
Windows artifacts pass. Implemented, tested and scoped review accepted;
activation and operational acceptance remain open. CL-3 remains partial.

The reasoning-result feature was subsequently approved, characterized before
relocation and committed as `eb05ac8` in `reasoning_result.go`. See the
[result](cl1-cleanup-candidates.md#cl-32-engine-reasoning-result-2026-11-02) and [validation](../runtime/qa/engine-reasoning-result-20261102/validation.json), [review](../runtime/qa/engine-reasoning-result-review-20261102/review.json) and [commit readback](../runtime/qa/engine-reasoning-result-review-20261102/final-readback.json).
All three free helper signatures/bodies and the production caller are identical;
the Engine remainder differs only by removing the unused `fmt` import.
Characterization passes 52 results in both default/dev before/after; ten-package
regressions pass 1,622/2,464 results with 48/60 skips and zero failures. Selection
is added to the preceding nine-package scope. Four build-tag graphs and 12
Windows artifacts pass. Implemented, tested and scoped review accepted;
activation and operational acceptance remain open. CL-3 remains partial.

The capture-round feature and the separately approved Store assertion fix are
committed/published as `683b1e8`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-capture-rounds-result-2026-11-02)
and [final validation](../runtime/qa/store-tombstone-assertion-20261102/validation.json), [review](../runtime/qa/engine-capture-rounds-review-20261102/review.json) and [commit readback](../runtime/qa/engine-capture-rounds-review-20261102/final-readback.json). Eight helpers now belong to `capture_rounds.go`, with exact
signatures/bodies and Engine remainder; caller and production guard ownership
are unchanged. Characterization passes 59 default/dev before/after. The initial
development run hit an unrelated probabilistic tombstone assertion; its failed
and composite receipts remain historical. The corrected exact-HMAC test passes
20 repetitions per build, and new full runs pass default 1,705/48 skip and dev
2,549/60 skip, zero failures. Thirteen packages are selected: 12 execute tests,
and collection/bridge compiles without test files. Four build-tag graphs and
16 Windows artifacts pass, including Store tests. This sync reuses matching
evidence without new tests/builds, activation or trial. Implemented, tested and
scoped review accepted; operational acceptance and CL-3 closure remain open.

The local acquisition-policy feature is now committed as `f16f11a` in
`acquisition_policy.go`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-acquisition-policy-result-2026-11-02), [validation](../runtime/qa/engine-acquisition-policy-20261102/validation.json), [review](../runtime/qa/engine-acquisition-policy-review-20261102/review.json) and [commit readback](../runtime/qa/engine-acquisition-policy-review-20261102/final-readback.json).
Four free functions retain exact signatures, bodies, comments and reason strings;
the Engine remainder and session-execution caller are unchanged. Identical
characterization passes 206 default/dev parent/subtest results before and after
relocation. Matching 13-package regressions (12 tested; Bridge builder compile-only)
pass default 1,911/48 skip and dev 2,755/60 skip, zero failures. Four build-tag
graphs and 16 Windows amd64 artifacts pass. This review/sync reuses matching
source/log/artifact evidence without new product tests/builds. Implemented,
tested and scoped review accepted; activation and operational acceptance remain
open. CL-3 remains partial.

The vision-lane feature is now committed/published as `25c3339` in
`vision_lane.go`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-vision-lane-result-2026-11-02), [validation](../runtime/qa/engine-vision-lane-20261102/validation.json), [review](../runtime/qa/engine-vision-lane-review-20261102/review.json), [commit readback](../runtime/qa/engine-vision-lane-review-20261102/final-readback.json) and [code publication receipt](../runtime/qa/engine-vision-lane-review-20261102/push-readback.json).
Two free functions retain exact signatures and bodies; Engine loses only the
contiguous 27-line block, with the remainder and session-execution caller unchanged.
Identical characterization passes 83 default/dev parent/subtest results before and
after relocation. Matching 13-package regressions (12 tested; Bridge builder
compile-only) pass default 1,994/48 skip and dev 2,838/60 skip, zero failures.
Four build-tag graphs and 16 Windows amd64 artifacts pass. This documentation sync
reuses matching source/log/artifact evidence without new product tests/builds.
Implemented, tested and scoped review accepted; activation and operational
acceptance remain open. CL-3 remains partial.

The selection-correction workflow is now committed as `f3d46b8` in
`selection_corrections.go`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-selection-corrections-result-2026-11-02), [validation](../runtime/qa/engine-selection-corrections-20261102/validation.json), [review](../runtime/qa/engine-selection-corrections-review-20261102/review.json) and [commit readback](../runtime/qa/engine-selection-corrections-review-20261102/final-readback.json).
Two Engine methods retain exact signatures and bodies; the 68 removed lines are
the only Engine change, with imports, retry method, callers and Store unchanged.
Identical characterization passes 32 default/dev parent/subtest results before
and after relocation. Matching 14-package regressions (13 tested; Bridge builder
compile-only) pass default 2,048/50 skip and dev 2,892/62 skip, zero failures.
EventEngine is added to the previous regression selection. Four build-tag graphs
and 16 Windows amd64 artifacts pass. This sync reuses matching source/log/artifact
evidence without new product tests/builds. Implemented, tested and scoped review
accepted; activation and operational acceptance remain open. CL-3 remains partial.

The AI-feedback workflow is now committed as `6a1ce33` in `ai_feedback.go`.
See the [result](cl1-cleanup-candidates.md#cl-32-engine-ai-feedback-result-2026-11-02), [validation](../runtime/qa/engine-ai-feedback-20261102/validation.json), [review](../runtime/qa/engine-ai-feedback-review-20261102/review.json) and [commit readback](../runtime/qa/engine-ai-feedback-review-20261102/final-readback.json).
Three Engine methods retain exact signatures/bodies; the 31-line block removal
is the only Engine change, with imports, Store, HTTP callers and detector helpers
unchanged. Identical characterization passes 58 default/dev parent/subtest results
before and after relocation. Matching 14-package regressions (13 tested; Bridge
builder compile-only) pass default 2,106/50 skip and dev 2,950/62 skip, zero failures.
Four build-tag graphs and 16 Windows amd64 artifacts pass. This sync reuses
matching source/log/artifact evidence without new product tests/builds.
Implemented, tested and scoped review accepted; activation and operational
acceptance remain open. CL-3 remains partial.

The FullReset workflow is committed and published as `da019dd` in
`full_reset.go`. See the [FullReset result](cl1-cleanup-candidates.md#cl-32-engine-fullreset-result-2026-11-02) and [validation](../runtime/qa/engine-full-reset-20261102/validation.json), [review](../runtime/qa/engine-full-reset-review-20261102/review.json) and [commit/publication readback](../runtime/qa/engine-full-reset-review-20261102/final-readback.json).
The exact 44-line method block is removed from Engine; its remainder and imports
are unchanged. The same final characterization bytes pass 23 default/dev
parent/subtest results before and after relocation. Matching regressions pass
default 2,129/50 skip and dev 2,973/62 skip, zero failures, across 14 selected
packages (13 tested; Bridge builder compile-only). Four Windows tag graphs and
16 artifacts pass. This sync reuses matching evidence without new product
tests/builds. Implemented, tested and scoped review accepted; activation and
operational acceptance remain open. CL-3 remains partial.

The central [Engine facade checkpoint](code-structure-audit.md#engine-facade-stopping-checkpoint-2026-11-02)
now retains all 52 declarations remaining in engine.go. The one proposed passive
evidence colocation is implemented as `2bdc0f1` in the existing media-recapture
module. Stop further central-file splitting unless a concrete coupling,
testability or behavior problem justifies it. This completes the bounded
central-file responsibility review, not the entire Engine package, CL-3.2 or
CL-3. Adjacent runtime/HTTP and CL-5/CL-7/CL-8 retain separate scope and evidence.

At that checkpoint, the next proposal was the read-only HTTP router
responsibility map. That analysis and its separately approved bootstrap
extraction are now recorded in the
[bootstrap/router-map result](cl1-cleanup-candidates.md#cl-33-http-bootstrap-and-router-map-result-2026-11-02).
The map does not authorize extraction of deferred groups.
The [Engine responsibility map](code-structure-audit.md#current-engine-responsibility-map-2026-11-02)
records completed and remaining areas. No further implementation is approved.

| Increment | Implemented commit | Tested evidence | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Remove unused Personal Memory wrappers | `f348b9b` | Accepted cleanup review and regression history | Not recorded | Bounded removal only; not a repository-wide legacy audit |
| Move production native factories out of QA workflows | `29ec2b4` | [Factory receipt](../runtime/qa/native-factory-extraction-20261101/validation.json); its original known provenance-test failures are not a green result. The [later correction receipt](../runtime/qa/provenance-restoration-tests-20261101/validation.json) records 148/148 passing relevant tests with production guards unchanged | Not recorded | Production/QA responsibility separation; no provider qualification |
| Repair standalone harness dependency | `cd8fd60` | Harness/staging regression and subsequent Windows compilation | Not recorded | Compile boundary repaired; protocol/guard unchanged |
| Move production shared helpers out of fixture folder | `2ba01bb` | [Shared-helper receipt](../runtime/qa/native-shared-helpers-20261101/validation.json): equivalence, production package graphs and Windows checks | Not recorded | Scoped dependency separation; QA fixtures retained |
| Engine work bookkeeping/lifecycle | `202ecbb` | [Work lifecycle receipt](../runtime/qa/engine-work-lifecycle-20261101/validation.json) | Not recorded | `e.mu`, worker lifetime, cancellation and deferred cleanup retained |
| Scheduler lifecycle | `056b53b` | [Scheduler receipt](../runtime/qa/scheduler-lifecycle-20261101/validation.json) | Not recorded | Start-once/wake policy retained; no drain added |
| Session execution file separation | `d897130` | [Execution receipt](../runtime/qa/session-execution-20261101/validation.json) | Not recorded | Six declarations moved unchanged; Engine remains the owner |
| Session finalization helper | `ae1a535` | [Finalization receipt](../runtime/qa/session-finalization-20261101/validation.json): structural equivalence; 1,445 full-suite passes, additional default/fixture/combined selections of 65/65/132, seven Store passes, zero failures; eight Windows amd64 artifacts | Not recorded | Bounded helper extraction reviewed and committed; no operational acceptance claimed |
| CL-2 G10 unused deletion alias | `21ac9ad`, with cleanup roadmap | [G10 receipt](../runtime/qa/cl2-delete-memory-alias-20261101/validation.json), captured pre-commit: exact remainder equivalence, Store/HTTP 382/258 passes with 5/23 skips and zero failures, eight Windows artifacts, zero remaining code references | Not performed by this increment | Reviewed and committed for this removal only; not pushed in that follow-up |
| G06 config decoder relocation and repository smoke-test context | `d82f53b` | [Initial receipt](../runtime/qa/g06-reasoning-config-decode-20261101/validation.json) retains the historical baseline failure. [Correction receipt](../runtime/qa/repository-config-test-context-20261101/validation.json): default/dev each 46 pass, zero failures, two skips; eight Windows artifacts pass. [Review](../runtime/qa/g06-review-roadmap-20261101/review.json) confirms unchanged decoder and tested payload | Not performed | Scoped code review complete and committed; config/guards unchanged; non-Windows rejection branch reviewed but not executed on this host; no operational acceptance claim |
| E01 provider candidate failure cleanup | `7d00d9c` | [Receipt](../runtime/qa/provider-swap-candidate-cleanup-20261101/validation.json): six red-to-green failure scenarios; default 526 pass/38 skip, dev 1,199 pass/48 skip, zero failures; eight Windows artifacts | Not performed | Scoped fix/test reviewed and committed; original error/cause and successful ownership transfer preserved |
| E01 Settings execution file boundary | `02e063e` | [Receipt](../runtime/qa/settings-execution-20261101/validation.json): unchanged 120-line method and file remainder; default 526 pass/38 skip, dev 1,199 pass/48 skip, zero failures; four build-tag graphs and eight Windows artifacts | Not performed | Scoped review complete and committed; Settings getter/helpers/callers, lock/effect order, config/protocol/guards unchanged; further phase decomposition remains proposed |
| E01 pure profile-map merge | `aea3867` | [Initial receipt](../runtime/qa/settings-profile-merge-20261101/validation.json): characterization before extraction, precedence/independence/normalization and validation coverage; its default cancellation failure is historical, not a green result. Latest combined evidence is in the following row | Not performed | Scoped helper/test review complete and committed; raw/partial entries, both normalization positions and guard behavior preserved |
| Reader cancellation validation detour | `edb246d` | [Combined receipt](../runtime/qa/split-reader-cancellation-20261101/validation.json): deterministic red-to-green regression; 300 stress passes; default 547 pass/38 skip, dev 1,220 pass/48 skip, zero failures; three profile contracts, four build-tag graphs and eight Windows artifacts | Not performed | Scoped correction/test review complete and committed; cancelled waiters retain context errors, new requests after close remain rejected; tombstone/replay/activation guards retained |
| E01 executable-change preparation | `c303ed4` | [Initial extraction receipt](../runtime/qa/settings-executable-20261101/validation.json): 20 characterization results pass before/after extraction; exact helper/caller equivalence. Its native-pilot failures remain historical. [Review](../runtime/qa/settings-executable-review-20261101/review.json) verifies the unchanged payload against the latest combined passing evidence below | Not performed | Scoped review complete and committed; receiver, locks, trim-equal no-op, errors, active-provider ownership, pre-save discovery and post-save activation retained |
| Offline Node startup timing validation detour | `8dc3f5e` | [Combined receipt](../runtime/qa/native-pilot-node-integration-20261101/validation.json): default 568 pass/38 skip, dev 1,241 pass/48 skip, zero failures; 98 Node tests; two independent 100-iteration integration batches; four build-tag graphs and eight Windows artifacts. Review verifies hashes, raw logs and staging pins | Not performed | Scoped fixture/test correction reviewed and committed; real bounded scheduling intervals, original monotonic clock, protocol, timestamps, document/freshness/deadline guards retained. This is fixture stabilization, not universal clock synchronization or live-provider acceptance |
| CL-3.2 diagnostics export feature | `1068e35` | [Receipt](../runtime/qa/diagnostics-export-20261101/validation.json): identical characterization tests pass 17 results before and after the move; recorded default 585 pass/38 skip, dev 1,258 pass/48 skip, zero failures; four build-tag graphs and eight Windows artifacts. [Review](../runtime/qa/diagnostics-export-review-20261101/review.json) verifies source equivalence, raw logs and artifact hashes | Not performed | Scoped source/test review complete and committed; `DiagnosticsExport` and nine helpers relocated unchanged, Engine receiver and Store/read/error/output/protocol/guard contracts preserved. Default-range zero usage remains existing behavior; no operational acceptance claim |
| CL-3.2 Bridge status/compatibility feature | `7b2d0d8` | [Receipt](../runtime/qa/bridge-status-20261101/validation.json): identical characterization passes 38 results before/after relocation; recorded default 623 pass/38 skip, dev 1,296 pass/48 skip, zero failures; four build-tag graphs and eight Windows artifacts. [Review](../runtime/qa/bridge-status-review-20261101/review.json) checks source/log/artifact hashes, existing protocol/guard regressions and identity-checker anchors | Not performed | Scoped source/test review complete and committed; nine functions/two types relocated unchanged; constants, `expectedBridgeActions`, `e.mu`, heartbeat/revocation/recovery/reload/scheduler ordering retained. No protocol, authority, freshness or operational acceptance change |
| CL-3.3 HTTP Bridge command feature | `7cb712b` | [Receipt](../runtime/qa/bridge-commands-20261102/validation.json): identical characterization passes 53 results before/after relocation; recorded default 676 pass/38 skip, dev 1,349 pass/48 skip, zero failures; four HTTP build-tag graphs and 12 Windows amd64 artifacts (Engine tests, HTTP tests and application). [Review](../runtime/qa/bridge-commands-review-20261102/review.json) and [commit readback](../runtime/qa/bridge-commands-review-20261102/final-readback.json) verify source/log/artifact hashes and exact router/moved-body equivalence | Not performed | Scoped source/test review complete and committed; four Server methods extracted, existing route/context/authentication/protocol/guard/status/error/effect contracts retained. Native trace order checked structurally without starting a real OS observer; no live acceptance claim |
| CL-3.3 HTTP Bridge media-recapture feature | `be9fcd2` | [Receipt](../runtime/qa/bridge-media-recapture-20261102/validation.json): identical characterization passes 54 results before/after relocation; recorded default 730 pass/38 skip, dev 1,403 pass/48 skip, zero failures; four HTTP build-tag graphs and 12 Windows amd64 artifacts. [Review](../runtime/qa/bridge-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/bridge-media-recapture-review-20261102/final-readback.json) verify the exact source, logs, artifacts and committed blobs | Not performed | Scoped source/test review complete and committed; three Server methods retain route/context/authentication/protocol/guard/status/error/effect contracts. Durable result precedes success response; queued/claimed failure acceptance and stale-claim failure side effect retained. No live acceptance claim |
| CL-3.2 Engine media-recapture feature | `c614512` | [Receipt](../runtime/qa/engine-media-recapture-20261102/validation.json): identical characterization passes 34 default/35 dev results before/after relocation; latest default 764 pass/38 skip, dev 1,438 pass/48 skip, zero failures; four Engine build-tag graphs and 12 Windows amd64 artifacts. [Review](../runtime/qa/engine-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/engine-media-recapture-review-20261102/final-readback.json) verify exact signatures/bodies, source/log/artifact hashes and committed blobs | Not performed | Scoped source/test review complete and committed; ten methods moved with Engine receiver, locks, context, error/effect ordering, protocol and guards retained. State ownership, capture pump, generic cleanup, Store and HTTP unchanged; no operational acceptance claim |
| CL-3.2 Engine observation contracts | `d9c3025` | Identical characterization passes 61 default/74 dev parent/subtest results before/after relocation; eight-package suites pass default 1,409/45 skip and dev 2,251/57 skip, zero failures; four build-tag graphs and 12 Windows amd64 artifacts; [validation](../runtime/qa/engine-observations-20261102/validation.json), [review](../runtime/qa/engine-observations-review-20261102/review.json) and [commit readback](../runtime/qa/engine-observations-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three free functions move unchanged into existing observations.go; reconciliation, five production callers, normalization/validation/error order, native identity rejection, protocol and guards retained. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine process-error and usage-limit recovery | `5b39fa4` | Identical characterization passes 53 default/dev parent/subtest results before/after relocation; nine-package suites pass default 1,564/48 skip and dev 2,406/60 skip, zero failures; four build-tag graphs and 12 Windows amd64 artifacts; [validation](../runtime/qa/engine-process-recovery-20261102/validation.json), [review](../runtime/qa/engine-process-recovery-review-20261102/review.json) and [commit readback](../runtime/qa/engine-process-recovery-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Seven declarations in process_recovery.go retain three Engine receivers, exact bodies and Engine remainder, locks, caller contexts, persistence/error/effect order, accepted-capture/queued-peer guards, protocol and nonblocking wake. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine reasoning-result acceptance and Timeline mapping | `eb05ac8` | Identical characterization passes 52 default/dev parent/subtest results before/after relocation; ten-package suites pass default 1,622/48 skip and dev 2,464/60 skip, zero failures; four build-tag graphs and 12 Windows amd64 artifacts; [validation](../runtime/qa/engine-reasoning-result-20261102/validation.json), [review](../runtime/qa/engine-reasoning-result-review-20261102/review.json) and [commit readback](../runtime/qa/engine-reasoning-result-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three free helpers in reasoning_result.go retain exact signatures/bodies; Engine remainder identical except unused fmt import removal. Caller binding/validation/selection/completion order, error/coverage semantics, Store ownership, protocol and guards unchanged. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine capture rounds and privacy-test validation detour | `683b1e8` | Capture-round characterization passes 59 default/dev results before/after relocation; privacy test passes 20 repetitions per build after deterministic legacy failure; fresh 13-package selection (12 tested, Bridge builder compile-only) passes default 1,705/48 skip and dev 2,549/60 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [final validation](../runtime/qa/store-tombstone-assertion-20261102/validation.json), [review](../runtime/qa/engine-capture-rounds-review-20261102/review.json) and [commit readback](../runtime/qa/engine-capture-rounds-review-20261102/final-readback.json) verify combined source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed/published. Eight free helpers and Engine remainder byte-identical; callers, telemetry/coverage/alias/reconciliation/continuation semantics, locks, contexts, protocol and guards retained. Exact independent HMAC test vectors replace the probabilistic assertion; production privacy behavior unchanged. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine local acquisition policy | `f16f11a` | Identical characterization passes 206 default/dev parent/subtest results before/after relocation; matching 13-package selection (12 tested, Bridge builder compile-only) passes default 1,911/48 skip and dev 2,755/60 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-acquisition-policy-20261102/validation.json), [review](../runtime/qa/engine-acquisition-policy-review-20261102/review.json) and [commit readback](../runtime/qa/engine-acquisition-policy-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four free functions retain exact signatures/bodies/comments/reasons; Engine remainder, caller priority, scalar readers, continuation, source descriptors, Store authority, queue/receipt/error order, protocol and guards unchanged. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine vision-lane routing and coverage | `25c3339` | Identical characterization passes 83 default/dev parent/subtest results before/after relocation; matching 13-package selection (12 tested, Bridge builder compile-only) passes default 1,994/48 skip and dev 2,838/60 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-vision-lane-20261102/validation.json), [review](../runtime/qa/engine-vision-lane-review-20261102/review.json) and [commit readback](../runtime/qa/engine-vision-lane-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed/published. Two free functions retain exact signatures/bodies; Engine remainder, caller, source predicates, Coverage/nested aliasing, queue capacity, Store ownership, context/error/side-effect order, protocol and guards unchanged. Current manual_review availability is preserved. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine selection-correction workflow | `f3d46b8` | Identical characterization passes 32 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,048/50 skip and dev 2,892/62 skip, zero failures; EventEngine added to prior scope; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-selection-corrections-20261102/validation.json), [review](../runtime/qa/engine-selection-corrections-review-20261102/review.json) and [commit readback](../runtime/qa/engine-selection-corrections-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Two Engine methods retain exact signatures/bodies; Engine remainder, caller/Store/background ownership, operation/mu scopes, partial errors, durable/enrichment/cancellation order, protocol and guards unchanged. Undo retains same-session AI cancellation only and post-commit cleanup error behavior. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine AI-feedback workflow | `6a1ce33` | Identical characterization passes 58 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,106/50 skip and dev 2,950/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-ai-feedback-20261102/validation.json), [review](../runtime/qa/engine-ai-feedback-review-20261102/review.json) and [commit readback](../runtime/qa/engine-ai-feedback-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three Engine methods retain exact signatures/bodies; Add/Undo operation locks, lock-free history, durable-write/read/handoff order, review failures logged after commit while saved feedback is returned, append-only clear, cancellation/context behavior, Store/domain/HTTP ownership, protocol and guards unchanged. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine FullReset orchestration | `da019dd` | Identical characterization passes 23 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,129/50 skip and dev 2,973/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-full-reset-20261102/validation.json), [review](../runtime/qa/engine-full-reset-review-20261102/review.json) and [commit/publication readback](../runtime/qa/engine-full-reset-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed and code published. One Engine method retains exact signature/body; operation/mu scopes, cancellation before admission/errors, default/provider resolution, candidate disposal, Store authority and post-commit capture/headless/swap order unchanged. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine Library removal/Forget workflow | `a19bf86` | Identical characterization passes 28 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,157/50 skip and dev 3,001/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-library-removal-20261102/validation.json), [review](../runtime/qa/engine-library-removal-review-20261102/review.json) and [commit/publication readback](../runtime/qa/engine-library-removal-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; code committed/published. Two exact Engine methods retain topic snapshot before Store mutation, sorted/background queue handoff, per-topic error logging/continuation, caller return/error/context semantics, absence of entry-level locks, Store privacy authority, protocol, guards and AI/provenance cancellation. CL-3 remains partial; operational acceptance remains open |
| CL-3.2 Engine passive X media evidence | `2bdc0f1` | Identical characterization passes 20 default/dev parent/subtest results before/after relocation; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,177/50 skip and dev 3,021/62 skip, zero failures; four build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/engine-passive-media-20261102/validation.json), [review](../runtime/qa/engine-passive-media-review-20261102/review.json) and [commit readback](../runtime/qa/engine-passive-media-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Exact method moved to existing media_recapture.go with no import changes; development gate, operation lock, Store result/errors, conditional Timeline/provenance handoff, background contexts, unlock-before-terminal-cleanup order, protocol and guards retained. Central-file checkpoint retains 52 declarations; CL-3 remains partial and operational acceptance open |
| CL-3.3 HTTP update/session lifecycle | `d32c065` | Identical characterization passes 53 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,230/50 skip and dev 3,074/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-session-updates-20261102/validation.json), [review](../runtime/qa/http-session-updates-review-20261102/review.json) and [commit readback](../runtime/qa/http-session-updates-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four Server handlers preserve router predicates/order/normalization, request contexts, shared loopback/origin security and progress projection, raw start output, asymmetric errors and durable cancel/read/write order. Engine, Store, protocol, guards and cancellation unchanged. CL-3 remains partial; operational acceptance open |
| CL-3.3 HTTP media-recapture status/enqueue/passive evidence | `90e3a7f` | Identical characterization passes 42 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,272/50 skip and dev 3,116/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-media-recapture-20261102/validation.json), [review](../runtime/qa/http-media-recapture-review-20261102/review.json) and [commit readback](../runtime/qa/http-media-recapture-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Three Server handlers preserve router predicates/order/normalization, distinct ID parsing, private status projection, UI versus Bridge security, authentication before decode, unfenced passive relay, validation/defaults/errors, caller context and durable effects before response. Legacy Facebook branch, Engine/Store, Bridge claim/result handlers, protocol, guards and cancellation unchanged. CL-3 remains partial; operational acceptance open |
| CL-3.3 HTTP bootstrap composition | `97bbe51` | Identical characterization passes 25 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,297/50 skip and dev 3,141/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-bootstrap-20261102/validation.json), [review](../runtime/qa/http-bootstrap-review-20261102/review.json) and [commit readback](../runtime/qa/http-bootstrap-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. One Server method preserves route predicates/order/normalization, shared loopback/origin security, request context, complete output/progress projection, ten ordered fallible data calls, first-error mapping, token initialization, first-run calibration, prepared-batch expiry and durable effects before response-write failure. No new interface, transaction, cache or concurrency; Engine/Store, protocol and guards unchanged. CL-3.3 and CL-3 remain partial; activation and operational acceptance open |
| CL-3.3 capture-owner path guard | `b482ba2` | Regression added before the fix reproduced unauthorized action acceptance and telemetry persistence for trailing/encoded slash forms. The unchanged regression payload then passed default/dev; matching 14-package selection (13 tested, Bridge builder compile-only) passed default 2,348/50 skip and dev 3,192/62 skip, zero failures; four Windows amd64 build-tag graphs include the new tests and 16 artifacts pass; source and artifact identities reverified before commit, without new test/build runs | Not performed | Scoped admission-path fix reviewed and committed. One classifier assignment now trims exactly one trailing slash like dispatch; missing/stale keys return 409 before accept/event mutation. Valid workers, contract/token priority, UI relay and non-split behavior remain. No handler move or wire change. Post-admission rotation is separate and untested; CL-3.3/CL-3 partial; operational acceptance open |
| CL-3.3 HTTP Bridge reload actions | `6ea303c` | Identical characterization passes 45 default/dev parent/subtest results before/after extraction; matching 14-package selection (13 tested, Bridge builder compile-only) passes default 2,393/50 skip and dev 3,237/62 skip, zero failures; four HTTP build-tag graphs and 16 Windows amd64 artifacts; [validation](../runtime/qa/http-bridge-actions-20261102/validation.json), [review](../runtime/qa/http-bridge-actions-review-20261102/review.json) and [commit readback](../runtime/qa/http-bridge-actions-review-20261102/commit-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped review found no blocking issue; committed. Four private Server methods preserve route predicates/order/normalization, request context, auth/decode/error order, UI relay versus worker guard, action output, empty 204, wait bound, delivery/cancellation priority and in-memory effects before write failure. Heartbeat, shared helpers, Engine/Store state and locks, owner-path fix, protocol and guards unchanged. CL-3.3/CL-3 partial; operational acceptance open |

Counts include subtests and repeated build-tag selections; do not add them as
unique test cases. Receipts captured before commit may retain `committed:false`
and a parent `sourceCommit`; the commit mapping above records the later Git
state. Source-equivalence hashes identify the tested payload. Do not rewrite
raw receipts to make their historical fields look current.

Remaining structural candidates are organized in the
[cleanup roadmap](code-cleanup-roadmap.md#execution-stages-and-current-status)
and grounded in [the structure audit](code-structure-audit.md):
feature-level Timeline/frontend Settings extraction, probe dispatch, remaining
Engine/HTTP capture-service boundaries and a broader
legacy/compatibility inventory. These are proposals,
not authorization to implement or delete. Scheduler drain and further execution
refactors have not been added to this completed slice. Further small backend
Settings splitting remains deferred; keep the existing method cohesive.

The HTTP router map is recorded. Six scoped HTTP boundaries, including reload actions, are implemented/tested/reviewed. The current snapshot retains 54 switch arms (28 delegated, 26 thin/control/heartbeat) and defers 52 (nine native/dev/telemetry, 43 product domains). Shared infrastructure and 30 server.go declarations remain; central Engine retains 52 declarations. Keep central-file splitting stopped without a concrete new ownership/coupling/testability reason. The next proposal is read-only telemetry/owner-rotation lifecycle analysis; no implementation or trial is authorized.
Bridge command, observation, process-recovery, reasoning-result, capture-round,
local acquisition-policy, vision-lane, selection-correction, AI-feedback, FullReset, Library removal/Forget, passive media evidence, HTTP update/session, HTTP media-entry, HTTP bootstrap and HTTP reload-action boundaries are implemented and reviewed; do not repeat those moves. This roadmap sync reuses
matching test/build evidence and changes no runtime or configuration.

### HTTP bootstrap/router-map checkpoint (2026-11-02)

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

### Engine read-model and enrichment ownership follow-up completed

The scoped read-only ownership review is complete. Retain Engine read facades
for Session/ActiveSession/Run, Timeline, Inbox/trace, Library and usage. Store
continues to own durable query/hydration/projection. VisionEvaluationJobs retains
its run lookup before source-scoped jobs. AI detection, media provenance,
selection correction, AI feedback, recapture and Library removal retain their
existing feature-specific orchestration, locks, effect order and cancellation.
Do not introduce a generic enrichment service or reopen the 52 retained central
Engine declarations. This checkpoint does not close all of CL-3.2 or CL-3.

The owner separately approved one backend continuation at the existing Store
owner: move the complete Timeline read-model group after characterization on
the original code. This is file separation within Store, not a new Engine owner.

## Store Timeline read-model result (2026-11-03)

The owner approved review, roadmap synchronization, commit and publication of
this bounded increment to origin/main at https://github.com/abangkis/AkuSidecar.git.
The preceding published code/roadmap checkpoint is 4c6baf3, including empty-candidate
completion de95c58. Production/test code is now bee6a9f.

| Dimension | Evidence and scope |
| --- | --- |
| Implemented | bee6a9f: six methods and timelinePresentationOrderSQL moved unchanged from store.go to timeline_read_model.go (257 lines). The original declaration block is 245 lines; store.go is now 2,573 lines. All callers retain package symbols and Store receivers |
| Tested | Frozen characterization test and pass/skip names match before/after: 72 pass/1 skip in each default/dev mode. Six new roots cover 29 scenarios (35 parent/subtest results). Matching 14-package regression (13 tested; Bridge builder compile-only): default 3,222 pass/50 skip; dev 4,066 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the new test and produce 16 PE/amd64 artifacts |
| Activated | Not performed; no runtime, configuration, restart or live trial change |
| Acceptance | Scoped source/test review accepted with no findings. Engine read-model/enrichment ownership retained and Store file separation established; CL-3 remains partial. Broader integration, package and operational acceptance remain open |

The moved methods are ListSessionItems, TimelineItem, ListTimeline, listItems,
attachTimelineMemoryProjections and attachEffectivePreferenceDecision. SQL,
receiver, context, read/error order and source remainder are unchanged. Evidence
still resolves from retained snapshot to the first matching accepted observation
and then to a recapture override. Hydration then attaches semantic, AI/provenance,
effective preference and bounded memory projections in the original order. Rows
are closed before nested queries on the single SQLite connection. No interface,
state owner, migration, lock or transaction redesign was introduced.

Characterization locks empty/nil results and sql.ErrNoRows, rank/GetSession
hydration, evidence precedence, malformed-data behavior, combined projection
in show_all/hide/collapse, feedback timestamp/authority/id precedence, memory
absence/privacy, first-error ordering, caller cancellation and connection waits.
It verifies that read projection does not mutate durable data. Existing page
selection, relation, detached-origin, retention, AI and recapture tests remain
in the focused suite. The one focused skip is TestTimelinePageReadOnlyPerformance,
which requires an explicit read-only database path; no user database or new
performance measurement was used. Synthetic provenance rows are fixture data,
not real-provider or trusted-manifest qualification.

Review independently compares all seven declarations and the source remainder
against Git source 4c6baf3 and verifies frozen source/test/log/graph/helper/artifact
hashes. All 1,449 other previously tracked files were unchanged at validation
and review, including Engine/HTTP, existing tests, protocol, guards and the
already dirty user config. Only the three approved Store source/test files and
four roadmap documents are committed by this publication; .tmp/ and experiments/
remain excluded. No race-detector or broader runtime acceptance is claimed.

Evidence: [validation](../runtime/qa/timeline-read-model-20261103/validation.json),
[extraction proof](../runtime/qa/timeline-read-model-20261103/extraction-proof.json),
[scoped review](../runtime/qa/timeline-read-model-review-20261103/review.json),
and [commit/publication readback](../runtime/qa/timeline-read-model-review-20261103/final-readback.json).
These are local ignored QA receipts. Original sourceHead/sourceDirty/committed
fields describe capture time and remain unchanged; the later readback binds the
frozen tested payload to commits and the verified publication target.

### Routine feedback follow-up completed within the automatic run

The owner broadened authorization from one-at-a-time feature work to automatic
Store refactoring: "Oke berdasarkan pola yang ada, jalankan secara otomatis proses
refactoring utk store." The bounded run separates the remaining mixed feature
responsibilities in store.go, adds characterization before changes, reviews the
exact moves, runs regression/Windows checks and synchronizes these documents.
Routine feedback moves as one complete transaction owner; its feedback row,
preference learning and memory projection/retraction are not split apart.

## Automatic Store feature-separation result (2026-11-03)

This result is committed as `f27e91b`, after published checkpoint 6f3b5bc
(Store Timeline read-model bee6a9f). The owner approved scoped review, roadmap
synchronization, commit and push to origin/main at
https://github.com/abangkis/AkuSidecar.git. The commit/publication readback below
binds the frozen tested payload to this code and roadmap publication. No runtime
activation, trial, user database operation or configuration change is included.

| Dimension | Evidence and scope |
| --- | --- |
| Implemented | `f27e91b`: 95 original declarations move into 15 feature files; store.go falls from 2,573 to 233 lines and retains 13 foundation declarations. No function body, signature, SQL, lock, transaction, error return or guard call is rewritten |
| Tested | Whole Store before/after pass and skip names match: default 462 pass/5 skip; dev 614 pass/7 skip. Six new roots add 40 scenarios (45 parent/subtest results). Matching 14-package regression (13 tested; Bridge builder compile-only): default 3,267 pass/50 skip; dev 4,111 pass/62 skip; zero failures. Four Windows amd64 tag combinations include the new test and produce 16 PE/amd64 artifacts |
| Activated | Not performed; no runtime, restart, configuration or live trial change |
| Acceptance | Scoped source/test review accepted with no findings and committed; publication is bound to the readback below. The selected central Store feature-separation cohort is source-complete. CL-3 remains partial; this does not certify the entire Store package, every migration path or operational reliability |

All files below remain in package store with their existing receivers and shared
state owner. Symbols and callers are unchanged; only file ownership/imports and
spacing between declarations change.

| Feature file | Declarations | Responsibility |
| --- | ---: | --- |
| feedback.go | 4 | Feedback timing context and atomic routine feedback/learning/memory action |
| schema_migrations.go | 18 | Existing supported schema migration steps; registry and semantics retained |
| source_registry.go | 2 | Domain source registry sync and one-time default adoption |
| bridge_token.go | 3 | Bridge token read/create and constant-time comparison |
| settings.go | 5 | Runtime settings and onboarding profile persistence |
| session_creation.go | 4 | Ordinary/daily session admission and durable creation |
| session_read_model.go | 5 | Session/run reads and captured-owner metadata projection |
| session_lifecycle.go | 8 | Pipeline stages, completion/finalization, cancellation and replay admission |
| capture_dispatch.go | 20 | Command start/follow-up/claim, collector routing and bounded dispatch expiry |
| observations.go | 6 | Accepted observation/identity/receipt transaction and resumable reads |
| capture_failure.go | 5 | Claimed/queued failure, accepted-capture fallback and failure persistence |
| reasoning_telemetry.go | 2 | Invocation persistence and diagnostic reads |
| knowledge.go | 3 | Knowledge and prior delivered/known evidence queries |
| session_composition.go | 2 | Global session composition and its private item type |
| database_reset.go | 8 | Reset/idle gates, verified backup and existing profile-marker consumption |

Retain Store, Clock/systemClock, both clock accessors, Open/OpenReadOnly/
OpenWithClock/openWithClock, initialization, Close/Path and the shared decodeJSON
helper in store.go. Initialization remains the ordered composition root and
SQLite driver registration remains there. Existing cohesive feature files,
schema definitions, the migration registry, native/QA ownership and guard modules
are retained unchanged; no compatibility path was deleted merely to reduce size.
There are no pending feature extractions in this selected central-file cohort.

The new characterization exercises real Store operations and disposable SQLite
faults: feedback normalization/timing and feedback/learning/memory rollback;
onboarding/registry/session creation atomicity; capture start/observation/identity/
coverage/follow-up/cancellation failures; completion/finalization/telemetry/replay
contracts; reset/backup/marker/cancellation behavior; and exact queued-expiry
boundaries. Timing tests use sentinels to identify executed stages, accepting
valid zero durations on Windows. Existing Store tests run in full before and
after the moves, including source/capture/identity/provenance and migration gates.
Skipped optional/environment tests retain their own existing scope; no real
provider, user database performance run or race-detector result is claimed.

An independent readback matches every one of the 108 original declaration texts
against Git source 6f3b5bc, then verifies their new placement: 95 moved and 13
retained. All 1,451 other previously tracked files were unchanged at validation,
including Engine/HTTP, existing tests, schema, protocol, guards and already dirty
user config. Four roadmap documents change only after validation. User .tmp/ and
experiments/ remain excluded. SourceHead/sourceDirty/committed in the original
receipt describe capture time and are preserved.

Evidence: [frozen validation](../runtime/qa/store-auto-refactor-20261103/validation.json),
[feature plan](../runtime/qa/store-auto-refactor-20261103/plan.json),
[declaration review](../runtime/qa/store-auto-refactor-20261103/move-review.json),
and [final candidate readback](../runtime/qa/store-auto-refactor-20261103/final-readback.json).
The capture-time candidate receipts remain unchanged. Additional publication
evidence: [scoped review](../runtime/qa/store-auto-refactor-review-20261103/review.json)
and [commit/publication readback](../runtime/qa/store-auto-refactor-review-20261103/final-readback.json).
These are local ignored QA receipts, not a published release manifest.

Current CL-3 status: **source closed** following the accepted consolidated
responsibility-map/exit-criteria review at `6b9b3a0`, with product code through
`f27e91b`. See [CL-3 source closure](code-cleanup-roadmap.md#cl-3-source-closure-2026-11-03). Earlier partial status in
this result is its historical capture-time boundary. Intentional retain/defer
decisions are recorded in the closure map; no additional backend extraction is
required for this gate. Activation and operational acceptance remain separate.

### Proposed next cleanup analysis

Source CL-3 is closed. The proposed next bounded task is a read-only CL-4.1
Timeline responsibility map in app.js and existing web modules. Trace Timeline
rendering and item-scoped actions against app bootstrap/polling, revision/epoch
checks, cache invalidation, lazy media, drawer/context behavior, focus and
teardown. Choose one coherent feature boundary only if it provides a real owner
and testability benefit; otherwise retain the existing owner. Name its
characterization needs and accountless rendered desktop/mobile interaction
checks before proposing implementation. Frontend work uses Sol/Astra under the
active routing policy. This proposal authorizes no code change, visual redesign,
configuration change, activation or trial; the next analysis requires approval.


## Product and installation roadmaps

| Workstream | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Personal Memory foundation, Living Topics Thin Slice, Full Stage 1 and local knowledge use | Recorded in the existing Browser contracts/roadmap | Historical contract and source acceptance; no feature retest in this documentation update | No fresh runtime identity established here | Local graduated stages remain graduated; no external acquisition authority |
| Ask this topic | Not implemented in the roadmap | Not established | Not established | Requires a separate cited-answer contract |
| Bookmark Import and Management | Frozen, not started by this synchronization | Not established | Not established | Explicit authorization remains required |
| Broader Living Topics discovery/Keep Up | Deferred | Not established | Not established | Does not inherit local-stage graduation |
| Provider onboarding stages 1-6 | Recorded complete on 2026-08-28 | Historical development E2E | Historical development use; no new activation | Development acceptance complete; installed Gemini-key acceptance remains a separate INST-05 gate |
| Legacy Bridge setup retirement | Recorded complete | Historical migration/retirement acceptance | Historical installed-app lane, not a new rollout | Retirement remains complete; signing/clean-machine/rollback gaps are separate |
| Installation recovery INST-01/02/03/05 | Partial mitigations recorded | Local fixtures and bounded recovery checks | Historical test artifacts | Still open in the issue track; RC9 existing-account success does not close independent first-install, problematic-DB or real-key gates |
| Installation recovery INST-04 | Isolated-profile/login flow recorded | Bounded restart and same-version continuity checks | Historical local builds | Complete only in that selected scope; upgrade certification remains separate |
| Windows distribution | Existing RC9 tuple/installer machinery | RC9 existing-account startup and four-source receipt | RC9 installation recorded on 2026-09-23 | Independent clean-machine, signing and whole-tuple rollback gaps remain; newer source is not the published RC9 payload |

Browser sequencing authority: `AkuBrowser/docs/personal-memory-roadmap.md`.
Installation issues: `AkuBrowser/docs/installation-onboarding-recovery-track.md`.
Release authority: `AkuBrowser/docs/stable-release-checklist.md` and the frozen
RC9 receipt at `AkuBrowser/acceptance/windows-installed-app-rc9-2026-09-23.md`.
The release documents record RC9 as the latest published prerelease; remote
release/tag state was not queried during this synchronization. Comment-control
SITC remains an exploration document, not an implementation roadmap commitment.

## Latest operational evidence and next boundary

- X repeat: [local trial receipt](../runtime/qa/auth-session-sidecar/run-x-diagnostic-repeat-ab2dc40-814c0722/trial-summary.json),
  completed 2026-11-01 11:00:57 WIB. Session and run completed; temporary settings
  were restored. The recorded source is `ab2dc40` with `sourceDirty=true`.
- IG: [local trial receipt](../runtime/qa/ig-history-caller/live-32b8552/trial-summary.json),
  completed 2026-11-01 12:36:49 WIB. Recorded source `32b8552`,
  `sourceDirty=true`, 93 verified artifacts, activation exit 0. The trial failed
  with `native_source_evidence_document_changed` / `document_within_navigation`,
  `navigationType=historyApi`, role `reader`, verifier step `readiness`.
  Cleanup and temporary-setting restoration were verified. The final document
  diagnostic contains no History method/caller tuple, so its caller is unknown.
- [Accountless diagnostic fixture](../runtime/qa/ig-history-caller/offline-chromium-result-chrome156.json):
  `replaceState` and `pushState` caller locations were captured on loopback;
  native methods were unchanged and `document_within_navigation` still fired.
  This proves fixture instrumentation, not attribution on the real IG page.

Local `runtime/qa` receipts are ignored evidence artifacts and may be absent in
another checkout. Retain their source identities and limits when sharing this
document; do not interpret a missing local file as a fresh test result. None of
the receipts proves that `ae1a535` is active now or that its artifacts form a
clean published tuple. No process, endpoint or profile was accessed here.

Next technical question: why the real reader invalidation has no method/caller
tuple despite the accountless fixture passing. Investigate that attribution gap
without weakening the observer. Repeated native IG and mixed-source acceptance
remain open. This documentation update authorizes no code change, activation,
probe, login/session transfer, configuration change or additional trial.
