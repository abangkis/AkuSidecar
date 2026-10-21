# X bounded dev pilot contract

Decision date: 2026-10-21. Status: selected policy with an implemented offline
synthetic model; native adapter and runtime activation remain unavailable. The
user authorized an offline definition of a limited dev pilot and
minimum evidence renewal without reload, deferring complete refresh/rotation
qualification. This change opens no browser/profile, transfers no session,
modifies no Store code/policy/schema and performs no user-runtime operation.

The selected contract is `aku.x.dev-pilot.snapshot.v1`, with proposed provider
identifier `x-dev-pilot-snapshot`. Neither identifier is registered. It is
separate from the strict production contract `aku.x.concurrent-authorization.v1`.
Pilot observations never satisfy that contract by relabeling unknown continuity.
The existing synthetic authority, qualification flags and QA reports are unchanged.

## Purpose, bounds and retained evidence

Support one explicitly opted-in account, one reader owner and one true-headless
collector owner in distinct owned profiles. Opening a reader must preserve the
collector PID, generation, driver and active leases. Both browser/helper trees
remain children of Sidecar, including during startup and cleanup.

V1 has a **60-second maximum attempt lifetime**, measured monotonically from
before initial evidence acquisition or credential seeding, whichever occurs
first. Startup and cleanup retain their own bounded deadlines. Renewal cannot
extend this absolute lifetime. Expiry closes admission, invalidates unfinished
result authority and retires the collector; reader windows remain until user
closure or Sidecar exit. No automatic new attempt, reseed, login or lifetime
extension is selected. A longer dev window requires a separately reviewed policy
change; the present cap is not evidence that 60 seconds of native renewal works.

Reuse the accepted short-X concurrency evidence in
[MVP-1](x-concurrency-mvp-experiment.md#synchronized-checkpoint-continuation-2026-10-20).
Reuse the synthetic main/broker/Coordinator and real-Chrome ownership/renewal
evidence published as `bc64be767dae93f8607a12fff194523400f6d119`.
Neither needs repeated login, source A/B/A controls or overlap discovery merely
because this policy was selected. They do not qualify native in-place renewal.

## Evidence meaning

| Dimension | Minimum pilot requirement | Deferred or unsupported claim |
| --- | --- | --- |
| Enrollment | Explicit expected account and reader-profile selection, private confirmation and exact policy/consent generation | A first successful QA identity does not silently enroll an account |
| Owners | Trusted process/image/creation/Job, physical directory, context, role and generation bindings; complete reader-window inventory including bootstrap windows | Equal paths, PIDs or account IDs do not collapse two owners into one |
| Derivation | One separately authorized seed of the reviewed two-cookie manifest into the exact fresh collector context, with a private pair/attempt-bound receipt | No reverse merge, automatic reseed, expanded tuple set or reuse of an old QA report as a runtime receipt |
| Identity | Fresh, source-validated current-self identity in each role, privately equal to the enrolled account; retain exact owner/context/document/request provenance | Feed authors, handles, UI labels and rereading stale bootstrap data are insufficient |
| Login | Fresh successful response from the single pinned, previously tested settings route and its bounded semantic predicate in each role | HTTP 200, cookie possession or the other role's login cannot substitute |
| Session lineage | Explicitly `unknown`; reader and collector each have a local observation epoch for fencing only | No qualified native `ReaderSessionRevision` or `CollectorSessionRevision` is asserted |
| Global coverage and refresh | Explicitly partial/unqualified, with bounded diagnostics for observed writes and unsupported targets | Complete startup/worker/cookie-writer coverage, refresh/fanout and uninterrupted native continuity are deferred |

Pin login to `GET https://api.x.com/1.1/account/settings.json`, using the existing
bounded success predicate and reviewed request-local rejection qualification
`api_v1.cookie-auth-rejection.2026-10-20`. Other historical selector alternatives
are not automatic fallback sources. Preserve the guarded current-self projector;
its use as a live in-place identity source still needs qualification. Derivation
retains the exact [two-cookie transfer ceiling](x-concurrency-mvp-experiment.md#reviewable-one-way-transfer-ceiling),
including all attributes and private-data restrictions, without new execution
permission or a broader cookie read.

Local pair/observation epochs and evidence versions are not native session
revisions. Equal accounts at successive observations establish sampled agreement,
not an unchanged session between them. A pilot evidence-version chain records
continuous **local permission validity**, not continuous X session lineage.

Deferral permits an unobserved account/session transition between successful
checkpoints to escape detection. The opt-in description must state that limitation.
It does not claim instantaneous server-side revocation detection or silently
convert this bounded policy into production qualification.

## Minimum renewal without reload

1. Start with admission closed. Pin consent/policy, expected account, exact pair,
   owner generations and derivation. Install required evidence-channel and owner
   failure observation before exposing work. Native values cannot enter the
   synthetic-qualified source or its unconditional fixture grant.
2. Obtain a new one-use challenge vector for both roles. Bind each fresh identity
   and login observation to the authority challenge and actual private
   owner/context/target/frame/document/request references. The local challenge is
   request correlation, not an X session identifier or a request header to inject.
3. Preserve the existing **five-second evidence-age ceiling** and **two-second
   joint acquisition-to-publication deadline** for both roles together. Validate
   source timestamps and monotonic elapsed bounds; do not compare unrelated
   process monotonic clocks directly. Expiry is no later than the earliest source
   validity, evidence-age limit or absolute attempt deadline.
4. Acquire evidence in the existing owned contexts without `Page.reload`,
   navigation, debugger pauses/instrumentation, browser restart, collector
   replacement, cookie reseed, another login or an exclusive handoff. Preserve
   reader content, focus and window inventory. An existing capture may remain
   active while probes run, with bounded IPC and one serialized renewal vector.
5. A fresh read of an immutable bootstrap object is not automatically fresh
   current-account proof. The adapter must qualify a live in-place identity source
   and pin one login route; otherwise return `identity_unavailable` or
   `source_unavailable`. Reuse the existing guarded projectors where applicable,
   but do not reuse the QA bootstrap/reload procedure as the renewal operation.
   No guessed endpoint scan, reconstructed authorization header or replayed
   response is selected by this contract.
6. Schedule renewal early enough to finish before the current permit expires;
   never start an acquisition that is allowed to keep old permission alive while
   waiting. A failed or expired vector revokes the attempt. A later successful
   observation cannot bridge that permission gap. The synthetic 1.5-second timer
   is not a qualified native request cadence or a rate-limit guarantee.
7. Before publication, recheck parent/owners, consent/policy generation, pair
   fence and pending cancellation. Atomically replace the pilot permit only if
   its predecessor is still valid. Increment only evidence version, retaining
   the collector, pair and original work-ticket version.
8. Dispatch, claim and result acceptance recheck pilot mode/version, scope,
   account, owners, fence, validity and the uninterrupted local permission chain.
   Final result acceptance requires fresh successful per-role observations still
   within their deadlines and no known intervening invalidation. A result retains
   its original ticket/version; stale, canceled, duplicate or mismatched work is
   rejected. This remains sampled verification, not proof across unobserved
   session transitions.

The current native QA adapters do not yet meet steps 4-5 or the joint timing
requirement. Contract selection does not make them usable for native renewal.
If a supported source cannot provide those minimums without reload, stop at that
named source/timing blocker rather than reverting to broad refresh experiments.

## Deferred coverage versus observed failures

| Event or condition | Pilot decision |
| --- | --- |
| Global startup/worker coverage or refresh/rotation behavior is not fully qualified | Keep explicit partial/unknown status; this alone does not prevent the bounded snapshot pilot |
| A non-candidate or unclassified cookie-write header is observed | Record a bounded `cookie_write_unqualified` diagnostic; it is not proof of harmlessness, an accepted rotation or unchanged lineage, and does not alone revoke this weaker snapshot policy |
| A scoped post-seed write/delete/blocked-write attempt is observed for `auth_token` or `ct0` | Revoke conservatively; do not inspect values, reconcile, reseed or accept a fresh checkpoint as repair within this attempt |
| Required page/evidence channel detaches, crashes, overflows, loses correlation, or becomes ambiguous | Revoke; a known failure in the selected channel is not deferred global coverage |
| Selected evidence is redirected, cached, served through an unqualified service worker, duplicated, malformed, missing or stale | Reject the vector and revoke; partial global coverage does not relax source validation |
| Account mismatch, authentication rejection/challenge, owner/context/document replacement, unexpected target loss or cleanup uncertainty | Revoke and preserve pending work; unknown source data is not reported as logout |
| Non-final tracked reader window opens/closes under the same valid pair | Keep the collector and its work; update inventory through the trusted owner adapter |
| Last reader closes, Sidecar exits, opt-out occurs, consent/policy changes or the attempt expires | Revoke dependent authority; apply the selected ownership/cleanup rules, with no automatic independent collector or exclusive fallback |

The one expected initial seed is recognized only by its exact authorized stage,
pair, destination and receipt. It creates no general cookie-write suppression
window. Missing writer coverage means some writes may remain unseen; the pilot
must not label that absence as `no_mutation` or native continuity.

## Data, persistence and activation boundaries

Keep raw identity and approved credential material inside the trusted private
adapter. Export only fixed reason categories, match/mismatch/unavailable states,
bounded timings/counters and policy/eligibility status. Preserve the reviewed
16 KiB body ceiling, strict JSON/type/correlation validation and one-use reads;
no raw body, cookie/token value, credential hash, account ID or arbitrary error
text enters public reports, logs, argv or environment.

Every pilot report must distinguish mode/version, snapshot freshness, partial
coverage, `continuity:unknown`, `refresh:not_qualified`, observed failures and
cleanup. Production/native-continuity qualification stays false. Existing QA
`canAuthorize` fields remain false; a separate future pilot-eligibility result
must not be consumed as a production or Store permit.

This policy selects weaker ephemeral pilot eligibility only. It does not change
Store `AcceptedRevisions`, native revision checks, durable holds, migration,
queue claims or result ingestion. Until a separately authorized compatibility
contract exists, exercising the pilot remains fixture/QA with in-memory results;
native persistent ingestion remains unavailable. Do not bypass a durable gate
by labeling an account ID, local epoch or pilot policy ID as an accepted revision.

Future activation must be explicit, default-off, dev-only and bound to this
mode/version and limitation acknowledgement. Do not expose a normal-build
setting, endpoint or automatic downgrade from the strict contract. Opt-out closes
admission before retiring only the collector and preserves reader windows. A
Sidecar restart inherits no permit, consent-to-run attempt or evidence chain.
It may reuse an authorized retained login after fresh verification; a new login
is not inherently required. Unknown cleanup blocks replacement. Rollback keeps
pending work and existing guards; it never clears them to force resume.

## Next offline acceptance delta

Implement only after separate implementation authorization. Use synthetic inputs,
an injected clock and an in-memory sink; no browser, account, session transfer,
Store/database or runtime is needed for these contract cases:

- Default/unsupported mode has no side effects; the strict production contract
  and existing synthetic contracts reject pilot observations and policy/version
  mismatches.
- Two successive same-document renewal vectors use no reload/navigation/debugger
  command, preserve the original ticket and collector stamps, and finish a long
  synthetic capture while global continuity remains explicitly unknown.
- Joint deadline, source expiry, absolute 60-second expiry and clock rollback
  reject; a later success never revives invalidated work or extends the attempt.
- Unknown global coverage/unclassified writes remain diagnostic, while candidate
  cookie writes, known channel gaps, account/owner/policy drift, cancellation and
  stale/duplicate results revoke or reject as specified.
- A stale bootstrap reread cannot fabricate live identity. Cache/SW/correlation
  ambiguity and missing either role's fresh proof fail closed. Race renewal with
  opt-out, last-window close and parent exit; no reauthorization wins afterward.
- Private identifiers, raw body/credentials and arbitrary error text cannot escape
  the result schema; existing QA/native qualification flags cannot be promoted.

Passing those cases establishes the pilot model only. The next native gate is
separately authorized, bounded in-place source/timing QA with the retained QA
profile, not a repeated login/source-control/overlap campaign. Complete native
refresh, rotation, fanout and writer/startup coverage remain deferred from this
pilot gate and required before claiming the strict production contract.

## Offline pilot model implementation checkpoint (2026-10-21)

The three policy/design/roadmap documents were reviewed and published as
`b86e355728af5353f4a47758413e03f6a0ceaa43`, with origin/main verified. The user
then authorized this model and offline regressions, still excluding native login,
Store and user-runtime operations. This checkpoint is not native activation.

The independent model lives in
`internal/engine/development_concurrent_pilot.go`, with a separate model test and
cross-policy boundary test. All three files require both `auth_session_dev` and
`auth_session_fixture`. Types and methods are private. There is no new main,
broker, Coordinator or Chrome binding, provider registration, runtime setting or
activation endpoint. The existing strict authority and its fixtures are unchanged.

Explicit fixture grants, opt-in and limitation acknowledgement are required
before accepting a single attempt-bound invented seed receipt or probing either
role. The injected clock starts the absolute cap before either operation. A
probe-only callback returns invented live-identity/settings evidence with exact
owner/context/target/frame/document, request and source-qualification bindings.
Request replay keys include role/owner/context, so identical lexical CDP request
IDs in distinct browsers are not falsely treated as the same request.

Renewal probes execute outside the state mutex. Publication rechecks cancellation,
the unchanged pair, policy/consent, pinned owner bindings, previous permit and
deadlines under the state lock. Ordinary non-final inventory changes preserve
renewal and the collector. Modeled expiry/revocation invalidates unfinished work
and retires the collector while retaining reader windows; modeled parent exit
ends both owners. These are memory-state transitions, not OS exit/cleanup proof.
Original tickets survive only successful uninterrupted local permission renewal;
cancelled commands cannot be resurrected by issuing a new ticket for the same ID.

The model checks expiry at its API/report barriers. It runs no background native
scheduler. Source callbacks receive a bounded context and must cooperate with
cancellation; arbitrary Go callbacks cannot be forcibly interrupted. Late outputs
are rejected at publication, including cancellation while waiting for its mutex.
Future transport/scheduling and live in-place identity availability still require
their own qualification. No fabricated native revision or future timestamp is used.

The canonical helper is `scripts/test-concurrency-pilot.ps1`. Accepted run
`a718f7419b5440afaa41065e48e5071e` passed **18 top-level tests and 57 subtests**,
four source/test build-tag gates and combined dual-tag Engine compilation. Only
named stdlib sources execute; Engine initializers and Store/database tests do not.
Old authority test helpers support the cross-policy case, but its old test cases
are excluded by the `TestConcurrentPilot` selection.

The suite includes a twelve-second fake-clock capture through four renewals:
ten per-role observations, original ticket version one, current evidence version
five and one accepted result. It covers exact two-second rejection, five-second
and earlier-source deadlines, absolute 60-second expiry, clock rollback, future
seed receipts, role/owner/source/request replay and drift, stale bootstrap,
known-channel/cookie invalidations versus deferred coverage, cancellation at
publication, blocked-renewal races, window changes and bounded report privacy.
Native/production/Store authorization and existing `canAuthorize` flags remain
false even while model-only pilot eligibility is true.

The log is
`runtime/qa/auth-session-sidecar/concurrent-pilot-a718f7419b5440afaa41065e48e5071e.log.txt`.
Its unique owned temporary build root was removed. No browser, profile, session
transfer, native-source request, Store/database, network listener or user runtime
was opened. Native timing, refreshed account proof without reload, persistent
ingestion compatibility and actual runtime lifetime remain unqualified.

## Fresh current-self source candidate (2026-10-21)

The user authorized qualification of one no-reload identity source in the retained
QA profile. The selected candidate is a fresh same-origin Home document fetch,
not a reread of the existing document's bootstrap. The bounded mode is documented
in the [fixture README](../scripts/auth-session-fixture/README.md#no-reload-current-self-source-qualification).
It has one fixed GET, no custom authentication headers, no transfer, a two-second
abort and the existing 16 KiB body ceiling. The optional extra byte detects
overflow, never becomes parsed source data and does not authorize a larger read.
Only one inert literal bootstrap initializer and its exact current-self ID/entity
projection may qualify as a candidate, with response/cache/frame/loader guards.
An oversized, absent, ambiguous or unqualified response stops this candidate;
it does not authorize endpoint scanning, broader body reads or a login attempt.
This diagnostic does not change the pilot/Store policy, qualify two-browser timing
or register a native provider.

Offline preflight run `30f4a1ed16ae408bbf565bcde37bcdec` passed 39 Node tests
and one Go report-validation test. The separately authorized native run
`50a535ebbaa94311b8684b48ea5e9642` repeated that narrow preflight and completed
on the same retained login, with no user input or additional login. One exact
Fetch returned HTTP 200; its body exceeded 16 KiB. BYOB reading stopped at
16,385 bytes (one overflow-detection byte), aborted/cancelled the stream and
returned `outcome:body_limit`, `identity:unavailable`, `projection:null` and
`responseFresh:false`. No HTML was parsed, and no identity value was projected.
The document/frame/loader remained unchanged. The measured 424.2414 ms is time
to this bounded negative result, not successful identity or two-browser timing.
The native Go case took 5.43 seconds including owner startup/cleanup.

The report is
`runtime/qa/auth-session-sidecar/x-qa-source-50a535ebbaa94311b8684b48ea5e9642.report.json`;
the same prefix has its sanitized log. Graceful QA closure, whole-Job cleanup,
retained lease state `clean` and removal of the disposable control root were
verified. The retained profile remains available under its existing authorization.
No cookie API, transfer, authentication attempt, main-profile/runtime operation,
Store access or durable authority was introduced. `canAuthorize`,
`nativeQualified` and `sessionTransferred` remain false.

This candidate is **unqualified under the existing read ceiling**. The result
does not establish that fresh Home HTML lacks a current-self initializer, nor
that the retained session is logged out. No larger read, alternate endpoint or
automatic retry was run. A larger HTML envelope or different source requires a
separately reviewed data contract before another native attempt; the existing
16 KiB limit was not silently reinterpreted as only a report-size limit. Keep
the accepted MVP-1 concurrency result and offline pilot evidence unchanged.

## V2 HTML envelope implementation (2026-10-21)

After review/publication of V1 in `29bf7a5d407a25959279ba1f23390a84b9b5bcde`,
the user explicitly authorized implementation, offline regressions and one retained
QA probe with a larger, separate HTML envelope. This supersedes the V1 HTML-only
ceiling for this diagnostic; it does not change account-settings or Store policy.

| Boundary | V2 limit |
| --- | --- |
| Decoded response-body HTML bytes, before fatal UTF-8 decoding | 512 KiB, with one extra overflow-detection byte |
| Selected literal initializer, before strict JSON parsing | 256 KiB UTF-8; an overflow report uses the 262,145-byte detection sentinel |
| Strict initializer parser | Depth 24 and 16,384 value nodes; duplicate keys, invalid Unicode and unsafe/nonfinite numbers rejected |
| Page-to-CDP diagnostic and combined RPC report | Each at most 4 KiB serialized UTF-8; no raw initializer/HTML/account value |
| Existing account-settings JSON | Unchanged at 16 KiB |
| Existing shared CDP framing | Unchanged: 262,144-byte frame, 1,048,576-byte input chunk, depth 20/8,192 value nodes |
| Acquisition and publication | Two-second acceptance deadline; four-second transport watchdog only for failure/cleanup |

Schema `aku.x.qa-live-self.v2` adds the fixed `initializerBytes` counter and
separates HTML, initializer and parser failures. Its wrapper is
`aku.x.qa-live-self-run.v2`, source `home_document_fetch.v2`. V1 reports and
historical evidence are retained unchanged. The strict parser and guarded
bootstrap projector are serialized from trusted local helper implementations
into the fixed expression, not supplied by RPC or fetched from X. Both execute
inside the page. The Node boundary and Go decoder independently validate the
small diagnostic before persistence. The live document's bootstrap is never read.

BYOB reads remain at most 4 KiB with independent 256-nonempty/256-empty read
guards. Overflow is checked before parsing. An inert, uninserted template selects
one exact literal initializer; fetched JavaScript is never executed. Monotonic
checks around synchronous decode/template/JSON/projection stages reject late or
regressed-clock results; timer delivery is not a CPU preemption guarantee.
Network-disable acknowledgement and whole-Job cleanup precede a saved result.

512 KiB is an experimental ceiling, not an observed native HTML size or promise
that the initializer fits. One exact same-origin GET, no reload/navigation/debugger,
no custom authentication headers, no cookie APIs/transfer, no new login and no
automatic retry/source/cap escalation remain mandatory. Native/Store authority
flags stay false. The retained profile is reused only under its existing pinned
ownership lease. This change does not qualify two-browser timing, runtime renewal
cadence, session lineage or main-dev activation.

Offline preflight `15aa9a800e8b48a5bb105d4b7ebbfaf7` passed 80 Node tests and
one Go report-validation test. The one authorized V2 retained-QA run
`77a8a92cb2044f319875b775f4e2983c` repeated that preflight and completed the
native Go case in 3.23 seconds, including startup and cleanup. Its single HTTP
200 response completed with fresh transport correlation and an unchanged document.
HTML was 311,797 bytes and the selected initializer was 198,561 UTF-8 bytes;
both fit the authorized ceilings. Strict processing returned `parser_rejected`,
`identity:unavailable`, `projection:null` after 609.5355 ms. This fixed category
does not distinguish malformed input, duplicate keys, numeric/Unicode rejection,
parser budget exhaustion or projection-stage failure; no particular cause is
established by the report. No rejected content was retained for retrospective
inspection. The result is not logout or successful identity qualification.

The sanitized report is
`runtime/qa/auth-session-sidecar/x-qa-source-77a8a92cb2044f319875b775f4e2983c.report.json`;
the same prefix has its log. Graceful closure, complete owned-Job cleanup,
retained-profile state `clean` and removal of the disposable control root were
verified. No retry, limit increase, alternate source, session transfer or new login
was performed. Main login, Store and user runtime were untouched; all authority
and transfer flags remain false. A future probe should first define fixed,
privacy-preserving parser rejection categories and offline regression cases;
another native run needs its own authorization. Reuse the accepted MVP-1 overlap
evidence rather than repeating session transfer.
