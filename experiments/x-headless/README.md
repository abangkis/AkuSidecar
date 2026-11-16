# X and Facebook headless capture PoC

Independent experiment package inside AkuSidecar. It never imports results into
Sidecar, loads AkuBridge's service worker, or changes application settings.
The default mode uses an isolated profile. The explicitly selected shared mode
uses the registered capture profile after a controlled shutdown. No production
source files are edited and the daily Chrome profile is never selected.

## Dependencies

- Node.js 22+ (tested with Node 24); no npm dependencies or installation.
- Default isolated mode: existing workspace Chrome for Testing at
  `../../runtime/chromium/bin/chrome.exe`. Its SHA-256 must match the
  adjacent `pin.json` before launch. This PoC currently targets Windows.
- The existing `AkuBridge/source-adapter-runtime.js` and the selected
  `adapters/x-adapter.js` or `adapters/facebook-adapter.js`
  are read and injected into the page without modifying them. Source hashes and
  the browser identity are included in each result for reproducibility.

Shared mode uses the same browser executable and `--browser-profile` configured
for `akusidecar` in the user Supervisor registration. It reads only the last-used
profile directory from `Local State` and gives that profile directly to Chrome.
Chrome handles session storage itself; the PoC never reads/exports cookie values.

Chrome uses `--headless` and a private CDP pipe. No debugging TCP port, external
service, stealth patch, disabled TLS verification, or disabled browser sandbox.
The PoC does not establish that the complete AkuBridge extension works headless.

## Facebook

The same experimental package supports `--source facebook`, defaulting to
`https://www.facebook.com/`. It uses the existing Facebook discovery/admission,
native permalink, author, text, relative timestamp, media-root and scroll policies.
An X quote-navigation resolver is never used for Facebook. No result is ingested
into production Sidecar.

```powershell
node facebook-smoke.mjs
node shared-profile-test.mjs --source facebook --max-posts 20 --max-scrolls 12 --wait-ms 60000
node facebook-verify.mjs
```

The shared-profile command briefly closes registered Sidecar/UI/capture and
restores it in `finally`; it refuses to stop an active Inbox session. It uses the
existing Facebook session directly, with no cookie copy or automatic login.
Facebook timestamps may remain unknown; relative estimates are labelled.
Shared-post bodies are not separated, video is poster/metadata evidence only,
and structured Facebook media/permalink resolution is outside this first PoC.
See `FACEBOOK_VALIDATION.md` for measured results and gaps. X-only audit/video
helpers filter out Facebook runs; the Facebook verifier reads its own source.
See `FACEBOOK_RECOVERY_VALIDATION.md` for subsequent bounded hover/permalink and
timestamp evidence recovery. `node facebook-recovery-verify.mjs` checks the saved
paired recovery suite.

The bounded comparison harness runs the checkout's unmodified production
content-script/policies and structured-video resolver in a separate isolated CDP
world, alongside the PoC in the same owned headless page. This tests extraction;
it is **not** an installed extension or headed-versus-headless runtime comparison.

```powershell
node facebook-comparison-smoke.mjs
node shared-profile-test.mjs --source facebook --facebook-compare --wait-ms 30000 --max-scrolls 0
node facebook-comparison-verify.mjs <capture.json> <shared-profile-receipt.json>
```

Timeline targets are selected before shutdown; Inbox is checked again immediately
before stopping. Cases cover saved native long-text, multi-image, video candidates
and shared posts when available. Missing categories remain untested. Acceptance
checks author/text binding against saved evidence, not ID alone; same-ID records
with different content are explicitly ambiguous. See
`FACEBOOK_COMPARISON_VALIDATION.md` for the measured failure and fixture results.
The subsequent PoC ownership/fragment fix and same-target rerun are documented in
`FACEBOOK_BOUNDARY_VALIDATION.md`. To preserve target identity across reruns, add
`--comparison-baseline <prior-comparison-capture.json>` to the live comparison
command; the report must already exist under this package's ignored `runs/`.

## Commands (PowerShell)

Run from `C:\WorkspaceCodex\AkuWorkspace\AkuSidecar\experiments\x-headless`:

### Recommended: existing signed-in capture profile

```powershell
node shared-profile-test.mjs --max-posts 10 --max-scrolls 3
```

This is an operator command that temporarily stops **registered akusidecar**
through the existing authenticated Supervisor CLI. The UI and capture close.
It requires a healthy running service, exactly one matching capture-profile
owner, and terminal Inbox sessions before stopping anything. It verifies the
profile has no owner before headless launch, then restores the original service
in `finally` and verifies service health plus Bridge compatibility. It writes
a private `runs/shared-profile-*.json` lifecycle receipt. No registration,
configuration, cookies, or profile directory is copied or deleted. Run this only
when temporary shutdown is intended; closing the shell forcibly can prevent
`finally` from restoring the service. Keep the command running until restoration
is reported.

The registered profile must be inside AkuSidecar's runtime directory; the UI's
`-ui-split-cft` profile is rejected. It does not operate on unrelated browsers.
Direct `node capture.mjs --use-capture-profile` is available only if the capture
browser was already stopped; it refuses a live owner and does not restore the
service itself. The default/manual-login commands below remain isolated.

### Isolated fixture or optional new-profile experiment

```powershell
node smoke.mjs
node capture.mjs --help
node capture.mjs --max-posts 20 --max-scrolls 6
```

Capture defaults to `https://x.com/home`. To read one public post instead:

```powershell
node capture.mjs --url https://x.com/codr_1/status/2105265191986598158 --max-posts 5 --max-scrolls 0
```

### Manual login, only when you choose to open a visible browser

```powershell
node capture.mjs --login
```

This command opens a **visible** Chrome window with the PoC's own `profile/`.
Log in yourself, then close **all** PoC browser windows before running capture.
Existing passwords/cookies are not copied, inspected, exported, or printed.
Subsequent headless captures reuse this same PoC profile; login is not required
on every run. Existing AkuBrowser login does not automatically sign in a new
isolated PoC profile. Login/challenges must be completed manually; the collector
does not click consent, CAPTCHA, like/repost/follow, or other action controls.

Do not point this experiment at a running profile. A PoC-specific lock
rejects simultaneous runs. If the process is forcibly terminated and leaves
`profile/poc-owner.lock`, first verify its recorded PID and that all PoC Chrome
processes are stopped before removing that one stale lock. Never remove Chrome's
own profile locks. Normal completion closes only the Chrome child it owns.

## Results and bounds

Each run writes `runs/<timestamp>-<pid>/capture.json`; stdout prints only status,
post count, duration, and result path. `--screenshot` optionally saves `page.png`.
Both may contain private feed content. `profile/` and `runs/` are ignored and
remain local. Do not commit or share them. This package's `.gitignore` excludes
these private artifacts from the AkuSidecar repository.

The JSON contains visible DOM posts with stable ID/permalink, author, text,
timestamp, relationship/engagement, and observed image/video-poster URLs. Posts
are deduplicated across scrolls. Readiness is bounded to 30 seconds by default,
scrolling to 6 steps, and output to 20 posts. Numeric limits have hard bounds.
Three rounds without new identities stop scrolling. Media URLs are observations,
not downloaded files. Missing fields are null/empty observations, not evidence
that no media or engagement exists.

Current capture uses a 1280×900 viewport and a bounded 750-ms passive hydration
observation per sampling round. Image records include natural dimensions and
decode readiness; video-poster records include video dimensions, readyState,
paused/currentTime, and blob/direct source classification. `loaded` on a video
record means video metadata readiness, not poster decode or complete download.
CDP stores bounded media response metadata (URL/status/MIME/encoded bytes) without
headers, cookies, or response bodies. Network URLs can be sensitive; keep runs local.
`node audit.mjs 4` writes and prints aggregates only; `node video-test.mjs`
selects a previously observed video post and invokes the shared-profile wrapper.

Statuses:

- `captured_partial`: at least one identified post; never a complete-feed claim.
- `login_required`: the page exposes the adapter's login signals.
- `challenge_required`: a detected challenge; no automatic bypass.
- `empty_unverified`: no accepted post; not a verified empty account/feed.
- `error`: browser, navigation, or extraction failed; inspect the JSON error.

Only `captured_partial` exits with code 0; other capture outcomes exit with code 2.
Argument/startup errors outside capture exit with code 1. Reaching the post limit
does not prove freshness, coverage, production admission, or media completeness.
Text may be collapsed; quoted text/media are exported separately when observed.
Quote permalinks may remain unknown, videos only have observed posters,
and no structured response/media resolver is loaded. Challenge detection
is bounded/heuristic. There is no automatic foreground fallback.

## Validation

`node smoke.mjs` launches a real headless browser against a loopback fixture and
uses the real adapter. It checks identity/text/timestamp, quote separation,
engagement, rejected records, viewport bounds, and argument bounds. It does not
prove logged-in X capture or zero OS focus events. The shared-profile live gate
is recorded in `VALIDATION.md`. Compare several bounded runs with the current
collector before claiming freshness, media coverage, timing, or UI parity.
See `FEED_MEDIA_VALIDATION.md` for the three-feed-batch/video suite and its gaps.
See `TEXT_VALIDATION.md` and `QUOTE_MEDIA_VALIDATION.md` for later text/quote work.
See `QUOTE_IDENTITY_VALIDATION.md` for bounded identity recovery and detail navigation.
