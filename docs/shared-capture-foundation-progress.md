# Shared capture foundation and completion status

Created: 2026-10-30. Status synchronized: 2026-11-01.

This report combines the X/Instagram lifecycle comparison and the comparison of
native headless capture with the established windowed Bridge path. The current
follow-up is documentation only. Earlier isolated implementations and trials
retain their own evidence; no new activation, session transfer, account probe,
user database or Auto Update change is authorized here.

## Current status by evidence dimension

| Scope | Implemented | Tested | Activated | Acceptance |
| --- | --- | --- | --- | --- |
| Shared lifecycle/dispatch foundation | Present in reviewed source; production helpers and Engine responsibilities separated through `ae1a535` | Existing offline mixed-source conformance and later regression/equivalence receipts | No activation receipt for the later cleanup commits | Offline boundary covered; repeated real mixed X/IG operation remains pending |
| IG evidence and diagnostics | Bootstrap verifier, continuous observer and History caller diagnostic in `32b8552` | Node/Go contracts and accountless History fixture | Recorded IG-only `32b8552` artifact with `sourceDirty=true`; not a clean-tuple claim | Rejected reader/historyApi/readiness document change; caller attribution absent; native IG unqualified |
| X daily path | Existing selected main-dev implementation | Historical lifecycle tests and later completed X-only repeat | Historical daily-MVP and later trial artifacts | Accepted selected X MVP remains valid; no full rotation or mixed IG acceptance inferred |

Exact commit/receipt mappings and limits are in the [status index](roadmap-status.md).
HTTP 400 below is the former IG endpoint's historical failure. It must not replace
the later bootstrap/document-continuity finding as the current diagnostic gate.

## Shared foundation

| Responsibility | Existing reuse | Remaining difference |
| --- | --- | --- |
| Scheduling and backend ownership | Engine, Coordinator and capture Manager | Browser commands and private native operations have different admission seams; no second scheduler is needed |
| Settings, permission and Store | Source-bound daily controller and guarded claim/result transactions | Reader/collector account proof and permits remain source-specific |
| Extraction and media | Bridge capture primitives, adapters and media resolvers are loaded by the worker | Controller timing and wrapper capabilities can still suppress behavior supported by those adapters |
| Native cleanup | X and IG use the existing kernel cleanup executor, commit 4e4ca99 | X retains its qualified owner/protocol compatibility adapter |
| Cancellation and deadline | Shared operation scope, commit 09f4f33 | Navigation, wire timeouts and provider evidence stay with their existing owners |
| Browser cleanup | Managed capture runtime and durable cleanup journal | Requested lease identity must be independent of the current managed-window lease |
| Readiness | Existing source assessment and login/challenge observations | Windowed and worker decisions must consume the same base rule rather than copy it |
| Capture capability | Existing worker and owned target ports | Native X does not authorize same-document expansion/freshness mutation or temporary detail pages; IG owns temporary media targets |
| Failure reporting | Closed diagnostic projections; primary cause separated from cleanup | IG bootstrap needs the exact rejected structural predicate, with no raw identity or body |

Windowed capture uses the browser's existing session directly. Native capture
derives a bounded subset into a disposable collector and must verify the pair.
Consequently DOM login readiness alone cannot replace native account proof.
Convergence means one implementation per shared responsibility, not identical
identity endpoints or authority for every source/backend.

## Observations and implementation status

Historical observations at the original 2026-10-30 checkpoint:

- Six recent LinkedIn Browser commands were never claimed and timed out after
  124–149 seconds. Stale-lease cleanup can prevent the pending-command query
  while heartbeat remains healthy. This is not evidence of native X/IG rendering
  failure; those sources were under maintenance holds during those batches.
- The per-lease Bridge patch is isolated and not activated. Its 118 offline
  tests cover retired A/current B, exact old-lease tab retirement, reader
  preservation, ambiguous ownership, old-generation absence and worker restart
  polling. This result does not prove a real repeated batch has succeeded.
- Captured reasoning failure has an existing local negative-only Store patch.
  It proves the accepted capture receipt before terminating a protected run
  after permit expiry/closure. The current slice reviews and validates that
  integration rather than weakening the generic capture fence.
- Diagnostic commit 7360d68 is active from the preceding authorized handoff.
  One passive IG observation returned modulePresent=true, reason=shape,
  scanComplete=false and budgetExpanded=false, with verified cleanup. This
  removes budget exhaustion as the cause of that observation, but does not
  identify the rejected field or establish account identity.
- The candidate current-user endpoint returned HTTP 400 even in a visible
  reader. Bootstrap discovery is a separate candidate source. Neither result
  establishes that headless rendering or the user's login is broken.

## Current authorized work and exit conditions

The heading is retained for existing links. The table records the original
completion slice; it does not renew implementation or operational authorization.

| Step | Offline exit condition | Separate operational gate |
| --- | --- | --- |
| 1. Predictable batch termination | Per-lease cleanup and accepted-capture reasoning failures preserve authority, evidence and independent source progression; repeated synthetic batches drain | Approved release/handoff and repeated real Browser/native batch acceptance |
| 2. Exact IG predicate diagnostics | Closed structural categories agree across Node/Go; malformed/unknown/private projections reject; proof remains unqualified | Approved single passive observation, source-contract review and independent reader/collector qualification |
| 3. Shared readiness and explicit capabilities | Existing common readiness decision is reused; parity fixtures cover ready/login/challenge/unavailable/pending and native capability restrictions | Real source behavior/quality comparison after authorized activation |
| 4. Daily acceptance | Not part of this implementation slice | Repeated updates, Auto Update, restart, cancel, saved consent and reader coexistence |
| 5. Obsolete-path retirement | Not part of this implementation slice | Demonstrated parity and absence of consumers before deletion |

## Offline completion checkpoint

This section records the original isolated offline checkpoint, before its later
integration and the separate diagnostic activations. At that time the source
was prepared in worktrees and was not committed or activated by that slice.
Do not carry that Git-state statement forward: current commits and tested
payloads are mapped in the status index. Neither later diagnostics nor source
cleanup establish complete real-provider convergence.

Step 1 reuses the per-lease Bridge correction and adds the reviewed Engine/Store
terminal-failure integration. Closed or expired daily capture authority cannot
block a negative-only reasoning failure backed by the original committed result
receipt. Synthetic X/IG orders preserve accepted evidence, allow the next source
and next batch, and release collection intent after worker drain. The existing
quota-stop policy remains unchanged; the tests do not change operator settings.

Step 2 supplies required closed shapeReason predicates across Node/Go. A later
malformed tuple clears flags from a previously valid candidate, remains
incomplete and keeps its exact structural category. No tuple schema, identity
type, Store authority or real-provider qualification was relaxed.

Step 3 reuses `sourceReadinessState` in the already shared Bridge primitives.
Windowed content probing and native X/IG snapshots call the same base decision.
Windowed adapter assessment/visual hydration and headless post/media settling
remain distinct; structural `feed_empty` is not valid-empty or account proof.
Conformance runs the actual content-script message handler and headless
extractor against the same fake page observations.

| Native owner capability | X | Instagram |
| --- | --- | --- |
| Content expansion / freshness mutation | Unsupported by requested-document guard | Existing attempt-owned capture behavior preserved |
| Quote click navigation | Unsupported; unresolved quote remains unresolved | Not exposed |
| Temporary detail/media target | Unsupported | Existing attempt-owned temporary media pages |
| Account and result checks | Existing X proofs and header guard | Existing pair verification before/after capture |

The X wrapper previously could still enter generic quote-click recovery despite
its one-document guard. Owner capability now prevents that route and reports
`coverage.quoteIdentityProbe.reason: owner_quote_navigation_unsupported`.
Recovery factories retain their existing unavailable/partial diagnostics rather
than silently presenting unsupported recovery as complete. These capability
declarations come from owners, not request payloads.

Evidence: 118 offline cleanup/reader/polling tests from the unchanged per-lease
increment; 84 combined diagnostic, transport, capability and sealed-package
tests; 14 readiness/extraction tests; targeted native X/IG, recovery and asset
suites; selected tagged Go Engine/Store/diagnostic tests and default-build
compatibility checks. Suite counts overlap and must not be added as unique
tests. Bridge package verification succeeds without writing an artifact.

The closed observational `sourceReadinessState` is retained in successful
coverage and local empty-capture diagnostics. Missing, unknown, object-valued or
accessor-backed inputs are omitted, preserving the previous JSON when absent.
It is distinct from admission's sourceReadiness/account proof. Native X's private
failure DTO remains unchanged and does not carry this additional classification;
the Node projection still selects its fixed fields and Go rejects unknown ones.

The original next operational gate required a reviewed paired Sidecar/Bridge
package and approved graceful handoff. Later diagnostics do not certify the
full repeated real-batch gate. The matching Go
binary and Node transport must agree on the required shapeReason projection.

Capabilities that are intentionally unsupported must stay visible in the
contract and tests. A native X guard currently authorizes the requested
navigation, not arbitrary same-document actions or additional targets. Removing
those restrictions requires a corresponding owned capability and validation;
sharing readiness code alone does not authorize it. No new parser, controller,
registry, reservation system or telemetry library is needed for this slice.

Progress must distinguish source implementation, offline tests, packaged
artifacts, activated runtime and real-provider acceptance. The absence of a
qualified IG verifier remains an operational limitation after offline completion.
