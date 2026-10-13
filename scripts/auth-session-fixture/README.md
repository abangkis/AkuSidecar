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
