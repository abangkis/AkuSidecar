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
`createOwnedManualStartupCaptures` hook (superseded by the two-phase owner hook
below) connects the existing owned reader and
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

## Owner/broker startup binding (2026-10-21)

The preceding 15-file startup/controller increment was reviewed and published as
`ebc061ede87451b49c518f97285a5f1e00557894`, with remote main verified. The next
authorized increment connects a private dev-only owner broker and two-phase Node
actors. It fixes the ordering issue without accepting a wildcard document,
relaxing identity, inventing OS ownership or reopening renewal/parser research.

1. Broker construction is default-off. Admission stays closed; initial role
   configuration contains only selected profile/parent references. Old document
   pins are rejected as configuration, rather than overwritten after capture.
   The original attempt clock starts before preparation or authorized seeding.
2. Both owners prepare concurrently. Each driver reaches a verified normal
   startup breakpoint before evaluating current-self. The binder derives actual
   owner/profile/target/frame/loader/execution-context pins and the Node actor
   returns an opaque ready handle while keeping that same pause held.
3. Trusted Go owner ports inspect physical directories, process/image/creation,
   Job/parent and generation, policy/consent and full reader inventory. Both
   ready owners must pass before the exact pair is frozen. Document pins come
   from the prepared startup document, not the old pre-reload document.
4. The existing startup source issues new random challenges and acquires both
   prepared handles concurrently. Acquisition releases the held pauses; there
   is no second reload/navigation after binding. Callframe/context eligibility
   is rechecked before evaluation. A different loader, execution context, role,
   process or handle rejects the observation. Repeated empty bootstrap reads
   remain limited to the same frozen startup context and existing pause budget.
5. Only matching fresh account/login receipts open manual-controller admission.
   The joint two-second evidence deadline begins after both owners are ready;
   preparation still consumes the original absolute 60-second attempt window.
   The existing startup capture retains its shorter five-second observation and
   command deadlines. Reader requests, dispatch, claim and result publication
   pass through the broker and existing manual-controller fences.

The driver hook is now `createOwnedManualStartupOwners`; each role exposes
prepare/acquire/abort rather than the earlier one-shot capture function. It is
still an in-process, explicitly guarded hook with no public RPC or main provider
registration. Trusted private owner/transport ports compose it with the broker.
No cookie transfer, login operation, Store transaction or user-runtime activation
is added. These injected ports must use actual Windows ownership checks for a
native run; a Node ready handle is not an OS attestation or derivation receipt.

Partial startup and invalidation cancel preparation and abort registered owners.
The broker waits for registered and late prepare/abort acknowledgements under an
independent two-second cleanup deadline, without holding its state lock. An
uncooperative callback or failed cleanup yields permanent `cleanup-uncertain`;
a late owner cannot turn it into a successful completion. Opt-out preserves the
reader tree via the trusted owner port; parent exit requests retirement of both
trees. That OS behavior remains the port's responsibility and was not exercised
against Chrome in this increment. Synchronous effect callbacks stay local and
non-reentrant; no native operation is authorized by an offline success flag.

Canonical `scripts/test-native-manual-pilot-broker.ps1` run
`245501fa56f24dd685b21ca51c26da19` passed 44 Node tests, 7 Go top-level tests/20
subcases, four source/test tag gates and dev/combined Engine compilation. It
includes two persistent accountless Node processes: prepare holds each actual
synthetic document, Go freezes returned pins, acquire uses the same actor/handle,
then broker reader admission and exactly-once in-memory result completion pass.
The negative cross-language case rejects a wrong frozen document. Normal-pause
tests reject context loss before identity evaluation. Broker cases cover partial
and late prepare cleanup, uncooperative cleanup, policy/consent/process drift,
original expiry/clock rollback, last reader and parent exit. A three-second
synthetic preparation still requires a fresh sub-two-second acquisition afterward;
it does not reset the original attempt clock.

Log: `runtime/qa/auth-session-sidecar/native-manual-broker-245501fa56f24dd685b21ca51c26da19.log.txt`.
The owned temporary root was removed. OS/process/directory/Job claims in this test
are explicitly invented; actual normal-pause behavior is tested with the local
VM/handoff harness, not Chrome. No browser/profile, account, transfer, Store or
user runtime was used. The owner/broker binding code is connected and verified
offline; native Windows ownership/transport instantiation and bounded native QA
acceptance remain unproven. Main-dev activation still requires its own explicit
authorization, with the existing Store compatibility boundary unchanged.

## Windows/Chrome synthetic ownership checkpoint (2026-10-22)

The preceding 16-file owner/broker increment was reviewed and published as
`80b73a6ca491e8313bf0cac1c500f57f48cec9e5`, with origin/main verified. The next
authorized test instantiates that private broker using real Windows process,
Job, physical-directory and Chrome document checks, while all account/login
evidence remains invented loopback data with explicit offline provenance.

Run `scripts/test-native-manual-pilot-chrome.ps1` for three Node boundary tests,
four build-tag checks and standalone compilation. Add `-VerifyChrome` to run the
two accountless Chrome scenarios. Each scenario creates fresh, distinct reader
and headless profiles, one synthetic loopback login and one invented-cookie seed.
The original bridge and selected process/window functions are staged unchanged
with SHA-256 provenance. Engine initialization, Store and user-runtime dispatch
are excluded from the named-file test binary; unused grant/recovery branches
reject rather than simulate native authority.

Canonical Chrome run `881a30d5861441589ed3ad981adb3819` passed both scenarios
(2.69 seconds total), the three Node tests and four tag checks:

- Actual Node/Chrome executable paths, PID creation identities, parent chain,
  separate Windows Jobs and distinct physical profile directories are checked.
  Node is assigned to its Job before Chrome can launch. Both startup documents
  pause normally, and actual target/frame/loader/context pins are frozen before
  challenge-bound acquisition. The original joint two-second evidence bound,
  five-second initial age and absolute 60-second attempt limit are unchanged.
- Opening another real reader window preserves the collector's process,
  generation, profile and document. Bootstrap remains in the window inventory.
  The in-memory work/result sink accepts exactly one completion and rejects its
  duplicate. Closing the independent window preserves bootstrap; closing the
  final reader window revokes offline admission.
- Destroying the prepared startup target rejects acquisition. Every role tree
  is retired, Node/root exit is observed and owned Jobs are verified empty before
  disposable profile deletion. This tests explicit owned-tree cleanup; it does
  not add a new abrupt Sidecar-parent-death acceptance claim.

Real Chrome exposed one adapter assumption: execution-context IDs are scoped to
their owning process/session, so both roles can legitimately report the bare
value `2`. Distinct PID/owner/profile/physical-directory checks remain mandatory;
per-role receipts still bind the complete owner/context tuple. The new regression
accepts equal bare IDs only in separate owners and rejects wrong-process,
wrong-role and same-process vectors. Canonical adapter run
`bfc987d9ae89473caa9c5dea54ae986c` passed 12 top-level tests/67 subcases, four tag
checks and two Engine compilation modes.

The fixture also had to consume its settings fetch body: discarding the response
left both roles waiting for `Network.loadingFinished`. A fixture-only 1,750 ms
watchdog now returns a fixed phase category before the unchanged Go deadline;
late private receipt buffers are zeroed. Failed runs remain represented by their
bounded logs/provenance, while their disposable roots were removed after owned
cleanup proof or verified rejection before process launch.

Evidence: `runtime/qa/auth-session-sidecar/native-manual-chrome-881a30d5861441589ed3ad981adb3819.log.txt`
and the adjacent `.provenance.json`. No real X request, retained QA profile,
real-session transfer, main login, Store or user runtime was used. All native
qualification, manual eligibility, `canAuthorize` and Store flags remain false.
No main provider or activation endpoint is registered. This closes the fresh
synthetic Windows/Chrome ownership check, not native X evidence or main-dev
activation. The next bounded step is review of the native-X owner-port composition
and its explicit QA authorization boundary; the deferred renewal/rotation work
does not become a prerequisite for this manual policy.

## Private X owner-port composition (2026-10-22)

The 11-file synthetic Windows/Chrome checkpoint was reviewed and published as
`c77dc3a52906da0056b25074c02ac271d68c0357`, with origin/main verified. The next
increment connects the existing X startup recipe to the same manual broker and
controller through a private QA owner port. It registers no main provider,
activation endpoint or persistent setting and remains default-off.

The Windows constructor accepts only already-owned QA role records: pinned
Node/Chrome/parent creation identities, live Chrome handles, actual Job membership
and physical root/profile handles. It launches nothing, acquires no retained
profile lease and seeds no session. Reader and collector must have distinct
Chrome processes and physical profiles; they may share their owned Node and Job.
Native provenance is fixed in the compiled Windows backend. Offline tests use a
different backend that can produce only offline provenance; no request/config
boolean can promote a sanitized QA report into native evidence.

The fixed private stdio operations are `manual_inventory`, `manual_prepare`,
`manual_inspect`, `manual_acquire` and `manual_abort`, each scoped to the existing
reader protocol owner and one exact role. They add no arbitrary URL, expression,
body, cookie or transfer operation. Both prepare/acquire calls can run concurrently
on one Node, preserving the existing identity rendezvous. The Go bridge reuses
its correlated writes/replies without serializing one role behind the other.
Other QA workflows are rejected once this port is selected, except release.

Preparation binds the actual Chrome PID and canonical owned profile, replacing
the old diagnostic-only protocol-owner/profile labels in manual mode only. It
holds the existing verified startup pause, returns the actual document pins and
requires matching live frame/loader/context observations before acquisition.
Each role creates a separate Page/Runtime observer session before release of the
held pause. The startup capture still performs its real cleanup; the observer
retains context/document loss observation afterward. Reader inventory includes
every page window and the original bootstrap window. This is limited lifecycle
observation, not complete cookie/worker/rotation coverage or identity renewal.

The same original attempt clock, initial account/login predicate, five-second
sample age and joint sub-two-second acquisition/publication bound apply. Work
dispatch, claim and result completion still pass through the existing controller.
Lost/malformed PREPARE replies can be cancelled with a null handle through the
trusted role port; cancellation revokes the actor before readiness and emits no
evidence. Uncertain cleanup stays uncertain, including after a later retry.
Reader-preserving cleanup can escalate to full retirement on parent exit.

Run `scripts/test-x-manual-pilot-owner.ps1` for the accountless regression suite,
including one shared real Node stdio process feeding the strict Go source and
manual controller, four tag gates and standalone Windows-backend compilation.
The staged binary excludes Engine initialization and Store. The helper has no
native execution switch and opens no Chrome or retained profile. It does not
perform an X request or transfer a real session. Native X acceptance, including
the additional observer-session behavior, remains unmeasured until a separately
authorized QA run. Main-dev activation remains a separate authorization boundary.

Canonical owner-port run `915ed92f84984779a7bc03704f386617` passed 30 Node tests,
eight Go top-level tests plus ten subcases, all four source tag gates and the
staged Windows-backend compilation. The shared-Node case feeds the real private
stdio protocol through the strict Go startup source, broker and controller,
then verifies dispatch/claim, one result and duplicate rejection. Regressions
cover lost prepare cleanup, role/process/document drift, policy/observer loss,
expiry, null/case-aliased/duplicate JSON fields and post-capture context loss.
The fake backend uses a deterministic whole-second clock to avoid synthetic
epoch-float rounding against one Windows clock tick. The compiled native Windows
backend retains `time.Now`; no timestamp tolerance or production deadline changed.

Evidence: `runtime/qa/auth-session-sidecar/x-manual-owner-915ed92f84984779a7bc03704f386617.log.txt`
and the adjacent `.provenance.json`. The accountless build root was removed.
This run executed neither Chrome nor the native Windows ownership backend;
the earlier synthetic Chrome evidence remains scoped to its original fixture.
