# Roadmap status and evidence index

Status synchronized: 2026-11-02 (Asia/Jakarta).
Reviewed local source: AkuSidecar `b84a62d` (Engine AI-detection separation, after media provenance `8d7f7b6`, Engine Bridge commands `3e13434`, Engine media recapture `c614512`, HTTP recapture `be9fcd2`, HTTP commands `7cb712b`, Bridge status `7b2d0d8` and diagnostics export `1068e35`),
AkuBrowser `3a3c043`, AkuBridge `3676f5c`.
This is a documentation checkpoint, not a runtime inspection or release.
The [latest recorded push receipt](../runtime/qa/engine-media-provenance-roadmap-sync-20261102/push-readback.json)
verifies four Bridge-command/media-provenance code and roadmap commits through
`222bcee` published to `origin/main`. The owner has authorized publication of
the subsequent AI-detection code and roadmap commits, including this checkpoint,
to `origin/main` at `https://github.com/abangkis/AkuSidecar.git`. The exact new
remote SHA must be established by the post-push readback under
`runtime/qa/engine-ai-detection-roadmap-sync-20261102/`; publication does not
establish runtime activation or operational acceptance.
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

Next proposal: read-only analysis of Engine session admission/creation, including
guard/lease ownership, durable-write and initial-scheduling boundaries. No further
implementation is approved.

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

Next source-cleanup proposal: read-only analysis of Engine Bridge command
orchestration in `engine.go`: claim, pending, observation and failure. Map
admission, collector/owner fencing, Store effects, follow-on scheduling,
lock/context scopes and error contracts before selecting one coherent extraction
and missing characterization. Recovery also serves heartbeat/scheduler;
normalization/validation also serve media recapture and native consumers.
Preserve those shared contracts, Engine/Store/Manager authority, protocol and
guards; do not duplicate helpers or repeat completed lifecycle work.
Implementation requires separate approval after analysis. This documentation
synchronization changes no code and runs no tests, builds, activation or trial.

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
