# CL-1: cleanup candidate ledger

Date: 2026-11-01. Source and method: [consumer inventory](cl1-consumer-inventory.md).
Scope: CL-1 candidate classification, followed by separately approved G10, G06,
provider-candidate cleanup, E01 file/profile-merge/executable-preparation
extractions, the cancellation/Node-fixture validation detours and the separately
approved CL-3.2 diagnostics/Bridge status/Engine media-recapture/Bridge command/media-provenance/AI-detection/session-admission and CL-3.3 HTTP Bridge command/media-recapture features below.
Other candidates remain proposals or retained contracts.

## Dispositions and approval

| Disposition | Count | Meaning |
| --- | ---: | --- |
| `remove` | 1 | G10 removal separately approved, tested and committed; see its CL-2 record |
| `extract` | 5 | Responsibility seam worth a separately approved move/split |
| `retain-required` | 19 | Supported caller, data, build or packaging obligation demonstrated |
| `defer-until-replacement` | 3 | Consumer/authority migration must precede retirement |
| `unverified` | 4 | Evidence or contract decision still needed |

**Approval at the CL-1 checkpoint was inventory only.** G10, G06, candidate cleanup,
the narrow E01 file move, profile merge, executable preparation and separately
approved cancellation/Node-fixture corrections were
subsequently approved and applied. Further E01 phase decomposition and
implementation of the other rows remain unapproved. Retained
rows are not deletion candidates. No global absence of out-of-tree consumers
is claimed. The 32 rows are reviewed contract clusters, not a count of unique
files or a verdict on every keyword hit. The 246-file mechanical keyword queue
includes tests; 101 files are not named as tests/fixtures, which does not prove
they are production entry points. Residual keyword hits remain `unverified` in
the local census until a concrete change requires their semantic resolution.

Common rollback rule: no runtime/schema/configuration changes are authorized.
A source revert cannot undo data migration, released wire behavior or a native
resource side effect. Each future patch must preserve the row's contract and
update source pins/staging if its actual consumer boundary changes.

## Go, durable data and configuration

Paths in this section are relative to AkuSidecar.

| ID | Disposition | Target and role | Supported consumers / evidence | Checks and rollback constraint before any change |
| --- | --- | --- | --- | --- |
| G01 | `retain-required` | `internal/store/migration_registry.go`, `compatibility.go`, schema migration chain | Store Open and database inspection use the complete 7-to-31 registry; compatibility tests cover chain completeness and original preservation | Preserve all supported starting versions, transactional failures and unsupported/newer rejection; no schema-marker downgrade |
| G02 | `retain-required` | `source_daily_migration.go`, semantic 30-to-31 fence | Older X-only guards must not open newer source-scoped authority; metadata-only does not mean redundant | Preserve exact source version and one-row update; Git rollback is not permission to lower schema |
| G03 | `retain-required` | `native_capture_migration.go`, 25-to-26 table rebuild/absent-table case | Existing rows, indexes/triggers and canonical schema creation after migration; `native_capture_migration_test.go` | Retain absent-table behavior and rollback on rebuild failure; no unconditional drop/create |
| G04 | `retain-required` | `runtime_version.go`, `.runtime-version` marker | Store Open and compatibility inspection reject newer-runtime data even at equal schema; runtime-version tests | Preserve version/prerelease comparison and marker checks; equal schema alone does not establish downgrade safety |
| G05 | `retain-required` | `account_session_policy.go`, `account_checkpoint_guard.go`, historical durable keys | `synthetic_account_checkpoint_policy` and related records deliberately fence older guard-aware binaries; staged compatibility tests | Never rename/remove keys during opt-out; ordinary skipped harnesses are not cross-version passing evidence |
| G06 | `extract` | `reasoning_config_decode.go:ReasoningConfig.UnmarshalJSON`, relocated from `config.go` | Config Load and all main variants retain modern/flat shapes; both shape tests pass | Reviewed and committed with the smoke-test context correction as `d82f53b`; default/dev regressions and four-mode Windows compilation pass; original baseline failure preserved as historical evidence below |
| G07 | `defer-until-replacement` | `ModelConfig` model/effort aliases and display projections | Config validation, reasoning calibration/profiles and Codex binding still consume them | Inventory installed config/telemetry consumers and migrate precedence/normalization explicitly; no alias deletion inside a file move |
| G08 | `retain-required` | Disabled Windows/development stubs; candidate-probe schemas 1/2 | Shared main symbols need `!windows || !auth_session_dev` implementations; main tests protect exact schema-1 probe shape | Preserve build constraints and unavailable-mode behavior; schema-2 DB metadata cannot be added to schema-1 replies |
| G09 | `retain-required` | `engine.go:bridgeProtocol`, legacy v2 heartbeat inference | `RecordHeartbeat`/`BridgeStatus`; `TestBridgeProtocolNegotiationPreservesLegacyV2Heartbeat` | Preserve valid-v2-contract inference to 2.0, degraded warning and bounded negotiation; distinct from Native Messaging v1 |
| G10 | `remove` | `personal_memory.go:DeleteMemoryItem`, forwarding alias | CL-1 found only its definition; removal leaves zero tracked code references | Committed with roadmap as `21ac9ad`; Store/HTTP behavior and four-mode Windows compilation passed; no push/activation in that follow-up |
| G11 | `retain-required` | `personal_memory.go:DeleteMemory` and `ForgetMemory` | HTTP permanent-Forget -> Engine `ForgetLibraryItem` -> `ForgetMemory` -> `DeleteMemory`; direct Store privacy/search tests | Preserve transaction, opaque tombstones, content/provenance/action scrubbing and recapture suppression; never replace with physical deletion or remove ignored-reason privacy behavior |

## Engine orchestration

| ID | Disposition | Target and role | Supported consumers / evidence | Checks and rollback constraint before any change |
| --- | --- | --- | --- | --- |
| E01 | `extract` | `internal/engine/settings_execution.go:SaveSettings`, pure `settings_profiles.go:mergeReasoningProfilesByProvider` and `settings_executable.go:prepareReasoningExecutableChange` | HTTP settings/onboarding/provider paths; existing `providerSwapPlan` | Candidate cleanup `7d00d9c`, file move `02e063e`, profile merge `aea3867`, executable preparation `c303ed4` reviewed/committed; cancellation/Node-fixture validation detours `edb246d`/`8dc3f5e` resolved observed failures. Lock, disposal, normalization, errors and side-effect order preserved. Further decomposition requires a separate proposal |
| E02 | `extract` | `internal/httpapi/server.go` capture/application adaptation | HTTP routes, Bridge contract/token checks, split-capture ownership and Engine calls | Preserve route/DTO/status/error shapes, request context and owner gates; identify one complete use case before a service layer |
| E03 | `retain-required` | Work/scheduler/Engine/session lifecycle files completed in CL-0 | Cross-source admission, cancellation, restart and deferred-cleanup consumers; existing equivalence and build receipts | Retain `e.mu`, `schedule`, worker count through cleanup, sticky scheduler start and wake semantics; do not redo completed moves or add drain |

## Native runtime and QA

| ID | Disposition | Target and role | Supported consumers / evidence | Checks and rollback constraint before any change |
| --- | --- | --- | --- | --- |
| N01 | `retain-required` | Production factories and `native-pilot/shared/` helpers | Actual manual/daily/main roots contain 72/73/92 modules, zero fixture imports; 45 helper payload hashes match prior evidence | Keep one implementation per shared responsibility; preserve source/resource anchors and sealed module pins |
| N02 | `defer-until-replacement` | Compatible X owner versus generic source/IG owner | X qualified recipe and `main_x` behavior coexist with source-bound generic `main_source`; shared kernel is already reused | Full proof/lifecycle parity must precede migration; unqualified IG is not a replacement authority for X |
| N03 | `extract` | `x-native-owner-runtime.mjs` framing, operation routing and owner orchestration | Production factories/package roots and private owner protocols; not just QA | Preserve parser limits, prepare/acquire ordering, original deadlines, account/document checks and independent cleanup failures; split one responsibility |
| N04 | `retain-required` | `packaged-assets.mjs` rejecting source placeholder and generated catalogs | `build-x-native-owner.mjs` replaces it, embeds timing policy and seals native bundles; worker dynamically selects declared resolver paths | No mutable sibling fallback or direct source execution substitute; verify packaged consumers and assets after any move |
| N05 | `retain-required` | Offline owner ports, provenance fixtures and source-copy compatibility harnesses | Go/Node/PowerShell QA entry points import production helpers but provide synthetic policy/resources | Keep unique negative/provenance/restoration/cancellation coverage; do not rewrite old staged snapshots or remove environment gates |

## Bridge and frontend

Bridge paths are relative to AkuBridge; web paths are relative to AkuSidecar.

| ID | Disposition | Target and role | Supported consumers / evidence | Checks and rollback constraint before any change |
| --- | --- | --- | --- | --- |
| B01 | `retain-required` | `native-runtime-client.js` v1 retry and `native-host/protocol.go` versions 1/2 | Host actively accepts both; client maps v1 reconcile to status/start; `native-runtime-client.test.mjs:115,189,752` | Preserve retry bounds, exact request identity and v1 exclusion of v2 capabilities/policy; installed old hosts not enumerated |
| B02 | `unverified` | `aku-browser-tab-bridge.js:capabilitiesForSidecar` versus `bridge-capabilities.js:bridgeCapabilitiesForProtocol` | Both are referenced by loopback message/heartbeat flows; latter strips `focusPolicyRevision`, former does not | Resolve supported legacy consumer shape and add boundary coverage before consolidation; do not silently change negotiated protocol or label this a proven regression |
| B03 | `retain-required` | `content-script.js:legacyExpansionState` | Two extraction/presentation call sites map `no_collapse_observed` to `already_complete`; observation/presentation shape remains a consumer | Retain wire/presentation interpretation until an explicit migration; current state names alone do not make this dead code |
| B04 | `retain-required` | `surface-cleanup-receipt.js` and web `capture-surface-cleanup-receipt.js` | Background service and embedded UI are separately packaged; `capture-surface-policy.test.mjs:44` enforces byte-normalized parity | Retain exact lease/source/count/reason predicates in both packages until a packaging replacement is approved |
| B05 | `retain-required` | `service-worker-entry-v3.js` | Manifest background root directly imports it; it selects registration generation while delegating logic | Not an unused wrapper; changing/removing it affects extension registration/update identity |
| B06 | `retain-required` | Source-adapter DOM/media fallback branches, including LinkedIn old/new author shapes | Active adapter calls under current registered scripts and page-world resolver catalogs | Preserve attribution/identity/unknown states; layout fallback retirement requires source evidence, not a name search or broader permissive parsing |
| F01 | `extract` | Web `app.js` Timeline and Settings feature/event ownership | 37-module app graph, HTML bootstrap, DOM actions, revision/epoch and dirty-state flows | Preserve event attach/teardown, focus, cache invalidation, cleanup barriers and provider actions; separate UI behavior checks from authenticated trials |
| F02 | `unverified` | Direct web `.test.mjs` files under `go:embed web/*` | App imports do not consume them, but embedded filesystem/static routing does | Review asset/route and test-location contracts before excluding or moving files; tests are not removable merely because app does not import them |

## Launcher, installer and build consumers

Paths in this section are relative to AkuBrowser.

| ID | Disposition | Target and role | Supported consumers / evidence | Checks and rollback constraint before any change |
| --- | --- | --- | --- | --- |
| P01 | `retain-required` | Launcher `--legacy-single-process`, runtime args and relaunch forwarding | Launcher CLI/manifest/run paths and tests; capture-split graduation explicitly retains ordinary-path rollback | Preserve opt-in rollback and argument propagation; packaged rollback acceptance remains separate |
| P02 | `retain-required` | Installer `legacySetupPath` removal/uninstall cleanup | `installer/windows/installer.go` removes old setup executable during install and includes it in uninstall paths | Old-file cleanup is active upgrade behavior; retiring the old UI did not retire upgrade cleanup |
| P03 | `defer-until-replacement` | Signed update manifest v1 and conditional legacy artifact names | Windows/macOS companion builders, manifest signer, Bridge host update reader and tests; Windows emitter requires matching product/Bridge/Sidecar identity | Keep exact signature/version/compatibility policy until supported old host/update consumers migrate; no manifest relabel or schema reinterpretation |
| P04 | `unverified` | Frozen Store/macOS/preview asset-generation and distribution tooling | README/release contracts and explicit developer scripts still reference historical lanes | Establish supported recovery/maintenance consumers and owner retirement decision; frozen publication is not proof that every tool is obsolete |
| P05 | `unverified` | Existing uncommitted Browser build/cache scripts and helpers | Current worktree has 13 preserved dirty files; root metadata can be read but launcher/installer dependency closure is incomplete offline | Separate ownership/review before inclusion; keep hashes unchanged and do not commit them under CL-1 or claim a fresh clean tuple |

## First concrete proposal for CL-2

This proposal was subsequently approved and applied. Its original scope/evidence
is retained below; the final result is recorded in the next section.

**Remove only `Store.DeleteMemoryItem` at
`internal/store/personal_memory.go:1129-1131`.** It is a three-line forwarding
alias with no validation, state, resource, transaction, authorization or cleanup
behavior of its own.

Evidence supporting this narrow proposal:

1. Tracked code-only reference search across Sidecar, Bridge and Browser finds
   exactly the definition, with no caller, interface or named wire consumer.
   Documentation and saved evidence are excluded from caller counts; the new
   audit documents intentionally name the proposed symbol.
2. The four Go tag graphs and auxiliary/package roots are inventoried. Repository
   scripts contain no generator call to the symbol. Bounded peer inspection of
   staged source copies found definitions, not live callers.
3. No Store method-by-name/reflection/linkname dispatch consumer was found.
   HTTP permanent Forget explicitly calls Engine -> `ForgetMemory` -> `DeleteMemory`.
4. The symbol is in an `internal/store` package, not an HTTP/wire endpoint or
   published SDK. Unknown forks/out-of-tree tools are not globally excluded,
   but no supported consumer is demonstrated for this alias.

Approved scope and verification:

- Delete the alias definition only, with no forwarding replacement.
- Preserve `DeleteMemory`, `ForgetMemory`, their callers, reason handling,
  migrations, tombstones and every privacy/recapture guard.
- Preserve historical receipts/staged source copies; do not edit them to make
  text searches empty. This is source cleanup, not a schema or data operation.
- Repeat the code-only reference census at the then-current HEAD; inspect the
  exact diff; run existing Store and HTTP Library behavior coverage, including
  `TestMemoryDeleteClearsPrivacyAndLeavesOpaqueTombstone` and
  `TestLibraryHTTPForgetPermanentlyTombstonesAndSuppressesRecapture`.
- Compile the relevant Windows build variants under the established policy;
  use broader regressions only for a concrete integration gap. Add no test whose
  only assertion is that the deleted method's name is absent.

Code-only rollback is a normal revert of the removal commit because this patch
must not alter durable data, schema or runtime. A newly discovered supported
caller would change the disposition. The CL-1 inventory itself ran no product
tests/builds/trials and applied no removal; subsequent G10 results follow below.

## CL-2 G10 result (2026-11-01)

| Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- |
| Removed the forwarding declaration only in `internal/store/personal_memory.go`; committed with roadmap as `21ac9ad` | Exact file-remainder equivalence; Store 382 pass/5 skip and HTTP API 258 pass/23 skip, zero failures; eight Windows amd64 artifacts across four tag modes | Not performed by this increment | Scoped change reviewed and committed; not comprehensive CL-2 completion or operational acceptance |

Evidence: [validation receipt](../runtime/qa/cl2-delete-memory-alias-20261101/validation.json).
The receipt was captured before commit with source parent `24e5a08`; its hashes
identify the payload later committed as `21ac9ad`. The only code delta is the
three-line alias declaration plus its blank
separator. `DeleteMemory`, `ForgetMemory`, all other file contents, schema,
protocol and guards remain unchanged. Privacy/tombstone, recapture-suppression
and physical-remove HTTP behaviors passed existing tests. Skips are recorded,
not counted as passes; no new test was added merely to assert symbol absence.

The post-removal code-only census has zero references. Historical staged
definitions and saved evidence are preserved. The later local commit was
explicitly requested. No runtime, data, configuration, trial or push action was
performed by this follow-up.

## G06 result (2026-11-01)

| Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- |
| `ReasoningConfig.UnmarshalJSON` and comment moved unchanged to `internal/config/reasoning_config_decode.go`; smoke test now supplies the development app-shell/split context; committed as `d82f53b` | Source equivalence passes. Latest default/dev each: 46 pass, zero failures, two skips; eight Windows amd64 artifacts across four tag modes pass | Not performed | Scoped review found no actionable issue; decoder and test correction committed. No repository-wide cleanup or operational acceptance claim |

Evidence: [initial G06 validation](../runtime/qa/g06-reasoning-config-decode-20261101/validation.json),
[test-context correction and passing regressions](../runtime/qa/repository-config-test-context-20261101/validation.json),
and [review of the tested payload](../runtime/qa/g06-review-roadmap-20261101/review.json).
All were captured with source parent `21ac9ad`. The initial receipt identifies
the decoder payload; the correction and review identify the final tested payload
committed as `d82f53b068c99c2f5b60d512702d7ad23ce5be35`. Both strict decoder
calls, modern/flat branch selection, receiver, error returns and every other
byte-normalized part of `config.go` remain identical. Existing flat/modern
provider configuration tests pass.

Initially, `TestRepositorySidecarConfigLoads` failed with
`Instagram trial requires development split capture and concurrent Instagram mode`.
The repository config enables development Instagram trial, while the original
smoke test called `Load` without development/split options. A Go overlay restored the old
decoder at compile time and removed the new file without touching the worktree;
the baseline config suite produced the same failure and message (27 pass, one
fail). The initial new-source default/dev runs each recorded 45 pass, one fail
and two skips. Those raw receipts remain historical non-green results.

The separately approved correction sets `Dev`, `AppShell` and
`WindowsCaptureSplit` in the test's `Load` options. Existing provider resolution,
C2PA path and workload-budget assertions remain. On non-Windows platforms the
test expects the Windows trial config to be rejected; that branch was reviewed
but not executed on this Windows host. Repository config, production sources
and guards are unchanged by the test correction. Latest default/dev runs of
`./internal/config ./cmd/akusidecar` each record 46 pass, zero failures and two
skips, including the repository smoke test and existing Instagram/split guard
tests. Engine and application Windows artifacts across default, dev, fixture
and combined tags passed compilation; all eight hashes match the preceding
G06 production builds. Review reused this evidence after matching current
source/artifact hashes, without repeating unchanged tests or builds.

No push, activation or live trial was performed by the G06 follow-up. The later
bounded E01 changes are recorded below; larger seams remain proposals.

## E01 candidate cleanup and Settings execution (2026-11-01)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Provider candidate cleanup on planning failure | `7d00d9c`: deferred disposal begins immediately after successful candidate construction; a ready plan transfers ownership to the caller | [Cleanup receipt](../runtime/qa/provider-swap-candidate-cleanup-20261101/validation.json): six failure scenarios reproduced before the fix and passed after it; default 526 pass/38 skip, dev 1,199 pass/48 skip, zero failures; eight Windows artifacts | Not performed | Scoped fix and regression reviewed/committed; primary error/cause, success ownership, active runtime and durable settings preserved |
| Settings execution file boundary | `02e063e`: only `Engine.SaveSettings` moved to `settings_execution.go` | [Extraction receipt](../runtime/qa/settings-execution-20261101/validation.json): 120-line method and original file remainder equivalent; same default/dev counts with zero failures; new file included in all four build-tag graphs; eight Windows artifacts | Not performed | Scoped review found no issue; committed. This is a file boundary, not completion of Settings phase decomposition or CL-3 |

The cleanup receipt was captured with source parent `4c8c06e`; its source/test
hashes identify the payload committed as `7d00d9c`. The extraction receipt was
captured with parent `7d00d9c`; its source hashes identify the method move
committed as `02e063ed6a57c6c32f06914927dfd3d666cf40bd`. The original receipts
remain unchanged, including their pre-commit fields. Review matched current
source and artifact hashes before reusing the passing checks.

The production factory remains `reasoning.NewProvider`; an explicit local
factory parameter permits deterministic lifecycle tests without a global hook.
Failure disposal calls `Close` once when supported, ignoring its error so the
planning error and wrapped cause remain primary. A successfully returned plan
is not closed by the builder. Existing `SaveSettings` ownership and lock scope
remain unchanged.

The getter, helpers, callers, config, protocol and guards were not moved or
changed by the extraction. Candidate cleanup still precedes operation unlock;
the Store save, runtime requests/cancellation/activation, retention and both
existing scheduler notification sites keep their order. An error after
persistence does not imply rollback. No further push, activation or live trial
was performed for these two increments.

The later approved profile-merge extraction and cancellation validation detour
are recorded below. No broader Settings service was introduced.

## E01 profile merge and cancellation validation detour (2026-11-01)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Pure provider-profile merge | `aea3867`: `mergeReasoningProfilesByProvider` in `settings_profiles.go`; caller retains both normalization positions | [Initial receipt](../runtime/qa/settings-profile-merge-20261101/validation.json): nine characterization results pass before extraction; helper/merge/validation tests pass afterward. Its default reader-cancellation failure remains historical; latest combined evidence below is green | Not performed | Scoped helper/test review complete and committed; precedence, map independence, partial/raw entries and existing validation retained |
| Reader cancellation validation detour | `edb246d`: closed transport returns an already-cancelled waiter's context error; a new active-context request still gets `reader broker stopped` | [Combined receipt](../runtime/qa/split-reader-cancellation-20261101/validation.json): deterministic failure before fix, 300 stress passes afterward; default 547 pass/38 skip, dev 1,220 pass/48 skip, zero failures; three profile contracts, four build-tag graphs and eight Windows artifacts pass | Not performed | Scoped fix/test review complete and committed; original strict cancellation test, tombstone/replay and activation guards retained. No operational acceptance claim |

Both receipts record Git parent `5bc8387`. The initial merge receipt pins the
helper payload committed as `aea3867c2ddb49603d7278a2d3a9961133d46b02`.
The cancellation receipt captured that helper as uncommitted input and pins
the combined payload later committed through
`edb246d91711ce8ed1787c68ee9214b5546729bf`. Its preservation hashes identify the
unchanged Engine inputs. Original pre-commit fields and raw logs are preserved;
review reused passing checks after matching source/artifact hashes.

The initial default suite had 544 passes, one failure and 38 skips in
`TestSplitCaptureCloseCancelsReaderBrokerWaiter`. An old-source overlay also
recorded 99 passes and one failure in 100 iterations. That result was not
reported as green. A log barrier then forced close between registration and
the handler's first closed check, reproducing the same error deterministically.
The four-line correction preserves cancellation as the first cause and keeps
the existing stopped rejection for later requests. The same regression now
passes, including cancellation diagnostics and replay tombstone checks.

The merge allocates a new map, copies stored entries then submitted entries,
preserves the old provider's active projection, and uses the submitted active
projection when the provider stays unchanged. It does not normalize, trim,
validate or activate a provider. Existing guards still reject invalid IDs and
more than 32 providers; tests retain the valid 32-provider boundary. Remaining
Store/runtime effects and their error order are unchanged.

No push, activation or live trial was performed for these increments. The next
proposal at that checkpoint was read-only analysis of reasoning-executable
change validation in `SaveSettings`, covering capability checks, busy-session/work rejection and
discovery before persistence, with activation still after persistence. The
later separately approved executable helper and fixture correction follow below.

## E01 executable preparation and Node fixture validation detour (2026-11-01)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Executable-change preparation | `c303ed4`: `Engine.prepareReasoningExecutableChange` in `settings_executable.go`; `SaveSettings` retains persistence and activation | [Initial extraction receipt](../runtime/qa/settings-executable-20261101/validation.json): 20 characterization results pass before/after extraction, helper/caller structural equivalence and eight Windows artifacts. The original dev-suite native-pilot failure is historical; latest combined evidence below is green | Not performed | Scoped source/test review found no blocking issue; committed. Receiver, operation/work locks, trim-equal bypass, original errors, active-provider ownership and side-effect order preserved; CL-3 remains partial |
| Offline Node startup timing detour | `8dc3f5e`: shared fixture wall-wait helper, bounded observation/reply scheduling, failure-only diagnostics and staging dependency pin | [Combined receipt](../runtime/qa/native-pilot-node-integration-20261101/validation.json): default 568 pass/38 skip, dev 1,241 pass/48 skip, zero failures; 98 Node tests; two independent 100-iteration integration batches, including unchanged wrong-document rejection; four tag graphs and eight Windows amd64 artifacts | Not performed | Scoped fixture/test review found no blocking issue; committed. Original monotonic clock, timestamps, receipt shape, document binding, cancellation, freshness and deadline guards retained. Bounded fixture stabilization only; not universal cross-runtime clock synchronization or live-source qualification |

Both original receipts identify parent `eaee999` and the pre-commit worktree.
The extraction payload is now committed as
`c303ed4ff5f37f869c6057baff9ae872c7fc0151`; the combined tested payload is
complete through `8dc3f5e7dcd04af6995fbf9390f9441b6a67421e`.
The [review](../runtime/qa/settings-executable-review-20261101/review.json)
matches all eleven source hashes, reruns the exact source-transformation proof,
checks raw successful logs, four build graphs, eight PE artifacts and fixture
dependency hashes. Passing suites were reused because their inputs match.
Historical receipts and raw failure evidence were not rewritten. The fixture
receipt explicitly records the overwritten initial diagnostic-log gap; the
earlier 30-run raw baseline and subsequent failed stress logs remain available.

Characterization covers supported/unsupported trim-equal no-op, capability and
queued/running-session/active-work rejection, no new pending/shutdown guard,
trimmed/case-changed/empty requests, empty successful resolution, discovery and
persistence failures, context-error identity and current-provider activation
before swap. Discovery remains before persistence; `UseExecutable` stays after
persistence and before `swap.apply`. The new helper neither owns persistence
nor starts a new lifecycle or service.

The Node failure was reproduced without the executable extraction. Synthetic
events and reply delivery could finish within a cross-process clock boundary,
causing `wire_decode/time_invalid` or `timestamp_future`. Fixture scheduling now
crosses a complete observed wall interval before identity and after observation,
inside the original capture budget. Deterministic regressions retain rejection
on stalled/regressing clocks, stale samples and expired deadlines. No timestamp
offset, tolerance, guard relaxation or production clock change was introduced.

No push, activation or live trial was performed for these increments. At that
checkpoint the next proposal was read-only analysis of provider-selection
validation before executable discovery in `SaveSettings`, covering existing
bypasses, lock/probe/Store-read/error order and characterization. Analysis was
completed and the micro-extraction was then declined for insufficient benefit,
as recorded below. No provider-selection helper was implemented.

## CL-3.2 diagnostics export feature result (2026-11-01)

This is a separately approved feature boundary, not a reclassification or
completion of the original 32-boundary CL-1 inventory.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Diagnostics export feature | `1068e35`: `Engine.DiagnosticsExport` and nine related helpers moved to `internal/engine/diagnostics_export.go`; caller and receiver unchanged | [Receipt](../runtime/qa/diagnostics-export-20261101/validation.json): 17 characterization results pass before and after the move; default 585 pass/38 skip, dev 1,258 pass/48 skip, zero failures; four build-tag graphs and eight Windows amd64 artifacts | Not performed | Scoped source/test review found no blocking issue; committed. Exact declaration/Engine-remainder equivalence preserves output, read/error order, protocol, guard and Store authority; CL-3 remains partial |

The receipt identifies parent `5f41e58` and the pre-commit worktree. Its three
source hashes identify the payload now committed as
`1068e359ea71c8543266509079c4d04ffb4011a6`.
The [review](../runtime/qa/diagnostics-export-review-20261101/review.json)
matches source hashes, checks all ten declarations and the Engine remainder,
verifies identical characterization before/after, reads the original passing
logs and checks four source graphs plus eight PE artifact hashes. The other
1,357 tracked-file hashes and unrelated Browser work remain unchanged at review.
Passing checks were reused after matching inputs; original receipts were not
rewritten to claim post-commit execution or activation.

Characterization covers top-level JSON keys, omitted fields and null/empty-array
distinctions; explicit/active/latest-ten-inbox selection; default and explicit
time ranges; grouped run telemetry with first-invocation identity and last detail;
usage/config/Bridge projections; parse/Settings/session/inbox error precedence;
and best-effort telemetry/usage failure. SQL failure injection touches disposable
fixture databases only. Not every dependency failure is separately injected;
exact unchanged bodies preserve all original reads and ignored-error branches.

The default 24-hour export still requests an unsupported aggregate usage window;
the existing ignored error produces a successful report with zero usage. The
test characterizes that behavior; this source move neither repairs it nor adds
any new redaction or time-filtering promise. A functional correction would need
a separate proposal and approval.

The provider-selection micro-extraction was declined after analysis: the
98-line `SaveSettings` already expresses a coherent operation, and another
single-caller helper offers insufficient value. No such helper was implemented.
Prefer complete responsibility boundaries over further small Settings splits.

No push, activation or live trial was performed for this increment. At that
checkpoint the next proposal was read-only analysis of Bridge status/compatibility.
That analysis was completed and its bounded feature move was separately approved;
the result follows below.

## CL-3.2 Bridge status feature result (2026-11-01)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Bridge status/compatibility feature | `7b2d0d8`: nine functions and the two public status/negotiation types moved to `internal/engine/bridge_status.go`; existing callers and Engine receiver retained | [Receipt](../runtime/qa/bridge-status-20261101/validation.json): identical characterization passes 38 results before and after relocation; default 623 pass/38 skip, dev 1,296 pass/48 skip, zero failures; four build-tag graphs and eight Windows amd64 artifacts | Not performed | Scoped source/test review found no blocking issue; committed. Constants, `expectedBridgeActions`, locks, protocol, origin/source/capability guards, heartbeat pointer guard and side-effect order retained; CL-3 remains partial |

The receipt identifies parent `ee9a2a2` and the pre-commit worktree. Its three
source hashes map to the payload now committed as
`7b2d0d8dd935b17287f42002417c2736fa80581c`.
The [review](../runtime/qa/bridge-status-review-20261101/review.json)
checks the source/log/artifact hashes and repeats exact moved-block
and Engine-remainder comparisons. Both types and all nine function bodies,
including the full `e.mu.RLock` scope, are unchanged. At review, the other 1,359
tracked-file hashes and unrelated Browser changes remained intact. Existing
passing suites were reused after matching inputs; historical receipts were not
rewritten as post-commit executions.

Characterization covers status/negotiation JSON keys and null/omission behavior,
ordered reasons/warnings and incompatible precedence, capability count/length/
syntax boundaries, source-readiness consistency, origin allowlist normalization,
read-only status behavior and independent heartbeat action/adapter projections.
Existing regressions also pass for legacy v2 negotiation, future protocol minors,
surface policy, strict authority/limits, multiple origins, headless revocation,
reload completion, expired claims and diagnostics projection. A valid unready
source can remain compatible without acquiring permission; existing admission
checks retain that authority. The existing empty-config test-engine path remains
unchanged, not a new production allowlist bypass.

`RecordHeartbeat` remains in Engine with its original pointer check, retained
authority update, command recovery, reload observation and scheduler notification
order. A status read does not gain heartbeat/origin expiry or lifecycle effects.
Constants and `expectedBridgeActions` stay in `engine.go`; the unchanged Browser
identity checker still finds version/revision/ID markers there. Its release-tuple
checker was not executed; source anchors were verified. No new concurrency,
copying or protocol policy was introduced.

This is a Sidecar CL-3.2 increment. CL-6 still concerns extension background
dispatch and surface ownership; neither it nor comprehensive cleanup is closed.
No push, activation or live trial was performed.

At that checkpoint, the next proposal was read-only analysis of HTTP Bridge
command handlers. The analysis and a separately approved implementation are now
complete; the result follows below. No heartbeat, surface-event or recapture
handler was included in that extraction.

## CL-3.3 HTTP Bridge command feature result (2026-11-02)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| HTTP Bridge command feature | `7cb712b`: next, pending, observation and failure moved to four Server methods in `internal/httpapi/bridge_commands.go` | [Receipt](../runtime/qa/bridge-commands-20261102/validation.json): the identical characterization suite passes 53 results before/after relocation; default 676 pass/38 skip, dev 1,349 pass/48 skip, zero failures; four HTTP build-tag graphs and 12 Windows amd64 artifacts | Not performed | Scoped review found no blocking issue; committed. Route/context/authentication/protocol/guard and side-effect contracts preserved. CL-3 remains partial; no operational acceptance established |

The validation receipt identifies parent `3a3ab3f` and the pre-commit worktree.
Its three source hashes match the payload committed as
`7cb712bc9f9b7bbb6d6d58862484f262a6521120`. The
[review](../runtime/qa/bridge-commands-review-20261102/review.json) checks source,
test-log and binary hashes, exact four-body relocation and the unchanged router
remainder. The [commit readback](../runtime/qa/bridge-commands-review-20261102/final-readback.json)
verifies the committed blobs, parent, three-file scope and clean tracked tree;
existing `.tmp/` and `experiments/` were preserved. Passing evidence was reused
after matching inputs; historical receipts were not rewritten as post-commit
test executions. Counts include parent tests and subtests, not 53 unique cases.

Only the four case bodies change in `server.go`. Method/path predicates, their
order and single-trailing-slash normalization remain there. POST methods receive
that normalized path; `path.Base` command selection is unchanged. Each method
uses the same request context, and contract/token/capture-instance checks still
precede query/body parsing and Engine calls. GET errors remain forwarded; POST
Engine errors remain `badRequest(err.Error())`. Polling's empty 204 bodies,
200 command/runId envelopes, observation's 202 and failure's 200 run envelopes,
JSON/cache headers and existing decoder semantics are retained.

Characterization covers authentication/fence precedence, unchanged state after
rejection, empty/available/repeated polling, required run ID, strict JSON/media
type and bounded-reader behavior, route/path/method behavior, durable observation
acceptance, Engine failure defaults, queued/replayed results, missing run/command,
source mismatch, stale runtime owner and cancellation before authentication or
after body decode. Fixtures use the real Engine/Store with disposable data and
omit `captureLeaseId`; no account/profile/network capture or OS observer starts.
Native trace order remains claim, nil check, trace, response and is verified by
source equivalence. Engine/Store retain admission, fencing, normalization,
failure defaults and lifecycle authority. No new service layer or owner exists.

No push, activation or live trial was performed. This 2026-11-02 documentation
synchronization changes no code, reruns no product tests/builds and inspects no
live runtime. It does not establish CL-6 extension or comprehensive CL-3 completion.

At that checkpoint the next proposal was read-only analysis of the three HTTP
Bridge media-recapture handlers. Analysis and a separately approved feature move
are now complete; the result follows below. UI creation/status, heartbeat and
surface-event routes were not included.

## CL-3.3 HTTP Bridge media-recapture feature result (2026-11-02)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| HTTP Bridge media-recapture feature | `be9fcd2`: claim, observation and failure moved to three Server methods in `internal/httpapi/bridge_media_recapture.go` | [Receipt](../runtime/qa/bridge-media-recapture-20261102/validation.json): identical characterization passes 54 results before/after relocation; default 730 pass/38 skip, dev 1,403 pass/48 skip, zero failures; four HTTP build-tag graphs and 12 Windows amd64 artifacts | Not performed | Scoped source/test review found no blocking issue; committed. Route/context/authentication/protocol/guards, status/error behavior and side-effect order preserved; CL-3 remains partial |

The validation receipt identifies parent `cd0adb3` and the pre-commit worktree.
Its three source hashes match the payload committed as
`be9fcd296449e2b382c216cee1f55ac1a709487f`. The
[review](../runtime/qa/bridge-media-recapture-review-20261102/review.json) verifies
raw logs/artifacts and exact moved-body/router-remainder equivalence, including
the other 1,363 tracked-file hashes. The
[commit readback](../runtime/qa/bridge-media-recapture-review-20261102/final-readback.json)
verifies committed blobs, parent, three-file scope and the clean tracked tree at
commit. `.tmp/` and `experiments/` remain untouched. Passing tests were reused
after matching inputs; historical validation receipts were not rewritten.

Only three case bodies change in `server.go`; predicates, order and path
normalization remain there, with normalized `p` forwarded to each method.
Bodies remain identical except local `r.Context()` bindings. Contract, token and
capture-instance checks still precede input/Engine calls. Claim retains ordered
404/409/204/200 outcomes; POST Engine errors retain `badRequest(err.Error())`
and successful results retain 200. JSON/cache headers and single-Decode behavior
remain unchanged. Engine/Store locks, admission/fencing, persistence, provenance
and deferred cleanup ordering are unchanged.

Characterization covers authentication/fence precedence and rejection without
job/evidence mutation; missing/available/repeated claims, exact Bridge-ID
forwarding, wrong-collector/waiting-admission 204 responses; strict JSON/media
type/reader limits; path/method/trailing-slash behavior; committed results before
success response, durable evidence replacement without new Timeline items;
queued/replayed observations, source/target mismatch, stale owner and cancellation.
Failure intentionally accepts queued and claimed jobs, defaults only stage to
`media_recapture` and preserves an empty code. Stale-owner claim intentionally
can fail a job; not every rejected claim is read-only. These contracts were not
changed to match command-handler behavior. Fixtures attach no real collector,
account, provenance inspector or OS observer.

The initial non-green characterization run is preserved in the receipt. Its
source-mismatch fixture still carried a LinkedIn URL after changing source to X;
the existing canonical-URL guard correctly rejected it before the Store source
check. Correcting that fixture produced the passing baseline before extraction.
Oversize cases also use known string fields to exercise the actual reader bound.
No production fix or relaxed guard was needed. The 54 results include parent
tests and subtests and are not a unique-case count.

No push, activation or live trial was performed. This documentation update
changes no code, reruns no tests/builds and inspects no live runtime. Neither
comprehensive HTTP/Engine cleanup nor CL-6 extension acceptance is established.

At that checkpoint, the next proposal was read-only analysis of Engine
media-recapture orchestration. That analysis and a separately approved feature
move are now complete; the result follows below. Generic capture-runtime,
passive enrichment and HTTP responsibilities were not included in that move.

## CL-3.2 Engine media-recapture feature result (2026-11-02)

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Engine media-recapture feature | `c614512`: ten methods moved unchanged to `internal/engine/media_recapture.go`; receiver and callers retained | [Receipt](../runtime/qa/engine-media-recapture-20261102/validation.json): identical characterization passes 34 default/35 dev results before/after relocation; default 764 pass/38 skip, dev 1,438 pass/48 skip, zero failures; four Engine build-tag graphs and 12 Windows amd64 artifacts | Not performed | Scoped review found no blocking issue; committed. Receiver, locks, request/background contexts, error/effect order, protocol and guards retained; CL-3 remains partial |

The validation receipt identifies parent `9d6bf20` and the pre-commit worktree.
Its four source hashes match the payload committed as
`c61451227ae014be3815a318d1a312cffc563894`. The
[review](../runtime/qa/engine-media-recapture-review-20261102/review.json) verifies
raw source/log/artifact hashes, all ten method signatures/bodies and the Engine
remainder. The existing final CRLF in `engine.go` was normalized by gofmt; this
is the only remainder difference. The other 1,365 tracked-file hashes match.
The [commit readback](../runtime/qa/engine-media-recapture-review-20261102/final-readback.json)
verifies committed blobs, parent, four-file scope and clean tracked tree at
commit; `.tmp/` and `experiments/` were preserved. Passing suites were reused
after matching inputs. Historical receipts were not rewritten as later runs.

The feature includes queue/default-reason adaptation, driver/collector claim
delegation, admitted Browser owner checking, observation acceptance, status
reading and failure completion. `operation`, `e.mu`, `captureMu` and
`captureBehaviorMu` scopes remain identical, including route release and
unlock-before-cleanup defer order. Primary work retains request context;
terminal cleanup retains its existing background context. Engine fields and
Store/Manager/Coordinator state ownership do not move. Capture pumps, generic
owner/collector/cleanup helpers, provenance implementation, HTTP and passive
enrichment remain unchanged; no new service, wrapper or state owner was added.

Characterization covers queue guard/error precedence, no added jobs/leases on
rejection, Bridge/Quiet/headless/foreground selection, epoch/generation stamps,
duplicate/Store failure lease disposal, persisted waiting jobs after collection
hold rejection, durable result before provenance/cancellation, best-effort
provenance failures, challenge-only X cancellation, request cancellation and
background cleanup. Cleanup checks the admitted identity, behavior-lock release,
pending receipt and lease retention until acknowledgement. The hold-rejection
test uses only the existing `auth_session_dev` concurrent binding with fake
processes; no production admission exception was added. The 34/35 results count
parent tests and subtests, not unique cases. Existing hybrid/owner/photo/restart
coverage remains in the combined suites.

The initial non-green fixture receipt is preserved. A queue rejection fixture
incorrectly assumed zero leases despite an already active session; its final
assertion preserves the pre-call lease count. The fake inspector initially used
unsupported `absent`; it now returns schema-supported `no_manifest`. Both fixture
corrections preceded the passing baseline and required no production fix or
relaxed guard. All new tests use disposable Store data and fake process/inspector
boundaries without network capture.

No push, activation or live trial was performed. This documentation update
changes no code and reruns no product tests/builds. It does not close CL-3,
extension CL-6 or operational source acceptance.

At that checkpoint, the next proposal was read-only analysis of Engine Bridge command orchestration in
`engine.go` (claim, pending, observation and failure). Map admission/fencing,
Store effects, scheduling, lock/context scopes, error order and characterization
gaps before selecting one extraction. Recovery is shared with heartbeat and
scheduler; normalization/validation is shared with media recapture and native
consumers. Preserve shared guards and Engine/Store/Manager authority; do not
duplicate helpers or repeat completed lifecycle work. No implementation approved.

## CL-3.2 Engine Bridge command feature result (2026-11-02)

This separately approved feature separation extends the CL-3.2 ledger. It does
not change the original CL-1 inventory counts or establish global stage closure.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Engine Bridge command feature | `3e13434` | Identical characterization passes 37 results in both default/dev before and after relocation; latest default 801 pass/38 skip, dev 1,475 pass/48 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-bridge-commands-review-20261102/review.json) and [commit readback](../runtime/qa/engine-bridge-commands-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Seven methods in Engine `bridge_commands.go` retain receiver, lock/context scopes, recovery and persistence/scheduling order, errors, protocol and guards. Shared normalization/validation, Store, capture runtime, heartbeat and scheduler unchanged; CL-3 remains partial |

The [validation](../runtime/qa/engine-bridge-commands-20261102/validation.json) is the original pre-commit receipt at parent
`7b30a00bfe89e08a3813f3ba04fae4b4d9dfdece`. Its three source hashes match
the payload committed as `3e13434daa8282d361316fa4cf280954121a1ccd`. The [review](../runtime/qa/engine-bridge-commands-review-20261102/review.json)
checks all seven signatures/bodies, exact Engine remainder, all six passing
test logs, four build-tag graphs and 12 PE artifact hashes. The other 1,368
tracked files match the implementation baseline. The [commit readback](../runtime/qa/engine-bridge-commands-review-20261102/final-readback.json)
checks parent, three-file scope and committed-blob hashes. The tracked tree was
clean at that commit; pre-existing `.tmp/` and `experiments/` remain preserved.
Historical receipts are unchanged; passing product tests/builds were reused
after matching their source and artifact identity, not rerun for this review.

The moved methods are `ClaimCommand`, `claimCommandForDriver`,
`claimCommandForCollector`, `PendingBridgeRunID`,
`recoverExpiredBridgeCommands`, `AcceptObservation` and `FailCommand`.
They remain Engine methods with existing callers, fields, locks and request/
background contexts. Recovery remains shared with heartbeat/scheduler. Observation
normalization/validation and `afterAcceptedObservation` remain in their existing
files. No new service, wrapper or state owner is introduced.

Characterization records development/fixture/collector/owner/frozen-plan guards,
queued versus claimed expiry during claim/pending polling, validation before
writes, durable normalized evidence despite a later projection error, replay
rejection, failure defaults and errors, X-only challenge cancellation after
persistence, behavior-lock release before scheduling, request cancellation and
background terminal-claim cleanup. The 37 results count parent tests and subtests,
not 37 unique cases. Existing follow-up fallback, cross-driver, pacing and stale
owner coverage remains in the combined suites. Tests use disposable Store data
and fake capture processes without network capture or production test hooks.

Initial non-green fixture logs are retained. Corrections attach the fake managed
runtime before session creation, select the non-X source before creating its
session rather than violating source uniqueness, and inject a projection-read
error after durable capture without violating the transaction's CHECK constraint.
These corrections preceded the passing baseline and required no production fix
or guard relaxation.

No push, activation or live trial was performed for this increment. This roadmap
update changes documentation only. Implemented/tested/scoped review acceptance
does not establish activation, live source acceptance, CL-3 closure or CL-6 work.

At that checkpoint, the next proposal was read-only analysis of Engine media-provenance orchestration.
Its current Engine methods combine inspector availability/runtime projection,
resume recovery, durable queue/claim/result handling and a cancellable background
worker. Map callers, worker registration/retirement under `e.mu`, request versus
background contexts, error/effect order and characterization gaps before choosing
one coherent feature boundary. Retain Engine/Store/inspector ownership and
best-effort enrichment semantics. Do not change the worker algorithm, add drain,
combine it with AI detection, or implement an extraction without separate approval.

## CL-3.2 Engine media-provenance feature result (2026-11-02)

This separately approved feature separation extends CL-3.2 without changing the
original CL-1 inventory counts or closing comprehensive backend cleanup.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Engine media-provenance feature | `8d7f7b6` | Identical characterization passes 24 results in both default/dev before/after relocation; six-package Inspector/Store/Engine/HTTP/config/application suites pass default 1,214/43 skip and dev 2,040/55 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-media-provenance-review-20261102/review.json) and [commit readback](../runtime/qa/engine-media-provenance-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Five Engine methods in `media_provenance.go` retain receiver, locks, request/background contexts, queue/worker/result order, protocol and guards. Exactly two gofmt-only alignment changes remain in `engine.go`; Store, Inspector, worker algorithm and authority unchanged; CL-3 remains partial |

The [validation](../runtime/qa/engine-media-provenance-20261102/validation.json) is the original pre-commit receipt at parent
`5d79fd4156d09031e8b29b227682c40ee1c863ee`. Its three source hashes match
`8d7f7b606c7cfecdabd00d0fda5b7e8bb4d1c083`. The [review](../runtime/qa/engine-media-provenance-review-20261102/review.json) checks all five unchanged methods,
the two explicit whitespace-only Engine alignment changes, six test logs,
four build-tag graphs and 12 Windows artifact hashes. The other 1,370 tracked
files match the implementation baseline. The [commit readback](../runtime/qa/engine-media-provenance-review-20261102/final-readback.json)
checks parent, exact three-file scope and committed blobs; tracked source was
clean at that commit and pre-existing `.tmp/`/`experiments/` were preserved.
Passing tests/builds were reused at matching source and artifact hashes;
historical receipts were not rewritten as later runs.

Moved methods: `SetMediaProvenanceInspector`, `MediaProvenanceRuntime`,
`ResumeMediaProvenance`, `launchMediaProvenance` and
`launchMediaProvenanceItems`. Receiver and callers remain Engine-owned. The
`mediaOrigin` field, Store/Inspector implementations and shared work lifecycle
stay in their existing owners. Queue precedes the active-worker check; the
global `media-provenance` key and e.mu register/retire sections remain unchanged.
Resume retains caller context for durable recovery; worker inspection uses an
independent cancellable background context and result persistence retains its
background context after cancellation. No new service, interface, state owner,
wake/drain policy, worker admission or protocol exception is introduced.

Characterization covers inspector/runtime availability, resume guard/context/
Store errors and metadata reset, verifier-version isolation, source host allowlist,
queue-before-active admission, single-worker serialization, session/empty queues,
load/queue/claim/inspect/finish error behavior and continuation after a failed
result write. Shutdown cancels inspection, keeps the worker registered through
result persistence, and then allows WaitForIdle to finish. All 24 results count
parents/subtests. Tests use disposable Store data, fake inspectors, SQL failure
injection and channels; no network capture, c2patool or new production test hook.

The first mechanical proof expected byte-identical Engine remainder; gofmt
instead aligned adjacent Epoch/ProviderName declarations after method removal.
The final proof enumerates exactly those two whitespace-only changes. Completing
the proof did not change the already tested source. This is a tooling assertion
correction, not a test failure or production behavior fix.

Regression scope is six packages, explicitly including Inspector and Store;
the 1,214/2,040 pass totals and 43/55 skips are not directly comparable with
earlier four-package results. No runtime activation or live trial is performed.
Publication occurs after the documentation commit and is verified separately
by the post-push readback; it does not close CL-3 or live source acceptance.

At that checkpoint, the next proposal was read-only analysis of Engine AI-detection orchestration.
Map fast/deep candidate selection, reasoning-profile/model selection, durable
job/result handling, background work registration and cancellation before
selecting one coherent extraction and its characterization tests. Preserve
`e.mu`, worker keys, settings/budget gates, result contexts and Store/resolver
authority. In particular, `cancelDeepDetections` also cancels media provenance;
keep that existing cross-feature behavior explicit. Do not change worker
algorithms, add drain, reopen the declined Settings micro-extraction, or implement
another refactor without separate approval.

## CL-3.2 Engine AI-detection feature result (2026-11-02)

This separately approved feature separation extends CL-3.2 without changing the
original CL-1 inventory counts or closing comprehensive backend cleanup.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Engine AI-detection feature | `b84a62d` | Identical characterization passes 32 results in both default/dev before/after relocation; seven-package Detector/Inspector/Store/Engine/HTTP/config/application suites pass default 1,270/45 skip and dev 2,096/57 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-ai-detection-review-20261102/review.json) and [commit readback](../runtime/qa/engine-ai-detection-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Five methods in `ai_detection.go` retain receiver, locks, request/background contexts, admission/profile/job/result order, errors, protocol, guards and AI/media-provenance cancellation. Only standalone setter alignment normalized; Engine remainder byte-identical; CL-3 remains partial |

The [validation](../runtime/qa/engine-ai-detection-20261102/validation.json) is the original pre-commit receipt at parent
`222bcee45a4dff832810d1589237e16794fdceae`. Its three source hashes match
`b84a62d2b050dc27cd32e77865ed15d78b37e1f2`. The [review](../runtime/qa/engine-ai-detection-review-20261102/review.json) verifies all five unchanged signatures/
bodies, exact Engine remainder, six passing test logs, four build-tag graphs and
12 Windows artifact hashes. The other 1,372 tracked files match the implementation
baseline. The [commit readback](../runtime/qa/engine-ai-detection-review-20261102/final-readback.json) verifies parent, exact three-file
scope and committed blobs. The tracked tree was clean at that commit; existing
`.tmp/` and `experiments/` were preserved. Passing tests/builds were reused
at matching source/artifact hashes; historical receipts were not rewritten.

Moved methods: `SetAIDeepResolver`, `runFastDetection`,
`launchDeepDetection`, `launchDeepDetectionItems` and `cancelDeepDetections`.
Receiver, fields, callers, Store/Resolver ports and shared work lifecycle remain
Engine-owned. Nil resolver/empty shortlist precede same-session cancellation;
eligible work cancels predecessors before settings reads. Job create/register/
start/resolve/assessment/finish order, profile snapshot and usage metadata remain
unchanged. Fast detection retains caller context; deep worker uses cancellable
background context and durable result writes retain background context. Global
`ai:*` and exact `media-provenance` cancellation still runs under e.mu without
removing active entries or waiting for drain. No new service, interface, state
owner, worker admission, protocol or guard exception is introduced.

Characterization covers fast persistence/context/errors, admission/cancellation
precedence, session-loading before the resolver guard, plain/profiled resolver
dispatch and Model/ModelForProfile order, candidate selection, running state
before resolution, unlocked resolver boundary, assessment supersession before
job completion, model/token/timing receipts, start/resolve/save/finish failures,
usage-limit persistence before scheduler notification, cancellation/error versus
success-after-cancellation, real provenance-worker cancellation/result persistence,
and Settings persistence before cancellation. The 32 results count parents and
subtests. Existing caller onboarding/budget, feedback, shortlist/resolver and
Store coverage remains in the seven-package suites.

The initial Settings integration fixture incorrectly reused synthetic orphan
timeline rows. Settings persisted and cancellation occurred, but post-save
retention correctly returned database cleanup required. The final Settings test
uses a clean disposable Store and retains strict durable-before-cancellation
and rejected-save assertions. This fixture correction preceded the passing
baseline; no health/retention guard or production behavior was changed. Fake
resolvers/inspectors, channels and SQL failure injection perform no network or
live inference and introduce no production test hooks.

No runtime activation or trial is performed. Publication follows the documentation
commit and is verified separately by the post-push readback. Implemented/tested/
scoped review does not establish CL-3 closure or operational source acceptance.

At that checkpoint, the next proposal was read-only analysis of Engine session admission/creation.
Map visible/automatic callers, readiness and consent, source selection, database/
onboarding/calibration/recapture gates, enrichment cancellation, capture lease
ownership, durable session creation and initial scheduling. Preserve operation/
schedule lock ordering, release-before-schedule, request/background contexts,
cleanup and error/effect order, protocol and guards. Identify one useful feature
boundary and characterization gaps before any extraction. Keep daily/native,
Store/Manager authority and completed lifecycle/scheduler work unchanged; do not
add drain or implement another refactor without separate approval.

## CL-3.2 Engine session-admission feature result (2026-11-02)

This separately approved increment extends CL-3.2 without changing the original
CL-1 inventory counts or closing comprehensive backend cleanup.

| Increment | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Engine session-admission feature | `4bf0731` | Identical characterization passes 31 default/34 dev parent/subtest results before/after relocation; seven-package suites pass default 1,301/45 skip and dev 2,130/57 skip, zero failures; four Engine build-tag graphs and 12 Windows artifacts; [review](../runtime/qa/engine-session-admission-review-20261102/review.json) and [commit readback](../runtime/qa/engine-session-admission-review-20261102/final-readback.json) verify source/log/artifact and committed-blob identity | Not performed | Scoped source/test review found no blocking issue; committed. Three Engine methods and one error helper in `session_admission.go` retain signatures/bodies, caller/receiver ownership, lock/context/error/effect order, source/policy/lease behavior, protocol, guards and AI/provenance cancellation; Engine remainder byte-identical; CL-3 remains partial |

The [validation](../runtime/qa/engine-session-admission-20261102/validation.json) is the original pre-commit receipt at parent
`105c427edeee71fbee6bc0eb023770a52a016d0e`. Its four source hashes match
`4bf0731e1874130cbd9ee503cc50fbb00685cc4f`. The [review](../runtime/qa/engine-session-admission-review-20261102/review.json) verifies unchanged declarations and
Engine remainder, six passing test logs, four build-tag graphs and 12 Windows
artifact hashes. The other 1,374 tracked files match the implementation baseline.
The [commit readback](../runtime/qa/engine-session-admission-review-20261102/final-readback.json) verifies parent, exact four-file scope and
committed blobs; existing `.tmp/` and `experiments/` remain preserved.
Passing tests/builds were reused at matching source/artifact hashes. Historical
receipts were not rewritten as later runs.

Declarations: `StartVisibleUpdate`, `grantedActiveSources`,
`noGrantedActiveSourceError` and `startSession`. The first, second and fourth
retain Engine receivers; the error factory remains a free helper. Automatic
status/update and HTTP callers retain their signatures and context choices.
Store creates and freezes durable policy/source plans; Manager retains lease
authority. Shared scheduling, daily/source helpers, AI/provenance cancellation,
Engine fields and protocol/guard implementations remain in their existing owners.

Characterization covers visible user/onboarding versus prepared policy, guard
precedence, browser/headless source intersection/order and saved-preference
preservation, cancellation before lease/Store failure, lease release on Store
write/readback error, retained durable session/lease after scheduling failure,
operation release after durable ownership and before blocked scheduling, and
request cancellation before admission or after durable creation. Development
coverage uses the existing offline daily fixture to verify source append order,
frozen browser/daily drivers and ready-but-unselected daily routing without
changing saved settings. The 31/34 counts include parents/subtests; no live
source/account/browser capture or new production hook is used.

The initial source-selection fixture marked X not ready while still retaining
inconsistent permission/script/grant fields. The strict Bridge contract rejected
that heartbeat. The final fixture uses a consistent readiness/grant tuple before
the passing baseline. No protocol, health, ownership or admission guard changed.
No service/interface/state owner or new rollback/drain behavior was introduced.

No activation or trial is performed. Publication follows the documentation commit
and is verified separately by the post-push readback. Scoped review acceptance
does not establish CL-3 closure or operational source acceptance.

The next proposal is read-only analysis of shared observation normalization and
validation in Engine: `normalizeObservation`, `validateObservation` and
`blockHasEvidence`. Map ordinary Bridge, media-recapture and native/development
consumers, error precedence, evidence-key derivation, canonical source URLs,
direct-context/attachment/quoted-post rules and characterization gaps before
choosing one useful feature boundary. Preserve all evidence admission guards,
protocol and callers; do not add fallback acceptance or implement another
extraction without separate approval.
