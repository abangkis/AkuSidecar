# Comprehensive code cleanup roadmap

Date: 2026-11-01 (Asia/Jakarta).
Status synchronized: 2026-11-04 (Asia/Jakarta). Selected source CL-3 through CL-8 retain their closures; source identity v118 and historical release v116 remain separate. P05 cache 8c39d97, Step 3B ordering 5fa73c9 and completed scratch maintenance 54a1b49 retain their published status. Windows installed-app cache lifecycle characterization is implemented, tested and reviewed as Browser 16a6ff2; its publication readback is recorded below. Further Mac work and qualification remain frozen and do not gate Windows cleanup. Full P05 closure and operational/release acceptance are not declared; implemented, tested, published and acceptance remain separate.
Status: the selected CL source cleanup is signed off through CL-8, with retained
contracts, explicit deferrals and the historical P05 exclusion. The later
[P05 cache increment](#p05-source-build-cache-publication-2026-11-04) has its own
scoped review and publication; it does not rewrite the CL-7/CL-8 checkpoint. See the
[final source closure](#cl-8-source-closure-2026-11-04) and earlier stage maps for
return triggers. This does not certify every legacy-looking symbol or a release.
Other workstreams and changes outside approved increments require approval.
Earlier partial/open statements retain their historical checkpoint scope.
No implementation, deletion, dependency upgrade, activation or trial is approved
by creating this roadmap. Approve one concrete increment, implement it, verify
it and review its result before approving another increment, unless the owner
explicitly authorizes a broader bounded run as recorded below.

## Objective and scope

Make the AkuSidecar, AkuBridge and AkuBrowser source easier to change and verify:
remove proven obsolete code, separate production from QA, give each module a
clear responsibility, and preserve supported behavior and durable data.

Scope covers tracked product source, build/package entry points, tests, fixtures,
contracts and developer tooling in these three repositories. Inspect generated
bundles and manifests as consumers without treating generated output as source.
The previously dirty Browser build/cache package was separately reviewed and
published in the P05 increment below. Remaining tooling gaps require their own
approved scope. Untracked experiments, profiles,
databases, user data and build/cache deletion are outside this roadmap.

This roadmap does not start Bookmark Import, Keep Up, shared reservations,
scheduler drain, a protocol/schema redesign, dependency upgrades, or further IG
account/session experiments. The IG History caller-attribution investigation and
real-provider qualification remain separate workstreams. A cleanup must not
silently incorporate a functional fix discovered during inspection.

Status vocabulary and product relationships are defined by the
[roadmap status index](roadmap-status.md). This roadmap expands the cleanup
workstream; it does not reset the existing product or capture roadmaps.

## Verified starting point

Reviewed source: Sidecar `24e5a08`, Browser `3a3c043`, Bridge `3676f5c`.
Sidecar and Browser main pushes were verified against those remote SHAs before
this document was created. The Browser worktree still contains unrelated
build/cache changes. Git publication is not runtime activation.

The completed increments remain the baseline:

| Increment | Commit | Existing evidence |
| --- | --- | --- |
| Remove unused Personal Memory wrappers | `f348b9b` | Scoped removal review and regression history |
| Production native factory / QA separation | `29ec2b4` | Factory equivalence and package checks; later relevant provenance corrections passed 148 tests |
| Standalone harness dependency | `cd8fd60` | Staging regression and subsequent Windows compilation |
| Production shared helpers | `2ba01bb` | 45 helper payloads still match their recorded hashes; manual/daily/main package receipts record zero fixture imports |
| Engine work state/lifecycle | `202ecbb` | Preserved atomic sections, worker retirement and cleanup accounting |
| Scheduler lifecycle | `056b53b` | Start-once/wake equivalence; no drain introduced |
| Session execution | `d897130` | Six existing declarations relocated without body changes |
| Session finalization | `ae1a535` | Structural equivalence, Go regressions and eight Windows artifacts |

Exact test scopes and pre-commit receipt identities are retained in the
[cleanup ledger](roadmap-status.md#cleanup-and-modularization). Do not rerun a
passing suite merely because a commit or roadmap changed. Reuse evidence only
while its relevant source, dependencies and artifact inputs remain unchanged.

The read-only tracked-source inventory currently identifies these review areas:

| Source | Lines, excluding trailing empty line | Reason to inspect |
| --- | ---: | --- |
| `internal/httpapi/web/app.js` | 9,988 | Timeline, Settings and application orchestration remain together |
| `internal/engine/engine.go` | 2,604 | Settings, Bridge integration and other Engine responsibilities remain |
| `internal/httpapi/server.go` | 2,007 | HTTP adaptation and capture-related routing need a boundary review |
| `internal/collection/headless/worker/native-pilot/x-native-owner-runtime.mjs` | 2,020 | Transport, operation routing and owner orchestration remain substantial |
| `AkuBridge/service-worker.js` | 2,548 | Background dispatch, surface ownership and extension integration |

Size is a navigation aid, not proof of dead code or a required line-count target.
The inventory found no unreferenced local named functions in its narrow `app.js`
scan. It found one byte-identical receipt helper pair in separately packaged
Sidecar/Bridge consumers, covered by a contract-parity test. Neither observation
is whole-program dead-code proof or automatic permission to remove a file.

## Invariants for every increment

- Engine, Store, Coordinator, capture Manager and source controllers retain
  their existing authority. Do not introduce another scheduler or owner of the
  same durable state merely to wrap a large method.
- Preserve `e.mu`, existing atomic sections, `schedule` ownership, worker/drain
  accounting, cancellation tombstones, start-once/wake behavior and lock order.
  Finalization retains `schedule`; session creation releases `operation` before
  entering scheduling. Deferred cleanup remains part of worker lifetime.
- Preserve public/private wire schemas, DTOs, routes, status/error categories,
  null/default semantics, supported versions and build-tag boundaries.
- Preserve Store migration/downgrade barriers, claim/result fencing, replay
  rejection and accepted-evidence restart behavior. Existing non-atomic error
  outcomes must not become an invented transaction or rollback contract.
- Preserve native account/session evidence, source isolation, cookies, document
  observers, capabilities, deadlines, owned-target cleanup and reader lifetime.
  X and IG retain distinct evidence recipes. Unqualified IG is not a replacement
  authority for X.
- Preserve first cause separately from cleanup/delivery failure and retain
  bounded diagnostic projections. Product preferences, consent and retention
  semantics do not change as an incidental cleanup effect.

## Execution stages and current status

Each stage contains separately reviewable increments; a row is not permission
to execute every substep in one patch. The recommended order is CL-1 through
CL-8. Select only evidence-backed removals in CL-2; retain candidates whose
replacement or supported-consumer decision is still pending. Revisit their
ledger entries when a later modularization creates a proven obsolete path.

| Stage | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| CL-0: inherited bounded cleanup | Eight increments above complete | Scoped receipts retained | Cleanup activation not recorded | Bounded code reviews complete; no whole-repository completion claim |
| CL-1: consumer and compatibility inventory | [Entry/consumer map](cl1-consumer-inventory.md) and [32-boundary candidate ledger](cl1-cleanup-candidates.md) delivered | Read-only four-tag metadata, native/package/reference and source-preservation checks; no product test/build at the inventory checkpoint | Not applicable to an audit | Inventory accepted as the historical baseline; final dispositions reviewed under CL-8. Original counts are not the current backlog; external-consumer and capture-time dependency limits remain scoped |
| CL-2: proven obsolete-code retirement | Earlier wrappers plus G10 `DeleteMemoryItem` removal committed with roadmap as `21ac9ad` | G10 exact remainder check, full Store/HTTP regressions and four-mode Windows compilation passed | Not performed for G10 | Selected removal reviewed/accepted; remaining ledger clusters are retained, deferred or excluded under the final CL-8 map, not unapproved deletion tasks |
| CL-3: backend responsibility boundaries | G06 `d82f53b`; candidate cleanup `7d00d9c`; Settings file move `02e063e`; profile merge `aea3867`; executable preparation `c303ed4`; diagnostics export `1068e35`; Bridge status `7b2d0d8`; HTTP Bridge commands `7cb712b` and media recapture `be9fcd2`; Engine media recapture `c614512`; Engine Bridge commands `3e13434`; Engine media provenance `8d7f7b6`; Engine AI detection `b84a62d`; Engine session admission `4bf0731`; observation contracts `d9c3025`; process recovery `5b39fa4`; reasoning result `eb05ac8`; capture rounds and deterministic Store test `683b1e8`; local acquisition policy `f16f11a`; vision-lane routing/coverage `25c3339`; selection corrections `f3d46b8`; AI feedback `6a1ce33`; FullReset `da019dd`; Library removal/Forget `a19bf86`; passive media evidence `2bdc0f1`; HTTP update/session `d32c065`; HTTP media entry `90e3a7f`; HTTP bootstrap `97bbe51`; capture-owner path guard `b482ba2`; HTTP reload actions `6ea303c`; telemetry rotation/shutdown guard `237dbd1`; HTTP development X controls `1814c9b`; HTTP semantic corrections `3c5e61a`; HTTP AI feedback `a3718ff`; HTTP Inbox selection review `3f4a66d`; HTTP Inbox reads `ce57e79`; HTTP model usage `af8d722`; HTTP vision evaluation `cf534ec`; HTTP Timeline composition `3cf90b4`; automatic HTTP handler completion `9da6ed8`; acquisition execution `8027599`; candidate evaluation `ed35d82`; vision persistence tests `70ea120`; empty-candidate completion `de95c58`; Store Timeline read-model `bee6a9f`; automatic Store feature separation `f27e91b` (95 declarations/15 feature files); validation detours `edb246d`/`8dc3f5e`; earlier lifecycle/session seams retained | Latest combined Capture/Collection/Bridge builder/Selection/EventEngine/Reasoning/Domain/Detector/Inspector/Store/Engine/HTTP/config/application scope selects 14 packages (13 with tests, Bridge builder compile-only): default 3,267 pass/50 skip and dev 4,111 pass/62 skip, zero failures; whole Store default 462 pass/5 skip and dev 614 pass/7 skip before/after (45 new results); earlier Store Timeline read-model characterization 72 pass/1 skip default/dev retained; empty-candidate completion characterization 218 default/dev before/after retained; earlier vision persistence characterization 94 default/dev retained; candidate evaluation characterization 88 default/dev before/after retained; acquisition execution characterization retained; remaining HTTP feature characterization retained; Timeline characterization retained; vision-evaluation characterization retained; model-usage characterization retained; Inbox read characterization retained; selection-review characterization retained; AI-feedback characterization retained; semantic correction characterization retained; development X characterization retained; earlier telemetry/reload characterization and concurrency evidence retained; earlier guard/bootstrap receipts retained, four Store build-tag graphs and 16 Windows artifacts pass. Earlier acquisition-policy/capture-round/privacy/HTTP/Bridge status/diagnostics/Node/cancellation/profile receipts retain their own scope | Not performed for these increments | Backend source CL-3.1/CL-3.2/CL-3.3 closed after accepted responsibility-map/exit-criteria review at 6b9b3a0, with code through f27e91b; [retain/defer decisions](#cl-3-source-closure-2026-11-03) recorded. No material ownership gap requires another extraction. Runtime activation, operational acceptance and CL-8 source sign-off remain separate |
| CL-4: frontend feature ownership | 3b9cfd1: 374 functions/20 feature factories; 30 shared/root functions retained | Frozen 29/29 characterization; Node 360/360; HTTP/app default 1,322 pass/25 skip and dev 1,345 pass/12 skip; four Windows modes/12 artifacts; actual embedded desktop/mobile checks | Not performed | CL-4.1/CL-4.2/CL-4.3 source closed after scoped review; retain/defer map below. Operational acceptance and CL-5 through CL-8 remain separate |
| CL-5: native runtime and QA boundaries | Reviewed automatic cohort based on 555dea9: 47 declarations/seven production modules; e146d40 projector fix retained; commit bound to publication readback | New characterization 31/31 before; combined 62/62 after/final; native Node 780/780; seven-package default/dev Go; four Windows modes/16 PE artifacts; sealed manual/daily/main 80/81/100 modules | Not performed | Source CL-5 closed after owner-authorized review; retain/defer map below. Operational acceptance remains separate |
| CL-6: Bridge responsibility boundaries | AkuBridge 6ce23f7: reporting factory/three moved declarations; retained operation/resource owners mapped | Reporting 12 before/after; Bridge Node 515, native-host Go, Sidecar Node 44 and default/dev Go pass; 18 Windows artifacts; three projected packages/70 files each | Not performed | Source CL-6 closed after owner-authorized scoped review; explicit retain map below and publication readback bind the code/docs/remote identities. Operational acceptance remains separate |
| CL-7: build, packaging and test organization | Sidecar F02 2af9698, Browser worker filter 8190b45 and Bridge package test-filename guard/10 CLI cases 6875355; source/package/harness ownership reconciled | Bridge 525 pass and final/publication characterization 10/10; Sidecar Node 762 pass/2 opt-in skip; embedded/static 16 default/dev; four tag graphs and 27 auxiliary/platform/tag metadata queries; 12 Windows PE/amd64 artifacts; five 70-file projected packages and 20 source recipes verified. Earlier worker cold/warm and exact sealed-byte receipts retained | Not performed | Selected source CL-7 closed after owner-authorized scoped review; retain/defer decisions and triggers below. Dirty Browser build/cache P05 excluded, macOS/installed/live acceptance and CL-8 remain separate |
| CL-8: integration and source sign-off | Bridge B02 32b3834: 18 real-caller tests; final 32-row ledger and cross-repository matrix reconciled; explicit source-candidate identity v118 remains separate from historical release v116 | B02 18/18; canonical Bridge 543/543; Sidecar negotiation/security/split-heartbeat 47 default and 47 dev. Candidate gate passes; prior behavioral, package, visual and 12-artifact Windows evidence reused after source/receipt/artifact hash review | Not performed | Selected source CL-8 closed after owner-authorized final review. One removal, six extractions, 21 retains, three deferrals and one excluded P05. Commit/remote identities bound to readback; release/install/macOS/live acceptance remains separate |

### CL-1: inventory before deletion

1. Map production roots for default, development, fixture and combined builds,
   native Node bundles, extension assets, installer/launcher and QA harnesses.
   Include dynamic imports, generated catalogs, HTTP/extension messages, event
   handlers and externally consumed symbols; text-search absence is insufficient.
2. Inventory legacy paths, wrappers, version branches, migration code and test
   fixtures. Identify the supported consumer/version or durable-data boundary
   for each. A filename containing `legacy`, `fixture` or `compat` is not a verdict.
3. Map locks, resources, effect order and failure propagation for candidate seams.
   Preserve existing proof and cleanup authority rather than moving it twice.
4. Produce a candidate ledger with path/symbol, role, callers in every relevant
   build/package, supported contract/version, proposed disposition, evidence,
   required checks, approval and rollback constraints.

Dispositions: `remove`, `extract`, `retain-required`, `defer-until-replacement`,
or `unverified`. Every removal requires consumer evidence and a separately
approved patch. Unknown external consumers remain unresolved, not presumed dead.

Exit: reviewed candidate ledger, production dependency graphs, retained-contract
list and the first concrete proposed cleanup. No code deletion in this stage.

### CL-2: retire only proven obsolete paths

- Remove one approved wrapper/path at a time after all relevant entry points
  have migrated and supported compatibility obligations are accounted for.
- Reconcile imports, staging lists, sealed-package manifests, fixtures, CLI
  help and docs in the same bounded increment. Prove the replacement remains
  reachable in every supported build; do not leave a permanent forwarding shim.
- Retain migration, provenance, negative guard and supported-version tests.
  Remove an obsolete test only when its contract is explicitly retired or an
  equivalent test still checks the required behavior; record that rationale.
- Treat the two receipt-helper copies as a packaging/contract decision. Keep
  parity until an approved distribution mechanism can preserve both packages.

Exit per increment: no remaining supported caller of the retired path, unchanged
supported behavior, relevant negative tests passing and a reviewed commit.
Do not delete a consumer merely to make the dependency scan appear clean.

### CL-3: backend responsibility boundaries

Current source gate: closed; see the [closure decision](#cl-3-source-closure-2026-11-03).
The increment chronology below retains its original evidence scope.

- **CL-3.1 Settings:** candidate failure cleanup is committed as `7d00d9c`;
  the first file boundary moves `SaveSettings` unchanged to
  `settings_execution.go` in `02e063e`; pure profile-map merge follows in
  `aea3867`; executable preparation follows in `settings_executable.go` as
  `c303ed4`. Validation detours `edb246d` and `8dc3f5e` resolve observed reader
  cancellation and offline Node clock-boundary failures while preserving guards.
  The later provider-selection micro-extraction was declined: the 98-line
  `SaveSettings` remains cohesive and that extra wrapper has insufficient value.
  Further Settings phase splitting is deferred. Reuse `providerSwapPlan`;
  preserve candidate cleanup, `operation` locking, profile projection and
  post-save error behavior. Analyze one coherent seam before considering a
  broader service interface.
- **CL-3.2 Engine:** diagnostics export and its nine helpers are separated unchanged
  in `diagnostics_export.go` as `1068e35`; Bridge status's nine functions/two types
  follow in `bridge_status.go` as `7b2d0d8`, retaining constants, action catalog,
  lock scopes and heartbeat side effects. Both have characterization and passing
  combined regressions/Windows compilation. Ten Engine media-recapture methods
  now belong to `media_recapture.go` as `c614512`, with unchanged receivers,
  locks, context and queue/claim/result/cleanup/provenance order. Seven Engine
  Bridge command methods now belong to `bridge_commands.go` as `3e13434`;
  receiver, lock/context scopes, protocol, guards and recovery/persistence/
  scheduling order remain unchanged. Five Engine media-provenance methods now
  belong to `media_provenance.go` as `8d7f7b6`, with worker admission,
  queue/claim/result order and context/cancellation contracts retained. Five
  AI-detection methods now belong to `ai_detection.go` as `b84a62d`, retaining
  fast/deep admission, profile/job/result order and media-provenance cancellation.
  Session admission/creation now belongs to `session_admission.go` as
  `4bf0731`, retaining guard/source/lease/cancellation ordering and operation
  release before scheduling. Three shared observation functions now belong to
  existing `observations.go` as `d9c3025`, with unchanged caller/error/evidence
  contracts. Process-error/usage-limit recovery now belongs to
  `process_recovery.go` as `5b39fa4`, retaining durable failure/pause/peer/
  advancement order, restore gates and existing locks/contexts. Three reasoning
  result helpers now belong to `reasoning_result.go` as `eb05ac8`, with
  binding/validation/selection/completion order and evidence guards unchanged.
  Eight capture-round/coverage/yield/continuation helpers now belong to
  `capture_rounds.go` as `683b1e8`; the validation detour fixes only the
  probabilistic Store privacy-test assertion. Four local acquisition-policy helpers
  now belong to `acquisition_policy.go` as `f16f11a`, with unchanged signatures,
  reasons, caller order, scalar readers, continuation and guards. Two vision-lane
  helpers now belong to `vision_lane.go` as `25c3339`, preserving media-only/text
  isolation, shared Coverage/nested aliases, queue/receipt order and manual_review
  availability. Selection correction/undo now belongs to `selection_corrections.go`
  as `f3d46b8`, with Engine receivers, operation/mu scopes, durable/enrichment/error
  order and exact cancellation behavior retained. AI feedback/write/undo/history
  now belongs to `ai_feedback.go` as `6a1ce33`, retaining per-method locks, durable
  writes before review handoff, logged failures after commit and exact cancellation
  behavior. FullReset now belongs to `full_reset.go` as `da019dd`, preserving
  cancellation/admission, Store authority, candidate cleanup and runtime effects.
  Library removal/Forget now belongs to `library_removal.go` as `a19bf86`, with
  exact entry methods and Store/queue/context/cancellation contracts retained.
  The passive evidence method is colocated in existing `media_recapture.go` as
  `2bdc0f1`; all 52 remaining central declarations are retained. Central-file
  splitting stops at this checkpoint. HTTP update/session adaptation is now
  separated as `d32c065`, without closing broader Engine/runtime or CL-3 work.
  Acquisition planning orchestration now belongs to acquisition_execution.go as
  `8027599`, with 230 before/after characterization results and passing default/dev
  regression plus four-mode Windows builds. Candidate evaluation/completion now
  belongs to candidate_evaluation.go as `ed35d82`, with 88 characterization
  results before/after and passing regression/four-mode Windows builds; startNext
  stays in process. Vision queue handoff is reviewed and retained, with partial
  persistence/promotion/cross-run deduplication tests in `70ea120`. Observation
  preparation is reviewed and retained; empty-candidate completion now belongs
  to empty_candidate_completion.go as `de95c58`, with 218 before/after
  characterization results and passing regression/four-mode Windows builds.
  Scoped enrichment/read-model ownership review retains Engine facades and
  existing feature owners. The approved Store continuation moves six methods
  and one SQL constant to timeline_read_model.go as `bee6a9f`, with unchanged
  bodies/SQL and passing characterization/regression/four-mode Windows builds.
  The later automatic Store run separates 95 further declarations into 15
  feature files and retains 13 foundation declarations in store.go (233 lines);
  it is reviewed/committed as `f27e91b` and not activated. This ownership/file-
  separation checkpoint does not close CL-3.2 or CL-3. Keep the Engine
  facade and existing Store authority; do not repeat
  completed lifecycle/session moves or split small blocks merely to reduce size.
- **CL-3.3 HTTP capture:** separate request/response adaptation from application
  orchestration where a complete use case is identifiable. Preserve endpoint
  shapes, authentication/admission, status codes, error categories and call order.
  Four Bridge command handlers (next, pending, observation and failure) now belong
  to `bridge_commands.go` as `7cb712b`, with characterization before/after and
  passing regressions/Windows compilation. Router predicates, normalized path,
  context, authentication, protocol, guards and side-effect order are retained.
  Media-recapture claim, observation and failure follow in `bridge_media_recapture.go`
  as `be9fcd2`; 54 characterization results pass before/after, with combined
  regressions and four-mode Windows compilation. Their distinct error mappings,
  queued/claimed failure behavior and stale-claim side effect remain unchanged.
  Update creation, active-session polling, detail and cancellation now belong
  to `session_updates.go` as `d32c065`, with 53 characterization results before/
  after and passing default/dev regressions plus four-tag Windows compilation.
  Existing router, shared security/projection and Engine/Store ownership remain
  unchanged. Media-recapture status, enqueue and passive evidence now belong to
  `media_recapture.go` as `90e3a7f`, with 42 characterization results before/
  after, passing regressions and four-mode Windows compilation. UI/Bridge security,
  relay authority, privacy projection, ID parsing, defaults and error order remain
  unchanged. Bootstrap composition follows in `bootstrap.go` as `97bbe51`,
  with 25 characterization results before/after, matching default/dev regression
  and four-mode Windows compilation. The router responsibility map is recorded;
  reload-action adapters follow in `bridge_actions.go` as `6ea303c`, with
  45 characterization results before/after and matching regression/Windows checks.
  Fourteen scoped HTTP boundaries are established, including development X controls
  `1814c9b`, semantic corrections `3c5e61a`, AI feedback `a3718ff`, Inbox selection
  review `3f4a66d`, Inbox reads `ce57e79`, model usage `af8d722`, vision
  evaluation `cf534ec` and Timeline composition `3cf90b4`, with unchanged
  characterization, matching regression and four-mode Windows compile; CL-3.3
  remains partial.
  Capture-owner path normalization is corrected separately as `b482ba2`,
  with default/dev regression and four-mode Windows compile; the guard fix itself
  adds no feature extraction. Telemetry rotation/shutdown compatibility follows
  as `237dbd1`, with controlled concurrency, matching regression and four-mode
  Windows compile; this is another correctness increment, not an extraction.
  Automatic HTTP handler completion follows as `9da6ed8`: 58 adapters in 24
  feature boundaries, all 106 top-level arms delegated, 89 characterization
  results before/after, default/dev regression and four-mode Windows builds.
  Central handler separation is complete; infrastructure/shared helpers remain
  retained. Broader CL-3 and operational acceptance stay open.

Exit: a reviewed responsibility map and separately validated extractions, with
no second source of state, broader admission, new concurrency or wire changes.

### CL-4: frontend feature ownership

Current status: source closed after the owner-authorized automatic run and
scoped review/publication of 3b9cfd1. See the [current result](#automatic-frontend-feature-separation-result-2026-11-03)
for CL-4.1/CL-4.2/CL-4.3 ownership, verification and retained boundaries. Scoped
local acceptance passes; publication is bound to the readback below.
The earlier [More/Less result](#cl-41-timeline-moreless-result-2026-11-03) keeps
its historical scope. Source CL-3 closure is unchanged.

- **CL-4.1 Timeline:** separate rendering and item-scoped action ownership from
  application bootstrap/polling. Preserve revision/epoch checks, lazy media,
  drawer/context behavior, cleanup barriers and keyboard/focus behavior.
- **CL-4.2 Settings:** separate draft/read/serialization, rendering, persistence
  actions and dialogs while retaining dirty-state and save/error semantics.
- **CL-4.3 App entry:** keep bootstrap, navigation, polling and teardown explicit;
  attach handlers once and release them at the same lifecycle boundaries.

Use existing modules and contracts first. Introduce a feature controller only
if it gives a real owner to event/subscription lifetime. Avoid a generic global
controller or a visual redesign inside this cleanup. Under the active Goblin
workflow, frontend work uses Sol/Astra profiles.

Exit: focused feature tests plus accountless rendered desktop/mobile interaction
checks for the moved behavior. No signed-in source or live capture is required
to establish a source-only frontend extraction.

### CL-5: native runtime and QA boundaries

Current status: source closed after owner-authorized review. The [automatic CL-5 result](#automatic-cl-5-native-responsibility-separation-result-2026-11-04) records the responsibility map, unchanged passing evidence and explicit retain/defer decisions. Publication readback binds the reviewed code to its commit and remote SHA. No activation is established.

- Map operation routing, CDP framing/projection, owner lifecycle and source
  evidence orchestration in `x-native-owner-runtime.mjs` before choosing a split.
- Keep production factories and the 45 shared helpers in their production roots.
  Refine QA probe dispatch only around existing sealed operation contracts.
- Preserve startup/seed ordering, original deadlines, late-result cleanup,
  first-cause diagnostics and all source-specific authority checks. Reuse the
  shared kernel and existing narrow ports; do not build another native framework.

Exit: runtime/QA dependency direction remains correct, no fixture imports in
production package graphs, existing protocol/provenance/negative conformance
passes, and source-pinned manual/daily/main bundles remain complete.

### CL-6: Bridge responsibility boundaries

Current status: source closed after owner-authorized review of AkuBridge 6ce23f7. See the [automatic CL-6 result](#automatic-cl-6-bridge-reporting-separation-result-2026-11-04) for the reporting extraction, unchanged guard/ownership proof, validation and deliberate retain map. Publication readback binds both repositories; activation remains separate.

- Map background messaging, capture dispatch, tab/window ownership, readiness
  and lifecycle consumers in `service-worker.js` and related modules.
- Extract one responsibility with its message/event wiring and tests. Keep
  source parsers in adapters and durable decisions in Sidecar/Store.
- Preserve managed-window journals, exact lease/generation checks, reader
  lifetime, extension identity and separately packaged receipt parity.

Exit: message/manifest/asset compatibility and contract-parity checks pass;
resource ownership is explicit without additional singleton state or fallback.

### CL-7: build, packaging and test organization

Current status: selected source closed after the [CL-7 source-closure review](#cl-7-source-closure-2026-11-04), including [F02](#cl-7-f02-test-asset-boundary-result-2026-11-04), the [worker packaging boundary](#cl-7-worker-test-filename-boundary-result-2026-11-04) and Bridge 6875355. Existing supported owners are retained with return triggers. Pre-existing dirty Browser build/cache P05 remains excluded for separate review; source closure does not establish runtime, macOS, installed or signing acceptance.

- Reconcile source lists, package seals, generated manifests, asset pins and
  harness inputs after each move. Test the packaged consumer, not just imports
  from the source checkout.
- Classify tests by the behavior/boundary they prove. Retain unique guard,
  migration, cancellation, restart and failure coverage; consolidate duplicated
  setup only when it preserves independent assertions and lifetime ownership.
- Review build/cache tooling separately, including pre-existing Browser changes.
  Do not absorb unrelated work into a cleanup commit or delete cached resources.
- Use project-local QA/build output and the existing source-cache policy. Do
  not install or upgrade tools/dependencies merely to reorganize code.

Exit: supported entry-point/build-tag/package graphs are accounted for; no
production dependency on QA paths or silently missing asset; test commands and
evidence locations are reproducible for a clean source checkout.

### CL-8: integration and source sign-off

Current status: [selected source closed](#cl-8-source-closure-2026-11-04)
after owner-authorized final review, with commit/publication bound to readback.
The selected ledger is reconciled, including B02 retention and the explicit P05
exclusion. No new activation or release qualification follows from this status.

Close every selected candidate as removed, extracted, retained with a supported
reason, or deferred with an explicit dependency/owner. Material unresolved
consumer or guard questions prevent claiming comprehensive source completion.
Record separately what was intentionally deferred rather than hiding it in a
lower file count.

Complete the affected cross-repository validation matrix, review the final
dependency direction and update the status index with source/artifact identities.
Commit/push only when requested. Source sign-off means implemented/tested and
reviewed for that scope; it does not mark activated or operationally accepted.

## Design patterns: use only where the boundary earns them

| Existing mechanism / candidate pattern | Appropriate use | Constraint |
| --- | --- | --- |
| Engine facade and application use cases | Stable entry points around cohesive orchestration | Do not duplicate Store/controller authority behind another object |
| Plan/apply separation | Settings/provider candidate construction versus post-persistence effects | Reuse `providerSwapPlan`; no invented atomic rollback |
| Adapter / narrow port | Source-specific evidence and platform transport integration | Preserve distinct X/IG proofs and existing sealed capabilities |
| Explicit state transitions | Existing work/scheduler/source lifecycles | Reuse current states/locks; no new protocol state machine during cleanup |
| Feature controller / explicit disposal | Frontend event, subscription and rendering ownership | Avoid new global mutable state and preserve handler teardown |
| Pure policy helper | Shared deterministic predicates | Packaging, resource anchors and cross-runtime parity must remain explicit |

No pattern is a completion requirement by name. Prefer fewer responsibilities
and clearer dependency direction over new layers, factories or interfaces.

## Validation and evidence rules

Choose checks for the actual boundary changed; start with structural equivalence
and the narrowest meaningful behavioral regressions. Add a test only for a real
coverage gap. Do not replace meaningful tests with assertions that mirror a move.

| Boundary | Required evidence for the affected increment |
| --- | --- |
| Go lifecycle/Settings/HTTP/Store | Changed-package regressions; relevant cancellation, lock/order, accepted-evidence restart and error cases; HTTP/Store contracts as affected |
| Native runtime/packaging | Node protocol/owner/transport/provenance checks; production graphs and sealed assets; known baseline failures identified before a success claim |
| Frontend | Existing feature tests, event/teardown/cache/epoch regressions and accountless rendered interaction checks |
| Bridge | Affected module/message/adapter tests and receipt/policy parity; package verification when wiring/assets change |
| Windows | Engine test binary and Sidecar application compile for default, `auth_session_dev`, `auth_session_fixture` and combined tags when Go/native composition changes; related Bridge native-host/Browser builder checks when touched |
| Final source integration | Relevant full application-package suites and cross-package regressions; fresh artifact hashes and packaging evidence for changed inputs |

The established broad Go baseline covers `internal/igdiagnostic`, `internal/appshell`,
`internal/engine`, `internal/httpapi` and `cmd/akusidecar`, with Store coverage
selected or broadened for actual persistence changes. Avoid indiscriminate
`go test ./...` across synthetic build fixtures. The existing read-only
`scripts/audit-source-structure.mjs` is an inventory aid, not semantic proof.
Review scripts for opt-in/profile/process behavior before executing them;
synthetic and authenticated acceptance are distinct.

Each increment's result must name scope, source commit/tested payload, affected
consumers, equivalence allowance, commands/outcomes, known failures, artifact
identities, remaining risk and four-dimensional status. Preserve raw receipts;
record later Git state in the roadmap instead of rewriting old evidence.

## Separate operational gate

Activation or a live trial requires a concrete, separately authorized operational
step with reviewed artifacts, compatible Store schema, cleanup/drain and rollback
plan. Git revert is not database rollback. A new binary build is not evidence
that a running process uses it. Do not recycle old trial permission.

The source cleanup may reach source acceptance while activation remains pending.
Operational acceptance, if requested later, records its exact scenarios and
does not silently qualify native IG, longer acquisitions or repeated mixed-source
reliability. This roadmap schedules no live trial or configuration change.

## CL-3 source closure (2026-11-03)

Decision: **source CL-3 is closed**. The owner accepted the read-only closure
review of Settings, Engine, HTTP and Store at Sidecar `6b9b3a0`, then authorized
this documentation synchronization, commit and push. Product code remains
through `f27e91b`. No material ownership gap requires another extraction to
meet the existing CL-3 exit condition. This is backend source acceptance for
CL-3.1/CL-3.2/CL-3.3, not whole-repository source sign-off or runtime acceptance.

| Area | Responsibility and established boundary | Source disposition |
| --- | --- | --- |
| CL-3.1 Settings | Engine owns normalization/validation, provider/executable preparation, persistence ordering and post-save runtime effects; Store owns durable settings. settings_execution.go, settings_profiles.go, settings_executable.go and providerSwapPlan retain those roles | Implemented/tested and source accepted; retain cohesive SaveSettings and existing plan/apply ownership |
| CL-3.2 Engine | Engine owns dependencies, locks, admission, cancellation and dispatch. Existing feature/lifecycle/session files own substantive workflows; process delegates acquisition, candidate evaluation and empty completion. Enrichment remains feature-specific | Implemented/tested and source accepted; retain the facade, observation preparation and compact vision handoff |
| CL-3.3 HTTP | Server feature handlers own request/response adaptation. Engine/Store keep application/data authority; all 106 top-level router arms delegate. Server/private transport retain HTTP and connection-bound infrastructure | Implemented/tested and source accepted; retain routing, security, serialization, lifecycle and split transport |
| Store continuation of CL-3.2 | Store owns durable transactions, claims, pause/fencing/replay checks, migrations and read-model projection. Timeline read-model and the 95-declaration/15-file central feature cohort are separated | Implemented/tested and source accepted; retain 13 core declarations and existing cohesive feature owners |

| Existing exit criterion | Closure assessment |
| --- | --- |
| Reviewed responsibility map | Satisfied by the cross-owner map above and the read-only review; no unowned selected backend workflow was identified |
| Separately validated extractions | Satisfied by recorded characterization before/after, scoped equivalence/reviews, default/dev regressions and four-mode Windows compilation; each receipt keeps its original scope |
| No second source of state | Engine state/locks, Store durable authority, capture Manager/Coordinator leases and private transport conversation state retain their existing owners; feature files do not create a replacement owner |
| No broader admission or new concurrency | Refactor boundaries preserve existing operation/mu/schedule scopes, start-once/wake, worker lifetime, cancellation, lease handoff and effect order; no new scheduler, drain or worker is introduced by source closure |
| No wire changes | Routes, DTOs, protocol/constants, authentication, error/output contracts and guards are retained; no observer or supported compatibility contract is relaxed |

### Retained responsibilities and deferred changes

Retain is an intentional current owner, not an unfinished extraction queue.
Deferred work is not needed for this source gate and needs a new bounded
proposal/approval when its trigger occurs. File length alone is not a trigger.

| Responsibility/change | Decision and current owner | Concrete revisit trigger |
| --- | --- | --- |
| SaveSettings sequencing and provider-selection block | retain-required: cohesive Engine operation; the additional provider-selection wrapper remains declined | A concrete new caller or validation/order defect shows a reusable responsibility that cannot remain clear in the existing method |
| Further Settings phase/service decomposition | deferred: reuse Engine and providerSwapPlan; preserve candidate cleanup, profile projection and post-save partial-error behavior | A provider/runtime change creates an independently testable phase with distinct resource ownership and a demonstrated coupling problem |
| Engine facade and scalar coverage readers | retain-required: 52 central declarations keep dependencies, stable entry points and existing consumers; no pending central-file move | A concrete testability or coupling problem justifies a complete responsibility, rather than another forwarding wrapper |
| Observation preparation and vision queue handoff | retain-required: process keeps the accepted-evidence/coverage/continuity/queue sequence; Store owns durable queue state | A new consumer or behavior change needs an independent boundary with explicit partial-persistence, promotion, deduplication and error-order characterization |
| Read-model and enrichment ownership | retain-required: Store queries/hydrates/projects; Engine facades and feature-specific enrichment coordinate existing cancellation/effect order | A demonstrated cross-feature contract requires shared behavior; no generic enrichment service is preselected |
| HTTP routing, lifecycle, security and JSON helpers | retain-required: Server infrastructure and existing feature subrouters | A real shared contract or lifecycle defect identifies a useful extraction; central-file size is insufficient |
| Private split-capture/native-reader transport | retain-required: Server transport owns capability/epoch, queue, waiter and connection lifetime; borrows existing capture leases and delegates application admission to Engine/Store | A concrete transport/owner coupling, cancellation regression or protocol evolution requires a separately reviewed boundary; preserve the single queue/lease authority |
| Store opening/initialization, clock and supported schema/compatibility paths | retain-required: Store composition root and existing migration/guard owners; initialize preserves ordered setup | A supported-consumer retirement decision or concrete initialization/migration change supplies evidence and negative tests; old names/versions alone do not authorize deletion |
| Deeper native/QA, Bridge, consumer/package and final integration work | outside this closure: CL-5, CL-6, CL-7 and CL-8 retain their separate owners/status | Select an evidence-backed increment in that stage; CL-3 closure does not waive its entry, compatibility or acceptance checks |

### Evidence and status dimensions

The latest combined [Store-run validation](../runtime/qa/store-auto-refactor-20261103/validation.json)
records default 3,267 pass/50 skip and dev 4,111 pass/62 skip, zero failures,
across 14 selected packages (13 tested, Bridge builder compile-only), four tag
graphs and 16 Windows amd64 artifacts. Whole-Store before/after results match:
default 462 pass/5 skip and dev 614 pass/7 skip, including 45 new parent/subtest
results. These are recorded results, not new runs or counts of unique scenarios.
The [published readback](../runtime/qa/store-auto-refactor-review-20261103/final-readback.json)
binds the Store payload and preceding roadmap publication. Prior
[HTTP](../runtime/qa/http-features-20261103/validation.json),
[Engine completion](../runtime/qa/empty-candidate-completion-20261103/validation.json)
and [Settings review](../runtime/qa/settings-executable-review-20261101/review.json)
retain their source/artifact scope. The read-only closure review checked current
source/test hashes and receipt identities; later approved moves account for
historical parent-file hashes. No old receipt was rewritten to claim a new run.

| Dimension | CL-3 closure status |
| --- | --- |
| Implemented | Recorded scoped extractions through f27e91b; retained owners are intentional |
| Tested | Existing bounded characterization/regression/Windows evidence accepted; no product test or build rerun for this documentation-only update |
| Activated | Not performed or established for the closure payload; no restart/configuration change/trial |
| Acceptance | Backend source CL-3 closed by accepted responsibility-map/exit-criteria review; operational qualification and comprehensive CL-8 sign-off remain separate |

This closes the outstanding consolidated map review; it does not require
another backend extraction. At this closure checkpoint CL-4 was pending,
CL-5/CL-6 were partial and CL-7/CL-8 were open under their recorded scopes.
The later CL-4.1 result below records the first approved frontend increment. CL-2 retains its candidate
and supported-consumer constraints. Native IG qualification, real mixed-source
acceptance, installation/release work and unrelated dirty files are unchanged.

## Earlier incremental proposals and results

CL-1 deliverables are now available in the [consumer inventory](cl1-consumer-inventory.md)
and [candidate ledger](cl1-cleanup-candidates.md). The first concrete proposed
CL-2 patch, removal of the unused internal `Store.DeleteMemoryItem` forwarding
alias, has now been separately approved, applied and validated while preserving
the active `ForgetMemory`/`DeleteMemory` tombstone path. It and this roadmap were
committed as `21ac9ad` on request; no push or activation was performed.

G06 moved `ReasoningConfig.UnmarshalJSON` unchanged to
`internal/config/reasoning_config_decode.go`, retaining both strict modern/flat
shapes, receiver, precedence and errors. The separately approved repository
smoke-test correction now supplies the existing Windows development app-shell
capture-split context without modifying config or guards. Both changes were
reviewed and committed as `d82f53b`. See the
[result and historical baseline failure](cl1-cleanup-candidates.md#g06-result-2026-11-01)
and [latest passing evidence](../runtime/qa/repository-config-test-context-20261101/validation.json).

E01 / CL-3.1 then received two separately approved increments: candidate cleanup
on failed provider planning (`7d00d9c`) and the unchanged `SaveSettings` move to
`settings_execution.go` (`02e063e`). Both were reviewed and committed. See the
[scoped result](cl1-cleanup-candidates.md#e01-candidate-cleanup-and-settings-execution-2026-11-01)
and [latest extraction evidence](../runtime/qa/settings-execution-20261101/validation.json).
That file move introduced no phase helper. The later separately approved pure
profile-map helper is committed as `aea3867`; the observed baseline reader
cancellation race was separately fixed in `edb246d`. See the
[merge and validation-detour result](cl1-cleanup-candidates.md#e01-profile-merge-and-cancellation-validation-detour-2026-11-01)
and [latest combined passing evidence](../runtime/qa/split-reader-cancellation-20261101/validation.json).
Historical non-green receipts are retained; no broader service was introduced.

Executable preparation was subsequently approved, characterized and extracted
as `c303ed4`. The separately approved Node fixture correction is `8dc3f5e`.
The [result and limits](cl1-cleanup-candidates.md#e01-executable-preparation-and-node-fixture-validation-detour-2026-11-01)
and [review](../runtime/qa/settings-executable-review-20261101/review.json)
retain the initial failing receipt and verify the latest combined passing
source, logs, dependency pins and Windows artifacts. No runtime was activated.

The proposed provider-selection micro-extraction was analyzed and then declined:
`SaveSettings` is already cohesive at 98 lines, and another single-caller wrapper
would add navigation without a sufficient responsibility or testability benefit.
No such helper was implemented. This decision does not undo the completed
executable/profile boundaries or prohibit revisiting Settings if a concrete need
appears later.

The user instead approved moving the complete diagnostics export feature.
`DiagnosticsExport` and nine helpers now belong to `diagnostics_export.go`,
committed as `1068e35`. See the [result](cl1-cleanup-candidates.md#cl-32-diagnostics-export-feature-result-2026-11-01),
[validation](../runtime/qa/diagnostics-export-20261101/validation.json) and
[review](../runtime/qa/diagnostics-export-review-20261101/review.json).
All ten declarations and the original Engine remainder are equivalent; the
17 characterization results pass before and after the move. The default-range
unsupported usage window remains deliberately unchanged, not repaired here.

The user subsequently approved the complete Bridge status/compatibility move.
Nine functions and two types now belong to `bridge_status.go`, committed as
`7b2d0d8`. See the [result](cl1-cleanup-candidates.md#cl-32-bridge-status-feature-result-2026-11-01),
[validation](../runtime/qa/bridge-status-20261101/validation.json) and
[review](../runtime/qa/bridge-status-review-20261101/review.json). Constants and
`expectedBridgeActions` remain in Engine; AkuBrowser's text-based identity checker
retains its original anchors. `RecordHeartbeat`, revocation, recovery, reload and
scheduler ordering are unchanged. This is CL-3.2, not CL-6 extension completion.

The four HTTP Bridge command handlers were subsequently analyzed, separately
approved, characterized and moved to `bridge_commands.go` as `7cb712b`. See the
[result](cl1-cleanup-candidates.md#cl-33-http-bridge-command-feature-result-2026-11-02),
[validation](../runtime/qa/bridge-commands-20261102/validation.json),
[review](../runtime/qa/bridge-commands-review-20261102/review.json) and
[commit readback](../runtime/qa/bridge-commands-review-20261102/final-readback.json).
Only four case bodies changed in the router; moved bodies are identical except
for local `r.Context()` bindings. The same 53 characterization results pass before
and after; combined default/dev regressions and 12 Windows artifacts pass.
Engine/Store retain admission, fencing and lifecycle ownership. No push,
activation or live trial was performed for this increment.

The three HTTP Bridge media-recapture handlers were then separately approved,
characterized and moved to `bridge_media_recapture.go` as `be9fcd2`. See the
[result](cl1-cleanup-candidates.md#cl-33-http-bridge-media-recapture-feature-result-2026-11-02),
[validation](../runtime/qa/bridge-media-recapture-20261102/validation.json),
[review](../runtime/qa/bridge-media-recapture-review-20261102/review.json) and
[commit readback](../runtime/qa/bridge-media-recapture-review-20261102/final-readback.json).
Only three router case bodies changed. The initial source-mismatch fixture was
corrected before the passing baseline; no production guard was changed. Exact
source/log/artifact hashes retain the passing evidence through commit. No push,
activation or live trial was performed.

The ten Engine media-recapture methods were subsequently approved, characterized
and moved to `media_recapture.go` as `c614512`. See the
[result](cl1-cleanup-candidates.md#cl-32-engine-media-recapture-feature-result-2026-11-02),
[validation](../runtime/qa/engine-media-recapture-20261102/validation.json),
[review](../runtime/qa/engine-media-recapture-review-20261102/review.json) and
[commit readback](../runtime/qa/engine-media-recapture-review-20261102/final-readback.json).
Signatures and bodies are identical; the Engine remainder differs only in its
existing final CRLF normalized by gofmt. Characterization passes 34 default/35
dev results before/after, with current regressions and 12 Windows artifacts.
Generic capture-runtime ownership, Store, HTTP and passive enrichment remain
unchanged. No push, activation or live trial was performed.

The seven Engine Bridge command methods were subsequently approved, characterized
before relocation and committed as `3e13434`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-bridge-command-feature-result-2026-11-02),
[validation](../runtime/qa/engine-bridge-commands-20261102/validation.json), [review](../runtime/qa/engine-bridge-commands-review-20261102/review.json) and [commit readback](../runtime/qa/engine-bridge-commands-review-20261102/final-readback.json).
All seven signatures/bodies and the remaining Engine file are identical to their
original content, apart from removing the declarations from the original file.
The identical characterization payload passes 37 parent/subtest results in both
default/dev before and after relocation. Combined suites pass 801/1,475 results
with 38/48 skips, zero failures, and 12 Windows amd64 artifacts across four modes.
Shared guards, normalization/validation, Store and capture-runtime authority,
heartbeat/scheduler consumers and post-persistence error contracts are retained.
This is scoped implemented/tested/reviewed acceptance, without activation or live
source acceptance. No push, activation or trial was performed for this increment.

The five Engine media-provenance methods were subsequently approved, characterized
before relocation and committed as `8d7f7b6`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-media-provenance-feature-result-2026-11-02),
[validation](../runtime/qa/engine-media-provenance-20261102/validation.json), [review](../runtime/qa/engine-media-provenance-review-20261102/review.json) and [commit readback](../runtime/qa/engine-media-provenance-review-20261102/final-readback.json).
All five signatures/bodies are identical. The Engine remainder differs only in
gofmt alignment of `Epoch` and `ProviderName`; no worker algorithm or state
ownership changed. The same characterization payload passes 24 parent/subtest
results in both modes before/after. The latest combined regression scope now
includes Inspector and Store as well as the prior four packages: 1,214/2,040
passes, 43/55 skips and zero failures. These broader counts are not directly
comparable with the previous four-package totals. Four build-tag graphs and
12 Windows amd64 artifacts pass. Existing source/log/artifact evidence is reused
at matching hashes; no new product test/build, activation or trial is run by this
documentation synchronization. CL-3 and operational source acceptance remain open.

The five Engine AI-detection methods were subsequently approved, characterized
before relocation and committed as `b84a62d`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-ai-detection-feature-result-2026-11-02),
[validation](../runtime/qa/engine-ai-detection-20261102/validation.json), [review](../runtime/qa/engine-ai-detection-review-20261102/review.json) and [commit readback](../runtime/qa/engine-ai-detection-review-20261102/final-readback.json).
Method signatures/bodies are unchanged; only standalone setter alignment was
normalized by gofmt. The remaining Engine file is byte-identical after removing
the declarations. The identical characterization payload passes 32 parent/subtest
results in both modes before/after. Latest regression scope adds aidetector to
the previous six packages: default 1,270 pass/45 skip, dev 2,096 pass/57 skip,
zero failures, four build-tag graphs and 12 Windows artifacts. Broader totals are
not directly comparable with earlier six-package totals. Caller budget/onboarding
gates, worker/result contexts, usage-limit order and AI/provenance cancellation
remain unchanged. Source/log/artifact evidence is reused at matching hashes;
no new product test/build, runtime activation or trial is run by this sync.
CL-3 and operational source acceptance remain open.

The session-admission feature was subsequently approved, characterized before
relocation and committed as `4bf0731`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-session-admission-feature-result-2026-11-02),
[validation](../runtime/qa/engine-session-admission-20261102/validation.json), [review](../runtime/qa/engine-session-admission-review-20261102/review.json) and [commit readback](../runtime/qa/engine-session-admission-review-20261102/final-readback.json).
All four declarations and the Engine remainder are byte-identical. Characterization
passes 31 default/34 dev parent/subtest results before/after; seven-package suites
pass 1,301/2,130 results with 45/57 skips, zero failures. Four build-tag graphs
and 12 Windows artifacts pass. Guard/error order, source/policy selection,
cancellation before acquisition, lease ownership, durable-session error behavior
and operation release before schedule are retained. Source/log/artifact evidence
is reused at matching hashes; this sync runs no new product tests/builds,
activation or trial. CL-3 and operational source acceptance remain open.
The [current Engine responsibility map](code-structure-audit.md#current-engine-responsibility-map-2026-11-02)
separates completed boundaries, retained facade ownership and remaining review areas.

The observation-contract feature was subsequently approved, characterized before
relocation and committed as `d9c3025`. See the [result](cl1-cleanup-candidates.md#cl-32-engine-observation-contracts-result-2026-11-02)
and [validation](../runtime/qa/engine-observations-20261102/validation.json), [review](../runtime/qa/engine-observations-review-20261102/review.json) and [commit readback](../runtime/qa/engine-observations-review-20261102/final-readback.json). Identical characterization passes 61/74 default/dev results;
eight-package regressions pass 1,409/2,251 results with 45/57 skips and zero
failures, four build-tag graphs and 12 Windows artifacts. This adds Domain to
the preceding seven-package scope. Five caller files and all moved bodies are
unchanged; native validation still rejects missing evidence identity without
normalization. No activation or trial is performed; CL-3 remains partial.

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

No calendar estimate or cleanup percentage is assigned: this bounded inventory
does not establish a complete repository-wide denominator or close every
supported-consumer obligation.

## HTTP bootstrap and router-map checkpoint (2026-11-02)

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

### Feature placement for the automatic completion

| Feature file | Moved adapters |
| --- | ---: |
| `internal/httpapi/appshell_actions.go` | 1 |
| `internal/httpapi/diagnostics_export.go` | 1 |
| `internal/httpapi/ui_performance.go` | 1 |
| `internal/httpapi/health.go` | 1 |
| `internal/httpapi/collection_controls.go` | 2 |
| `internal/httpapi/runtime_control.go` | 2 |
| `internal/httpapi/calibration.go` | 2 |
| `internal/httpapi/database.go` | 3 |
| `internal/httpapi/calibration_sessions.go` | 4 |
| `internal/httpapi/onboarding.go` | 2 |
| `internal/httpapi/settings.go` | 2 |
| `internal/httpapi/reasoning_configuration.go` | 3 |
| `internal/httpapi/session_updates.go` | 1 |
| `internal/httpapi/content_context.go` | 3 |
| `internal/httpapi/living_topics.go` | 5 |
| `internal/httpapi/library.go` | 9 |
| `internal/httpapi/auto_update.go` | 6 |
| `internal/httpapi/capture_recovery.go` | 1 |
| `internal/httpapi/native_reader.go` | 1 |
| `internal/httpapi/timeline_memory.go` | 2 |
| `internal/httpapi/timeline_feedback.go` | 1 |
| `internal/httpapi/bridge_heartbeat.go` | 2 |
| `internal/httpapi/capture_surface_events.go` | 1 |
| `internal/httpapi/operations_reset.go` | 2 |

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

## CL-4.1 Timeline More/Less result (2026-11-03)

Code commit: `80acfb9`. The approved first Timeline item-action feature now
belongs to timeline-feedback.js: createTimelineFeedbackUI builds More/Less
controls and keeps sendFeedback private. app.js composes the feature using its
existing API, feedbackButton, cache invalidation, error and trace ports.
The request/handler bodies and app remainder are unchanged except the import,
composition and delegation, allowing indentation and CRLF/LF differences.
Shared Inbox buttons and native-post tracing stay in app.js. Bootstrap/polling,
Timeline rendering, Read later, semantic correction, AI feedback, media and
context-drawer ownership are unchanged; no new timer, observer or state owner.

| Dimension | Current evidence |
| --- | --- |
| Implemented | 80acfb9: isolated More/Less feature plus its composition, characterization, offline UI fixture and contract-test corrections |
| Tested | Frozen characterization 17/17 before and after; latest complete Node regression 304/304, zero failures. HTTP/application default 1,322 pass/25 skip, dev 1,345 pass/12 skip, zero failures. Four Windows amd64 tag modes include the embedded module and produce 12 Engine/HTTP/application artifacts |
| Activated | Not performed; no restart, production configuration change or signed-in provider trial |
| Acceptance | Scoped source/test review accepted with no findings. Accountless real-Chromium desktop 1100x850 and mobile 390x844 interaction/geometry match before/after; screenshots inspected and owned-tree cleanup verified. This is the item-action fixture, not full-app/provider acceptance. CL-4.1/CL-4 remain partial; source CL-3 stays closed |

The existing API wrapper and epoch rejection are retained. Success still
invalidates Library storage before returning feedback, then mutates the same
entry and renders selection. Failure retains the previous choice. Feedback
does not refresh Timeline. Repeated/concurrent clicks remain unblocked and
responses apply in completion order; no optimistic selection, deduplication,
serialization or new stale-response contract is added.

The initial broad Node run had two pre-existing restart-dev source assertions
that expected registration inside the old entry script. The separately approved
test correction follows restart-dev.ps1 delegation to dev-restart-workflow.ps1,
checks rejection of registration takeover, both vendor ownership checks, and
database/preflight/graceful-shutdown/resource/config/promotion order. Explicit
registration keeps its separate fences. No production restart/registration
script changed. Latest 304/304 Node evidence supersedes those failures without
rewriting the historical failing receipt. Library cache and Go embedded-asset
assertions now verify the new module and actual app wiring rather than the old
function location; their behavioral/guard intent is preserved.

Evidence: [feature validation](../runtime/qa/timeline-feedback-20261103/validation.json),
[latest combined Node correction](../runtime/qa/timeline-feedback-restart-tests-20261103/validation.json),
[scoped review](../runtime/qa/timeline-feedback-review-20261103/review.json), and
[publication readback](../runtime/qa/timeline-feedback-review-20261103/final-readback.json).
Earlier sourceHead/sourceDirty/committed fields retain capture-time meaning.
Publication reuses passing evidence after payload/log/artifact hash checks;
it does not rerun tests/builds or establish runtime activation.

### Proposed next cleanup analysis

Source CL-3 remains closed. The first CL-4.1 item-action increment, More/Less,
is implemented/tested/reviewed as 80acfb9; CL-4.1/CL-4 remain partial. The next
proposed bounded task is a read-only ownership review of buildTimelineReadLaterAction
and its existing timeline-memory-state.js helpers. Trace confirmation, per-item
in-flight admission, Saved response projection, Library storage invalidation,
status timer and re-render lifetime against existing app rendering/polling.
Decide whether a separate UI feature owner provides a concrete benefit or the
current owner should be retained. Name characterization/error/concurrency and
accountless desktop/mobile checks before selecting any implementation. Keep
Library full-copy/Remove/Forget semantics distinct. No code, protocol, guard,
configuration, activation or trial change is authorized by this proposal.

## Automatic frontend feature-separation result (2026-11-03)

The owner authorized the automatic frontend run, then scoped review, roadmap
synchronization, commit and push to origin/main at
https://github.com/abangkis/AkuSidecar.git. Code `3b9cfd1` is the reviewed result
after baseline 0533ad3 (More/Less code 80acfb9). Source **CL-4.1, CL-4.2 and CL-4.3
are closed within the selected central frontend responsibility scope**. The
publication readback below binds the frozen tested payload and roadmap commit
to the verified remote branch. Original candidate receipts retain their
capture-time sourceHead/sourceDirty/committed fields. Source CL-3 stays closed;
activation, operational acceptance and CL-5 through CL-8 remain separate.

| Dimension | Evidence and scope |
| --- | --- |
| Implemented | `3b9cfd1`: 374 original functions move into 20 named feature factories. app.js falls from 9,937 to 2,225 lines, retaining 30 navigation/shared/transport functions and explicit composition/global wiring. Existing More/Less owner remains intact |
| Tested | Frozen characterization: 29/29 before and after. Node baseline including these tests: 333/333; final regression including 23 actual-factory composition results and four checkout-regression results: 360/360. HTTP/application default: 1,322 pass/25 skip; dev: 1,345 pass/12 skip; zero failures. Four Windows amd64 tag combinations include all new embedded modules and produce 12 Engine/HTTP/application PE artifacts |
| Activated | Not performed. Disposable loopback QA uses its own Store, profiles and owned process tree; no managed service, user database, configuration, source session, provider or capture trial changes |
| Acceptance | Scoped source/test review found and resolved a test-only CRLF parsing defect and staged whitespace defects; final review has no blocking findings. CL-4 source scope is closed against the responsibility map and exit criteria below. Actual embedded desktop/mobile acceptance retains its stated limits; no real-provider/native-capture/clean-install qualification is implied |

| CL-4 boundary | Current responsibility and status |
| --- | --- |
| CL-4.1 Timeline | Rendering, card content, AI actions, media, item actions and context drawer have explicit feature owners; existing feedback/state/policy modules retained. Source separation and scoped tests accepted in 3b9cfd1 |
| CL-4.2 Settings | Draft/render/save behavior stays cohesive in settings-feature.js; storage, provider setup, reset/revocation operations and Auto Update presentation have their own owners. Existing dirty tracker remains the single baseline authority |
| CL-4.3 App entry | Lifecycle/bootstrap/runtime polling and session UI orchestration have named factories. app.js retains initial state, navigation, API/bridgeApi/epoch enforcement, global listeners, observers/queues and existing startup order. No new scheduler, teardown policy or duplicate owner |

### Feature ownership

| New feature owner | Functions | Purpose |
| --- | ---: | --- |
| capture-ui.js | 21 | Bridge readiness/recovery, source access and native-post UI routing |
| app-lifecycle.js | 10 | Bootstrap, activity reporting, runtime polling and database health/cleanup |
| settings-storage-ui.js | 3 | Settings storage summary read/render |
| living-topics-ui.js | 38 | Existing local topic/evidence/snapshot UI operations |
| library-ui.js | 49 | Library/Saved/detail/storage actions and request lifetimes |
| inbox-ui.js | 35 | Inbox review, model usage and capture diagnostic presentation |
| settings-feature.js | 20 | Settings draft, rendering, validation/admission and persistence |
| auto-update-ui.js | 10 | Existing Auto Update status, mode presentation and user actions |
| timeline-rendering.js | 35 | Timeline batches, render scheduling, side pane and scroll geometry |
| onboarding-ui.js | 24 | Onboarding, learning panel and calibration presentation/actions |
| provider-setup-ui.js | 11 | Existing provider readiness/setup/switch UI |
| settings-operations.js | 5 | Reset/revocation/AI Hide confirmation operations |
| diagnostics-ui.js | 4 | Existing timing trace and diagnostic export actions |
| session-ui.js | 29 | Update/session polling, cancellation, cleanup and outcome presentation |
| passive-media-ui.js | 5 | Existing bounded passive X media enrichment presentation flow |
| timeline-content.js | 12 | Card/source/text/attachment/engagement construction |
| timeline-ai-ui.js | 3 | Existing AI detection detail/feedback controls |
| timeline-media.js | 26 | Media construction, playback fallback, carousel and viewer |
| timeline-actions.js | 12 | Read later, semantic correction and existing item actions |
| timeline-context.js | 22 | Context lookup, feedback, drawer, trigger and placement lifecycle |

Factories receive explicit named ports and the same state object. Cross-feature
callbacks and late-bound host objects resolve lazily; no bootstrap snapshot,
generic registry, Proxy or second state authority is introduced in production.
Feature-only Set/WeakMap/primitive state moves with its existing readers/writers.
The existing factory constructors do not call late host ports during composition.
These boundaries organize responsibilities; they are not security/capability
isolation. No pattern is introduced solely to reduce line counts.

Retain the original API/fetch/epoch and source-URL authority, shared notices,
formatters, navigation, root state and event/observer/queue composition in app.js.
Retain existing pure helper modules, X/IG control owners, dirty tracker, screenshot,
frame/idle and recovery queues. Remove 102 unused root import bindings after their
consumers move; retained helper implementations/exports are not deleted. Existing
top-level lifecycle wiring and side-effect statement order are preserved.

### Verification and limits

The declaration review matches every moved function against source 0533ad3,
allowing only indentation/EOL and explicit late getter/setter accesses. It also
checks lexical dependency resolution independently of the extraction scanner,
owned local declarations, module imports/exports, all 30 retained functions and
an exact reconstruction of the composition root. API wrappers, epoch rejection,
original state initialization and global listener/observer/timer ordering remain
unchanged. All 1,434 other tracked files match the captured baseline, including
the already dirty user config; .tmp/ and experiments/ remain excluded.

Existing source/VM contracts now read declarations from their actual feature
files through a test-only source view. Their assertions remain unchanged except
one obsolete unused-app-import location check: its state-owner assertion remains.
The adapter is not production code or an executable fallback. HTTP tests also
fetch every actual new module, check JavaScript content/type and factory wiring.
Twenty-three additional results import/instantiate the actual factories and
exercise late dirty-tracker resolution and stale session-response rejection.
The frozen characterization covers Read later admission/concurrency/error,
Settings save order and AI Hide admission, selective provider baseline updates,
session cleanup retry and bootstrap stale-attempt/polling contracts.

The actual embedded app runs against a disposable real Store/Engine/HTTP server,
with a provider that rejects generation and no scheduler or capture started.
Desktop 1440x1000 and mobile 390x844 exercise bootstrap, More/Less, Read later,
Settings dirty/save, all primary navigation and the Saved Library read. Desktop
also opens/loads/closes the context drawer; mobile preserves the existing hidden
trigger when the narrow gutter cannot fit it. This is not mobile drawer-opening
acceptance. Before/after screenshots were inspected, JavaScript exceptions and
horizontal overflow are absent, and Go-owned process-tree cleanup is verified.
Existing CSS/HTML are unchanged. A fixture assumption about context-trigger
actionability failed on the baseline; its logs/screenshots remain recorded.
The corrected fixture waits for layout, checks the real hidden-panel class and
respects reading-position/gutter eligibility, without changing product behavior.

Relevant Go regression covers HTTP and application packages; Windows compilation
covers default, auth_session_dev, auth_session_fixture and combined tags. This
run does not rerun the whole backend, race detector, signed-in/native-source media,
provider credentials, performance qualification or installed-app acceptance.
Those limits do not reopen source CL-3 or close CL-5 through CL-8.

Evidence: [validation and payload hashes](../runtime/qa/frontend-auto-20261103/validation.json),
[extraction proof](../runtime/qa/frontend-auto-20261103/extraction-proof.json),
[scoped review](../runtime/qa/frontend-auto-20261103/review.json),
[desktop/mobile result](../runtime/qa/frontend-auto-20261103/visual-after/result.json),
and [final candidate readback](../runtime/qa/frontend-auto-20261103/final-readback.json).
These are local ignored QA receipts. sourceHead/sourceDirty/committed identify
capture-time state, not a published commit or running managed binary.


## CL-4 source closure (2026-11-03)

The owner authorized review/publication of the automatic FE result; scoped review
accepts code `3b9cfd1`. No material responsibility gap requires another
extraction in this central frontend cohort. This closes the source gate for
CL-4.1/CL-4.2/CL-4.3, with the following evidence and explicit retain/defer scope.

| Exit criterion | Assessment |
| --- | --- |
| Timeline rendering/actions separate from bootstrap/polling | Met: six Timeline owners plus retained More/Less and existing state/policy helpers; original request, epoch, cache, focus, media and context behavior preserved |
| Settings ownership and dirty/save semantics explicit | Met: cohesive draft/render/admission/save owner, separate provider/storage/reset/automation UI owners, one existing dirty tracker; no new serializer or single-caller wrapper merely to split phases |
| App entry/lifecycle explicit without duplicate ownership | Met: lifecycle/session factories and explicit root wiring; same state/API/epoch owner, same attach/cleanup points, same existing polling cadence and shared pollInFlight barrier |
| Focused feature and rendered desktop/mobile checks | Met in source-only scope: 29 frozen characterization results, 23 real-factory results, four checkout-regression results, final Node 360/360, Go default/dev and four Windows modes; actual embedded default-app desktop/mobile checks pass |

| Retain/defer decision | Return trigger |
| --- | --- |
| Retain app.js shared state, transport/epoch/URL authority, navigation, shared notices/formatters and global event/observer/queue composition | A concrete behavior/lifetime change demonstrates multiple owners or a responsibility conflict; file length alone is insufficient |
| Retain existing helpers, dirty tracker, X/IG controls, screenshot/frame/idle/playback queues and feature-local state | A new supported feature requires a documented ownership change; no generic controller or additional state source is needed now |
| Retain existing mobile narrow-gutter context-trigger behavior and existing bootstrap timer policy | A separately approved UX/lifecycle change defines new behavior and acceptance; this source move does not introduce mobile drawer access or timer deduplication |
| Defer test-asset packaging F02, native operation/CDP/QA ownership, Bridge dispatch and final integration | Respectively CL-7, CL-5, CL-6 and CL-8 entry/consumer/acceptance work; CL-4 closure does not retire supported paths or waive those gates |

Review found that CRLF module footers could make the test-only source reader
include a factory return in a function slice. It now normalizes source inputs;
the added regression reads copies of actual declarations under LF, CRLF and
mixed line endings. The six original failures remain in the local receipt.
Whitespace cleanup preserves complete module ASTs, including string/template
content. These corrections do not change production logic, protocol or guards.
Final checks were repeated after these corrections: Node 360 pass; HTTP/app
default 1,322 pass/25 skip and dev 1,345 pass/12 skip; zero failures; 12 fresh
Windows PE/amd64 artifacts and embedded accountless desktop/mobile pass.

Only the 59 approved code/test/fixture files and four canonical roadmap documents
belong to this publication. The pre-existing config/sidecar.json change, .tmp/
and experiments/ remain excluded. The 374-function declaration/lexical/root
proof is retained and extended by full-module AST equality for formatting; the
review also verifies all 319 named dependency ports across 20 factories.

Evidence: [final validation](../runtime/qa/frontend-auto-review-20261103/current-candidate.json),
[scoped review](../runtime/qa/frontend-auto-review-20261103/review.json),
[formatting equivalence](../runtime/qa/frontend-auto-review-20261103/formatting.json),
and [commit/publication readback](../runtime/qa/frontend-auto-review-20261103/final-readback.json).
These are ignored local receipts; the original implementation receipts remain
unchanged. Published source does not establish managed runtime activation.

Next proposal: read-only CL-5 ownership analysis of operation routing, CDP
framing/projection and owner/source-evidence lifetime in x-native-owner-runtime.mjs.
Identify one real boundary and its characterization needs, or retain the owner
if no gap exists. Preserve deadlines, first cause, cancellation, provenance and
owned cleanup. No CL-5 implementation, restart or live trial is included here.

## CL-5 CDP result projection and live-state binding result (2026-11-04)

The owner approved the CDP-result extraction and active/driverMode correction,
then roadmap synchronization, commit and push to origin/main at
https://github.com/abangkis/AkuSidecar.git. Code `e146d40` follows checkpoint
b3566dc. The reviewed four-file payload is bound to the unchanged passing
evidence and the later publication readback. **CL-5 remains partial**; source
CL-3 and CL-4 remain closed in their recorded scopes.

| Dimension | Current evidence and scope |
| --- | --- |
| Implemented | e146d40: projectCDPResult moves to production shared/x-native-cdp-result-projection.mjs. One factory per runtime supplies live getActive/getDriverMode ports and the same validation helpers; 19 state reads are bound correctly |
| Tested | Final characterization: 24 pass/7 binding failures on original ESM, then 31/31 pass on the candidate. Relevant Node regression: 667/667, zero failures/skips. Seven-package Collection/Engine/HTTP/config/application Go scope: default 2,600 pass/44 skip; dev 3,292 pass/54 skip; zero failures. Four Windows amd64 tag modes produce 16 PE artifacts. Sealed manual/daily/main graphs contain the projector and exclude fixture dependencies |
| Activated | Not performed; no managed runtime restart, production configuration change or signed-in source trial |
| Acceptance | Scoped review has no blocking finding; the frozen payload is committed and publication is bound to the readback below. This closes one CL-5 result-projection increment, not the whole native runtime/QA stage or operational qualification |

### Ownership and behavior

The corrected function validates/projects CDP replies and performs its existing
bounded owner updates. It retains its three-argument signature, branch order,
closed error codes and existing production projection helpers. The runtime
retains the only active/driverMode state; getters are resolved when projecting a
reply, including callbacks, rather than capturing a bootstrap/owner snapshot.
No generic registry, second singleton or additional admission authority exists.

Before correction, the module-level function referenced active/driverMode that
were declared inside createNativeOwnerRuntime. Valid manual inventory could
therefore be rejected as xnp_cdp_result_invalid. The real-runtime regression
reproduces this with mock CDP pipes, without flattening the closure. Related
stateful branches cover instrumentation, body/cookie modes, enrollment and
bootstrap. The same final test payload was replayed against the saved original
ESM using a load hook, preserving its actual lexical scope. One early mock
assertion was corrected to require the strict decoder's null prototype; the
earlier receipts are retained and no parser/guard is relaxed.

The declaration/remainder proof permits only the function move, 19 live getter
substitutions, the named composition and import relocation. send, cdpWrite,
dispatchCDP, pending accounting, first-cause handling, command timeout, owner
admission, cancellation and cleanup bodies remain unchanged. Fixture exports
and production factory admission remain unchanged. The existing package-graph
test adds one assertion that the new production helper is included; none of its
fixture-exclusion or protocol assertions is removed.

### Remaining CL-5 scope

| Responsibility | Current disposition |
| --- | --- |
| Production factory/shared-helper dependency direction | Existing completed increments retained; no production import from scripts/auth-session-fixture |
| CDP result projection | Implemented, tested and reviewed as e146d40; live-state binding defect resolved |
| Framing/decoding and dispatch boundaries | Remain in the runtime; read-only codec ownership review is the next proposal |
| Operation admission and manual/daily/borrowed owner lifecycle | Retain the current owner and guards; further separation needs a concrete lifetime/responsibility gap, not a file-size target |
| Remaining QA dispatch/source-evidence orchestration and packaged consumer closure | Still open under the CL-5 exit criteria; this increment does not retire supported modes or certify every entry point |

The sealed graphs contain 73 manual, 74 daily and 93 main modules, with unchanged
schema/capability contracts and a pinned projector hash. Windows modes are
default, auth_session_dev, auth_session_fixture and combined. The Go headless
package has no go:embed files: the sealed Node manifests establish JavaScript
consumer completeness, while Windows compilation establishes its stated Go
artifact scope. These are separate kinds of evidence.

All 1,495 other previously tracked files match the captured baseline, including
the already dirty config/sidecar.json. Only four code/test files and these four
canonical roadmap documents belong to publication; .tmp/ and experiments/
remain excluded. Publication reuses validated checks after hash verification;
no product suite is rerun merely for commit/push. There is no race-detector,
full-repository, installed-tuple or real-provider acceptance claim.

Evidence: [original validation](../runtime/qa/native-cdp-result-projection-20261104/validation.json),
[structural review](../runtime/qa/native-cdp-result-projection-20261104/review.json),
[candidate readback](../runtime/qa/native-cdp-result-projection-20261104/final-readback.json),
[publication review](../runtime/qa/native-cdp-result-projection-publication-20261104/review.json),
and [commit/publication readback](../runtime/qa/native-cdp-result-projection-publication-20261104/final-readback.json).
These are ignored local receipts. Original sourceHead/sourceDirty/committed
fields preserve capture-time meaning; the later readback binds tested payload,
commits and remote SHA without rewriting historical receipts.

### Next proposed cleanup analysis

The next proposal is a read-only ownership review of CDPFrameDecoder and parseJSONBytes: map framing/decoding, limits, strict JSON/UTF-8 rejection, buffer zeroization and handoff to dispatchCDP. Decide whether one codec owner is justified without duplicating guards; name characterization before selecting a move. No further implementation, runtime activation or trial is included in this publication.

The earlier CL-4 next-proposal paragraph records its original checkpoint and is
superseded by this CL-5 result and the narrower next proposal above.

## Automatic CL-5 native responsibility separation result (2026-11-04)

The owner authorized automatic CL-5 refactoring using the established
characterize-before-move pattern. This run starts at 555dea9 and retains the
published e146d40 live active/driverMode projector fix. It implements the
remaining justified source boundaries, then assesses the retained owner and QA
paths against the CL-5 exit criteria. Source CL-3/CL-4 remain closed.

| Dimension | Current evidence and scope |
| --- | --- |
| Implemented | Reviewed source cohort based on 555dea9 (commit identity bound to publication readback): 47 existing declarations move unchanged to seven production shared modules; runtime composition and supported exports remain intact |
| Tested | Frozen new characterization: 31/31 before relocation; combined new and existing projector characterization: 62/62 after relocation and on final source. Relevant native/QA/package Node regression: 780/780 before and after. Seven-package Go: default 2,600 pass/44 skip; dev 3,292 pass/54 skip; zero failures. Four Windows amd64 tag modes: 16 PE artifacts. Sealed manual/daily/main graphs: 80/81/100 modules |
| Activated | Not performed; no restart, account/profile/config change or live source trial |
| Acceptance | Owner-authorized review has no blocking finding; CL-5 source scope is closed with the retain/defer map below. Commit and remote identity are bound to the publication readback. Activation and operational acceptance remain separate |

### Extracted responsibility map

All files below are under internal/collection/headless/worker/native-pilot/shared.
Declaration counts include constants and private helpers, not just functions.

| Production module | Declarations | Responsibility |
| --- | --- | --- |
| x-native-value-validation.mjs | 7 | Closed value, key, identifier and loopback control-origin checks reused by runtime features |
| x-native-cdp-framing.mjs | 7 | Bounded NUL frames, strict JSON/UTF-8, buffer clearing and decoder limits |
| x-native-network-metadata.mjs | 14 | Closed operation classification, value-free metadata projection and bounded aggregation |
| x-native-cdp-event-projection.mjs | 1 | Opaque CDP event projection before stateful dispatch |
| x-native-observation-policy.mjs | 6 | Fixed X target/readiness policy and per-instance login observation gate |
| x-native-owner-workflow.mjs | 5 | Directory identity, launch envelope and consumed two-phase grant policy |
| x-native-cdp-command-policy.mjs | 7 | Closed CDP vocabulary, existing deadlines and branded failure categories |

The central runtime falls from 1,898 to 1,327 lines and retains 13 top-level
declarations. File size is descriptive, not an exit criterion. Existing public
exports directly reference the same moved constructors/functions, including the
frozen nativeProbeDefinitions fixture port. No fallback shim, generic registry,
second owner or exported decoder singleton is added. Imports resolve to the same
strict parser, launch policy, clock/timing policy and private error-brand module.

### Retain and defer decisions

| Boundary | Decision and concrete return trigger |
| --- | --- |
| send, cdpWrite, dispatchCDP, pending commands, first-cause latch and active/driverMode | Retain inside createNativeOwnerRuntime. These own the same state and synchronous reply/ACK ordering. Revisit only if another real consumer requires an independently owned transport lifecycle |
| Manual/daily operation admission and post reservation | Retain with the owner. Failed init/seed consumption, generation checks, batchPending, fresh grants, retry admission and reservation-before-await remain exact. Revisit if supported lifetimes diverge, not to reduce line count |
| Borrowed main reader versus owned collector | Retain the existing boundary: Go owns main Chrome/profile/process lifetime; the borrower owns collector/bootstrap/attachments and their bounded cleanup. Revisit only with an explicitly approved ownership change |
| Reader/collector evidence, enrollment, provenance and paired checkpoints | Retain role-specific orchestration and shared first-cause/rendezvous. Cookie-write ACK baselines remain synchronous in result projection; observer/enrollment order remains unchanged. Revisit a narrow duplicated policy only when its exact independent contract is demonstrated |
| QA operation dispatch and fixture ingress | Retain sealed operation validators and existing QA adapters. The fixture dispatcher remains outside production graphs. Revisit an actual shared production/QA consumer need; do not export owner authority or flatten runtime closures |
| Live-provider qualification, clock parity, installed tuple and final integration | Separate operational/product evidence and CL-8 where already assigned; no acceptance claim or authorization is inherited from this source run |
| Test-asset/package organization F02, Bridge dispatch and consumer cleanup | Retain existing CL-7, CL-6 and consumer-stage ownership. Their work is not required to manufacture another CL-5 extraction |

The independent read-only owner-lifecycle assessment supports retaining these
closures. A common reader/collector runner would require broad callback/state
ports and would obscure the authority boundary. There is no demonstrated source
gap requiring that abstraction in this cohort.

### Exit criteria assessment

| CL-5 source criterion | Evidence and result |
| --- | --- |
| Correct runtime/QA dependency direction | Production factories and shared helpers remain production-owned; fixture adapters consume native definitions rather than becoming production dependencies |
| No fixture imports in production package graphs | All three sealed manifests exclude scripts/auth-session-fixture and the fixture probe driver, include the seven new modules and existing projector, and bind current source hashes |
| Existing protocol/provenance/negative conformance passes | The 780-result Node cohort includes owner/daily/main protocol, provenance, startup, ingress, seed, reader-post and package tests. Existing guard assertions remain; only graph-inclusion assertions are added |
| Source-pinned manual/daily/main bundles complete | Manifest schema/capabilities and top-level dailyLifecycle/mainLifecycle contracts pass; all module/data-asset hashes and driver identities are verified |

All source criteria are satisfied. The owner authorized source closure after
review; the reviewed cohort closes source CL-5. The remaining decisions are
explicit retain/defer outcomes, not an unbounded file-splitting backlog. Commit
and remote readback identify the exact published payload.

The 47 moved declaration bodies match exactly ignoring EOL; all retained
declarations, including the complete createNativeOwnerRuntime body, are byte
identical. A reconstruction proof permits only import/export composition,
declaration relocation and removal of temporary location comments/extra blank
lines. Final characterization is rerun after that presentation cleanup; the
780-result regression remains applicable to identical execution bodies.

The Go headless package has no embedded JavaScript. Sealed Node graphs establish
JavaScript inclusion; Windows compilation establishes only its Go artifact
scope. No race-detector, whole-repository, installed-tuple or real-provider
acceptance is claimed. All 1,493 other tracked files match the original baseline,
including already dirty config/sidecar.json. Unrelated .tmp/ and experiments/
remain excluded. No configuration, guard, protocol, timeout, cancellation or
cleanup policy is changed.

Evidence: [validation](../runtime/qa/cl5-auto-20261104/validation.json),
[structural proof](../runtime/qa/cl5-auto-20261104/structural-review.json),
[sealed package readback](../runtime/qa/cl5-auto-20261104/packages.json),
[scoped review](../runtime/qa/cl5-auto-20261104/review.json), and
[final candidate readback](../runtime/qa/cl5-auto-20261104/final-readback.json).
These are ignored local receipts, not repository-shipped assets. Earlier
e146d40/555dea9 receipts retain their capture-time source and publication scope.

Publication: [scoped review](../runtime/qa/cl5-publication-20261104/review.json)
and [commit/remote readback](../runtime/qa/cl5-publication-20261104/final-readback.json).
The original candidate receipts are preserved unchanged; later readback binds
that tested code to the committed Git blobs and origin/main SHA. No product
suite is rerun solely for documentation, commit or push.

### Next boundary

Source CL-5 is closed after scoped review. The next proposal is read-only CL-6
background dispatch ownership analysis in AkuBridge: map message routing,
capture dispatch, tab/window ownership, readiness and lifecycle consumers before
choosing one responsibility and its characterization tests. No CL-6 edit,
restart or trial is part of this publication. The prior codec-analysis proposal is superseded by this authorized automatic
run; historical partial-status sections retain their capture-time meaning.

## Automatic CL-6 Bridge reporting separation result (2026-11-04)

The owner authorized automatic CL-6 refactoring using the established
characterize-before-move, verify, review and responsibility-map pattern.
AkuBridge starts at 3676f5c; the canonical Sidecar roadmap starts at aac4967.
The bounded source outcome separates reporting from decisions about resource
release and explicitly retains the existing operation/resource owners.

| Dimension | Current evidence and scope |
| --- | --- |
| Implemented | AkuBridge code 6ce23f7 after 3676f5c: three reporting declarations move to capture-surface-reporting.js with one stateless factory; service-worker wires the existing header/error ports. Eight reviewed Bridge code/test/tooling files are committed; these four canonical Sidecar roadmap documents record source closure, with publication bound to readback |
| Tested | Frozen reporting characterization: 12/12 before extraction; all 12 pass unchanged through the actual factory after extraction. Related Bridge cohort 145/145; final full Bridge Node 515/515; canonical syntax and native-host Go checks pass. Sidecar consumer Node 44/44; seven-package Go default 2,600 pass/44 skip and dev 3,292 pass/54 skip, zero failures. Windows PE/amd64: two Bridge artifacts plus 16 Sidecar artifacts across four tag modes. Three local projected extension packages each contain 70 verified files and pass actual reporter/identity/policy/receipt checks |
| Activated | Not performed; no extension reload, native runtime restart, source/account/config changes or live trial |
| Acceptance | Owner-authorized scoped review has no blocking finding; source CL-6 is closed with the explicit retain/defer map. Publication readback binds commits and both remote SHAs. Activation and operational acceptance remain separate; source CL-3/CL-4/CL-5 retain their published closure |

### Implemented feature and behavior

The new capture-surface-reporting.js owns postCaptureSurfaceEvents,
captureSurfaceEvent and private createCaptureSurfaceEventId. A single
createCaptureSurfaceReporter({ bridgeHeaders, responseError }) factory returns
the two existing caller operations. It owns no Chrome object, lease, queue,
generation, timer, storage record or admission state. Headers are resolved at
request time through the existing bridgeHeaders function, including the current
split-capture instance. There is no header snapshot or second command guard.

Function signatures, input rejection, source fallback, event ordering, policy
identity, outcome limits, timestamp/UUID fallback, payloads, HTTP endpoint,
headers and error behavior remain exact. Callers retain their existing awaits
and best-effort catches. Observation submission precedes successful telemetry
and command completion; original capture failures remain authoritative when
telemetry or failure submission also fails. Release request telemetry precedes
the owned resource release, followed by receipt telemetry; a telemetry failure
cannot prevent release or replace its outcome.

The service worker changes from 2,549 to 2,483 lines. Size is descriptive, not an
exit target. All 112 retained top-level declarations/statements, including the
complete listener and orchestration bodies, are byte-identical. A reconstruction
proof permits only the three declaration moves, import/composition wiring and
blank-line cleanup. Moved declaration bodies are identical ignoring indentation
and EOL. The final 12 behavior assertions are frozen before extraction; the
baseline adapter reads the original declarations, then the final adapter imports
the actual production factory. No baseline fallback remains in the final helper.

### Responsibility map and deliberate retains

| Boundary | Retained owner and reason | Concrete return trigger |
| --- | --- | --- |
| Message ingress, split handlers, event registration and sender checks | service-worker.js composes existing capabilities and preserves synchronous true/false/undefined replies, rejection envelopes and response-before-reload order | A supported second ingress needs exactly the same routing/authorization contract, or a demonstrated listener-order defect requires an independent owner |
| Capture and media-recapture dispatch | One commandGuard and the existing claim/capture/submit/finish ordering; recapture retains its distinct release finally | An independently supported capture caller needs a stable execution lifetime, or a demonstrated ordering defect justifies separation |
| Background config, heartbeat, lease reconciliation, polling and release | One persisted config/poll owner; createSingleFlightSessionPump already owns coalescing. Cleanup receipt completion remains before new pending-command admission | A genuinely independent background lifetime/consumer or an evidenced admission/config race |
| Managed capture tab/window resources | createManagedCaptureWindowRuntime and surface-cleanup-journal retain serialized operations, ledger, exact lease/generation checks and durable pending cleanup | An explicitly approved resource-lifetime change with restart/negative ownership characterization |
| Reader and source windows | Existing reader-window-runtime/source-window-runtime own explicit user actions. Reader windows remain excluded from capture; managed capture windows remain excluded from reader opening | A supported new reader/source-window lifetime or native foreground handshake requirement |
| Navigation, readiness, freshness and prepared capture | Existing policies/adapters plus operation-local activation/focus callbacks. Source parsers remain in adapters; navigation completion is distinct from final capture readiness | A real reusable browser-operation consumer or an approved readiness/focus behavior change |
| Installed-tab recovery, self-reload, native lifecycle and source access | Existing recovery policy, native scheduler/client and local serialized reconciliation queues | A second consumer needs the complete recovery lifecycle or a concrete restart/ordering defect |

An independent read-only Sol assessment found no demonstrated additional owner
conflict or mandatory extraction. The CL-6 criterion requires a justified
responsibility split, preserved contracts and explicit ownership. The remaining
service-worker composition would otherwise become a broad callback map without
a new resource boundary. These are explicit retain decisions rather than an
unbounded file-splitting backlog.

### Characterization, baseline correction and consumers

The original full cohort reported 508 pass/one failure: an old contract test
looked for login_required in content-script.js although readiness decisions
already belong to capture-primitives.js. Once that location was corrected, a
second assertion in the same test exposed the same stale availability context.
Both now check the real owner and the content-script delegation path. A direct
behavior test proves login blocking, including visible candidates, and
availability precedence. Production readiness code is unchanged. Intermediate
failed checks are retained in the local logs, including one corrected matcher
that initially assumed an if statement instead of the existing ternary.

The corrected full pre-move cohort passed 510/510. Five additional dispatch and
release characterizations also passed before extraction and bring the final
full cohort to 515/515. Tests cover invalid/no-op input, payload/fallback order,
input preservation, current policy fingerprint, live headers, UUID fallback,
server/JSON/network errors, successful and failing dispatch, original-error
identity, and release/report ordering with authoritative cleanup outcomes.

The existing admission-diagnostic regression now uses the actual reporter
instead of slicing its old worker location. It retains all admission-before-
cleanup, privacy, byte-budget, source and no-tab-deletion assertions, and checks
the real executing policy fingerprint instead of a stub string. The extension
contract follows the moved endpoint and includes the new file in forbidden-API
coverage. Existing readiness, native lifecycle, sender, reader-exclusion and
ownership assertions remain intact. npm run check adds the new module syntax
check; existing checks are retained.

### Source exit assessment and limits

| Source criterion | Evidence |
| --- | --- |
| Message/manifest compatibility | Retained listener bodies and manifest/capability files remain exact; source contract and Bridge/split regressions pass |
| Asset completeness and extension identity | Static relative imports make the new reporter reachable. Verifier follows 70 files; canonical Browser projection is applied only to local development/acceptance/production-app copies, and the reporter is exercised from each copy with controlled fetch |
| Policy/receipt contract parity | Executing policy fingerprint remains equal to the unchanged Sidecar pin; packaged Bridge cleanup receipt matches its separately packaged Sidecar consumer. Negative cleanup and lease/generation regressions remain |
| Explicit resource ownership | Existing managed journal, reader lifetime, command guard, split epoch, source access and native scheduler remain sole owners; reporting adds no mutable owner or fallback |

The owner-authorized review confirms the source exit criteria; source CL-6 is
closed for code 6ce23f7 and the recorded retain/defer map. Local staged
packages remain verification artifacts rather than installed releases. Node/Go/Windows
checks do not prove loaded-extension identity, clean-machine startup, live
provider behavior, UI focus/quality parity, race-detector or full-repository
acceptance. No visual UI is changed. Source CL-7/CL-8 retain their separate scope.

All 166 other tracked Bridge files, 1,503 non-roadmap Sidecar files and 200
tracked Browser files match the captured baseline, including pre-existing dirty
configuration/build files. Untracked user work is excluded. This automatic run
did not commit, push, activate, reload or run a live source trial. The owner
subsequently authorized reviewed source closure and publication; activation
and live trials remain separate.

Evidence: [validation and structural review](../../AkuBridge/build/cl6-auto-20261104/qa/validation.json),
[declaration/remainder proof](../../AkuBridge/build/cl6-auto-20261104/qa/extraction-proof.json),
[projected package readback](../../AkuBridge/build/cl6-auto-20261104/qa/packages.json),
[scoped acceptance review](../../AkuBridge/build/cl6-auto-20261104/qa/review.json), and
[final candidate readback](../../AkuBridge/build/cl6-auto-20261104/qa/final-readback.json).
These are ignored project-local receipts. Prior CL-5 receipts retain their
capture-time commit/publication identities; no historical receipt is rewritten.

Publication review: [frozen candidate and scoped recheck](../../AkuBridge/build/cl6-publication-20261104/qa/review.json)
and [commit/remote readback](../../AkuBridge/build/cl6-publication-20261104/qa/final-readback.json).
The original implementation receipts retain capture-time candidate fields;
publication readback binds their unchanged tested payload to code 6ce23f7,
the roadmap commit and both origin/main SHAs. Passing product checks are reused
after hash verification, without rerunning suites solely for publication.

### Next boundary

Source CL-6 is closed after scoped review. The next proposed analysis is
read-only CL-7 source lists/package seals/
generated manifest and harness organization, with existing dirty Browser build
changes treated separately. No CL-7 implementation is included. Earlier CL-5
next-proposal text retains its historical meaning and is superseded by this run.

## CL-7 F02 test-asset boundary result (2026-11-04)

The owner approved F02 after the read-only CL-7 analysis, then requested scoped
review, roadmap synchronization, commit and push when no blocking issue remained.
This result is one CL-7 increment. Existing source CL-3/CL-4/CL-5/CL-6 closures
and their historical receipt identities retain their scope.

| Dimension | Current evidence and scope |
| --- | --- |
| Implemented | Reviewed F02 source cohort based on Sidecar 10c1daf: two suites relocate from internal/httpapi/web to test/; only three relative imports change. New assets_boundary_test.go enforces the embedded/static boundary; commit identity is bound to publication readback |
| Tested | Original nine tests pass before/after; full selected Node cohort 369/369 before/after with the same test-name multiset. New Go boundary: six expected failures/ten positive passes before relocation, then 16/16 pass. HTTP default 1,320 pass/23 skip; dev 1,343 pass/10 skip. Four Windows tag modes yield eight PE/amd64 HTTP test/app artifacts; both test-suite byte payloads absent and all 65 production web assets present |
| Activated | Not performed; no runtime restart, live source trial, account/profile/configuration change or installer activation |
| Acceptance | Owner-authorized scoped review has no blocking finding; F02 source change is accepted for commit/publication. CL-7 remains partial; other package/harness boundaries and pre-existing dirty build/cache changes are not closed by F02 |

### Source and harness boundary

| Original embedded location | Retained test location | Preserved cases |
| --- | --- | ---: |
| internal/httpapi/web/collection-mode.test.mjs | test/collection-mode.test.mjs | 5 |
| internal/httpapi/web/instagram-trial-controls.test.mjs | test/instagram-trial-controls-retry.test.mjs | 4 |

Both complete suite payloads match the original bytes except the three import
paths that now resolve through ../internal/httpapi/web/. All assertions,
fixtures, test names and existing test/instagram-trial-controls.test.mjs remain
unchanged. The existing native-reader gate enumerates test/*.test.mjs; it now
includes these nine cases without a new test catalog or harness fallback.

The production go:embed web/* directive, static handler, host/origin checks,
security headers, all production JS/CSS/HTML and remaining web assets are
unchanged. The intentional asset-set change removes the two Node test suites
from the embedded filesystem, so their former GET/HEAD URLs return 404. The
wildcard still discovers production files automatically. No runtime denylist,
protocol revision, weaker guard or test deletion is introduced.

### Boundary regression and review

Before moving files, the new regression reproduced both embedded suites and
four reachable GET/HEAD routes. Production asset/body/header and foreign-host/
origin checks already passed. The same frozen Go test file passes after moving
the suites. It walks the actual embedded filesystem for test/spec filenames,
checks both old routes for 404 and security headers, verifies five served
production assets against embedded bytes, verifies the UI entry, and retains
403 for foreign host, foreign origin and extension-origin UI access.

Windows modes are default, auth_session_dev, auth_session_fixture and combined.
Each produces the HTTP test binary and Sidecar application. Readback checks
PE/amd64, pins artifact hashes, verifies that complete original test payloads
are absent (not merely that their filenames are absent), and finds every one
of the 65 remaining production asset byte sequences in every artifact.

The scoped review revalidates unchanged source, suite/assertion payloads, all
passing logs and artifact identities. It reuses those successful checks for
documentation/commit/push; no product suite is rerun merely for publication.
All 1,505 other tracked Sidecar files match the implementation baseline before
the four authorized roadmap edits. All 174 tracked Bridge files and 200 tracked
Browser files, including pre-existing dirty build/cache work, remain unchanged.
Unrelated Sidecar config/sidecar.json, .tmp/ and experiments/ remain excluded.

This is source/offline verification. The in-process HTTP fixture does not
activate the managed runtime. No loaded-runtime/installer, signed-in source,
race-detector, full-repository or complete CL-7 acceptance is claimed.

Evidence: [original validation](../build/f02-20261104/qa/validation.json),
[move/assertion proof](../build/f02-20261104/qa/move-proof.json),
[publication review](../build/f02-publication-20261104/qa/review.json), and
[commit/remote readback](../build/f02-publication-20261104/qa/final-readback.json).
These are ignored project-local receipts. Original candidate fields retain
capture-time meaning; later readback binds the tested payload, commit and
origin/main SHA without rewriting the implementation evidence.

### Remaining CL-7 scope and next proposal

F02 is implemented, tested and source-reviewed; CL-7 stays partial. Next propose
a read-only comparison of headless-worker staging source filters, its generated
inventory and the manual/daily/main sealed graphs against supported harness
inputs. The result should identify one real gap or an explicit retain decision
before implementation. Existing dirty Browser build/cache files require their
own review; F02 does not adopt, edit, commit or validate that pending work.
Earlier F02-unverified and next-proposal paragraphs describe capture-time
checkpoints and are superseded only for this selected boundary.

## CL-7 worker test-filename boundary result (2026-11-04)

The owner approved the filename exclusion and packaging fixture after read-only
source-filter, inventory, sealed-graph and harness analysis, then authorized
review, roadmap synchronization, commit and push if no blocking issue remained.
The reviewed code is AkuBrowser `8190b45371c940b1e5e461cc8d8cc7958893a687`,
based on `3a3c043`; Sidecar remains at the F02 source `2af9698` for validation.

| Dimension | Current evidence and scope |
| --- | --- |
| Implemented | Only Browser scripts/stage-headless-worker.ps1 and scripts/test-headless-worker-packaging.ps1 change. The existing directory exclusions gain a filename suffix exclusion for *.test.mjs, *.test.cjs and *.test.js. The fixture adds root/nested negative cases, positive native/shared helpers, source-byte checks and exact payload/source-count assertions |
| Tested | Expanded fixture fails on the unchanged stager with worker.test.mjs leaking, then passes after the filter. Existing cold/warm cache regression passes fresh source discovery, corrupt extracted-cache recovery and untrusted archive rejection. Three sealed Node suites pass 16/16. Real staging retains all 111 production source files byte-for-byte, excludes 26 test files (313,806 bytes), and retains all 13 existing shared/x-qa-*.mjs helpers |
| Activated | Not performed; project-local packaging/cache checks only, with the same official pinned archive reacquired as described below. No runtime restart, live source trial, account/profile/configuration change, installer activation or dependency upgrade |
| Acceptance | Scoped review has no blocking finding. The owner-authorized change is accepted for commit/publication; it closes this packaging gap, not all CL-7 or operational acceptance |

### Package and guard readback

| Sealed entry | Modules | Capture assets | Readback |
| --- | ---: | ---: | --- |
| manual | 80 | 26 | All worker dependencies staged; driver and manifest SHA-256 unchanged from the CL-5 receipt |
| daily | 81 | 26 | All worker dependencies staged; driver and manifest SHA-256 unchanged from the CL-5 receipt |
| main | 100 | 32 | All worker dependencies staged; driver and manifest SHA-256 unchanged from the CL-5 receipt |

No test/fixture module enters these sealed graphs. Native helpers keep their
production ownership; a QA-like helper name alone is not grounds for removal.
The schema, capabilities and daily/main lifecycle flags remain unchanged.
Official archive pin/provenance, Node/license hashes, path/reparse checks,
source enumeration on every stage, cache verification and cleanup remain intact.
The generated development inventory still hashes the staged files; its synthetic
prepare-only harness remains a separate promotion/provenance test, not proof of
real source selection. The real stager fixture and actual package readback now
cover this selected source boundary without changing that harness.

The initial sandbox run could not inspect Windows processes, so output retention
failed closed before the fixture assertion. Authorized tests ran outside the
sandbox with that guard intact. The previously available Node archive was no
longer present when cache regression began; the official stager reacquired the
same pinned 24.16.0 archive, verified SHA-256
`edaca9bd58ec8e92037dac4e877d52f6b8f430b81c18b57e264b4e2fb111cd56`,
and the unchanged cache regression then passed with its explicit local path.

### Reproduction and evidence

From AkuBrowser, use PowerShell with normal Windows process-inspection access:

```powershell
pwsh -NoProfile -File scripts/test-headless-worker-packaging.ps1
pwsh -NoProfile -File scripts/stage-headless-worker.ps1 -DestinationDirectory build/cl7-worker-filter/headless-worker -CacheRoot build/cl7-worker-filter/cache
pwsh -NoProfile -File scripts/test-headless-worker-cache.ps1 -ArchivePath build/cl7-worker-filter/cache/24.16.0/node-v24.16.0-win-x64.zip
```

From AkuSidecar, run the sealed graph/protocol regressions:

```text
node --test internal/collection/headless/worker/native-pilot/x-application-owner-factories.test.mjs scripts/build-x-daily-owner.test.mjs scripts/x-native-owner.test.mjs
```

Ignored project-local receipts: Browser
`build/headless-worker-source-filter-20261104/qa/packaging-green.log`,
`cache-regression.log`, `readback.json`, `review.json` and `final-readback.json`;
Sidecar `build/cl7-worker-source-filter-20261104/qa/sealed-regression.log` and
`build/cl7-worker-source-filter-20261104/packages/{manual,daily,main}`.
Source/log/artifact identities are checked for review; passing suites are reused
while those inputs remain unchanged. Earlier Go/HTTP/four-mode Windows evidence
belongs to F02; no new application compile or whole-repository test is claimed.

### Remaining scope and next proposal

Next propose a read-only CL-7 exit-criteria reconciliation across the selected
supported source lists, package/build entry points, generated manifests and
harness commands. Decide retain/defer explicitly and identify any remaining real
consumer gap before proposing more code. F02 and this worker filter are accepted
increments; comprehensive CL-7 source closure is not yet established.

Pre-existing Browser build/cache changes (including the dirty check.ps1), Sidecar
config/.tmp/experiments and all unrelated source remain outside these commits.
Existing cached/generated files are test inputs or disposable receipts, not
source changes adopted by this review. The earlier F02 next proposal is now
historical for this selected boundary. No next implementation, activation or
trial is approved by this documentation update.

## CL-7 source closure (2026-11-04)

The owner authorized an automatic CL-7 run, followed by review, source closure,
roadmap synchronization, commit and push when no blocking issue remained.
The accepted delta is AkuBridge `68753553644203bfa0ed0bdd9cf553e6c5e669e0`,
based on `6ce23f7`; validation also uses Sidecar `fc50f9d` and Browser `8190b45`.
Earlier F02 and worker-filter source identities/evidence retain their scope.

| Dimension | Accepted status and scope |
| --- | --- |
| Implemented | Bridge verifier adds one forbidden filename-suffix branch for referenced *.test.js/mjs/cjs. A new project-local CLI fixture suite adds ten cases for exact inventory/hash output, reference closure, missing files, prior directory exclusion, test suffix rejection through imports/HTML/manifest/source registry, and preservation of test-helper.js/x-qa.js. Runtime, protocol, manifest, version, dependencies and existing guards remain unchanged |
| Tested | Seven initial characterization cases demonstrate four failures before the fix. Final ten cases pass, including after safe fixture cleanup/timeout handling and again during publication review. Canonical Bridge npm test: 525 pass. Sidecar worker/native/package Node: 762 pass, two disposable-Chrome opt-in skips, zero failures. Embedded/static: 16 pass in each default/dev mode. Four Windows tag modes compile app + HTTP test binaries; Browser launcher/installer/signer and Bridge host add four binaries: 12 PE/amd64 artifacts total |
| Activated | Not performed. Only project-local packaging, synthetic harness, metadata and compilation checks; no runtime restart, live source trial, installation, dependency upgrade or account/profile/configuration change |
| Acceptance | Scoped code/test review has no blocking finding. Selected clean source CL-7 is closed with the retained owners and explicit exclusions below. This is source acceptance, not comprehensive CL-8 sign-off, macOS execution/signing, installed/upgrade acceptance or live behavior acceptance |

### Exit criteria and consumer evidence

| Exit criterion | Evidence and limit |
| --- | --- |
| Supported source/build-tag/package entries accounted for | Four primary Go selections retain 31/34/32/34 local packages. 27 auxiliary/tag/platform queries retain four expected fixture-only build exclusions and no unresolved dependency errors. Production entry points remain distinct from explicitly gated QA executables |
| No missing production payload or test leakage in selected packages | Bridge production inventory stays exactly 70 files with fingerprint d0e7e9645ce4d40c820f604ceccd8a2b62e64a35f1a76e240d086fcbad70efee. All five identity projections preserve that file set and all non-projection bytes; four source recipes per profile preserve hashes and resolver identity. The staged worker retains all 111 production sources; tests remain excluded. All 65 production web byte payloads occur in both Windows binaries in each mode |
| Package/production ownership remains explicit | Sealed manual/daily/main roots retain 80/81/100 modules, 26/26/32 assets, immutable generated catalogs/timing and exact previous driver/manifest hashes. QA-like native helper names remain legitimate production dependencies. No fixture-driver/source-checkout fallback is introduced |
| Harness behavior and evidence are reproducible | Existing real stager fixture proves source selection; synthetic bundle harness separately proves promotion, rollback, tamper rejection, service-plan checks and copied PlanOnly/PrepareOnly behavior. Ten new CLI tests run with the ordinary Bridge npm test discovery. Passing broader evidence is reused only after protected source, log and artifact hash checks |

### Retain/defer map and return triggers

| Boundary | Decision | Return trigger |
| --- | --- | --- |
| Sidecar embedded/static web | Retain wildcard embedding and HTTP/security owners; F02 removes only suites | Asset, bootstrap, test location or static exposure changes |
| General worker staging/cache | Retain real pin-checked stager, full non-test source set and all 13 shared/x-qa helpers | Source layout/extension, asset recipe, pin or cache contract changes |
| Sealed native packages | Retain distinct manual/daily/main entries, graph builder, factories and generated recipes | Entry/import/factory or capture/timing recipe changes |
| Bridge inventory and Browser consumers | Retain manifest/registered-source/HTML/import closure, deterministic hashes, shared verification.files and identity projection | Reference syntax or executable asset kind changes, including non-js import roots; package/profile/source registration changes |
| Development bundle harness | Retain separate real source-selection and synthetic lifetime/provenance checks | Bundle entry, provenance schema, staging/promotion/rollback changes |
| Go tag/auxiliary roots | Retain supported product/diagnostic tools and explicit fixture gates | Build tags, entry points, imports, embeds or platform selectors change |
| Frozen Store/macOS/preview tooling P04 | Retain maintenance/recovery lanes still named by release contracts and tests; frozen publication is not retirement evidence | Explicit lane-retirement decision or changes needing platform-native validation |
| Compatibility P01/P02/P03 | Retain launcher rollback, old-file cleanup and signed update formats | Approved replacement/migration with consumer and rollback evidence |
| Pre-existing dirty Browser build/cache P05 | Exclude from this closure and publication; all eight tracked dirty files and five untracked source-cache files retain baseline hashes | Separate owner-authorized review/inclusion of that existing work |

P04's source-retention decision supersedes its earlier unverified retirement
classification only within this reviewed source scope. P05 remains unreviewed
outside the closure. Original CL-1 aggregate counts and old partial/next-proposal
paragraphs remain historical checkpoints; they do not override this decision.

The initial shared cache lacked pinned x/sys v0.19.0 for Browser Windows roots.
The existing Sidecar .go-build/mod cache already contained it; using that cache
with GOPROXY=off and -mod=readonly resolved metadata and compilation without any
installation, upgrade or module-file change. This resolves the earlier offline
dependency limitation for these queries/artifacts, not the excluded cache tools.

### Commands, evidence and limits

From AkuBridge, the current source and new characterization are reproducible with:

```text
node --test --test-isolation=none test/extension-package.test.mjs
npm test
npm run package:verify
```

The Sidecar Node cohort selects worker/native-pilot/*.test.mjs,
worker/test/*.test.mjs and scripts/{build-x-daily-owner,x-native-owner,
x-development-pilot-package,x-daily-development-package}.test.mjs. HTTP boundary
checks select TestEmbeddedAssetsExcludeTestSuites, TestStaticAssetsExcludeTestSuites
and TestStaticProductionAssetsPreserveBodiesAndGuards in default/dev. Windows
compilation uses go build for cmd/akusidecar and go test -c for internal/httpapi
in default, auth_session_dev, auth_session_fixture and combined tags. Auxiliary
builds use the existing launcher, installer, signer and native-host module roots.
Use the canonical source-cache adapter, project-local temporary/output directories
and no inherited live-test consent flags; no runtime launcher is a validation step.

Ignored Sidecar receipts under build/cl7-auto-20261104/: CL7-source-review.md,
qa/final-review.json, qa/publication-review.json, qa/publication-readback.json,
qa/go-validation.json, qa/entry-metadata.json, qa/auxiliary-compilation.json,
qa/package-consumers.json and the Node/synthetic harness logs. Bridge receipts
under build/cl7-auto-20261104/ retain red/green/final/publication characterization,
canonical npm output and before/after/publication inventories. Source/artifact/log
hashes and publication readback bind tested payloads, commits and remote main SHAs.
Receipts may be absent in another checkout; absence is not a new passing result.

The two Chrome cases remain opt-in skips; no live/browser acceptance is claimed.
Darwin metadata is source selection only. The existing Browser dirty builders,
user config, .tmp/experiments, profiles and data are not included or validated by
this closure. No full-repository test or complete installer execution is claimed.

Next proposal: read-only CL-8 integration/sign-off planning. Reconcile the final
candidate ledger and cross-repository validation matrix, decide which prior
receipts remain valid, and identify the minimum remaining integration checks.
CL-8 implementation/testing, cache-tool adoption, activation and trials require
their own scope; this closure does not authorize them.

## CL-8 source closure (2026-11-04)

The owner authorized an automatic CL-8 run, then final review, source closure,
commit and push if no issue remained. Accepted B02 commit:
AkuBridge `32b38348a82292bee8ca77b337bb32ad180a44ec`, based on `810bd99`.
Other validation baselines: Browser `1b96fff`, Sidecar `848ee55`. The change
adds one Bridge test suite and synchronizes three Sidecar ledger/status documents.
No production runtime, transport, protocol, guard or historical release metadata
changes. Publication review/readback binds the tested payload, final commits and
remote main identities; original candidate receipts retain capture-time meaning.

| Dimension | Accepted status and scope |
| --- | --- |
| Implemented | Eighteen tests execute the complete classic loopback script and actual private background caller/helper declarations with synthetic external ports. The production ESM binding and manifest registration are checked; no helper is exported or consolidated. All 32 candidate dispositions are reconciled |
| Tested | B02 18/18 and canonical Bridge npm test 543/543. Existing Sidecar Engine/HTTP negotiation, capability/security bounds and split-heartbeat/rotation fencing tests pass 47 results in default and 47 in dev, zero failures/skips. The current source-candidate v118 gate passes. Broader previously passing evidence is reused after input/receipt/artifact verification |
| Activated | Not performed; no real browser/source/account/profile, installation, service restart, configuration change or dependency upgrade |
| Acceptance | Owner-authorized final source/test/evidence review has no blocking finding and accepts the prior bounded independent review. Selected source CL-8 is closed with the complete 32-row disposition map and explicit exclusions. This does not close operational or release qualification gates |

### Final disposition reconciliation

| Disposition | IDs | Count |
| --- | --- | ---: |
| Removed | G10 | 1 |
| Selected extraction completed | G06, E01, E02, N03, F01, F02 | 6 |
| Supported retain | G01-G05, G08, G09, G11, E03, N01, N04, N05, B01-B06, P01, P02, P04 | 21 |
| Deferred pending replacement/migration | G07, N02, P03 | 3 |
| Explicitly excluded, unreviewed | P05 | 1 |

These are 32 selected contract clusters, not a census of all legacy-looking
symbols or a claim of no out-of-tree consumer. The CL-1 summary remains an
original inventory classification. G07 stays with config/reasoning owners until
config/telemetry consumers and precedence/normalization migrate. N02 stays with
native-source owners until qualified X and generic-source proof/lifecycle parity.
P03 stays with host/update/packaging owners until old signed-update consumers
migrate with compatibility and rollback evidence. Return triggers in the earlier
closure maps remain effective. P05's eight tracked dirty and five untracked
build/cache files remain outside sign-off; separate owner-authorized review is
required before inclusion. Sidecar config/.tmp/experiments are also excluded.

### B02 retain decision

The classic content script and ESM background heartbeat have supported, separately
packaged callers. Numeric protocol 2 preserves the full capability payload. The
existing legacy projection differs: loopback retains focusPolicyRevision while
background removes it along with protocolMajor/protocolMinor/updateCapabilities.
The tests freeze that difference and preserve additive/source-access/surface-policy
fields without mutating the input. This is a retention decision, not proof that
the two shapes are interchangeable or that a legacy regression has been found.
Revisit only for a concrete consumer/legacy-shape migration with exact boundary
coverage; do not create another production abstraction solely to remove a name.

Negative cases retain loopback source/origin/message guards, response origin and
error envelopes. Background checks preserve capture-instance/token/identity/contract
headers, authorization retirement before alarm clearing on 401/403, and independent
delivery failure behavior on 500. All work uses synthetic ports and payloads.

### Cross-repository validation matrix

| Boundary | Evidence retained or added | Reuse reason / limit |
| --- | --- | --- |
| Backend/durable/config | Accepted CL-3 responsibility/characterization and subsequent CL-6 seven-package default/dev receipts; F02 full HTTP default 1,320 and dev 1,343 pass; current 47 default/dev interoperability results added | Production Go sources unchanged; F02-specific embedded/static changes have their own HTTP evidence. No new cross-version installed DB qualification |
| Frontend | CL-4 frozen characterization, Node and actual embedded desktop/mobile receipts; F02 Node 369 and asset boundary 16 default/dev | Production UI bytes unchanged; later changes concern removed test assets and separately tested tooling. No new visual/provider acceptance |
| Native owners | CL-5 native/QA 780 receipts, later CL-7 worker/package 762 pass/2 opt-in skip and exact sealed manual/daily/main manifests | These are different cohorts, not additive unique-test counts. Production graph/payload inputs remain unchanged; disposable-Chrome cases remain opt-in skips |
| Bridge lifecycle/protocol | CL-6 reporting/lifecycle/native-host receipts; current canonical Node 543, B02 18 and Sidecar negotiation/security/fencing 47 default/dev | No runtime source change. Private caller fixture uses real declarations and synthetic ports, not a loaded extension or installed host |
| Build/package consumers | CL-7 four tag graphs, 27 auxiliary/tag/platform metadata queries, 12 Windows PE/amd64 artifacts, five 70-file projections, 20 source recipes, 111 worker sources and cold/warm cache receipts | Artifact and protected input hashes remain valid. No recompilation needed for the new test/documentation. Darwin metadata is not macOS execution/signing |
| Source versus release identity | Published explicit candidate manifest/checker: 44 identity and 14 Bridge contract tests; current candidate gate v118 passes with releaseQualified=false | Historical release authority/contracts/examples remain v116. Default release mismatch is expected and remains enforced; no installer/update qualification or publication rewrite |

### Evidence and final gate

New source: AkuBridge/test/bridge-capability-projection.test.mjs. Bridge ignored
receipts: build/cl8-auto-20261104/{b02-characterization.log,npm-test.log,
package-inventory.json}. Sidecar ignored receipts: build/cl8-auto-20261104/qa/
{before.json,interop-default.jsonl,interop-dev.jsonl,source-candidate.json,
validation.json,publication-review.json,publication-readback.json} and the source
review report. Earlier CL-7 and source-candidate
receipts retain capture-time scope and source/artifact identities. A missing
local receipt in another checkout is not a new passing result.

Final review confirms the B02 test, ledger totals, source/receipt/artifact hash
bindings, explicit deferral triggers and exclusions. No additional mandatory
production extraction or broad test/rebuild is identified. A new failure or
changed input reopens only the affected row. Earlier candidate/pending-review
statements are historical within this selected source scope.

At the CL-8 closure checkpoint, the next cleanup proposal was read-only P05 ownership/diff review of the 13 pre-existing
dirty Browser build/cache files. Map cache paths, producer/consumer boundaries,
retention/cleanup guards and necessary tests before any inclusion proposal.
This source sign-off does not approve those changes or cache deletion. Release
preparation, activation, installer/macOS acceptance and live X/IG qualification
also remain separate future work, not conditions asserted as passed here.

## P05 source-build cache publication (2026-11-04)

The owner separately authorized P05 ownership/diff review, bounded ownership
tests, narrow ignore rules, the relative npm cache fix and coherent package
publication after review of its test dependencies. The original
13-file Browser package is now published. This supersedes current descriptions
of that package as dirty/unreviewed; earlier CL-7/CL-8 exclusions and their
32-row disposition snapshots retain their capture-time meaning.

| Dimension | Current status and evidence |
| --- | --- |
| Implemented | Browser `8c39d979292232884a431d04d240cbc3f0c91495` adds the Node/PowerShell cache helpers, documentation and suites, updates five Windows and two macOS wrappers, and scopes cache/temp ignores: 13 files. Relative npm overrides remain relative and are rebased for the Bridge command directory, preserving the caller-selected cache location and override precedence. Supporting Sidecar `ff65e9e6e392a05ed065a61ed183132f3b42b3f4` adds only root `/.tmp/` |
| Tested | Final Node suite: 5/5, zero failures/skips, both in the working tree and in the exact LF-normalized staged payload. The exact staged PowerShell suite passes, including owned-temp cleanup, caller-owned preservation and out-of-root preservation with `OwnsTempDirectory=true`; both staged shell wrappers separately pass syntax checks. Actual npm config queries pass three cases: a relative path with spaces, a cache at the command directory and an existing absolute override. Removing npm rebasing only in an isolated wrapper copy produces the intended location assertion failure. Extra CLI arguments/unknown cache names remain rejected. Fourteen ignore-boundary checks retain visibility outside their narrow roots; earlier ownership mutation receipts retain their stated scope |
| Published | Browser commit `8c39d97` was pushed to `Origin/main`; Sidecar `ff65e9e` was pushed to `origin/main`. Both remote main SHAs were rechecked on 2026-11-04 (Asia/Jakarta) and match the full commit IDs above. Committed Browser blobs match the recorded staged SHA-256 inventory. Publication is source availability, not runtime activation or a release |
| Acceptance | The reviewed cache increment is accepted within its recorded source/test scope. Full P05 closure is not declared. Native macOS package execution/signing, Windows installer qualification and broader operational/release acceptance were not performed. Existing CL source closures and deferred migrations are unchanged |

### Ownership and preserved boundaries

- Helpers select shared compiler/package caches; the wrappers own temporary
  process environment changes and restore them on exit. Shared caches are not
  treated as disposable per-run output or deleted by temporary cleanup.
- Generated Go scratch directories remain project-local. Caller-provided
  `GOTMPDIR` remains caller-owned; the PowerShell cleanup retains containment,
  generated-name and reparse-point checks. The macOS staging owner starts false,
  claims only after exclusive creation and releases ownership after promotion.
- The new Browser rule scopes `SharedTemp/build-cache/` to the repository root;
  it adds no exclusions for other `SharedTemp` paths. The Sidecar change adds
  only a root `.tmp/` rule. Existing ignore rules continue to apply.
  The two previously authorized obsolete test logs were removed separately;
  this documentation synchronization deletes nothing.
- The initializer assertion is an explicit presence/order check. The older
  suite also rejected the missing-initializer mutation through its ordering
  assertion; this is not recorded as a demonstrated full-suite false-positive fix.
- No runtime restart, source/account/profile/configuration change or live trial
  is included. The retained local Sidecar config and experiments were not committed.

### Evidence identity and reuse

Local ignored Browser receipts under `build/p05-relative-npm-20261104/`:
[final publication readback](../../AkuBrowser/build/p05-relative-npm-20261104/publication-readback.json),
[staged payload inventory](../../AkuBrowser/build/p05-relative-npm-20261104/staged-payload.json),
[exact staged Node log](../../AkuBrowser/build/p05-relative-npm-20261104/staged-node-suite.log),
[exact staged PowerShell log](../../AkuBrowser/build/p05-relative-npm-20261104/staged-powershell.log),
[npm integration](../../AkuBrowser/build/p05-relative-npm-20261104/npm-integration.json), and
[isolated negative proof](../../AkuBrowser/build/p05-relative-npm-20261104/without-rebase.json).
Earlier `p05-ownership-initializer-20261104/qa/validation.json` and
`p05-out-of-root-regression-20261104/validation.json` preserve their original
input identities. Git normalized CRLF to LF when staging; the final suites ran
against those exact staged bytes before commit. The pre-publication
`validation.json` has a historical pending field; `publication-readback.json`
records the completed push and supersedes that field for publication status.

This synchronization verifies receipt/committed-blob identities and remote SHAs;
it does not rerun product tests, compile applications, run installers or trials,
or activate a runtime. Publication of this roadmap update is tracked separately
from the verified source commits above.

### Remaining scope and next proposal

At this cache-publication checkpoint, two inherited macOS limits were recorded:
system-temp packaging scratch and Step 3B's early `$unpacked_root` use. The latter
is now corrected in the separate result below. The later scratch-maintenance
record relocates the runtime installer's packaging scratch to project-local.
Native Mac qualification remains deferred under the owner's freeze; syntax and
extracted-operation tests do not establish successful native packaging.

That packaging scratch analysis was subsequently approved and implemented.
The active next Windows proposal is recorded in the freeze decision below.
No further implementation, cleanup deletion, activation, commit/push or trial
is authorized by this documentation update.

## P05 Step 3B projection order (2026-11-04)

| Dimension | Current status and evidence |
| --- | --- |
| Implemented | Browser `5fa73c9703f881495aab345e97749956dfd58dca` moves the existing identity-projection invocation after the verified-file copy and before fingerprinting. No invocation arguments, package format, helper bodies or staging/cleanup guards change. New `scripts/macos-3b-package-order.test.mjs` owns four regression cases; two Browser files are committed |
| Tested | All four new cases fail on the original placement. Final combined cohort passes 9/9 with zero failures/skips: four Step 3B cases plus the existing five cache/ownership cases. The cases check unset/inherited target handling, projected manifest/deployment bytes and fingerprint, prior-staging preservation and owned cleanup on projection failure. Shell syntax and whitespace checks pass. Source/helper hashes match the receipts; staged test bytes match the tested file, and the staged shell matches the LF-normalized source executed by the fixture |
| Published | Browser `5fa73c9` was pushed to `Origin/main`; remote SHA was verified as `5fa73c9703f881495aab345e97749956dfd58dca` on 2026-11-04 (Asia/Jakarta). Sidecar's earlier local roadmap commit `7a533e1` and this synchronization are published separately; final readback binds the resulting documentation commit and remote main |
| Acceptance | Scoped source/test review finds no blocking issue and accepts this correction. The early/unset target and copy/projection/fingerprint ordering gap is closed in source. Full P05 closure, native macOS package execution/signing, Windows installer qualification and operational/release acceptance are not established. No activation or live trial is included |

The required sequence is containment/staging guard, exclusive creation and
ownership claim, verified-file copy, acceptance identity projection, then
fingerprint. The projection helper reads and writes the copied manifest and
deployment file. Merely initializing the variable earlier would not provide
those files and could let subsequent copy overwrite projected bytes.

The test executes the production package segment, copy logic, projection and
fingerprint helpers and EXIT cleanup in isolated project-local directories.
The verifier inventory is synthetic; Windows has a path-format adapter for the
normalization port. The production containment guard still executes. The full
Darwin/signing/source-tuple preflight and archive/installer workflow are not run
or weakened. Passing this fixture is not native macOS acceptance.

Ignored Browser receipts in `build/p05-step3b-order-20261104/`:
[original validation](../../AkuBrowser/build/p05-step3b-order-20261104/validation.json),
[four expected failures](../../AkuBrowser/build/p05-step3b-order-20261104/red.log),
[nine passing results](../../AkuBrowser/build/p05-step3b-order-20261104/green.log), and
[publication readback](../../AkuBrowser/build/p05-step3b-order-20261104/publication-readback.json).
Original `committed=false`/`pushed=false` fields describe pre-publication capture;
the readback supersedes them for publication. The existing product checks are
reused after source/hash verification; a documentation commit does not require
another full test or build.

At this checkpoint, the next proposal was read-only packaging scratch analysis.
The owner later approved the one-line relocation recorded below, then froze
further Mac work and selected Windows-only focus. This earlier proposal is
historical and is no longer the active next task.

## P05 completed scratch maintenance and Mac freeze (2026-11-04)

| Dimension | Current status and evidence |
| --- | --- |
| Implemented | Browser `54a1b49535be4f3a7c84b75af98611d67b7bc8f5` changes only the packaging mktemp template parent to `$source_build_temp_root`, retaining `akubrowser-macos-installer.XXXXXX`, trap, guards and cleanup. `scripts/macos-runtime-scratch.test.mjs` adds six cases. Two source/test files are committed |
| Tested | Existing evidence passes 15/15 without skips: six scratch, four Step 3B and five cache cases. Before the production change, four location assertions fail and two failure-path cases pass after the fixture's Windows drive-letter adapter correction. Success/error cleanup, caller GOTMPDIR, cache/sibling/output preservation and allocation failure are covered. Source/test hashes and normalized staged payload match those receipts. No Mac tests, signing or installer workflow are rerun during publication |
| Published | Browser 54a1b49 pushed to Origin/main; remote SHA verified as 54a1b49535be4f3a7c84b75af98611d67b7bc8f5 on 2026-11-04. The supporting roadmap publication is separately bound by the final readback |
| Acceptance | The completed maintenance increment is accepted within its source/test scope. The owner freezes further Mac refactoring, packaging/signing/install qualification and live Mac validation until explicit unfreeze. Native Mac acceptance remains unproven/deferred and does not block Windows-focused cleanup. Full P05 or release/operational acceptance is not declared |

Evidence: Browser-local ignored
[validation](../../AkuBrowser/build/p05-macos-scratch-20261104/validation.json),
[corrected pre-fix run](../../AkuBrowser/build/p05-macos-scratch-20261104/red.log),
[15 passing results](../../AkuBrowser/build/p05-macos-scratch-20261104/green.log), and
[publication readback](../../AkuBrowser/build/p05-macos-scratch-20261104/publication-readback.json).
Initial fixture-only drive-case failures are retained in the original log and
are not represented as production regressions. Pre-publication false commit/push
fields retain capture-time meaning; the readback establishes publication.

The fixture executes the actual initializer, allocation and EXIT cleanup with
synthetic work/failure points. It is not a native installer/signing run. Source
availability and a maintenance fix do not unfreeze the platform. Release metadata,
supported Mac recovery paths and all protocol/guard contracts remain unchanged.

At this freeze checkpoint, the next minimum proposal was read-only lifecycle analysis of the active Windows
`scripts/build-windows-installed-app.ps1` cache/temp wrapper, starting with failure
at the runtime-identity gate. Select one integration characterization that verifies
the actual wrapper restores environment and removes only owned scratch. Existing
PowerShell evidence exercises the helper; wrapper checks in that suite are parse/
source assertions, and the installed-app artifact validator inspects outputs.
This is a scoped coverage question, not a claim that every Windows test is absent.
Do not change the identity gate or release metadata. No Windows implementation,
runtime restart, installer execution or trial is authorized by this record.

## P05 Windows installed-app cache lifecycle characterization (2026-11-04)

The owner subsequently authorized the failure-path characterization, reviewed
the result and approved roadmap synchronization, commit and push. This closes
the selected wrapper-execution coverage gap without changing production code.

| Dimension | Current status and evidence |
| --- | --- |
| Implemented | Browser `16a6ff2d0c1d3f8a0db6a0cb31d42810dd72ac4a` adds only `scripts/test-windows-installed-app-cache-lifecycle.ps1`. Each fixture executes a complete byte-copied production builder and helper in an isolated child PowerShell process. No protocol, identity gate, guard, timeout, cleanup or release metadata changes |
| Tested | Two real-wrapper cases pass: generated owned scratch and caller-provided GOTMPDIR/cache overrides. Two isolated copies with the wrapper cleanup call disabled fail at the expected environment-restoration assertion. All eight environment variables are checked after catch in the same child process, before harness teardown. Owned scratch is removed; caller scratch, cache sentinels, output and a pre-existing scratch sibling are preserved |
| Published | Browser 16a6ff2 pushed to Origin/main; remote SHA verified as 16a6ff2d0c1d3f8a0db6a0cb31d42810dd72ac4a on 2026-11-04. The local publication receipt below records that readback and separately binds the documentation commit/remote identity after publication |
| Acceptance | Scoped test/source review has no blocking finding. This proves the actual wrapper's cleanup after a controlled failure at the first runtime-identity Node port. It does not execute that checker, prove a successful full build or qualify the installer. Full P05 and operational/release acceptance remain open; Mac stays frozen. No activation or live trial |

Canonical command from AkuBrowser:
`pwsh -NoProfile -File scripts/test-windows-installed-app-cache-lifecycle.ps1`.
The Node port verifies its exact arguments and returns controlled exit 17; the
wrapper retains its exact error contract. Unexpected Node, Go, Git and npm
execution is rejected. The two mutations affect fixture copies only; the real
builder and helper remain unchanged. Fixture paths are project-local, and
harness deletion is restricted to its verified immediate GUID child.

Ignored local Browser receipts:
[validation](../../AkuBrowser/build/p05-windows-installed-app-cache-20261104/validation.json),
[two passes and two expected failures](../../AkuBrowser/build/p05-windows-installed-app-cache-20261104/integration.log),
and [publication readback](../../AkuBrowser/build/p05-windows-installed-app-cache-20261104/publication-readback.json).
They may be absent in other checkouts. The suite SHA-256 is
`fe4c5c3dd76b372f6bc96b421cee58a58e20a59d719e30d457d99588a82f4222`;
the exact staged test bytes match that successful run. Builder/helper hashes
also match the validation receipt. Testing is reused after identity checks;
publication alone does not require another test/build. The validation's
`committed=false` and `pushed=false` fields retain capture-time meaning;
publication readback supersedes those fields for publication status.

Next minimum proposal, requiring separate approval: read-only comparison of
the remaining active Windows cache/temp wrappers against their execution tests.
Choose one concrete uncovered failure path, or retain the existing coverage
if sufficient. Do not infer a production defect or start implementation, a full
build, installer, activation or trial. Preserve the Mac freeze and unrelated
Sidecar config/experiments.
