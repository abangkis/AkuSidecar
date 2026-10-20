# Minimal X concurrency experiment

Status: bounded short-QA concurrency accepted, 2026-10-20. The user authorized
deferring mutation/continuity qualification for this feasibility milestone.
Native state export/import still requires a separate,
explicit authorization after the exact transfer manifest and implementation are
reviewable. Main login, Store data/policy and user-runtime lifecycle are excluded.
Planning approval must not be described as execution approval.

The synchronized evidence is published in `2fbf338bf0258140f56cb1da567a1145834c5fd8`.
Continue with [the default-off MVP-2 integration design](x-concurrency-mvp2-integration-design.md),
reusing this accepted result. No additional native run is part of that design.

## Synchronized checkpoint continuation (2026-10-20)

The preceding 18-file QA implementation/evidence was published as
`d32768e7e176685a77f13846388e7879ae6a75c3`; remote main was verified. The user
requested publication and continuation of the proposed timing alignment, then
gave fresh explicit permission for one synchronized transfer after automatic
approval review required that clarification. The rejected attempt started no
process. This continuation covers one bounded repeat of the same two-cookie QA
experiment after offline regression checks. It does not broaden the manifest or authorize
main login, Store or user-runtime work. No automatic sequence of native retries.

Only the parallel after pair uses a private two-role rendezvous. After a guarded
call-frame read accepts a strong identity, that browser waits at its existing
ordinary breakpoint for the other role, for at most 1,000 ms. Both then continue
the natural page execution. Empty/weak identity cannot arrive. Existing five-second
capture/pause deadlines and all cleanup requirements still apply. No response is
held, rewritten, replayed or synthesized, and no arbitrary request/header is sent.
Timeout or an unsuccessful checkpoint stops acceptance rather than extending
the pause budget or retrying with more cookies.

The separate `aku.x.qa-synchronized-concurrency.v1` wrapper retains the original
V1 experiment unchanged and adds closed rendezvous status, reader-window status
and a private account-comparison result. Four completed checkpoints establish
`accountComparison:match` independently of overlap; this does not rewrite the old
captured report's conservative `accountMatch:unknown`. Reader target/window
ownership is now checked before applying the overlap gate, so zero overlap cannot
skip that observation. Missing window acknowledgement is not reported as preserved.
Neither wrapper nor experiment can authorize Store/native admission.

Command: `scripts/test-x-qa-source.ps1 -RunNativeQA -ShortConcurrency -AllowQASessionTransfer -SynchronizeCheckpoints`.
This is the only approved native repeat in this continuation. Preserve the result
and stop at a named blocker if synchronization does not establish overlap.

Acceptance evidence: offline run `10997e6cb6c541be95ce881e4983964b` passed
184 Node and 14 Go top-level tests. Native run
`90f3e12182b2494ba7933511ac3150a5` repeated the full preflight and passed
with `short_overlap_observed`. Both roles reached the rendezvous and were
released within the 1,000 ms bound. Four fresh protected-settings 200/body-match
checkpoints and the private account comparison passed without another login.
The reader retained the same target/window; the after-pair request lifetimes
overlapped for **447.682861328125 ms**. The experiment took 11,478.8404 ms,
and the whole native case took 15.65 seconds. The exact two-cookie seed was
acknowledged once. Collector context/root closure, complete owned-Job cleanup,
clean retained-lease release and disposable-root removal were verified.

The sanitized report is
`runtime/qa/auth-session-sidecar/x-qa-source-90f3e12182b2494ba7933511ac3150a5.report.json`.
All original V1 observations still report cookie writes and unknown binding;
`continuity:unknown`, `refresh:not_qualified`, `nativeQualified:false` and
`canAuthorize:false` remain unchanged. This closes the agreed short native
concurrency feasibility question, not durable refresh/startup coverage, enrollment,
provider revision acceptance or main-dev activation. Do not repeat login, source
discovery, tuple qualification or this successful overlap experiment merely to
publish changes. Reuse this evidence for the next scoped integration design.

## Short feasibility milestone: approved deferral (2026-10-20)

The immediate question is whether two separately owned Chrome processes can use
the same QA account concurrently for a short observation. Cookie setter coverage,
refresh/fanout and durable continuity qualification are deferred for this first
experiment. They are recorded as unknown, rather than inferred from a successful
request or from the apparently incidental name of a cookie. This narrows the
first experiment; it does not complete durable MVP-1 or authorize main-dev use.

The separate `aku.x.qa-short-checkpoint.v1` wrapper may accept a fresh private
current-self candidate plus the already qualified protected settings response
across an observed cookie write. The original V1 observation continues to report
`binding:unknown` and `diagnostic:observed_cookie_write` in that case. Context,
document, target, owner, cache, malformed response, duplicate request, freshness
and debugger-cleanup failures still reject the checkpoint. Store policy and the
production verifier are unchanged. No native qualification revision is minted.

### Reviewable one-way transfer ceiling

The fixed implementation manifest is `QA_SHORT_MANIFEST` in
`scripts/auth-session-fixture/x-qa-short-feasibility.mjs`:

| Name | Domain | Path | Secure | HttpOnly | SameSite |
| --- | --- | --- | --- | --- | --- |
| `auth_token` | `.x.com` | `/` | true | true | None |
| `ct0` | `.x.com` | `/` | true | false | Lax |

Both must be unique, unpartitioned, non-session cookies with a finite future
expiry, Medium priority, Secure source scheme and source port 443. Preserve the
freshly observed expiry exactly; reject a changed static attribute instead of
widening the manifest. `sameParty` may be absent or false. Limit each serialized
tuple to 8 KiB and the pair to 16 KiB. No credential fingerprints are produced.

One scoped `Network.getCookies` request uses only `https://x.com/home` and
`https://api.x.com/1.1/account/settings.json`. Chrome may return other applicable
cookies in that private CDP reply; discard those entries without reading their
value properties or exporting them. Only the two approved values/attributes may
reach one `Network.setCookies` command in a fresh true-headless Chrome process
and isolated disposable BrowserContext. No Cloudflare, Google or Apple cookies,
storage, profile files, database, application headers or reverse merge are allowed.
Values stay inside Node/Chrome and private pipes, outside RPC/reports/logs/argv/env;
JavaScript garbage collection is not a guaranteed memory-zeroization primitive.

The parent fixture Job owns both Chrome trees. Verify the new Chrome image and
Job membership before any export/import. Take reader-before and collector-before
checkpoints, then one parallel reader-after/collector-after pair. Compare the four
raw validated account IDs only inside the private Node comparator; export only
match/mismatch/unknown. Require actual protected-request interval overlap,
unchanged reader target/window ownership, no second login, four accepted checks
and verified collector-context/root cleanup. Overlap uses each Chrome request's
wallTime plus its monotonic request-to-loadingFinished duration; delayed delivery
of events through two pipes is not itself overlap proof. Missing, stale or invalid
timing rejects the sample. A 60-second experiment deadline,
challenge/rejection, identity drift or a failed checkpoint ends the attempt;
do not broaden cookie scope or automatically retry with all cookies.

`aku.x.qa-short-concurrency.v1` always retains `continuity:unknown`,
`refresh:not_qualified`, `canAuthorize:false` and `nativeQualified:false`.
An acknowledged seed is distinguished from an attempted seed with a lost reply.
The final Go Job cleanup remains authoritative before saving a sanitized report
or removing the disposable profile/control root. The retained QA lease is released
only after cleanup verification; no main-login/runtime process is involved.

Preparation command (no cookie read or transfer):
`scripts/test-x-qa-source.ps1 -RunNativeQA -PrepareShortConcurrency`.
The separate transfer command is deliberately gated by both `-ShortConcurrency`
and `-AllowQASessionTransfer`; do not execute it before the user's specific approval.

Preparation checkpoint: offline 180 Node/13 Go tests passed. Native no-transfer
run `711de0092ff844ffaf936879b39973fa` completed in 6.91 seconds with a valid
short reader checkpoint, distinct blank headless profile/context, verified
collector closure, whole-Job cleanup and clean retained lease. Its original V1
binding remains unknown after an observed cookie write. No cookie-store read or
transfer occurred; concurrent authenticated requests are not yet proven.

One specifically authorized transfer was subsequently executed in run
`45f648c0cfd348239ac95a07b79ec192`. The exact seed was acknowledged and all four
fresh reader/collector checkpoints passed, including the private same-ID checks.
No second login was needed. The experiment took 10,344.0848 ms; the full native
case took 13.73 seconds. Its parallel after pair produced no intersection of
the actual settings-request intervals, so the preserved report says
`outcome:overlap_not_observed`, `overlapMS:0`, `accountMatch:unknown`. In V1 that
last field is emitted as match only on overall experiment success, even though
four completed checkpoints require all private IDs to have agreed. Do not rewrite
the captured report to imply stronger acceptance. The final window-ID comparison
was not reached after the overlap failure. The after-reader checkpoint passed
after the collector-before checkpoint while both browser processes were alive.

This establishes sampled cookie sufficiency and before/after authenticated access
from both processes, not literal request overlap or durable continuity. All four
V1 bindings remain unknown after cookie writes; Store/native admission is still
unauthorized. Collector context/root closure, whole-Job cleanup, clean retained
lease release and disposable-root removal passed. The one-transfer approval was
consumed and no automatic second transfer was performed. Preserve this evidence;
if stronger simultaneous-request proof is required next, address phase timing
without expanding cookie scope, delaying/replaying X responses or repeating login.

This is the MVP-1 experiment in the [concurrency-first roadmap](collection-modes-roadmap.md#session-reuse-mvp-concurrency-first-critical-path-2026-10-19).
It refines the [one-login design](auth-session-reuse-design.md), reusing the
reviewed baseline `d397cdd` and the original synthetic broker/central experiments.

## Question and minimum useful result

Can one QA X login support a visible native reader and a separate true-headless
collector at the same time, without a second login, reader disruption or accepted
work from an unverified session?

Separate profiles do not inherit each other's login. A one-way seed tests whether
the minimum approved native state is portable and sufficient for the same account.
It also tests whether ordinary collector activity changes or invalidates the
reader's session. It does not establish that copying cookies always works, that
the session can refresh indefinitely, or that a central vault can control every
site request.

The one-way direction establishes one authority for this experiment: reader to
derived collector, never collector to reader. It prevents automatic selection or
reverse-merging of conflicting state. It is a qualification order, not a final
choice against the owner's central-ticket preference. Central refresh/fanout is
a later candidate only if the native behavior supports it.

## Fixed scope and reusable evidence

- One enrolled QA account, one identity-pinned retained QA owner profile and one
  fresh disposable headless user-data-dir with an isolated derived context.
  Never copy a profile or attach to the user's main browser.
- One visible reader role and one actual headless collector role, not background
  tabs silently substituted for headless. Preserve existing QA reader windows.
- Use the retained-profile ownership lease. Reject active/foreign ownership,
  identity changes or incomplete prior cleanup. Reuse its valid login; do not ask
  for repeated sign-in. Expiry/challenge ends the run for a user decision.
- Both browser trees and helpers belong to the existing kill-on-close fixture
  Job before work begins. In the integrated pilot their parent is Sidecar. No
  child may survive its owning parent; do not change the user runtime to test it.
- Reuse synthetic overlap, refresh/conflict, stale-result, queue and Job evidence
  while inputs are unchanged. Add only future checks for changed boundaries.
- Early native bootstrap proves a self-ID candidate and matching entity in one
  sample, not authenticated semantics or continuity. Account-settings and absent
  login flags must not be promoted to stronger evidence.

## Selected native evidence contract (2026-10-19)

Plan identifier: `aku.x.native-concurrency-evidence-plan.v1`. This selects sources
and a transfer ceiling for implementation review; it is not an implemented DTO,
native qualification, enrollment or permission to read/export credentials.
The synthetic baseline is `86b411369e309924a2864716efa116c3a4d2fe57`.
No browser, native observation or test is run by this documentation update.

### Account identity: fixed current-session bootstrap

Select the existing guarded early projection of `window.__INITIAL_STATE__` on
exact `https://x.com/home`. The identity candidate is the own data property
`session.user_id`, a positive decimal string of at most twenty characters, with
the same ID at `entities.users.entities[session.user_id].id_str` or `.id`.
If both entity ID fields exist, both must be valid strings and agree. An optional
`session.user_id_str`, if present, must also agree. Do not coerce numeric IDs,
invoke accessors, scan entity maps or fall back to a handle/content author.
Conflicting supported paths invalidate the observation rather than selecting
the convenient match. An absent source is unavailable, not logged out.

Native run `35bcb23af6164b14981bb585077f8045` observed this candidate and direct
entity agreement; the four inspected auth fields were absent. This establishes
availability in that sample, not trustworthy login semantics by itself. Keep the
existing one-use handoff, exact frame/context generation, evaluation guards,
pause limits and verified debugger cleanup. Do not restart broad timing searches.

For a future cross-browser comparison, the fixed adapter may pass only this one
validated ID to a private in-memory comparator, then retain match/mismatch/unknown
outside it. The current value-free projector cannot compare two browsers merely
because both report one candidate and an entity match; that private comparison
boundary still needs implementation and review. Never export IDs in public RPC,
reports, logs or screenshots. Pin the owner ID for this QA attempt only; equality
with the derived ID proves same-owner binding, not enrollment of a named account.
Any separately supplied expected QA identity must agree. Production enrollment
and Store authorization remain separate and unchanged.

### Login: qualified protected-response semantics, not a guessed flag

Select the already observed account-settings operation as the sole initial login
source candidate. The existing diagnostic selector has this closed route set:

| Method | Exact origin | Exact path |
| --- | --- | --- |
| GET | `https://x.com` | `/i/api/1.1/account/settings.json` |
| GET | `https://api.x.com` | `/1.1/account/settings.json` |
| GET | `https://api.x.com` | `/i/api/1.1/account/settings.json` |

These are existing selector alternatives, not three routes qualified by a native
sample. Existing reports preserve an operation category, not the exact route.
Before transfer, pin exactly one naturally observed method/origin/path in the
reviewable source manifest. No fallback across alternatives during a transfer
attempt, endpoint guessing, manually reconstructed requests or header replay.
OPTIONS is never a competing GET candidate or login proof. Query text is neither
persisted nor treated as identity; changes relevant to semantics require review.

The login claim requires all of the following, in the same role and observation
window as the current-self candidate:

- A fresh, uniquely correlated, completed response from the pinned route: HTTP
  200, JSON, no redirect, cache/service-worker/prefetch reuse or transport gap.
  Unknown cache/correlation state does not pass. Bind owner, browser context,
  target/frame, request, observation window and active session baseline.
- A bounded success projection with the six existing settings fields present
  at their expected types: `language` string, `time_zone` object, string or null, and
  `protected`, `always_use_https`, `discoverable_by_email`,
  `discoverable_by_mobile_phone` booleans. Reject malformed/duplicate-key JSON,
  errors, challenges and conflicting auth fields. A renamed/removed field is
  source drift, not permission to loosen the selector. The V3 QA projection adds
  the observed object alternative without projecting timezone contents; arrays,
  numbers, booleans and absence still fail. Offline qualification fixtures use
  `account-settings-protected.fixture.v2` for this predicate.
- A versioned qualification of that route's user-authenticated semantics,
  including an appropriate signed-out/guest comparison. A failed request due
  to networking, rate limiting, CSRF or anti-abuse is not a successful negative
  control. No request observed in a guest context is not a negative response.
  A comparison must use a separate empty QA context, natural web actions and
  separate execution authorization; never log out the retained owner. The
  authorized metadata-only control now opens one fresh BrowserContext on Home
  with no cookie import or constructed API request. It only observes the pinned
  `api_v1` route, reads no guest body and preserves unknown authentication even
  for HTTP 401, which remains an unqualified rejection candidate.
- Current-self identity agrees with the attempt's expected owner identity, with
  no relevant mutation or observation gap between these two observations.

The settings fields are a candidate success discriminator, not proof that this
body is exclusive to authenticated users. The native samples did not establish
the last two requirements or protected-route semantics. A field-type match alone
cannot close them.
Accordingly `authenticated` remains unknown until those requirements pass.
An explicit boolean is not mandatory if protected semantics are qualified, but
neither HTTP 200, a settings shape, a Home screen, `isActive`, nor possession of
a cookie alone may be relabeled as authenticated. Do not change the existing
fixture verifier to accept a native composite observation implicitly.

Keep the existing body ceiling of 16,384 decoded bytes, depth eight and 1,024
JSON tokens. Project once at the private response boundary and discard raw body
references before producing an external result. Only fixed field types,
success/error categories and match states leave that boundary. Ordinary Home
reads establish concurrent activity separately; feed authors are not self proof.

### Continuity and admission remain independent

Keep the current native lineage/coverage requirement. Bind identity and login
observations before dispatch and recheck within the existing five-second
freshness limit. Relevant mutation, account change, challenge, target/startup
gap, detach, overflow or timeout fences affected work. Pre/post cookie equality
and a local nonce/generation cannot qualify a gap or mint a Store revision.
A short successful transfer with no observed refresh reports
`refresh_not_observed`; it does not qualify an unrestricted concurrent pilot.

## Transfer manifest and data boundary

Select the following **two-tuple candidate ceiling**,
`aku.x.native-transfer-plan.v1`. Selection is complete; native presence,
attributes, portability and sufficiency are **not yet qualified**. This plan
does not authorize reading or transferring either value now.

| Exact name | Exact cookie domain | Path | Host-only | Partition | Treatment |
| --- | --- | --- | --- | --- | --- |
| `auth_token` | `.x.com` | `/` | false | none | One required nonempty value; never inspect its contents as identity or lineage. |
| `ct0` | `.x.com` | `/` | false | none | One required nonempty value; transfer as a cookie only, never extract/replay a CSRF header. |

The name choices are supported by X's [cookie help page](https://help.x.com/en/rules-and-policies/x-cookies)
and its linked [cookie-description list](https://help.x.com/content/dam/help-twitter/rules-and-policies/cookie-description-lists/cookie-description-list-en.pdf),
which describe account-login/authentication cookies. The linked PDF still uses
`.twitter.com`; it does not qualify a modern `.x.com` tuple or prove that these
two cookies alone are sufficient. `.x.com` above is a deliberately narrow
candidate scope to verify in QA, not a fact inferred from that older table.
No `.twitter.com`, host-only `x.com`, alternative path or partition is substituted
automatically. Any mismatch stops with `transfer_manifest_unqualified`.

Before export authorization, a metadata-only QA qualification must freeze the
exact `secure`, `httpOnly`, `sameSite`, session/persistent expiry representation
and supported CDP attributes of each tuple. Require Secure; preserve the
remaining observed attributes exactly, including fractional expiry and omitted
SameSite. Unknown attributes, opaque/partitioned state, duplicates/shadowing or
expired values stop rather than coercing them. Non-secret attributes may be
reviewed; values and credential-derived fingerprints may not. The frozen run
manifest must also bind the owned source/destination, exact source route,
expected account and one-use attempt. No unresolved field or wildcard is valid
at execution time. Existing fixture-only schemas/adapters cannot be relabeled
native to bypass these requirements.

Limit the native attempt to exactly these two tuples, at most 8 KiB per complete
serialized tuple and 16 KiB total. These are stricter than the generic synthetic
harness ceilings of 16 tuples/64 KiB; no code limit changes are made here.
Overflow stops instead of truncating credentials or raising limits. Read through
the owned private browser connection scoped to the pinned HTTPS source URLs,
never a whole-profile cookie API/database. If the protocol returns additional
cookies for those URLs, enforce a bounded frame before decoding, immediately
discard nonallowlisted values at the private boundary and publish only the two
exact tuples. Reject ambiguous same-name scope variants rather than selecting
one. This is incidental private decoding, not permission to collect, log or
transfer other cookie values. An implementation unable to enforce the boundary
must stop before export.

"Metadata-only qualification" describes its retained output, not a claim that
CDP cannot deliver values. If the chosen cookie API returns values alongside
attributes, its future approval must explicitly cover incidental private
in-memory decoding and immediate value discard, even when no transfer follows.
Do not use a metadata label to inherit credential-read permission from earlier
body/bootstrap observation. This document grants neither permission.

Only the explicitly pinned X web/source origins are eligible for the experiment;
the preserved `.x.com` cookie scope is not permission to browse arbitrary
subdomains. New cookies produced by X itself in the derived profile are not
additional imports. Observe session-relevant changes under the reviewed coverage
contract; unknown relevant mutations close the attempt instead of expanding the
allowlist or syncing anything back.

Exclude `twid` (including ID inference), `auth_multi`, `kdt`, `guest_id`,
`personalization_id`, other device/anti-abuse state and every unlisted cookie from
helper export/import. This does not assert those cookies are unnecessary for X.
If the two selected tuples are insufficient, report `additional_state_required`
and review the exact missing requirement; do not retry with all cookies.

| Data | Permitted boundary for a separately authorized run |
| --- | --- |
| Approved login cookies | Read only the enumerated tuples through the owned private browser connection; send values directly to the owned derived context in process memory/private IPC. Preserve Secure, HttpOnly, SameSite, expiry, domain/path and partition attributes; reject unsupported representation instead of broadening scope. |
| Origin scope | Only the pinned X web/source origins and the two exact candidate cookie tuples above. No cross-site or all-cookie export. Additional origins or tuple variants need manifest review and authorization. Google/Apple login-provider state is never part of the transfer. |
| Identity/authentication evidence | Read only the fixed, bounded source projection needed for current-self and authenticated semantics. Compare account values transiently; export only match/mismatch/unknown and qualified source/version categories. |
| Runtime binding | Carry private owner/context identity, observation freshness, one-use transfer nonce and local generation. These bind operations and reject replay; they do not create native session lineage. |
| Persistent evidence | Fixed diagnostics, bounds, enum outcomes, coarse lengths/counts, overlap timing and cleanup status only. No native account IDs/handles, cookie values or reversible/credential-derived fingerprints. |

Passwords, SMS codes, Google/Apple tokens, authorization/CSRF headers, raw request
or response bodies, browser history, localStorage, IndexedDB, sessionStorage,
device keys and whole-profile files are outside the transfer. If cookies alone
are insufficient, stop with `additional_state_required`; do not add storage,
headers or keys automatically. Ordinary website traffic remains Chrome-managed;
this restriction concerns helper extraction and transfer, not blocking X's own
requests. Main profiles and Chrome cookie database decryption are forbidden.

No credential values may appear in argv, environment, URLs, shell output, public
HTTP/status APIs, logs, screenshots, traces, receipts, Git or helper snapshots.
Do not persist a native vault/checkpoint for this experiment. Clear transfer
buffers/references and dispose the derived context on completion; do not promise
secure memory erasure. Chrome can write its own login state inside the authorized
QA profiles: memory-only helper transport is not proof of disk-free Chrome.

Keep the retained owner's profile under its existing retention authorization.
Remove only disposable task artifacts after proven Job/context cleanup. If cleanup
is unproven, retain that scoped root and report failure; never kill unrelated
Chrome processes. Local disposal/deletion does not prove server-side revocation.

### Offline implementation and next boundary

Subsequent explicit authorization implemented
`scripts/auth-session-fixture/x-native-plan-offline.mjs` and its regression suite.
It validates the closed synthetic plan and combines per-role identity, settings
and coverage witnesses. It reuses only the pure bootstrap projector and strict
JSON parser. It implements no browser/network/profile access, transfer callback,
provider or Store integration. See the [offline API and canonical commands](../scripts/auth-session-fixture/README.md#offline-native-plan-contract-and-evidence-combiner-2026-10-19).

The combiner privately snapshots metadata, consumes an attempt once, checks exact
tuple attributes and serialized limits, and compares validated IDs privately.
Its fixed reports contain no IDs, bindings, cookie values or body content.
Authentication/coverage success has synthetic provenance only; source-control
assertions are trusted invented inputs, not native qualification. Every result
remains non-authorizing with `refresh_not_observed`. Native schemas/values cannot
be supplied as a supported mode; no new opt-in reaches real authentication.

Canonical offline preflight `scripts/test-x-concurrency-fixture.ps1 -NoChrome`
passed 45 Node tests (18 new contract/combiner cases) and one Go report-boundary
test, with no failures or skips. Run `ed5aa4c683ed4a22b204419968fe7192` is recorded
in `runtime/qa/auth-session-sidecar/x-concurrency-ed5aa4c683ed4a22b204419968fe7192.log.txt`.
It started no Chrome or network listener and removed its disposable root. This
is offline contract evidence only; it does not qualify X source semantics,
credential sufficiency, actual transfer, native continuity or Store admission.

The next boundary is separately authorized metadata/source qualification on the
retained QA profile, after implementation review. Freeze its attribute/route
manifest before requesting value transfer. No extra manual login is expected
while that retained QA session remains valid. The incidental credential-decoding
permission boundary above still applies to any future metadata probe.

The [retained QA adapter](../scripts/auth-session-fixture/README.md#retained-qa-source-and-cookie-attribute-adapter-2026-10-19)
is now prepared under subsequent explicit implementation authorization. Its
default command `scripts/test-x-qa-source.ps1` runs offline only. Preflight
`c848552abe5d4dd182a2db61ce3a9408` passed 37 Node and seven Go top-level cases.
No retained profile, Chrome or listener was opened. The prepared native branch
requires an existing leased QA profile, Home readiness, a bounded exact-route
response and a one-use scoped cookie-attribute projection at the CDP boundary.
It cannot set cookies, seed a context or transfer a session. It preserves raw
value secrecy in reports, not a claim that Chrome/CDP never decodes values.

This adapter samples the selected source and attributes only. The signed-out
comparison required for protected-login semantics is not implemented/executed by
this phase, and no private account-ID join or full mutation coverage is claimed.
Reports explicitly retain authentication `unknown`, negative control `not_run`
and non-authorizing flags. After an approved metadata run, assess the exact route
and attributes and propose the narrow protected-source comparison; do not infer
that two present cookies or a matching settings body closes MVP-1. Native value
transfer remains a later separately authorized boundary.

The subsequently authorized native metadata run is recorded in the
[QA checkpoint](../scripts/auth-session-fixture/README.md#native-metadata-checkpoint-2026-10-19).
Preparation commit `a85cb7f` was pushed and verified before execution. After two
local harness-order corrections, run `e44f91354b004039bf2a48ba9c7d69d2` passed
38 Node/seven Go preflight cases and the 22.85-second QA case. It reused the same
retained login, pinned `api_v1`, projected exactly one settings response and
observed both candidate cookie tuples with metadata only. No transfer occurred.
The six-field settings contract did not match; authentication remains unknown,
the negative control is not run, and no native account/lineage proof is added.
The next source assessment should project only the six fixed field kinds and
fixed mismatch labels to localize this shape failure, rather than guessing a
new success rule or scanning other endpoints. A protected-source control and
review of CDP attribute/import compatibility remain required before transfer.
The Job was verified empty, the retained lease returned clean and the disposable
root was removed. The post-commit harness fixes and this checkpoint remain local.

Those eight files were later reviewed and published as `9ad4ab9`. A subsequent
authorized settings-only V2 projection then localized the mismatch in native run
`2fc104cde2444bc891a2f105b3da08a4`: only `time_zone` was object-shaped, while
the selected predicate allows string/null. Language was a string and the four
other fixed settings were booleans, all matching. Forty Node/seven Go offline
cases and the 24.22-second QA passed. No cookie reads, nested timezone inspection,
raw value persistence, login attempt or transfer occurred. The Job was emptied,
the retained lease returned clean and the disposable root was removed.
See the [fixed type checkpoint](../scripts/auth-session-fixture/README.md#fixed-settings-type-checkpoint-2026-10-19).
The source predicate is unchanged and still fails; authentication and native
continuity remain unqualified. Next review whether the metadata predicate should
accept this observed type, then qualify protected-source semantics. Do not infer
login from a shape adjustment or repeat broad body/source discovery.

The authorized 2026-10-20
[V3 timezone/control checkpoint](../scripts/auth-session-fixture/README.md#timezone-contract-and-empty-context-control-2026-10-20)
now matches all six settings types in retained QA. A separate empty context made
no settings request during its natural Home navigation. This closes that bounded
control as inconclusive; no negative response, login-source qualification or
transfer permission follows. Choose a meaningful scoped guest-response mechanism
or replacement source before proceeding. No additional login was needed.

Subsequent explicit authorization
implemented a separate fixed direct-navigation A/B control, retaining the
Home-only control above unchanged. It permits exactly the already observed
api_v1 URL in a retained-context tab and a fresh empty-context tab, without
header/cookie extraction or transfer. This is a scoped authorization beyond the
earlier natural-Home-only action, not permission for arbitrary API construction.
The [native checkpoint](../scripts/auth-session-fixture/README.md#direct-navigation-ab-checkpoint-2026-10-20)
returned A=403/aborted, B=400/completed and zero body reads, with QA Home surviving
and full ownership cleanup verified. The positive control failed. Neither error
cause nor native login semantics is inferred. Stop this mechanism; the next
decision must concern the working natural web-client path or another qualified
source, not additional login or automatic header replay.

The subsequent authorized [natural web A/B/A control](../scripts/auth-session-fixture/README.md#natural-web-aba-checkpoint-2026-10-20)
now preserves the working application request and copies it only within the same
QA Chrome page with credentials:omit for B. A and A again stay normal. The
reviewed exception is this one browser-local copy of an observed request, not
helper header extraction, arbitrary reconstruction or cross-context transfer.
Native 200/401/200 was observed with the original XHR versus Fetch for B, and
all cleanup checks passed. The result is cookie_dependency_candidate; generic
error-envelope 401 is not yet an explicit authentication rejection. Keep
authentication unknown until a bounded rejection projection rules out the
relevant alternatives. No new login, Store policy or user runtime access is
needed for this control. Account binding and native continuity remain separate.

The authorized [bounded rejection projection](../scripts/auth-session-fixture/README.md#bounded-401-rejection-checkpoint-2026-10-20)
later reproduced 200/401/200 with a single code-32 envelope and a private canonical
message match on B. The V2 report records provider_authentication_error and
cookie_auth_rejection_observed without native messages, arbitrary codes or raw
bodies. This resolves the rejection-meaning checkpoint for the tested request-
local no-cookie control. Unknown codes, message drift, conflicts and invalid
brackets fail closed. It does not establish fully guest semantics, account
binding, durable native continuity or sufficiency of only the two candidate
cookies; B omitted all browser-managed cookies. Store/production authority and
session transfer remain unauthorized. Reuse this result when integrating the
remaining evidence, rather than repeating broad source discovery.

The [same-document binding adapter](../scripts/auth-session-fixture/README.md#same-document-identitylogin-binding-checkpoint-2026-10-20)
now privately compares the strong current-self ID and correlates the next natural
settings GET within one retained-QA document and five seconds. Runtime observation
continues through response draining; required ExtraInfo and actual cleanup are
checked. Source semantics reuse the reviewed request-local A/B/A checkpoint;
neither a new negative control nor credential transfer is performed.
Offline checks passed 154 Node/eleven Go cases. Native binding remains unknown:
the corrected run observed a cookie write after the identity baseline, fencing
the HTTP-200 settings body read. Identity was observed; this does not prove a
session change or make login unknown equivalent to logged out. Qualify the
write's name/scope through a bounded value-free projection before another native
attempt. The present header-presence guard cannot distinguish session cookies
from unrelated cookies; never silently classify unknown writes as harmless.
Enrollment, native continuity, Store authority and transfer permission remain
separate. No main login, Store or user runtime was accessed.

The [cookie-write qualification adapter](../scripts/auth-session-fixture/README.md#cookie-write-namescope-qualification-checkpoint-2026-10-20)
now projects recognized names and declared scope from already-observed page
responses, without cookie values, raw headers, jar reads or more body reads.
Both QA samples projected five post-identity __cf_bm entries with domain x.com,
path /, Secure/HttpOnly, SameSite none and no Partitioned attribute. The second
sample correlated every setter but still reported partial/unresolved 2. Its
request-only counter was subsequently corrected offline; final checks passed
165 Node/twelve Go cases, without another native run or retrospective artifact
promotion. Metadata is header-only, not final storage/value-change proof.
The independently documented bot-management role does not authorize ignoring
these writes, enlarging the auth_token/ct0 ceiling, or changing Store policy.
Complete write classification before reviewing a narrow fixture-only distinction
between bot-state writes and account-session mutation; unknown writes must still
fence. Native identity/login binding remains unknown under the existing guard.

Native source failure is a named feasibility result, not a reason to repeat broad
bootstrap probes or another fallback campaign. If account-settings cannot meet
the protected-source contract, stop with `login_source_unqualified` and propose
one specific alternative with evidence; no automatic endpoint scan is allowed.

## Proposed bounded procedure

These are future steps, not commands approved for execution now.

1. **Offline boundary preflight.** Review exact tuple/source manifests, ownership,
   redaction, body limits, replay rejection and failure cleanup. Reuse passing
   synthetic tests; run only necessary changed-boundary checks after separate
   implementation/test authorization. A `verified` capability input alone is
   not native qualification.
2. **Owner readiness.** Acquire the retained QA lease, prove the old Job released
   it, and open the QA reader. Use the existing login. Install the required
   observation before session-bearing work; any needed close/reopen for attaching
   to this QA profile is setup only, not a continuity claim across that interval.
   Pin a fresh observed baseline after setup. Qualify stable current-self and
   authenticated source semantics; a UI label or generic HTTP 200 is insufficient.
3. **Single transfer.** Create the fresh headless profile/context under the owned
   Job. Export/import the exact manifest once through the private connection.
   Validate the same account and authenticated semantics in the derived context.
   No second login, reverse sync, automatic reseed or retry loop is allowed.
4. **Actual overlap.** Observe at most 60 seconds, with two bounded pairs of
   ordinary read-only X operations. Use only reviewed natural page actions/routes,
   never guessed API or refresh endpoints. Record request start/completion timing
   and require at least one pair whose intervals overlap and whose qualified
   responses succeed for the same account. Merely keeping two processes alive
   does not prove concurrent authenticated work. Preserve reader target/window
   identities and verify readiness again after overlap. No like/post/follow,
   logout, account switch or forced refresh action is part of this minimal run.
5. **Mutation/coverage decision.** Independently record observed rotation,
   correlation barriers, target/startup coverage and detach/transport failures.
   Unexpected mutation or coverage loss closes admission and rejects affected
   results. Keep windows while the parent lives; do not inject derived state into
   the reader, automatically reconcile conflict or declare fallback resumed.
6. **Stop and report.** Dispose the derived context, stop all owned QA processes,
   prove Job empty, clean only disposable roots and release the retained lease.
   Keep the owner's retained profile. Emit the fixed non-authorizing result and
   stop; do not extend sampling to wait for a rotation or request another login.

The initial 60-second cap and two pairs are proposed transport/experiment bounds,
not proof of session durability. If rotation does not occur, report
`refresh_not_observed`. A source challenge or need for more state is a named
feasibility result, not an invitation to retry with broader access.

## Decisions and exit gates

| Outcome | Meaning and next decision |
| --- | --- |
| `overlap_qualified` | One-way portability, same-account authentication and overlapping reads were demonstrated within stated coverage. This alone does not authorize a main-dev pilot. |
| `overlap_partial` | Process overlap, UI readiness or identity candidate only; authentication, mutation coverage or another required claim remains unknown. Preserve partial evidence without accepting real work. |
| `refresh_not_observed` | No refresh claim. Do not silently release an unbounded pilot or infer durability from elapsed time. Qualify a later bounded refresh path or propose an explicit limited-session acceptance decision consistent with policy. |
| `additional_state_required` / `transfer_unsupported` | The approved cookie set is insufficient or credentials are nonportable/device-bound. Stop this transfer strategy; a vault is not a workaround for nonportable credentials. |
| `cross_browser_invalidation` | Collector activity or rotation invalidates the reader, or vice versa. Stop; central coordination needs source-specific evidence before it is considered a remedy. |
| `coverage_unqualified` | Required startup/target/worker coverage is absent or lost. No concurrent admission under the existing policy. Checkpoints do not substitute for required continuous observation. |

Service-worker targeted response observation and shared-worker checkpoints have
synthetic evidence, but initial script-fetch/startup coverage remains unqualified.
Do not assume disabling workers preserves X behavior. Scope may be bounded to
supported behavior, but accepting a known relevant observation gap requires an
explicit policy decision; no such change is approved here. Safe closure acts on
qualified events/barriers, not instantaneous server-wide logout detection.

Every receipt remains `canAuthorize:false`; this experiment does not open Store,
claim jobs, enroll a real provider or create an accepted revision. Native account,
authentication and session-generation provenance must ultimately meet the
unchanged Store contract. A nonce, local epoch, cookie equality or matching IDs
before/after a gap is not a replacement for native lineage.

The enrolled authority profile and the derived profile/context have distinct
identities. Future integration needs an explicit verified mapping between them,
including generation/revocation binding. The existing single-profile checkpoint
must not be satisfied by relabeling the derived profile as the authority. Define
any provider/version compatibility change separately; this plan changes no Store
schema, accepted revisions or claim policy.

Do not wait for the exclusive fallback's close/reopen continuity proof to attempt
this concurrency experiment. Do not drop native proof requirements merely because
both processes now remain open. A successful bounded feasibility result permits
planning concurrent integration; activation still requires qualified proof,
integrated acceptance and explicit authorization.

Before the pilot, define last-reader-close behavior: in the tested reader-owned
launch configuration the owner exits with its last window, so reseeding becomes
unavailable. Continuing an existing derived session requires qualified admission
scope; otherwise fence collection and retain the queue. Reopening an owner needs
a newly observed/verified generation. Central-vault has synthetic reader-absent
refresh evidence but no native qualification. Invisible owner lifetime, seamless
restart, broader platforms and media remain separate work, not implicit promises.

## Offline harness checkpoint (2026-10-19)

After the planning documents were reviewed and published as `d1db051`, the owner
authorized an offline harness and its tests only. The implementation is
`scripts/auth-session-fixture/x-concurrency-harness.mjs`, with the canonical
offline command documented in the [fixture README](../scripts/auth-session-fixture/README.md#offline-concurrency-harness-2026-10-19).

The targeted suite passed 16 tests, zero failures/skips, using the bundled Node
runtime. It covers one exact synthetic seed, two in-memory overlapping read
pairs, primitive/data-only manifest bounds, account/profile/generation binding,
replay, evidence freshness/deadlines, fixed error redaction, observation loss,
cancellation and cleanup after late effects. Reader inventory is reported as
retained, changed or unverified; pending/failed cleanup is separate from success.
Static owner evidence expires after five seconds and is rechecked before
dispatch; the 60-second experiment ceiling does not extend that evidence's life.

All adapters in this suite are trusted in-process synthetic test code. No Chrome,
network listener, native X transfer, retained-profile access, Store access or
user-runtime operation occurred. The receipt remains non-authorizing and native
unqualified, with `refresh_not_observed`. This is harness scheduling/contract
evidence; the native transfer allowlist, browser adapter and coverage qualification
remain unfinished. No existing synthetic capability flag is promoted to native
evidence by these tests.

## Synthetic Chrome adapter checkpoint (2026-10-19)

The offline harness delta was reviewed and published as
`9b62f12fd2d2844a08c23a561074575b4d9c3a3c`. The subsequent authorization covered
fresh synthetic Chrome profiles and loopback tests only, not native X transfer.
`x-concurrency-chrome-adapter.mjs` now connects the same harness to a normal
Chrome reader and a separate true-headless Chrome tree through private pipes.
The gated driver launches only after the Go harness has assigned Node to its
kill-on-close Job. Go verifies the Node/Chrome images and Job membership before
running the experiment; no Store or production controller is opened.

Canonical command: `scripts/test-x-concurrency-fixture.ps1`; `-NoChrome` selects
offline checks, while `-Scenario` can isolate one named browser case. Browser
execution on this host required an unsandboxed tool invocation; no permission or
authentication guard was removed. All test data stayed inside the project QA
root and all native account/login/runtime boundaries remained unchanged.

Final run `f8a1306db4b64b91b4c9a00ca9d8166e` passed 27 Node cases, one Go report
boundary case and three Chrome cases on Chrome 155.0.8059.84:

| Case | Observed result |
| --- | --- |
| Concurrent reads | One invented login, one exact seed, four authenticated same-account reads; independent Go server observed two simultaneous requests. Reader/bootstrap windows were retained. |
| Cookie mutation | After the server confirmed active overlap, the reader rotated its invented cookie. The response observer fenced the run with `unexpected_mutation`; no successful pair was published. Reader windows remained. |
| Collector target loss | After the same server barrier, the exact collector target was closed. Actual target/observer events fenced the run with `coverage_unqualified`; context disposal completed and reader windows remained. |

Each browser case used its own new pair of profiles and exactly one synthetic
login. The final Go browser suite completed in 2.80 seconds. Job cleanup was
verified and the helper removed the disposable root. Sanitized command output is
in `runtime/qa/auth-session-sidecar/x-concurrency-f8a1306db4b64b91b4c9a00ca9d8166e.log.txt`.
No raw credential/body artifact or additional acceptance archive was created.

Two compatibility findings were corrected with relevant coverage. CDP can
return fractional cookie-expiry seconds; both manifest validation and the adapter
now preserve the finite value exactly instead of requiring/rounding to an
integer. This follows the [CDP Cookie definition](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Network.pdl),
where expiry is a number. Destroying a target can leave its pending evaluation
without a response: the adapter now retires a collector read only after the exact
target's destruction event, while retaining tracking for creation/seed effects
and verifying physical context disposal. Neither correction changes Store policy.

These results qualify only this controlled, worker-free loopback page topology.
The target-loss case does not prove arbitrary transport-loss handling, startup
worker coverage or instantaneous global revocation. The deliberate synthetic
rotation proves failure closure, not a native refresh/reconciliation protocol.
Reports retain `canAuthorize:false`, `nativeQualified:false`, `realAuthTested:false`
and `refresh_not_observed` for the unqualified durable-refresh claim. Native X
allowlist/authentication/lineage and source-specific observation coverage remain
the unfinished MVP-1 gate. The production concurrent controller is not activated.
