# Passive Windows capture trace

The Sidecar starts a native trace when an authenticated Bridge claims a
`collect_visible` command with a capture lease. Setup is awaited for at most
250 ms before the command is returned. No runtime activation or foreground
repair is performed. Existing installations require the newly built Sidecar
to be loaded separately; implementation alone does not activate this trace.

The trace uses `SetWinEventHook(EVENT_SYSTEM_FOREGROUND, WINEVENT_OUTOFCONTEXT)`
on its own message-loop thread and samples changed window state every 100 ms.
Only one run is watched at a time. The next run supersedes it (another claim
in the same active run retains the original bounded trace and command ID); a
matching `released` receipt, server shutdown, 120 seconds, or 256 records ends
it. The record budget reserves a terminal record. Writes are buffered, have
a one-second database deadline, and failures do not fail capture. Unsupported
platforms emit `unsupported_platform`; hook failure explicitly reports polling
fallback. Duration and record limits are explicit terminal statuses.

Readback uses the existing Inbox API (`GET /api/inbox?limit=1`, with the
normal application authentication): each run's `captureSurface` array contains
`event: native_trace`, `detail.schema: windows-capture-native-v1`, and
`detail.native`. The same rows are in SQLite `capture_surface_events`; no new
endpoint is needed. Schema 26 atomically extends the capture-event CHECK
constraint through the registered 25-to-26 migration, preserving existing rows,
indexes, and triggers. No existing user database is migrated until the new
runtime is deliberately loaded. Use the relevant session/run rather
than sharing the complete Inbox response, which includes ordinary user content.

Sort native and existing Bridge lifecycle records by `occurredAt`. Their common
session/run/source and command ID correlate the native samples with created,
reused, focus_intervention (including containment/minimize detail),
release_requested, and released receipts. Bridge receipts may arrive later
than their own timestamps. `phase` identifies the command's capture/cleanup
window; it is not a fabricated precise native-to-JavaScript call attribution.

Interpretation:

- `foreground_event` records the WinEvent target HWND, PID, root HWND,
  classification, and Windows event tick, plus the current foreground snapshot.
  Queued delivery can make the event target differ from the current foreground.
- `processClass: akubrowser_root` means the exact live app-shell session PID.
  `chrome_other` means a different process whose executable basename is
  chrome.exe; it does not establish that it is the user's normal Chrome profile.
  Other applications are `other`, inaccessible processes `unavailable`, and PID
  zero `unknown`. If `browserRootPID` is zero, AkuBrowser ownership is unknown.
- A foreground event targeting AkuBrowser proves native activation. The
  foreground thread's keyboard-focus HWND is also sampled using
  `GetGUIThreadInfo`; unavailable focus is explicitly flagged. No keystrokes
  are observed. Focus snapshots are current-state evidence, not a history of
  every child-control focus transition.
- If foreground remains the working Chrome HWND, while an AkuBrowser window
  becomes visible, non-minimized, above it, and geometrically overlapping it,
  the evidence supports a Z-order/covering change. `zOrderAvailable` and
  `overlapAvailable` must be true. Bounds overlap does not prove pixel occlusion
  (cloaking, transparency, virtual desktops, and DWM rendering are not measured).
  The same check applies to Codex or any other external application. Readback
  sets `classification: akubrowser_above_external_with_sampled_focus_unchanged`
  when the current and previous recorded foreground/focus handles match and
  the visible, non-minimized AkuBrowser window is above and overlaps it.
  Missing focus/ownership/geometry evidence yields `insufficient_evidence`.
  This classification describes sampled state, not uninterrupted input history.
- On Windows only, each retained top-level window also reports `topmost` from
  `GetWindowLongW(GWL_EXSTYLE) & WS_EX_TOPMOST`, with `topmostAvailable` to
  distinguish a failed read from a known false value. A true value establishes
  membership in the topmost window group at that sample; false still permits
  ordinary Z-order covering. Compare consecutive samples of the same HWND to
  observe a topmost-state change. The existing covering classification remains
  geometric and does not require topmost membership. These booleans participate
  in changed-state detection.
  This is a passive diagnostic, not a Windows workaround. macOS/Linux retain
  the unsupported-platform observer and perform no Windows style queries.
  Neither sampled topmost state nor foreground WinEvents reveal who called
  `SetWindowPos`, its flags, or the Chromium call stack. A short-lived transition
  between polls can be missed; a false value cannot rule such a transition out.
- A Windows `EVENT_OBJECT_REORDER` out-of-context hook supplements polling when
  the browser-root PID is known. Registration restricts events to that PID;
  callbacks additionally require `OBJID_WINDOW`, `CHILDID_SELF`, the same live
  PID, and a top-level root HWND. Only changed snapshots emit `reorder_event`,
  using the existing duration/record limits and event HWND/PID/tick fields.
  A failed hook reports `reorder_hook_unavailable_polling` (or
  `foreground_and_reorder_hooks_unavailable_polling` if both hooks fail).
  An unknown browser PID disables this hook. Reorder delivery is asynchronous:
  a rapid TOPMOST-to-NOTOPMOST pair may finish before the callback samples it.
  Events for a parent desktop or other nonmatching HWND are intentionally
  excluded. Missing reorder events therefore cannot disprove a reorder.
- Window snapshots retain at most 16 browser/foreground windows and scan at
  most 512 top-level windows; the foreground window is retained preferentially.
  `windowsTruncated` makes incomplete coverage explicit. Native HWNDs are not
  Chrome extension window IDs; timestamp correlation does not prove identity.
  Changes between 100 ms polls can be missed; foreground hooks cover native
  activation events while the watcher is active. Check trace start/end before
  drawing conclusions about missing events.

Privacy: native telemetry contains only timestamps, native handles, PIDs,
fixed classifications, visibility/minimized/topmost booleans, order/overlap evidence,
and trace provenance. It never reads window titles, URLs, page text, command
lines, screenshots, or input. Executable paths are read transiently only to
classify a basename and are never retained. Rectangles are used transiently
to compute overlap and are never retained. No focus/window mutation APIs are
called. Native diagnostics inherit existing capture-event retention.
