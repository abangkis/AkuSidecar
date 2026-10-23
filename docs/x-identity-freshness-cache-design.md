# X identity freshness and cache authority

Status: implemented as a bounded in-memory authority in the Manual V2 fixture
observation/driver/owner path. No new acceptance policy, recovery command, or
freshness refresh is activated. The implementation preserves the `d33bfdb`
acceptance envelope and the preceding root diagnostics.

## Problem and existing contract

Identity read time, account identity provenance, authenticated settings evidence,
and browser cache controls describe different facts. A recent read of a cached
bootstrap value does not establish a fresh server identity. A successful cache
disable command is an acknowledged request, not proof that a particular response
was uncached. Fresh settings evidence alone must not silently certify the age or
account identity of a separate bootstrap source.

The current manual pilot still requires both independently bound roles and their
joint account match. Its existing five-second identity/settings, two-second joint,
and sixty-second pilot limits remain unchanged. The `691875d` root guard accepts
absent or explicitly undefined cache attributes and false attributes, and rejects
all other present values. Settings evidence separately requires all three cache
flags to be false. These are different existing rules, not an interchangeable
definition of freshness. Earlier `284d5e4` did not reject cache flags on a
nonredirect root response; the full header/evidence separation also strengthened
that root policy. Whether each root cache class meets the intended identity
freshness invariant requires explicit policy review, not a diagnostic exception.

## Implemented owner-local component

`IdentityFreshnessLedger` is a pure, bounded, synchronous authority owned by one
manual observation generation. It owns no browser, credentials, timers, storage,
network client, or process. Its immutable policy has a stable version, explicit
allowed source/cache classes, monotonic age limits, and requirements for source
proof. A policy change creates a new generation and requires separate approval
before production wiring. Reading cannot upgrade an observation.

Private binding includes owner, profile, session, target, root frame, document
loader, default execution context, role, and generation. Exact values remain
private and never appear in diagnostic projections. Reuse across restart, owner
replacement, document replacement, or peer role is forbidden. No partial tuple
can acquire authority through a later unrelated event.

Creation initially pins the owner/session/target/role generation. Before the new
document is known, bounded provisional observations may record cache-control
receipts and the explicitly armed root request, but remain nonauthorizing.
`bindDocument` installs the document/context tuple already validated by the bound
observer. `observeRoot` consumes only the same-generation guard receipt after its
finish validation. Only an armed, accepted root produces `root_bound`; an unarmed
legacy observation remains `document_bound` even when accepted under existing
policy. Orphan, foreign, reused, or previous-document evidence cannot be adopted.
The ledger does not independently discover or guess the document relationship.

Implemented API (revision and generation are required for every mutation):

| Operation | Contract |
| --- | --- |
| `new IdentityFreshnessLedger(binding, now, guard)` | Create an empty generation under the fixed exported policy. Validate private binding and bind the guard exactly once. |
| `bindDocument` / `observeIdentity` / `observeRoot` / `observeCacheControl` | Accept bounded facts from their existing sole owners; reject stale revisions, foreign generations, and contradictory source tuples. Never execute effects. |
| `read(now)` | Return an immutable age-adjusted view; no refresh, mutation, or implicit command. |
| `readRootPolicyVerdict(now)` | Return `accepted_under_policy`, `rejected`, or `unknown`, separately from freshness proof. Acceptance under a permissive cache class does not mean server freshness is proven. |
| `invalidate(revision, generation, reason, now)` | Record a sticky generation-local loss; later success cannot repair it. |
| `update(revision, generation, evidence, now)` | Atomically publish a new evidence snapshot only after policy-required source proof and exact binding validate; preserve observation time separately from read time. |

An update never overwrites evidence piecemeal: identity, source proof, cache
observations, age anchors, and policy version form one immutable revision. A
caller supplies a monotonic clock; reversal or incomparable clock epochs produce
unknown/invalid evidence, never a fresh zero age. Wall time is explanatory only.
No identity refresh can reuse a prior generation's proof.

A read projection explicitly separates `{state, policyVersion, revision,
rootPolicyVerdict, readyUnderPolicy, sourceFreshnessProven, sampleAge,
identityCaptureAge, missingProof}`. Ages can be unknown; booleans with insufficient
evidence are unknown rather than false or zero. `readyUnderPolicy` states only that
the declared policy's requirements passed. In particular, acceptance of absent
cache flags under the current root rule does not set `sourceFreshnessProven`.

## Independent state dimensions

1. Cache control: requested command, request generation, acknowledgment state,
   and acknowledgment time. Requested, acknowledged, failed, and unavailable
   stay distinct.
2. Observed response cache: disk, service worker, and prefetch each retain false,
   true, absent, or malformed. Present undefined is represented separately in the
   private observation if useful, while the current root policy treats it like
   absence. A getter is malformed and is never invoked.
3. Identity: private account identity, exact source/document/context binding,
   capture time, source freshness proof, sample/read time, and policy verdict.
   A young sample does not reset the capture age or manufacture server freshness.
4. Availability: `unknown`, `stale`, `expired`, `excluded`, `invalidated`, and
   `ready` are explicit states. `ready` requires every policy-required fact; an
   allowed but unproven source remains distinct from proven freshness.

Header safety remains independent: observed candidate/ambiguous headers retain
their existing guards; ordinary headers cannot invalidate identity merely because
their request is ineligible for account evidence. Conversely, cache metadata or
an ordinary header cannot qualify, repair, or select an account source. Shared
owner/session/transport/inspection coverage loss still invalidates the owning
observation. Two-role rendezvous consumes two ready, independently bound snapshots
under compatible policies and the existing strict joint match.

## Explicit effects and future integration

Any later reload, cache-control command, cache clear, service-worker bypass,
login, network read, or retry must carry the applicable owner/run capability and
execute through the existing owner command gate. Existing authorization may cover
bounded internal commands; this design adds no per-command human approval flow.
It grants no native QA, transfer, main-profile login, or user-runtime permission.
The ledger may return a closed explanation of missing
proof, but cannot schedule or perform recovery. A successful effect only produces
an observation to validate; it never directly sets `ready`.

Before changing policy: agree on root/identity freshness classes, map each
affected consumer to a policy verdict, test contradictory and out-of-order
observations, expiry and clock reversal, stale revision updates, restart isolation,
both-role account mismatch, and absence of implicit effects. Shadow comparison
must establish parity before any deliberately versioned acceptance change.

The implemented integration map has one authority per fact: the driver supplies cache-control
command receipts; the root guard supplies selected root observations; the bound
observation supplies identity and settings evidence; the existing pair controller
consumes the unchanged private receipts and enforces joint matching. The ledger consolidates policy
evaluation without duplicating command ownership or account selection. It remains
in-memory only: no Store writes, persisted credentials, cross-run cache, or restart
inheritance are part of this implementation.

## Root diagnostic implementation in this change

The existing V7 wire keeps its nineteen numeric fields and eighty-character
limit. Closed conditions are appended to the Node and Go catalogs; V2–V7 decoding
and earlier indices remain intact. One first-failed predicate is retained in
evaluation order, with its original epoch, request-frame projection, and the
response's three cache projections when a root response fails, or the already
observed closed cache snapshot for a later root failure. Conditions name
request, response, commit, or ExtraInfo explicitly. A response-frame condition
does not reinterpret `guardRequestFrame`, which remains request-time metadata.

Coverage includes arm preconditions; root request method/URL/loader/history,
redirect and reuse; response duplicate/frame/loader/URL/status/ExtraInfo/cache;
commit duplicate/loader/URL/parent; ExtraInfo duplicate/status/declaration;
loading failure/cache event; and missing root/response/commit/ExtraInfo at finish.
First-cause ordering also preserves a prior header safety or coverage failure.
Arm failures retain their legacy failure domain. Diagnostic instrumentation does
not turn an unselected event into root evidence.

This is a bounded first-failure snapshot, not an event log or a list of every
failed term. Cache fields describe the offending response or the already
observed root response; they remain unknown when
no response was inspected. A loading-failed condition does not retain browser
error text. No raw URLs, request IDs, cookies, account IDs, or accessor results
are emitted. Cache controls and identity age are not inferred from this trace.
The historical native failure remains unresolved because its old trace discarded
the specific predicate; instrumentation cannot reconstruct it retroactively.

## Integration and verification

The bound observer replaces its Manual V2 five-second calculation with the ledger
verdict; ordinary/deferred QA retains its existing calculation. It supplies the
first valid identity capture, selected settings request and validated login proof,
and the completed guard receipt as one publication decision. Root cache attributes
still allow absent/undefined values while settings still requires three false
flags. Unarmed fixture compatibility remains explicit rather than claiming an
armed root proof. Source freshness remains unknown even for an accepted root.

The real driver records cache-control request and acknowledgment times without
issuing extra commands. Imported control times can precede ledger creation; they
are not fabricated as observation-start times. Root response observation time and
ledger receipt time are separate. An unknown timestamp stays unknown.

The existing private startup sample is branded in a WeakMap only after ledger
readiness. Production owned startup actors require and consume that one-use brand,
matching role and pinned scope before forwarding the unchanged sample to the
existing two-second challenge/source verifier and Go controller. Copies, JSON
roundtrips, cross-role samples, replay, and invalidated ledger generations cannot
provide that capability. Offline generic owner fixtures retain their explicit
non-production path. No policy or identity fields are added to the wire.

Reads are pure: age is evaluated against the supplied monotonic time without
advancing revision, capturing identity again, or refreshing evidence. Sample age
means time since the last atomic evidence update; identity capture age remains
time since the first valid capture. Cache-control observations cannot reset sample
age. A read earlier than the last observed mutation returns invalidated evidence;
clock reversal observed by a mutation is sticky. The bound observer remains the
monotonic event-clock owner and invalidates on reversal, including read boundaries.

Unit and real-binder tests cover atomic/descriptor validation, stale revisions,
generation/role isolation, root ordering, strict settings, expiry, clock reversal,
private owner publication gates, and no authority from cache acknowledgment. A
12-case comparison against immutable d33bfdb matches complete public reports and
publication presence. Existing ten-second observation, five-second evidence,
two-second source, and sixty-second pilot limits remain distinct and unchanged.

The first retained native attempt with this implementation rejected at the
reader root response: `root_response_service_worker`, with `fromDiskCache=false`,
`fromServiceWorker=true`, and `fromPrefetchCache=false`. A response delivered by a
service worker is not automatically stale; the preserved root policy nevertheless
rejects that class. No policy exception or recovery was applied. This result
qualifies the failing predicate for this run, not the unknown predicate of an
older run. Final cleanup was verified without retry. See the manual pilot record
and the bounded `fad333f8489b49d296a40f97aeee1b8a.run.log.txt` evidence.
