# Comprehensive code cleanup roadmap

Date: 2026-11-01 (Asia/Jakarta).
Status synchronized: 2026-11-03 (Asia/Jakarta), through Sidecar `ce57e79`.
Status: execution in progress; each bounded increment requires separate approval.
No implementation, deletion, dependency upgrade, activation or trial is approved
by creating this roadmap. Approve one concrete increment, implement it, verify
it and review its result before approving another increment.

## Objective and scope

Make the AkuSidecar, AkuBridge and AkuBrowser source easier to change and verify:
remove proven obsolete code, separate production from QA, give each module a
clear responsibility, and preserve supported behavior and durable data.

Scope covers tracked product source, build/package entry points, tests, fixtures,
contracts and developer tooling in these three repositories. Inspect generated
bundles and manifests as consumers without treating generated output as source.
The existing dirty Browser build/cache changes are a separate work item; inspect
their ownership before any proposed inclusion. Untracked experiments, profiles,
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
| CL-1: consumer and compatibility inventory | [Entry/consumer map](cl1-consumer-inventory.md) and [32-boundary candidate ledger](cl1-cleanup-candidates.md) delivered | Read-only four-tag metadata, native/package/reference and source-preservation checks; no product test/build | Not applicable to an audit | Ready for review; external-consumer and offline Browser dependency limits explicit; no implementation approved |
| CL-2: proven obsolete-code retirement | Earlier wrappers plus G10 `DeleteMemoryItem` removal committed with roadmap as `21ac9ad` | G10 exact remainder check, full Store/HTTP regressions and four-mode Windows compilation passed | Not performed for G10 | G10 reviewed/committed; comprehensive retirement remains scoped by the remaining ledger entries |
| CL-3: backend responsibility boundaries | G06 `d82f53b`; candidate cleanup `7d00d9c`; Settings file move `02e063e`; profile merge `aea3867`; executable preparation `c303ed4`; diagnostics export `1068e35`; Bridge status `7b2d0d8`; HTTP Bridge commands `7cb712b` and media recapture `be9fcd2`; Engine media recapture `c614512`; Engine Bridge commands `3e13434`; Engine media provenance `8d7f7b6`; Engine AI detection `b84a62d`; Engine session admission `4bf0731`; observation contracts `d9c3025`; process recovery `5b39fa4`; reasoning result `eb05ac8`; capture rounds and deterministic Store test `683b1e8`; local acquisition policy `f16f11a`; vision-lane routing/coverage `25c3339`; selection corrections `f3d46b8`; AI feedback `6a1ce33`; FullReset `da019dd`; Library removal/Forget `a19bf86`; passive media evidence `2bdc0f1`; HTTP update/session `d32c065`; HTTP media entry `90e3a7f`; HTTP bootstrap `97bbe51`; capture-owner path guard `b482ba2`; HTTP reload actions `6ea303c`; telemetry rotation/shutdown guard `237dbd1`; HTTP development X controls `1814c9b`; HTTP semantic corrections `3c5e61a`; HTTP AI feedback `a3718ff`; HTTP Inbox selection review `3f4a66d`; HTTP Inbox reads `ce57e79`; validation detours `edb246d`/`8dc3f5e`; earlier lifecycle/session seams retained | Latest combined Capture/Collection/Bridge builder/Selection/EventEngine/Reasoning/Domain/Detector/Inspector/Store/Engine/HTTP/config/application scope selects 14 packages (13 with tests, Bridge builder compile-only): default 2,849 pass/50 skip and dev 3,693 pass/62 skip, zero failures; Inbox read characterization 70 default/dev before/after; selection-review characterization retained; AI-feedback characterization retained; semantic correction characterization retained; development X characterization retained; earlier telemetry/reload characterization and concurrency evidence retained; earlier guard/bootstrap receipts retained, four HTTP build-tag graphs and 16 Windows artifacts pass. Earlier acquisition-policy/capture-round/privacy/HTTP/Bridge status/diagnostics/Node/cancellation/profile receipts retain their own scope | Not performed for these increments | Scoped changes reviewed/committed; Engine/HTTP media-recapture, Engine/HTTP command, media-provenance/AI-detection, session admission, observation contracts, process recovery, reasoning-result acceptance/Timeline mapping, capture rounds, local acquisition policy, vision-lane routing/coverage, selection corrections, AI feedback, FullReset, Library removal/Forget, passive media evidence, HTTP update/session, HTTP media entry, HTTP bootstrap/router map, HTTP reload actions, development X controls, semantic corrections, HTTP AI feedback, HTTP Inbox selection review, HTTP Inbox reads, diagnostics/Bridge status and E01 boundaries established; further Engine/HTTP boundaries pending. Provider-selection micro-extraction declined; keep Settings cohesive; CL-3 remains partial |
| CL-4: frontend feature ownership | Existing navigation module retained; broader extraction proposed | Existing baseline only | Not recorded | Timeline/Settings/app orchestration pending |
| CL-5: native runtime and QA boundaries | Factory/shared-helper separation complete; fixture-only timing correction `8dc3f5e` committed; deeper runtime split proposed | 98 Node tests, 200 offline integration iterations and dependency-pinned staging verified for this fixture correction; existing baseline only for remaining seams | Not performed for this correction | Bounded fixture acceptance only; remaining operation/transport/QA separation pending; no universal cross-runtime clock or real-provider qualification claim |
| CL-6: Bridge responsibility boundaries | Existing source/lifecycle modules retained; further split proposed | Existing baseline only for remaining seams | No new rollout | Background dispatch/surface integration pending |
| CL-7: build, packaging and test organization | Existing packaging retained; full consumer reconciliation proposed | Partial prior package evidence | No new rollout | Supported-entry/build-tag closure pending |
| CL-8: integration and source sign-off | Not started for the comprehensive roadmap | Final matrix pending | Separate operational gate | Comprehensive source acceptance pending; runtime acceptance separate |

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
  Remaining reasoning/acquisition, enrichment and read-model boundaries still need
  evidence. Keep the Engine facade and existing Store authority; do not repeat
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
  Eleven scoped HTTP boundaries are established, including development X controls
  `1814c9b`, semantic corrections `3c5e61a`, AI feedback `a3718ff` and Inbox
  selection review `3f4a66d` and Inbox reads `ce57e79`, with unchanged
  characterization, matching regression and four-mode Windows compile; CL-3.3
  remains partial.
  Capture-owner path normalization is corrected separately as `b482ba2`,
  with default/dev regression and four-mode Windows compile; the guard fix itself
  adds no feature extraction. Telemetry rotation/shutdown compatibility follows
  as `237dbd1`, with controlled concurrency, matching regression and four-mode
  Windows compile; this is another correctness increment, not an extraction.
  Keep infrastructure/shared helpers and deferred groups pending separate review.

Exit: a reviewed responsibility map and separately validated extractions, with
no second source of state, broader admission, new concurrency or wire changes.

### CL-4: frontend feature ownership

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

- Map background messaging, capture dispatch, tab/window ownership, readiness
  and lifecycle consumers in `service-worker.js` and related modules.
- Extract one responsibility with its message/event wiring and tests. Keep
  source parsers in adapters and durable decisions in Sidecar/Store.
- Preserve managed-window journals, exact lease/generation checks, reader
  lifetime, extension identity and separately packaged receipt parity.

Exit: message/manifest/asset compatibility and contract-parity checks pass;
resource ownership is explicit without additional singleton state or fallback.

### CL-7: build, packaging and test organization

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

## Immediate next proposal

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

### Proposed next cleanup analysis

Read-only review of the two model-usage read adapters: GET
/api/inbox/sessions/{sessionId}/model-usage and GET /api/model-usage. Trace callers,
session ID parsing, windowDays default/clamp/allowlist order, output projection
and error precedence before deciding whether one cohesive feature extraction is
useful. Specify characterization first; preserve receiver, request context,
Engine/Store ownership, protocol and guards. This proposal authorizes no further
implementation, activation, runtime/configuration change or trial.
