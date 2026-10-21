# X bounded dev pilot contract

Decision date: 2026-10-21. Status: selected policy/design only; not implemented or
activated. The user authorized an offline definition of a limited dev pilot and
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
