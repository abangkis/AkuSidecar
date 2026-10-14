# One-login collector and native reader proposal

Status: Astra Medium proposal, 2026-10-04. Production enablement is not implemented;
limited synthetic Sidecar bindings are available only in an explicit fixture build.
The owner authorized a local-fixture PoC on 2026-10-13; real-authentication testing
requires separate confirmation. See the bounded experiment below.
Owner prioritizes avoiding a second login. Keep optimized shared-profile handoff
as the working baseline and Facebook's existing Browser collector exception.

## Current milestone: main dev pilot (2026-10-13)

The owner approved a bounded main-dev pilot milestone after fixture checkpoint
`dca33be` and the source-only lifecycle parity review. The planning estimate is
one to three focused working days, conditional on the gates below, not a release
date or a claim that production authentication is already compatible.

1. **DEV-1: ordinary launcher lifetime.** Exercise actual `appshell.Launch` using
   a project-owned synthetic executable with no UI and its synthetic child. Test
   parent death at the Start/Job-assignment boundary, then close any demonstrated
   ownership hole with a bounded shared-launcher correction and regressions.
2. **DEV-2: explicit development integration.** Wire reader priority, drained
   capture/reasoning, durable pause, never-claimed queue resume and account/browser
   verification behind an explicit development opt-in. Ordinary production builds
   remain disabled; define the source-session verification contract before
   claiming real-account compatibility. Do not silently promote the fixture binder.
3. **DEV-3: pilot acceptance.** Verify reader during an active batch, multiple
   windows/bootstrap/final close, restart/failure cleanup, account/session changes,
   cancellation and stale/duplicate rejection through the integrated dev path.
   Use synthetic profiles/login first; media/non-headless work needs explicit
   supported coverage or a closed admission boundary.

The current continuation updates these documents and implements DEV-1 first.
Passing a fixture does not activate a running main-dev instance. Testing actual
accounts, restarting user runtime, installing or registering components requires
separate explicit authorization. Commit/push is also outside this continuation.
Browser ownership remains a child of Sidecar; no independent keeper is planned.

## Approved lifetime boundary (2026-10-13)

The owner selected browser ownership beneath Sidecar. No managed child may have
an independent lifetime after its parent exits. In the current tagged fixture,
Manager ownership is in the Sidecar worker; its Node bridge and Chrome processes
are owned descendants, not a separate keeper. A child of AkuBrowser, independent
reader owner or reconnect to a surviving old browser is not the selected design.

Retain reader windows during normal collector/reader handoff while Sidecar is
alive. Full Sidecar shutdown or crash closes its reader/bootstrap windows and
owned browser tree. Restart recovers persisted queued work, not the old windows.
The same fresh fixture profile may reopen only after complete old-tree release
and scoped reopen authorization; admission still requires synthetic authority
and actual browser-session verification. Session failure cannot trigger a second
automatic login or automatic reader/foreground intent replay.

The candidate alternatives below are historical feasibility proposals. This
lifetime decision does not enable them or authorize production authentication,
runtime operations, installation or registration.

## Candidate architecture

Keep the existing signed-in application profile as the persistent normal Chrome
authentication owner and reader. Give true headless a different user-data-dir.
A Sidecar AuthSessionBroker exports only approved source authentication state
through the private CDP pipe and seeds disposable headless browser contexts.
Do not copy the entire profile or read/decrypt Chrome's cookie database.
An isolated context does not automatically inherit another context's login.

Suggested boundaries: AuthOwner, AuthSessionBroker, DerivedSession, source-specific
AuthProbe and AuthRevocation. Commands conceptually Export, Bootstrap, Validate,
Revoke carry an auth epoch and expected account identity. Secret payloads remain
local and in memory where possible, excluded from public API/status, logs,
receipts, arguments and repository files. This is not permission to export state.

Bootstrap cookie attributes faithfully, including HttpOnly, Secure, SameSite,
expiry and partition keys. Add localStorage/IndexedDB only where demonstrated
necessary; sessionStorage has separate handling. Normal profile is the application
authority. Do not merge headless state back or overwrite active contexts with old
snapshots. Rebuild a stale derived context once at a safe work boundary. If the
owner also needs authentication/challenge, route the user to that same login UI.

States: UNSEEDED -> BOOTSTRAPPING -> VERIFIED; expired/challenge -> STALE -> one
reseed -> VERIFIED or NEEDS_USER. Logout/account switch -> REVOKED; stop work,
discard snapshots/contexts and reject old-epoch results. In-site logout detection
and server session revocation must not be claimed instantaneous.

## Critical feasibility gates

Both browsers can receive server-driven cookie updates despite single application
authority. Concurrent refresh may invalidate the other browser's session. This
is a per-source no-go gate, not something to hide behind unlimited retry/merge.
App-Bound disk encryption differs from device-bound session credentials: portable
cookies do not prove the derived profile can refresh a session tied to a
non-exportable key. X/Instagram/LinkedIn adoption is unknown until inspected.

A persistent normal profile is not proof Chrome can stay alive without windows
or restart without blinking. Test cold start, closing the last reader window,
and keeping the auth owner available before promising background-only startup.
Do not claim a true-headless process can emit normal visible native windows.

## Bounded PoC proposal

Separate authorization before real auth export, new profiles or foreground tests.
Start with local fixture cookies/partitioning, rotation, logout, account switching,
epoch rejection and secret-free receipts. Then test X, Instagram, LinkedIn one at
a time: bootstrap same account, collector/reader concurrency, cross-browser
session invalidation, headless/Sidecar/application restart, observed refresh,
challenge recovery, logout/account switch and expiring media recovery. Keep
Facebook outside this auth research initially. A test across days is useful but
does not itself prove token refresh.

Track bootstrap, concurrency, restart, refresh and revocation independently per
source. An initial success is not one-login durability. If a source fails refresh,
retain its shared-profile handoff. Selective transfer is the main experiment;
full-profile snapshots after verified shutdown are only a possible comparator,
not an enduring sync design. Effort is medium-to-large: two lifecycle owners,
separate leases, auth epochs and worker context routing; reuse extractors,
admission, private CDP and trusted-click reader components.

## Local fixture PoC (2026-10-13)

The owner approved implementation and execution of local fixtures only, with
explicit confirmation required before real-authentication testing. The isolated
implementation and reproducible commands are in
[`scripts/auth-session-fixture/README.md`](../scripts/auth-session-fixture/README.md).
The existing application still uses exclusive shared-profile handoff.

Final validation: 15 broker unit tests passed; the real Chrome 154.0.8037.98
fixture passed 26 checks on Windows at 09:05 WIB. A headed reader and headless
collector with distinct fresh profiles served overlapping requests after one
synthetic owner login. Cookie attributes, actual path/partition isolation,
HttpOnly restrictions, owner/collector restart, broker-object replacement,
bounded reseed, logout notification, account switching and stale-result fencing
were exercised. Failed disposal now blocks new context allocation and reports
an explicit cleanup failure; it cannot silently report revocation success.

Two deliberate findings constrain the next stage:

1. Under a fixture server that invalidates the old credential on refresh, a
   derived refresh invalidates the owner session. The broker stops at
   `NEEDS_USER`; it never copies derived state back. Real source refresh behavior
   remains an independent go/no-go gate.
2. The normal owner browser exits when its last window closes in this launch
   configuration. The collector continues with its current valid session, but
   reseeding still requires an available owner. An owner can be reopened with
   its preserved synthetic profile without login; invisible owner lifetime and
   real Windows native activation have not been proven.

All scenarios through restart and owner-driven rotation retain a login count of
one. The final count of three additionally includes explicit simulated recovery
after the conflict and switching to the second invented account. This does not
claim that every failure can recover without user login.

Both browser roots exited, a separate Windows process inspection found zero
fixture Chrome processes, and all generated fixture profile directories were
removed. The receipt was printed to the task only; no acceptance archive or
credential snapshot is retained. No production runtime/config, source account,
installation, real capture or recapture was touched. Real-source feasibility,
application integration, and full Sidecar restart remain unverified.

## Central session vault alternative (2026-10-13)

The owner subsequently proposed a central ticket store rather than using the
reader as the permanent session authority, and approved extending the local PoC.
`scripts/auth-session-fixture/central.mjs` now holds a private in-memory cookie
snapshot with an epoch/version and coordinates managed browser requests. This
is a separate fixture alternative; the owner-authoritative proposal above and
production shared-profile handoff are not silently changed.

One simulated reader login bootstraps the vault. The existing collector browser
becomes the designated refresh executor, avoiding a third browser. Concurrent
refresh requests for the same version coalesce; new work is gated and already
admitted work drains before refresh. Only verified fanout makes the new version
available. Old, foreign, detached and revoked leases cannot start new work, and
stale results cannot be accepted. Failed publication closes admission; timed-out
adapter effects must settle before cleanup can claim completion.

Validation on Chrome 154.0.8037.98 at 09:36 WIB: the central real-browser fixture
passed 18 checks, with exactly one simulated login for the entire run. Two
overlapping reader/collector requests succeeded; three concurrent refresh
requests produced one server rotation. After reader detach and last-window exit,
the collector performed a second refresh. A reopened reader received the newest
version without another login. A third server rotation deliberately triggered
by an ordinary reader response was detected; its result and all subsequent work
were rejected with `CONFLICT` rather than choosing an arbitrary cookie snapshot.
Revocation cleared both client jars. Both Chrome roots exited; independent
Windows process inspection found zero fixture processes and zero profile
directories remaining. No receipt archive was saved.

The combined broker/central unit suite passed 36 tests (including subtests).
Coverage includes bootstrap from the reader into an initially unauthenticated
collector, in-flight rotation racing refresh, partial fanout, stale compare-and-
swap requests, partition-aware fingerprints, detach/reattach, logout fencing,
identity mismatch, and retryable cleanup after timeout or clear failure.

Remaining limits:

- The coordinator lives outside reader windows, but the vault is volatile:
  closing the coordinator loses it. Durable encrypted storage and actual Sidecar
  restart recovery are not implemented.
- Managed work finishing while refresh is pending is observed for cookie changes
  but its result is conservatively rejected in this prototype. Production must
  define retry/checkpoint semantics so useful capture evidence is not lost.
- The fixture guards only its explicit `use()` calls and rotation notifications.
  It does not intercept every real navigation, service worker, or background
  request. A central store alone cannot guarantee one refresher on a real site.
- Unexpected rotation has a safe stop, not automatic reconciliation. Real-source
  refresh protocol, binding constraints, and recovery rules remain unverified.

Real auth export/testing still requires separate user confirmation; this
authorization covered local fixtures only. Commands and detailed scope are in
`scripts/auth-session-fixture/README.md`.

## Switchable fixture strategies (2026-10-13)

The owner explicitly requested interchangeable strategies while prohibiting
real-auth access. `SessionStrategyController` in
`scripts/auth-session-fixture/strategies.mjs` wraps the existing broker as
`reader-owned` and central coordinator as `central-vault`. Both expose the same
start/switch, lease/use, refresh, attach/detach, revoke and status contract.
Selection is available through the local runner's `--strategy` argument and
runtime `switchTo(id)`; no production setting or auth code is activated.

A switch fences leases immediately, drains admitted work, verifies the same
fixture account, cleans the previous strategy and starts the target. A private
verified cookie snapshot bridges central cleanup into reader-owned bootstrap.
Old results cannot cross the generation boundary. Cleanup failure, identity
mismatch and drain timeout block publication; unsupported IDs leave the current
strategy intact. Concurrent control operations are rejected. A switch requires
an available fixture reader, while central refresh itself supports an absent
reader. No reader window is automatically opened by the controller.

At 09:58 WIB, Chrome 154.0.8037.98 passed 20 checks from each initial strategy,
including both switch directions, correct refresh ownership and in-flight drain.
Each run used exactly one invented login. The combined unit suite passed 50
tests including subtests. Independent process/directory inspection confirmed
zero fixture Chrome processes and profile directories after cleanup. No raw
acceptance archive, real credential access, production restart, commit or push
was performed. The volatile vault, managed-request observation and real-source
feasibility limitations above still apply.

## Local resilience extension (2026-10-13, 10:26 WIB)

The owner approved the next fixture steps while retaining the prohibition on
real-auth access. The PoC now observes attached-page CDP Network responses,
checkpoints pending read jobs across strategy changes, and explicitly checkpoints
synthetic tickets to a DPAPI CurrentUser-encrypted vault for process recovery.

An unexpected session-cookie response fences the controller and active engine,
including an in-progress refresh. A one-use nonce binds an expected refresh to
one actual request; simultaneous unrelated requests are not exempt. Conflicts
cannot be escaped by switching strategy. This extends observation beyond managed
`use()` callbacks to ordinary page navigation and fetches. It does not cover new
tabs, worker/service-worker targets, cookie changes without a network response,
or events before subscription. No automatic conflict reconciliation is added.

Stale read jobs stay pending and can be resumed with fresh leases after a control
operation. Attempts are bounded, foreign-account payloads are rejected, and each
job retains one admitted result. This checkpoint is in memory, not durable Store
admission or preservation of identical source content across repeated reads.

The vault is restricted to the invented fixture account/cookie and exact local
origin, with a 15-minute lifetime. It stores ciphertext only and requires a fresh
server identity probe before a recovered strategy becomes ready. A new Node
coordinator and new browser profiles recovered from it without login while the
fixture server stayed alive. A still-decryptable ticket invalidated by the server
was correctly rejected. Saves are explicit; production vault integration,
automatic update/revocation policy and actual Sidecar restart remain unimplemented.

Validation: 72 unit tests passed, none skipped; the real Chrome 154.0.8037.98 and
DPAPI resilience fixture passed 26 parent/child checks using one synthetic login.
All fixture Chrome roots/profiles and vaults were removed; independent Windows
inspection found zero remaining. No real auth, production runtime restart,
commit or push was performed for this extension. Reproduction and exact limits:
`scripts/auth-session-fixture/README.md`.

## Owned browser target observation (2026-10-13)

The next approved scope extends only the local fixture: new pages and worker
targets must be discovered automatically, network observation must be installed
before a paused new worker runs, and losing coverage must close session admission.
The browser is always a newly launched, privately connected fixture process;
there is no discovery of an existing user browser or production CDP endpoint.

The target manager reuses the per-target nonce-bound session observer. One
target's expected refresh must not exempt another target's response. Duplicate
CDP connections for the same page must not create duplicate independent refresh
authorities. A confirmed owned close retires its target; unexpected detach,
crash, unsupported relevant target or transport failure is a coverage failure.

Controller status distinguishes `EXTERNAL_SESSION_CHANGE` (observed mutation,
`CONFLICT`) from `SESSION_OBSERVATION_LOST` (coverage unavailable, `NEEDS_USER`).
Both invalidate old leases and prevent further session use. Neither condition
selects a new credential snapshot automatically. The runner is
`scripts/auth-session-fixture/run-browser-targets.mjs`; actual validation results
are recorded in the fixture README after execution.

This phase does not add production integration, real auth, automatic vault
refresh/revocation, or durable capture-job storage. Its scope is target lifecycle
and response observation after monitoring starts, not interception of all cookie
writes or proof that a real social platform's refresh protocol is supported.

Chrome 154 also exposes `browser_ui` and `background_page` targets in fresh
fixture profiles. These receive observation too; only requests proven to use
`chrome:`, `chrome-extension:` or `data:` resource schemes are excluded from fixture
cookie checks. HTTP requests from the same targets remain checked.

Real Chrome tracing found worker response metadata split across parent and
worker sessions. The bounded browser correlator therefore retains original
target/session provenance and pairs evidence only within a proven attachment
family, with unique observed request/response owners. Refresh authority must
never move across targets with the correlated response. Ambiguous or incomplete
evidence stops admission. Request IDs plus observed ancestry do not prove the
absence of colliding incomplete streams; this remains a local fixture result,
not a universal cross-target identity guarantee for production authentication.

The browser trials qualified new-page and dedicated-worker mutation detection.
They did not qualify shared-worker cookie visibility or HTTP service-worker
initialization: shared-worker rotations lacked cookie events, and the service
worker stalled at Network enablement. The implementation explicitly denies
coverage for these types rather than admitting unverified sessions. Discovery
of an unsupported service worker also invalidates an in-flight refresh. Internal
service workers are admitted only while their requests stay non-HTTP. Buffered
cross-target evidence requires a flush boundary; immediate global fencing is not
established. See the fixture README for exact final checks and reproduction.

## Isolated worker capability experiments (2026-10-13)

A separate small fixture now distinguishes worker functionality, response-cookie
evidence and checkpoint capability from browser-wide admission policy. At 14:05
WIB, 11 new unit tests and 28 Chrome checks passed. The service-worker baseline
registered and activated; one exact fixture connection enabled Network before
resume and observed the first commanded rotation's ExtraInfo and Set-Cookie.
The broad recursive observer's earlier initialization stall was not reproduced;
this comparison does not identify its precise cause or justify removing its gate.

The shared worker completed its rotation without worker-session cookie evidence,
but a direct Storage checkpoint detected the changed ticket. Both workers
preserved the original account during rotation; derived contexts with the old
ticket received 401, and explicit reseeding restored the expected account.
An intentional synthetic alpha-to-beta switch verified identity distinction.
These probes never infer account identity from a fingerprint alone.

Capability decision: service-worker response observation is feasible through a
scoped attachment; the shared worker has checkpoint verification only through
this path. Initial worker-script fetch remains unqualified for both, and no
instant browser-wide fencing or real-site authentication support is established.
Existing admission gates remain intact. The next policy phase must retain this
distinction when choosing concurrency or the existing exclusive handoff.
Reproduction, exact evidence scope and cleanup are in the fixture README.

## Reader-priority exclusive execution fixture (2026-10-13)

The approved fallback now has a generic local-fixture controller, separate from
the authentication strategy. It consumes capability evidence and injected control
adapters; it does not select credentials or hardcode a social platform. Concurrent
execution needs verified cookie observation, startup coverage and ownership.
Missing coverage downgrades to exclusive for this controller's lifetime; an
automatic capability upgrade cannot reverse that decision.

An exclusive reader request immediately fences batch leases, aborts/drains active
reads and pauses the owned collector before reader opening. Pending work remains
in the existing in-memory queue, and obsolete results cannot publish. Existing
reader windows are preserved. Collection resumes only after authoritative
inventory confirms no readers remain and fresh session verification proves the
expected account. Account mismatch, logout, unknown inventory or control failure
requires user action. Late partial starts are tracked and cleaned; incomplete
physical shutdown is explicitly reported as cleanup pending.

At 15:03 WIB, 19 unit tests and 30 Chrome checks passed. The real Chrome fixture
preserved two exact reader windows during downgrade, retained/retried batch work,
and blocked restart after synthetic account change or logout. All owned browsers
and profiles were removed, with zero independently observed leftovers.

The browser runner supplies synthetic capability flags and invented loopback
tickets. It verifies exclusive collection scheduling, not universal worker
coverage or production shared-profile handoff: a bootstrap authentication page
remains open outside explicit reader inventory. Production adapters, close-event
integration, durable queue retention and real authentication are still deferred.
The controller has no reader-close adapter; test actors alone simulate user close.

## Limited synthetic Sidecar bindings (2026-10-13)

The approved integration reuses the real Go Coordinator/Manager/Engine and SQLite
command path. It is gated by the `auth_session_fixture` build tag plus development
config and an explicit injected synthetic probe; ordinary startup has no binder
or setting that enables it. Authentication strategy selection is not changed.

Reader priority closes admission before waiting for the current collector step
and reasoning to drain. It then persists a session pause flag and releases the
drained session lease for the existing owner handoff. Unstarted source runs and
queued headless commands survive in the real Store. A transactional claim gate
honors the pause flag. After verified reader release and expected-account checks,
never-claimed queued commands get the new Engine epoch and owner generation.
The fixture can resume these commands after Engine and SQLite replacement.
Cancelled/completed work is not resurrected. Drain failure or invalid identity
keeps admission closed; reader windows are never automatically closed.

This adapts exclusive execution to Sidecar; it is not concurrent real auth reuse.
Nineteen fixture leaf cases passed, plus relevant default regressions and an
ordinary Sidecar build. A loopback invented-cookie login/logout test and the
existing close-and-resume HTTP handler are included. Capture processes, profile
inventory and ownership-release adapters are fake, so physical Chrome lock
handoff and OS broker opening remain unproven. Active media recapture and queued
non-headless routes are rejected in this bounded fixture. Real profiles,
production processes and authentication remain untouched. See the fixture README
and `scripts/test-auth-session-sidecar.ps1` for reproducible commands and scope.

## Owned Chrome fixture adapter (2026-10-13, validated locally)

The owner authorized actual Chrome fixture processes with a new profile and
invented loopback authentication. This phase connects the tagged Go fixture
to a narrow Node/private-pipe adapter, without enabling ordinary startup or
connecting to existing browser sessions. The same new profile is used by
sequential collector and reader owners; the fixture never needs a second
automatic login to repair a failed handoff.

Windows Job Object membership is checked before profile reuse. The Node bridge
joins the job before it can launch Chrome, every returned Chrome PID must belong
to it, and the complete job process list must return to bridge-only before a
successor launches. Natural browser exit is not published as owner release until
the Chrome descendants have exited. Actual window inventory includes bootstrap;
remaining windows keep the reader owner and batch pause intact.

The fixture uses server-side synthetic authority before launch and an actual
browser-session probe after launch, matching account and revision before queue
admission. The original Chrome-created bootstrap target is navigated to the
loopback fixture and stays in window inventory. Explicit final-window close
waits boundedly for the exact root exit; Chrome descendants must still be gone
before the Go owner publishes release.

Seven real-Chrome leaf cases passed, including profile reuse, actual Engine
capture drain and resumed Chrome capture, multiple windows, Engine/SQLite
replacement while retaining the reader/Manager/bridge, cleanup rejection,
unknown inventory, logout/account change and loss of the browser's fixture
cookie while server authority remains valid. The existing 19 synthetic cases,
five-package default regression selection and ordinary build passed. The
standalone fallback regression passed 30 browser checks. Fixture output and
owned Chrome/Node processes were independently confirmed cleaned up.

This is a local Windows fixture result, not a production Sidecar process restart
or real-source DOM/authentication test. It cannot establish Windows foreground
activation, full OS broker opening, concurrent authentication reuse or real-site
compatibility. See the fixture README and the opt-in `-VerifyChrome` helper for
the exact scope and validation record. No commit/push or user runtime operation
was performed for this delta.

## Isolated OS reader-broker fixture (2026-10-13, validated locally)

The next authorized phase stages a real native fixture UI, broker helper and
test server in a new project-local output directory. Tagged wrappers choose a
fresh local pipe while reusing the production parent/image/Job authentication,
request validation, one-use ticket and HWND/PID/property activation checks.
Ordinary builds exclude these entry points. No host registration or existing
browser profile is involved; source-shaped HTTPS metadata maps only to the
invented loopback page.

The actual first request failed because UI foreground changed during Chrome
preparation. The retained Chrome reader and paused queue were verified before
arming a native Retry explicit reader button. A coordinated physical click
created a fresh request, helper and ticket against the same retained owner.
Actual activation returned Applied and Readback success; independent Windows
checks matched the exact HWND/PID, visible and not minimized. The original
request was not replayed, its deadlines were not extended, and no input injection
or foreground privilege delegation was used.

The full opt-in broker command passed, including actual identity/binding
rejections, single-use native binding consumption, one synthetic login, retained
reader/bootstrap ownership and resumed capture only after natural root exit and
whole-Job release. A separate protocol test rejects ticket replay on a new
exchange. Default broker/appshell regressions, the 19 synthetic cases,
five-package regression selection and ordinary Sidecar build passed. The seven
existing Chrome leaf cases passed again with the aggregate cleanup marker;
independent inspection found no remaining fixture processes.

Cold first-request foreground activation remains unresolved. A physical fixture
button is not proof of the installed extension/native-host journey. Changes to
foreground or session after ticket claim, full production binary restart,
production DOM extraction and real-site authentication remain outside this
result. No real authentication, user runtime operation, installation, commit or
push occurred in this phase. See the README's `-VerifyBroker` command and exact
accepted path for reproducible scope.

## Cold first-request fixture continuation (2026-10-13, validated locally)

After the validated broker delta was committed and pushed as `f141dc6`, the owner
authorized the next bounded fixture phase. Its acceptance uses one physical
Open explicit reader action before Chrome preparation; a retry against a retained
reader cannot satisfy this gate. The new preset follows production normal
window startup and reuses the initial target, while preserving ownership of the
actual startup/bootstrap target and any later independent reader window.

The cold fixture passed its first actual request: `cold_actual_success`,
Readback true and Applied false, exact native HWND/PID visible and not minimized.
The normal initial reader was already foreground; the existing broker verified
it without an extra activation write. One synthetic login persisted. Closing the
initial reader/bootstrap left the independent window owning the session and
holding batch pause; final close and natural whole-Job release preceded resumed
capture (three launches/release proofs). Five binding negatives, changed UI
foreground before a new request and native binding replay rejection passed.
Parent/image negatives retain their separate preceding warm-case evidence.

For coordinated input, the owned cold UI is shown without activation, placed once
on the active monitor with temporary topmost state, and verified visible with an
enabled button. The physical click removes that state before helper launch. Its
in-window instruction tells the owner to click when enabled, without waiting for
chat. Earlier timeouts reached no reader request because chat coordination was
late; they are not failed native-reader evidence.

The exact `-VerifyBroker -BrokerTestRegex '^TestBrokerFixtureOSColdFirstRequest$'
-VerifyDefault` helper exited zero. Tagged contracts, 19 synthetic cases, ordinary
broker/appshell regressions, five-package default selection and ordinary build
passed; seven existing Chrome leaf cases and 30 standalone fallback checks also
passed with the shared launcher change. Fixture jobs and output were cleaned.

Production identity, focus, single-use binding and lifetime guards stay unchanged.
The preset addresses the fixture/production preparation mismatch; it does not
prove the installed extension/native-host flow, full production binary restart,
changes after ticket claim, production DOM extraction or real-site compatibility.
This phase does not register a native host, change authentication, touch an
existing profile or operate user runtime. After validation, the owner separately
authorized commit/push of the nine-file cold delta on 2026-10-13.

## Full fixture process lifetime review (2026-10-13, validated locally)

The owner authorized review and testing of full Sidecar fixture process exit and
restart after commit `cf8ea50`. The test must replace the OS process and its
Engine, Manager and bridge, using persisted SQLite and the same profile created
fresh for that isolated test. The earlier component restart retained all of those
process-owned objects and does not establish this boundary.

The Chrome fixture Job uses KILL_ON_JOB_CLOSE. Measure actual reader/window fate
on graceful exit and abrupt termination; a controller must not hold that inner
Job alive to claim survival. Bound all child processes with independent cleanup
ownership and prove the old tree released before profile reuse. Queue pause,
account/browser proof, fresh epoch and stale/duplicate rejection must be checked
before recovered work is allowed to proceed. No second automatic login may
repair a failed session check.

This phase uses invented loopback authentication and only owned fixture process,
profile and database paths. Whether the executable uses production startup or a
tagged fixture composition must be explicit in the results. No real runtime,
authentication, installation, registration, commit or push is authorized. A new
process-owner architecture for surviving reader windows requires a separate
scope decision after the baseline evidence.

The reviewed implementation uses a dedicated fixture-tagged command with private
startup IPC before browser launch. Actual parent PID/creation time, staged image,
controller lineage and directory identity bind the newly created run. A separate
one-use profile-reopen grant crosses that private channel only after the controller
has observed zero old containment-job members and release of recorded reader
windows. The ordinary new-profile launch path remains strict. Each replacement
gets a fresh Engine epoch; Manager generation is local and may restart at one.
Compare otherwise identical valid pending payloads to isolate old-epoch rejection
from the independent duplicate-completion check.

The dedicated `cmd/auth-session-process-fixture` command passed graceful and
abrupt OS worker replacement. Both actual PIDs changed; old reader/bootstrap
windows were destroyed and the complete old tree was empty before successor
launch. No inner Chrome Job handle was held by the controller. Reader closure
is therefore the current fixture contract, not window-preserving restart.

The same owned profile/SQLite retained run IDs, account/revision, one synthetic
login and two collections. Fresh epochs rejected an otherwise valid payload
changed only to the old epoch; duplicate current completion was separately denied.
Seven malformed grant fields and persistent one-use replay were denied. Missing
external authority kept durable pause despite browser authentication; restored
invented authority and explicit recheck allowed only real binder/browser recovery.
Pause was cleared by existing hooks, not test-side mutation.

The final `-VerifyProcessRestart -VerifyDefault` helper exited zero (lifecycle
5.03 seconds). Nineteen synthetic cases, tagged contracts, five-package defaults
and ordinary build passed; seven Chrome cases and 30 standalone fallback checks
also passed. Owned jobs/output and independent process inspection were clean.
This tests full fixture worker replacement, not production main startup. The
subsequent approved lifetime boundary accepts reader closure on parent exit;
keeping old readers alive is not a remaining requirement. No keeper or production
change was introduced. Authentication and user runtime were untouched. This
validation preceded commit/push; the owner subsequently
authorized committing and pushing this fixture delta on 2026-10-13.

## Child-Sidecar lifecycle acceptance (2026-10-13)

The standalone owner/reconnect proposal was reviewed and rejected by the owner.
Keep the existing process boundary rather than introducing a new owner process:

```text
Fixture controller (synthetic authority and test cleanup only)
  -> Sidecar fixture worker (Engine / Store / Manager / Coordinator)
       -> owned Node bridge
            -> owned Chrome tree and reader/bootstrap windows
```

The controller's outer containment Job is a test cleanup guard. It must not hold
the worker's inner Chrome Job handle, preserve an old reader, or force-clean the
old Job before the test claims ordinary parent-death cleanup succeeded. A bounded
wait may observe kernel/process teardown; unknown or nonempty ownership prevents
profile reuse and successor launch.

Acceptance remains distinct from same-process Engine/SQLite replacement:

- Normal handoff keeps reader ownership until the last actual window, including
  bootstrap, closes. Capture drains before reader ownership, and resume uses the
  existing authority/browser-session checks.
- Graceful and abrupt full worker exit must both destroy old native windows and
  empty the old process tree before a successor receives a reopen grant.
- Restart constructs a fresh Engine epoch and local Manager. Only never-claimed
  pending commands can be rebound; old results, completed duplicates, cancelled
  work and previously claimed commands cannot be revived as new work.
- Account/revision mismatch, unavailable authority, unknown inventory or failed
  cleanup keeps admission closed. The fixture never repairs these by login.

Baseline `f17ca37` already records graceful/abrupt worker replacement,
`readerPreserved=false`, `innerJobDuplicated=false`, whole-tree/window release,
same owned profile/SQLite, one login, fresh-epoch and duplicate rejection, grant
negatives and retained pause on missing authority. Reuse that evidence while
the relevant implementation is unchanged; add a test only for a concrete gap
against this contract. The dedicated worker does not establish production main
or installed extension/native-host lifecycle behavior.

The source review confirms that abrupt worker testing terminates only the worker
root, then checks the old outer Job and windows before authorizing replacement.
Controller force-cleanup is deferred until after those assertions. It therefore
does not manufacture a passing parent-death result.

The new bridge-crash case found an actual fixture defect: after killing only the
verified Node bridge, Chrome/windows were gone but Manager remained Ready because
the active owner's Done never settled. The first correction also exposed that an
empty Job process list can precede the Chrome root handle's exit signal. The
tagged adapter now waits for actual Node exit, terminates/verifies only its owned
Job, waits on Chrome root handles pinned while membership was known, and then
publishes owner failure. Unknown transport cannot silently retain a usable owner.
The pinned handle is checked against the exact Job; late owner registration after
bridge EOF is rejected under the owner-map mutex. No PID is reopened after death
to manufacture release evidence, and no inner Job handle is duplicated.

The regression observes root signal, verified empty tree and destroyed recorded
reader/bootstrap windows before Manager Done failure; Acquire/Accepts are denied
without relaunch, login or capture. Both process-restart cases now also wait for
the pre-stop Chrome root handle before issuing the profile-reopen grant.

Final `scripts/test-auth-session-sidecar.ps1 -VerifyChrome -VerifyProcessRestart`
exited zero: the eight Chrome cases passed in 17.027 seconds and graceful/abrupt
process replacement in 4.99 seconds. Nineteen synthetic cases and tagged broker
contracts passed through the same helper. Existing ordinary-build/default and
standalone-30-check evidence is reused because those inputs did not change.
Independent inspection found zero matching fixture processes and zero run
directories. This continuation changes three tagged Go files plus three documents.
Validation preceded the owner's subsequent commit/push authorization on 2026-10-13.

At that checkpoint, startup before Node Job assignment remained unverified:
Chrome cannot launch before assignment/private RPC, but the initialized-owner
test did not prove Node lifetime during abrupt Go startup death. The authorized
continuation below addresses that separate fixture boundary. Production startup
remains unproven; authentication and user runtime were untouched.

## Pre-assignment startup death fixture (2026-10-13, authorized continuation)

The owner authorized commit/push of the six-file child-Sidecar delta, published
as `168cc5d`, then testing actual Sidecar worker death during startup before Node
joins the inner Chrome Job. This is a new fixture phase, not production startup
or permission to touch a user's authentication/runtime.

Use a deterministic private startup barrier after the verified Node ready
handshake and before Job assignment. The controller must observe the actual Node
identity and prove it is not yet in the worker's inner Chrome Job before stopping
only the verified Go parent. Pin the real process handles before stop; require
Go/Node exit and empty outer containment before any fallback force-cleanup. A
live-parent stdin-close simulation or random kill timing does not satisfy this
gate. No Chrome launch, reader/bootstrap window, login or capture may occur.

Keep existing private startup authentication, project-owned fresh run/profile
scope and conservative recovery. Add a startup fix only if the test demonstrates
a lifetime failure. Do not introduce an independent owner, weaken containment,
or issue a recovery grant on unknown release. Record final results and cleanup
after the actual OS-parent-death case runs.

The narrow canonical startup command passed on its first measured attempt:
`-VerifyProcessRestart -ProcessTestRegex '^TestProcessFixtureStartupBeforeNodeJob$'`
(0.12 seconds). The barrier reported actual Go PID 34884 and Node PID 37312,
verified Node image/creation/direct parent, known false inner-Job membership and
zero inner members. Independent controller queries proved exact outer Go+Node
membership, with no windows/profile/SQLite/login/capture before stop.

Only Go was terminated. Both pre-stop process handles signalled and the outer
tree became empty while the controller retained its outer Job, before fallback
cleanup. No recovery grant was issued. Node exited naturally in this pre-Chrome
fixture state; no startup lifetime fix was needed. The private barrier and test
do not establish production startup or all possible instruction-level crash
points. The final regression/cleanup readback follows below.

Final `scripts/test-auth-session-sidecar.ps1 -VerifyChrome -VerifyProcessRestart
-VerifyDefault` exited zero: startup 0.12 seconds (Go 40320, Node 19572), eight
Chrome cases 17.164 seconds, full graceful/abrupt restart 5.10 seconds. The startup
case again proved known false inner membership, exact outer Go+Node, signalled
root handles and empty containment before fallback, with no browser/state/login/
capture or reopen grant. Nineteen synthetic cases, tagged broker contracts, the
five-package default regressions and ordinary Sidecar build passed. Existing
session/epoch/duplicate/grant checks passed; the standalone-30 evidence remains
valid on its unchanged JavaScript inputs.

Independent process inspection and project QA inspection found zero fixture peers
and zero run directories. The new delta is four fixture/helper files plus three
documents; validation preceded the owner's subsequent commit/push authorization
on 2026-10-13. It measures Node-ready/before-assignment, not
earlier pre-ready intervals, production startup or real-site authentication.

## Before-ready death and handshake timeout (2026-10-13, authorized fixture gate)

After checkpoint `112f2df`, the owner authorized testing actual Sidecar worker
death before Node sends ready, and a stalled handshake through its existing
eight-second timeout. These are separate gates from the preceding Node-ready,
before-Job-assignment case. Use the actual fixture Node bootstrap and a private,
fresh-grant-only deterministic barrier; no ordinary startup setting or public
endpoint may activate the fault.

For parent death, verify actual Node identity/direct parent and pinned handles,
known unassigned inner membership and exact outer Go+Node peers, then terminate
only Go. Require both process exits and empty containment before fallback, with
no Chrome/window/profile/SQLite/login/capture or reopen grant. For timeout, keep
Go alive and ready withheld, exercise the original eight-second deadline, and
require explicit timeout failure plus verified child/tree cleanup. EOF is not
process-release proof; do not change the timeout merely to make the test pass.

Only a demonstrated fixture failure justifies a bounded startup correction. Real
authentication, user runtime, installation and registration remain outside scope.
Commit/push was excluded from implementation scope and separately authorized
after validation, as recorded below.

The first narrow run measured both gates. Actual pre-ready Go death passed
(0.12 seconds), but the eight-second timeout exposed a release-order defect:
the error returned at 8021 ms with the pinned Node handle still WAIT_TIMEOUT
while Go stayed alive. `stopBeforeChrome` had waited for stdout/closed, which
precedes the sole `cmd.Wait()` completion. Later parent teardown was not accepted
as evidence that cleanup succeeded at return.

The tagged bridge now signals a separate Node-wait channel after that real wait
and startup cleanup waits for it. The eight-second handshake timer is unchanged;
unverified wait is recorded as cleanup failure. Corrected narrow cases passed:
pre-ready death 0.16 seconds (Go 3252, Node 26004); timeout 8.07 seconds (Go 1440,
Node 6508), with 8042 ms elapsed, ready false, exact timeout classification and
pinned Node signalled at return while only the still-live Go remained in the
outer Job. No Chrome/window/profile/SQLite/login/capture or reopen grant occurred.
The final broader regression and independent cleanup readback follows below.

Final `scripts/test-auth-session-sidecar.ps1 -VerifyChrome -VerifyProcessRestart
-VerifyDefault` exited zero. Eight Chrome cases passed in 17.522 seconds and the
process suite in 13.477 seconds: full restart 5.16, Node-ready startup 0.11,
pre-ready death 0.12 and original timeout 8.06 seconds. Final pre-ready Go 51904 /
Node 8172 released both roots and the outer tree before fallback. Final timeout
Go 12908 / Node 24600 reported 8026 ms, ready false, pinned Node signalled at
return, Go still alive and exact Go-only containment before its error exit.
Both new cases performed no browser/state/login/capture/reopen work.

Nineteen synthetic cases, tagged broker contracts, five-package default regressions
and ordinary build passed. Existing browser/session/epoch/duplicate/grant checks
passed. Only the Go/Chrome RPC driver JavaScript changed; shared launcher and
standalone inputs did not, so the preceding 30-check evidence is reused.
Independent inspection found zero matching fixture peers and zero QA run
directories. Validation of three tagged Go files, one fixture driver and three
documents preceded the owner's subsequent commit/push authorization on 2026-10-13.
No production code, authentication or user runtime changed.

The pre-ready barrier measures actual driver bootstrap before ready emission;
instruction-level deaths before that marker (such as module loading), production
main startup, installed native-host behavior and real-source authentication remain
outside this result. No deadline extension or owner-survival design was introduced.

## Ordinary headed launcher ownership gate (DEV-1, authorized)

The read-only comparison at `dca33be` found a different startup boundary:
ordinary headed UI/capture/native reader starts Chrome before assigning its root
to a Sidecar-owned Job. The earlier Go/Node fixture does not exercise that path.
On assignment failure the private-CDP launcher quarantines its pipes because
descendant cleanup is unknown; source inspection alone did not demonstrate a
surviving production Chrome process.

Use actual `appshell.Launch` from a dedicated fixture Go process and a new
project-owned profile/run. Its synthetic executable creates a synthetic child
without UI, authentication or Chrome. A private, tagged startup barrier exposes
the exact Start/assignment interval. Independently verify parent/root/child image,
creation identity, pinned handles and membership. Hold only outer test cleanup
containment, never an inner Job handle that would preserve children.

Terminate only the verified Go parent and measure root/child fate before fallback
cleanup. A survivor is a failed lifetime assertion, not a pass manufactured by
the controller. If demonstrated, correct the shared launcher startup boundary
while preserving arguments, private inherited handles, real process waits and
conservative failure reporting. A suspended but unassigned child is still a child
that can outlive its parent and is not sufficient proof of this invariant.

After a correction, rerun relevant ordinary appshell/headless/capture regressions,
the existing Chrome/process fixtures and ordinary build. No UI/foreground, real
authentication, user runtime, independent owner or automatic activation belongs
to this gate. Record exact baseline, final validation and residual limitations.

The first actual-launcher baseline reproduced a failure: the verified Go parent
PID 7352 exited, but synthetic root 27548 and child 31276 both returned
`WAIT_TIMEOUT` (258) and remained in the outer controller Job before fallback
cleanup. The private per-window Job was still empty. Image, creation identity,
actual parent lineage and pinned handles were checked; no window, login or
capture occurred. This is a measured synthetic startup failure, not evidence
that an existing user Chrome process leaked. The correction must also preserve
per-tree cleanup while the Go parent remains alive; process-lifetime containment
alone does not prove that already-created descendants belong to a later Job.

The bounded correction establishes an unnamed, noninherited process-lifetime
Job containing the Go process before managed launch. Only process exit releases
that handle; per-window cleanup must never close it because Go itself is a
member. Children created after initialization inherit this lifetime boundary.
The ordinary Windows `Launch` and `StartOwnedCommand` paths additionally create
their roots suspended, assign and verify the per-tree Job, then resume the
verified startup thread. This prevents root code from creating descendants
before their per-tree ownership exists. It retains `exec.Cmd.Start`/`Wait`,
arguments and private pipe inheritance rather than replacing process creation.

Nested Jobs are required. No UI, resource or breakaway limits are added. Failure
to establish ownership prevents execution; a failed startup uses bounded
terminal-wait verification and reports unverified cleanup conservatively. The
process-wide lifetime Job applies to children created after initialization, not
only one reader. This source correction does not enable reader-priority binding
or modify any running user instance. Final regression evidence is required
before declaring DEV-1 complete.

### DEV-1 final evidence (2026-10-14)

DEV-1 is complete at source/fixture scope. The canonical command
`scripts/test-auth-session-sidecar.ps1 -VerifyDefault -VerifyChrome
-VerifyProcessRestart` exited zero after the final shared-launcher changes.
The Chrome suite passed in 16.354 seconds: seven top-level tests / eight leaf
cases, including separate logout and account-change cases. The process suite
passed in 21.265 seconds: seven top-level tests / nine leaf cases.

Four new launcher cases exercise the actual shared paths:

- Before per-tree assignment, Go 30320 / suspended root 35036 had verified
  lifetime membership and a noninherited lifetime handle. Only Go was killed;
  the root signalled and the outer Job was empty before fallback (0.09 seconds).
- Injected rejection before assignment returned after actual `Cmd.Wait` terminal
  proof and pinned root release, while Go stayed alive. Root code never executed
  and the inner Job stayed empty (0.06 seconds).
- Resumed `Launch` and `StartOwnedCommand` each created a synthetic child with
  verified inner/lifetime membership and actual lineage. Killing only Go
  released every pinned peer before fallback.
- `CloseForRetry` of one tree released its root and child while Go and the
  sibling worker tree stayed alive; subsequent Go death released the remainder.
  The two resumed cases together took 8.32 seconds, including the existing
  eight-second graceful-before-hard-cleanup timeout for a windowless root.

Nineteen synthetic leaf cases, broker contracts, five-package default regressions
and ordinary Sidecar build passed. Relevant appshell source contracts,
headless/capture runtime regressions and a Linux appshell compile check passed.
The ordinary private-CDP smoke additionally used existing checksum-pinned CfT
152.0.7977.54, a fresh profile and loopback page: pipe roundtrip, exact target
close, clean root exit, empty per-tree Job and pipe release after drain passed
in 0.47 seconds. This directly exercises ordinary `appshell.Launch`, unlike the
separate Node/Chrome adapter fixture.

An earlier broad run failed the Node-only shared-profile smoke at initial
`bridge.launch` with `fixture_chrome_closed`. The other Chrome cases ran, and
the final complete run passed. Its cause remains unidentified; no correction
to that adapter or claim of a fixed Chrome startup flake is made. Standalone
JavaScript inputs were unchanged, so preceding standalone evidence is reused.

Independent read-only process inspection found zero matching fixture peers;
QA run directories and disposable ordinary-CDP profile directories were empty.
Ignored, project-local baseline/final transcripts remain under
`runtime/qa/auth-session-sidecar/launcher-dev1-*.txt`; no disposable binary is
retained. There are 15 source/test/document files changed, with no commit/push.
Real authentication, user runtime, foreground operations, installation and
registration were untouched. Clean-machine/installed-main startup parity remains
unmeasured; the shared code correction does not enable the reader feature.

The next milestone remains **DEV-2**, explicit development integration of
priority/drain/durable pause and verified resume, followed by **DEV-3** integrated
synthetic pilot acceptance. Actual-account verification and user runtime
activation still require separate authorization.

### DEV-1 checkpoint and DEV-2 authorization (2026-10-14)

After validation, the owner explicitly authorized commit/push and continuation.
All 15 DEV-1 files were committed as `7740deb` and pushed to `origin/main`; remote
readback matched `7740deb565c1ffd1099d40db4005339215bbec5b`. Existing validated
payload evidence remains applicable because commit/push changed no code.

DEV-2 source implementation is now authorized: explicit development opt-in,
reader priority/drain, durable pause and verified resume with synthetic tests.
Define the account/browser session capability before enabling the policy; Bridge
permission readiness cannot substitute for session authority. Missing or unknown
verification must deny admission/resume. Ordinary defaults stay disabled; real
authentication, running user instances, installation/registration and foreground
operations remain outside scope. DEV-2 changes require a subsequent commit/push
authorization after their reviewable result.

The bounded implementation uses a distinct `auth_session_dev` capability and
explicit development opt-in. The concrete synthetic runner also requires the
fixture adapters (`auth_session_dev` plus `auth_session_fixture`) and dispatches
from the actual Sidecar executable before ordinary runtime startup. It accepts
only private, fresh project-QA grants with verified process/profile lineage.
This runner shares the new development binding/startup contract and the existing
Chrome/Engine/SQLite/Coordinator orchestration; it must not call the old fixture
binder as a substitute for the new development policy. Ordinary builds, non-dev
launch and missing/unsupported session providers reject opt-in before state or
runtime mutation. Real-source adapters remain unavailable.

Evidence distinguishes synthetic and real sessions and binds source/profile/
account/revision to the current browser owner, including its actual generation
and process identity. Unknown, stale or conflicting authority denies admission
and resume without login. Unsupported media and non-headless routes remain
closed. Actual ordinary UI/native-broker acceptance belongs to DEV-3 rather than
being inferred from an Engine borrow or this isolated runner.

### DEV-2 source/synthetic result (2026-10-14)

DEV-2 is complete within the selected source/synthetic contract, based on pushed
DEV-1 `7740deb`. Actual `cmd/akusidecar` dispatches the explicit
`--dev --dev-session-reuse=synthetic-fixture` mode only in a Windows build with
`auth_session_dev auth_session_fixture`. The default build rejects the opt-in,
and an `auth_session_dev` build without a concrete adapter rejects it as
unavailable. These rejections precede ordinary config, Store, provider and
browser startup. The successful provider accepts private grants only from the
verified actual `engine.test.exe` controller under the same fresh QA task root;
it validates image, creation identity and profile lineage before work.

The new `DevelopmentSessionCapability`/`StartDevelopmentSession` contract uses
source/profile/account/revision plus current browser generation/PID/driver and
Engine epoch. Browser evidence comes from the actual queried owner, revalidated
before/after the probe and against physical profile directory identity. Invalid
evidence leaves capture admission closed; mismatch during handoff latches pause
and does not recover merely through Coordinator Retry. Bridge permission is not
session proof. The shared lifecycle retains the old fixture wrappers separately;
development status identifies this binding and reports its declared source scope.

Active capture and associated accepted-capture reasoning drain before reader
ownership. Durable pause gates SQLite claim transactionally, restart restamps
only never-claimed work, and cancel while paused does not revive capture. New
media/Browser work is rejected before creation/claim, including passive media
mutation. Previously frozen Browser/Facebook plans are rejected before startup
resume even if newer settings are headless. There is no real-source verifier.

Canonical validation from the project root:

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault -VerifyChrome -VerifyProcessRestart
```

That command exited zero: dev engine suite 13.220 seconds, existing Chrome suite
16.632 seconds (8 leaf cases), existing process suite 21.439 seconds (9 leaf
cases), 19 synthetic cases, broker contracts, config/main tests and default
five-package regressions/build. All three actual-main variants compiled. After
the final development-only frozen-plan guard, `-VerifyDevelopment` exited zero
again in 12.293 seconds, including actual-main graceful/abrupt restart in 4.67
seconds. Shared/default inputs were unchanged after the complete run.

Physical actual-main restart proved old Chrome roots/windows and whole trees
released before fallback, persisted SQLite IDs/profile, a fresh Engine epoch,
one synthetic login across restart, rejection of old/duplicate results and no
capture until renewed authority. Twelve foreign/unknown evidence cases, startup
missing-provider/non-dev/not-ready rejection, accepted-reasoning deferral,
capture/reasoning drain, mismatch latch, durable restart, paused cancellation
and two unsupported frozen-plan cases passed. Five CLI rejection variants
proved ordinary state stayed absent.

Initial unit failures were incorrect assertions: a pending-reader busy failure
was mistaken for verifier denial; a weak post-cancel tail observed the expected
stale-owner denial. The predicates were corrected, and cancellation was replaced
with a dedicated paused-cancel proof. An initial sandbox loopback-denial run was
retried outside the sandbox within the authorized synthetic scope. Exact
commands, initial failures and final success remain in the ignored project-local
`runtime/qa/auth-session-sidecar/dev2-validation-index.txt` and linked transcripts.

Independent read-only inspection found zero matching fixture peers and zero
disposable QA run directories; `git diff --check` passed. There are 27 uncommitted
source/test/helper/document files after DEV-1. No DEV-2 commit/push, real
authentication, user runtime, installation, registration or foreground operation
occurred. Ordinary UI/native-broker startup and all reasoning lanes are not
proved: this synthetic dispatch bypasses ordinary main startup and covers the
accepted-capture reasoning lane. The next gate is DEV-3 through the actual UI/
native-broker journey with synthetic authority; real accounts and user runtime
activation remain separately authorized gates.

### DEV-2 checkpoint and DEV-3 authorization (2026-10-14)

The owner explicitly authorized commit/push of all 27 DEV-2 files and DEV-3
UI/native-broker testing with fresh profiles and synthetic login. Commit
`0498548` was pushed to `origin/main`; local and remote readback matched
`04985483bd2b4c961e7b034826f6e8760dd48b34`. Commit/push changed no payload inputs,
so DEV-2 acceptance evidence remains applicable.

DEV-3 must exercise explicit UI intent through the unchanged native-message
helper, broker authorization, exact HWND/PID/marker binding, foreground readback
and reader lifetime on the new development policy. An Engine borrow alone or
the old fixture-only binder is insufficient. Fresh project-owned profiles,
private IPC and loopback synthetic authority remain mandatory; no original
authentication, user runtime, host registration or installation is authorized.
Physical-click trials require an unambiguous ready/button cue and adequate
waiting time; do not treat operator timing or fallback cleanup as acceptance.
Record which actual UI/main paths run, and keep any untested ordinary web UI or
installed-native-host parity distinct. DEV-3 changes remain uncommitted until
a subsequent explicit checkpoint authorization.

The selected DEV-3 composition runs inside actual dual-tag `cmd/akusidecar`:
an owned Win32 development UI submits the existing authenticated split-capture
API action and directly launches the unchanged native-message helper. Actual
`httpapi.Server.HandleReaderBroker` performs the request handoff; production
parent/image/Job/age authorization and HWND/property/readback logic are reused.
Fixed invented X metadata maps only to a loopback fixture post. This establishes
the native UI/API/helper/server path, not the ordinary web frontend or installed
extension/native-host registration.

The first positive trial requires one actual physical Open click while capture
is active. Drain capture and hold related reasoning long enough to prove reader
ownership has not advanced; then require first-request exact-reader foreground
readback without a warm retry. Count the startup bootstrap and extra reader
window in ownership; closing one must retain pause, and final close must resume
never-claimed work under verified synthetic session/account. Unknown or foreign
bindings are denials rather than fallback successes.

After positive completion, a dev-only internal pause operation may prepare one
durable never-claimed queue for lifetime testing. With a direct UI child helper
blocked before reading stdin (no native intent), pin main/UI/helper/Chrome roots,
terminate only actual main, and prove all exit plus outer-tree release before
fallback. This separately measures helper startup ownership, not another physical
activation. No HTTP pause/activation endpoint is added. An orchestration cue only
starts the operator waiting deadline; it never grants broker authority, resets
native intent expiry, or delays a physical click that already arrived.

### DEV-3 physical evidence and corrections (2026-10-14)

The initial cold physical trial failed before the native helper wrote its broker
request: the staged `aku-sidecar-dev.exe` name did not satisfy the unchanged
client's Sidecar image allowlist. The concurrent API then lacked its correlated
broker attachment. The original generic transport error does not establish the
exact timeout subtype. The broker fixture now stages actual main as
`akusidecar.exe` beside its helper, keeping production client checks unchanged.
A non-foreground test validates that filename/directory contract; another proves
that helper rejection cancels pending API work immediately, with safe error enums.

The corrected physical trial passed its first native open: actual main PID 37432,
owned UI PID 35020, reader PID 30672/HWND 2819820, exact foreground readback,
completed actual API, durable audit, capture/reasoning drain, bootstrap reuse and
one synthetic login. No warm retry or simulated input established that result.
It then failed its final-close API assertion because the fixture requested
`/api/health`, whose contract excludes `collectionRuntime`; the corresponding
status endpoint is `/api/collection/runtime`. This is a fixture readback defect,
not proof that the complete resume/lifetime acceptance passed. Both physical
transcripts remain separate; scoped post-run process census found zero peers.

The runtime endpoint correction passed a real `httpapi.Server` contract test,
including rejection of missing/null/paused/pending runtime bodies. A fresh full
physical trial then reached READY/ARMED (main PID 30684, UI PID 39596, HWND
983652) and received its operator cue, but no physical Open receipt arrived in
the 120-second operator window. That timeout establishes neither a regression in
the prior positive opening nor full acceptance of the corrected resume/lifetime
path. Scoped cleanup census again found zero peers. User observation is required
before another physical trial; do not manufacture input or repeat automatically.

### DEV-3 synthetic native composition accepted (2026-10-14)

The operator confirmed that the preceding timeout trial's window was visible
but its button had not been clicked, then explicitly stated readiness to repeat.
The next fresh-profile trial passed in 6.01 seconds, with helper exit 0. An early
physical click was accepted immediately without requiring the operator cue to
arrive first. Actual main PID 36464 owned UI PID 37308/HWND 3016214; the first
request produced reader PID 39428/HWND 1115512 with exact foreground readback,
actual API completion, durable action audit, capture/reasoning drain, bootstrap
reuse and one synthetic login. No warm retry established this result.

The same trial proved that an independent reader window kept ownership/pause
after bootstrap closure, final-window natural exit allowed verified continuation
on the same account/profile (two captures, one login), and actual
`/api/collection/runtime` reported ready headless state. Changed-foreground helper
replay and duplicate/stale-epoch/cancelled API admission were denied.

The post-positive lifetime stage pinned actual UI/helper/Chrome roots, then
terminated only actual main. All roots signalled, the UI HWND vanished and the
outer Job tree was empty before fallback. The held helper was a direct UI child,
blocked before stdin read, with no native intent. Reopened SQLite retained the
durable pause and rejected claim of the never-started command. Independent
post-run census found zero scoped QA processes and zero disposable `run-*`
directories. Raw evidence is
`runtime/qa/auth-session-sidecar/dev3-physical-operator-retry-validation.txt`.

This completes the selected synthetic Win32 development UI/actual main/API/
unchanged native-helper composition gate. Ordinary web frontend, installed
extension/native-host registration, real-source authentication, media recapture
and other production readiness gates remain unverified. No user runtime,
original authentication, installation or registration was touched. At this
acceptance checkpoint, the 23 DEV-3 source/test/helper/document files were
uncommitted pending new authorization.

### DEV-3 publication checkpoint authorized (2026-10-14)

The owner subsequently authorized review and commit/push of those 23 files.
Scoped review found no concrete blocker. The passing physical retry and strict
actual-server preparation evidence are reused because the implementation inputs
are unchanged; JavaScript syntax and patch whitespace checks also passed. The
ordinary-build regression/build evidence remains applicable. This checkpoint
does not activate the feature, register a native host or touch user runtime/auth.
The next proposed gate is ordinary web UI/bridge acceptance in an isolated QA
composition with fresh profiles and synthetic login. Its design must preserve
private transport and distinguish any remaining installed-host coverage gap.

## Primary references

- [Windows nested Jobs and inherited membership](https://learn.microsoft.com/en-us/windows/win32/procthread/nested-jobs)
- [Windows Job lifetime and kill on last handle close](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects)
- [Windows process creation flags](https://learn.microsoft.com/en-us/windows/win32/procthread/process-creation-flags)
- [Windows startup thread enumeration](https://learn.microsoft.com/en-us/windows/win32/api/tlhelp32/nf-tlhelp32-thread32first)
- [Windows ResumeThread suspend-count contract](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-resumethread)
- [Playwright persistent contexts and profile exclusivity](https://playwright.dev/docs/api/class-browsertype#browser-type-launch-persistent-context)
- [CDP Network cookie contracts](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Network.pdl)
- [CDP Storage](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Storage.pdl)
- [CDP context isolation](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Target.pdl)
- [Playwright auth reuse](https://playwright.dev/docs/auth)
- [Chrome App-Bound Encryption](https://security.googleblog.com/2024/07/improving-security-of-chrome-cookies-on.html)
- [Device-bound session credentials](https://blog.google/security/protecting-cookies-with-device-bound-session-credentials/)
- [Chrome remote debugging restrictions](https://developer.chrome.com/blog/remote-debugging-port)
