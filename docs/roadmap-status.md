# Roadmap status and evidence index

Status synchronized: 2026-11-02 (Asia/Jakarta).
Reviewed local source: AkuSidecar `97bbe51` (HTTP bootstrap/router map, after HTTP media-recapture entry points `90e3a7f`, HTTP update/session lifecycle `d32c065`, Engine passive media evidence `2bdc0f1`, Library removal/Forget `a19bf86`, FullReset `da019dd`, AI feedback `6a1ce33` and selection corrections `f3d46b8`, vision-lane routing/coverage `25c3339`, local acquisition policy `f16f11a`, capture-round separation and deterministic Store privacy test `683b1e8`, reasoning result `eb05ac8`, process recovery `5b39fa4`, observation contracts `d9c3025`, session admission `4bf0731`, AI detection `b84a62d`, media provenance `8d7f7b6`, Engine Bridge commands `3e13434`, Engine media recapture `c614512`, HTTP recapture `be9fcd2`, HTTP commands `7cb712b`, Bridge status `7b2d0d8` and diagnostics export `1068e35`),
AkuBrowser `3a3c043`, AkuBridge `3676f5c`.
This is a documentation checkpoint, not a runtime inspection or release.
The [HTTP bootstrap commit readback](../runtime/qa/http-bootstrap-review-20261102/commit-readback.json)
verifies exact committed source. The previous code/roadmap checkpoint was published
as `d8eb318`, including HTTP media-entry code `90e3a7f`. The owner has authorized
publication of this code and roadmap synchronization to origin/main at
https://github.com/abangkis/AkuSidecar.git. The documentation commit and verified
remote SHA are recorded separately under
`runtime/qa/http-bootstrap-roadmap-sync-20261102/`.
Publication does not establish runtime activation or operational acceptance.
The Browser worktree contains pre-existing build/cache changes; those changes
are outside this synchronization and are not evidence of a released tuple.

## Status vocabulary

| Dimension | Meaning | Does not establish |
| --- | --- | --- |
| Implemented | Code exists at the named commit, or an explicitly identified historical contract records implementation | Test success, deployment or provider qualification |
| Tested | The named source/artifact passed the stated offline, synthetic or local checks | A current running binary, a clean-machine install or real-provider acceptance |
| Activated | A dated receipt identifies the artifact used in a running process | Activation of later commits, continuous current health or a clean source build |
| Acceptance | The stated product/operational gate has its own evidence and scope | Acceptance of another source, mode, release or broader reliability claim |

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

The HTTP router responsibility map is recorded and bootstrap composition is extracted/tested/reviewed. After the move, 49 switch arms are retained and 57 deferred; shared Server infrastructure remains. The next proposal is read-only ownership/contract analysis of the five Bridge heartbeat/reload-action handlers, with at most one justified feature boundary. Deferred groups are not an automatic extraction queue. The central Engine file retains 52 declarations; no further Engine file split is proposed.
Bridge command, observation, process-recovery, reasoning-result, capture-round,
local acquisition-policy, vision-lane, selection-correction, AI-feedback, FullReset, Library removal/Forget, passive media evidence, HTTP update/session, HTTP media-entry and HTTP bootstrap boundaries are implemented and reviewed; do not repeat those moves. This roadmap sync reuses
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

The next proposal is read-only analysis of the five remaining Bridge
heartbeat/reload-action handlers: heartbeat, reload-self, next, accept and detail.
Map Engine heartbeat/action ownership against split-capture key/mutex ownership,
authentication/decode/error order, long-poll request cancellation and existing
tests. Decide whether one complete feature boundary is justified or retention is
better, and specify missing characterization before any implementation. This is
a proposal requiring separate approval; no code, test/build, runtime/configuration
change, activation or live trial is authorized by this documentation update.

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
