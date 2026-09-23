# Experimental Windows capture containment

Enabled only with the existing Windows app-shell capture-split flag. The
separate capture process owns a Job Object; ordinary app-shell launches and
macOS/Linux do not start this monitor. The monitor stops before that job is
released. No release schema or profile migration is involved.

Background containment observes top-level window show/reorder/location events
from the capture root, with a 250 ms fallback scan for missed events or owned
child processes. It lowers only visible capture-job windows above the sampled
external foreground (or an explicitly registered reader). Immediately before
each write it rechecks the foreground, Job Object ownership and reader exemption.
`SetWindowPos(HWND_BOTTOM)` removes accidental topmost status and uses
`NOACTIVATE`, `NOOWNERZORDER`, `NOMOVE`, `NOSIZE`, `NOSENDCHANGING`, and
`ASYNCWINDOWPOS`. It never activates, restores or reorders an external HWND.
Unknown ownership, an absent foreground anchor, or a changed anchor causes no
write. It never calls `SetForegroundWindow` in the background path.

Native callbacks enqueue one coalesced wakeup. Enumeration caches ownership per
PID within each pass. No capture command, screenshot, readiness or lease waits
on this monitor, and asynchronous native positioning avoids waiting on the
Chromium UI thread. Windows may paint a window before the show event is delivered;
this is prompt corrective containment, not a guarantee of zero transient paint.
Performance parity and actual desktop visibility still require forward live
samples after a separately authorized restart.

## Explicit Open native post

Only a claimed, authenticated `open_native_post` action receives the reader
handshake. A cold reader window is created directly on the local marker page;
after native binding, that same tab navigates to the post. A reused reader uses
a temporary marker tab and removes it after binding, including on failure. The
marker title carries the opaque current action ID; the native side matches one
exact capture-job HWND, rejecting no match or ambiguity. A native property
marks the reader exemption and prevents a recycled HWND from inheriting it.

Both new and reused readers are restored to normal state before focus. A new
split reader is initially created without focus until the exemption exists.
The isolated UI loads only `ui-reader-broker`, an extension with loopback host
access and native messaging, without cookies or source-host permissions. A
capture-phase, trusted primary click on `data-aku-native-post` assigns a random
request ID and launches one short-lived `aku-reader-broker.exe`. The ordinary UI
bubble handler copies and clears that ID; the helper and page action must match
the same ID, source and URL. The capture action cannot be claimed until that
helper has been authenticated and attached to it.

Authentication uses `GetNamedPipeClientProcessId`, exact helper executable path,
direct parent equal to the current UI Chromium root, inherited UI Job Object
membership and process age no greater than five seconds. No caller-supplied PID,
shared bootstrap secret, capture token or HTTP route can obtain activation.
The helper independently checks its native caller origin, initial foreground UI
parent and named-pipe server image. Native host registration permits only broker
extension `dlibmmlopdahibfniinemhnghlifiple`, never AkuBridge's extension IDs.

After preparation and Chrome's explicit focus request, the foreground endpoint
waits for the helper. An opaque single-use ticket is issued and consumed on its
OS-authenticated pipe connection before any HWND is disclosed. Its target carries
the exact action, HWND, PID, native property/value and a five-second binding
expiry. The helper checks that binding immediately before activation and readback.
Only this separate executable calls `ShowWindowAsync(SW_RESTORE)` and
`SetForegroundWindow`; Sidecar only verifies the actual result independently.
If Chrome already foregrounded the exact reader, the helper records readback
without another activation. If foreground changed to another application, it
fails closed. No `AttachThreadInput`, injected input, topmost workaround or
process-wide foreground grant is used.

Helper lifetime, pipe operations and UI action are bounded to five seconds;
native activation readback is bounded to 500 ms. Windows rejection or missing
registration is surfaced to the original UI request, with no fallback window,
automatic replay or profile copying. Background actions have no broker entry.
The reader exemption is removed when that one foreground attempt succeeds or
fails. While the reader remains the actual foreground, containment naturally
performs no write; after the user switches away, later background batches treat
the same HWND like every other capture-owned window. A later explicit open must
bind it again with a fresh action ID.

## Bounded evidence

`capture_zorder` emits aggregate counters at most once every five seconds while
there is activity, plus a final flush: `foreground_cycles`, `attempted`, `applied`, `readback`, `failed`,
and one fixed `last_failure` code. `applied` means Windows accepted asynchronous
positioning; only `readback` proves a subsequent below-anchor observation.
`foreground_cycles` counts sampled owned non-reader capture foreground states;
it does not repair them or identify the activating caller.
Unverified readback becomes a failure after one second; tracking is capped at
128 HWNDs and reader exemptions at 32. No titles, URLs, post text, credentials or
external application identity enter these records. Hook failure is explicitly
logged with the fallback interval. `reader_broker` records native-call acceptance,
helper readback and independent verification; `reader_foreground` is readback-only. The
`split_reader` phase records identify whether an explicit request reached
preparation, foregrounding and final result, without recording the post URL.

Schema 29 also keeps at most 512 `split_action_audit` transitions for explicit
`open_source` and `open_native_post` actions. Each row has only an opaque action
ID, action type, phase, accepted/rejected/pending outcome, and UTC time. URL,
title, post content, credentials, and arbitrary error messages are excluded.
These rows can be correlated with native foreground samples; they do not by
themselves prove why Chromium activated or that containment succeeded.

Tests cover ownership/anchor/reader rejection before a write, valid and invalid
readback, platform gating, authenticated claimed-action fencing and replay,
new/reused/minimized reader ordering, marker cleanup and surfaced rejection.

## Packaging and development

Windows split mode uses the pinned **Chrome for Testing only for the UI**.
Branded Chrome removed `--load-extension` in Chrome 137; CfT retains it
([Chromium announcement](https://groups.google.com/a/chromium.org/g/chromium-extensions/c/1-g8EFx2BBY)).
`--chromium-path` and the original authenticated profile still select the capture
process. The UI gets a separate `-ui-split-cft` profile, leaving the former
`-ui-split` directory intact and avoiding a branded-Chrome-to-CfT downgrade.
No cookies, source profiles or login data are copied.

`--ui-chromium-path` is accepted only in Windows experimental split mode.
Installed builds default to sibling `chromium/bin/chrome.exe`; development
restart configuration explicitly selects `runtime/chromium/bin/chrome.exe`
without changing capture arguments. Startup fails closed unless `pin.json`
identifies the stable win64 official CfT artifact and its executable hash,
PE version and `Google Chrome for Testing` product name all match. Packaging
checks the same product identity. There is no system-browser UI fallback.

The UI adapter probes the isolated broker content script for up to five seconds.
Native-post requests are rejected visibly before transport until readiness is
observed and a trusted-click correlation ID exists. Readiness is availability
evidence only; native authorization still requires every OS/action check below.

The same Windows-only UI broker watches the acknowledgement-only startup URL.
If the application does not reach its `ready` stage within eight seconds, the
broker navigates the existing UI tab to that exact loopback URL once. This is an
in-place equivalent of the manually verified Ctrl+R recovery: it creates no
second window, does not activate a background window, never touches the signed-in
capture profile, and cannot loop after a failed retry.

Windows packages include `aku-reader-broker.exe`, `ui-reader-broker/`, and
`com.akubrowser.reader_activation.json` alongside the Sidecar executable. The
manifest uses a relative helper path and an exact broker-only allowed origin.
The installer registers its manifest under HKCU Chromium/Chrome native messaging
keys. Chromium must launch the `.exe` directly; an enterprise policy forcing a
`cmd.exe` intermediary fails the exact-parent check rather than weakening it.

`scripts/build-dev.ps1` builds/stages these artifacts and prepares the manifest;
it does not change the registry. `scripts/restart-dev.ps1` registers that staged
development host before promoting the candidate. Both vendor keys are preflighted
before any write. Another runtime's existing registration blocks the restart
unless an intentional switch is explicitly requested with
`-ReplaceReaderBrokerRegistration`; the lower-level registration script exposes
the equivalent `-ReplaceExistingRegistration` switch. No profile files are
read, copied, or changed by either registration flow.

Remaining live gates: freshly launched UI helper parent/job proof, direct native
host launch on the bundled Chromium, cold and reused/minimized reader activation,
foreground-switch cancellation, and a subsequent background batch remaining
non-activating. Static/unit checks do not establish OS foreground-policy success.
