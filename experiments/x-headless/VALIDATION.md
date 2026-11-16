# Validation receipt — 2026-10-01, Asia/Jakarta

## Successful existing-profile test, 02:29 WIB

- Same registered capture executable: Google Chrome 154.0.8037.58.
- Same user-data directory: `AkuSidecar/runtime/dev-chrome-profile`; selected
  profile directory `Default`. No cookie copy or login command.
- Preflight found a healthy running service and no nonterminal Inbox sessions.
  Historical `partial` sessions had `completedAt`; they were terminal, not active.
- Supervisor stopped akusidecar. Its owned process list was empty and the
  capture profile had no owner before headless launch.
- X Home returned HTTP 200. The browser user agent contained `HeadlessChrome`.
- The real adapter captured **3 unique posts**, all with text and timestamps;
  **2 posts had observed media URLs**. The signed-in account-switcher UI marker
  was observed in all four sampling rounds.
- Capture took **7.684 seconds**. Status remains `captured_partial`; this is not
  proof of full feed coverage, fresh content, or complete media acquisition.
- Headless exited and released the profile. The original registered service
  was restarted, verified healthy, and Bridge was `healthy`/`compatible=true`.
  The restored capture profile had exactly one owner using the original Chrome.
  Stop/test/restore receipt duration was approximately **11.3 seconds**.
- After restoration, a direct shared-profile capture was rejected with
  `Capture profile is still owned by a running Chrome process. Stop it first.`
  No second browser was launched against the active profile.
- No additional login, cookies export, extension reload, source integration,
  registration/configuration changes, commit, or push. Zero OS focus events were
  not instrumented; restoration of the normal UI is an intentional visible action.

The earlier HTTP failures below used a different browser **and** an empty
isolated profile. This successful run changed both variables, so it does not
identify which variable caused those failures.

Private lifecycle receipt:
`runs/shared-profile-2026-09-30T19-29-19-819Z.json`.
Private captured data:
`runs/2026-09-30T19-29-20-976Z-6576/capture.json`.

## Earlier isolated-profile tests

- Node.js: 24.16.0.
- Browser: pinned Chrome for Testing 152.0.7977.54, checked against pin.json.
- Reused adapter: x-dom-v22.
- Real headless loopback fixture passed: primary identity, text, author,
  timestamp, engagement, quote separation, invalid-record rejection, viewport
  bounds, numeric/URL input bounds, profile lock, and idempotent shutdown.
- The page reports `document.visibilityState = visible` while Chrome runs
  headless. This is document state, not evidence of an OS-visible window.
- Chrome terminated during sandbox startup. The successful fixture was run
  outside the sandbox using the same PoC profile and browser, without disabling
  Chromium's own sandbox or opening a window.
- Live public X post and X Home navigation both returned
  `net::ERR_HTTP_RESPONSE_CODE_FAILURE`, before extraction. No live posts were
  captured. HTTP status/root cause and authenticated-feed behavior are unverified;
  this result does not establish headless detection, login expiry, or a parser bug.
- In these earlier isolated tests, no login command was run and no existing profile/cookies were accessed. No
  application restart, extension reload, Sidecar ingestion, commit, or push.
- This experiment proves headless launch and reused DOM extraction on controlled
  content. It does not yet prove live authenticated X capture, feed freshness,
  full AkuBridge extension compatibility, quality parity, or zero OS focus events.

Capture JSON files are private local run artifacts under ignored `runs/`.
For an independent new-profile experiment, `node capture.mjs --login` opens a
visible PoC browser only when explicitly run by the user. It is not needed for
the successful shared-profile flow above.
