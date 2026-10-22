# Local one-login concurrency fixture

## No-reload current-self source qualification

`scripts/test-x-qa-source.ps1 -LiveCurrentSelf` runs offline preflight only.
Add `-RunNativeQA` only for an authorized retained-QA run. The independent mode
cannot combine with cookie reads, seeding, concurrency or other source modes.
It acquires the existing identity-pinned retained-profile lease and owned Job;
it starts no login or headless collector and never changes the main profile.

The single candidate is a fresh `GET https://x.com/home` HTML response fetched
inside the existing Home document. It uses browser-owned same-origin credentials,
`cache:no-store`, `redirect:error`, no custom headers and a two-second abort.
The V4 helper reads at most 524,289 decoded response-body bytes (the extra byte
detects the fixed 512 KiB HTML ceiling). It independently limits the initializer
to 256 KiB UTF-8 before strict JSON parsing, with depth 24 and 16,384 value nodes.
It rejects oversize data and never widens either cap automatically.
Only one literal `window.__INITIAL_STATE__ = <JSON>;` script in an inert,
uninserted template is eligible; no script executes and no resource is loaded.
Strict parsing and fixed current-self projection occur inside the QA page.
Only a validated diagnostic of at most 4 KiB UTF-8 crosses CDP; the combined RPC
report is also capped at 4 KiB. Raw HTML, initializer, identity values, headers and
arbitrary errors never cross this projection boundary or enter reports/logs.
Shared CDP frame/chunk limits and the account-settings 16 KiB ceiling are unchanged.

Monotonic checks before/after synchronous processing and driver publication enforce
the two-second acceptance deadline. The timer cannot preempt synchronous parsing;
the four-second command watchdog remains failure/cleanup protection. Fixed failure
categories distinguish HTML overflow, initializer overflow, HTML parsing rejection
and strict-parser rejection. This is source qualification, not a renewal cadence.

The closed V4 observation (`aku.x.qa-live-self.v4`) requires `parserRejection`:
`malformed_json`, `duplicate_key`, `invalid_unicode`,
`depth_limit`, `value_limit`, `parser_internal` or `projection_failed` only when
the outcome is `parser_rejected`; all other outcomes require JSON null. The
wrapper/source are `aku.x.qa-live-self-run.v4`/`home_document_fetch.v4`.
Both Node and Go reject missing/unknown/inconsistent categories and older schema
versions. No error message, stack, input offset, field name, source fragment or
raw content is returned. The shared strict parser is unchanged. V4 uses the
separate QA-only lossless parser described below, with the same syntax, duplicate
key, Unicode, depth and value-count guards. V1/V2/V3 native reports remain
historical and are not assigned inferred categories or replayed as V4.

Every JSON numeric lexeme is held exactly in a private WeakMap behind an empty,
frozen token. There is no numeric conversion, exponent arithmetic or numeric-to-ID
string coercion. A separate bounded projection view maps these tokens to a
private primitive Symbol. The existing bootstrap projector therefore rejects them
as IDs, booleans or required records, while unrelated numeric fields cannot cause
rounding or whole-document rejection. An original string ID must still match all
present current-self/entity IDs. `NaN`, `Infinity`, malformed numbers and duplicate
decoded keys remain invalid JSON. Valid huge exponents remain opaque text.
Tokens, lexemes and the private view never cross CDP or enter reports/logs.
The 512/256/4 KiB limits, two-second acceptance deadline and false authority flags
are unchanged. Lossless parser correctness alone authorizes no native probe,
Store admission or main-dev activation.

V4 offline preflight `20774f66ae0441fb88ae57b4f054bba4` passed 99 Node tests and
one Go test with seven category subcases. The canonical `-LiveCurrentSelf` helper
includes the new `x-qa-lossless-json.test.mjs` suite. It opens no browser or
retained profile. Exact numeric representation, original-string identity guards,
foreign-context rejection and previous size/deadline/privacy guards passed;
the shared strict parser and bootstrap projector were unchanged. At that offline
checkpoint no V4 native result existed. The sanitized log uses the run ID under
`runtime/qa/auth-session-sidecar/`; the disposable control root was removed.

After publication as `6e68702`, separately authorized native V4 run
`dc56485eb6b9449193026a9c7b458cb9` repeated all 99 Node/one Go tests with seven
category subcases and passed the native cleanup case. One fresh HTTP 200 response
contained 311,797 HTML bytes and a 198,561-byte initializer. Processing returned
`parser_rejected` / `malformed_json` in 1,406.4764 ms, with identity unavailable,
projection null and unchanged document. No failing location/value was retained.
This category also covers an unexpected internal parser exception mapped by the
current parser fallback; it does not conclusively identify invalid source syntax.
Graceful closure, complete owned-Job cleanup, retained state `clean` and disposable
root removal were verified. No retry, transfer, new login, Store or main-runtime
operation occurred. See the
[V4 native checkpoint](../../docs/x-concurrency-dev-pilot-contract.md#v4-retained-qa-result-2026-10-21).

Offline run `784ee64c8314441ba6180fdc02e78a81` passed 84 Node tests and one Go
test with eight category subcases. It covered category production, thrown parser/
projection faults, closed-schema/privacy checks, legacy parser behavior and the
existing byte/deadline regressions. No Chrome, retained profile, listener, network
probe or user runtime was opened for that offline change.

After publication as `6449cfc`, the separately authorized single V3 native run
`9c71cf93547a405b9dcd9bebd1372552` repeated the preflight successfully and
returned `parser_rejected` / `unsafe_number` after 536.7426 ms. HTML/initializer
were 311,797/198,561 bytes; the fresh HTTP 200 response completed without changing
the document. No field/value or numeric subtype is established by this category,
and the historical V2 rejection is not retrospectively classified. Identity remains
unavailable. Graceful closure, complete owned-Job cleanup, retained state `clean`
and disposable-root removal were verified. No retry, transfer, raw-content
retention, new login, Store or main runtime operation occurred. See the
[V3 native checkpoint](../../docs/x-concurrency-dev-pilot-contract.md#v3-retained-qa-result-2026-10-21).

The transport requires one exact request, matching target/frame/loader, fresh
non-cache/non-service-worker response, completion and unchanged document before
reporting a fresh identity candidate. It issues no reload, navigation, debugger
or cookie command. A negative candidate is not logout. All authorization/native
qualification/transfer flags stay false, including on a positive single response;
two-browser renewal timing and broader source controls remain separate.

Previous V1 negative native run `50a535ebbaa94311b8684b48ea5e9642` passed 39 Node
tests and one Go preflight test, then recorded HTTP 200 with `body_limit` at the
16,385-byte detection bound after 424.2414 ms. No initializer or account value
was parsed. The original document remained intact, the owned Job was emptied,
the retained lease returned `clean` and the disposable root was removed.
The successful test means bounded diagnostics and cleanup passed; the source
remains unqualified. See the
[source checkpoint](../../docs/x-concurrency-dev-pilot-contract.md#fresh-current-self-source-candidate-2026-10-21).

V2 run `77a8a92cb2044f319875b775f4e2983c` passed 80 Node/one Go preflight
tests and the native cleanup case. One fresh HTTP 200 response contained 311,797
HTML bytes and a 198,561-byte initializer, within both caps. The result was
`parser_rejected`, identity unavailable and projection null after 609.5355 ms;
the fixed diagnostic does not establish the parser's specific rejection cause.
The document was preserved, the owned Job emptied, retained state returned to
`clean` and the disposable control root was removed. No raw content was retained,
no transfer/new login occurred and no second native attempt ran. See the
[V2 evidence](../../docs/x-concurrency-dev-pilot-contract.md#v2-html-envelope-implementation-2026-10-21).

## Offline manual startup pilot adapter

Current MVP policy is
[`aku.x.dev-pilot.manual-startup.v2`](../../docs/x-native-manual-pilot.md), explicitly
selected by the user: strict initial account/login evidence for both roles and a
manual attempt capped at 60 seconds from before initial acquisition or seeding.
Initial freshness/joint timing remain strict. No automatic identity renewal is
required by this policy; observed invalidations still stop use. Account/session
changes after the checkpoint can remain undetected until observation or expiry.
This supersedes V1 renewal as the MVP gate without changing V1 fixture behavior.

The manual path uses the [conservative V2 startup cookie guard](../../docs/x-native-manual-pilot.md#conservative-startup-cookie-guard-v2-2026-10-22):
post-seed `auth_token`/`ct0` attempts reject even before identity; clearly
classified other cookies are diagnostic-only; ambiguous observations reject.
Ordinary QA and deferred-mutation receipt rules stay unchanged. This is startup
coverage only, with full account/login verification still required in both roles.
Offline tests do not authorize a new retained-QA transfer.

The historical [provisional root-navigation correction](../../docs/x-native-manual-pilot.md#provisional-root-navigation-correlation-2026-10-22)
lets one exact root `/home` navigation wait for matching commit/default-context
evidence during the trusted reload. Only parsed known non-candidate headers may
wait; candidate attempts and truly ambiguous headers still reject immediately.
Unresolved or contradictory pending state cannot publish a startup receipt.

The [post-commit request extension and distinct guard reasons](../../docs/x-native-manual-pilot.md#post-commit-requests-and-distinct-guard-reasons-2026-10-22)
also let bounded same-document resources wait for the default context after a
verified new-root commit. Private diagnostics distinguish binding, capacity,
classification and metadata/completion failures; the first cause remains intact.
Parser rules, observation limits and admission authority are unchanged.

The [network-provenance revision](../../docs/x-native-manual-pilot.md#network-provenance-and-explicit-post-seed-boundary-2026-10-22)
supersedes that document/context coupling for cookie safety. Bounded request
provenance survives the enrollment/seed/startup boundary, with an explicit
post-seed decision epoch. Identity receipts still require the exact new document
and default context. Missing request predecessors and disqualified records stay
distinct in private closed diagnostics; unknown scope and candidate attempts
remain fenced. This authorizes offline implementation and tests only.

The [redirect/reuse discriminator](../../docs/x-native-manual-pilot.md#declared-redirect-versus-undeclared-request-reuse-2026-10-23)
reports same-ID declared redirects separately from undeclared request reuse.
That increment rejected both branches. The subsequent
[per-hop resource correlation](../../docs/x-native-manual-pilot.md#per-hop-resource-redirect-correlation-2026-10-23)
keeps undeclared reuse closed and permits known-other cookie classification only
for fully correlated supported resource hops. Candidate or ambiguous evidence,
missing correlation, and root/account-settings redirects still reject. No
redirect can supply identity authority or authorize another native QA run.

The [early cache preparation and pre-transfer fences](../../docs/x-native-manual-pilot.md#early-cache-preparation-and-pre-transfer-guard-fences-2026-10-23)
require acknowledged cache disabling before enrollment on both owned sessions,
and stop export/write when either guard has already rejected. V5 adds closed
cache-attribute diagnostics; the seed diagnostic envelope preserves the first
guard trace and truthful `not_attempted`/`unknown`/`acknowledged` write status.
This adds no SW bypass, cache clearing, guard reset or broader traffic authority.
Validation remains offline and grants no new transfer permission.

The [first failed redirect predicate and observation epoch](../../docs/x-native-manual-pilot.md#first-failed-redirect-predicate-and-observation-epoch-2026-10-23)
add closed `guardCondition` and immutable `guardEpoch` fields to private V4
diagnostics, with V2/V3 decode compatibility. A structural redirect rejection can
occur without a cookie attempt, including before seed; the legacy
`observed_cookie_write`/`bound_finish` wrapper does not establish either fact.
This is diagnostic-only: unsupported redirects still reject and no native
execution or transfer is authorized.

Manual startup diagnostics also retain the [first cause separately from terminal
failure](../../docs/x-native-manual-pilot.md#first-cause-and-separate-startup-diagnostics-2026-10-22).
Generic early-capture fallback must not hide a prior cookie/scope rejection.
Only closed, value-free categories cross the private diagnostic boundary;
startup receipts and admission requirements are unchanged. Default preparation
remains offline and does not consume permission for a native transfer.

Run `.\scripts\test-native-manual-pilot-adapter.ps1` for the new dev-only private
receipt adapter. It performs named-file standard-library offline tests, four tag
gates and dev/combined Engine compilation without package runtime execution.
Run `8c439688d6f64d95be8f6ee85cb9a917` passed 11 tests/63 subcases and all gates;
the owned temporary build root was removed. Evidence:
`runtime/qa/auth-session-sidecar/native-manual-adapter-8c439688d6f64d95be8f6ee85cb9a917.log.txt`.
Tests cover strict identity/pins, receipt replay, owner drift, absolute expiry,
cancellation/revocation races and private diagnostics. Offline vectors cannot
yield native eligibility. All native/Store authority flags stay false.

The adapter baseline is published as `eb12655`. Its subsequent private startup
producer and manual controller are now connected offline; no native main/broker
provider or activation endpoint is registered. Run
`.\scripts\test-native-manual-pilot-startup.ps1` for concurrent per-role startup
capture, the strict 16 KiB private receipt transport and dispatch/claim/result
fencing. It runs Node projections, named-file Go tests (including an accountless
Node stdio roundtrip), four tag gates and two Engine builds. Canonical run
`9049d389eb4c4669b5f5ad2546edf043` passed 27 Node tests and 10 Go tests/49 subcases;
one subsequent default-off driver-hook test brings the distinct Node set to 28.
Its project-owned temporary build root was removed. Details, logs and the native
startup document/context ownership limit are in the
[connection checkpoint](../../docs/x-native-manual-pilot.md#startup-producer-and-manual-controller-connection-2026-10-21).
Trusted owner/bridge instantiation and separately authorized native QA acceptance
remain unfinished. These helpers open no
Chrome/profile/login/listener, transfer no session and access no Store/user
runtime. The snapshot V1 helper and its evidence below remain unchanged.

Run `.\scripts\test-native-manual-pilot-broker.ps1` for the subsequent two-phase
owner/broker connection. Both role actors prepare at the new startup document
before any identity evaluation, return observed context/frame/loader pins and an
opaque handle, then acquire on the same handle after Go inspects/freezes the
pair. Admission stays closed until the existing manual controller accepts both
roles. There is no post-binding reload or wildcard/stale-document adoption.
This uses the guarded in-process `createOwnedManualStartupOwners` driver hook,
not a new main/runtime activation endpoint.

Accountless run `245501fa56f24dd685b21ca51c26da19` passed 44 Node tests and 7 Go
tests/20 subcases, four tag gates and two Engine builds. Persistent Node stdio
tests cover same-owner preparation/acquisition, reader admission and single
result publication; normal-pause tests reject context loss before evaluation.
Partial/late startup and failed/uncooperative cleanup are covered. Native OS
ownership facts are invented, while document pins are actually derived from
the fixed synthetic document. No Chrome/profile/account/transfer, Store or user
runtime is used. The temporary build root was removed. Details and native
acceptance limits are in the
[owner/broker checkpoint](../../docs/x-native-manual-pilot.md#ownerbroker-startup-binding-2026-10-21).

For actual Windows/Chrome ownership with invented loopback account data, run
`.\scripts\test-native-manual-pilot-chrome.ps1 -VerifyChrome`. Without that switch
the helper runs only three Node boundary tests, four tag checks and standalone
compilation. Chrome execution creates fresh role profiles, checks actual
process/image/creation/parent/Job and physical-directory identities, and freezes
real startup document pins before acquisition. The named-file binary excludes
Engine initialization and Store; exact source/extraction hashes are retained.
Run `881a30d5861441589ed3ad981adb3819` passed both Chrome cases: reader/collector
ownership with bootstrap/last-window handling and one result, plus rejection of
startup context loss. Owned Jobs were verified empty and fresh profiles removed.
Logs/provenance remain in project-local QA; no user click or login is required.

This fixture remains offline for authorization. It contacts no X endpoint, uses
no retained profile and transfers no real session. It neither registers a main
provider nor touches Store or user runtime. Full evidence and limits are in the
[Windows/Chrome checkpoint](../../docs/x-native-manual-pilot.md#windowschrome-synthetic-ownership-checkpoint-2026-10-22).

Run `.\scripts\test-x-manual-pilot-owner.ps1` for the subsequent private X
owner-port composition. Its accountless Node/Go checks exercise concurrent role
acquisition, exact pinned scope, controller work/result fences, null-handle
cancellation and cleanup uncertainty. Four tag checks and exact-source Windows
backend compilation keep the native constructor confined to dev+fixture builds.
The helper runs no Chrome/native QA/retained profile or real session transfer,
has no native switch, and excludes Engine initialization and Store. The fixed
native backend accepts already-owned Windows handles; it never launches or seeds
a session. No main provider or runtime endpoint is registered. Details and the
unmeasured native observer-session boundary are in the
[X owner-port checkpoint](../../docs/x-native-manual-pilot.md#private-x-owner-port-composition-2026-10-22).
Canonical run `915ed92f84984779a7bc03704f386617` passed 30 Node tests, eight Go
tests plus ten subcases, four tag checks and Windows-backend compilation. The
accountless build root was removed; bounded logs and source provenance remain
under project-local QA. This does not qualify native X operation or activate dev.

To prepare the separate retained-X integrated test, run
`.\scripts\prepare-x-manual-retained-qa.ps1` with no arguments. Preparation builds
a source/binary-pinned standalone executable and proves its entry skips before
lease/profile access with all execution gates disabled. It does not open Chrome,
inspect the retained profile or transfer a session. Canonical preparation
`5d87b8f06d9b46f2be3540998d0904bf` passed 14 Node tests, four tag gates,
compilation and the disabled entry. The ready root/plan are retained locally.

`-RunPrepared` is a separate operation requiring a prepared root and fresh
`-TransferAuthorized` approval; it was not used during preparation. It admits one
attempt, one qualified two-cookie transfer and one in-memory result, using only
an existing clean QA profile and a fresh disposable collector. See the exact
[prepared test and limits](../../docs/x-native-manual-pilot.md#prepared-retained-qa-integrated-test-2026-10-22).

## Offline bounded dev snapshot pilot (V1 reference)

The separate pilot model follows
[`aku.x.dev-pilot.snapshot.v1`](../../docs/x-concurrency-dev-pilot-contract.md).
Run `.\scripts\test-concurrency-pilot.ps1` from the repository root. This helper
has no native/browser switch. It uses invented account/owner/process references,
an injected clock, a probe-only synthetic source and an in-memory work/result sink.
It opens no profile, login, browser, network listener, Store/database or user runtime.

The model is private and dual-tagged (`auth_session_dev && auth_session_fixture`).
It is independent from the strict pair authority. Its controller bridge is selected
only by the existing early synthetic main fixture's six `pilot-*` scenarios, with
an explicit fixture grant; there is no native provider or activation endpoint. Its pilot
eligibility can be true while global coverage stays partial, continuity unknown
and refresh unqualified. Native, production, Store and existing `canAuthorize`
flags remain false. A local permission-version chain does not establish X lineage.

The helper checks the new source/tests in all four build-tag modes, runs only
`TestConcurrentPilot*` cases as named-file Go tests, and compiles the dual-tag
Engine without executing its initializers or package tests. Old strict-authority
test helpers support the cross-policy regression; their existing test cases are
not repeated. Build/cache paths stay project-local. Logs use
`runtime/qa/auth-session-sidecar/concurrent-pilot-<run>.log.txt`; the unique owned
temporary build root is guarded and removed after the run.

Tests exercise the absolute 60-second attempt cap, five-second freshness and
joint two-second acquisition/publication limit, same-document renewal without
reload, immutable work tickets, known invalidation versus deferred coverage,
mode/policy isolation, cancellation and bounded public reports. These model
checks cannot qualify native in-place identity/login sources, timing or OS cleanup.

The controller bridge passed 23 pilot tests/66 subcases and all four tag gates
in run `1bd7f9f5e47147a98377dfbaeacb602b`. Run
`.\scripts\test-concurrency-integration.ps1 -PilotOnly` for the six bounded actual
main/broker/Coordinator cases: overlap, renewal failure, policy drift, opt-out,
last close and parent cancellation. Run `e994a879ac934631a23277b6a6336a0a` passed
these cases, all four actual-main build gates, closed receipt/input guards,
untouched config/database sentinels and three default Coordinator regressions.
It uses existing Managers with invented processes and an active capture lease;
collector PID/generation/driver/lease preservation is checked during reader open
and renewal. Cleanup verifies no owned references or active leases remain.
Home-parser diagnostics are deferred by the user's MVP decision. This path opens
no Chrome, profile, login, Store, listener or user runtime. Detailed claims and
remaining native boundaries are in the
[pilot integration checkpoint](../../docs/x-concurrency-dev-pilot-contract.md#pilot-controller-and-main-fixture-integration-2026-10-21).

Full integration run `a50bfe651871439e872e6db89ae592c6` also passed all 26
actual-main scenarios, four build gates and three default Coordinator tests after
the shared fixture-clock correction. Existing overlap and strict-authority behavior
remain covered. No native source or retained-profile probe was repeated.

Accepted run `a718f7419b5440afaa41065e48e5071e` passed 18 top-level tests and
57 subtests, four tag gates and combined Engine compilation. A twelve-second
fake-clock capture used four renewals and ten per-role observations, preserving
the original ticket version one through current evidence version five. Its log
follows the prefix above; the owned temporary build root was removed. See the
[implementation checkpoint](../../docs/x-concurrency-dev-pilot-contract.md#offline-pilot-model-implementation-checkpoint-2026-10-21).
Expiry is checked at model API/report barriers; no native renewal scheduler runs.
Source callbacks must cooperate with their bounded context; late outputs cannot
publish authority, but arbitrary blocked callbacks are not forcibly interrupted.

## Pair-authorized main and Chrome fixtures

The existing fixture providers now accept explicit `authorized-*` scenarios.
They select the private pair authority through actual main early dispatch,
the framed broker, Coordinator and capture Managers. The ordinary runtime and
single-tag builds still cannot select this path. The older scenarios keep their
legacy authority and remain a separate compatibility baseline.

```powershell
.\scripts\test-concurrency-authorized-controller.ps1
.\scripts\test-concurrency-integration.ps1
.\scripts\test-concurrency-chrome-integration.ps1
.\scripts\test-concurrency-chrome-integration.ps1 -VerifyChrome -Scenario @(
  'authorized-renewal', 'authorized-renewal-delay', 'authorized-renewal-failure',
  'authorized-account-drift', 'authorized-policy-drift', 'authorized-coverage-gap',
  'authorized-opt-out', 'authorized-last-close-denied', 'authorized-parent-process-death'
)
```

The Chrome helper remains offline unless `-VerifyChrome` is present; its default
live selection remains the eight legacy cases. The explicit selection above uses
fresh loopback-only profiles and automatic synthetic login, with no manual click.
There is no native account/session input, Store/database access, retained QA
profile, registry write or user-runtime activation.

The authorized long-capture case holds one protected request for at least ten
seconds on the real clock. A fixture-owned 1.5-second timer automatically renews
both roles while that original request and ticket remain active. A bounded Node
dispatcher allows one pending collection alongside serialized control/probe
commands, with a maximum 64 waiting commands. Acceptance requires at least three
renewals, probes during collection, the original evidence-version-one ticket,
unchanged collector PID/generation/driver/lease, retained reader windows, and
rejection of stale/duplicate completion. The five-second proof age and two-second
binding window are unchanged.

Only the invented loopback service has a local, controlled lifecycle witness.
It holds its source mutation lock through dispatch/claim/result commit, rejecting
scope/account/policy/owner/revision drift and coverage gaps. It neither extends
evidence timestamps nor proves native X cookie-writer coverage. Timer cancellation
and joining precede final retirement. Chrome death testing still requires empty
owned Jobs and settled pinned handles before any emergency fallback can count as
cleanup; unsuccessful cases retain their profile roots.

Strict receipts retain `canAuthorize=false` and `nativeQualified=false`. Authorized
Chrome receipts additionally record renewal/probe counts, original/final evidence
versions, protected-request duration and stale/duplicate checks. See the
[authorized integration checkpoint](../../docs/x-concurrency-mvp2-integration-design.md#pair-authorized-main-and-chrome-integration-2026-10-21)
for accepted runs and limits.

Accepted core run `d93664019d734a86be7988763e7fd8c6` passed 51 top-level tests
and 87 subtests; main run `4fba3d718e424999aa4b8cd9feb57be8` passed 20 scenarios
and three Coordinator regressions. Chrome offline run
`4ff65aee51474fa3b72f61a2cfb8271e` passed nine Node cases and strict receipt,
input, build and root gates. Actual Chrome run
`8357c06e63fe4a84ae92188c2d022d3f` passed the nine authorized cases plus legacy
`overlap-last-close`, including actual QA parent-process death. Its long capture
measured 11,104 ms, seven automatic renewals and 14 in-flight probes, with the
original ticket version one completing against evidence version eight.
Successful profile/build roots were removed. The first failed live root remains
retained; its exact scope and corrective evidence are recorded in the checkpoint.

## Offline controller with pair authorization

Run `.\scripts\test-concurrency-authorized-controller.ps1` from the repository
root to exercise the private pair authority through the existing synthetic
controller. The selection is an explicitly granted in-process fixture seam,
not a main option, native provider or runtime setting. The legacy nil-seam path
remains separate, including the previously accepted Chrome ownership fixture.

The helper checks source/test exclusion in all four build-tag modes, compiles
each original standalone named-file test pair without executing its cases, then
runs the combined controller, authority and bridge regressions once. It also
compiles the dual-tag Engine without executing its initializers or package tests.
Temporary files and build caches stay project-local. Synthetic logs use
`runtime/qa/auth-session-sidecar/concurrent-authorized-controller-<run>.log.txt`.
There is no native-run switch, browser, Store/database, login, network listener
or user-runtime access. In-memory result ordering is not durable Store acceptance.

Accepted run `4ebf169674ee491fb051999a89074b8b` passed 48 top-level tests and
78 subtests, all tag/standalone compatibility gates and combined Engine compile.
The owned temporary build root was removed. Tests exercise a twelve-second
fake-clock capture through three renewals, unchanged collector stamps, original
ticket evidence versions, blocked-renewal invalidation and cancellation at the
final claim/commit barrier. Disable retains reader windows; their final close
finishes cleanup. A focused legacy fake-probe synchronization fix removes an
`AfterFunc` scheduling race without changing production cancellation semantics.
See the [controller wiring checkpoint](../../docs/x-concurrency-mvp2-integration-design.md#synthetic-controller-authorization-wiring-2026-10-21)
for the exact scope and remaining native/durable integration limits.

## Offline pair authorization and evidence renewal

Run `.\scripts\test-concurrency-authorization.ps1` from the repository root.
This separate helper qualifies the
[pair authorization contract](../../docs/x-concurrency-mvp2-integration-design.md#production-pair-and-evidence-renewal-contract-2026-10-21)
with invented enrollment, owner, derivation and lifecycle evidence, an injected
clock and an in-memory work/result sink. It has no native-run option and does
not wire the authority into main, the broker or the existing Chrome controller.

Only `development_concurrent_authorization.go` and its regression source execute
in named-file Go tests. The helper checks both files against all four build-tag
modes and compiles the combined dual-tag Engine package without running its
initializers or package tests. It uses project-local temporary/cache paths and
keeps a synthetic log under
`runtime/qa/auth-session-sidecar/concurrent-authorization-<run>.log.txt`.
No Store/database, browser, login, credential, listener or user runtime is opened.
Native/production authorization remains unavailable even when fixture permits
and uninterrupted renewal succeed. Model atomicity is not durable transaction
acceptance; native continuity and source latency remain separately unqualified.

Accepted run `40b28739e91f4dff9d2d7127c05d7463` passed 19 top-level tests and
28 subtests, four tag gates and combined Engine compilation. A twelve-second
fake-clock capture completes through three uninterrupted renewals; gaps, expiry,
revision change and concurrent invalidation deny authority. The temporary build
root was removed. Expiry uses explicit clock checks, and evidence callbacks must
cooperate with their two-second context; no native scheduler is qualified.

## Chrome-backed main, broker and Coordinator integration

The separate `scripts/test-concurrency-chrome-integration.ps1` helper tests the
new default-off provider `synthetic-concurrent-chrome-fixture`. Its default mode
is offline. Add `-VerifyChrome` to run the built-in cases using fresh reader and
headless profiles, automatic loopback synthetic login and two independently
owned Windows Jobs. No manual login/click is needed. `-Scenario` limits a causal
recheck to selected built-in cases.

```powershell
.\scripts\test-concurrency-chrome-integration.ps1
.\scripts\test-concurrency-chrome-integration.ps1 -VerifyChrome
```

This adapter uses the same private controller, actual main early dispatch,
Coordinator, capture Managers and framed broker route. It handles no native
session data and opens no Store/database, user profile, production runtime,
registry key or activation endpoint. See the
[Chrome ownership checkpoint](../../docs/x-concurrency-mvp2-integration-design.md#chrome-ownership-adapter-increment-authorized-2026-10-21)
for evidence and limits. Unknown cleanup retains the failed disposable roots;
emergency cleanup cannot turn a failed lifetime case into a pass.

Eight Chrome scenarios passed across runs `43ee70a67f9342e6ad1909b0ca96bf94`
(overlap/last-close), `3932122bc03645128931a851a68da108` (six failure/lifetime
cases) and `5af9217aba664ddd96fa256fe1b0fd7a` (actual parent-process death).
Accepted live runs executed outside the restrictive sandbox after sandbox
startup exited before browser readiness. Successful roots were removed; four
earlier failed-run profile roots remain retained as listed in the checkpoint.
The physical junction-negative test was unavailable in the sandbox; lexical
alias/reuse gates passed and the runtime no-reparse guard remains enforced.

Final offline run `7f37e0d6a6e043a6bea0cb58cd0413f4` passed three Node tests,
one Go input test, 17 malformed-report rejections, four actual-main builds and
seven rejection/root gates. The report validator rejects unknown fields, false
cleanup/overlap, authority claims, missing/wrong scalar types and invalid counts.
The helper build root was removed without launching Chrome in this final run.

## Synthetic main, broker and Coordinator integration

Run `.\scripts\test-concurrency-integration.ps1` from the repository root. This
offline helper builds actual main in four tag modes and tests the explicit
`--dev --dev-session-reuse=synthetic-concurrent-fixture` early fixture dispatch.
It uses only built-in synthetic input and two capture Managers with invented
Processes, framed broker buffers and memory work/results. It opens no Store,
database, Chrome/profile, native messaging host, listener, registry entry or UI.
Missing configuration/database sentinel paths must remain absent. Owned QA
executables are hidden, bounded and removed after exit; the synthetic log remains.
The [wiring checkpoint](../../docs/x-concurrency-mvp2-integration-design.md#main-broker-and-coordinator-synthetic-wiring-2026-10-21)
records validation and limits. This does not activate the user's dev runtime or
establish native foreground, Chrome ownership or durable admission guarantees.

Accepted run `e7e4f5da31f442a59fcf09ce3c00a2a0` passed four actual-main build
modes, 11 rejection cases, 11 synthetic scenarios, the absent-config/database
sentinel check and three ordinary Coordinator regressions. Its log is
`runtime/qa/auth-session-sidecar/concurrent-integration-e7e4f5da31f442a59fcf09ce3c00a2a0.log.txt`;
the unique temporary build root was removed. The earlier controller-only evidence
below is reused because that controller and its helper are unchanged.

## Offline synthetic pair controller

`scripts/test-concurrency-controller.ps1` is the separate controller-only
regression helper. It has no native-run option and uses no account, profile,
browser, Store or database. It checks the dual build-tag boundary, then tests
only the named controller/test files with fake lifecycle/verifier ports and an
in-memory work/result sink. No production or main/broker integration is enabled.
The [MVP-2 implementation checkpoint](../../docs/x-concurrency-mvp2-integration-design.md#offline-controller-implementation-checkpoint-2026-10-21)
records scope and remaining integration limits. These model tests do not replace
the existing actual-Chrome overlap/cleanup evidence below.

Offline run `106377b1916348b3b40b0b81c32f1322` passed four build-tag gates,
14 top-level tests and 34 subtests. Its synthetic log is retained under
`runtime/qa/auth-session-sidecar/concurrent-controller-106377b1916348b3b40b0b81c32f1322.log.txt`;
the owned temporary build root was removed.

## Separate retained-X short concurrency feasibility (2026-10-20)

Latest accepted result: opt-in `-SynchronizeCheckpoints`, combined with
`-RunNativeQA -ShortConcurrency -AllowQASessionTransfer`, aligns only the after
pair at existing guarded ordinary breakpoints. Each validated role waits at most
1,000 ms for the other before natural page execution continues; timeout rejects
acceptance and does not expand the five-second capture budget. No X response is
held/replayed and no extra cookie/header is introduced. The synchronized wrapper
reports rendezvous state, reader-window state and private account comparison
independently while retaining the original non-authorizing V1 experiment.

Offline run `10997e6cb6c541be95ce881e4983964b` passed 184 Node and 14 Go
top-level tests. After fresh explicit approval for one repeat, native run
`90f3e12182b2494ba7933511ac3150a5` repeated that preflight and returned
`short_overlap_observed`: both roles reached the rendezvous, all four fresh
account/login checks matched, the reader target/window was preserved and the
protected request lifetimes overlapped by 447.682861328125 ms. The experiment
took 11,478.8404 ms, with a 15.65-second complete native case. The exact seed
was acknowledged once, without another login. Collector context/root and complete
Job cleanup, retained lease release and disposable-root removal passed. The
sanitized report is
`runtime/qa/auth-session-sidecar/x-qa-source-90f3e12182b2494ba7933511ac3150a5.report.json`.
This closes short-QA feasibility only. Cookie writes were observed in all four
original V1 samples; binding/continuity remain unknown, refresh unqualified and
Store/native admission unauthorized. The one-repeat approval is consumed. No
main login, Store or user-runtime process was touched.

This opt-in native QA path is separate from the loopback fixture described below.
It reuses only the identity-pinned retained QA profile and creates a fresh
headless profile plus disposable BrowserContext under an owned fixture Job.
The user approved deferring cookie-mutation/continuity qualification for a short
experiment. That does not grant Store authorization or main-dev activation.

Run `scripts/test-x-qa-source.ps1` for offline validation.
Run `scripts/test-x-qa-source.ps1 -RunNativeQA -PrepareShortConcurrency` for a
blank headless collector lifecycle and a fresh reader account/login checkpoint;
this mode cannot read/import cookies. The real short experiment additionally
requires `-ShortConcurrency -AllowQASessionTransfer` and specific user approval.
Its [fixed manifest and experiment limits](../../docs/x-concurrency-mvp-experiment.md#short-feasibility-milestone-approved-deferral-2026-10-20)
allow exactly `auth_token` and `ct0`, once, reader to isolated headless context.
Never broaden to all cookies when a request fails. No main login, Store policy
or user-runtime process is involved. A short pass reports continuity unknown,
refresh unqualified and no production authority. Preparation and real-transfer
results must be reported separately; passing offline tests is not native proof.

Preparation evidence: canonical offline run `c8d29cdb8a3348e7af30672f7b626c2d`
passed 180 Node and 13 Go top-level tests. Native preparation run
`711de0092ff844ffaf936879b39973fa` repeated that preflight and completed the
native case in 6.91 seconds. The fresh blank headless process used a distinct
profile and disposable context; its image/Job membership and context/root
closure passed. The retained reader checkpoint observed current-self and one
fresh completed settings 200/body match, with `passed:true` only in the separate
short wrapper. Its V1 binding remains unknown because a cookie write occurred.
No cookie-store read or export/import was executed. Whole-Job cleanup, clean
lease release and disposable-root removal passed. The sanitized report is
`runtime/qa/auth-session-sidecar/x-qa-source-711de0092ff844ffaf936879b39973fa.report.json`.
This validates preparation, not concurrency or two-cookie sufficiency.

The user then explicitly authorized one bounded real-QA transfer. Run
`45f648c0cfd348239ac95a07b79ec192` completed the native case in 13.73 seconds
(the bounded experiment itself took 10,344.0848 ms). The exact two-cookie seed
was acknowledged. All four reader/collector checkpoints passed: each observed
the private current-self candidate and fresh completed settings 200/body match.
All four passed the private same-account comparator before their completed count
was incremented. The after pair ran concurrently at the harness level, but the
Chrome request intervals did not overlap (`overlapMS:0`), so the recorded outcome
is `overlap_not_observed`, not `short_overlap_observed`. The V1 report conservatively
keeps `accountMatch:unknown` unless the entire experiment succeeds; do not relabel
that captured field. The final target/window comparison was not reached because
the overlap gate failed. The after-reader checkpoint does prove that the retained
reader remained authenticated after headless activity in this sampled interval.

This proves sampled two-cookie sufficiency and successful before/after access
from both separately running Chrome processes without a second login. It does
not prove literal simultaneous settings requests, refresh, durable continuity,
full worker coverage or production admission. All four original V1 bindings remain
unknown after observed cookie writes. Collector context/root and complete owned
Job cleanup passed, the retained lease was released cleanly and the disposable
root was removed. No raw values/body/native IDs were reported or retained by the
helper. The report is `runtime/qa/auth-session-sidecar/x-qa-source-45f648c0cfd348239ac95a07b79ec192.report.json`.
The one-transfer approval is consumed; no automatic repeat was performed.

This opt-in experiment launches two **new, isolated** Chrome profiles: a minimized
headed owner/reader and a true-headless collector. A fresh loopback HTTP server
provides invented accounts and credentials. It never reads production config,
connects to production CDP, reads a social cookie store, calls Sidecar APIs, or
loads extensions. The broker rejects non-loopback origins. No production module
imports this experiment.

## Offline concurrency harness (2026-10-19)

The concurrency-first MVP now has a separate offline harness in
`x-concurrency-harness.mjs`, exercised by `x-concurrency-harness.test.mjs`.
Its dependency-injected adapters simulate owned reader and collector contexts;
they do not launch Chrome, connect to X, open a listener, inspect a retained
profile or access Store. Loopback origin and explicit synthetic provenance are
mandatory. There is no native adapter or native-transfer opt-in in this harness.

From AkuSidecar, run only the offline suite:

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\x-concurrency-harness.test.mjs
```

This validates one-way manifest-bound transfer, distinct owner/context binding,
bounded overlapping adapter reads, stale-result rejection and cleanup behavior.
In-memory overlap proves harness scheduling, not real browser overlap or native
X compatibility. Injected observation capabilities and account evidence remain
synthetic inputs; reports cannot authorize work or establish native refresh or
session lineage. Arbitrary in-process adapters are trusted test code, not a
sandbox for untrusted code.

The exported `runXSyntheticConcurrencyHarness(options)` consumes explicit
synthetic owner/reader evidence, a coverage record, an exact cookie manifest,
an injected monotonic clock, cancellation/deadline bounds and adapter callbacks.
Adapters provide observation subscription, reader inventory, collector creation,
cookie selection/seed, paired reads and collector cleanup. There is no reverse
transfer callback. The returned receipt is an immutable snapshot;
`cleanupStatus()` reports subsequent late-effect cleanup, while
`waitForCleanup()` is only useful when the test controls eventual settlement.
Pending or failed cleanup must not be reported as complete.

The [minimal native experiment](../../docs/x-concurrency-mvp-experiment.md)
still requires a qualified native transfer manifest, source evidence and separate
execution permission. The browser commands below document older synthetic Chrome
experiments; the offline continuation did not execute or newly authorize them.

## Offline native-plan contract and evidence combiner (2026-10-19)

`x-native-plan-offline.mjs` exercises the selected
[native evidence/transfer plan](../../docs/x-concurrency-mvp-experiment.md#selected-native-evidence-contract-2026-10-19)
using trusted invented inputs only. It imports the existing pure bootstrap
projector and strict JSON parser, not a browser, filesystem, network or Store API.
It cannot read a retained profile, transfer state, enroll a provider or authorize
production work. Native-source qualification assertions are not accepted.

```powershell
# Only the new contract/combiner regression tests:
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\x-native-plan-offline.test.mjs
# Canonical combined offline preflight; no Chrome or network listener:
.\scripts\test-x-concurrency-fixture.ps1 -NoChrome
```

`validateXOfflineContract(plan)` returns a frozen, value-free validity report.
`createXOfflineEvidenceCombiner(plan)` validates and privately snapshots the plan;
its `evaluate(evidence)` method consumes one attempt even on failure, and `close()`
discards an unused attempt. Attempt IDs cannot be reused in the same module
instance; its bounded registry stops after 1,024 attempts rather than evicting
replay protection. This is process-local test behavior, not durable native replay
protection. Both public reports and errors use fixed fields/categories.

The closed synthetic plan pins distinct owner/profile bindings, one exact settings
route, expected account ID, synthetic source qualification and the two exact
`auth_token`/`ct0` attribute tuples. Invented values must have a `synthetic:`
marker; the marker and provenance are misuse guards, not proof about arbitrary
caller-supplied data. Each complete serialized tuple is bounded to 8 KiB, the pair
to 16 KiB. No cookie values are retained by the combiner or returned in receipts.

Each role supplies a scoped bootstrap sample, bounded settings response and
synthetic coverage witness. The private comparison requires the actual validated
IDs to match, not two shape-only DTOs. Five-second freshness, exact bindings,
strict settings types/JSON and mutation/gap closure are mandatory. Matching IDs,
HTTP 200 or `authenticated:true` cannot replace qualified protected-source
semantics. A synthetic positive/negative control exercises that rule; it is not
native proof. `isActive` is never a login flag. All results retain
`canAuthorize:false`, `nativeQualified:false`, `realAuthTested:false` and
`refresh_not_observed`; synthetic coverage is not native session lineage.

## Retained QA source and cookie-attribute adapter (2026-10-19)

The dedicated `x-retained-source-qualification-driver.mjs` entrypoint extends
only the private QA transport. The Go fixture must supply the existing retained
profile lease after Node enters its kill-on-close Job. A new/missing profile,
foreign/replaced directory, dirty lease or expired Home login stops the run;
the adapter never initiates login, logout or profile migration. Ordinary modes
cannot issue this diagnostic or read cookies. No transfer command exists.

```powershell
# Default: offline projections, protocol denials, lease and Go decoder checks.
.\scripts\test-x-qa-source.ps1

# FUTURE native run only after explicit cookie-read/execution approval:
# .\scripts\test-x-qa-source.ps1 -RunNativeQA -AllowCookieMetadataRead
```

The native path is prepared, not run by this checkpoint. It would reopen only
the retained QA profile, require Home within thirty seconds, then observe one
twenty-second Home reload. One completed uncached main-frame settings GET may
be read once (16 KiB decoded body, strict JSON limits). The exact route is retained
as a fixed enum: `web_i_api`, `api_v1` or `api_i_api`, mapping respectively to
the three reviewed routes in the native contract. Query strings and values are
not exported. Frame mismatch, reused IDs (including OPTIONS collisions), route
changes, duplicate candidates, cache ambiguity and observation overflow reject
the sample. A separate OPTIONS preflight is ignored.

After a projected response and a fresh Home check, one fixed scoped
`Network.getCookies` call uses Home and that pinned route. The bounded CDP decoder
handles at most 262,144 bytes per frame; the projection accepts at most 64 cookies.
CDP includes values in those replies, including incidental nonallowlisted cookies.
Native approval must explicitly cover that transient private decoding even though
only attributes are retained. Do not describe this as a value-free browser read.
The projector never returns values, credential hashes or nonallowlisted cookies;
its selected names are exactly `auth_token` and `ct0`. It rejects duplicate/name
shadows, unknown attributes, partitioned or unsupported state and oversized tuples.
Cookie values are never persisted by the helper, sent to another context or
placed in arguments/environment/logs. Parsed strings can remain until garbage
collection; no secure-erasure or disk-free Chrome guarantee is made.

The fixed report separates missing, ambiguous, scope mismatch, unsupported,
expired and candidate tuples. Candidate metadata preserves Secure/HttpOnly,
SameSite absence, fractional expiry, session status and known CDP priority/source
attributes. Those extra attributes still need a reviewed native import mapping;
this adapter does not update the offline transfer schema to ignore them.
An independent closed Go decoder rejects secret-bearing/unknown fields, missing
guards, authorization claims and invalid attribute combinations before persistence.

This phase records source availability and whether the bounded body matches the
selected settings types. It does **not** run a guest/signed-out control, compare
private account IDs or establish native lineage. Consequently authentication
stays `unknown`, negative control stays `not_run`, and `canAuthorize` plus
`sessionTransferred` stay false. A matching body/cookie pair is not qualification
of protected login semantics. No Store admission or production setting is added.
The owned Chrome tree/Job must exit before the report is saved and the lease is
marked clean. Only the disposable control root is removed; the retained profile
remains. Unverified cleanup retains the root/dirty ownership state for diagnosis.

Offline run `c848552abe5d4dd182a2db61ce3a9408` passed 37 Node tests and seven Go
top-level cases (including isolated lease/window subtests), with no failures or
skips. It opened no Chrome, listener or retained profile and removed its temporary
root. Its sanitized log is under `runtime/qa/auth-session-sidecar/x-qa-source-<run-id>.log.txt`.

### Native metadata checkpoint (2026-10-19)

The owner authorized the twelve-file preparation commit/push (`a85cb7f`, remote
verified), followed by this retained-QA diagnostic only, without transfer.
Two harness-order findings required local fixes after that commit: initial Home
polling was incorrectly reused after its observation gate closed, and the final
bridge-only wait preceded Job cleanup and could consume the outer timeout.
The fixed path uses the same bounded Home expression as a separate final read,
keeps initial polling closed, bounds graceful close to seven seconds, and saves
the decoded report only after complete owned-Job termination is verified.
Regression tests cover ordering, skipped cookie reads and denied persistence on
unverified cleanup. These fixes are not part of the published preparation commit.

Native run `e44f91354b004039bf2a48ba9c7d69d2` then passed in 22.85 seconds, after
38 Node and seven Go offline cases. Home succeeded on the same retained QA
profile without a manual login. Exactly one natural settings GET and one body
read were observed at `api_v1` (`https://api.x.com/1.1/account/settings.json`).
The bounded body was projected, but `bodyContractMatches:false`; this report
does not identify which of the six fixed field types differed. Do not guess or
relax the success predicate from a generic HTTP 200.

Both selected tuples were present as candidates at `.x.com`, path `/`, domain
scope, Secure, persistent fractional expiry and no partition. `auth_token` had
HttpOnly and SameSite None; `ct0` had no HttpOnly and SameSite Lax. Both reported
priority Medium, source scheme Secure and source port 443. These are attribute
observations, not credential sufficiency, identity, native lineage or transfer
qualification. Values were not retained or transferred. Authentication stays
unknown and the signed-out control remains not run.

The root acknowledged graceful close; the full owned Job was then stopped and
verified empty. The retained lease returned clean and the disposable control
root was removed. Sanitized artifacts are
`runtime/qa/auth-session-sidecar/x-qa-source-e44f91354b004039bf2a48ba9c7d69d2.{report.json,log.txt}`.
The earlier runs `34a91c28540f450cb554072406ef2033` (post-Home gate) and
`7bc9e15e6fb9483095faec16d6380cad` (release wait) failed; both Jobs were cleaned,
both leases returned clean and no raw credential/body report was retained.
Main login, Store and the user runtime were outside all three runs.

### Fixed settings-type checkpoint (2026-10-19)

The eight-file finalization/checkpoint delta was subsequently reviewed and pushed
as `9ad4ab9` (remote SHA verified). The owner then authorized identifying the
settings mismatch through a bounded projection on the same retained QA profile.
The local diagnostic envelope is now `aku.x.qa-source-qualification.v2`, with
`aku.x.qa-source-capture.v2` adding only `settingsFields`: the six fixed names,
each with an enum `kind` and boolean `matches`. V1 artifacts remain historical;
they are not silently interpreted as V2 results. The independent Go decoder
rejects unknown keys/types, missing pairs, reflected values and inconsistent
match flags before persistence. No production API or verifier schema changes.

```powershell
# Explicitly authorized native type projection only; no cookie-read permission:
.\scripts\test-x-qa-source.ps1 -RunNativeQA -SettingsTypesOnly
```

SettingsTypesOnly dispatches the fixed `qualify_settings_types` RPC and disables
the cookie-reader callback even after Home succeeds. Combining it with
AllowCookieMetadataRead is rejected. The default command remains offline only.
Body/frame/correlation/one-use/cleanup limits remain unchanged; object/array
contents and unknown settings keys are not projected. Source errors, malformed
or ambiguous responses retain null diagnostics instead of inventing field types.

Run `2fc104cde2444bc891a2f105b3da08a4` passed forty Node tests, seven Go offline
cases and the native QA case (24.22 seconds). Home reused the retained login.
One `api_v1` settings GET and one body read produced:

| Fixed field | Observed kind | Existing predicate |
| --- | --- | --- |
| `language` | string | match |
| `time_zone` | object | mismatch: only string/null accepted |
| `protected` | boolean | match |
| `always_use_https` | boolean | match |
| `discoverable_by_email` | boolean | match |
| `discoverable_by_mobile_phone` | boolean | match |

No actual timezone, nested key, setting value, ID or raw body was exported. The
cookie result is null: no cookie read or transfer occurred. The predicate was
deliberately left unchanged, so bodyContractMatches remains false, authentication
unknown, negativeControl not_run and canAuthorize/sessionTransferred false.
The root acknowledged close, the entire Job was verified empty, the retained
lease returned clean and the disposable root was removed. Sanitized report/log:
`runtime/qa/auth-session-sidecar/x-qa-source-2fc104cde2444bc891a2f105b3da08a4.{report.json,log.txt}`.
Contract adjustment is a separate next decision; accepting object-shaped
timezone metadata alone would still not prove authenticated-source semantics.

### Timezone contract and empty-context control (2026-10-20)

The nine-file fixed-type delta above was reviewed and published as `ae17d91`
(remote SHA verified). Under subsequent authorization, the QA source/root schemas
are now V3: `time_zone` accepts object, string or null. Arrays, booleans, numbers
and absence fail. No nested timezone keys/values are exported. The independent
Go decoder recomputes the same predicate; older V2 reports remain historical,
not silently reinterpreted. The synthetic combiner uses qualification version
`account-settings-protected.fixture.v2`; native authority is still unsupported.

```powershell
# Offline regression preflight, including timezone and guest-control guards:
.\scripts\test-x-qa-source.ps1
# Explicitly authorized retained-QA control; no cookie metadata/value reads:
.\scripts\test-x-qa-source.ps1 -RunNativeQA -AuthenticationControl
```

AuthenticationControl is mutually exclusive with cookie inspection and
SettingsTypesOnly. The fixed `qualify_auth_control` RPC first requires one fresh,
matching settings projection from the retained owner's natural Home reload. It
then creates one empty BrowserContext, attaches a new about:blank page, enables
bounded metadata observation and navigates once to Home. Only the pinned
`api_v1` settings route participates. No credential copying, cookie/body read
in the guest, constructed API call, header replay or logout is available. The
guest command guard rejects extra parameters, other contexts/URLs and replays.
The context must be disposed and the whole owned Job verified empty before
saving the strict, independently validated comparison report. The retained QA
Home is checked before and after the comparison; no user login is requested.

The wrapper `aku.x.qa-auth-control.v1` preserves separate signedIn and guest
results. No request means `not_observed`, not an authentication rejection.
HTTP 200 remains `http_200_unqualified`; HTTP 401 is only
`authentication_rejection_candidate`, because metadata alone cannot exclude
every anti-abuse failure. Incomplete, cached/ambiguous and other status responses
remain separate. All results retain authentication unknown and authority/transfer
false. The nested signedIn negativeControl stays not_run because that individual
capture has no control; the wrapper records the actual comparison separately.

Offline run `a219c298ab794daf8c4ebe723e15d5e0` passed 67 Node and seven Go
top-level cases. Native run `51e6e4560844477bb16fd819347600b1` reused the QA
login and passed in 49.44 seconds. One api_v1 GET and one signed-in body read
matched all six types, including object-shaped time_zone. The guest made zero
matching requests: status/freshness remain null, body reads zero, completed
false. Home before/after and contextDisposed were true, cookies null, no session
transferred. Source login semantics and native continuity remain unqualified.
Chrome's normal-close acknowledgment was false; the enclosing Job was then
terminated and verified empty. The lease returned clean and the disposable root
was removed. Main login, Store and user runtime were untouched. Sanitized report:
`runtime/qa/auth-session-sidecar/x-qa-source-51e6e4560844477bb16fd819347600b1.report.json`.

Final offline run `d5fbcba0686342f29fdc280137f5fca5` passed 68 Node/seven Go
cases after adding entrypoint denial checks and explicit non-authorizing status
decoder cases. The observed native `not_observed` path was unchanged; no second
native run or manual login was needed. An earlier test expansion exceeded the
existing eight-command input-chunk limit; splitting that test into separate
bounded invocations fixed the test without relaxing the driver guard.

This finishes the authorized natural-Home control with an inconclusive result.
Do not repeat it expecting absent requests to become proof. The next decision is
a separately scoped protected-source comparison mechanism that can actually
produce a meaningful guest response, or replacement of this login-source
candidate. Session transfer remains unauthorized.

### Direct-navigation A/B checkpoint (2026-10-20)

The 13-file timezone/empty-Home delta was published as `5e2eaa9` (remote SHA
verified). Subsequent authorization adds one deliberately fixed direct-navigation
comparison, not endpoint discovery or production authentication integration:

```powershell
# Offline only by default:
.\scripts\test-x-qa-source.ps1
# Explicit authorization required; retained QA login is reused:
.\scripts\test-x-qa-source.ps1 -RunNativeQA -DirectAuthenticationAB
```

The closed `qualify_direct_ab` RPC accepts only its owner binding. A opens one
about:blank tab in the retained QA context; B opens one tab in a fresh empty
BrowserContext. Each navigates once to the previously observed, pinned
`https://api.x.com/1.1/account/settings.json`. Browser-managed credentials stay
within their own context. No cookie getter/setter, header extraction/replay,
arbitrary URL/expression, profile copy, login/logout or transfer operation is
available. The original QA Home tab is preserved and checked before/after.
The earlier Home-only control and its schema are unchanged.

Each page observes at most 4,096 network events over ten seconds and selects
one same-frame GET with no redirect/duplicate/cache ambiguity. The optional
one-use body read requires a completed fresh JSON response, a 16 KiB encoded
length ceiling and status 200/401/403/429. Its four-second read deadline and
strict decoded-body/depth/token limits also apply. HTTP 400, HTML, failed,
oversized and incomplete responses remain metadata only. Body projection occurs
inside private CDP dispatch before promise resolution; only settings_match,
error_envelope, challenge, other_json or unreadable categories are returned.
No IDs, setting values, arbitrary error codes/messages, headers or raw bodies
are exported. Chrome may abort an error navigation; this does not relax the
completed-response prerequisite for body access.

The new closed report `aku.x.qa-direct-ab.v1` contains separate A/B metadata,
optional value-free body categories, Home checks and verified tab/context
disposal. Go independently validates fields, cardinalities, status/body limits
and recomputes the outcome before saving, after complete owned-Job cleanup.
Both observations run even if A fails, to record the bounded comparison. Only
fresh HTTP 200 plus settings_match on A and surviving QA Home can pass the
positive discriminator. Two 401s, a failed A or lost Home cannot qualify it.
Even a positive A with a JSON error-envelope 401 on B is only
negative_response_candidate: route authentication semantics/identity/continuity
are not automatically qualified. All reports retain authentication unknown,
canAuthorize false and sessionTransferred false.

Initial offline run `e0046fdf7bba4694b07f64ab4239e8bb` passed 75 Node/eight Go
top-level cases. Native run `ad7d6db6db2b4386bafb8bb78d5ebc34` passed the harness
in 5.06 seconds and produced two actual server responses:

| Observation | HTTP | Completion | Body reads | Classification |
| --- | --- | --- | --- | --- |
| A: retained QA context | 403 | aborted / candidate_rejected | 0 | positive control failed |
| B: empty context | 400 | completed / other_response | 0 | unqualified response |

The paired outcome is positive_control_failed. This proves direct navigation
reaches the selected operation in both contexts; it does not prove that this
mechanism is suitable for authentication comparison. The reason for the 403/400
is unknown; no header/credential/challenge cause is inferred. No raw body or
cookie was read, no extra login requested, and no state transferred. QA Home
before/after, A tab close, B context disposal, root graceful-close acknowledgment
and complete owned-Job cleanup were all true. The retained lease returned clean
and the disposable control root was removed. Main login, Store and user runtime
were untouched. Sanitized report:
`runtime/qa/auth-session-sidecar/x-qa-source-ad7d6db6db2b4386bafb8bb78d5ebc34.report.json`.

Final offline run `8eecdff9333047fdb3dd745db683948e` passed 76 Node/eight Go
cases, including the observed aborted-403/completed-400 pair and dedicated RPC
denials for caller URLs/headers. The implementation used for the native run was
unchanged; only regression coverage and documentation were added afterward.

Stop this direct-navigation feasibility branch with that result. Do not repeat
login or reconstruct/add application headers to turn it into a passing test.
The next design decision must compare the already successful natural web-client
request path, or select another qualified source, under a separately reviewed
boundary. Native login semantics and continuity remain unresolved within MVP-1.

### Natural web A/B/A checkpoint (2026-10-20)

The direct-navigation 11-file delta was reviewed and published as `fc6bf3e`
(remote SHA verified). Subsequent authorization adds a browser-local, request-
specific no-cookie comparison on the previously successful natural web path:

```powershell
# Offline only by default:
.\scripts\test-x-qa-source.ps1
# Explicit retained-QA authorization; no login prompt or session transfer:
.\scripts\test-x-qa-source.ps1 -RunNativeQA -WebAuthenticationABA
```

The closed `qualify_web_aba` RPC accepts only its owner. A fixed main-world hook
is installed only for the owned QA Home document and exact api_v1 settings GET.
It passes the original Fetch/XHR call and return value through unchanged, while
privately snapshotting at most one request inside Chrome. Fetch uses a native
Request snapshot; XHR records only its actual application-supplied headers in a
page-local closure (at most 32 entries/8,192 string code units), then snapshots
them into a native Request. The temporary XHR record is discarded after send.
No cookie getter/setter, storage read, arbitrary endpoint/header reconstruction
or request-header export to Node/Go is provided. Opaque authorization/CSRF header
values, if present in the application's own request, remain inside QA Chrome.
This expressly authorized browser-local copy does not authorize cross-context
or browser-to-headless session transfer.

The bounded sequence is:

1. A: one natural Home reload, fresh unique completed settings HTTP 200 and
   settings_match projection. A's result is delivered normally to the app.
2. B: only after A and a unique prepared hook snapshot pass, one fixed action
   sends the captured request through the original native Fetch with
   credentials:omit, no-store, redirect:error and a four-second abort timer.
   B's response/promise never reaches application callbacks. Its response stream
   is drained/discarded in-page with a 16 KiB ceiling; credentials:omit also
   ignores response cookies. Original cookie storage is not cleared or changed
   by the hook. This is a no-cookie request control, not an empty guest context.
3. Dispose the page hook and remove its new-document script, then A again:
   reload Home normally and require another fresh settings_match HTTP 200.

Fetch and XHR selection are supported. B uses Fetch even when A uses XHR; that
transport difference remains explicit rather than claiming two identical calls.
Each phase has a ten-second window, a 4,096-event ceiling and the existing
single-frame, unique GET/cache/redirect/body/read-deadline constraints. Preflight
tests reject foreign origins/methods, ambiguity, over-limit XHR state, replay,
caller scripts/URLs and direct CDP operations. Runtime expressions and installed
source are fixed constants; arbitrary caller evaluation remains unsupported.

Report `aku.x.qa-web-aba.v1` contains separate A/B/A-again phase DTOs, a closed
value-free hook snapshot, Home readiness and hook-removal checks. Go independently
validates all phases using the existing response decoder and recomputes the
outcome. Missing B, failed A/A-again, lost Home or unresolved rejection remains
distinct. A 200/401/200 with an error envelope is only cookie_dependency_candidate:
the generic envelope has not excluded CSRF/challenge or established a fully
guest request, named-account binding or native continuity. Authentication stays
unknown, canAuthorize/sessionTransferred false. No Store policy is changed.

Initial offline run `1cb594483dda400e8662cbb2ac13ee64` passed 82 Node/nine Go
top-level cases. Native run `c08b440dc64c4097a023569b83640829` passed the harness
in 32.81 seconds on the retained QA login, without manual input:

| Phase | HTTP | Body category | Body reads |
| --- | --- | --- | --- |
| A: original XHR | 200 | settings_match | 1 |
| B: browser-local Fetch, credentials:omit | 401 | error_envelope | 1 |
| A again: normal web request | 200 | settings_match | 1 |

Every phase had one matching GET, completed and fresh cache flags. Hook transport
was xhr, candidates one, attempted/completed true, disposed true, prepared false
and rejected false. Home before/after, hook removal, graceful root close and
whole owned-Job cleanup were verified. The lease returned clean and the disposable
control root was removed. No login, cookie inspection, header/body value export,
transfer, main login, Store or user-runtime operation occurred. Sanitized report:
`runtime/qa/auth-session-sidecar/x-qa-source-c08b440dc64c4097a023569b83640829.report.json`.

Final offline run `98dd4e4e99164053a701e3ecbe50ddbf` passed 83 Node/nine Go
cases after adding dedicated RPC denial coverage. Native implementation inputs
were unchanged; no extra QA or login repetition was needed. Next qualify the
401 rejection category with a bounded projection before promoting this result
to authenticated-source evidence. Actual account binding, continuity and any
session transfer remain separate authorization/evidence gates within MVP-1.

### Bounded 401 rejection checkpoint (2026-10-20)

The natural-web 11-file delta was reviewed and published as `2696b5a` (remote
SHA verified). Subsequent authorization adds a value-free rejection projection
to the same retained-QA A/B/A sequence; the hook/request/header boundaries and
number of body reads are unchanged:

```powershell
# Offline regression checks only:
.\scripts\test-x-qa-source.ps1
# Explicit retained-QA authorization; reuse the existing login:
.\scripts\test-x-qa-source.ps1 -RunNativeQA -WebAuthenticationABA -Web401Projection
```

The new fixed `qualify_web_401` RPC accepts only its owner. It projects B's one
eligible response at private CDP dispatch before returning the existing body
category to the capture. The projector reuses strict 16 KiB UTF-8/base64 and
duplicate-key/depth/token validation. Only a root containing exactly one errors
entry with exactly numeric code/message fields can qualify. The code must be
32, and the bounded message must privately match the reviewed canonical form.
The [official legacy X error table](https://developer.x.com/en/support/twitter-api/error-troubleshooting)
classifies code 32 as a request-authentication-data error. This fixture predicate
is deliberately stricter than code-only matching: message drift remains unknown.

Projection `aku.x.qa-rejection.v1` has only schema, shape, codeClass, messageMatch
and classification. The only disclosed numeric-code identity is the fixed enum
code_32; other valid numbers collapse to other. Unknown/mixed error arrays,
extra root/entry fields, nonnumeric codes, message drift, malformed and oversized
bodies cannot produce provider_authentication_error. No message, arbitrary code,
body key, account ID or credential-derived fingerprint is exported or persisted.
The existing WebAuthenticationABA mode/report remains V1; it is not silently
reinterpreted. The opt-in projection emits `aku.x.qa-web-aba.v2`, with an extra
rejection DTO. Independent Go validation rejects unknown/reflected fields,
recomputes the DTO classification and validates the original A/B/A bracket.

Only a fresh valid 200/settings_match A, a 401/error_envelope B with the narrow
provider error, another 200/settings_match A, completed non-rejected hook and
verified hook removal can yield cookie_auth_rejection_observed. This qualifies
the provider-reported meaning of B in this tested control. It does not prove
every internal reason for the provider's response, a wholly guest request,
specific-account binding, native continuity or the sufficiency of auth_token/ct0
alone: B omitted all browser-managed cookies, while opaque application headers
remained inside Chrome. Authentication stays unknown and canAuthorize plus
sessionTransferred stay false. No Store policy or production activation changes.

Initial offline run `652ee99c37c047ce9caf56b17c5acc8c` passed 89 Node/ten Go
top-level cases. Native run `cd2ea57f2b384e2f93e4f1659d6b5d76` passed in
32.82 seconds using the retained login without input. It reproduced 200/401/200,
one matching fresh completed GET and one body read in each phase. B projected:
single_error, code_32, messageMatch true, provider_authentication_error. The paired
outcome was cookie_auth_rejection_observed. Hook transport was xhr, completed,
disposed and not rejected; Home before/after and hook removal were true.
Graceful root close, empty owned Job, clean retained lease and disposable-root
removal were verified. No raw message/body, cookie inspection, header export,
session transfer, main-login, Store or user-runtime operation occurred. Report:
`runtime/qa/auth-session-sidecar/x-qa-source-cd2ea57f2b384e2f93e4f1659d6b5d76.report.json`.

Final offline run `e645b274666943cc91de4c587a3c09cd` passed 90 Node/ten Go
cases after dedicated RPC denial and failed-bracket promotion checks were added.
Native implementation inputs remained unchanged, so no extra QA/login run was
needed. The 401-meaning checkpoint is now resolved for this bounded control;
account binding, continuity and any credential transfer still need their own
evidence/authorization within MVP-1. Do not restart broad source discovery or
infer that all native-authentication gates have passed.

### Same-document identity/login binding checkpoint (2026-10-20)

The opt-in `-BoundIdentityLogin` mode reuses the existing retained-QA lease and
performs one Home reload. It combines the verified instrumentation-to-normal
breakpoint handoff with one fresh natural GET on the pinned `api_v1` settings
route. It does not repeat the accepted A/B/A source qualification, read cookies,
transfer credentials, log in/out, or change Store/main runtime behavior.

```powershell
.\scripts\test-x-qa-source.ps1
# Separately authorized retained-QA observation only:
.\scripts\test-x-qa-source.ps1 -RunNativeQA -BoundIdentityLogin
```

The fixed paused expression privately returns only the validated self ID beside
the existing bootstrap projection. CDP dispatch consumes the ID before resolving
the promise and returns only the value-free projection. The direct entity ID,
optional second session ID, and all present supported entity paths must agree;
numeric/coerced IDs, accessors, conflicting fields and explicit false/invalid
authentication fields cannot establish identity. The private comparator pins
that account for this attempt only; it does not enroll a named account.

An observation binds the retained owner/profile, target/CDP session, root frame,
new document loader and default execution context. The settings request must
start after the identity baseline, complete within five seconds, have known
freshness and consistent required ExtraInfo, and pass the existing bounded body
projection. Context/target/document loss, stale/overflowed observations, cache,
redirects, duplicates and any observed post-baseline cookie write reject binding.
Startup cookie writes are recorded separately and cannot excuse later writes.
Runtime context-loss observation stays enabled until the bounded response drain;
actual debugger/Runtime/Network cleanup must then complete. No fake cleanup
acknowledgment or observation-domain disable/re-enable gap is introduced.

The closed 15-field `aku.x.qa-bound-observation.v1` report contains only fixed
claims and sanitized bootstrap/source DTOs. Independent Go decoding rejects
duplicates, excessive nesting, reflected values and contradictory claims.
Enrollment remains not_verified, continuity unknown, and canAuthorize plus
sessionTransferred false, including for a successful same-document sample.

Final offline acceptance passed 154 Node cases and eleven Go top-level tests.
The initial native run `914b0a63a26943c8b07601bb151d707f` (7.10 seconds) was
rejected by the initial reload-wide cookie-write guard. After baseline ordering
and ExtraInfo regressions passed, run `0184891780c8447e994d6ea7ad172117`
(5.83 seconds) still reported observed_cookie_write, now with
startupCookieWriteObserved false. Both observed a valid self-ID/entity match
and one fresh completed settings HTTP 200; the mutation fence prevented the body
read, so login/binding remain unknown. Passing native test execution means
diagnostic/ownership cleanup succeeded, not that binding qualified.

This establishes a post-identity Set-Cookie observation, not a proven account or
session change: the guard currently inspects header-name presence only. Stop
native retries and qualify that write using a bounded value-free cookie-name/
scope projection before deciding whether it is relevant. Do not silently ignore
unknown writes or weaken Store continuity. Both runs reused the retained login
without input, verified graceful root/whole-Job cleanup and clean lease release,
and removed disposable control roots. No native ID/body/header value was saved.
Reports/logs use the run IDs above under `runtime/qa/auth-session-sidecar`.

### Cookie-write name/scope qualification checkpoint (2026-10-20)

The preceding 15-file binding delta is published as `b906bd1` with exact remote
SHA verification. The separately authorized follow-up adds the opt-in fixed
`qualify_bound_cookie_writes` RPC and the command below. The existing bound V1
report and its post-identity cookie-write fence are unchanged.

```powershell
.\scripts\test-x-qa-source.ps1
.\scripts\test-x-qa-source.ps1 -RunNativeQA -BoundIdentityLogin -CookieWriteScope
```

`aku.x.qa-cookie-write-scope.v1` wraps the original observation and a closed
cookie metadata projection. It projects Set-Cookie at the private page-session
event boundary: recognized names, normalized declared domain, host-only/default
scope, a fixed path category, Secure/HttpOnly/SameSite/Partitioned attributes,
pre/post-identity phase, setter origin category, request role and event channel.
Unknown names/domains/paths collapse to fixed labels; malformed/duplicate scope
attributes and missing correlation remain partial. No value, raw cookie line,
expiry, arbitrary attribute, URL, request ID or unknown native name leaves the
projection. Limits are 16 KiB per header, 4 KiB per line, 32 lines/attributes,
128 X request bindings, 32 retained entries and 64 header events. Getter-backed
input is not invoked. Foreign asset requests cannot exhaust X correlation.

Setter correlation includes already-observed X requests in the owned page CDP
session, including child-frame requests that can write cookies. It does not
attach to more targets, inspect more profiles, or make those requests root
identity/login evidence. Cookie-store APIs, extra response-body reads, session
transfer and Store/main-runtime operations remain unavailable. Metadata records
the attempted header scope only (`acceptance: header_only`); it does not prove
browser acceptance, the final cookie jar, value rotation or server revocation.

Native run `559dbda6d7e94d0988a4daf421361698` (6.97 seconds) projected five
entries, all named __cf_bm with normalized domain x.com, hostOnly false, path /,
Secure/HttpOnly true, SameSite none and Partitioned false, after identity. Three
setters were unbound and one diagnostic remained unresolved. After broader
already-observed X setter correlation and an offline regression, run
`add59a158c09423aaa382a523c87007b` (6.23 seconds) projected five matching entries:
four x.com/other_x and one api.x.com/settings. All setters were correlated, but
the projection still reported partial with unresolved 2. Both runs reused the
retained login without input; graceful root close, empty owned Job, clean lease
release and disposable-root removal passed. Identity was observed and settings
HTTP 200 completed, but the unchanged mutation fence prevented the body read;
login/binding remain unknown. No auth_token/ct0 entry was projected; partial
coverage does not prove their absence or that no other write occurred.

The counter was then corrected offline: ambiguous/over-limit requests without
Set-Cookie no longer count as unresolved cookie writes; an actual header from an
ambiguous setter still produces unknown scope and a partial result. Final
preflight passed 165 Node cases and twelve Go top-level tests, including strict
15/7/12-field nested validation, null/duplicate/reflection denial and preservation
of the original fence. No third native run was made. Do not retroactively upgrade
either partial artifact or claim that the counter correction explains all
unresolved native observations. Revalidate corrected correlation separately.

[Cloudflare's cookie reference](https://developers.cloudflare.com/fundamentals/reference/policies-compliances/cloudflare-cookies/)
identifies __cf_bm as Bot Management state generated independently of the
customer application's user ID. This supports distinguishing its role from the
two candidate account cookies, not treating it as harmless: bot protection can
affect request acceptance. The transfer ceiling remains auth_token/ct0; no
cookie exception or Store-continuity relaxation is implemented. The next
bounded gate is complete write classification, followed by review of any
fixture-only bot-state distinction while unknown/account writes still fence.

## Synthetic Chrome concurrency adapter (2026-10-19)

A later authorization adds `x-concurrency-chrome-adapter.mjs` and its gated
private-RPC driver. They connect the existing harness to a fresh normal Chrome
reader and a separate true-headless Chrome collector. Only invented loopback
authentication is transferred; no retained X profile or main login is inspected.
The Go test owns the HTTP server and a kill-on-close Job containing Node and both
browser trees. The driver cannot launch Chrome before the bootstrap handshake
and a subsequent Go request, after Job assignment.

```powershell
# Offline harness/adapter and Go report-boundary checks only.
.\scripts\test-x-concurrency-fixture.ps1 -NoChrome

# The same preflight, then fresh synthetic Chrome cases; no manual login/clicks.
.\scripts\test-x-concurrency-fixture.ps1
```

The positive case requires exactly one invented login and independently observed
simultaneous server requests. The mutation and target-loss cases wait for the
server's `/read-barrier` before rotating the invented cookie or closing the exact
owned collector target. Observation must fence the run before successful results
can publish. These are controlled fixture operations, not extra endpoints or
mutations approved for the native X experiment.

The adapter must preserve the whole reader window inventory, including the
startup/bootstrap target. It exports only the fixed fixture cookie through the
private connection, with exact attributes and explicit scope. Browser PID/image
and Job membership are checked before the run. Profile roots and helpers remain
project-local; cleanup removes disposable roots only after the owned Job is
verified empty. A failed cleanup retains the scoped root for diagnosis.

This qualifies only the controlled loopback page topology. Startup/worker
capabilities remain synthetic inputs; it does not establish arbitrary X target,
worker or transport coverage. A target-loss case is not a transport-disconnect
test. Reports retain `canAuthorize:false`, `nativeQualified:false` and
`realAuthTested:false`; native transfer and Store admission remain disabled.

From AkuSidecar (Node 24 and an existing Chrome executable):

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\broker.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'
```

The browser command creates minimized fixture windows; it requires permission to
launch processes and listen on loopback. It does not install Chrome or restart
AkuBrowser. The only CLI inputs are the explicit local-fixture flag and the Chrome
executable: existing profiles, URLs, CDP endpoints, and credentials cannot be
supplied. Runtime data goes under `runtime/qa/auth-session-fixture/run-*`, never
under build or system temp. The process may reopen only its own fixture profiles
after their previous child exits. Both Chrome instances use private inherited CDP
pipes and external hostname resolution is disabled.

The runner prints a bounded, secret-free summary and closes both browsers/server.
It validates the resolved generated path before removing its own profiles. No
acceptance archive is written. Cleanup failure is reported separately and exits
nonzero; do not delete a failed run directory until its Chrome processes are gone.
The harness does not force-kill any browser.

## What it measures

- Two different browser PIDs and user-data directories. An unseeded context cannot
  authenticate. Exactly one simulated login precedes overlapping successful reader
  and collector requests, measured as two requests in flight on the fixture server.
- CDP export/bootstrap through `Storage.getCookies`/`Storage.setCookies` into an
  isolated collector context; cookie values never enter the summary or argv.
  HttpOnly, Secure, SameSite, persistent/session expiry, path, and partition keys
  round-trip. Wrong-partition and wrong-path cookies are not sent; HttpOnly cookies
  remain unavailable to page JavaScript. Expired cookies are absent.
- Collector restart, owner restart, and replacement of the broker object can
  reuse the initial simulated login. Object replacement is **not** a full Sidecar
  restart or recovery test. Owner restart uses only this run's synthetic profile.
- Owner-driven token rotation invalidates the derived session; one reseed restores
  it. The same recovery episode cannot be replayed. No reverse cookie synchronization.
- A deliberately hostile refresh policy in the fixture lets a derived refresh
  invalidate the reader. Recovery stops at `NEEDS_USER` when the owner cannot
  authenticate. This demonstrates a feasibility gate, not a bug to retry forever.
- Explicit logout notification fences old results immediately and disposes the
  derived context. Account switching and stale callbacks cannot cross epochs or
  broker instances. There is no claim of automatic in-site logout detection.
- Closing all owner windows records whether the owner stays alive and checks that
  the collector can still use its existing valid session.

The summary ends with three simulated login calls: the initial one, an explicit
relogin after the deliberate refresh conflict, and the account-switch login. The
concurrency, restart, and owner-rotation checks all finish while the count is one.

## Limits and next gate

This is a cookies-only transport/lifecycle feasibility test. Local HTTP loopback
is not production HTTPS/cross-site policy, actual token refresh, device-bound
credentials, source permissions, source extractors, media playback, or native
foreground activation. It does not transfer localStorage, IndexedDB, or
sessionStorage. Session identity checks depend on the fixture's source probe.
The broker state machine is single-source; per-source production routing is not
implemented. Cookie selection for real sources must be an explicit allowlist,
not a dump of all cookies from a signed-in browser.

Real-authentication testing requires new user confirmation. The suggested first
slice is X only: acquire the application auth owner at an idle boundary, export
only approved X authentication state in memory to a new isolated collector,
verify the same account, perform one bounded reader/collector overlap, dispose
the derived state, and verify the original runtime is restored. Do not clear
cookies, log out, change accounts, force refresh/challenges, install a package,
or broaden to other sources as part of that first test. Restart, refresh and
revocation durability would remain separate acceptance gates.

Protocol references: [CDP Storage](https://chromedevtools.github.io/devtools-protocol/tot/Storage/)
and [Network CookieParam](https://chromedevtools.github.io/devtools-protocol/tot/Network/#type-CookieParam).

## Central vault alternative (local fixture only)

The second experiment implements the user's central-session proposal without
changing the first experiment or the application. `central.mjs` holds the latest
validated cookie snapshot in private process memory, with an epoch, version,
account identity and per-client generation. It does not persist a credential
vault to disk. Losing the coordinator process loses this in-memory vault; durable
encrypted storage and full Sidecar restart recovery are not implemented.

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\central.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run-central.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'
```

The fixture logs in once through the reader, validates that initial snapshot,
then makes the collector the designated refresh executor. This requires only two
browser processes; the vault lives outside both. Managed reader and collector
requests can overlap. A refresh request gates new work and drains existing
managed operations before one refresh executes. Concurrent refresh requests for
the same version share that operation. Publishing the next version requires
seeding and verifying every attached client. Partial publication cannot admit
work; stale leases and late results are rejected.

An explicit reader detach clears its fixture cookie jar while preserving the
central snapshot. The reader process can exit; the collector can still refresh.
Reopening and attaching the reader installs the latest snapshot before admitting
work, instead of trusting its old on-disk cookie state.

Managed requests are followed by account probes and cookie observations. A
normal response that unexpectedly rotates authentication moves the coordinator
to `CONFLICT`; the response result and new work are fenced. The PoC does not pick
the last-arriving browser snapshot as truth or reverse-sync it blindly. Resolving
such conflicts requires a source-specific rule that has not been implemented.

This scheduling guarantee covers only requests made through `use()`. Arbitrary
navigation, page scripts, workers, background refresh and every network response
in a real native browser are **not** intercepted by this PoC. A production design
needs reliable source-specific observation and quiescence, not merely a shared
cookie store. The fixture's refresh endpoint is an invented protocol; it does
not establish that any social platform has an equivalent refresh operation.

As with the first experiment, the server, browser profiles and credentials are
synthetic and disposable. Real-auth testing remains separately authorized.

On 2026-10-13 at 09:36 WIB, Chrome 154.0.8037.98 passed all 18 central browser
checks: one login across the entire run, two overlapping reads, three coalesced
refresh requests producing one rotation, a second refresh while the reader was
absent, reader reattachment without login, and safe rejection of an unexpected
third rotation. Both browsers/profiles were cleaned up; an independent Windows
process inspection found no fixture processes or remaining profile directories.
The combined broker/central unit suite passed 36 tests including subtests.

An admitted read that drains during refresh is inspected for unexpected cookie
updates but conservatively not returned as successful in this prototype. Its
result must be retried or checkpointed by a future production integration; this
PoC must not be used to silently drop real captured evidence.

## Switchable strategy harness

`strategies.mjs` exposes a fixture-only `SessionStrategyController` over the two
existing implementations. This is an actual strategy boundary, with separate
adapters around `AuthSessionBroker` and `CentralSession`; callers do not need
source-specific branches to select or switch policy.

| Strategy ID | Implementation | Refresh executor | Session authority |
| --- | --- | --- | --- |
| `reader-owned` | Existing copy/reseed broker | Reader | Signed-in fixture reader |
| `central-vault` | Existing central coordinator | Collector | Private in-memory central snapshot |

The common operations are `start`, `switchTo`, `lease`, `use`, `refresh`, `attach`,
`detach`, `revoke` and `status`. `listStrategies` exposes the supported choices
and capability differences. IDs are an explicit registry, not module paths or
dynamic plugins. No production configuration or Settings option is changed.

```javascript
const sessions = new SessionStrategyController({
  origin: fixture.origin,
  expectedAccountId: 'alpha',
  clients: { reader: fixtureReaderAdapter, collector: fixtureCollectorAdapter },
  timeoutMs: 5000,
});
await sessions.start('reader-owned');
await sessions.switchTo('central-vault');
await sessions.switchTo('reader-owned');
```

A switch request immediately fences old leases/results and gates new work. It
waits for admitted work to finish before verifying the current reader's
account and snapshot. The old strategy is cleaned before the target starts; the
verified fixture snapshot remains private in memory during handoff. In particular,
central cleanup clears browser jars, so handoff restores the verified reader
snapshot before the reader-owned broker starts. Activation is published only
after target startup verifies the same account. Invalid IDs and selecting an
already-ready strategy do not reset the session. A failed drain, cleanup, identity
check or startup cannot silently become a successful switch or fallback.

Switches require the fixture reader to be available as the verified bootstrap
source. A request is allowed during managed work but is not an instant takeover;
it completes at a safe boundary or reports failure. It never opens a reader
window automatically. Detaching a reader and switching strategy are different
operations. Runtime strategy selection is not persisted across harness restarts.
Overlapping control operations are rejected (`BUSY` for a switch request;
`SESSION_NOT_READY` can be returned by other operations); callers must retry
after the existing control operation finishes. Central refresh can
continue while the reader is detached. The reader-owned strategy requires its
reader and supports detaching/reattaching the collector.

The command below runs initial -> alternate -> initial, checks the correct
refresh executor, and switches while a real Chrome fixture request is in flight.
Use either initial strategy:

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\strategies.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run-strategies.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe' --strategy reader-owned
# Alternatively, use --strategy central-vault.
```

Only local simulated authentication is permitted. The CLI cannot accept a real
profile, source URL or CDP connection, and nothing is wired to production auth.

Validation on 2026-10-13 at 09:58 WIB: Chrome 154.0.8037.98 passed 20 checks
for each initial strategy (40 browser checks total). Each run retained exactly
one fixture login, two overlapping reads, and two rotations performed by the
correct strategy authority. Both runs exercised switching during in-flight work
and returning to the initial strategy. The final broker/central/strategy unit
suite passed 50 tests including subtests. All fixture roots exited and profiles
were removed; independent process/directory inspection confirmed zero remaining.

The drain deadline bounds waiting for managed work. Adapter calls retain their
own deadlines (private CDP calls use the fixture transport timeout); this is not
a single hard deadline for the entire handoff. Failed cleanup can be retried,
but there is no automatic fallback to an unverified strategy. The vault is still
volatile and these results do not establish real-source authentication support.

## Observed changes, capture retry and encrypted restart fixture

The next local-only extension adds three boundaries without activating production
authentication. Run it explicitly on Windows with the bundled Node and installed
Chrome; it creates only disposable fixture profiles and a synthetic ticket vault:

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\broker.test.mjs .\scripts\auth-session-fixture\central.test.mjs .\scripts\auth-session-fixture\strategies.test.mjs .\scripts\auth-session-fixture\observer.test.mjs .\scripts\auth-session-fixture\capture-queue.test.mjs .\scripts\auth-session-fixture\vault.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run-resilience.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'
```

### Attached-page session observation

`observeSessionMutations(page, options)` subscribes to private CDP Network events.
It observes `fixture_session` Set-Cookie attempts from ordinary navigation and
page fetches, including requests made outside `session.use()`. Its synchronous
callback fences the strategy controller and central engine even during refresh
or handoff. Switching strategy cannot bypass a detected conflict.

The sole controlled refresh request carries an ephemeral nonce. The observer
correlates that nonce with the exact CDP request ID before exempting its response;
another request is not exempt merely because a refresh is in progress. Signals
and status contain counters/categories only, never raw headers, cookie values,
URLs or nonces. Missing response evidence, ambiguous redirects, disconnects and
blocked controlled cookies stop admission. `flush()` provides a CDP barrier and
a bounded ExtraInfo pairing wait after an awaited request/navigation.

This lower-level observer covers **attached-page Network events only**. Other tabs,
worker/service-worker targets, cookie changes without observed network responses,
and events before subscription are not covered. Detection occurs when CDP
reports evidence; this is not a network interceptor or a global browser lock.
Conservative stopping does not reconcile independently rotated credentials.

### Explicit retry checkpoint

`FixtureCaptureQueue` keeps a read job pending when an admitted capture loses its
lease during refresh or switching. After the awaited control operation, calling
`resume()` retries it using a fresh lease. There is one running attempt per ID,
a default maximum of three attempts, and one retained admitted result. Payloads
must belong to the verified account; rejected/stale results are never published.
Conflicts, authentication failures, invalid responses and exhausted retries stay
explicitly blocked/exhausted rather than looping or silently discarding the job.

This queue accepts synthetic read-only post jobs, not arbitrary writes. It is an
in-memory checkpoint across handoffs, not durable queue persistence across
process restart, and it does not implement production Store idempotency. Repeating
a read may return newer source content; it cannot recover an already vanished
post. Callers explicitly resume only after the control boundary completes.

### Encrypted synthetic-ticket checkpoint

`FixtureVaultStore` explicitly saves/restores a checkpoint using Windows DPAPI
CurrentUser. Its helper receives input over private stdin; no ticket is placed
in command arguments or plaintext files. The ciphertext is atomically replaced
under a validated project-owned `runtime/qa/auth-session-fixture/run-*` directory.
Roots/files that are links are rejected. Format, exact loopback origin, invented
alpha/beta account, fixture cookie shape and a 15-minute lifetime are validated.
This uses the current Windows user's DPAPI protection; it does not protect
against other code running as that same user. It never reads production auth.

`recovery-child.mjs` creates a fresh coordinator process and fresh Chrome profiles,
decrypts the synthetic checkpoint, and performs a fresh server identity probe
through normal strategy initialization before admission. The fixture server
stays alive during this test. Decryption alone cannot admit a ticket the server
has invalidated. Checkpoint saves are explicit, not automatic background vault
updates, and this is not an actual Sidecar restart or crash-durability guarantee.
The helper is bounded to ten seconds. Vault files are removed with test profiles;
there is no acceptance archive or retained credential fixture.

### Validation

2026-10-13 at 10:26 WIB, Chrome 154.0.8037.98: **72 unit tests passed, none skipped**
(including the Windows junction rejection case). The resilience runner passed
**26 checks** (18 parent and 8 child checks), with one invented login, overlapping
reader/headless reads, correct nonce-authorized refresh, a single retry after
both switch and refresh, real DPAPI save/restore in a new process, observed
unmanaged page fetch, ordinary-navigation rotation fencing, and server rejection
of the still-decryptable stale checkpoint. Both child coordinators and all
Chrome roots exited. Independent Windows inspection confirmed zero fixture
Chrome processes and zero remaining profile/vault directories.

## Owned browser target supervision

`observeBrowserSessionMutations` installs the shared response observer on targets
of an explicitly launched disposable fixture browser. It discovers existing and
new pages, dedicated workers, shared workers and service workers. Shared workers
and HTTP/unknown service workers are discovered but explicitly deny coverage;
they are not supported session observers. Recursive CDP auto-attachment pauses
supported new targets until observation is ready. Each supported target has
one authoritative observer even when Chrome exposes multiple attachment sessions.
The controlled refresh exemption belongs to its exact target and request nonce.

An observed synthetic session-cookie mutation fences admission as `CONFLICT`.
Missing observation, unexpected detach or browser disconnect fences admission
as `NEEDS_USER` / `SESSION_OBSERVATION_LOST`. A confirmed fixture-owned target
closure can retire that target without discarding coverage of the remaining
targets. Status exposes target types, lifecycle counts and evidence categories;
it does not expose ticket values, response headers or request nonces.

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\browser-observer.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run-browser-targets.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'
```

This remains a local PoC. Observation starts after subscription and covers network
response evidence; it does not intercept cookie writes, recover earlier events,
or prove real-platform authentication works. It does not change the production
browser lifecycle, automatically update the vault, or persist capture jobs.

Chrome 154 splits dedicated-worker response evidence across its parent and worker
sessions. A bounded correlator retains provenance and pairs only unique observed
owners within a verified attachment family. A nonce-bearing refresh cannot move
across sessions. Ambiguous ownership, collisions, incomplete evidence and capacity
limits stop admission. Pairing may delay a cookie signal until response ownership
is known; callers must await `flush()` at the fixture verification boundary.
This is not proof of immediate fencing for arbitrary live workloads or a universal
request-ID uniqueness guarantee. Request tombstones are bounded to 512 per browser.

Shared-worker rotation was observed at the fixture server without corresponding
cookie response evidence in Chrome. HTTP service-worker initialization stalled at
`Network.enable`; activation and positive mutation detection remain unqualified.
These types therefore deny admission explicitly, including discovery during a
controlled refresh. Internal service workers may be observed, but any HTTP request
from them also denies coverage. This gate does not claim registration succeeded.
Chrome internal `chrome:`, `chrome-extension:` and `data:` resource requests are
handled separately; their opt-in exclusion never exempts HTTP cookie evidence.

For focused reproduction, set the process environment `AKU_FIXTURE_CASE` to one
of `new-tab`, `dedicated-worker`, `shared-worker-coverage-boundary`,
`service-worker-refresh-race`, or `target-lifecycle`; unset it for all scenarios.
Receipts distinguish supported mutation detection from unsupported-coverage
checks. They are printed to stdout, without retained acceptance archives.

### Browser-target validation

2026-10-13 at 11:36 WIB, Chrome 154.0.8037.98: **143 unit tests passed, none
skipped**, and the final full target runner passed **38 checks across five
scenarios**. New pages and the first dedicated-worker request correctly detected
session mutation. Shared workers and HTTP service workers passed explicit
unsupported-coverage checks; service-worker discovery during a controlled refresh
prevented publication of a fresh lease. Those checks do not establish positive
cookie observation or successful activation for those unsupported worker types.

Owned target retirement preserved the remaining coverage, including Chrome UI
type changes and a disappearing target's pending barrier. Browser disconnection
produced explicit coverage loss. Every scenario used one invented login; all
fixture Chrome roots exited and all disposable profiles were removed. The existing
resilience runner also passed 26 Chrome/DPAPI checks at 11:06 WIB, covering the
unchanged default strict page-observer path and encrypted restart recovery.
Production runtime and real authentication remain untouched by this fixture
validation.

## Small worker capability fixture

`run-worker-capabilities.mjs` isolates worker behavior from the larger browser
observer and session-policy controller. It launches fresh owned profiles against
the invented loopback server, filters auto-attachment to the selected worker type,
and accepts authority only for the exact fixture script URL. Unrelated internal
Chrome workers are counted separately and do not become fixture authorities.
It never connects to an existing browser/profile or transfers real credentials.

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\worker-capability-probe.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run-worker-capabilities.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'
```

Set process environment `AKU_FIXTURE_WORKER` to `service_worker` or `shared_worker`
for one worker; default `all` exercises both. A resume-before-Network variant is
permitted only as one diagnostic retry after an actual enablement timeout. Its
observation window is explicitly post-resume and cannot qualify startup coverage.
The successful final run did not require that variant.

Validation on 2026-10-13 at 14:05 WIB, Chrome 154.0.8037.98: **11 new unit tests
and 28 browser checks passed**. The baseline service worker registered and reached
the live `activated` state. In the scoped experiment, Network enablement completed
before debugger resume; one fixture authority observed the first commanded
rotation request/response, ExtraInfo and Set-Cookie. The earlier recursive
observer stall was not reproduced, but its precise cause remains unresolved.
The probe checks live activation because `serviceWorker.ready` may resolve while
the active worker is still activating.

| Evidence | Service worker | Shared worker |
| --- | --- | --- |
| First commanded rotation request/response | HTTP 200 observed | HTTP 200 observed |
| Worker-session ExtraInfo and Set-Cookie | Observed | Not observed |
| Explicit Storage checkpoint changes | Verified | Verified |
| Old derived session rejects after rotation | HTTP 401 | HTTP 401 |
| Same-account reseeding and explicit account-switch readback | Verified | Verified |
| Initial script-fetch / universal startup coverage | Unqualified | Unqualified |
| Existing browser observer admission | Remains denied | Remains denied |

`fixtureCookieCheckpoint` compares private session-cookie fingerprints in memory;
neither cookie values nor their fingerprints appear in receipts. Storage snapshots
are scoped to fresh fixture contexts. Server probes separately verify account
identity: a cookie change alone never proves the expected account. The explicit
alpha-to-beta test copies a synthetic ticket only to prove distinguishable
identity readback; it is not policy permission to admit another account.

The Network probe handles ExtraInfo arriving before requestWillBeSent using a
bounded 64-key boolean-only metadata buffer. Overflow reports unavailable evidence.
Its observations are exact worker-session evidence, not a browser-wide assertion
that a parent session could not receive additional events. The protocol does not
guarantee ExtraInfo for every response ([CDP Network](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Network.pdl)).
Direct snapshots are supported by [CDP Storage](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Storage.pdl).

The capability decision is consequently bounded: service-worker response-cookie
observation is feasible with one scoped connection; shared-worker mutation has
only checkpoint verification through this path. Checkpoints do not establish
instant fencing, and neither result qualifies the initial worker-script fetch.
The existing browser observer gates, strategy controller and production runtime
are unchanged. Fallback policy and Sidecar integration are separate next steps.

The final run exited all owned Chrome roots and removed every disposable profile;
independent process/directory inspection confirms no fixture leftovers. Receipts
are stdout-only. Failed launches without an owner handle retain their root because
child exit status is unknown. Locked files are left untouched; cleanup never kills
other processes. No authentication, acceptance archive, restart, commit or push
against the real runtime was performed for this phase.

## Generic exclusive fallback fixture

`exclusive-fallback.mjs` separates execution admission from authentication
authority. Its injected adapters provide reader inventory, reader opening,
collector pause/start and fresh session verification. The controller contains no
platform URLs, browser APIs or reader-close callback. Concurrent admission
requires verified cookie observation, startup coverage and ownership; missing
coverage selects exclusive execution. A downgrade is sticky for the controller's
lifetime. Restoring capability flags does not silently restore concurrency.

Reader requests in exclusive mode invalidate collector leases immediately,
abort/drain outstanding reads and stop the owned collector before opening the
reader. Old-generation results are rejected; the existing in-memory fixture
queue retains interrupted/unstarted work. Remaining reader windows keep batch
admission closed. After the last confirmed user close, fresh authenticated account
and session-revision checks precede collector restart and queue resume. Unknown
inventory, changed account, lost login or failed control operations require user
action. A timed-out start remains tracked through its late cleanup; `stop()`
reports `cleanupPending` instead of claiming physical shutdown prematurely.

```powershell
.\runtime\dev\headless-worker\node.exe --test .\scripts\auth-session-fixture\exclusive-fallback.test.mjs .\scripts\auth-session-fixture\capture-queue.test.mjs
.\runtime\dev\headless-worker\node.exe .\scripts\auth-session-fixture\run-exclusive-fallback.mjs --run-local-fixture --chrome 'C:\Program Files\Google\Chrome\Application\chrome.exe'
```

Validation on 2026-10-13 at 15:03 WIB, Chrome 154.0.8037.98: **19 unit tests and
30 browser checks across four scenarios passed**. A downgrade during active
batch preserved the exact two reader target/window IDs and browser process,
stopped only the fixture collector and resumed both retained jobs after the last
reader closed. One interrupted job retried once; completed jobs did not publish
twice. Starting exclusive also gave reader requests priority. Synthetic account
change and logout both blocked restart. Successful handoffs retained one invented
login. Target closure is confirmed by bounded inventory readback because the CDP
close acknowledgment may precede disappearance.

These are local execution-policy results. Capability flags are explicit synthetic
test inputs, not new worker qualifications. The bootstrap authentication window
remains in the owned source browser and is excluded from reader inventory; this
does not prove production shared-profile lock handoff. Queue retention is only
in memory, and production close-event wiring, durable jobs and actual platform
authentication remain outside this phase. Test actors explicitly close fixture
readers to simulate user action; the controller itself never closes readers.
All fixture browsers exited and disposable profiles were removed, independently
confirmed with zero remaining fixture Chrome processes/directories. Receipts are
stdout-only; real profiles, production runtime and source data were untouched.

## Limited Sidecar integration (synthetic build only)

The Go integration now uses Sidecar's actual Coordinator, capture Manager,
Engine, SQLite command queue and existing close-and-resume HTTP handler. It is
compiled only with `auth_session_fixture`, additionally requires `config.Dev`
and an explicit injected synthetic-session probe. Ordinary binaries have no
binder, config flag or HTTP activation endpoint. Existing production config and
startup remain unchanged; new status fields are omitted when inactive.

The integration adapts the approved exclusive policy to the existing Go owner
manager; it does not enable concurrent derived authentication. A reader intent
closes batch admission immediately. A physically active collector step and its
pending reasoning finish before session leases are released. This phase drains
the current operation rather than aborting/replaying an already claimed command.
Unstarted commands/source runs remain in the actual persistent queue. A minimal
SQLite pause flag fences claims in the same claim transaction, without saving
cookie values, account identities or session fingerprints.

The Manager still owns replacement/readiness and verified cleanup. Reader windows
stay open until an explicit user-close adapter or natural fixture process exit.
Fresh expected-account verification gates headless handoff, and queued,
never-claimed headless commands are rebound to the new Engine epoch/generation.
Completed/cancelled commands remain terminal; late old-owner evidence and duplicate
publication are rejected. Pause markers and queued work survive closing/reopening
SQLite and creating a new Engine. Media recapture adoption and non-headless queued
routes are explicitly outside this bounded fixture; they are rejected.

Run from the AkuSidecar root with cached Go dependencies:

```powershell
.\scripts\test-auth-session-sidecar.ps1
.\scripts\test-auth-session-sidecar.ps1 -VerifyDefault
```

The helper uses unique project-local temporary directories, restores process
environment settings and removes its disposable outputs. Dependency download is
disabled. `-VerifyDefault` also runs existing capture/reader regressions and builds
an ordinary Sidecar executable without starting it. Locked outputs are retained;
cleanup does not terminate processes. Results are printed, with no acceptance
archive. This does not install, restart or enable anything in the user's runtime.

Validation on 2026-10-13: **19 fixture leaf cases passed** (8 Engine cases, 8 Store
cases and 3 HTTP lifecycle cases). They cover actual collector drain and automatic
next-source resume, same queued IDs/continuation, stale/duplicate rejection,
durable restart, cancellation, failed owner cleanup, unknown reader inventory,
account mismatch and logout. One new loopback server exercises an invented
HttpOnly login cookie and verifies no second login on logout rejection. Default
capture/reader regression selections passed across five packages; an ordinary
`cmd/akusidecar` build succeeded and Go file selection confirmed fixture binders
are excluded.

Browser/process adapters in these Go tests are synthetic. The ownership checks
prove Manager ordering and conservative reader preservation, not Windows/Chrome
profile-lock release or real foreground activation. Reader opening is tested via
the Engine borrow path; the existing HTTP close-and-resume endpoint is exercised.
Full OS reader-broker opening and real-source authentication remain untested and
require separately scoped validation. Earlier standalone Chrome results are not
new evidence that these Go adapters control an actual Chrome instance.

## Owned Chrome adapter for Go Sidecar (opt-in, Windows)

The next fixture phase uses actual Chrome processes behind the tagged Go
process adapter and a narrow Node bridge. `-VerifyChrome` is explicit opt-in;
ordinary Sidecar startup and the default helper do not launch this adapter.
The helper uses a new project-local QA root and profile. Existing profiles,
external sites and existing debugging endpoints are rejected. Chrome uses the
private debugging pipe; the bridge exposes only named local fixture operations,
not arbitrary URLs, cookies or CDP requests.

The bridge is assigned to a Windows Job Object before it may launch Chrome.
Every launched Chrome PID must belong to that job. Handoff requires the job's
complete process list to contain only the known bridge process; Chrome root exit
alone is insufficient. A subsequent successful launch and authenticated browser
probe on the same fixture profile provide the profile-reuse evidence. Cleanup
is restricted to the owned job, and disposable output is removed only after
verified job cleanup. Unknown release or inventory keeps the operation blocked.

The local HTTP server invents accounts and persistent cookies. Its fixture-only
authority probe checks revocation/account state before launch, when the old
browser may already have exited. The additional browser probe checks the actual
new owner after launch; both account and revision must match before queue
admission opens. A valid server-side session alone cannot resume a browser whose
fixture cookie is missing. Neither probe performs login or exports cookie values.

The smoke exercises one login, collector-to-reader-to-collector profile reuse,
multiple actual windows including Chrome's bootstrap window, ownership while a
window remains, and natural final-window exit. The original bootstrap target is
navigated to the loopback fixture and counted alongside every remaining reader
window. Page handles are selected from confirmed live fixture targets after a
window closes. An explicitly requested final-window close can wait up to ten
seconds for the exact Chrome root's exit after pipe closure; pipe closure alone
never proves process exit or ownership release.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyChrome
.\scripts\test-auth-session-sidecar.ps1 -VerifyChrome -VerifyDefault
```

Validation on 2026-10-13 with Chrome **154.0.8037.98**, Node **24.16.0** and Go
**1.25.13 windows/amd64**: the combined helper exited **0**. The **7 real-Chrome
leaf cases** (6 test functions, including logout/account-change subcases) passed:
profile-reuse smoke; active collector drain, reader handoff, multiple windows,
Engine/SQLite replacement and resumed capture; injected cleanup rejection;
unknown inventory; logout; account change; and browser-cookie loss with otherwise
valid server authority. The continuation uses authenticated Chrome `/collect`,
retains its run IDs and current epoch/generation, and rejects stale/duplicate
publication. Engine/SQLite replacement retains the existing capture Manager,
bridge and reader process; it is not a restart of the production Sidecar binary.

The existing **19 synthetic leaf cases**, selected default capture/reader
regressions in five packages, and an ordinary Sidecar build also passed. The
separate standalone fallback regression passed **30 browser checks** across four
scenarios after the shared launcher change. Cleanup markers are issued only
after all registered fixture jobs in the test run are verified released. The
helper's output parent is empty, and independent process inspection found zero
remaining Chrome/Node processes belonging to these fixtures. Receipts remain
stdout-only; no acceptance archive was created.

The ordinary 19 synthetic cases remain useful regression coverage. Real Chrome
evidence is separate from their simulated process/capture effects. Windows
foreground activation, full OS reader-broker opening, concurrent session reuse,
media recapture and real-site authentication remain outside this fixture phase.
Capture sends actual browser HTTP requests to the local fixture; observation
content is invented test data, not production DOM extraction. Cleanup failure
is deliberately injected against a live owned Chrome process. No real profiles,
authentication, installation or user runtime were changed; this delta is not
committed or pushed by this implementation request.

Implementation references: [Windows Job Objects](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects),
[job process ID list](https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-jobobject_basic_process_id_list),
and [Chrome debugging isolation](https://developer.chrome.com/blog/remote-debugging-port).

## Isolated Windows reader-broker fixture (validated locally, 2026-10-13)

`-VerifyBroker` stages a tagged helper, an owned native fixture UI and the test
server in a unique project-local directory. The fixture uses a fresh random
named pipe, not the production pipe or port. No native messaging host or
extension is registered. The production request URL, direct UI parent, helper
image/UI Job/age, server image/directory, single-use ticket and native HWND/PID/
property checks remain in the actual broker implementation. Ordinary builds do
not contain the isolated pipe or fixture UI entry points.

The native fixture UI launches the helper directly for an explicit synthetic
intent. The source-shaped HTTPS URL is validated request metadata only; the
prepared Chrome page and authentication stay on the invented loopback server.
The capture-owned Chrome Job and tracked reader/bootstrap windows are retained
from the earlier adapter phase. The broker binds an actual native window and
the client activation result is checked against real OS foreground, visibility
and minimized state. Server-side activation readback and the helper's terminal
acknowledgment are separate protocol evidence.

Windows can reject the fixture UI's initial foreground request. In that case,
the opt-in test waits up to 60 seconds for a physical click on **AkuBrowser
Synthetic Reader Broker Fixture**. If the first reader request receives an exact
supported activation denial, the test verifies retained reader ownership and
batch pause, then arms **Retry explicit reader**. A physical button click within
60 seconds launches a new direct-child helper with a fresh request and ticket.
The completed request is never replayed, and the existing 30-second preparation
and 5-second activation lifetimes are not extended. A missing click or rejected
new activation fails the test. There is no injected input, focus privilege
workaround or repeated focus restoration. This foreground exercise is explicitly
authorized and does not run with the ordinary helper.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyBroker -BrokerTestRegex '^TestBrokerFixtureOSReaderHandoff$' -VerifyDefault
```

The command above exited **0** with a coordinated physical button click. The
first actual activation was denied with `ui_foreground_changed`, `Applied=false`
and `Readback=false`. The accepted path was
`warm_retained_owner_fresh_physical_intent`: actual client activation returned
`Applied=true`, `Readback=true`, `focus=exact_reader`, visible and not minimized.
Independent OS checks matched the exact HWND and PID. The retained reader was
not replaced for the retry. A single synthetic login remained valid; the reader
and bootstrap windows held batch pause until final close. Natural root exit and
whole-Job release preceded resumed Chrome capture (three launches and three
release proofs).

The actual helper/pipe path rejected indirect parent, wrong helper image, changed
UI foreground before a new request, wrong HWND/PID/property/value and expired
binding. Successful native binding verification rejected a second consumption.
A separate protocol regression rejected a consumed ticket on a fresh exchange.
The tagged broker contracts, ordinary broker/appshell regressions, 19 synthetic
cases, five-package default selection and ordinary Sidecar build passed. The
existing seven real-Chrome leaf cases also passed again after the aggregate
Chrome/UI cleanup-marker change. Independent process inspection found zero
remaining processes belonging to the fixture. Session
loss/account change remain covered by the earlier Chrome suite; this OS case
does not test a focus or session change after ticket claim.

Fixture output is removed only after both Chrome and UI/helper jobs are proven
released and the private broker server has stopped. Cold first-request activation
remains unresolved. This result proves a fresh physical fixture intent through
the actual helper, authenticated pipe and Windows activation path; it does not
prove a click from the installed AkuBrowser extension, native-host registration,
production process restart or real-site authentication. No installation, real
authentication, user runtime operation, commit or push was performed for this
phase.

Windows foreground policy: [SetForegroundWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow).

## Cold first-request continuation (validated locally, 2026-10-13)

The authorized cold fixture gate uses one physical **Open explicit reader**
button click before reader preparation, with no retry fallback. Its isolated
native-reader preset follows the production normal initial window and initial
target reuse, instead of preparing a second minimized window. Only this preset
uses `--new-window` with GUI startup visible; the retained fixture preset stays
unchanged. The original Chrome startup/bootstrap target remains in inventory
when it is also the first reader. An independent ownership window is added only
after the first acknowledgment and exact native readback.

The cold UI is shown with `SW_SHOWNOACTIVATE` and placed once on the active monitor
using owned-HWND-only `TOPMOST|SHOWWINDOW|NOACTIVATE` presentation. Native window,
button and bounds readback must pass. The temporary topmost state is removed and
verified before the physical click launches the helper. No other window is
modified. Click **Open explicit reader (klik sekarang)** as soon as it becomes
enabled; do not wait for a chat announcement. The button is disabled during
preparation and the same instruction is displayed in the fixture window. Earlier
click-gate timeouts did not execute a reader request; the owner confirmed the
visible window was being held pending a delayed chat instruction.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyBroker -BrokerTestRegex '^TestBrokerFixtureOSColdFirstRequest$' -VerifyDefault
```

This command exited **0**. The first handler invocation accepted
`cold_actual_success`, with no warm retry: `Applied=false`, `Readback=true`,
`focus=exact_reader`, visible and not minimized. Independent Windows readback
matched the exact HWND/PID. Chrome had already brought the normal initial reader
to foreground; the broker confirmed it without an additional focus write. The
fixture UI's startup readback was not focused and its explicit startup foreground
call was not made. One synthetic login remained valid. Closing the initial
reader/bootstrap left the independent window holding ownership and batch pause;
final close led to natural root/whole-Job release before resumed capture (three
launches and three release proofs).

Five actual wrong/expired binding cases passed, as did changed UI foreground
before a new request and native binding replay rejection. Parent/image rejection
coverage belongs to the preceding warm OS case; it was not repeated before this
first cold request. Tagged broker contracts, 19 synthetic cases, ordinary broker/
appshell regressions, default five-package selection and ordinary build passed.
The existing seven Chrome leaf cases and 30 standalone exclusive-fallback checks
passed on the current shared launcher. Owned UI/helper/Chrome jobs were verified
released before output removal.

The previously validated warm path retains its separate scope. Broker identity,
exact binding, UI foreground and 30-second preparation/5-second activation guards
are unchanged. No focus grants, input injection, deadline extension or automatic
native action replay was introduced. This resolves the cold fixture gate; it does
not prove the installed extension/native-host journey, full production restart,
post-claim focus/session changes, production DOM extraction or real authentication.
This validation preceded commit/push. On 2026-10-13 the owner subsequently
authorized commit/push of the nine-file cold delta; the preceding OS fixture
commit `f141dc6` was already pushed with explicit owner authorization.

## Full fixture process restart (validated locally, 2026-10-13)

The next authorized gate replaces the entire OS fixture worker process after
graceful shutdown or abrupt termination of the verified owned worker. Each
dedicated fixture-tagged executable instance composes real Sidecar Engine,
Store, Manager, Coordinator and Chrome bridge objects from scratch. This is
distinct from the earlier same-process Engine/SQLite replacement and from
production `cmd/akusidecar` startup. Synthetic external session authority may
remain in the test controller; the actual browser profile and SQLite persist
only inside the fresh owned test run.

The existing Chrome Job has KILL_ON_JOB_CLOSE. Reader closure on full owner exit
is the observed baseline and the subsequently approved child-Sidecar contract.
Reader preservation applies to normal handoff while Sidecar lives, not to full
parent process exit. There is no independent keeper or old-browser reconnect.
The controller must not keep the inner Chrome Job alive. Its independent outer
containment bounds cleanup and verifies the old tree and old reader windows are
released before a private one-use grant allows the same owned profile to reopen.
Default existing-profile rejection remains in force for other launch paths.

Acceptance includes persisted pause/run identities, fresh actual worker PID and
Engine epoch, authority plus real browser account/revision before admission,
one invented login, pending-command epoch rejection isolated from duplicate
completion, and conservative recovery on missing proof. This lifecycle gate
uses the retained minimized preset and does not require foreground activation
or a physical click. No authentication, runtime, installation, registration,
commit or push is part of this phase.

The dedicated command is `cmd/auth-session-process-fixture`, gated by Windows
and `auth_session_fixture`. Actual parent PID/creation time, staged image, directory
identities and controller lineage restrict reuse to the same fresh run. Recovery
consumes a persistent exclusive-create one-use grant. Both Go and Node enforce
the project QA boundary before lineage access; the authority client rejects
redirects. No ordinary launcher setting enables existing-profile reuse.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyProcessRestart -VerifyDefault
```

Final validation exited **0**. The lifecycle test passed in 5.03 seconds:
graceful worker PID 36420 to 31056; abrupt worker PID 37408 to 22236. Both observed
old root exit, zero old containment-job members and destroyed reader/bootstrap
HWNDs before the successor started. `readerPreserved=false` and
`innerJobDuplicated=false`. All successor components were new; the same owned
profile/SQLite retained run IDs, account `alpha` and revision, one login and two
source collections. A new Engine epoch rejected an otherwise valid payload
changed only to the old epoch; duplicate current-command completion was tested
separately. Manager generation legitimately restarted at one.

Seven independent bad grant fields and persistent grant replay were denied.
Missing external synthetic authority retained SQLite pause and collection count
at one despite authenticated browser evidence. Restoring only invented authority
and explicitly rechecking it let the real binder/browser hooks verify, rebind
never-claimed work and release pause. No direct pause mutation or second login
was used. Nineteen synthetic cases, tagged contracts, default five-package
regressions and ordinary Sidecar build passed. Current seven Chrome leaf cases
and 30 standalone fallback checks passed. Aggregate Job cleanup removed output;
independent process inspection found zero fixture processes and both QA parents
have zero run directories.

This completes the fixture lifecycle test. The owner subsequently selected child
ownership beneath Sidecar: full parent exit must close its owned windows/processes,
and restart recovers queued work instead of preserving the old reader. The
independent reader-owner/reconnect proposal was rejected. The dedicated composition
does not prove production main, installed extension/native-host startup, production DOM
extraction, real authentication or post-ticket-claim races. No user runtime,
authentication, installation, registration, commit or push changed.

### Approved child-Sidecar lifecycle contract (2026-10-13)

Manager ownership remains in the worker; Node bridge and Chrome are its owned
descendants. The test controller holds only outer cleanup containment and
synthetic external authority, not an inner Chrome Job handle. The selected
contract does not require an additional owner executable.

On graceful shutdown or abrupt worker death, observe old reader/bootstrap HWNDs
destroyed and the old tree empty before authorizing any successor. A controller
cleanup after a failed assertion does not count as proof of ordinary parent-death
cleanup. Unknown/nonempty release blocks profile reopening. The bounded restart
test uses the same profile created fresh inside that run, not a user's profile.

The replacement gets a fresh Engine epoch and must verify synthetic authority and
actual browser account/revision before resume. Only never-claimed queued commands
may receive new stamps. Old results, duplicates, cancellation and earlier claims
remain fenced. Missing authority or browser/session proof retains pause; no
automatic second login or reader/foreground intent replay repairs it.

The `-VerifyProcessRestart -VerifyDefault` evidence at `f17ca37` established the
full-worker baseline. This continuation reruns the relevant Chrome and process
cases after the tagged bridge changes below. The separate
same-process Engine/SQLite test may retain reader windows because their owning
Sidecar/Manager/bridge have not exited.

Source review confirms that the abrupt restart case terminates only the worker
root and checks old Job/window release before replacement; deferred controller
cleanup cannot satisfy that assertion.

### Intermediate bridge crash (validated locally, 2026-10-13)

The added `TestChromeFixtureBridgeCrashReleasesTree` keeps the Go parent and its
Job lifetime intact, verifies the actual Node executable/identity/membership and
terminates only that bridge. On the baseline, Chrome and its old windows were
already gone, but owner Done did not settle and Manager remained Ready. The first
tagged correction notified failure too early: Job membership was empty before
the pinned Chrome root process handle had signalled.

The adapter now waits for actual Node exit, cleans/verifies only its owned Job
and waits for Chrome root handles obtained while those processes belonged to the
Job before publishing failure. Handle membership is checked directly; registration
after EOF is rejected under the owner-map mutex. The regression requires the
pinned root signalled, exact tree empty and recorded reader/bootstrap HWNDs gone
before failure notification; new Manager leases are denied. One login and two
launches remain unchanged, with no capture, relaunch or second login. Both full
process-restart cases also pin the old Chrome root before stop and wait for its
signal before giving a recovery grant. No test-side wait weakens Done ordering.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyChrome -VerifyProcessRestart
```

The final command exited **0**: eight Chrome cases in 17.027 seconds and graceful/
abrupt worker replacement in 4.99 seconds. Nineteen synthetic cases and tagged
broker contracts passed. Account/revision, profile/SQLite/IDs, one login and two
collections, new epoch/local generation one, stale/duplicate rejection, retained
pause on missing authority, seven bad grant fields and replay rejection passed.
Default/ordinary and standalone-30-check evidence is reused on unchanged inputs.
Independent inspection found zero matching fixture processes and zero QA run
directories. Only three tagged Go files and three documents changed. Validation
preceded the owner's subsequent commit/push authorization on 2026-10-13; real
authentication and user runtime were untouched.

At this checkpoint, startup before Node Job assignment remained unverified.
Chrome cannot launch before assignment/private RPC, but the initialized-owner
result did not prove abrupt Go startup death and pre-assignment Node lifetime.
The authorized continuation below tests that separate fixture boundary; production
startup, installed host lifecycle and post-ticket-claim races remain unproven.

### Pre-assignment startup parent death (authorized fixture gate, 2026-10-13)

After commit/push of the six-file child-Sidecar checkpoint `168cc5d`, the owner
authorized actual worker death before Node enters the inner Chrome Job. Use a
private deterministic pause after the Node ready handshake, prove actual Node
identity and missing inner-Job membership, then kill only the verified Go parent.
Wait for pinned Go/Node handles and outer containment to release before fallback
cleanup; absence of Chrome launches/windows/login/capture is part of acceptance.
Closing stdin while Go stays alive or racing an arbitrary delay is insufficient.

The existing fixture startup authentication and project-local new run/profile
guard remain in force. Unknown release blocks any reopen grant. This phase tests
the tagged dedicated command, not production startup, and authorizes neither real
authentication/runtime operations nor commit/push of the new delta. Record the
actual result, command and cleanup below after execution.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyProcessRestart -ProcessTestRegex '^TestProcessFixtureStartupBeforeNodeJob$'
```

The narrow startup command passed on its first measured attempt (0.12 seconds).
Actual Go PID 34884 and Node PID 37312 were verified by canonical executable,
creation/direct-parent identity and process handles. The Node ready barrier proved
known false inner-Job membership and zero inner members; independent outer queries
found exactly Go+Node. No Chrome/windows/profile/SQLite/login/capture existed.

The controller terminated only the Go root. Both pinned handles signalled and
outer containment became empty before fallback cleanup while its outer Job stayed
held. No profile-reopen grant was issued. Node naturally exited in this pre-Chrome
state; no lifetime implementation fix was necessary. This is the tagged fixture
barrier result, not production startup or every instruction-level crash point.
The final regression and cleanup readback follows below.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyChrome -VerifyProcessRestart -VerifyDefault
```

Final validation exited **0**: startup 0.12 seconds (Go 40320, Node 19572), eight
Chrome cases 17.164 seconds and full OS restart 5.10 seconds. The canonical
`-VerifyProcessRestart` selection now includes both startup and full restart;
`-ProcessTestRegex` can select the startup case narrowly as shown above and accepts
only the same bounded filter grammar as the other fixture selectors.

The final startup observation repeated false inner membership/zero members,
exact outer Go+Node, both pinned handles signalled and outer tree empty while the
controller Job remained held, before fallback. No Chrome/window/profile/SQLite,
login/capture or reopen grant occurred. Nineteen synthetic cases, tagged broker
contracts, five-package default regressions and ordinary build passed; existing
browser/session/epoch/duplicate/grant guards passed. The standalone JavaScript
inputs did not change, so their preceding 30-check evidence is reused.

Independent audit found zero matching fixture processes and zero QA run
directories. Validation of four fixture/helper files plus three documents preceded
the owner's subsequent commit/push authorization on 2026-10-13. No lifetime
behavior fix, JavaScript change, real authentication, user
runtime, foreground action, installation or registration was needed. Coverage is
the actual Node-ready/before-assignment fixture boundary; earlier pre-ready
intervals and production startup remain unverified.

### Before-ready death and stalled handshake (authorized fixture gate, 2026-10-13)

After the seven-file checkpoint `112f2df`, the owner authorized actual Go worker
death before Node sends ready and a stalled handshake exercising the original
eight-second timeout. Use the real fixture Node bootstrap and a deterministic
private barrier bound to fresh authenticated fixture startup, not a production
setting or arbitrary kill timing.

The death case must observe verified Node image/creation/direct-parent identity,
pinned handles, known unassigned inner Job and exact outer Go+Node before killing
only Go. Both root handles and outer containment must release before fallback.
The stalled case keeps Go alive until an explicit timeout, retaining its original
deadline, then verifies exact child/tree cleanup. EOF alone cannot establish that
release succeeded. Neither case may start Chrome/windows/profile/SQLite/login/
capture or issue a recovery grant. Only a demonstrated failure permits a narrow
fixture fix; no real authentication, user runtime, foreground action, installation
or registration is part of this gate. Commit/push was excluded from implementation
scope and subsequently authorized after validation below.

The first narrow run passed actual pre-ready parent death (0.12 seconds) and
exposed a timeout cleanup race: at 8021 ms the error had returned but the pinned
Node handle was still WAIT_TIMEOUT while Go remained alive. EOF/closed arrived
before the sole `cmd.Wait()` finished; later parent teardown did not count as
successful cleanup at return. The tagged adapter now publishes a separate actual
Node-wait completion channel and waits on it in startup cleanup. The original
eight-second handshake timer remains unchanged; unknown wait is cleanup failure.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyProcessRestart -ProcessTestRegex '^TestProcessFixture(StartupBeforeReady|HandshakeTimeout)$'
```

Corrected narrow cases passed: pre-ready death 0.16 seconds (Go 3252, Node 26004),
timeout 8.07 seconds (Go 1440, Node 6508). At timeout return (8042 ms), ready was
false, failure was the handshake timeout, the pinned Node handle was signalled,
Go was still alive and outer containment held only Go before its controlled error
exit. Neither case created Chrome/windows/profile/SQLite/login/capture/reopen
authority. The final wider validation and cleanup readback follows below.

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyChrome -VerifyProcessRestart -VerifyDefault
```

Final validation exited **0**: eight Chrome cases in 17.522 seconds and process
suite in 13.477 seconds (full restart 5.16, Node-ready startup 0.11, pre-ready
death 0.12 and timeout 8.06). Pre-ready Go 51904 / Node 8172 released both roots
and containment before fallback. Timeout Go 12908 / Node 24600 reported 8026 ms
with ready false and Node already signalled at return while Go stayed alive;
independent containment was exactly Go-only before its controlled error exit.
No Chrome/window/profile/SQLite/login/capture or recovery grant occurred in the
new cases. Normal bootstrap immediately permits ready; withholding uses only
the private fresh-grant frame/QA-lineage/direct-parent checks, with no CLI/env
activation and no nonce in logs, receipts or arguments.

Nineteen synthetic cases, tagged broker contracts, five-package default regressions
and ordinary Sidecar build passed, including existing session/epoch/duplicate/
grant checks. Shared launcher and standalone JavaScript inputs were unchanged;
their preceding 30-check evidence is reused. Independent inspection found zero
matching fixture processes and zero run directories. Validation of three tagged Go
files, one fixture driver and three documents preceded the owner's subsequent
commit/push authorization on 2026-10-13. No real authentication, user runtime,
foreground, installation or registration changed.

This measures real driver bootstrap before ready and the original timeout, not
arbitrary earlier OS-spawn/module-loading deaths, production main startup or the
installed extension/native-host journey. Actual Node-wait completion fixes the
observed EOF/Wait race without changing the eight-second deadline.

### Main dev pilot / ordinary launcher gate (DEV-1, authorized 2026-10-13)

The next milestone is a prepared main-dev pilot: DEV-1 ordinary launcher lifetime,
DEV-2 explicit development integration of priority/pause/verified resume, and
DEV-3 integrated synthetic acceptance. The one-to-three-working-day estimate is
conditional planning; passing these fixtures does not activate user runtime or
prove real-source authentication.

DEV-1 uses actual `appshell.Launch` with a fresh project QA run/profile and a
no-UI synthetic executable which creates its own synthetic child. Pause at the
real Start/Job-assignment boundary, verify actual identities/pinned handles and
memberships, kill only the Go fixture parent, and measure root/child release
before outer cleanup. Source inspection found the headed path starts before
assignment and treats failed private-CDP descendant cleanup as unverified;
the prior Node-ready cases cannot establish this launcher boundary.

Do not duplicate inner Job ownership into the controller or count fallback
termination as a passing parent-death assertion. Correct only a demonstrated
lifetime hole, preserving arguments/private pipe inheritance/real waits. A child
that is suspended yet unassigned can still outlive its parent and is not enough.
Relevant ordinary regressions, Chrome/process fixtures and build follow a shared
launcher change. The new launcher cases use no Chrome UI, login or existing
profile. Broader Chrome regression retains its fresh synthetic profile/login
scope; no foreground request, user runtime, installation, registration, commit
or push is authorized by this gate.

The first baseline failed before fallback: Go 7352 exited, while synthetic root
27548 and child 31276 each returned `WAIT_TIMEOUT` (258) and remained in the
outer controller Job. The private per-window Job was empty. This measures the
ordinary launcher startup gap with synthetic processes; it does not establish
a leak from an existing user Chrome instance. The correction must also verify
per-tree cleanup while Go remains alive.

### DEV-1 result (2026-10-14)

The ordinary Windows `Launch` and `StartOwnedCommand` now establish process
lifetime containment before spawn, create the root suspended, assign/verify its
per-tree Job, and resume only the pinned, owner-verified startup thread. The
lifetime handle is noninherited and retained until Go exits; it is never closed
by per-tree cleanup. Caller attributes/private CDP handles and real `Cmd.Wait`
are preserved. Startup failure cleanup has bounded terminal-wait verification.

The canonical `-VerifyDefault -VerifyChrome -VerifyProcessRestart` run exited
zero: Chrome 16.354 seconds (7 top-level / 8 leaf cases); process suite 21.265
seconds (7 top-level / 9 leaf cases). Four new launcher leaf cases prove only-Go
death before assignment, rejected startup terminal cleanup while Go stays alive,
resumed actual launcher/owned-worker descendant membership, and isolated tree
closure followed by only-Go death. Acceptance is measured before fallback.
Nineteen synthetic cases, broker contracts, default regressions and build passed.

Relevant appshell/headless/capture checks and Linux appshell compilation passed.
The ordinary minimized loopback CDP smoke passed against existing checksum-pinned
CfT 152.0.7977.54 with a fresh profile (0.47 seconds), including exact target
closure, root exit, empty Job and pipe closure after drain. One earlier Node-only
Chrome startup failed with `fixture_chrome_closed`; its cause is unidentified,
and the final full suite passed. Do not describe that flake as repaired.

Independent inspection found zero scoped fixture peers and zero QA run/CDP
profile directories. Ignored project-local transcripts remain at
`runtime/qa/auth-session-sidecar/launcher-dev1-{evidence,final-validation,source-cdp-validation}.txt`.
Disposable binaries were removed; real authentication and user runtime were
untouched. No commit/push occurred. DEV-2 development binding and DEV-3 integrated synthetic
pilot acceptance remain; installed-main/clean-machine parity and real-account
compatibility are not established by this gate.

The owner subsequently authorized DEV-1 commit/push and continuation on
2026-10-14. Commit `7740deb` was pushed and verified at `origin/main`. DEV-2
development opt-in/session-verifier integration proceeds with synthetic tests;
no real authentication or running user instance is authorized, and DEV-2 code
will remain uncommitted pending its next reviewable checkpoint.

### DEV-2 explicit development integration (2026-10-14)

Use the canonical helper from the project root:

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopment
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault -VerifyChrome -VerifyProcessRestart
```

The helper builds three actual `cmd/akusidecar` variants in a fresh project QA
run. Only Windows `auth_session_dev auth_session_fixture` plus
`--dev --dev-session-reuse=synthetic-fixture` dispatches to the isolated concrete
provider, before ordinary config/Store/provider/browser startup. Manual opt-in
without its private scoped controller grant fails. Default builds, missing dev,
unsupported provider, unavailable adapter and missing grant are tested rejections
before ordinary state exists. No setting or HTTP endpoint enables this path.

The new development binding checks source/profile/account/revision, Engine
epoch, actual owner generation/PID/driver and physical profile identity; it does
not use the legacy fixture binder or Bridge readiness as login proof. Lifecycle
primitives are shared with fixture wrappers. Reader priority drains capture and
related accepted-capture reasoning; SQLite pause/restamp/restart and paused
cancellation retain work correctly. Foreign/unknown evidence stays closed, and
mismatch does not silently recover. Media/passive-media/Browser routes and old
frozen plans outside declared headless sources are rejected before resume.

The full command exited zero: new dev suite 13.220 seconds, existing Chrome
16.632 seconds, process 21.439 seconds, synthetic/default/config/main regressions
and three builds. Final development-only frozen-plan refinement was revalidated
with `-VerifyDevelopment`, exit zero, 12.293 seconds (actual-main graceful/abrupt
restart 4.67 seconds). Shared/default inputs did not change afterward.
Twelve invalid evidence cases, startup ordering/negative gates, active drain,
durable restart, paused cancellation and unsupported frozen plans passed.

Exact commands/results and initial corrected assertion failures are indexed in
`runtime/qa/auth-session-sidecar/dev2-validation-index.txt`. Independent inspection
found zero matching fixture peers and zero disposable run directories. All 27
DEV-2 files remain uncommitted; DEV-1 `7740deb` is pushed. Real authentication,
user runtime, install/register and foreground operations were untouched.
Ordinary UI/native-broker startup remains DEV-3: the synthetic main dispatch
bypasses ordinary startup and proves only accepted-capture reasoning recovery.
It does not establish real-source compatibility or activate main dev for users.

The owner authorized and verified commit/push of the 27 DEV-2 files as `0498548`
on 2026-10-14, then authorized DEV-3 UI/native-broker testing with fresh profiles
and synthetic login. DEV-3 will measure the actual UI/helper/broker/reader path
on the new dev binding. Give a clear button-ready/click cue for physical input;
do not count old Engine-borrow-only or fallback evidence as that acceptance.
Real accounts, user runtime, installation and registration remain excluded.

DEV-3 preparation is separate from physical acceptance:

```powershell
.\scripts\test-auth-session-sidecar.ps1 -PrepareDevelopmentBroker -VerifyDefault
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopmentBroker -DevelopmentBrokerTaskRoot '<fresh project QA run directory>'
```

The task root must be a fresh direct `run-*` child of
`runtime/qa/auth-session-sidecar`. The second command requires a physical Open
click in the owned Win32 development UI. The coordinator reads only safe fields
from `operator-cue.json.ready.json`, gives an explicit **click now** instruction,
and privately writes the independent operator cue. A click already received is
accepted immediately; the cue only starts the operator deadline. It grants no
native authority and does not reset native intent expiry. Do not print nonce,
bridge token, capture key or full private control receipts.

The first physical run failed on the staged main filename. The unchanged native
client's allowlist is preserved by staging `akusidecar.exe` beside its helper.
The corrected run passed cold physical opening, exact reader foreground, actual
API/audit and capture/reasoning drain with one login, then exposed a fixture
readback error: use `/api/collection/runtime` for `collectionRuntime`, not
`/api/health`. These partial results do not establish full DEV-3 acceptance.
The ordinary web frontend and installed extension/native-host registration are
not exercised by this Win32 development composition.

The corrected runtime endpoint passed strict actual-server contract checks.
The subsequent full physical run timed out waiting for an Open click after
120 seconds; no positive receipt was recorded in that run. Full resume/lifetime
acceptance remains pending. Preserve the earlier opening proof separately and
obtain the operator's observation before repeating a physical trial. The scoped
post-run QA census was empty.

The operator subsequently confirmed the window was visible but the button had
not been clicked, then gave readiness for another fresh-profile trial. That trial
passed the complete selected DEV-3 native composition in **6.01 seconds**,
helper **exit 0**: physical first opening with exact reader foreground, actual
API/durable audit, active capture/reasoning drain, bootstrap reuse, multiple
window ownership, final-close verified headless API readback, two captures with
one synthetic login, and negative foreground/duplicate/stale/cancelled admission.
Terminating only actual main then released the pinned UI/helper/Chrome roots and
outer Job tree before fallback; reopened SQLite preserved pause and claim fence.
The held direct UI child helper had not consumed stdin/native intent.

Evidence:
`runtime/qa/auth-session-sidecar/dev3-physical-operator-retry-validation.txt`.
Independent cleanup census: zero scoped QA peers and zero disposable run dirs.
Earlier failed/partial transcripts remain distinct. At that checkpoint, DEV-3 changes were uncommitted;
the ordinary web frontend, installed extension/native-host registration and
real-source authentication remain outside the measured synthetic composition.

On 2026-10-14 the owner authorized review and commit/push of the 23 DEV-3 files.
Scoped review found no concrete blocker; the unchanged implementation reuses the
passing physical retry and preparation evidence above. JavaScript syntax and
patch whitespace checks passed. Publication does not activate the feature or
touch original authentication/user runtime/install/registration. The next
proposed acceptance path is the ordinary web UI/bridge in an isolated synthetic
QA composition; installed native-host parity remains a distinct coverage gate.

The owner subsequently authorized that web UI/bridge QA design and testing after
DEV-3 commit `1812d98`. The new instance must use production web assets and real
correlation, fresh profiles, synthetic loopback login and private transport, with
Sidecar-owned lifetime. Before loading a native-capable extension, verify it
cannot invoke any existing registered host or production pipe. Global native-host
registration, installation, original authentication and user runtime operations
remain excluded. Keep substituted transport segments and untested parity explicit;
no additional commit/push is authorized by this testing continuation.

Selected frontend phase: actual web assets, server-injected split UI relay and
trusted-click content script, with real server/Store and synthetic authority.
The isolated QA manifest removes `nativeMessaging`; its staged worker explicitly
denies the native transport call. Verify both before launching pinned CfT with a
new owned UI profile and exact loopback origin. A synthetic Timeline item is
seeded through Store and rendered by the real frontend. A strict development-only
availability/router guard supports the reader-priority link; that exception stays
closed for ordinary/unknown/pending/browser/unsupported inputs, preserving the
existing ordinary Browser path. The human clicks the real
link; observe relay POST/cancel/audit and unsupported UI result without fabricating
input. Private CDP is used for observation, and cleanup remains scoped to the
owned QA process tree. Native host discovery/helper launch/foreground/handoff and
app-mode parity remain unmeasured in this phase; DEV-3 proof is separate.

The actual server may register a minimal dual-tag-only native denial after its
normal bridge/source/URL/epoch validation, before reader borrow, with nil state
in ordinary builds and no activation endpoint. Verify rejection audit and zero
helper/reader transition. The staged worker's bounded 1.5-second denial allows
the concurrent relay POST to be observed; it is explicit substituted transport
timing, not a native-ticket or native-handoff claim.

Frontend preparation distinguishes a rendered/enabled real link from an
in-viewport link. The initial headless readback confirmed render/relay/content
readiness but the link was below the viewport; retain that diagnostic failure.
The physical operator may scroll the actual page and click once. ARMED must
report viewport state honestly, without CDP scrolling/clicking or DOM rearrangement.

Canonical frontend commands (separate invocations):

```powershell
.\scripts\test-auth-session-sidecar.ps1 -PrepareDevelopmentFrontend
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopmentFrontend -DevelopmentFrontendTaskRoot '<fresh direct project QA run directory>'
```

Final preparation passed at
`runtime/qa/auth-session-sidecar/frontend-preparation-final.txt`: 29 Node cases,
three main builds/four early-rejection CLI variants, staging/admission/actual
server-relay/cancel/seed contracts, ordinary split reader/broker regressions and
actual headless frontend readiness (10.23s, one synthetic login). Typed passive
headless skips do not establish collector/source-auth parity. No native helper
was staged or sampled, no broker ran or native action was made; graceful exit
left the owned tree empty before fallback. The reviewed app-only screenshot is
`runtime/qa/auth-session-sidecar/frontend-preparation-viewport.png`.

Physical verification is still pending. In the distinct window **AkuBrowser QA -
Native activation DISABLED**, scroll to **QA frontend: native activation is
disabled** and click **Open source evidence** once after the coordinator's cue.
The expected outcome is a correlated unsupported rejection/cancel, with no reader
activation. Do not click Update now or repeat the link after denial. READY lives
at `frontend-operator-cue.json.ready.json`; only safe title/PID/HWND/rect/viewport
fields may be printed. The coordinator privately writes `{Nonce, MainPID}` to
`frontend-operator-cue.json`; it only starts the 120-second operator deadline.
An early real click proceeds immediately. Current source/doc changes are not
committed, and no visible UI has launched at this preparation checkpoint.

Independent post-preparation census: zero scoped QA processes and zero disposable
run directories. The two retained pre-Chrome staging outputs were removed after
exact path/non-reparse and zero-process verification; stable transcripts/PNG stay.

The owner subsequently approved one temporary QA-native-host leaf under HKCU
Google Chrome NativeMessagingHosts, with a unique `com.akubrowser.qa_reader_<32hex>`
name. Exact staged manifest/helper/main hashes/private pipe must be reviewed before
application; production keys and registry ancestors remain untouched. Read-only
parent checks passed in both views. Coordinator owns apply/receipt/cleanup, and
only deletes the owned leaf after trees exit and key contents match. No other
registration, installation, policy, original-auth or user-runtime work is allowed.
Full frontend native handoff/pause/drain/resume remains open; the denial/headless
fixture must not be substituted for it. See the acceptance audit and QA-native
plan under `docs/`. No new commit/push is authorized.

Native frontend preparation checkpoint (2026-10-14):
`scripts/test-auth-session-sidecar.ps1 -PrepareDevelopmentFrontendNative` passed;
transcript: `runtime/qa/auth-session-sidecar/frontend-native-preparation-ready.txt`.
The native branch preserves configured original extension source, uses an unchanged
private-transport helper and retains its prepared root on success/failure until
Coordinator registry cleanup. Registrar `scripts/qa-reader-host.ps1` defaults to
preview and targets only the approved unique QA leaf. Its application and physical
frontend/native verification have not run; operator readiness is pending. The
physical target is **Open source evidence** under **QA native frontend: synthetic reader**
in **AkuBrowser QA - Synthetic native reader**. Follow the exact frozen-root commands
and ownership conditions in `docs/auth-session-frontend-native-qa-plan.md`.

Latest trial/cleanup (2026-10-14): monitor-safe no-activation UI presentation
preparation passed. The actual physical frontend click and Chrome native helper
provenance passed; the trial failed the combined durable-pause/collector/driver
predicate before capture drain. Foreground reader and composed resume are not
proven. See `frontend-native-physical-monitor-trial.txt` and the acceptance audit.
The exact approved QA leaf was removed after tree emptiness; independent cleanup
confirmed absence in four registry views and zero scoped peers, then removed all
three attempted roots. Historical frozen-root commands above cannot be replayed.
Stable logs and minimal synthetic SQLite/checkpoint remain. No original auth or
user runtime was touched and no new commit/push occurred. This task now has no
budget by user override; the old 921,237-token measurement has partial coverage,
and native reset/new-phase measurement is unavailable.

Pause-order correction trial (2026-10-14):
`frontend-native-pause-order-contract.txt` and preparation/regression transcripts
record the corrected admission → capture/reasoning drain → SQLite commit before
original session-lease release. The physical transcript
`frontend-native-pause-order-physical-trial.txt` records pause/read error/PID/
driver/lease/work per phase, then accepted real frontend/helper/reader foreground
and terminal API/UI with one synthetic login. Full acceptance still fails at the
remaining-window combined predicate after bootstrap close; do not claim verified
resume or frontend parent-death acceptance. No automatic additional trial ran.
The approved QA leaf and exact disposable root were removed after owned-tree/
four-view/receipt checks. Stable logs, redacted window checkpoint and synthetic
failed SQLite remain. A future physical trial needs fresh preparation/operator
readiness; no original authentication/user runtime or new commit/push is involved.

Multiwindow evidence-only step (2026-10-14): the native controller now consumes
`dev_frontend_native_windows_before_close` and
`dev_frontend_native_windows_after_close` after accepted reader foreground. Logs
include inventory known/error/count/bootstrap flags, Coordinator versus SQLite
pause/read error, current/Manager PID, driver/generation/leases/work, expected
reader PID, bootstrap index and close error. Unknown inventory is -1/no flags,
not zero. The after-close receipt is retained even when close itself fails.
The unchanged window predicates report named failure reasons. Focused contracts
passed in `frontend-native-window-evidence-contract.txt`, and both Sidecar builds
passed. No physical trial or host application ran; next physical execution needs
fresh profiles/login/preparation and operator readiness within synthetic QA scope.

The diagnostic physical trial isolated a fixture bug: closing one target/tab
did not close its grouped bootstrap window, while pause/PID/driver remained valid.
The new `chrome-window-lifecycle.mjs` retains and closes every known target for
the selected window, checks target membership/window disappearance, and requires
natural root exit on final close. It is hash-pinned in native preparation. Six
Node contracts plus the actual Chrome restart/multiwindow regression passed.

The corrected physical frontend/native trial passed in
`frontend-native-window-close-physical-trial.txt`: actual foreground/API/UI,
bootstrap 2→1 non-bootstrap with pause/owner intact, final-close verified resume
with one login/capture count two, API denials and existing post-resume UI/headless
parent-death SQLite fence. That lifetime stage does not keep the reader open or
the helper blocked; live-reader parent death remains a separate test. Temporary
host absence/four-view checks, zero scoped peers and exact disposable-root deletion
passed; stable logs/redacted evidence retained. No original auth/runtime or new
commit/push occurred. Future trials always require new profiles/synthetic login.

## Live-reader parent-death scenario (pending physical proof)

Native frontend preparation/verification can both select
`-DevelopmentFrontendNativeScenario reader_open_parent_death`; the default is
`normal`, and a prepared mismatch is rejected before UI launch. The scoped main
stops after accepted real frontend/native reader activation. It requires the
original second run (zero-based ordinal 1) and its actual prepared `queued`
headless command, with null claim metadata, and compares private-payload hashes
and all row metadata across blocked claims/reopen. A completed request can have
zero leases while reader windows still retain profile ownership. The controller
must independently pin live process creation/image/Job/HWND identity before
terminating only the fixture main, then prove whole-tree/window release before
fallback and retained original SQLite pause/command state.

Contracts passed after three physical attempts exposed incorrect fixture
assumptions. The abrupt live-reader death stage has not yet passed; no production
claim follows. All new QA roots/registration are cleaned, so the next approved
physical trial requires fresh preparation, synthetic profiles/login, the same
approved host namespace and operator readiness. See the acceptance document for
logs and exact scope. No additional physical retry is automatic.

## Separate lifetime and physical foreground tests (passed)

```powershell
# Automatic actual-main/UI/reader death and original SQLite queue fence.
# No gesture, foreground callback, native host registration or native helper.
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopmentReaderLifetime
```

The automatic UI starts hidden through private CDP; read-only exact PID/Job/class/
title lookup supplies HWND evidence. After opening the real source reader through
the scoped Engine borrow, its request lease is released while its bootstrap window
retains ownership. The controller kills only actual main, requires UI/reader
handles and HWNDs gone plus empty Job before fallback, then verifies original
run/command/payload hash/null claim fields and durable SQLite pause. This passed
in 3.29s; it claims no physical foreground result. Sandbox scope probes produce
EPERM, so Windows process-fixture execution needs the approved scoped external
execution path; no validator is weakened to work around this.

Physical preparation/verification uses
`-DevelopmentFrontendNativeScenario foreground_only` on both existing native
commands. It remains a separately authorized one-click/operator-cue test with a
fresh root/profile/login and the single approved QA host. It ends after actual
app/relay/content/worker/native/private broker foreground, UI/API/audit and drain
proof, followed by owned graceful cleanup; it passed in 32.42s. `normal` and the
historical combined reader-death scenario retain distinct scope. All split-trial
roots/QA registration were cleaned. See the acceptance doc for evidence paths.

## Persistent pause: ordinary/fixture/development build compatibility

```powershell
.\scripts\test-capture-pause-compatibility.ps1
```

This automatic Store-only test compiles ordinary, fixture, and development test
binaries. Five separate processes reopen the same synthetic SQLite database:
development pause -> ordinary blocked claim -> fixture inspection -> fixture
resume -> ordinary claim. It verifies preserved queued payload/claim fields,
all runs/routes of the paused session, unrelated session admission, unknown marker
and query-error rejection, resume rollback/duplicate handling, and cancellation.
Successful runs clean their exact project QA database/binaries; failed artifacts
remain under the reported project QA root. No Chrome, native registration, login,
or user Sidecar runtime is involved.

The claim guard is common to all builds, but pause/resume activation remains
tagged and default-off. Rollback targets must include this guard: an older binary
that ignores pause markers remains unsafe for the same paused database. This test
does not prove real-account verification or full main startup/browser ownership.

The compatibility command passed all five stages and both common guard cases in
each build. `test-auth-session-sidecar.ps1 -VerifyDefault` also passed the existing
synthetic contracts/default capture-reader regressions and ordinary Sidecar build.
All owned test roots were cleaned. Logs: project-local
`runtime/qa/auth-session-sidecar/capture-pause-compatibility.txt` and
`capture-pause-default-regressions.txt` in the same directory. Engine's synthetic
loopback login test requires the scoped execution path outside the socket sandbox.

## Synthetic account/session verifier contract

The development binder uses version-1 request-correlated evidence, explicit
authentication states, a five-second hard freshness bound and expected account
identities per source. Unknown/logged-out/challenged evidence, cached/foreign
responses and mismatched browser ownership leave admission closed. Expected
identity declarations are copied at bind time; caller mutations cannot change
the bound identity. No new configuration or ordinary-build activation is added.

```powershell
# Includes verifier/reader/pause/status contracts and actual tagged main restart
# with fresh synthetic Chrome profiles/login. No physical click or registration.
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault
```

This remains a synthetic-only proof. The prelaunch authority callback belongs to
the fixture control server; it is not real browser authentication. The actual
browser callback checks the current scoped owner, and Engine verification precedes
Store resume. Contract evidence and callback error details stay private. Real
account/session compatibility requires a separate provider and explicit approval.

The combined command passed verifier/reader/pause/status tests, canceled-probe
denial, default-off CLI guards, synthetic contracts and default regressions/build.
Actual tagged-main graceful and abrupt restart both passed using fresh synthetic
profiles, one login/two captures per case, retained SQLite IDs/pause when authority
was lost, and fresh verified owner epoch before resume. All new roots were cleaned.
The drain regression waits for reasoning idle before database teardown. Log:
`runtime/qa/auth-session-sidecar/session-verifier-contract-validation.txt`.

## Account checkpoint and cold-open Store fence

```powershell
.\scripts\test-account-checkpoint-compatibility.ps1
```

This Store-only harness builds ordinary, fixture and development test binaries
and runs seven separate processes against one synthetic SQLite database:
fixture enrollment -> ordinary closed -> dev authorization -> ordinary cold closed
-> fixture cold closed -> fixture reauthorization -> ordinary final closed.
It also runs checkpoint/guard unit contracts in each applicable build. Success
cleans the exact owned QA database/binaries; failure retains the reported QA root.
No browser, login, native registration, physical click or Sidecar runtime runs.

Enrollment stores expected identities only. Authorization uses a simulated
synthetic owner/session receipt and grants a permit only to that Store handle;
persisted checkpoint state cannot substitute for a fresh permit after cold-open.
The required marker stays durable, old permits are invalidated by a new fence/
generation, and existing session pause and unsupported media/collector boundaries
remain closed. This is metadata/claim compatibility, not Engine/Chrome startup
integration or real-authentication proof. Normal databases without the protected
metadata preserve existing behavior. Ordinary rollback binaries must incorporate
the new guard to recognize these stable keys.

Validated on 2026-10-14: all three unit builds and seven database stages passed;
the five-stage pause compatibility helper and `test-auth-session-sidecar.ps1
-VerifyDefault` also passed. The reauthorization stage simulates draining its
previous claim with a terminal failure; it does not perform a browser capture.
New-owner authorization preserves old-stamped queued payloads, and their claim
remains denied. Stable logs are `account-checkpoint-compatibility.txt`,
`account-checkpoint-pause-regressions.txt`, and
`account-checkpoint-default-regressions.txt` under
`runtime/qa/auth-session-sidecar`. Owned disposable roots were cleaned.
The complete Store suite also passed in ordinary, fixture and development builds.

## Development owner startup checkpoint and Store authorization

Store checkpoint commit `8abe4e0` is published. The authorized next integration
prepares the scoped development Store fence before Node/Chrome startup, then
derives a Store receipt from correlated version 1 authority/browser observations.
Fresh enrollment uses explicit synthetic expectations; reopening the database
requires the same stored account/scope/profile identities and performs no login.
Initialization remains closed; post-initialization verification precedes
authorization and recovery. Reader handoff drains capture/reasoning and fences
before releasing leases. Headless resume verifies a new fence/owner before
restamping never-claimed commands. Changed fences, replaced owners, expired or
canceled probes and failed initialization/resume must stay closed.

The canonical automatic acceptance command is:

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault
```

This command uses newly scoped synthetic Chrome profiles and loopback login;
no physical click or native-host registry registration is part of this gate.
It also checks default-off dispatch and ordinary regressions/build. Passing
startup/session tests does not establish real source authentication or replace
the separate physical foreground/native-reader acceptance test.

Acceptance passed on 2026-10-14. Store compatibility passed across three builds
and seven processes. The command above passed five prepared-profile Node cases,
the correlated verifier/checkpoint/HTTP/default-off contracts, tagged-main graceful
and abrupt restart, selected default regressions and ordinary build. Each restart
case used one synthetic login/two captures and retained fixture SQLite/profile
identity while replacing the process/epoch, fencing before owner launch and
authorizing only after proof. Lost authority and stale/duplicate evidence stayed
closed. Fence/admission races kept queued payloads unchanged. The former cleanup
race is guarded by joining coordinator polling before Store close, with three
targeted repetitions passing. Private prepared-profile scope accepts only an
empty owned directory and rejects reuse/replacement before Chrome starts.

Logs are `startup-checkpoint-development-validation.txt`,
`startup-checkpoint-store-compatibility.txt`,
`startup-checkpoint-profile-scope.txt`, and
`startup-checkpoint-teardown-regression.txt` under
`runtime/qa/auth-session-sidecar`. Owned disposable roots were cleaned. This new
integration is approved for publication after review; source-provider authentication is still synthetic.

### Actual development startup failure cases (2026-10-15)

`-VerifyDevelopment` now also runs
`TestDevelopmentSessionMainStartupFailures`: parent death during the first
authority probe, stalled authority, and changed account. It needs no operator
click or native host registration. Each case uses a fresh profile and a synthetic
loopback login; the real tagged main is contained in the existing fixture Job.
The server stalls the probe without adding a runtime fault flag. The verifier's
two-second context deadline cancels the request before the HTTP client's
five-second timeout.

The test checks pending durable metadata before the fault, zero capture, owned
process handles signalled and Job empty, then two Store cold-opens of the same
SQLite database. Claim stays denied and command payload/claim fields plus the
expected account checkpoint remain unchanged. This is Store cold-open failure
acceptance, not successful full Sidecar recovery from this state or another
upgrade/downgrade matrix.

All three cases and the full `-VerifyDevelopment -VerifyDefault` invocation
passed. Log: `runtime/qa/auth-session-sidecar/startup-failure-validation.txt`.
Owned disposable roots were cleaned; original authentication and user runtime
were not accessed.

### Session policy and historical guard compatibility (2026-10-15)

Run `scripts/test-account-session-policy.ps1` from the Sidecar root. This helper
uses only Store test binaries and one synthetic SQLite database: no browser,
login, native host registration or Sidecar process is launched. It compiles current
default/dev/fixture builds plus default/fixture builds from local immutable commit
`9fc79f38921752628e13e23428683199833631cd`. Only the two compatibility probe test
files are added to that historical source copy; its guard implementation is unchanged.
No remote Git operation or dependency download is needed.

Six mutation stages (enroll, migrate disabled, enable/verify, reject changed
session, disable, re-enable/verify) are interleaved with twenty cold-open probes.
Both historical binaries refuse claims when the new policy record exists; the
historical tagged binary also refuses fence issuance. All queue payload/claim
fields remain unchanged by downgrade probes. New guard unit tests additionally
cover malformed/future/foreign policy, mismatched confirmation, stale policy
revision, transactional migration failure, actual second-handle invalidation,
successful authorized claim and opt-out preserving an active claim.

The policy key stays under `synthetic_account_checkpoint`, and is retained on
opt-out. A build predating that prefix's guard is not a supported rollback target.
Mutation APIs remain tagged and synthetic; no production activation UI, actual
human confirmation, session-reset flow or real provider adapter is supplied.
Successful runs remove their owned database, copied source and binaries; failed
runs print their exact retained QA root. Validation log:
`runtime/qa/auth-session-sidecar/account-session-policy-validation.txt`.

### Direct policy controller acceptance (2026-10-15)

Run `scripts/test-development-policy-controller.ps1`. The helper clears Chrome
and full-process opt-ins, redirects temp/Go output to a new owned QA root, and
uses simulated owners, synthetic request-bound probes and SQLite only. Controller
confirmation is an explicit test action, not a physical click or human-consent
receipt. No browser, login, registry or user Sidecar runtime is launched.

The controller persists a provisional enrollment hold before creating its owner.
Preview/cancel/expiry/replay never creates an account checkpoint. Affirmative
confirmation reobserves the identity and atomically creates disabled enrollment;
another enable confirmation and fresh Engine startup proof are required before
claim. Migration is explicit and default-off. Cold resume retains opted-in policy
but requires new owner proof without enrollment/login. Opt-out preserves active
work until drain, then stops its owner. A canceled drain leaves policy disabled
and the active owner alive until cleanup can complete.

Acceptance includes account/session/owner changes, stale observations/checkpoint,
canceled context, foreign and consumed confirmation IDs, post-confirm proof failure,
active capture completion, disabled successor rejection, verified cold resume,
request versus runtime lifetime, Store cold-open and transaction rollback.
No main/controller routing, production settings or actual confirmation UI is
activated. Real Chrome/OS lifecycle acceptance of this controller is still pending.

The helper cleans its owned roots on success and prints the retained root on
failure. Stable log: `runtime/qa/auth-session-sidecar/policy-controller-validation.txt`.

### Actual main policy provider and Chrome (2026-10-15)

`scripts/test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault` now
includes `TestDevelopmentPolicyMainChromeFixture`, which launches the actual
dual-tag main using `--dev --dev-session-reuse=synthetic-policy-fixture`. It uses
the existing verified parent/root/profile grant, an outer containment Job, a fresh
profile and one loopback synthetic login. Ordinary/default-off CLI rejection is
also checked. No host registration or physical click is used.

The test stops at the provisional fence before Node/Chrome, proves the outer Job
contains only Go, seeds a queued command and verifies claim is denied. Private
frames cancel one enrollment, confirm a second, and reject a changed authority
account while the policy remains disabled. Explicit enable then starts a verified
capture. The physical `/collect` already occurred when ingestion is held: count is
one and the command remains claimed, not completed. Opt-out must become durable
before that hold is released; capture/ingestion/reasoning drains without a second
capture, and the other unstarted source run remains retained.

The test pins Chrome and Node process identities/images/handles. After opt-out,
both roots must be signalled and the outer Job must contain only Go. After the
stop handshake the Go root must signal and the Job must be empty. Profile/root
physical identities and the controller lineage file remain unchanged throughout.
The receipt's startup-authorization flag is historical; the persisted policy flag
and claim checks determine current admission.

This initial scenario uses fresh non-native grants. Actual controller cold
restart/headless recovery is a separate acceptance case below. Real account
observation, human confirmation UI and foreground/native reader acceptance
remain outside this test. Existing reader
reopen grants still require their native-window release evidence. Stable log:
`runtime/qa/auth-session-sidecar/policy-main-chrome-validation.txt`.

### Actual main policy cold restart (2026-10-18)

`TestDevelopmentPolicyMainChromeFixtureColdRestart` extends the same dual-tag
provider with a private headless recovery grant. It is selected by the existing
`scripts/test-auth-session-sidecar.ps1 -VerifyDevelopment` helper. Each invocation
starts in a fresh owned QA root; process replacements within that invocation keep
the same physical SQLite database and Chrome profile. No registry, user profile,
real authentication or interactive input is involved.

The grant binds prior Go/Node/Chrome identities, database/profile file identities,
checkpoint/policy generation, last authorized epoch and the queued session/run/command. It
is consumed once before owner startup. The successor fences admission before
Node/Chrome, reuses persisted opt-in, and requires fresh authority/browser proof
through the controller's `Resume` method. No enrollment or login is repeated.
Legacy native/window reopen grants retain their separate release requirements.

Acceptance checks eligible queued work before owner startup and before proof,
preservation on unavailable authority, old-process cleanup, and successful drain
after valid revalidation. Private decision frames remain simulated confirmation;
this is not real-provider or human-consent acceptance.

Acceptance passed: one synthetic login, one physical capture, the same SQLite
file/profile identities, fresh successor proof, failed-authority admission held,
grant replay denied and the preserved command completed after resume. Only the
successful unclaimed-command epoch/generation rebind is permitted. The scenario
combines graceful enabled-idle shutdown with abrupt cleanup of a denied recovery
owner; it does not simulate abrupt death during an authorized active capture.

The final source does not inspect Chrome's cookie database. The canonical
`-VerifyDevelopment -VerifyDefault` helper passed, including existing native
restart/default regressions and the ordinary build. Final log:
`runtime/qa/auth-session-sidecar/policy-controller-restart-final-01955b2e025247f2a1f3f2bd635c84e5.txt`.

### X offline verifier prototype (2026-10-18)

Run `scripts/test-x-session-verifier.ps1` from the repository root. This separate
offline suite needs no Chrome, login, native host, registry or account access.
The helper uses project-local temporary/build/module caches, disables downloads,
restores process environment variables and removes its owned temporary root on
success. A populated project-local Go module cache is required for the ordinary
main dependency inspection; a missing dependency fails without downloading it.

The package requires both `auth_session_dev` and `auth_session_fixture`. It is
absent from ordinary main dependencies and has no controller/Store authorization
wiring. The X route, schema, login markers and lineage generations are invented
fixtures, not a qualified native X provider. Outputs never authorize capture.

Final acceptance passed 15 top-level tests and 41 subtests, plus both-tag exclusion
and ordinary-main isolation. Identity matching, login state and synthetic lineage
are separate outcomes; missing lineage is unsupported. Negative vectors cover
public/content-author identity, malformed/duplicate bodies, stale/future proofs,
scope mismatch, one-use replay, incomplete coverage and same-account relogin.
The completion-time expiry case clears previously matched identity flags.
Log: `runtime/qa/auth-session-sidecar/x-offline-verifier-validation.txt`.

Native schema and session-continuity qualification remain a later evidence gate.
Passing these invented fixtures does not authorize real-account observation or
user-runtime activation.

### Three-proof offline contract (2026-10-18)

The same offline helper also exercises distinct current-self identity,
authentication and continuity claims. These are three claims, not a promise of
three independent source authorities. Identity/authentication retain shared
self-response provenance; enrollment is an expected account, not login proof.
Missing identity does not discard a valid explicit authentication state.

Continuity uses a separate synthetic lifecycle witness whose coverage spans the
accepted baseline through verification. Equal endpoint snapshots cannot repair
an observation gap or rule out change-and-restore. Logout/login, cache/error,
restart/detach, worker-coverage gaps, replay and completion-time expiry are
negative fixture cases. Cookie hints and hashes never supply lineage.

All fixture outcomes remain non-authorizing and require both build tags. No
browser, real account, runtime, controller, Store or ordinary-main activation is
part of this test. The JSON contract and lifecycle generation remain invented
fixtures; native-source qualification is still an independent evidence gate.

Call `Verifier.VerifyThreeProof(ThreeProofInput)` for the new fixture contract.
The lifecycle interval starts at the accepted initial self observation; it does
not import a persisted Store checkpoint. Authentication `Initial`/`Recheck`
retain each phase's observation; aggregate `State` requires agreement. Authority
counts deduplicate declared fixture labels and do not prove source independence.

The canonical helper passed 37 top-level tests and 65 subtests, including 22 new
top-level tests and 24 new subtests. Both-tag exclusion and ordinary-main isolation
passed, all four protected runtime/configuration hashes matched, and the owned
temporary root was cleaned. Log:
`runtime/qa/auth-session-sidecar/x-three-proof-validation.txt`.

### Bounded offline ingress and projection (2026-10-18)

Run `scripts/test-x-evidence-ingress.ps1` from the repository root. The separate
Node-only suite feeds synthetic CDP-shaped byte streams into
`x-evidence-ingress.mjs`; it does not launch Chrome, use a profile, connect to a
network, retrieve a response body, change Store policy or operate user runtime.
The helper uses a unique project-local temporary root and restores its process
environment before cleaning the owned root after success.

The module projects allowed transport metadata before correlation/output queues.
Its bounded raw framing/parsing boundary is temporary memory, not a promise of
deterministic erasure. Fixed diagnostics exclude headers, credentials, full URLs,
queries, request bodies, content and raw errors. Fixture aliases preserve local
relationships but are not anonymous or native-qualified identities. All outcomes
remain non-authorizing, including a completed synthetic observation.

The local window must reject pre-window request initiation; it is not a
server-authenticated challenge. Loss, ambiguity and overflow remain unavailable.
Existing Chrome transport/observer and the three-proof verifier are unchanged
and are not connected to this module. Native collection and confirmed-baseline
recovery remain separate evidence and policy gates.

API: `createFixtureXEvidenceIngress({ scope, window, clock, limits })`. `push`
accepts UTF-8 byte chunks containing NUL-delimited CDP-shaped messages. `drain`
returns copied/frozen metadata; `finish`, `expire`, `changeOwner`, `loss` and
`close` manage observation lifetime. Scope is one trusted fixture session/target
and the invented loopback `POST /x-fixture-operation`, not all browser workers.
The injected clock is synthetic context without native Chrome clock calibration.

Default frame capacity is 256 KiB, capped at 1 MiB. Input chunk/frame-count,
parser depth/token, alias, correlation, orphan and output limits are separate.
The offline age/window limits do not relax the Go verifier's proof lifetime.
Expected ExtraInfo can arrive after loading finishes; unresolved evidence fails
at finish/expiry. Absent cache flags are `unreported`; only matching statuses or
the modeled 200/304 pair avoid a status-conflict verdict. No status means login.

Final helper acceptance passed 16 tests with no failures. Nine protected
runtime/configuration, Store/controller and verifier files matched their baseline
hashes, and the owned temporary root was cleaned. No Chrome, network or native X
qualification was performed. Log:
`runtime/qa/auth-session-sidecar/x-ingress-validation.txt`.

### Bounded Chrome body to verifier fixture (2026-10-18)

Run `scripts/test-x-evidence-chrome.ps1` from the repository root on Windows.
The dedicated helper runs ingress/body unit regressions, the existing offline
verifier and build-tag isolation checks, then actual headless Chrome through the
existing Windows Job harness. Each case creates a new owned profile, performs
one loopback-only synthetic login and makes initial/recheck browser requests.
It verifies Node/Chrome image identity, inherited Job membership and empty owned
Jobs after cleanup. No existing browser, login, registry or runtime is used.

The separate `x-evidence-chrome-driver.mjs` bounds raw CDP and control frames
before parsing. `x-evidence-body.mjs` requires a one-use qualified ingress ticket,
enforces a 4,096-byte decoded body limit, rejects malformed/duplicate/unknown
fields, and emits only invented schema/state/numeric-ID claims. Names and handles
are stripped before IPC. Tickets cannot survive observation loss or expiry.
The local freshness window does not constitute a server-authenticated challenge.

The dual-tag Go loopback constructor preserves the actual origin and operation;
it does not claim traffic came from X. A separately controlled server generation
supplies synthetic continuity. Happy, missing-identity, logout, account-switch,
malformed and oversized cases all remain non-authorizing. Native account proof,
all-worker lifecycle coverage, Store/controller wiring and production readiness
remain separate gates. A successful run cleans its owned profile root and keeps
the project-local build cache; a failure retains artifacts for diagnosis.

Use `-ChromeOnly` only when the prerequisite ingress/body and verifier/isolation
evidence is already valid for unchanged relevant inputs. The default command
runs every stage. Chrome loopback acceptance needs the scoped execution path
outside the socket sandbox; this does not authorize any user-runtime operation.
The Go server caps serialized wire bodies at 64 KiB and sets exact
`Content-Length`. The page trigger checks that length and consumes/discards
`arrayBuffer()` to avoid a direct stream close/cancel path observed in fixture
Chrome. This allocation bound relies on the controlled server; a remote header
alone would not establish it. No body leaves the page trigger; verification
still reads the body only through the qualified CDP ticket and 4 KiB parser.

`-Case oversized -Count 3 -ChromeOnly` isolates a bounded stability check with
three new profiles. The helper stops on the first failure and limits `Count` to
five. Request-clock regressions inject wall/elapsed clocks and sleep rather than
opening Chrome. A local catch-up wait never extends the original five-second
Go proof lifetime.

Final canonical acceptance passed 30 Node tests, 40 Go verifier top-level tests
and 65 subtests, tag/main isolation, the capped-JSON HTTP contract, and all six
actual Chrome cases. Oversized JSON also passed three fresh-profile samples
with exact size rejection. Every browser case used one login and two self
requests; physical process identity, inherited Job membership and empty-Job
cleanup were checked. Eleven protected file hashes were unchanged, and all
task profile roots were cleaned. Logs:
`runtime/qa/auth-session-sidecar/x-chrome-verifier-validation.txt` and
`runtime/qa/auth-session-sidecar/x-chrome-verifier-oversize-stability.txt`.
