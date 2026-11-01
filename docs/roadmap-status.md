# Roadmap status and evidence index

Status synchronized: 2026-11-01 (Asia/Jakarta).
Reviewed source: AkuSidecar `ae1a535`, AkuBrowser `523ef89`, AkuBridge `3676f5c`.
This is a documentation checkpoint, not a runtime inspection or release.
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
| Daily acceptance and obsolete-path retirement, stage 5 | Existing daily machinery and bounded cleanup extractions; not a wholesale retirement | Synthetic/repeated-batch checks exist | No activation of the 2026-11-01 cleanup commits recorded | Repeated real X/IG batches, restart/cancel/consent/reader acceptance remain pending |
| Broader browser/headless parity | Existing Manager/Coordinator, driver and handoff implementation | Historical component, source-parity and selected Windows evidence | Historical selected-mode evidence only | General integrated handoff, reader/login and full media/package parity are not all closed; do not reset the accepted X MVP |
| Shared capture reservation | Proposed only | Conservative predecessor-drain tests are not reservation tests | Not recorded | Deferred; not another gate for the accepted X MVP |

Detailed scopes: [collection roadmap](collection-modes-roadmap.md),
[X/IG convergence](x-ig-convergence-roadmap.md),
[IG evidence and diagnostics](instagram-bootstrap-daily-proof.md), and
[reservation proposal](shared-capture-reservation-plan.md).

## Cleanup and modularization

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

Counts include subtests and repeated build-tag selections; do not add them as
unique test cases. Receipts captured before commit may retain `committed:false`
and a parent `sourceCommit`; the commit mapping above records the later Git
state. Source-equivalence hashes identify the tested payload. Do not rewrite
raw receipts to make their historical fields look current.

Remaining structural candidates are described in [the structure audit](code-structure-audit.md):
feature-level Timeline/Settings extraction, probe dispatch, HTTP capture-service
boundaries and a broader legacy/compatibility inventory. These are proposals,
not authorization to implement or delete. Scheduler drain and further execution
refactors have not been added to this completed slice.

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
