# Native app-shell startup status

The Windows installed app has a small native status window owned by AkuSidecar.
It appears only for a fresh isolated browser profile or when the installed
source tuple has changed. Development and routine launches with an acknowledged
tuple do not show it. A per-profile marker records the release version and
source-freeze fingerprint only after a valid interface-ready acknowledgement;
it contains no browser session or capability. Missing, damaged, or unreadable
markers leave recovery available. The readiness handshake continues on quiet
launches. The marker is UI state, not proof of visible pixels or installation
integrity, and deleting it causes recovery to reappear on the next launch.

The status does not depend on Chromium loading HTML. After approximately one
second on an eligible launch it shows that the local HTTP service has started
and that interface initialization has not yet been acknowledged. At 60 seconds
without acknowledgement it offers recovery guidance in the same window. Keep
waiting resets that reminder timer;
Copy diagnostics copies fixed startup state and elapsed time. Retry window is
an explicit user action: it closes the existing owned Chromium tree, waits for
root exit and verified zero active processes in its Windows Job Object, then
opens exactly one replacement using the same executable, profile, extension,
and launch settings. AkuSidecar and its database stay running. There is no
automatic retry. With Chromium still running, the window X closes only the
status window. After a failed retry leaves no Chromium, closing the last
recovery window ends the session and shuts down Sidecar normally.

A session owner distinguishes manual replacement from normal Chromium exit;
normal exit still shuts down Sidecar. Duplicate clicks are gated while closing
or launching. Shutdown/normal exit already observed before a queued click wins
over retry. Each replacement gets a fresh capability and readiness state;
old acknowledgements cannot initialize the replacement. Extension-opening
actions are unavailable during replacement. A failed launch keeps native
recovery visible for another explicit attempt. Unverified process cleanup
blocks all replacements; it requires investigation or a full application
restart rather than risking overlapping profile owners. Pending cleanup still
observes the old root's exit: if status was dismissed and the old root later
exits, Sidecar shuts down rather than remaining headless. Closing the status
with X during a requested retry does not cancel that retry or reopen status.

The window is an ordinary minimizable taskbar window, shown with
`SW_SHOWNOACTIVATE`. It is never topmost and never calls `SetForegroundWindow`.
Timeout and acknowledgement update its text without activating or reopening it.
A user can explicitly activate it to use its keyboard-accessible controls.

The existing HTML startup panel and 60-second JavaScript watchdog remain.
After the existing bootstrap/view-selection ready event and two animation
frames, the watchdog sends one same-origin POST to
`/api/app-shell/startup-ready`. A fresh 256-bit capability travels only in the
initial launch URL fragment and the acknowledgement header. The fragment is
removed before app modules run. The server validates the capability, exact
origin/host, POST method, and same-origin fetch metadata; it rejects replay,
old launches, missing capabilities, and unrelated origins. This capability is
independent of privileged runtime control and grants no application-data access.
It is not persisted or included in diagnostics or server request URLs.

Acknowledgement changes the native message to "The interface reports ready";
it does **not** close the native window on that one fresh/install-update launch.
JavaScript and animation frames cannot prove that Chromium's compositor displayed
pixels. The recovery window remains reachable until the user closes it with X or
the shell exits. Subsequent launches of the same acknowledged tuple show no
native status window. A reload after the launch
fragment was removed cannot acknowledge that launch; if the UI becomes visible,
the user can still close the status with X. The window does not determine the cause
of a blank screen or repair it. If Windows cannot create the native window,
Sidecar logs a fixed-context error and continues launching Chromium.

Non-Windows builds retain the acknowledgement contract and HTML watchdog, with
a no-op native status implementation.

## Validation without opening UI

Run the targeted `TestSession*`, `TestWindowCloseForRetry*`, `TestStartup*`, `TestAppShellStartup*`, `TestBuildArgs*`, and
`TestLoopbackBoundary*` Go tests and `node --test test/startup-watchdog.test.mjs`.
The existing `TestTerminate*` tests in the appshell package launch real Windows
processes/windows and must be excluded when UI interaction is not authorized.

Before claiming live UX verification, inspect first and subsequent launches,
missing HTML, missing app module, delayed bootstrap, acknowledged-but-white
rendering, user focus in another application, high DPI, clipboard copy, manual
retry, double clicks, status X, and normal shell exit. Compile and unit tests do not verify native pixel layout
or foreground behavior on the user's desktop.
