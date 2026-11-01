# CL-1: entry points, consumers and compatibility inventory

Date: 2026-11-01 (Asia/Jakarta).
Scope: [CL-1](code-cleanup-roadmap.md#cl-1-inventory-before-deletion) only.
Sidecar `24e5a08`, Browser `3a3c043`, Bridge `3676f5c`; existing uncommitted
roadmap documents and Browser build/cache changes are preserved.

## Result and limits

The entry-point inventory, compatibility map, resource/effect map and
[candidate ledger](cl1-cleanup-candidates.md) are delivered. The ledger contains
32 reviewed boundaries: one `remove` proposal, five `extract`, 19
`retain-required`, three `defer-until-replacement` and four `unverified`.
The first proposed change is removal of the unused internal `DeleteMemoryItem`
forwarding alias; no removal has been applied or approved by this inventory.
That proposal was later separately approved and applied; see the
[CL-2 G10 result](cl1-cleanup-candidates.md#cl-2-g10-result-2026-11-01).
The metadata and source-preservation statements below remain the historical
CL-1 audit record, not a claim that the later code patch did not occur.

| Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- |
| Audit documents and metadata inventories produced; product code unchanged | Read-only dependency/package/reference checks; no product tests or compilation | Not applicable to the audit; no runtime accessed | CL-1 deliverables ready for review; CL-2 and all other implementation remain unapproved |

This is not a line-by-line semantic audit of every tracked function. Mechanical
keyword discovery remains a queue, with unknown external consumers and Browser
dependency gaps explicit. The audit does not prove that every legacy-named path
is removable or that every supported release has been installed successfully.

## Method and evidence

- Read tracked source and canonical build/package roots in all three repositories.
  Protected hashes cover 1,575 tracked non-Markdown files. Existing untracked
  Browser build/cache work is checked separately against its prior hashes.
- `go list` metadata, with Windows/amd64 and network lookup disabled, identifies
  source selection for default, development, fixture and combined tags. No
  application, observer, harness, installer or fixture executable was launched.
- Static import traversal follows the native builder's root/import rules and
  treats generated asset catalogs separately. It is not an execution/call graph.
- The existing Bridge package verifier is read-only (`writesArtifact=false`):
  it follows manifest, HTML, imports and registered-source scripts and identifies
  69 referenced package files. It did not create an extension package or run tests.
- A cross-repository tracked-source symbol census checks the proposed removal.
  Independent bounded Astra review covers Go durable compatibility and the alias.

Local machine-readable evidence, not committed product source:

| File under `runtime/qa/cl1-inventory-20261101/` | Purpose |
| --- | --- |
| `before.json` | Source HEADs, worktree state and protected source hashes |
| `go-entry-graphs.json` | Sidecar application dependency graphs for four tag selections |
| `go-auxiliary-roots.json` | Auxiliary Go executable and module source selection |
| `go-browser-dependency-gaps.json` | Offline unresolved Browser dependency closure |
| `js-entry-graphs.json` | Native/static web/Bridge import roots and recorded bundle comparisons |
| `bridge-package-references.json` | Read-only extension manifest/registered-asset reference closure |
| `keyword-census.json` | Complete mechanical keyword-hit queue for selected tracked source types |
| `candidate-ledger.json`, `alias-reference-census.json` | Reviewed dispositions and code-only references for the exact removal proposal |
| `validation.json` | Final source-preservation, reference and documentation checks |

These local evidence files may be absent in another checkout. Source references,
counts, limitations and dispositions remain recorded here; missing local files
must not be interpreted as newly passing checks.

## Go entry points and build selection

The primary dependency graph is `cmd/akusidecar/main.go`. Windows/amd64 metadata
selects 31 Sidecar-owned dependency packages for default, 34 for development,
32 for fixture and 34 for combined tags. These counts exclude standard-library
and third-party packages; they are not test or compilation results.

| Mode | Tags | Main source selection and consumers |
| --- | --- | --- |
| Default | None | `development_session_disabled.go`, `development_x_qa_disabled.go`, reader diagnostic/main/startup status; ordinary Store/Engine/HTTP path |
| Development | `auth_session_dev` | `development_session_unavailable.go`, Windows X QA entry, ordinary main; does not supply the combined fixture frontend |
| Fixture | `auth_session_fixture` | Disabled main-development selectors plus fixture-enabled packages; standalone fixture UI/process roots become selectable |
| Combined | `auth_session_dev auth_session_fixture` | Windows broker/frontend/lifetime/session-fixture selectors plus development X QA entry |

All four primary dependency queries completed without metadata errors. Two
standalone fixture executable roots are intentionally excluded in default/dev
metadata; this is an expected build constraint, not dead code or a compile failure.

| Executable/source root | Role and consumer | Selection/retention boundary |
| --- | --- | --- |
| `AkuSidecar/cmd/akusidecar` | Product service and development composition; Browser builders/launcher | Four selections above; default-off/native admission still enforced at runtime |
| `AkuSidecar/cmd/aku-reader-broker` | Installed/development reader activation broker | Explicitly built by Browser installed-app builder; not a child call to ordinary main |
| `AkuSidecar/cmd/semantic-replay` | Explicit offline replay CLI | Retain replay contract and evidence isolation; not a daily source collector |
| `AkuSidecar/cmd/auth-session-fixture-ui` | QA UI entry | `windows && auth_session_fixture`; not an ordinary product launch |
| `AkuSidecar/cmd/auth-session-process-fixture` | QA process-lifetime entry | Same fixture-only build boundary |
| `AkuSidecar/scripts/fixtures/sqlite-driver-compat` | Standalone compatibility harness | A supported test consumer, not a product source root to promote |
| `AkuSidecar/scripts/headless-native-observer` | Explicit Windows observer tool | Source selected on Windows; no observation executed by CL-1 |
| `AkuSidecar/scripts/quiet-authenticated-probe` | Explicit authenticated Windows probe | Selection is not permission to run it |
| `AkuBridge/native-host` | Native Messaging host | Separate Go module; protocol 1/2 and Windows/non-Windows source variants remain |
| `AkuBrowser/launcher/cmd/AkuBrowserLauncher` | Installed tuple launcher | Separate module; split capture and explicit single-process rollback |
| `AkuBrowser/installer/windows` | Installer executable | Separate module and platform files; upgrade/removal cleanup is active code |
| `AkuBrowser/installer/windows/cmd/sign-update-manifest` | Manifest signer CLI | Strict v1/v2 identities, signatures and compatibility fields |
| `AkuBrowser/scripts/generate-chrome-store-icons.go` | Standalone asset-generation source | Historical/tool consumer; not a product main binary or proof of active Store publication |

The Browser launcher/installer root metadata is available, but their full
transitive dependency closure is **unverified**: `golang.org/x/sys v0.19.0` is
not fully available in the selected cache and `GOPROXY=off` prevents lookup.
No dependency was fetched to resolve that gap. This limits claims about their
compile readiness, not the observed source consumers or Sidecar metadata.

## JavaScript, generated assets and event consumers

| Root | Static modules | Other consumers that prevent text-only deletion decisions |
| --- | ---: | --- |
| Native manual `application-owner.mjs` | 72 | Go-owned factory, stdio owner protocol, generated `packaged-assets.mjs`, 26 recorded capture assets |
| Native daily `daily-owner.mjs` | 73 | Daily owner protocol, source-bound controller/factory, generated catalog, 26 capture assets |
| Main `main-browser-transport.mjs` | 92 | Source registry, X/IG native composition and sealed main transport, 32 capture assets |
| General worker `worker.mjs` | 17 | Worker JSON protocol and four source asset recipes; not interchangeable with the three native owners |
| Bridge `service-worker-entry-v3.js` | 39 | Manifest registration generation, extension messages/events, popup, permission and placeholder HTML, source-script registration |
| Sidecar `web/app.js` | 37 | HTML script roots, DOM actions, polling, revision/epoch fences and module teardown |

The three native module sets match the recorded manual/daily/main package
manifests exactly; all contain zero `scripts/auth-session-fixture` imports.
This is source/package metadata, not newly built bundles or live acceptance.

Important non-static consumers:

- `scripts/build-x-native-owner.mjs` replaces `packaged-assets.mjs` with immutable
  source catalogs, embeds the timing-policy bytes and seals module hashes.
  The source catalog placeholder intentionally rejects direct unpackaged use.
- `worker.mjs:sourceAssets` selects shared adapter/runtime and source-specific
  Bridge/vendor files. Its two nonliteral `import()` expressions load declared
  media/feed resolver paths; imports from arbitrary filenames are not inferred
  safe or unused by this audit. Resolver functions are later injected into pages.
- `AkuBridge/manifest.json` names `service-worker-entry-v3.js`, popup and loopback
  content script roots. `source-access-policy.js` registers additional scripts
  for opted-in sources. Source-script metadata contains 16 distinct existing JS
  file references; their message/event registration is an executable consumer.
- The Bridge verifier's 69-file closure includes registered scripts and HTML
  resources, beyond the 39-module background import graph.
- Sidecar HTML loads `startup-watchdog.js`, `native-post-diagnostics.js` and
  `app.js`, with runtime query keys. `internal/httpapi/assets.go` embeds `web/*`;
  FileServer exposure is a separate consumer even for files not imported by app.
  Two direct `.test.mjs` files are therefore packaging-review candidates, not
  proven unused source or permission to change static-route behavior.

Browser packaging roots remain `build-windows-installed-app.ps1`, the preview
and companion-runtime builders, their macOS counterparts, launcher/installer
modules and `release/release-manifest.json`. Dynamic staging/embedded file lists
and environment-gated compatibility harnesses remain consumers. Frozen Store
publication does not establish the absence of installed v1 host/update clients.

## Retained compatibility contracts

| Contract | Required boundary / supporting source |
| --- | --- |
| Store schema 31 | Complete supported 7-to-31 migration registry; semantic 30-to-31 downgrade barrier; older unsupported/newer databases reject without implicit reset |
| Runtime data marker | `.runtime-version` prevents newer-runtime data from being opened by an older binary even at equal schema |
| Synthetic checkpoint/policy keys | Historical durable key names intentionally make older guard-aware binaries fail closed; opt-out is not key deletion |
| Reasoning config | Both modern provider map and flat provider shape remain accepted strictly; model/effort aliases still feed profile/calibration/provider code |
| Bridge v2 heartbeat inference | Missing major/minor on a valid v2 contract is inferred as 2.0; warning/degraded behavior has an explicit Engine regression |
| Native Messaging 1 and 2 | Host validates both; client has bounded v1 retry/reconcile translation; v1 must not gain v2 policy/capability fields |
| Candidate probe schemas 1 and 2 | Exact schema-1 response remains a tested host-consumer shape; schema 2 adds DB version |
| Launcher rollback | `--legacy-single-process` propagates through relaunch/runtime args and is explicitly retained by split graduation |
| Receipt parity | Identical Sidecar/Bridge cleanup predicates are separately packaged and compared in `capture-surface-policy.test.mjs` |
| Permanent Forget | HTTP -> Engine `ForgetLibraryItem` -> Store `ForgetMemory` -> transactional `DeleteMemory`; preserve tombstones, content scrubbing and recapture suppression |

## Locks, resources, effects and failure propagation

| Seam | Current authority/order | Extraction/removal constraint |
| --- | --- | --- |
| Config decode | Probe selects modern/flat shape; each strict decoder populates the same receiver | Do not normalize/remove a supported shape or change unknown-field behavior inside a move |
| `SaveSettings` | Holds `operation`; validate/plan candidate -> persist settings -> request backend/apply provider/executable effects -> reload/retention -> notify | Reuse `providerSwapPlan`; preserve candidate cleanup and errors after persistence; no invented atomic rollback |
| Session start/finalization | Start releases `operation` before `schedule`; finalization keeps `schedule` while calibration may acquire `operation` | Terminal gate and caller's deferred capture cleanup remain in place |
| Worker cancellation | `e.mu` protects tombstones/active/pending state; durable cancellation follows in-memory fencing; active admission retires before deferred worker cleanup finishes | Worker count remains nonzero until cleanup returns; late result/rerun cannot resurrect cancelled work |
| Native source batch | Source controller owns generation/cancel; owner verifies account/document/deadline; Store owns claim/result authority | No shared reservation, new seed, fallback authority or observer relaxation added by cleanup |
| Bridge surfaces | Managed journal/lease/generation and exact owned targets determine release; extension events are entry points | Empty or late receipts cannot authorize closing a successor or user reader |
| Package construction | Source-pinned module/catalog/timing bytes precede Go factory pinning | A source import move requires matching manifests/staging; never substitute mutable sibling files at runtime |
| Permanent Forget | Store transaction removes identifying content/provenance and records opaque tombstone; Engine performs existing follow-up effects | Removing a no-op alias must not alter the underlying transaction, reason scrubbing or HTTP behavior |

## Outstanding evidence, not automatic scope expansion

- Out-of-tree forks, installed config populations and historical host/update
  consumers are not enumerable from these checkouts. They block indiscriminate
  compatibility retirement, not every scoped removal of an unused internal alias.
- The two legacy capability projectors differ around `focusPolicyRevision`.
  Determine the supported consumer shape before consolidating; CL-1 records the
  discrepancy and does not label it a proven bug or change the protocol.
- Embedded web tests and retired-distribution build scripts need package/user
  contract decisions. Static graph absence does not settle them.
- Compatibility harnesses requiring staged source/environment inputs can skip
  in ordinary runs. Referencing those tests is not a fresh passing result.
- Browser dependency closure and preserved dirty build/cache work need their
  own bounded validation before a related code/package patch is accepted.

Proceed only with the [specific CL-2 proposal](cl1-cleanup-candidates.md#first-concrete-proposal-for-cl-2)
after approval. No product tests, compile, runtime inspection, configuration
change, trial, commit or push is performed as part of CL-1.
