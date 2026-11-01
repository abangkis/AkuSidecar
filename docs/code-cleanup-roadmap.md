# Comprehensive code cleanup roadmap

Date: 2026-11-01 (Asia/Jakarta).
Status: proposed execution roadmap; documentation created at the user's request.
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
| CL-3: backend responsibility boundaries | G06 `d82f53b`; candidate cleanup `7d00d9c`; Settings file move `02e063e`; profile merge `aea3867`; executable preparation `c303ed4`; diagnostics export `1068e35`; validation detours `edb246d`/`8dc3f5e`; earlier lifecycle/session seams retained | Latest combined Engine/HTTP/config/application suites: default 585 pass/38 skip, dev 1,258 pass/48 skip, zero failures; 17 characterization results before/after diagnostics move, four build-tag graphs and eight Windows artifacts pass. Earlier Node/cancellation stress and profile contracts retain their own receipt scope | Not performed for these increments | Scoped changes reviewed/committed; diagnostics feature and E01 boundaries established; further Engine/HTTP boundaries pending. Provider-selection micro-extraction declined; keep Settings cohesive; CL-3 remains partial |
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
  in `diagnostics_export.go` as `1068e35`, with characterization and passing
  combined regressions/Windows compilation. Next analyze the coherent Bridge
  status/compatibility feature read-only. Remaining command adaptation, observation
  acceptance, source admission, enrichment and read-model boundaries still need
  evidence. Keep the Engine facade and existing Store authority; do not repeat
  completed lifecycle/session moves or split small blocks merely to reduce size.
- **CL-3.3 HTTP capture:** separate request/response adaptation from application
  orchestration where a complete use case is identifiable. Preserve endpoint
  shapes, authentication/admission, status codes, error categories and call order.

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

The next proposal is read-only analysis of Bridge status/compatibility in
`engine.go`: assess one module for status projection and existing protocol,
capability and source-readiness predicates. Leave heartbeat mutation, authority
revocation, command recovery, reload/scheduler effects and ownership with Engine.
Map callers and existing tests, preserve `e.mu`, legacy negotiation and every
origin/source/capability guard, and identify missing characterization before
implementation approval. No further refactor, protocol change or compatibility
removal is approved by this status update.

No calendar estimate or cleanup percentage is assigned: this bounded inventory
does not establish a complete repository-wide denominator or close every
supported-consumer obligation.
