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
is the observed baseline, not proof of window-preserving restart.
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

This completes the fixture lifecycle test. Preserving reader windows across full
process death remains an architectural gap. The dedicated composition does not
prove production main, installed extension/native-host startup, production DOM
extraction, real authentication or post-ticket-claim races. No user runtime,
authentication, installation, registration, commit or push changed. The next
proposal is review of independent reader-owner lifetime and secure reconnection.
