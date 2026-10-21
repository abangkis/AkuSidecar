# X native manual startup pilot

Decision: 2026-10-21. Contract: `aku.x.dev-pilot.manual-startup.v2`.

The user explicitly selected a manual pilot of at most 60 seconds with strict
initial account/login verification, instead of renewing identity every five
seconds. This supersedes snapshot V1 renewal as the MVP gate only. V1's synthetic
model/controller/main fixtures and their successful evidence remain unchanged.
This decision does not enable a runtime, enroll an account or authorize another
credential transfer/native QA run.

## Minimum admission evidence

- Explicit opt-in, expected original stable X account ID, private enrollment,
  policy/consent version and acknowledgement of the limitation below.
- Two distinct owned reader/collector profiles, physical directories, processes,
  creation identities, contexts and role bindings under the Sidecar lifetime;
  trusted image/Job ownership and full reader-window inventory, including bootstrap.
- A new private derivation receipt for the exact authorized attempt, pair and
  collector. The previous QA report is evidence about feasibility, never a receipt
  that can be replayed into a new attempt. The two-cookie transfer ceiling is
  unchanged; code preparation does not authorize transfer execution.
- Fresh initial original-string current-self identity in both roles, matching each
  other and the enrolled account. Reuse the qualified startup/bootstrap projection
  recipe from MVP-1; no feed-author/handle inference, numeric ID coercion, stale
  bootstrap reread or promotion of a sanitized QA success boolean.
- Fresh initial protected settings evidence from exactly
  `GET https://api.x.com/1.1/account/settings.json`, with the previously qualified
  semantic predicate and request-local qualification
  `api_v1.cookie-auth-rejection.2026-10-20`.
- One-use challenge and exact owner/context/document/request correlation, direct
  fresh responses and initial samples no older than five seconds. Both roles'
  initial vector must complete in less than two seconds. Clock domains must not
  be compared as if unrelated process monotonic clocks shared an origin.

## Manual permission lifetime

The absolute clock starts before initial acquisition or authorized seeding,
whichever comes first. The attempt ends no later than that start plus 60 seconds
or an explicitly earlier owner/consent/attempt limit. Initial sample freshness
must pass at admission; its five-second timestamp does not secretly shorten the
selected manual window back to five seconds. Conversely, the manual window is
not a claim that account/session evidence stays fresh for 60 seconds.

There is no automatic identity renewal, attempt extension, restart inheritance,
reseed, reverse transfer or automatic exclusive fallback. A later attempt requires
new explicit initiation and fresh initial evidence. Known account/login rejection,
owner/context/policy/consent drift, missing last reader, channel loss/ambiguity,
uncertain cleanup, parent exit or opt-out revoke dependent use. Checks must occur
before dispatch/result publication, with no late result acceptance.

Acknowledged limitation: account/session changes after the initial checkpoint can
remain undetected until an observed invalidation or attempt expiry. Native session
continuity, complete writer/worker coverage, refresh/rotation and instantaneous
revocation detection remain unknown or unqualified. Full renewal is deferred.

## Implementation and execution boundary

The minimum dev-only native adapter accepts private typed receipts through trusted
owner/source ports and returns a sealed, attempt-bound validation vector. It does
not relabel native IDs into the V1 synthetic grant or create Store revisions.
Default construction is disabled; ordinary builds exclude it. No producer,
controller provider or activation endpoint is registered by this increment.

The subsequent startup/controller increment below implements those private
components without registering a native main/broker provider. The baseline
adapter acceptance and V1 reference model remain unchanged.

Explicit offline source provenance is distinct from trusted native startup
provenance. An offline vector is never native admission. Closed diagnostic output
keeps `canAuthorize`, `nativeQualified` and `storePermit` false; manual sampled
eligibility, when reported, is a separate value. Native IDs, paths, request values,
raw bodies, credentials and arbitrary exceptions stay out of public diagnostics.

Run `scripts/test-native-manual-pilot-adapter.ps1` for offline adapter regression,
four tag gates and dev/combined Engine compilation. It uses project-owned build
paths and named standard-library source files, executes no Engine/Store package
runtime and opens no browser/profile/login/listener. Producer/controller connection
and a separately authorized QA acceptance run remain necessary before a native
execution claim. Store/durable ingestion and user-runtime activation remain
separate approval boundaries.

## Offline adapter acceptance (2026-10-21)

Canonical run `8c439688d6f64d95be8f6ee85cb9a917` passed 11 top-level tests and
63 subcases, all four source/test tag gates and dev/combined Engine compilation.
The sealed vector rejects wrong account, owner/document/policy drift, replay,
stale or unqualified sources, exact deadline expiry, cancellation and late
publication after revocation. After initial acceptance it remains bounded by the
original attempt horizon rather than resampling or extending sample freshness.
Explicit offline provenance keeps manual native eligibility false. Public
diagnostics and ordinary formatting/JSON exclude private receipt values.

Log: `runtime/qa/auth-session-sidecar/native-manual-adapter-8c439688d6f64d95be8f6ee85cb9a917.log.txt`.
The owned temporary root was removed. No browser, profile, account access,
credential transfer, network listener, Store or user runtime was opened. This
validates the adapter's private receipt boundary; actual native acquisition and
controller/result fencing through the manual V2 path remain untested until those
producers/consumers are connected. Reuse the accepted startup recipe and MVP-1
overlap; full renewal and the parked Home parser are not prerequisites.

## Startup producer and manual controller connection (2026-10-21)

The user authorized connecting startup acquisition to the adapter and manual
controller, retaining the no-main-login, no-Store and no-user-runtime boundaries.
The new dev-only producer runs the two trusted role captures concurrently under
the adapter's joint deadline, then supplies the exact one-use reader/collector
receipts. It does not map native IDs to synthetic grants or manufacture revisions.
The controller checks the sealed vector at startup, dispatch, claim and local
result publication. A fixed collector binding and original 60-second attempt
horizon survive reader inventory changes that retain at least one window.
There is no renewal, retry, reseed or new attempt after cancellation/revocation.

The private Node transport projector receives a new optional startup sink from
`QABoundObservation`. The sink is called only after the fixed bootstrap and
protected-settings recipe succeeds, including source/body/freshness, cleanup and
document/context guards. It is not called for deferred-cookie-mutation samples.
Ordinary QA reports are unchanged and cannot supply receipts. The in-process
`createOwnedManualStartupCaptures` hook connects the existing owned reader and
collector recipes with a shared identity rendezvous. It is default-off, requires
an already-owned QA pair and has no RPC command or main activation setting.
It neither seeds credentials nor changes login. Its guard has offline coverage;
its native acquisition branch has not run in this increment.

The inherited private transport request has exactly eight fields: unpredictable
challenge, role, owner/profile/context/target/frame/document references. The
response schema is `aku.x.manual-startup-receipt.v1`, with exactly sixteen fields:
the request fields plus schema, original decimal account ID, separate identity
and settings request IDs, two observation timestamps, local elapsed duration and
the pinned settings qualification. It is capped at 16 KiB. The Go decoder rejects
unknown/duplicate/null fields, malformed UTF-8/surrogates, numeric IDs, replayed or
mismatched scope, reversed observation order and expired timing. No raw body,
headers, credential values or arbitrary exception text crosses this boundary.
Private identities exist transiently on this trusted channel, never in public
reports or logs. Physical directory/process/image/creation/Job, full reader-window
inventory, enrollment/policy/consent and derivation checks remain the trusted Go
owner port's responsibility; echoing transport fields cannot attest OS ownership.

Native context and loader pins are actual observations and must match the
configured startup document. The source does not substitute a newly navigated
document for an expected one. Before native acceptance, the owner/bridge must
bind the actual startup context/document at the appropriate capture phase.
The old reload-based QA choreography alone does not establish that ordering.
No wildcard pin, stale bootstrap reread or new identity investigation is selected.

The controller's effect callbacks are trusted synchronous, local, non-reentrant
atomic commits: returning false means no effect occurred. Its lock orders them
against explicit revocation and cancellation. These are in-memory effects, not
Store transactions, capture-manager wiring or a registered native collection path.
Offline mode must be explicit and match the source's offline provenance. All
native qualification, `canAuthorize` and Store flags remain false. The approved
post-start account/session observation gap remains; no continuity claim is added.

Run `scripts/test-native-manual-pilot-startup.ps1`. Canonical accountless run
`9049d389eb4c4669b5f5ad2546edf043` passed 27 Node tests, 10 Go top-level tests/49
subcases, four source/test tag gates and two Engine builds. The real Node stdio
roundtrip used a fixed synthetic bootstrap plus the actual bounded body projector,
then the strict Go producer, adapter, controller and exactly-once in-memory sink.
A mismatched actual document was rejected. Tests also cover concurrent acquisition,
source cancellation, timeout/expiry, malformed receipts, parent/owner/consent drift,
cancelled work, late startup callbacks and atomic revocation/result ordering.
One later driver-hook-only test passed after adding the default-off hook; the
combined set now has 28 distinct Node tests. Baseline adapter tests were reused.

Evidence is `runtime/qa/auth-session-sidecar/native-manual-startup-9049d389eb4c4669b5f5ad2546edf043.log.txt`
and `runtime/qa/auth-session-sidecar/native-manual-startup-owner-hook-validation.log.txt`.
The disposable build root was removed. No Chrome, profile, account, credential
transfer, Store or user runtime was used. The remaining native connection is the
trusted owner/main-broker instantiation and startup pin choreography, followed by
a separately authorized bounded QA run. Passing this offline path does not claim
that native Chrome or main dev is active.
