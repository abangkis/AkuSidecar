# X concurrency MVP-2 integration design

Status: the first offline synthetic controller increment was reviewed and
published as `e87c1b27e730b61d0d4c99a819b43035332a40d8`, with origin/main
verified. On 2026-10-21 the user additionally authorized default-off
main/broker/Coordinator wiring and integrated synthetic testing. That increment
is implemented with in-memory owners and admission; see its validation checkpoint
below. Native observation/session transfer, database access, Store
modification, installation and user-runtime operations remain excluded. Proposed
native integration surfaces below remain unavailable. This document does not
itself grant execution permission.

## Accepted baseline and immediate outcome

Commit `2fbf338bf0258140f56cb1da567a1145834c5fd8` publishes the reviewed
16-file synchronized-QA delta; origin/main was verified. Reuse native run
`90f3e12182b2494ba7933511ac3150a5`: one exact two-cookie seed, four fresh
same-account/login checkpoints, 447.68 ms of protected-request overlap, preserved
reader target/window and verified complete owned cleanup. No second login was
needed. Its preflight passed 184 Node and 14 Go top-level tests.

This closes the short feasibility milestone inside MVP-1. It does not qualify
durable continuity, refresh, all worker/startup coverage, production enrollment,
native provider revisions or Store admission. Those dimensions remain explicit;
do not repeat the accepted login/source/tuple/overlap work merely to advance the
integration. The three macro stages remain MVP-1, MVP-2 and MVP-3.

MVP-2's immediate deliverable is a separate, default-off controller and routing
contract: one X reader owner and one true-headless collector owner can coexist.
Opening a reader while a verified collector is working must not pause its batch,
release its leases, change its generation, replace it or close user windows.
The first implementation proposal uses invented identities and an in-memory
admission/sink harness, with no Store access. Native queue writes stay unavailable.

## Current seams and selected topology

The current Engine has one capture manager; attaching a second is rejected.
Coordinator reconciles that manager between drivers. The tagged exclusive hooks
pause collection before reader use and resume after handoff. They remain intact
as a separately selected fallback; they are not the concurrent implementation.

| Existing seam | Proposed concurrent use |
| --- | --- |
| `internal/engine/capture_runtime.go`: `AttachCaptureRuntime`, owner stamps and result validation | Keep `Engine.captureOwner` dedicated to the headless collector. Preserve its existing epoch/generation/driver and durable checks. |
| `internal/collection/coordinator.go` and `exclusive_session.go` | Keep batch reconciliation on the collector manager. Do not invoke exclusive hooks for a concurrent reader open. Reject simultaneous binding of both strategies. |
| `internal/captureruntime/manager.go` and `profile_lease.go` | Reuse per-process lifecycle and generation guards, with distinct physical-profile ownership. One manager must never adopt both profiles. |
| Tagged reader/broker dispatch | Route concurrent native-post opens to a separate reader owner through the new controller. Reader responses cannot enter capture-result ingestion. |
| `internal/engine/development_session.go` and `development_policy_controller.go` | Preserve the single-profile synthetic contract. A new pair-proof interface must not impersonate it or silently widen its provider/domain. |

Use a small proposed `ConcurrentSessionController` with an owner-pair registry,
not a second Engine or a second batch queue. Coordinator continues to own one
collector manager; the controller separately owns reader lifecycle and routing.
This avoids making reader activity look like a capture-driver replacement or
changing persisted queue stamps just to open a post.

```mermaid
flowchart TD
    Start[Explicit tagged QA opt-in] --> Gate[Concurrent controller: admission closed]
    Gate --> Pair[Validate distinct owner/profile bindings and fresh pair proof]
    Pair --> Admission[Admission port]
    Admission -->|synthetic permit only in first implementation| Batch[Coordinator + headless Manager]
    Batch --> Sink[In-memory synthetic capture sink]
    Open[Existing explicit native-post action] --> Router[Reader router]
    Router --> Reader[Separate reader owner and user windows]
    Pair -. owns and observes .-> Reader
    Pair -. owns and observes .-> Batch
    Native[Native X QA observations] --> Unsupported[No Store receipt: native policy/revision unsupported]
```

Both browser trees remain children of Sidecar's lifetime, under kill-on-close
ownership. A future runtime backend must support closing/verifying only the
collector tree while reader windows remain, as well as killing/verifying the
entire owned Job when Sidecar exits. Root-process exit alone is insufficient.
Reuse the existing process-close contract, but do not claim the standalone QA's
whole-Job result proves selective runtime cleanup. Failure to establish the
required ownership/cleanup capability keeps the new path unavailable.

## Private pair contract

The proposed pair registry holds the following private values, never public UI,
HTTP status, logs or caller-supplied attestation:

| Dimension | Required binding |
| --- | --- |
| Scope | Provider/version, source X, expected enrolled account reference, attempt ID, controller epoch and monotonic pair generation |
| Reader | Physical profile identity and directory identity, role, PID plus process creation identity, parent ownership, reader generation and tracked windows/targets |
| Collector | A different physical profile identity, disposable context, PID/process identity, parent ownership, headless manager generation and driver |
| Proof | Exact per-role challenge and scope, fresh account/login observation, private same-account comparison, source result and coverage/continuity status |
| Admission | A separate permit authority with explicit capability/result; observation, human confirmation and Chrome readiness are not permits |

Attempt IDs, epochs and pair generations are local fencing identifiers, not
authentication/session lineage revisions. No cookie hash, timestamp, successful
HTTP response or ordinary UUID may be substituted for `Revision`.

For the first synthetic implementation, an invented verifier may provide a
synthetic revision to the in-memory admission port. The native QA adapter must
retain unknown continuity and return unsupported for Store/native authorization.
The first synthetic harness handles no real account ID or cookie value. For a
future separately authorized native adapter, those values remain inside its
trusted private boundary; only fixed comparisons/diagnostics leave it. Do not
change the existing enrollment identity representation in this stage.

The transfer boundary remains exactly the reviewed reader-to-headless manifest
in [the experiment plan](x-concurrency-mvp-experiment.md#reviewable-one-way-transfer-ceiling).
One seed per disposable collector context; no reverse merge, profile copy,
cookie database read, expanded cookie list or automatic second login. A restart
gets a new attempt and no inherited permit. Reusing a valid login does not imply
reusing authorization. Future native runs require their own explicit scope and
permission; the prior one-repeat authorization has been consumed.

## Default-off and startup sequence

The proposed first executable surface is a dual-tag Windows fixture entrypoint
(`auth_session_dev` and `auth_session_fixture`), with `config.Dev`, a private
owner/controller grant, a new explicitly selected synthetic-concurrency provider
and isolated fixture paths. Its eventual option name must be documented when
implemented. Dispatch must precede ordinary config, saved database, real provider
and app-shell startup, as existing fixture entrypoints do.

Ordinary builds must reject that option; tagged builds without all grants reject
it. Default startup performs no enrollment, profile lookup, Chrome launch,
credential read, guard migration or configuration write for this feature. No
setting, HTTP/native IPC toggle, environment-only switch or saved opt-in can
activate native concurrency. The new strategy is mutually exclusive with the
existing exclusive binding. Reject mixed-source configuration for the X-only
pilot; do not rewrite active sources to make it fit.

Proposed first-start order:

1. Validate the explicit isolated synthetic grant and choose the concurrent
   strategy. Close the in-memory admission port before creating any owner.
2. Acquire distinct profile leases. Establish Sidecar/Job ownership before
   permitting Chrome work; reject path aliases, replaced directories or foreign
   ownership. Create the collector empty, verify its process image/ownership,
   then perform the allowed synthetic seed.
3. Obtain independent, correctly challenged per-role evidence and compare the
   same expected account privately. A saved identity or ready bridge is not proof.
4. Bind the collector manager and reader route once. Open synthetic admission
   only if the separate synthetic permit port agrees. Attach exit/revocation
   callbacks before exposing work. Native authorization remains unsupported.

The current verifier has a five-second evidence-age ceiling and a two-second
binding deadline. The standalone experiment's total duration does not establish
compatibility with that binding deadline. Do not enlarge those limits or replay
an old QA report to fit them. Native integration must separately demonstrate a
fresh challenge/consumption path within the applicable limits before admission.

## Reader opens, claims and failures

| Event | Selected behavior |
| --- | --- |
| Open an X reader during active collection | Validate the existing post-open request and obtain a reader-only lease. Reuse/create its tracked window. Do not touch collector generation, batch admission or active leases. |
| Capture completes while reader remains open | Release only the capture lease. Reader windows retain their own owner/profile lease. Count every window, including bootstrap windows. |
| Another reader window opens/closes | Update the owned reader inventory. Closing one tab is not proof that its window or profile owner exited. |
| Last reader closes naturally | Retire the reader role and verify its full tree/lease release. Do not replace the collector. Continue collection only if the provider's collector authority remains valid independently; otherwise close admission and retain queued work. Native continuity for this case is not qualified by short QA. Never reopen a window or log in automatically. |
| Account mismatch, logout, challenge, proof expiry, required coverage loss or owner/context change | Close admission, increment the pair fence, revoke permits and reject late results. Preserve unstarted work. Classify the actual failure; unknown source data is not logged out. |
| Collector dies | Close admission and reject its generation. Keep user reader windows. No implicit reseed, reverse transfer or switch to exclusive mode. |
| Reader dies unexpectedly | Revoke any authority depending on that reader; collector activity cannot silently prove its own lineage. Keep batch work pending until the separate permit authority can establish a permitted continuation. |
| Sidecar dies | The owned Job terminates all helpers/readers/collectors. No child outlives the parent. Restart begins closed and re-verifies; it does not necessarily require login again. |

Keep the collector's existing epoch/generation/driver stamps. Associate each
synthetic command with its current pair generation in the private controller;
reader IDs cannot masquerade as capture IDs. Recheck the pair at dispatch and
before accepting results. The in-memory harness can test that race atomically
with its sink, but this does not replace Store's transactional fence or prove
atomic durable ingestion. Native writes stay closed until that exact integration
contract is reviewed. Do not change command rows, reassign pending work to another
account, revive cancellation or accept stale/duplicate results.

Do not use the QA rendezvous in ordinary collection: it is a measurement tool,
not a required pause before every reader open. No background campaign, periodic
login or automatic retry is part of this design.

## Store/provider compatibility and rollback boundary

The current development capability, challenge and receipt use one `ProfileID`.
The documented policy format accepts provider `synthetic-fixture`; it requires
accepted revisions and fresh owner-bound proof. Two physical profiles cannot be
represented as that same profile merely because their account IDs match. The
new controller must not call existing synthetic authorization with native values
or manufacture a native revision from the experiment.

Selected first step: keep the entire pair-controller exercise outside Store,
using the in-memory admission/sink port. This is runnable synthetic controller
qualification, not queue persistence or main-dev activation. No Store source,
schema, policy, API, database or guard key changes are proposed for that increment.

Before real ingestion, a separate compatibility decision must specify the exact
mapping of authority enrollment and physical collector proof, native provider
version and accepted revision semantics. It must determine whether existing
guard-aware APIs can express that mapping truthfully or require an explicitly
approved versioned extension. This design does not assume that no Store change
will ever be needed, nor authorize one now. Cookie-writer qualification is not
a prerequisite for designing the controller, but unknown lineage cannot satisfy
the unchanged production permit gate.

Opt-out closes admission and revokes the pair first, drains/cancels active work
according to its existing ownership rules, then closes and verifies only the
collector tree. Preserve existing user reader windows until the user closes them
or Sidecar exits. Report disabled-with-reader-retained separately from complete
cleanup; do not start an exclusive owner against a profile still held by a reader.
Unverified cleanup blocks replacement and disposable-profile deletion.

Real-provider rollback must retain guard metadata and queued work, reject older
unaware builds, and never delete required markers to make collection resume.
The exact future format needs its guard-aware upgrade/downgrade matrix before
native activation. No rollback, migration or runtime opt-out is executed here.

## First implementation and acceptance boundary

The first implementation is an offline synthetic pair controller with fake
processes, verifier, reader router and in-memory capture sink. Its private tagged
source is `internal/engine/development_concurrent_session.go`, with a separate
regression test file. No existing single-profile contract is modified to accept
two profiles.

Required behavioral cases are: default/unsupported mode has zero side effects;
distinct profile ownership; admission closed before startup; reader opens during
an active synthetic capture without replacement/pause; captures finish without
closing readers; bootstrap/multiple-window accounting; stale owner/pair results
rejected; mismatch/challenge/unknown proof closes admission; last-reader-close
follows the provider capability; collector failure preserves reader; opt-out
retains windows; parent death leaves no child; failed cleanup blocks replacement.
Reuse existing lease/queue/Job evidence where unchanged, but do not claim a fake
backend tests Chrome locking, foreground behavior or durable Store transactions.

The controller qualified and the user subsequently authorized tagged
main/broker/Coordinator wiring and integrated synthetic acceptance. Any use of a
disposable database, Store interaction or new native transfer still requires an
explicit scoped approval; none is inferred from that authorization. The native provider/metadata
compatibility decision remains a release gate, not a reason to repeat completed
MVP-1 experiments. MVP-3 still owns focused integrated acceptance and separately
approved main-dev activation. No calendar estimate is implied by these milestones.

## Offline controller implementation checkpoint (2026-10-21)

The authorized increment contains only private synthetic controller/test sources
under both `auth_session_dev` and `auth_session_fixture`, plus the local regression
helper `scripts/test-concurrency-controller.ps1`. There is no new main option,
HTTP/native activation endpoint, environment switch or production binding. The
first harness uses invented values, fake owner/process/verifier ports, memory
work state and a memory result sink. No browser/profile/database is opened.

The helper checks source inclusion in ordinary, development-only, fixture-only
and dual-tag builds. It then runs named-file tests for just the new controller
and its test file, deliberately excluding other Engine/Store tests and package
initializers. Named-file testing alone would not establish build-tag exclusion;
the separate four-mode source inspection supplies that check. Build artifacts
stay in a unique project-local QA root, with a retained synthetic log and guarded
temporary-root cleanup. There is no native-run mode in this helper.

Accepted offline run `106377b1916348b3b40b0b81c32f1322` passed all four tag
gates, 14 top-level tests and 34 subtests. The retained evidence is
`runtime/qa/auth-session-sidecar/concurrent-controller-106377b1916348b3b40b0b81c32f1322.log.txt`;
the unique temporary build root was removed. Coverage includes reader opens
during active work, bootstrap/multiple-window ownership, fresh independent
collector continuation, proof/challenge/account/revision rejection, owner and
cross-controller ticket fencing, duplicate/cancelled/stale results, collector
loss, opt-out, parent cancellation during a blocked probe, foreign-owner safety
and cleanup failure. Pending permit revocation blocks continuation; late failure
atomically fences admission and active results. Negative cleanup cases explicitly
expect unverified cleanup instead of accepting an uncertain owner as absent.

These tests qualify the new in-memory state machine and lifecycle contract.
They cannot qualify actual Manager/Coordinator/main wiring, Windows Job behavior,
Chrome profile locking, foreground UI, durable queue transactions, real provider
revision semantics or native authorization. Existing accepted X overlap evidence
is unchanged and must not be rerun merely for this increment.

This increment has no automatic proof refresh, collector replacement or reader
reopening after its owner has retired. Ticket issuance, claims, reader opens and
result acceptance check proof freshness; expiry holds admission rather than
refreshing credentials. Last-reader closure fences active work before retirement;
only the explicit synthetic independent-collector capability plus a fresh proof
and permit can reopen collection. It does not establish native session continuity.
Parent cancellation exercises the fake ownership contract, not actual process
termination. Queued work is retained only in memory, without restart durability.

## Main, broker and Coordinator synthetic wiring (2026-10-21)

The next authorized increment adds the exact provider
`--dev --dev-session-reuse=synthetic-concurrent-fixture` to the existing early
Windows fixture dispatch. It requires both `auth_session_dev` and
`auth_session_fixture`; ordinary and single-tag executables reject the provider.
The dispatch runs before `config.Load`, Store initialization, provider startup
and app-shell launch. The strict, size-limited input selects only built-in
synthetic scenarios. Its fixture grant is an explicit synthetic selector, not a
native credential or an account/session attestation. No environment-only switch
or saved setting activates this path.

The Engine facade reuses the private controller with two actual capture Managers
backed by invented in-memory Processes. Coordinator's new route defaults to nil;
its binding API exists only in the dual-tag fixture. The separate reader Manager
supplies native-reader leases while the collector's Manager and generation stay
fixed. Concurrent binding rejects exclusive/development strategies, browser
collection and non-X source selection. A blocked synthetic CaptureBackend and
memory sink exercise the dispatch/result boundary without Store access.

The tagged broker adapter reuses the existing bounded frame and Request
validation, then calls the synthetic reader route. Its reply contains only a
synthetic acceptance flag. It does not expose a native target or claim foreground
activation, and opens no OS pipe, native messaging host, registry key or listener.
The invented X post URL is validated as data and is never navigated.

`scripts/test-concurrency-integration.ps1` builds actual main executables in four
tag modes under a unique project-local QA root. Explicit rejection cases and
positive synthetic scenarios use missing configuration/database sentinel paths;
those paths must stay absent. This tests early executable dispatch and real
Coordinator/Manager code with fake owners. It cannot establish Chrome behavior,
OS child/Job termination, durable ingestion or native provider authority. Source
publication and these synthetic tests do not activate the user's dev runtime.

Run the integration helper from the repository root:

```powershell
.\scripts\test-concurrency-integration.ps1
```

The helper executes built-in scenarios for active capture/reader overlap,
bootstrap and multiple-window accounting, fresh independent continuation and
dependent denial after last close, expired proof, cancellation, revocation,
parent cancellation, opt-out, collector exit and strategy/broker rejection. It
checks that a parent cancellation reaches the blocked backend, and that the real
Manager exit observer fences an unexpectedly exited fake collector. Success is
emitted only after capture/reconciliation drain and verification of fake process,
window, owner and lease cleanup. No model result is promoted to native authority.

Accepted integrated run `e7e4f5da31f442a59fcf09ce3c00a2a0` passed four actual-main
build modes, 11 early-dispatch rejection cases, 11 synthetic scenarios, the
absent-config/database sentinel check and three focused ordinary Coordinator
regressions. The retained log is
`runtime/qa/auth-session-sidecar/concurrent-integration-e7e4f5da31f442a59fcf09ce3c00a2a0.log.txt`;
the owned temporary executable/build root was removed. The unchanged controller
source and controller-only helper retain their earlier accepted evidence.

Both strategy bind directions and the legacy headless-backend setter/alias reject
mutation after concurrent binding. The next ownership-backend increment must
qualify actual Chrome behavior on this routing without treating the fake PID or
Done event as OS tree cleanup. Native provider/profile/revision compatibility
still needs its separate decision before Store-backed or real-session ingestion.
