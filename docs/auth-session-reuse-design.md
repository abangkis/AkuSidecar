# One-login collector and native reader proposal

Status: Astra Medium proposal, 2026-10-04. Production integration is not implemented.
The owner authorized a local-fixture PoC on 2026-10-13; real-authentication testing
requires separate confirmation. See the bounded experiment below.
Owner prioritizes avoiding a second login. Keep optimized shared-profile handoff
as the working baseline and Facebook's existing Browser collector exception.

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

## Primary references

- [Playwright persistent contexts and profile exclusivity](https://playwright.dev/docs/api/class-browsertype#browser-type-launch-persistent-context)
- [CDP Network cookie contracts](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Network.pdl)
- [CDP Storage](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Storage.pdl)
- [CDP context isolation](https://raw.githubusercontent.com/ChromeDevTools/devtools-protocol/master/pdl/domains/Target.pdl)
- [Playwright auth reuse](https://playwright.dev/docs/auth)
- [Chrome App-Bound Encryption](https://security.googleblog.com/2024/07/improving-security-of-chrome-cookies-on.html)
- [Device-bound session credentials](https://blog.google/security/protecting-cookies-with-device-bound-session-credentials/)
- [Chrome remote debugging restrictions](https://developer.chrome.com/blog/remote-debugging-port)
