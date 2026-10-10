# Headless idle resource release

The owned headless worker releases its Chrome instance after two minutes with
no queued/active RPC work and no collection lease. The Node worker remains
available. The next capture lazily starts Chrome with the same validated
executable, user-data directory and subprofile; cookies and authentication stay
on disk. This saves idle browser/renderer memory without changing collection
cadence or the user's selected mode. Resuming a cold browser adds startup cost.

Collection sessions hold the browser across their entire lifetime, including
quiet gaps between acquisition rounds. The capture manager synchronously sends
`setIdleHold: true` before admitting the first lease and `false` after the last
lease releases. Admission and these acknowledgements share the manager mutex,
so an idle timer cannot race an already admitted session. Missing or mismatched
acknowledgements block admission until verified runtime recovery.

The worker serializes idle closure, requests, shutdown and interrupt cleanup.
Requests already queued or active prevent idle closure. Only fully idle source
pages/frontiers are discarded; an active session keeps its continuation state.
Chrome process exit must be observed before idle closure succeeds. Unverified
cleanup blocks another launch for the profile. Each new launch also checks
existing profile ownership.

This applies only to owned `headless_worker` Chrome. Quiet/borrowed Chrome, the
long-lived AkuBrowser UI, login windows and native-post readers receive no idle
close operation. The explicit reader close-and-resume behavior stays separate.
There is no automatic worker retirement, profile deletion or acceptance archive.

Owned worker initialization advertises `idleReleaseVersion: 1`. The Go launcher
requires this capability before reporting readiness; an outdated bundled worker
is rejected with a rebuild instruction. The shared JSONL protocol remains 1 and
borrowed/Quiet initialization is unchanged.

## Verification

Worker lifecycle regressions use injected timers/browser/capture implementations.
The capture manager regressions cover overlapping leases, acknowledgement
failure and exclusion of browser owners. The opt-in machine test exercises the
actual Chrome pipe transport against a loopback fixture with a new disposable
`build/headless-idle-smoke-*` profile. It verifies active-batch frontier retention,
idle Chrome exit, lazy resume on the same profile and a persistent HttpOnly cookie.
It does not access a user account or prove live social-source capture/playback.

From AkuSidecar, using the packaged Node runtime and an installed Chrome:

```powershell
$env:AKU_TEST_CHROME = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
runtime\dev\headless-worker\node.exe --test internal\collection\headless\worker\test\headless-idle-machine.test.mjs
```

The machine test does not use the installed user profile, restart services or
close existing windows. Activation in the development service requires the
normal separately requested rebuild/restart; editing source alone does not
update a running packaged worker.
