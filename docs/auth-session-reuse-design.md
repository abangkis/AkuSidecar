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

### Ordinary web frontend/bridge QA continuation authorized (2026-10-14)

After DEV-3 publication at `1812d98`, the owner authorized design and testing of
the ordinary web frontend and bridge in a separate QA instance with new profiles
and synthetic login. This continuation must exercise production web assets and
their real request/result correlation, rather than substituting another Win32
button for the web frontend. The existing native fixture evidence can be reused
only for unchanged native inputs; it cannot substitute for the new web boundary.

Keep the actual server/database, browser profiles, synthetic authority and
disposable output beneath the project-owned QA root, using private IPC and
loopback-only source pages. Sidecar remains the parent lifetime owner. Before
launching the UI extension, prove that native messaging cannot accidentally use
an existing registered host or the production broker pipe. No global host
registration, installation, original authentication or user runtime operation is
authorized. Record real versus substituted transport segments explicitly; if
private Windows native messaging cannot be established within these boundaries,
do not claim full frontend-to-native acceptance. Commit/push requires a new
checkpoint authorization.

### Selected ordinary frontend/relay QA design (2026-10-14)

The approved continuation is proposal step 1: design and test the real frontend
and bridge under isolated QA. The selected composition uses actual dual-tag
main, `httpapi.Server`, Engine, SQLite and synthetic development authority; the
production web assets and injected `split_ui_bridge.js` serve one synthetic
Timeline item seeded through Store. Launch project-pinned Chrome for Testing
152.0.7977.54 through the existing owned Session/Job, with a new UI profile and
exact ephemeral loopback origin. This UI split uses its server relay and UI broker,
not the AkuBridge collector extension.

Windows native-host discovery is registry-based; no reliable profile-local
override was found. Therefore this composition explicitly closes native activation:
the staged QA manifest omits `nativeMessaging` and the staged worker replaces the
native call with an unsupported denial. Preserve trusted-click content-script
identity/source, constrain staged origin, verify no native calls exist in the
payload before launch, and record the worker transport substitution. Do not load
the unchanged native-capable extension into QA and risk invoking an existing host.

The only ordinary frontend behavior adjustment is a closed development-session
guard for reader-priority availability/routing: verified development status,
ready requested/effective headless runtime and a supported source are required.
The development exception stays closed for ordinary/unknown/browser/pending/
unsupported cases; existing ordinary Browser behavior is preserved. Actual server
session/account/owner verification remains authoritative. Non-foreground checks
precede a human click on the real rendered link. Measure actual bootstrap/render,
relay/extension readiness, ping/probe, correlated action/cancel/audit and terminal
unsupported UI result, then verify owned cleanup and isolated data. Private CDP
observation must not simulate a physical click or fabricate frontend results.

For the closed native boundary, a minimal dual-tag-only server registration may
return a typed rejection after the existing bridge/source/URL/epoch validation
and before any reader borrow. Its ordinary-build state is nil; it adds no public
endpoint or positive capability. Assert actual rejection audit and no reader
ownership transition/helper launch. The staged worker denial may be delayed by
1.5 seconds solely to observe the concurrent relay POST; this is explicit QA
orchestration, not evidence of a native ticket lifetime or native transport.

The first owned headless preflight confirmed actual app/Timeline/relay/content
readiness and an enabled link, but its rectangle was below the viewport. Treat
rendered/enabled and in-viewport as separate facts: preparation requires the
rendered real Store item and enabled link, while physical ARMED records viewport
position honestly. The operator may scroll the actual page before one click.
Do not scroll through CDP, rearrange DOM, or call an event handler to fabricate
the user journey. The failed viewport predicate is retained as diagnostic evidence.

This phase does not measure Chrome-launched native helper discovery, OS reader
foreground, capture/reasoning handoff through the frontend, app-mode shell parity
or an installed native host. DEV-3 native proof remains independently reusable;
combining the two does not establish a full frontend-to-native run. Further native
registration/runtime testing requires separate explicit authorization.

### Frontend preparation accepted; physical click pending (2026-10-14)

Canonical `scripts/test-auth-session-sidecar.ps1 -PrepareDevelopmentFrontend`
passed, with raw output at
`runtime/qa/auth-session-sidecar/frontend-preparation-final.txt`. It includes 29
Node checks, three actual-main builds/four CLI early-rejection cases, staged
payload checks, real-server closed admission before borrow, actual relay asset/
bootstrap/typed background action/correlated cancellation, Store Timeline seed,
and ordinary split reader/broker regressions. Headless ping/probe returned typed
skips, not evidence of collector extension or source authentication parity.

The final owned headless browser trial passed in 10.23 seconds: actual main PID
24072, UI PID 36376, pinned CfT 152.0.7977.54, separate fresh profile, real
app/relay/Timeline/content readiness, rendered/enabled link and one synthetic
login. No native helper was staged or sampled, no broker server ran, no native
action/cancel occurred in headless preparation, and graceful main exit left the
outer owned process tree empty before fallback. This is graceful cleanup proof,
not a new abrupt-parent-death or foreground-reader trial.

App-only full-page screenshot
`runtime/qa/auth-session-sidecar/frontend-preparation-viewport.png` was reviewed.
The physical target is **Open source evidence** in the card **QA frontend: native
activation is disabled**, below the onboarding card/initial viewport. The operator
must scroll the actual page and click once. The UI window is distinctly titled
**AkuBrowser QA - Native activation DISABLED**. Physical request correlation,
terminal rejection/cancel and live window readback remain pending; no visible UI
has been launched at this checkpoint. This continuation remains uncommitted.

Independent post-preparation census found zero scoped QA processes. Two retained
pre-Chrome staging run directories were removed only after exact path/non-reparse
and zero-process verification; disposable `run-*` inventory is now zero. Stable
failed/passing transcripts and the reviewed screenshot remain available.

### Full native frontend QA exception authorized (2026-10-14)

The full goal remains open: closed native denial cannot replace frontend-to-
reader foreground/ownership and drain/pause/resume proof. A pinned CfT 152 source
review confirmed registry discovery and direct-EXE feature/policy behavior. See
`auth-session-frontend-qa-acceptance.md` and `auth-session-frontend-native-qa-plan.md`.

The owner explicitly approved one temporary uniquely named QA host in
`HKCU\Software\Google\Chrome\NativeMessagingHosts\com.akubrowser.qa_reader_<32hex>`.
This bounded exception permits neither production host replacement nor any other
registration/install/policy/user-auth/runtime changes. Read-only checks confirmed
the parent exists in both registry views. Complete/review the concrete root-local
manifest/binary-hash/private-pipe receipt before applying only the new QA leaf;
remove it after all owned trees exit only when contents still match the receipt.
The Coordinator owns registry application/cleanup and human operational cues.
Positive mode must use real native messaging with configured QA hostname/origin,
unchanged Chrome-parent/image/Job/age/ticket/HWND/foreground checks, actual server/
Engine authority and a fresh synthetic profile/login. No denial substitution or
policy modification may stand in for full positive acceptance. Existing failed/
passing negative/headless evidence remains separate. No new commit/push is approved.

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

## Full frontend/native QA preparation checkpoint — 2026-10-14

The approved temporary QA native-host mechanism is implemented and its narrow
preparation passed in `frontend-native-preparation-ready.txt` beneath the project
QA directory. Configured original extension source, ordinary-build rejection,
read-only registry ABI/provenance, actual Server metadata, Store seed and
unregistered actual-main rejection are verified. The actual frontend click,
Chrome-launched helper, reader foreground and composed pause/drain/resume remain
pending operator readiness. No QA registry leaf has been applied at this checkpoint.
Use the exact frozen receipt/commands in
`auth-session-frontend-native-qa-plan.md` and the requirement-level evidence in
`auth-session-frontend-qa-acceptance.md`. Native artifacts are retained until the
owned trees stop and the one unchanged QA leaf is cleaned. Original authentication,
user runtime and publication remain outside this continuation.

Latest physical checkpoint (2026-10-14): monitor/desktop-safe no-activation QA
presentation preparation passed. One permitted additional trial passed the real
frontend click and Chrome-launched helper provenance, then failed the combined
durable-pause/collector/driver check before capture drain. Full foreground,
drain/resume, multiwindow and parent-death acceptance remains open. Review the
harness's pre-drain persistence expectation against the binding's immediate
admission fence and post-drain SQLite marker; no further retry was performed.
The owned QA leaf was removed, four-view absence/zero scoped peers verified, and
the three exact trial roots deleted. Stable logs and synthetic failure evidence
remain; original authentication and user runtime were untouched. Current task
budget is disabled by explicit user override; saved default remains 1,000,000.
Historical measured usage is 921,237 at an earlier blocked cutoff; the latest
native goal read returned null, so a native reset/new-phase count is unavailable.

Pause-order correction (2026-10-14): the authorized fixture now distinguishes
immediate admission fencing from SQLite persistence after capture/reasoning
drain. A nonblocking buffered observation at the tagged binding's commit point
records the original headless PID/driver/lease before session-lease release.
The physical frontend trial proved these three phases and actual reader
foreground, terminal app/relay/API completion, durable audit, bootstrap reuse and
one synthetic login. It subsequently failed the combined remaining-window
inventory/count/bootstrap/Coordinator-pause predicate after bootstrap close.
Those individual values were not recorded, so final-close resume and frontend
parent lifetime remain unproven. The exact QA host/tree/profile cleanup passed;
see the acceptance audit and `frontend-native-pause-order-physical-trial.txt`.
No original authentication/user runtime or new publication was changed.

Multiwindow observation (2026-10-14): the owner authorized adding diagnostic
receipts before/after bootstrap close. Sequential snapshots preserve unknown
inventory versus true zero, independent Coordinator/SQLite pause, current/Manager
PID, driver/generation/leases/work and close errors. The controller logs both
receipts before enforcing the existing window predicates. Focused contracts and
ordinary/dual-tag builds pass; no physical QA run or registry operation happened.
The earlier multiwindow failure is not yet isolated by live evidence. Refer to
the acceptance audit for the ready instrumentation and next physical boundary.

The subsequent diagnostic run isolated window closure rather than lost pause:
two windows/bootstrap remained after closing only one grouped page target, while
Coordinator/SQLite pause and reader PID/driver were intact. The fixture now closes
all known targets in the selected window, revalidates window membership, verifies
window disappearance and requires natural root exit for final close. Six Node
tests and actual Chrome restart/multiwindow regression passed. The corrected full
physical frontend/native trial passed, including remaining-window ownership,
final-close one-login verified resume and the existing UI/headless parent-death
SQLite-fence stage. Live-reader parent death is a separate remaining state; do
not describe the post-resume lifetime stage as that proof. Exact temporary-host/
profile/root cleanup passed; original auth/user runtime/publication were unchanged.

## Live-reader parent death: current synthetic evidence — 2026-10-14

The isolated frontend/native scenario `reader_open_parent_death` now reads the
original queued command directly from SQLite: exact row metadata and private
payload hash, with null claim fields, attached to the original ordinal-1 run.
Prepared/unclaimed is distinct from never-started. The reader request lease may
already be zero after activation; known live windows, the browser process owner
and replacement readiness retain the profile. Zero request leases alone are not
permission to replace the owner. These corrections passed focused Store/Engine/
Coordinator contracts. Three synthetic physical attempts reached accepted reader
activation but failed fixture assumptions before abrupt parent-death proof; that
state remains a pending gate. All new roots/QA registration are cleaned. No real
authentication or user runtime was used; ordinary production readiness is not
established. See `auth-session-frontend-qa-acceptance.md` for precise evidence.

## Separate gates verified — 2026-10-14

The owner-approved automatic lifetime gate now passes against actual tagged main,
real scoped UI/reader processes and SQLite, without a gesture, native helper or
registration. After only-main termination, pinned process handles and exact HWNDs
are gone, the outer Job is empty before fallback, and the original command payload
hash/null claim fields/durable pause are retained with one login/capture. The UI
can remain hidden: lifetime reads its exact PID/Job/class/title without presenting
it. Foreground remains separately verified through one real frontend/native click
in `foreground_only`, followed by owned graceful cleanup. The tests passed in
3.29s and 32.42s respectively; neither substitutes for the other's evidence.
Normal multiwindow/final-close verified resume retains its earlier passing proof.
All new profiles/binaries/private cues and the single temporary QA leaf are
cleaned. This establishes synthetic fixture acceptance, not production auth or
installed-runtime readiness. See `auth-session-frontend-qa-acceptance.md`.

## Persistent pause guard across builds — 2026-10-14

The SQLite claim guard now compiles in ordinary, `auth_session_fixture`, and
`auth_session_dev` builds. It honors the existing `fixture_capture_pause:<session>`
key inside the claim transaction, before any command is claimed. Marker presence
blocks every run/collector route in that session regardless of marker value;
query failure also blocks claim. An unmarked session retains normal admission.
The schema and marker format are unchanged. Pause/resume mutators and development
activation remain tagged and default-off; an ordinary build cannot clear the
marker through these APIs. Tagged pause/resume reject unsupported marker values
without replacing or deleting them.

`scripts/test-capture-pause-compatibility.ps1` compiles three Store test binaries
and runs five separate processes against one fresh synthetic database: development
pause, ordinary-build blocked claim, fixture reopen/inspection, fixture resume,
then ordinary-build claim. The harness also runs common guard/error tests in each
build and cleans its owned database/binaries after success. It launches no browser,
native host, or Sidecar runtime and accesses no authentication state.

This downgrade boundary requires a target binary containing the common guard.
Historical binaries without it still ignore these markers and are not safe rollback
targets for a paused database. Store compatibility does not establish normal main
startup/browser ownership or real-account verification. Account verification stays
in the Engine binder before resume; the Store harness supplies synthetic owner
epoch/generation parameters only. Restart continues to close owned reader windows
under the selected Sidecar-parent lifetime; it does not preserve old windows.

Validation passed: two common guard cases in each of the three builds and all
five cross-build stages; existing synthetic Engine/Store/HTTP and reader-broker
contracts, ordinary config/main and capture/reader regressions, and ordinary
Sidecar build via `test-auth-session-sidecar.ps1 -VerifyDefault`. Synthetic loopback
Engine tests required execution outside the socket-restricted sandbox. Successful
and initial failed test roots were cleaned; result logs are retained at
`runtime/qa/auth-session-sidecar/capture-pause-compatibility.txt` and
`runtime/qa/auth-session-sidecar/capture-pause-default-regressions.txt`. No real
authentication or user runtime was accessed, and no commit/push was authorized.

## Versioned account/session verifier: synthetic adapter — 2026-10-14

Persistent pause guard commit `d6d185e` is published on `main`. The next authorized
delta defines the private development verifier contract without enabling a real
authentication adapter. `DevelopmentSessionCapability.ExpectedAccounts` declares
the expected account separately for each allowed source. The binder copies that
map and the source list, rejects missing/extra/blank identities, and preserves
source/account pairs in a private ordered aggregate. Matching an Instagram account
never requires it to have the same identifier as an X account. Bridge permission,
login UI presence and content-author names cannot substitute for this identity.

Each callback receives a `DevelopmentSessionProbeRequest` with version 1, source,
profile identity, authority/browser phase, a new probe ID and request time. Its
evidence must echo that request, declare an explicit authentication state, and
include observation time plus session revision. Only `authenticated` is accepted;
`unknown`, `logged_out`, `challenge`, and `unsupported` remain closed. Zero/future/
pre-request timestamps, expired observations, old probe IDs, wrong phases and
unsupported versions are rejected. The hard maximum evidence age is five seconds,
and the complete multisource result must remain fresh when returned. Browser
evidence additionally matches the current stable Manager snapshot and actual
epoch/generation/PID/driver across the entire source vector. Canceled requests
cannot obtain or accept evidence, even when a callback returns otherwise valid
data. No public HTTP/config option can override these checks.

The synthetic process adapter performs a new authority/browser observation for
each request and binds the response to that request. Legacy boolean evidence maps
only a positive authenticated observation to `authenticated`; an absent/false
observation cannot distinguish logout from unknown and remains `unknown`. Revision
represents synthetic login/account/logout lineage, not a hash of every cookie.
Neither evidence nor raw adapter errors may disclose account/session data through
public status or error strings. A rejected proof keeps the existing failure latch:
correcting evidence followed by Coordinator Retry alone cannot reopen admission.

Authority before launch is the fixture's independent synthetic control authority.
It is not proof that a real browser is currently logged in. The current development
startup requires an already-ready scoped owner and a closed binder before recovery;
handoff retains prelaunch authority and postlaunch browser verification. A future
real adapter still requires a separate design for expected identity checkpoints,
quarantined owner startup and fresh browser proof. This contract authorizes no
credential export, cookie database access, automatic second login or real provider.
Domain/provider remain `synthetic-fixture`, with the existing build tags, explicit
opt-in and unsupported media/non-headless boundaries unchanged.

Validation passed through `test-auth-session-sidecar.ps1 -VerifyDevelopment
-VerifyDefault`: source-scoped identities, immutable expectations, state/version/
phase/nonce/timestamp rejection, final vector freshness, whole-vector owner
consistency, canceled requests, redacted errors and unchanged queued payload/null
claim fields across failed resume/Retry. Actual tagged main passed graceful and
abrupt full process restart with fresh synthetic profiles/login: one login and
two captures per case, old owned tree/windows gone before reopen, same SQLite
IDs/profile, fresh engine epoch, lost-authority pause retained, stale/duplicate
denial and verified resume. The existing synthetic integration/default regressions
and ordinary build also passed. The drain test now waits for resumed reasoning
to become idle before Store cleanup, rather than treating terminal status alone
as a completed teardown. Result log:
`runtime/qa/auth-session-sidecar/session-verifier-contract-validation.txt`.
All new temporary roots were cleaned, no physical click or registration was used,
and original authentication/user runtime remain untouched. Verifier changes are
not yet committed/pushed; only pause guard `d6d185e` was published in this step.

## Account checkpoint and Store fence: synthetic scope — 2026-10-14

Verifier commit `060d431` is published. The next authorized step adds Store-only
checkpoint/fence primitives; it does not reorder initial owner launch or connect
the Engine verifier to Store authorization yet. No browser, login, native host or
user runtime is required by the Store compatibility harness.

The private checkpoint records version 1, checkpoint ID/generation, declared
Store/profile scope, expected accounts for a nonempty supported subset of X,
Instagram and LinkedIn, and accepted session revisions after authorization.
Explicit fixture enrollment creates expected-only metadata; it neither proves
authentication nor learns accounts from a probe. Expected identities are copied,
existing enrollment cannot be overwritten, and later revision/account/scope
disagreement is rejected rather than silently updating expectations. No cookies,
credentials or bearer tokens belong in this record.

Stable meta keys `synthetic_account_checkpoint`,
`synthetic_account_checkpoint_required`, and `synthetic_account_checkpoint_fence`
carry the records; versions live in the values, not renamed keys. Any protected
metadata presence requires closed admission if the required record/checkpoint is
missing, malformed or unsupported. Both command and media claim paths consult the
common guard transactionally in every build before claim mutation. Databases
without these keys retain their existing behavior.

The required-verification marker survives authorization. A fresh durable HoldID
fences startup, and only the Store handle that accepts a matching synthetic
receipt receives a volatile permit. Cold-open, read-only open, another handle,
process restart and ordinary-build downgrade inherit no permit. Reauthorization
advances persisted generation so an older handle cannot reuse its permit even
with identical account/revision/time values. Existing per-session pause still
wins. Protected media and unsupported command collectors/sources remain blocked.
Authorization does not restamp queued command payloads in this Store-only step.
An eligible headless command must already carry the same owner epoch/generation
as the accepted synthetic receipt; an old stamp stays queued until a separate
verified Engine resume step restamps it.
Authorization itself may accept a fresh new owner while a drained, supported
queue still carries the old owner's stamp. Rejecting that receipt solely because
the queue stamp is old would prevent cold restart. It grants no permission to
claim the old-stamped command: the transactional claim gate still enforces exact
owner epoch/generation, and authorization leaves its raw payload unchanged.

The tagged synthetic authorization API validates checkpoint/fence identity,
declared scope, exact account/revision sets, owner receipt fields, observation age
and cancellation before its transaction commits. Invalid receipts, stale HoldID/
generation and database failure preserve the checkpoint/fence and queued payload/
null claim fields. Receipt values here are supplied by a trusted synthetic test
caller; they do not prove actual Chrome ownership, filesystem profile identity or
real account authentication. Wiring verified Engine evidence and the reordered
closed-admission owner startup remains a subsequent authorized step.

Ordinary builds contain the guard but no enrollment/fence/permit-activation API,
and no HTTP route/config setting activates it. Downgrade targets must include
this new guard; historical binaries cannot be assumed to honor protected metadata.

Validation passed on 2026-10-14: the checkpoint helper ran unit contracts in
ordinary/fixture/development builds and seven separate processes against the same
synthetic SQLite database. Cold-open and ordinary downgrade remained closed,
failed authorization left metadata/queue rows unchanged, and a new owner receipt
left old-stamped work queued and unclaimable. The cross-build test drains its
earlier claimed row with a simulated terminal failure; this is not proof of an
actual capture completing. Existing pause compatibility (three builds/five
processes), `test-auth-session-sidecar.ps1 -VerifyDefault`, selected regressions
and the ordinary Sidecar build also passed.
The complete Store test suite additionally passed in all three build variants,
covering the existing media and command paths beyond the selected regressions.
Logs:
`runtime/qa/auth-session-sidecar/account-checkpoint-compatibility.txt`,
`account-checkpoint-pause-regressions.txt`, and
`account-checkpoint-default-regressions.txt` in the same directory. Owned temporary
databases/binaries were cleaned. This 13-file checkpoint delta was committed and
pushed as `8abe4e0`; remote main was verified against the full commit SHA.

## Pre-owner fence and verifier-to-Store integration — 2026-10-14

The authorized continuation is synthetic development dispatch only. It must open
the scoped Store and durably prepare closed admission before starting its Node
bridge or Chrome owner. Fresh fixture enrollment is explicit, expected-only and
single-use. Reopen must validate the persisted expected account map and the same
Store/profile identity; it cannot re-enroll or log in to repair a mismatch.

Once the owned browser is ready, a correlated version 1 authority/browser vector
must precede Store authorization. The current fence, account/revision set, profile
scope and pinned Manager/PID/epoch/generation must agree. Persisted audit values
cannot replace a new observation. Only then may the existing verified resume
path restamp never-claimed headless work. Every reader handoff closes admission,
drains active capture/reasoning and fences the Store before relinquishing owner
leases; returning headless repeats verification and authorization.

Unknown, mismatched, expired or canceled evidence must preserve closed admission
and queued payloads. Ordinary production startup and the fixture-only legacy
launch order remain unchanged; fixture cleanup now joins coordinator polling.
This step has no real source verifier, registry
registration, installation or user runtime authority. Physical foreground/native
click acceptance is separate from the automatic startup/session tests.

Implemented and validated in the synthetic dev composition. Go captures the
checkpoint/fence and Manager before probes, pins the complete observation vector,
and rechecks fence hash/scope/HoldID and owner before authorizing Store. Receipt age
uses the earliest authority/browser observation. Pause-epoch invalidation during
verification prevents permit activation and queued-command restamping. Full
Snapshot equality remains required during probes; after deliberately acquiring
session leases, resume compares the owner identity without treating its own added
lease count as an owner replacement. Coordinator polling exposes completion so
fixture shutdown cancels and drains it before closing SQLite.

The fresh profile directory now exists before Node. Its grant marker is private
Go state, not an accepted controller JSON field. Go validates its physical ID
before and after the private Node scope RPC. Node accepts that prepared profile
only as an empty, unlinked directory under the owned QA root, pins its device/
inode/birth identity and checks it again before Chrome spawn. Populated profiles,
replay, prepared reopen and replaced directories are rejected; ordinary fresh
launches still reject existing profiles. Recovery retains the prior lineage and
old-owner-release checks. This grants no access to an original authentication
profile or ordinary startup.

Validation passed on 2026-10-14: three-build/seven-process Store compatibility,
five Node profile-scope cases, verifier/checkpoint/HTTP/default-off contracts,
default capture/reader regressions and ordinary build. Actual tagged-main graceful
and abrupt restart each passed with a new synthetic profile, one initial login,
two captures, the same SQLite/profile identity, fresh epoch, old owned tree/windows
gone, lost authority held closed and stale/duplicate rejection. Both initial and
reopen receipts assert a fence before owner startup and authorization after proof.
The physical foreground/native-click test is intentionally excluded. A cleanup
regression passed three repetitions after polling shutdown was joined.

Canonical acceptance command:
`scripts/test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault`.
Stable logs in `runtime/qa/auth-session-sidecar`:
`startup-checkpoint-development-validation.txt`,
`startup-checkpoint-store-compatibility.txt`,
`startup-checkpoint-profile-scope.txt` and
`startup-checkpoint-teardown-regression.txt`. All owned disposable roots were
cleaned. Original auth/user runtime were untouched; no registry/installation was
used. Publication of this integration delta is authorized after scoped review. Production source
verifier adapters and account/session acceptance remain a separate authorized gate.

### Startup failure acceptance after the durable fence (2026-10-15)

`TestDevelopmentSessionMainStartupFailures` exercises the actual tagged main
with three fresh profiles and loopback synthetic login: parent death while the
first authority request is pending, a stalled authority request canceled by the
binding's two-second deadline, and authority reporting a different account.
Faults are injected by the test's loopback server; no startup bypass, setting or
production runtime change was added. The existing five-second HTTP timeout is
not the effective deadline here: the binding context expires first.

At the pending authority boundary the test reads the durable expected-only
checkpoint and pending fence, pins handles for the owned process tree, and seeds
a queued headless command through Store APIs. Claims are denied before the fault
and on two subsequent Store cold-opens of the same database. After failure the
checkpoint account map and command payload/claim fields are unchanged. Each case
proved one synthetic login, zero captures, 12 owned process handles signalled and
an empty containment Job before cold-open. Parent-death uses the verified fixture
Sidecar PID/image/creation identity; normal verifier failures exit on their own.

Validation passed with `scripts/test-auth-session-sidecar.ps1 -VerifyDevelopment
-VerifyDefault`, including existing actual-main restart acceptance, development
contracts, default capture/reader regressions and ordinary build. Log:
`runtime/qa/auth-session-sidecar/startup-failure-validation.txt`. Owned disposable
roots were cleaned. No physical click, registry, original authentication or user
runtime was involved. These new cases prove Store cold-open after failure, not
successful full Sidecar recovery from this pre-authorization state, a new
cross-build downgrade matrix, or a real account verifier. The next design gate
is source-specific verifier/enrollment, default-off activation and rollback.

## Production contract decision, synthetic policy adapter — 2026-10-15

This section fixes the next production contract; it does not activate a real
source adapter. Executable acceptance remains a tagged synthetic Store adapter.
Ordinary builds read the policy guard but expose no policy mutator or activation
endpoint. The earlier review's revision concern needs clarification: Store already
rejects a revision different from `AcceptedRevisions`. That strict rule remains.

### Identity, proof and enrollment

An enrollment binds one database scope and one owned profile identity to an exact
map of source -> stable account ID. Display names, content authors, cookie
presence, bridge readiness and a previously saved checkpoint are not current
authentication evidence. No passwords, cookies, bearer tokens or raw responses
belong in checkpoint metadata or logs. IDs are private metadata, not public status.

The production controller must first close capture admission, obtain fresh
source-specific browser observations, present the account map for explicit user
confirmation, then compare that confirmation against the same scope and identity
before persisting expected-only enrollment. A canceled/stale confirmation writes
no enrollment. Initial enrollment does not authorize work; another fresh proof is
required. There is no automatic learning, overwrite, or substitution of an account.
Changing account requires a separate confirmed enrollment workflow, with old-account
work retained under its old identity or explicitly canceled; never reassign it.
That workflow and its UI are not implemented in this step.

The adapter receives a versioned, one-use source/phase/probe challenge and must
derive the account from a fresh supported observation in the owned browser. The
trusted controller pins profile, source origin, owner process identity, epoch,
generation and driver across the vector. Only authenticated, complete, current
evidence may produce a private Store receipt; unknown, challenge, logout,
unsupported, canceled, expired, account mismatch or owner change remain closed.
Receipt structs are trusted internal transport, not a public attestation API.
Production must replace synthetic control authority with expected checkpoint
identity plus fresh browser proof; replaying the same observation through two
callbacks is not independent authority. The five-second freshness ceiling and
two-second binding deadline are current limits, not a guarantee all real sources
can meet them; adapter qualification must establish that before activation.

`Revision` represents source-specific authentication/session lineage, not every
cookie update and not an arbitrary new value minted for each probe. Both probe
phases must agree and later authorization must match persisted accepted revisions.
Routine cookie rotation cannot silently be treated as either session change or
proof of continuity. Until an adapter can distinguish those cases, it is unsupported.
Observed logout/account/session change revokes admission; a real session change
requires an explicit recovery design and fresh confirmation. No session revision
reset or automatic relogin is introduced here. Point-in-time verification does not
claim continuous coverage of all workers or concurrent session changes; retain
exclusive handoff until that coverage is independently established.

### Durable policy and guard migration

The three legacy keys and their guarded prefix remain stable. Migration adds
`synthetic_account_checkpoint_policy` under the same prefix, with domain
`aku.account-session-policy`, version 1, provider, checkpoint ID, SHA-256 of the
expected account/scope map, monotonic policy generation and enabled flag. This
hash binds confirmation to the map; it is not authentication or encryption.
Version 1 currently accepts only provider `synthetic-fixture`. Real provider
support requires a separately reviewed adapter and versioned schema support;
changing that string alone is not an activation path.

`ApplySyntheticAccountPolicy` simulates trusted confirmed actions for acceptance.
It takes the exact captured checkpoint and policy generation. Migration inserts
disabled policy and rotates the durable pending hold in one transaction. Enable
requires the matching checkpoint/policy revision and drained supported work; it
rotates the hold but grants no permit. Disable rotates the hold and persists
disabled policy immediately; existing claimed work is left for normal drain.
All successful actions revoke previous receipts and handle permits through the
new hold. A subsequent enable still requires fresh verification. A stale action,
unexpected identity or invalid policy is rejected. Transaction failure rolls back
all durable changes and revokes the caller's volatile permit; failed opt-out must
be reported as failure, not claimed globally complete if other handles exist.

New guards accept three-record legacy synthetic checkpoints for compatibility.
Four-record policy checkpoints additionally require a valid enabled policy,
current owner receipt and handle-local permit. Missing, corrupt, unknown-version,
foreign-provider or extra records fail closed. Policy creation is explicit; no
database is migrated by ordinary open/startup. The synthetic policy method is
not wired into normal Engine startup or production settings in this step.

### Opt-out and rollback

Opt-out preserves the policy, required marker, checkpoint and queue. It stops new
claims, then the controller drains active capture/reasoning and closes only its
owned process tree. Reader lifetime remains bounded by Sidecar. A new owner must
start with admission closed; opt-in never restores a persisted permit.

Rollback is permitted only to a guard-aware build. Pre-policy guard code rejects
the fourth key and cannot issue a new fence; leaving this compatibility record
in place is essential even when disabled. Historical builds before the checkpoint
guard remain unsupported. Deleting the policy/required marker or renaming keys is
not rollback. This is a supported-binary compatibility boundary, not protection
against an administrator editing SQLite or running arbitrary old executables.
Before shipping real-provider metadata, repeat the historical binary matrix with
that exact format and establish an installer/runtime minimum-version gate.

Synthetic acceptance covers confirmed checkpoint matching, default-off migration,
explicit enable plus fresh proof, changed revision rejection, opt-out while work
is queued/claimed, re-enable, stale permit rejection and transactional write
failure. Historical downgrade tests compile the unchanged guard from commit
`9fc79f38921752628e13e23428683199833631cd` with test-only probes added. Current
default/development/fixture and historical default/fixture binaries operate on one
synthetic database. This does not test actual browser identity, human confirmation,
full runtime opt-out/drain, or real-provider rollback acceptance.

Validation passed: `scripts/test-account-session-policy.ps1` (five builds, 26
separate stage processes on the same database), existing
`scripts/test-account-checkpoint-compatibility.ps1` (three builds/seven processes),
full default and development Store suites, and
`scripts/test-auth-session-sidecar.ps1 -VerifyDefault` (synthetic integration,
default regressions and ordinary build). Logs under `runtime/qa/auth-session-sidecar`:
`account-session-policy-validation.txt`, `policy-legacy-compatibility.txt`,
`policy-store-default.txt`, `policy-store-dev.txt`, and
`policy-default-validation.txt`. Disposable roots, including the failed test's
retained source snapshot, were cleaned after test processes exited. No original
authentication or user runtime was accessed. Publication of this 13-file delta
was subsequently authorized; controller integration remains a separate step.

## Synthetic policy controller integration — 2026-10-15

`SyntheticPolicyController` now composes the durable policy with existing Engine
startup verification. It is available only with both `auth_session_dev` and
`auth_session_fixture`, `config.Dev`, and an injected `synthetic-fixture` capability.
No normal main dispatch, HTTP/native IPC route, setting, real source adapter or
foreground UI activates it. This step tests the controller directly with simulated
process ownership and versioned synthetic observations, not a full Chrome/OS run.

Before creating an enrollment owner the controller closes Engine admission and
persists `synthetic_account_checkpoint_enrollment_hold`. This provisional record
contains only a random hold and Store/profile scope, not account IDs. All guarded
builds treat it as incomplete protected metadata and refuse claims, including on
cold-open. Normal startup never erases it. A canceled enrollment leaves the hold
closed rather than reverting to an unprotected database. Recovery from an abandoned
provisional hold is a subsequent explicit workflow; no auto-reset is provided.

Enrollment preview requires fresh authority/browser probes for the exact declared
sources and expected accounts, bound to the stable owner pointer, epoch, PID,
generation, driver and profile. The complete vector must remain fresh. Returned
account maps are copies. A two-minute private confirmation ID is tied to the
observed owner and session revisions; it is consumed by any confirmation attempt,
including cancellation, expiry or a wrong ID. Confirmation performs another fresh
observation and rejects an account, revision, owner, scope or checkpoint change.
The confirmation interval does not extend evidence freshness: it always probes
again instead of accepting a two-minute-old login observation.

Only a matching affirmative confirmation atomically replaces the provisional hold
with expected-only checkpoint/required/pending records and a disabled policy at
generation 1. Invalid, foreign, canceled or stale completion and failed SQLite
writes preserve the hold; there is no transactionally visible unguarded gap.
Enrollment does not authorize work. A separate enable preview/confirmation applies
opt-in using the exact checkpoint and policy revision, then calls the existing
`PrepareDevelopmentSessionCheckpoint` / `StartDevelopmentSession` verifier again.
Thus a successful preview and confirmation cannot substitute for startup proof.
Legacy checkpoints have an explicit confirmed migration action that creates a
disabled policy; enable cannot skip migration.

Persisted opt-in can be resumed by a successor controller without repeating
enrollment or login. It still fences the database before owner startup and requires
fresh Engine account/session/owner proof. Disabled policy cannot resume; explicit
confirmation is needed to enable it again. The controller's lifetime owns polling;
completion/cancellation of a successful confirmation request does not own or stop
the runtime. Failure or cancellation during startup keeps admission closed and
renews the durable fence where possible.

Opt-out first closes local admission and persists disabled policy with a fresh
hold. Existing capture/reasoning drains through the existing pause binding before
coordinator polling is joined and the scoped owner is terminated. Cancellation
or incomplete drain reports a partial shutdown, preserves the disabled policy and
active work, and never force-kills the active owner. Outstanding reader/owner
leases remain held; no reader window is forcibly closed by this path. Re-enable
after shutdown uses a successor controller/owner. Synthetic tests cover the idle
and active headless path; actual reader-window opt-out and OS process cleanup
require separate acceptance.

The new helper is `scripts/test-development-policy-controller.ps1`. It runs direct
controller/Store acceptance plus existing non-OS development regressions with
Chrome/process fixture environment opt-ins cleared. Full OS fixture cases are
skipped deliberately. `test-auth-session-sidecar.ps1 -VerifyDevelopment` also
selects the new controller cases when a separately authorized complete fixture
run is wanted. No physical click or registry is required by the controller helper.

This phase does not implement a human confirmation UI, real account observation,
normal runtime wiring or an installed feature toggle. Physical profile identity,
Windows foreground and real Chrome lock release are not established by simulated
owner tests. The next gate is wiring this controller into a separate synthetic
main composition and proving the same enrollment/opt-out boundaries with a real
fixture-owned Chrome process, before any real authentication integration.

Validation passed with the narrow controller helper: eight controller test
functions (including nine changed/stale/canceled decision cases), four enrollment
Store test functions and existing non-OS development/checkpoint/policy regressions.
Existing checkpoint compatibility passed three builds/seven separate processes,
and `test-auth-session-sidecar.ps1 -VerifyDefault` passed synthetic integration,
default regressions and the ordinary build. Logs are
`runtime/qa/auth-session-sidecar/policy-controller-validation.txt`,
`policy-controller-checkpoint-compatibility.txt`, and
`policy-controller-default-validation.txt`. All owned disposable test roots were
cleaned after test exit; unrelated `experiments/` remains untouched. Publication
of this 12-file delta is authorized after review. No original authentication/user
runtime was used.

## Actual-main policy composition with synthetic Chrome — 2026-10-15

Controller commit `82e40c7` is published. The dual-tag Windows main now has the
explicit provider `--dev --dev-session-reuse=synthetic-policy-fixture`. Ordinary
builds and development builds without the fixture adapter reject this provider;
dispatch still precedes ordinary config, saved Store, real providers and app-shell
startup. The provider requires the existing private test-controller grant with
parent creation/image, owned QA root/profile identity and Job containment checks.
It registers no native host and exposes no public policy endpoint.

Private prepare/run hooks in the common process fixture allow this composition
to construct the controller before Node/Chrome, rather than first authorizing
through the legacy binder. The controller persists its provisional enrollment
hold before an owner-launch frame is accepted. The actual browser adapter checks
the pinned physical profile directory before and after observations, queries the
loopback synthetic authority and current Chrome session through the private Node
bridge, and binds browser evidence to the actual Manager PID/generation/epoch.

Confirmation actions use scoped controller-to-child stdin and private receipt
IDs. They simulate user decisions; no human confirmation UI or physical click is
claimed. The fresh-profile scenario cancels one enrollment, confirms a new one,
rejects a mismatched authority before opt-in, then confirms opt-in with fresh
Engine proof. It holds a real queued capture while opt-out becomes durable, drains
that capture, and checks the bridge-owned Chrome/Node tree is empty. OS process
handles provide evidence beyond Manager status.

This provider intentionally rejects reopen and native-host grants. Existing
reopen grants require retained native-window release proof; this headless scenario
does not weaken that contract or invent such windows. Controller restart using
simulated ownership remains separate evidence. A subsequent design/fixture step
must provide a scoped headless recovery grant and repeat cold resume with actual
Chrome before claiming that this new provider supports process restart.

Acceptance passed with actual main and Chrome: pre-owner Go-only Job plus durable
claim denial; canceled enrollment retains its hold; changed authority is rejected;
confirmed opt-in admits exactly one physical synthetic collect; policy disables
before ingestion/reasoning drain; completed capture and unstarted source IDs are
retained; Chrome/Node handles signal and the outer Job empties after Go shutdown.
There was one synthetic login and one capture. The complete development/default
helper passed; a final development run with the additional unstarted-source check
also passed. Latest detailed log:
`runtime/qa/auth-session-sidecar/policy-main-chrome-validation.txt`.
Owned run/compile artifacts were cleaned after their processes exited. No original
authentication, user runtime, native registration or physical input was used.
At this acceptance checkpoint the main-integration delta was uncommitted.
Publication of the combined integration/restart package was authorized after the
subsequent review described below.

## Policy controller cold restart: synthetic acceptance — 2026-10-18

The authorized recovery step keeps the same fixture-owned SQLite database and
physical Chrome profile across full Sidecar process replacement. Persisted opt-in
is intent, not a reusable capture permit. The successor controller loads existing
enrollment, persists closed admission before launching its owner, and calls
`Resume` with fresh synthetic authority and browser evidence. It does not repeat
enrollment, confirmation or login. A new Engine epoch prevents previous process
receipts from authorizing the successor.

Headless recovery uses a separate private grant. It must not weaken the existing
native-reader reopen requirement for retained-window release proof. The trusted
fixture controller must establish old-tree cleanup before issuing a one-use
recovery grant bound to the original root, profile, database, checkpoint and run
scope. A changed physical object, live old owner, mixed native grant or replay
must be rejected. Revalidation failure keeps admission closed and must not adopt
the changed account, log in again or execute queued capture.

Acceptance must distinguish same path from same physical database/profile, and
old process cleanup from a new Manager's status. Login and capture counters,
pinned process handles, Job inventories, Store readback and pre/post-proof claim
checks provide the required evidence. Ordinary startup, installed settings,
real-provider verification and the user's running development instance remain
outside this fixture step.

Graceful fixture shutdown must stop admission/polling and drain work before using
the existing Chrome `Browser.close` path and waiting for profile release. Killing
the containment Job proves process cleanup, not persistence of newly issued
browser cookies. Cold-restart acceptance must observe the browser's authentication
state directly; it must not export/reinject cookies or perform a second login to
make recovery pass. A missing browser session remains a closed-admission result.

Queued commands remain byte-for-byte unchanged while admission is closed or fresh
proof fails. After successful verification, an unclaimed headless command may
receive the new owner's `captureRuntime.epoch` and `captureRuntime.generation`,
as required by the existing restart contract. Session/run/command identity and
all other payload fields remain unchanged. This permitted rebind is distinct
from changing enrollment, adopting another account/session revision or reusing
an old owner's capture permit.

Acceptance passed on 2026-10-18 through actual tagged main and fixture-owned Chrome.
An enabled idle owner closes gracefully; a successor with unavailable authority
keeps the checkpoint and full queued payload unchanged, then is terminated
abruptly. A further successor reuses the same physical database/profile, performs
fresh verification and completes the same command without re-enrollment or login.
The test proves pre-owner Go-only containment, pre-owner/pre-proof claim denial,
invalid/replayed recovery denial, stable expected accounts/accepted revisions,
fresh owner epoch/generation and empty owned Jobs. Counters are one synthetic
login and one physical capture across the complete scenario.

The targeted test passed, followed by the canonical
`scripts/test-auth-session-sidecar.ps1 -VerifyDevelopment -VerifyDefault` with no
cookie-database observation in the final test source. Existing legacy native
graceful/abrupt restart cases and default config/main, capture/reader/Store/HTTP
regressions also passed; the ordinary build succeeded. Final log:
`runtime/qa/auth-session-sidecar/policy-controller-restart-final-01955b2e025247f2a1f3f2bd635c84e5.txt`.
Synthetic-only failure diagnostics distinguish generic verification failure from
known authority/browser failure and never expose cookie or token values.

This does not establish controller recovery after abrupt death during an
authorized active capture, real-source session persistence or human consent.
The abrupt case here terminates a failed-verification recovery owner. Enrollment,
opt-in and window/reader lifecycle tests from earlier steps retain their own
separate evidence. The main-development executable/provenance and Sidecar and
Supervisor configuration hashes are unchanged. Publication of the combined
11-file integration is authorized after review; normal runtime activation and
real authentication remain excluded.

### Proposed next gate toward a limited main-dev pilot

Implement a source-specific verifier for one pilot source, initially tested only
against synthetic or sanitized offline fixtures. It must distinguish stable
account identity, authenticated state and session continuity; missing, ambiguous
or changed proof keeps admission closed. Define the production metadata/provider
format and guard-aware migration before persisting real account expectations.

Then connect enrollment/confirmation, a default-off development toggle and
guard-aware rollback to the existing controller. Include active authorized-capture
crash recovery in that acceptance scope. Real-account observation and enabling a
pilot on the user's runtime require separate explicit authorization; publication
of this synthetic package grants neither.

## X verifier prototype with offline fixtures — 2026-10-18

This authorized step implements one-source verification logic without observing
an actual account or enabling a production provider. The new X package is isolated
behind both development/fixture build tags and is not wired into Engine, Store,
ordinary main, native IPC, HTTP or saved settings. Its outputs cannot authorize
capture. The published controller integration remains unchanged.

The repository has no qualified native X current-account/session observation
corpus. Therefore `x.browser-self.fixture/v1`, schema
`aku.x.browser-self.fixture.v1`, and path `/__fixture__/x/account-self/v1` are
explicitly invented offline contracts, not discovered X endpoints. Exact
`data.id_str` extraction models a stable string identity; fixture status markers
model login/challenge observations. Neither is a claim about live X browser wire
format. Content authors, public profile lookup, handles and display names cannot
substitute for the observed current account.

The official X SDK documents `/2/users/me` as a user-token API operation, not
evidence of the browser profile's login. That route is deliberately not accepted
as browser-session proof. Reference:
[X SDK requesting-user operation](https://github.com/xdevplatform/twitter-api-java-sdk/blob/main/docs/UsersApi.md#findmyuser).
No API credentials, OAuth setup or authenticated request is added by this step.

Identity, authentication state and continuity are separate outputs. A complete
synthetic lineage witness is supplied through a trusted fixture interface outside
the response JSON; it never comes from an invented native `session_id`, a cookie
hash, a display name or a random revision per probe. Ordinary response-only
observations cannot establish session continuity and must remain unsupported or
unavailable. The same account after logout/login must not be accepted as the same
session; cookie rotation alone proves neither continuity nor a session change.

Fixture acceptance checks challenge consumption, exact source/origin/operation,
freshness and declared fixture profile/owner/epoch/generation; two phases must agree before
synthetic continuity can be classified. Missing, malformed, ambiguous, stale,
changed or incomplete evidence remains closed. A fixture success is not a Store
permit, a qualified native-source verifier or continuous browser-worker coverage.

The accepted synthetic lineage generation is separate from owner generation.
Both request-bound lineage observations must match that accepted generation for
`same`; a different generation is `changed`, even for the same account. Missing
lineage is `unsupported`; revoked or incomplete coverage is `unavailable`. These
labels describe fixture inputs, not independently observed native lifecycle events.
Each verifier has a random challenge namespace, bounded one-use challenges and a
five-second issuance/evidence lifetime. Pair completion rechecks freshness and
clears identity flags on expiry. This offline harness is serial-use only.

Validation: `scripts/test-x-session-verifier.ps1` passed 15 top-level tests and
41 subtests. Coverage includes identity rename, content-author substitution,
malformed/duplicate/oversized bodies, missing identity, login/challenge/unknown,
account changes, replay (including across verifier instances), stale/future proof,
profile/owner mismatch, incomplete coverage, synthetic rotation versus relogin,
and expiry between observation and pair completion. The helper also verified that
both tags are required and the package is absent from ordinary main dependencies.
It disables module downloads, uses project-local temporary/cache directories and
cleans its owned temporary root. Log:
`runtime/qa/auth-session-sidecar/x-offline-verifier-validation.txt`.
Six protected runtime/config/source hashes remained unchanged.

Next gate: qualify a native browser current-account schema and a trustworthy
session-continuity observation contract from authorized, sanitized evidence.
Do not substitute OAuth identity, content authors or response-only identity for
browser-session continuity. Native observation still requires separate permission.
Until that evidence exists, this package remains an offline prototype, excluded
from controller/Store authorization and default-off production activation.
