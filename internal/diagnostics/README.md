# AkuBrowser internal diagnostics

This package family separates reusable recording mechanisms from AkuBrowser
contracts, collection authority, transport, and presentation. It is an internal
extraction, not a new published library or a new telemetry backend.

| Package | Responsibility | Reuse boundary |
| --- | --- | --- |
| `recording` | Concurrent progress snapshots, first observed cause, independent cleanup/delivery, explicit completeness | Reusable candidate; caller supplies vocabulary and observations |
| `snapshot` | Single latest value with copy-on-save/copy-on-read and concurrency control | Reusable candidate; the value supplies its deep-clone operation |
| `observability` | AkuSidecar instrumentation vocabulary and adapter over the published telemetry runtime | Product adapter; composition-root lifecycle and explicit export configuration |
| `capturestartup` | First split-owner launch, navigation, bootstrap, heartbeat and two document samples | Development supervisor log using `recording` and AI4UDiagnostics elapsed-time projection |
| `captureactivity` | Chrome process launches, owner transition timing and individual HTTP polls | Local supervisor log using the same recorder and elapsed-time projection |
| `timeline` | Closed browser-performance report schema, numeric/shape validation, cloning, and latest-report composition | AkuBrowser adapter; stage names, bounds, UTC receipt time and privacy policy stay here |
| `instagram` | Existing IG catalogs, attempt ownership, wire/public projection and validation | AkuBrowser adapter; does not grant capture, retry or mutation authority |
| `xreply` | Closed candidate outcomes and exact-observation binding using the AI4UDiagnostics count policy | AkuBrowser adapter; persisted with source observations |
| `linkedinrecovery` | Closed projection of LinkedIn surface inspection using the AI4UDiagnostics count policy | Optional observation coverage; independent of capture admission |
| `nativecapture` | Existing bounded platform-specific native-window observations and classification | AkuBrowser/Windows adapter; process classification and capture lifecycle stay here |

`recording` continues to use the generic values in the pinned AI4UDiagnostics
package. `snapshot` depends only on Go synchronization and its clone contract.
Neither package knows HTTP, browser selectors, native profiles, source identity,
timing acquisition, collector configuration, UI labels, or storage databases.
Independent-consumer tests use unrelated operations/value types rather than
claiming another application has already adopted these candidates.

## Browser boundary

For a development restart, filter the Sidecar stdout log for `capture_startup`.
The host document itself samples at window load and again after a 12-second
timer. Chrome can delay that timer. The first eight served documents receive
distinct ordinals, each with independent early and late receipts. A missing sample means
the document observation is unknown. On a loaded document, `script_started` records the
Bridge entry marker and `worker_receipt` records a best-effort acknowledgment
from the validated CONNECT receiver. A missing acknowledgment alone cannot
prove the worker never received CONNECT. Bootstrap and heartbeat receipts are
recorded separately at Sidecar. Sequence and elapsed time reflect observation
order in this process. A new capture owner retires this startup trace.

Filter Sidecar stdout for `capture_activity` to inspect activity across owner
changes. Each entry carries a JSON record. The `browser_launch` operation
identifies each process by PID, executable basename and version. Each
`capture_transition` observation includes the measured duration and, when
reported by the owner, its success flag. For `capture_poll`, match start and
end by `poll`; the end includes the outcome, queue counts and available action
and broker request IDs. An absent `elapsed_ms` means the duration is unknown.
The worker console uses the same filter and reports `capture_worker` events.
Match a claimed action across the two logs by its Sidecar-generated action ID.

For an admitted `open_native_post`, filter `capture_activity` by
`operation=split_action` and the issued `action`. The `queued` phase records
the admission queue snapshot and starts elapsed time at zero. The matching
`capture_poll` end names the claimed action. In the Bridge worker console the
same ID appears as `actionId` on `capture_worker` / `action_claimed`. A direct
reader can settle without a Bridge claim. Only an observed, well-formed broker
ID appears as `request_id`.

The Sidecar `result` phase records the first committed accepted or rejected
result. A `wait_ended` phase records a timeout, client cancellation or stopped
transport. A claimed action can still produce a later result. Use the local
sequence within each action and `elapsed_ms` since admission. Queue totals are
under `queue` on the admission record. These JSON fields replace the previous
`split_action action=...` text filter for queued and terminal lines. Durable
`split_action_audit` rows keep their existing phases.

For a reader-broker invocation, filter `capture_activity` by
`operation=reader_broker` and `request_id`. The `received` event starts the
local clock. After the broker matches the request ID, source and URL,
`attached` adds the issued `action` and the measured `queue_wait_ms` when
available. A refused duplicate or conflicting registration uses the separate
`reader_broker_admission` operation so the original invocation remains
traceable.

`activation_started` marks the helper call. `helper_reply` reports the
exchange duration, expiry/context observations and allowlisted helper fields.
`activation` records the local verification or manual foreground fallback.
The terminal outcome distinguishes accepted, manual_required, rejected,
cancelled, timed_out and stopped. Missing verification or helper fields mean
the observation is unavailable. The HTTP relay's prepare and foreground
samples use `operation=reader_relay` and the issued action ID.

Native-reader observations use `operation=native_reader`. Each scope owns its
sequence and elapsed clock. Each phase reports its own duration in milliseconds.
Cold launch and local reader initialization carry the runtime generation. The
request, profile handoff, direct preparation, tab selection, window verification
and navigation paths use the issued action ID when available. An ownership or
expiry rejection during `navigation_guard` ends navigation before dispatch.
A successful `post_navigation_dispatch` records Chrome accepting the command.
Remote page readiness requires a separate observation. The canonical
`split_action` result carries the foreground state (`verified`, `manual_required`
or `unverified`). The HTTP request total measures time until its handler exits.

For a Bridge-assisted claim, filter by the same `action` and
`producer=bridge`, `operation=native_reader`, `scope=navigation`.
The worker times the Chrome tab API call. `navigate_marker_tab` sends the post
URL to the bound marker tab, `create_post_tab` opens a tab in an existing
reader window, and `reuse_post_tab` activates an exact URL match. Sidecar
projects the optional sample on result delivery. Log arrival can therefore
follow foreground verification. Older workers omit this sample.

The reusable browser sampler lives in
`internal/httpapi/web/diagnostics/browser-performance.js`, because it runs in the
browser rather than in Go. It owns opt-in observations, bounds, timers, cleanup
and inactive behavior. Callers supply approved stage and stop vocabularies.

The established `scroll-performance-trace.js` entry point is the Timeline
adapter. It supplies the five existing Timeline stages and existing stop reasons.
`diagnostics-ui.js` still owns recording controls, upload and download, while
`ui_performance.go` retains HTTP admission, body limits, status/error mapping and
the development-only gate. No diagnostic starts just because a module is loaded.

Native-post UI diagnostics remain a separate tab-local product boundary. Their
allowlists, session storage and bridge readiness behavior are not merged with the
browser performance sampler. Diagnostic export/redaction remains with its
existing Engine/HTTP owners; operational export is not a generic recorder.

## Preserved contracts

- Existing diagnostic JSON shapes and unknown/null behavior remain unchanged.
- Existing IG catalogs and PowerShell decoding consume the moved catalog.
- Window hooks, bounded emission, cancellation and platform build tags remain
  in the native adapter; no browser or capture action is added.
- HTTP report admission remains bounded and strict, and the latest report
  remains in memory only. New modules do not change endpoint paths or error text.
- Runtime activation and additional telemetry coverage are separately verified
  changes; diagnostic records remain independent of best-effort telemetry.

AG now imports the published AI4UDiagnostics `telemetry` package from alpha.4.
AkuSidecar now adopts the same published alpha.4 module without a local replace.
The existing generic diagnostics values remain in `pkg/diagnostics`; the new
adapter delegates provider/export construction to `telemetry`. The application
composition creates and closes one runtime. HTTP borrows it for the selected
Timeline performance, IG diagnostic and split-host startup sample operations.

The composition passes an empty exporter endpoint. No CLI/Settings switch or
environment-based exporter activation is added. The adapter accepts an explicit
loopback endpoint for deliberate consumers/tests, with the library enforcing its
transport policy. Operations/outcomes are closed enums; spans and metrics contain
no post content, source URLs/IDs, diagnostic bodies, credentials, raw errors or
per-scroll observations. Existing response headers and JSON receive no new trace
fields. Native capture and browser sampling retain their existing contracts.
