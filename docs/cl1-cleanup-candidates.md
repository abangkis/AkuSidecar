# CL-1: cleanup candidate ledger

Date: 2026-11-01. Source and method: [consumer inventory](cl1-consumer-inventory.md).
Scope: CL-1 candidate classification, followed by separately approved G10 and
G06 implementation records below. Other candidates remain proposals or retained contracts.

## Dispositions and approval

| Disposition | Count | Meaning |
| --- | ---: | --- |
| `remove` | 1 | G10 removal separately approved and tested in the worktree; see its CL-2 record |
| `extract` | 5 | Responsibility seam worth a separately approved move/split |
| `retain-required` | 19 | Supported caller, data, build or packaging obligation demonstrated |
| `defer-until-replacement` | 3 | Consumer/authority migration must precede retirement |
| `unverified` | 4 | Evidence or contract decision still needed |

**Approval at the CL-1 checkpoint was inventory only.** G10 and G06 were subsequently
approved and applied; implementation of the other rows remains unapproved. Retained
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
| E01 | `extract` | `internal/engine/engine.go:SaveSettings` validation/planning/persistence/effects | HTTP settings/onboarding/provider paths; existing `providerSwapPlan` | Map `operation`, candidate disposal, post-save errors, retention and notify order; no new transactional promise |
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

No push, activation or live trial was performed. E01 Settings and larger seams
remain proposals outside this increment.
