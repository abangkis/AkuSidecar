# Local one-login concurrency fixture

This opt-in experiment launches two **new, isolated** Chrome profiles: a minimized
headed owner/reader and a true-headless collector. A fresh loopback HTTP server
provides invented accounts and credentials. It never reads production config,
connects to production CDP, reads a social cookie store, calls Sidecar APIs, or
loads extensions. The broker rejects non-loopback origins. No production module
imports this experiment.

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
