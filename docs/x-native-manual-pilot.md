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

## Conservative startup cookie guard V2 (2026-10-22)

This is an explicit manual V2 policy change. It does not reinterpret the
historical snapshot V1 allowance for unclassified cookie headers, relax the
ordinary QA guard, or permit deferred-mutation observations to publish startup
receipts. Only the trusted manual startup path uses this classification, after
its separately authorized seed has completed and before startup admission.

| Observed response cookie header | Manual V2 startup action |
| --- | --- |
| An `auth_token` or `ct0` write/attempt after seed | Reject, including before identity capture, deletion/expiry, blocked attempts and a candidate mixed with other or malformed lines |
| Only clearly classified other cookies with supported, correlated X scope | Keep a bounded diagnostic; this alone does not reject startup |
| Ambiguous name, syntax, scope, unsupported attributes, missing correlation or exhausted observation bounds | Reject; uncertainty is not a non-candidate exemption |

The guard classifies header attempts, not accepted writes or value changes. It
does not read cookie storage, compare values, prove unchanged session lineage or
infer harmlessness from a non-candidate classification. An absent header is not
evidence of complete mutation coverage. Both roles must still pass the same
fresh account/login, owner/context/document, one-use challenge, joint timing and
cleanup checks before a private startup receipt can be published.

Classification is bounded and private. Diagnostics contain fixed categories and
counts, never cookie values, raw headers, unknown cookie names, URLs, account IDs
or arbitrary exceptions. The policy cannot be selected through a public setting,
untrusted RPC payload or a sanitized QA success flag. Default-off gates and the
original absolute 60-second attempt deadline remain unchanged.

This guard covers the startup observation window. Network observation is torn
down at startup completion; it is not continuous cookie monitoring during the
manual attempt. Existing lifecycle invalidations still apply. Post-checkpoint
account/session changes, complete worker coverage and refresh/rotation remain
the acknowledged deferred limitations, rather than new MVP prerequisites.

Offline implementation uses `x-manual-startup-cookie-guard.mjs` through a trusted
bound-observation factory selected for both manual reader and collector startup.
The guard preserves the closed `observed_cookie_write` rejection reason and
adds a value-free category/counter projection. Ordinary QA construction remains
strict; deferred-mutation samples still cannot publish startup receipts.

Run `scripts/prepare-x-manual-retained-qa.ps1` with no switches for offline
regression and default-off preparation only. Run
`9c87833c8ec84c35b9fd95314e54d92e` passed 73 Node tests (13 dedicated guard tests),
three Go top-level tests with three startup subcases, four build tag gates and
standalone Windows compilation. The native entry skipped before lease/profile
access with all four execution gates disabled. No browser, retained profile,
X request, transfer, Store or user runtime was accessed. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-9c87833c8ec84c35b9fd95314e54d92e.prepare.log.txt`.
This is offline acceptance only; native V2 classification remains untested.

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

## Prepared retained-QA integrated test (2026-10-22)

The 17-file owner-port composition was reviewed and published as
`3847d4a98ba8298305b1c0448a7f37bf58207fb8`, with origin/main verified. One
integrated test was prepared and subsequently attempted once against retained
QA as recorded below. Product activation remains default-off and no main provider
is added.

Run `scripts/prepare-x-manual-retained-qa.ps1` with no arguments to prepare only.
It runs the focused accountless regressions, four tag gates, exact-source staging
and standalone compilation. The resulting test executable is invoked with all
four execution/transfer gates disabled and must skip before acquiring a lease or
accessing the retained manifest/profile. The staged sources contain no Engine
initialization, Store or package TestMain. The exact retained lease implementation
and pin helpers retain source provenance and original runtime.Caller mapping.

Canonical preparation `5d87b8f06d9b46f2be3540998d0904bf` passed 14 Node tests,
four tag gates, compilation and the disabled-entry check. A separate negative
check rejected `-RunPrepared` without fresh transfer authorization before runtime
setup. The now-consumed preparation root was
`runtime/qa/auth-session-sidecar/run-x-qa-source-5d87b8f06d9b46f2be3540998d0904bf/`.
Its `prepared-plan.json` pinned source, Node, Chrome and test-executable SHA-256,
records `executionApproved:false`, and contains no account or credential values.
Superseded prepare-only roots were removed; their bounded logs/provenance remain.

Execution requires a new preparation and fresh explicit one-transfer approval.
The generic invocation is:

```powershell
.\scripts\prepare-x-manual-retained-qa.ps1 -RunPrepared `
  -PreparedRoot '<new-prepared-root>' `
  -TransferAuthorized
```

The helper rejects source/binary drift, existing collector profiles and reused
execution claims before enabling process-scoped gates. This command was not
executed during preparation. The switch records the caller's fresh approval; it
is not approval inferred from a saved plan. No automatic retry is supported.

The prepared test requires an existing clean retained QA lease under this
checkout. Its dedicated existing-only acquisition has no create-profile fallback.
The immutable 60-second attempt starts before lease acquisition and collector
preparation. Home readiness may be polled for at most 15 seconds within that same
attempt; no login, SMS, account switch or new retained profile is attempted.
Node enters its Windows Job before Chrome launch. Both actual Chrome/profile
owners are pinned independently of the subsequent private seed metadata.

One guarded `manual_seed_bootstrap` command obtains an unqualified enrollment
candidate from the existing descriptor-only current-self projection, then copies
only `auth_token` and `ct0` through the previously qualified manifest: direction
reader to fresh disposable headless BrowserContext, domain `.x.com`, path `/`,
exact attributes, at most 8 KiB per tuple and 16 KiB total. Projection and write
validation remain at the bounded CDP boundaries. No value, raw body or account ID
is logged or written by the helper. This candidate and Home UI readiness do not
authorize admission: the two fresh, challenge-bound startup observations must
independently satisfy the existing strict account/login predicate and match it.

The real owner broker then prepares/acquires both roles under the unchanged
five-second age and joint sub-two-second evidence bound. A local controller
reservation is followed by a fixed `manual_open_reader` HOME window action
outside the controller lock, immediate controller recheck and unchanged frozen
collector inspection. CDP inventory retains bootstrap and actual visible Windows
are checked. This is a reserved QA action with a postcheck, not a new atomic
production OS-broker effect. One in-memory claim/result and duplicate rejection
follow; no real batch item or Store is used.

Cleanup retires only owned QA trees, verifies an empty Job, then releases the
retained lease as clean before writing the cleanup marker. Only that proof allows
collector/build-root deletion. Uncertain cleanup keeps its claim/root for review;
it does not trigger a retry. The retained reader profile is preserved. Actual
native identity acquisition, observer sessions and new-window behavior for this
integrated test have not passed native acceptance.

### One authorized attempt and diagnostic follow-up (2026-10-22)

Source `7db821137224cbe09f2a9413ceb7ecfe5c6102c5` was attempted exactly once
with explicit authorization for one bounded transfer. The test failed after
4.41 seconds at `manual-retained-one-seed-unavailable`. Home readiness, collector
preparation and Windows ownership pins had passed; broker startup and the
new-window action were not reached. The original error combined RPC and reply
decoding failures and erased inner setup causes. The failing phase and whether
cookie transfer completed are unknown; elapsed time does not identify a cause.

Owned Job cleanup and clean retained-lease release passed. The disposable
collector/build root was removed and the retained reader profile was preserved.
The authorization was consumed; no retry was performed. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-5d87b8f06d9b46f2be3540998d0904bf.run.log.txt`.
The follow-up scope is safe closed-category seed diagnostics and offline
regression coverage only. It grants no new transfer, profile access, login,
Store access, main-provider activation or user-runtime operation.

Seed failures now carry only closed phase, reason and write-state categories
through the private Node reply and the Go reporting allowlist. Enrollment phases
distinguish frame inspection, evaluation, projection and document recheck; cookie
phases distinguish reading, selection and writing. Arbitrary exception text,
properties, causes, account IDs, cookie values and response bodies are excluded.
The success DTO remains the same seven-field private enrollment record.

Write state is `not_attempted`, `unknown` or `acknowledged`. A failed or lost
write acknowledgement remains `unknown`; a later ownership failure can preserve
an already observed acknowledgement. RPC failure and strict reply-decoding
failure are reported separately. Neither a new diagnostic nor a decode error
can retroactively determine the previous attempt's transfer outcome. The
one-shot latch, default-off gates and 60-second deadline are unchanged.

Offline validation `dfa00d8aa5de44ec8faf3685859e8f33` passed 21 Node tests,
two Go tests (including the accountless Node-to-Go diagnostic reply), four build
tag gates and standalone Windows compilation. The retained native entry was
skipped with all four gates disabled, before any lease/profile access. Regressions
exercise the fixed enrollment command and synthetic projection, phase failures,
document drift, exception/getter suppression, asynchronous reply lifetime,
one-shot replay and uncertain write acknowledgement. No browser, X request,
credential transfer, Store or user-runtime action ran. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-dfa00d8aa5de44ec8faf3685859e8f33.prepare.log.txt`.

### Early enrollment follow-up (2026-10-22)

Source `b41d0b1cb5ce3df55aa5c3c75e9310638cdc29a6` was attempted once under a
new explicit one-transfer authorization. It failed after 2.96 seconds with
`phase=enrollment_project reason=bootstrap_missing write=not_attempted`.
The descriptor-only bootstrap source was absent when enrollment read it after
Home readiness; this does not establish when or why it became absent. Cookie
reading/writing and broker startup were not reached. Cleanup verified an empty
owned Job and a clean retained lease, removed the disposable collector/build
root and preserved the retained QA profile. No retry ran. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-dfa00d8aa5de44ec8faf3685859e8f33.run.log.txt`.

The authorized follow-up is a separate one-shot early-bootstrap enrollment
stage and offline regressions. Reuse the existing instrumentation-to-normal
breakpoint handoff with the fixed side-effect-checked identity projection.
Enrollment must complete with verified cleanup and a matching reader scope
before any cookie read or write. The candidate account is private enrollment
metadata only; after seeding, both roles must still produce the existing fresh,
challenge-bound startup evidence matching that candidate.

The enrollment stage owns separate state. It must not consume or reset the
startup observation gate, reuse a startup challenge, or treat an earlier
enrollment document as the later startup document. All preparation remains
inside the original absolute 60-second attempt. This follow-up authorizes no
browser execution, retained-profile access, credential transfer, Store change,
main-provider activation or user-runtime operation.

The enrollment adapter must retain an independently attached Runtime observer
on the exact owned reader target through the cookie handoff. Capture cleanup
disables its own Runtime session; that cleanup alone cannot establish continued
context observation. The independent observer binds the observed default
context, root frame and loader to the same reader process/profile/target and
rejects replacement, navigation, detach or crash. Scope checks are required
before cookie export, after the awaited read, before seed write and after its
acknowledgement. The observer is then detached; failed teardown is a rejected
outcome even when a write was already acknowledged.

Offline acceptance covers a valid synthetic bootstrap, exactly one reload,
fixed-expression/normal-hit guards, unrelated or changed scope, capture cleanup
failure, mid-handoff invalidation, observer teardown failure, one-shot replay,
safe diagnostics and the unchanged later startup gate. None of these results
constitutes native X acceptance or authorizes another transfer attempt.

Implemented in `x-manual-enrollment-capture.mjs`; the superseded post-Home
`Runtime.evaluate` enrollment route is removed. The shared bootstrap handoff and
capture implementations are reused unchanged. `enrollment_cleanup` is a closed
diagnostic phase in both Node and Go. Successful seed replies still have seven
fields; `homeReady` describes the earlier fixed UI check and does not claim a new
UI-readiness check after the enrollment reload. Both-role startup evidence still
supplies the actual account/login authorization boundary.

Canonical offline preparation `a3a1823152eb4769a12387424cb42294` passed 39 Node
tests, two Go tests (including the accountless Node-to-Go diagnostic bridge),
four build tag gates and standalone Windows compilation. Tests include old
context-clear ordering across the two sessions and cleanup that crosses the
deadline; late success is rejected while preserving acknowledged-write state.
The native entry skipped before lease/profile access with all four gates off.
No browser, X request, credential transfer, Store or user-runtime action ran.
The new native capture timing and dual-session ordering remain unmeasured.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-a3a1823152eb4769a12387424cb42294.prepare.log.txt`.

### Startup diagnostic follow-up (2026-10-22)

Source `aaa96419b578ace5934b9c98d21500291631818f` was attempted once under
fresh explicit authorization for one bounded retained-QA transfer. Enrollment
and seed setup passed. The test failed after 9.30 seconds at initial controller
startup with `source-unavailable`; admission never opened. Owned Job cleanup and
clean retained-lease release passed, the disposable collector/build root was
removed, and the retained QA profile was preserved. No retry ran. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-a3a1823152eb4769a12387424cb42294.run.log.txt`.

Offline review found diagnostic erasure, not a proven native source defect.
The bound observation already computes fixed identity/settings/freshness/cleanup
categories, but the startup runners discard their return values. Node source,
actor and owner-port wrappers further collapse failures; the Windows transport,
Go source and adapter drop the remaining role and stage. Both roles acquire
concurrently, so neither the outer reader-first loop nor total test duration
identifies the failing browser or establishes a joint timeout.

The authorized follow-up preserves safe closed role/phase/reason diagnostics
end to end and adds offline regressions. Preserve the first observed failure;
report cancellation of its peer separately. Unattributable pair cancellation or
deadline must not be assigned to either browser by inference. Diagnostic details
cannot contain identifiers, cookies, bodies, arbitrary exception text or causes,
and cannot supply authority or weaken any account/login, binding, time, cleanup,
one-shot or default-off requirement. This follow-up authorizes no new transfer,
browser/profile access, Store change, main-provider activation or user-runtime
operation.

Startup details use the private fixed code
`manual_initial_<role>_<phase>_<reason>_<kind>`, with closed catalogs on both
Node and Go. Node errors are branded in a WeakMap; Go accepts exact catalog
combinations and formats only validated categories. Public diagnostic Reason
and all eligibility/permit flags retain their existing behavior. The retained
QA test reads the added detail through a private broker readback.

The originating pair latch freezes the first observed cause before invalidating
the shared port or canceling the rendezvous. A `peer` marker identifies a reply
carrying that latched cause through another role's failure path; the reported
role remains the original cause's role. The Go acquisition loop starts both
roles concurrently and retains this attribution even if that peer reply arrives
first. A joint cancellation/deadline without a recorded role-specific primary
uses `pair/joint`; unqualified transport failures use a closed unknown fallback.

Canonical offline preparation `3d49942164334b6baf4266b2cc8572d4` passed 48 Node
tests, three Go top-level tests with reader/collector/peer-first startup subcases,
four tag gates and standalone Windows compilation. The accountless startup
roundtrip uses the actual correlated bridge and Windows backend RPC method,
then invented OS-owner snapshots through owner ports, source, adapter and broker
readback. It verifies diagnostic transport only; native ownership remains outside
this accountless test. The native QA entry skipped before lease/profile access
with all four execution gates disabled. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-3d49942164334b6baf4266b2cc8572d4.prepare.log.txt`.

The separately authorized one-shot run of source `99af7c1` then failed after
10.51 seconds with `reader/bound_finish/observed_cookie_write` as the primary
startup cause. Enrollment and seed setup passed; admission did not open. This
identifies a post-identity Set-Cookie observation, not a proven `auth_token` or
`ct0` change. The earlier guard did not classify its name/scope. Owned Job cleanup
and clean retained-lease release passed, the disposable root was removed, and
the retained QA profile was preserved. No retry ran. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-3d49942164334b6baf4266b2cc8572d4.run.log.txt`.
The conservative V2 guard above is the subsequent offline policy implementation;
it is not a successful native rerun or permission for another transfer.

Related canonical startup regressions
`3e02bb678c5d403e99c069c0e13ea391` passed 30 Node tests, Go startup/controller
checks, four tag gates and both development Engine builds. Owner regressions
`5da9929ebb8840e0b12fcba99d405493` passed 31 Node tests, nine Go top-level tests
with 12 subcases, four tag gates and Windows compilation. The focused checks
include closed formatting/typed-nil fallback, canceled entry, first-cause
retention, wire scope/time rejection, owner inspection stages, and peer delivery
without attribution changes. No browser, retained profile, X request, credential
transfer, Store or user-runtime action ran. The cause of the earlier native
`source-unavailable` remains unknown until a separately authorized observation.
