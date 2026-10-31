# X and Instagram implementation convergence

Date: 2026-10-30

Status: shared-foundation convergence in progress; steps 1–3 of the current
completion plan are authorized for isolated implementation and offline tests.
Scope: reuse and complete existing integration, not a replacement architecture.
This document does not authorize runtime activation, session transfer, login
changes, database migration, or changes to Auto Update.

## Current completion plan

### IG evidence-source increment (2026-10-31)

The bounded bootstrap reader diagnostic passed without a collector or transfer.
The private acquisition is now extracted and connected to both owned roles in
the existing native provider; 196 Node tests, targeted Go suites and an isolated
development build passed. This preserves the
daily controller, Store, permission restoration and X recipe. A stored disabled
IG choice is not a restart persistence failure and must not be silently enabled.
See the [implementation contract and concrete native trial](instagram-bootstrap-daily-proof.md).
Native activation/derivation and repeated capture acceptance remain separate.

The consolidated [shared-foundation progress report](shared-capture-foundation-progress.md)
combines X-versus-IG lifecycle convergence with windowed-versus-native capture
parity. These are two views of the same effort, not a new roadmap reset.

1. Finish terminal batch transitions and Browser cleanup per lease, then prove
   repeated offline batches can progress. Native result fences stay unchanged.
2. Split Instagram bootstrap shape rejection into closed, value-free predicate
   categories before considering a different identity contract. This is not
   provider qualification or permission to issue another authenticated probe.
3. Reuse the stable base readiness decision and make native owner capabilities
   explicit. Test common decisions and intentional unsupported capabilities with
   the same offline fixtures; do not enable mutation or temporary targets outside
   the owner's proven guard scope.
4. After a separately approved handoff, verify daily repeated mixed-source
   operation, cancellation, restart, remembered consent and reader preservation.
5. Retire obsolete paths only after their replacement has demonstrated parity.

Steps 1–3 are the current authorized implementation slice. Live activation,
provider qualification and acceptance in step 4 remain separate gates. A probe
that runs successfully but rejects a candidate is diagnostic progress, not a
successful Instagram capture.

## Objective

Make daily mixed-source updates complete or terminate predictably. X and IG
must share lifecycle contracts without sharing mutable source authority. A
provider-local failure must not block an unrelated source unless a verified
shared-resource dependency makes continuation unsafe. Preserve user readers.

## Existing architecture and historical evidence

Review baseline: main commit `7b3acc6`. The working tree also contains
uncommitted diagnostics and unrelated changes.

The existing [shared core design](source-concurrency-core.md) already assigns
permission, admission, generations, cancellation, deadlines and cleanup to a
shared core. The [native provider contract](native-source-provider-contract.md)
explicitly records that X retains its compatible native path while IG uses the
new generic owner. Commit `d4134d2` introduced that provider and generic kernel;
`f7018b2` added shared bounded retry policy. Convergence is therefore unfinished
adoption and integration, not evidence that these abstractions never existed.

| Component | Existing implementation | Planned treatment |
| --- | --- | --- |
| Backend selection and collection ownership | `internal/collection/coordinator.go` | Reuse; no second scheduler or coordinator |
| Managed browser process and generations | `internal/captureruntime/manager.go` | Reuse; do not confuse process ownership with per-tab ownership |
| Source settings, permission and state | `internal/engine/source_daily.go`, `source_daily_runtime.go` | Reuse, with independent state per source |
| Retry classification | `internal/engine/source_daily_retry.go` | Reuse; verify mixed-source behavior |
| Generic paired evidence and Store integration | `internal/engine/source_native_daily.go` | Existing IG path; assess X compatibility before adoption |
| Native resource lifecycle | `internal/collection/headless/worker/native-pilot/native-source-kernel.mjs` | Extend only for demonstrated missing behavior; no parallel kernel |
| Native owner factory | `main-browser-transport.mjs`, `source-owner-registry.mjs` in the same directory | Preserve sealed selection; bridge X compatibility incrementally |
| X native implementation | `development_x_daily_owner_windows.go` and historical X driver | Keep proven behavior while extracting responsibilities; no wholesale rewrite |
| IG evidence implementation | `instagram-native-session.mjs` | Retain provider-specific validation; resolve HTTP 400 independently |
| Browser surface ownership | AkuBridge `surface-cleanup-journal.js` and `capture-window-runtime.js` | Reuse; reconcile source/run/lease ownership with native cleanup |
| UI release barrier | `internal/httpapi/web/capture-surface-release-barrier.js` | Align dispatch with the actual resource owner |
| Capture, reasoning, Timeline and Store | Existing shared pipeline | Preserve authority checks and output behavior |

Source-specific responsibilities remain: endpoint and document policy,
account/login/session evidence, cookie manifest, challenge detection, parser,
and media extraction. X bootstrap/settings evidence must not be replaced by
IG's current-user request merely to normalize code.

## Observed failure and evidence limits

On 2026-10-30, the 01:27 WIB user batch failed IG identity verification with
HTTP 400. LinkedIn's command remained unclaimed until batch cancellation at
01:31:58; X never started. The source-release barrier repeatedly rejected
cleanup. Some receipts reported zero expected/pending targets and complete=true
while their event outcome remained cleanup_pending. The exact stale owner/lease
still requires confirmation; zero targets alone must not bypass ownership checks.

The UI barrier currently applies to every completed/failed source without
distinguishing its capture resource owner. Command expiry in Store covers
claimed commands, leaving pre-claim waiting outside that particular timeout.
These are integration gaps to investigate and cover, not grounds to remove
identity or cleanup guards. Scheduler backoff is a downstream effect and must
not be disabled to hide these failures.

## Execution roadmap

### 1. Map contracts and establish behavioral baselines

- Inventory existing tests and reuse them before adding missing cases.
- Map ownership for process, profile, target, source, run, collector and receipt.
- Identify which component creates, releases and confirms each resource.
- Freeze X's successful ordering and failure behavior as comparison evidence.
- Define a common phase/result/error vocabulary around existing interfaces.

Exit: reviewed responsibility map and offline tests exposing current integration
gaps. Do not create another controller, owner registry or telemetry subsystem.

### 2. Repair cleanup and bounded dispatch integration

- Route release to the actual owner; native collector cleanup is not equivalent
  to a Browser-window cleanup request.
- Resolve stale lease reconciliation while retaining exact ownership checks.
- Ensure duplicate release is idempotent and distinguish source-local resource
  uncertainty from uncertainty in a shared browser resource.
- Bound pre-claim waiting; terminate the affected run with a precise reason.
- Continue independent sources only when their required resources are safe.

Exit: mixed IG/LinkedIn/X tests cover both source orders, native identity failure,
zero-resource cleanup, stale lease, delayed/unclaimed command, cancellation and
user-reader preservation. No indefinitely queued batch or unbounded cleanup loop.

### 3. Complete adoption of the existing native lifecycle

- Compare X owner/broker semantics with `sourceNativeOwner` and the native kernel.
- Add a compatibility adapter for X only where needed; keep its evidence recipe.
- Consolidate duplicated deadline, cancellation, late-resource cleanup and
  first-cause handling into existing components one responsibility at a time.
- Do not migrate a responsibility already correctly shared by the controller,
  Coordinator, Manager or Store.

Exit: X and IG pass the same lifecycle conformance suite, plus their distinct
evidence suites. A generic abstraction is accepted only if it preserves proven
X behavior; IG's newer implementation is not automatically the reference truth.

### 4. Complete provider contracts and IG qualification

- Keep cookie allowlists and account proof rules source-specific.
- Diagnose IG HTTP 400 using bounded evidence, not speculative header changes.
- Normalize proof and diagnostic envelopes; keep primary failure separate from
  cleanup failure and redact bodies, tokens and account identifiers.

Exit: offline contracts pass; IG's actual account/login recipe is separately
qualified in an explicitly authorized run. Code convergence is not live success.
Offline investigation may proceed alongside earlier stages when authorized.

### 5. Validate daily operation and retire obsolete paths

- Run offline and local synthetic mixed-source acceptance first.
- After explicit authorization, validate repeated batches, Auto Update, restart,
  remembered permissions, reader coexistence, cancellation and source isolation.
- Activate per provider with a concrete rollback plan compatible with the active
  Store schema; do not lower schema metadata or restore a database implicitly.
- Remove old implementations only after no production or test consumer remains
  and parity has been demonstrated. Preserve a single implementation per
  responsibility rather than permanent compatibility forks.

Exit: both sources capture successfully across repeated daily updates; failures
terminate predictably; temporary resources retire and user readers remain.

## Approval and progress tracking

| Stage | State | Approval / evidence |
| --- | --- | --- |
| Roadmap documentation and historical review | Complete | Requested by user on 2026-10-30 |
| 1: Contract map and baseline tests | Complete for the current integration boundary | Existing lifecycle suites reused; ownership projection and UI/Bridge parity added |
| 2: Cleanup and dispatch integration | Wrong-owner routing and pre-claim expiry committed; per-lease retirement and post-capture reasoning failure prepared and tested offline | Browser retirement does not mutate a newer lease; accepted result receipts permit negative-only reasoning termination after capture authority ends; live repeated-batch acceptance is pending |
| 3: Existing lifecycle convergence | Cleanup, cancellation and deadlines shared; common base readiness and explicit native capabilities implemented offline | Full controllers are not identical; guarded X quote navigation is disabled explicitly, with unsupported recovery diagnostics; source evidence remains distinct |
| 4: Provider completion | HTTP 400 unresolved; closed shape predicates implemented offline after passive aggregate shape rejection | Budget separation is active in diagnostic commit 7360d68; the new predicate projection is not deployed and does not qualify an identity source |
| 5: Daily acceptance and retirement | Pending for the current convergence slice | Earlier X acceptance and the diagnostic-only handoff are not repeated mixed-source acceptance; no obsolete path is retired by this slice |

Record completed evidence and remaining differences here as work proceeds. This
is the execution checklist; existing design documents remain the detailed
contract references. Runtime activation remains a separate approval.

### Current isolated completion slice

Completion-plan steps 1–3 have an offline implementation checkpoint in the
[shared-foundation report](shared-capture-foundation-progress.md#offline-completion-checkpoint).
This slice preserves the existing core instead of introducing another pipeline.
The next release must include the matching Bridge readiness primitive/assets,
Sidecar worker/owner code, Go diagnostic decoder and sealed Node transport.
Neither the new per-lease cleanup nor the predicate/capability changes are
active merely because the source tests pass.

## Implemented increment: lifecycle routing and dispatch recovery

- Run JSON now projects `captureCollector` from its immutable first command.
  The native daily controller records lifecycle ownership before reader checks,
  so a failure before command creation cannot be mistaken for a Browser run.
  This metadata grants neither capture permission nor proof of account identity.
- UI and Bridge source-release predicates exclude native/quiet collectors.
  Native-only terminal sessions do not request release of an unrelated Browser
  lease. UI dispatch also excludes commands owned by native collectors.
- Legacy and unknown routes remain conservative. Actual Browser lease mismatch,
  incomplete cleanup and browser-generation ambiguity remain blocking conditions.
  The original increment did not retire an old-generation record even after
  successful absence inventory. The current per-lease correction below replaces
  that rule without granting authority to close an old-generation live tab.
- Ordinary Browser commands have a two-minute pre-claim budget. Expiry rechecks
  route and queued status transactionally; a competing successful claim cannot
  be failed from an older queued snapshot. Ordinary public failure submissions
  still require a claimed command. Native and Quiet deadlines are unchanged.
- Recovery uses the existing scheduler loop as well as Bridge activity; it does
  not depend on the stalled UI dispatching again. A failed pre-claim run advances
  through the existing Engine pipeline, with `capture_dispatch_timeout` distinct
  from `capture_lease_expired`.

### Responsibility map retained

| Resource / decision | Owner | Consumers must not assume |
| --- | --- | --- |
| Persistent main browser and process generation | Existing capture Manager / application owner | Collector cleanup closes the main reader |
| Source authorization and batch cancel/join | Existing per-source daily controller | One source's status authorizes another |
| Native collector and temporary native targets | Source's native owner/broker | Browser UI owns these targets |
| Browser capture targets and placeholders | AkuBridge managed window runtime and journal | A source name or empty receipt proves lease ownership |
| Claim and result acceptance | Existing Store permit transactions | UI route metadata grants authority |
| Provider evidence | X or IG adapter | A common lifecycle implies common identity endpoints |
| Dispatch recovery | Existing Engine / Store / scheduler | A queued command has already acquired a capture lease |

### Offline verification

- Store tests cover route projection, pre-command native ownership, exact queue
  deadline, native/Quiet exclusion, public failure rejection, and claim/expiry race.
- Engine tests cover unclaimed Browser expiry advancing the next source.
- UI/Bridge consumer tests cover native source failure, native-only completion,
  native dispatch exclusion, conservative unknown routes, and parity decisions.
- Existing target lifetime, user reader protection, old-generation ambiguity,
  provider/registry cancellation and first-cause suites remain required.

This increment does not claim resolution of the current IG HTTP 400, retirement
of unknown historical Browser resources, or complete migration of the X native
owner. Those claims require their own evidence; none is inferred from offline
passing tests or from the shared controller already existing.

## Owner convergence increment: shared cleanup execution

The compatible X daily owner now delegates its cleanup sequence through
`settleNativeCleanup` in the existing `native-source-kernel.mjs`. Generic native
attempts, used by IG, use the same executor. There is no second lifecycle core.

The shared responsibility is ordered release, continuation after independent
release failures, and value-free outcomes. A diagnostic projector cannot stop
later releases. Ownership checks, resource registration, cleanup deadlines,
source evidence and receipt projection remain with their existing callers.
This is intentionally not a migration of X to the IG identity recipe.

X retains collector/enrollment/attachment/bootstrap order, control-restoration
status, bootstrap diagnostic categories and its existing Go wire receipt.
Generic attempts retain reverse resource order, bounded cleanup, late-resource
release obligations, idempotent close and first-cause preservation. No deadline
or authority check was removed or increased.

Offline evidence: 68 tests passed across X daily protocol, main browser
transport, IG provider, cleanup conformance and package sealing. Conformance
includes all 16 X release-failure combinations and both source bindings for
cancellation, late creation and first-cause retention. Packaging verifies that
the X daily owner includes the shared kernel without running the owner.

Remaining adoption work: compare and consolidate cancellation/deadline adapters
where X's qualified startup and seed lifecycle can preserve their exact guards.
The full X owner migration and live daily acceptance are not complete. IG HTTP
400 qualification remains a separate provider issue. No runtime, login, session
transfer or Auto Update change occurred in this increment.

## Owner convergence increment: cancellation and operation deadlines

`createNativeOperationScope` in the existing kernel now supplies fixed-deadline
waiting, cancellation, clock-regression rejection and late-result rejection to
both generic native attempts and the X application seed grant. Callers still
own diagnostic categories and the first-cause record; no common identity or
cookie recipe is introduced.

X retains its monotonic elapsed-time clock, wall-clock capture deadline ceiling,
one-use grant and maximum 60 seconds from seed start. IG retains its original
absolute attempt deadline and integer-clock validation. Each operation receives
only the remaining budget, never a renewed budget. Cleanup keeps its independent
existing allowance and resource ownership obligations.

The daily X owner revokes its grant before batch-end cleanup, explicit abort or
capture cancellation, and owner close. Cancellation also consumes a grant that
has not started. Pending seed waits reject; guarded late writes and late seed
success cannot regain authority. A fresh batch still needs a fresh grant and
the existing generation, ownership and account checks.

Cancellation stops waiting and revokes authority; it does not magically cancel
an in-flight CDP request. Existing owner cleanup and pre-write guards remain
responsible for resource retirement and write admission. Per-operation X RPC
timeouts, root-navigation controls and provider-specific proof code remain in
their qualified implementations; this increment does not claim their wholesale
migration or live acceptance.

Offline evidence: 78 tests passed across grant/kernel cancellation, exact
deadline and clock regression, late completion, X daily protocol, main transport,
IG provider, cleanup conformance and sealed package construction. Native browser
execution, session transfer, activation and runtime changes were not performed.

## Offline owner / Engine / Store acceptance

The following boundaries were verified after commit `09f4f33`. The adapters
and account evidence are synthetic; databases and Node actors are test-owned.
No native Chrome acceptance is implied by these results.

| Boundary | Evidence |
| --- | --- |
| X startup cancellation and late owner | Existing `TestXDailyStopJoinsStartupAndRejectsLateOwner`: stop waits for startup retirement; late owner cannot inherit authorization |
| Capture cancellation and reader preservation | Existing X/IG Stop suites; grant/kernel cancellation suites reject late work |
| Late native startup result | New `TestSourceNativeLateStartupCannotIssuePermit`: adapter ignores cancellation, but real native owner refuses admission and retains cleanup obligation |
| Late native capture result | New `TestSourceNativeLateCaptureCannotCommit`: cancellation or expired owner deadline after claim rejects returned observation; Store remains empty |
| Cleanup failure after capture rejection | New late-result test preserves the first cause separately from cleanup failure and rejects replay |
| Mixed-source continuation | Existing `TestDailySourceFailureDoesNotDisableOtherSource` covers both source orders, healthy-source completion, remembered permissions and unchanged Auto Update |
| Shared-resource cleanup uncertainty | Existing predecessor tests block continuation until owned collector cleanup is verified |
| Go / Node protocol | Existing accountless Node owner diagnostic and shared-protocol fixtures pass without launching Chrome |

Validation: selected tagged Engine and Store suites passed, plus 48 JavaScript
grant/kernel/protocol/transport tests. Tests exercise these boundaries in
complementary fixtures; they are not one live end-to-end browser run. No new
production behavior was required by this acceptance increment. The native-port
late-result fixture uses the IG generic owner; X remains covered by its own
compatible owner/controller and shared grant tests, not an assumed migration
to the generic IG proof adapter.

Next unresolved provider gate remains IG identity HTTP 400. Runtime activation
and bounded real-provider qualification require their own authorization.
