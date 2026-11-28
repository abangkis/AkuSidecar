# AkuBrowser operational logging coverage

This inventory covers the Stage 08 review of AkuSidecar, AkuBridge and the AkuBrowser launcher and installer on 2026-11-28. Use the signal's owner to find the original evidence.

## Following a native reader action

| Question | Owner and signal |
| --- | --- |
| Was the action queued, claimed and acknowledged? | AkuSidecar and AkuBridge emit `capture_activity`. Follow the same opaque action ID through the split action, native navigation or tab reuse, and foreground result. Sequence numbers and elapsed times are local to the emitter. The SQLite audit supplies a separate durable record. |
| Did the UI extension reach the Windows helper? | The separately packaged Sidecar `ui-reader-broker` extension emits `aku_reader_broker` in its own service worker console. Match `requestId` to the broker request in the action trace. A forwarded click attempts one `helper_start` and one `helper_result`. Rejection or transport failure can occur before Sidecar receives anything. The result records categorical focus information when available, omits the URL and error text, and preserves the original reply if the console fails. |
| What happened during startup or outside a click? | The Sidecar startup recorder owns `capture_startup`. The Sidecar operational adapter owns `sidecar_activity` for application, engine and HTTP events. AkuBridge's diagnostics adapters own split-capture and service-worker events, including `bridge_activity`. The runtime native host writes diagnostics to stderr and reserves stdout for native messaging. |
| Where are Web UI and Headless events? | The Web UI adapter owns `ui_activity`, `native_post_trace`, `native_post_trace_recovered` and `timeline_feedback_trace` in the UI console. The Headless worker adapter writes `headless_worker` JSON to stderr. Sidecar forwards recognized worker errors as `sidecar_activity` with operation `headless_worker`. The detail includes the launcher (`headless` or `quiet`), worker sequence and categorical reason. Each launcher has a 16-event queue and a budget of 16 forwarded records per one-minute window. Lines over 1 KiB and incomplete lines are dropped. Raw stderr and arbitrary message fields stay out of the Sidecar log. Worker stdout carries protocol messages. |

## Stored evidence and export coverage

| Contract | Writer, reader and bounds |
| --- | --- |
| `capture_surface_events` | Sidecar's authenticated HTTP intake accepts batches of 1â€“32 events and reserves `native_trace` for the native path. The store checks event names, limits detail to 8 KiB per event, ignores duplicate IDs and returns a run's events in timestamp order. |
| `split_action_audit` | The Sidecar store retains the latest 512 entries. Its schema permits two action types, six phases and three outcomes. The reader exposes the action ID and timestamp without adding a URL or free-form error. |
| `runtime/calibration-results.jsonl` | The Sidecar calibration handler appends structured reports. Its reader returns the newest 20, newest first, with a 1 MiB scanner limit per line. The 20-report window bounds reads. Disk usage can grow between cleanups. |
| `update-audit.jsonl` | AkuBridge's native runtime updater appends time, source and target versions, phase and code. When the existing file exceeds 1 MiB before an append, the updater rotates it to `.previous`. The rotation fixture checks the boundary and the retained bytes. |
| Timeline snapshots | The development UI performance endpoint stores the latest validated report in memory, returns a copy, limits the request to 64 KiB and resets on restart. The SQLite timeline retention path separately preserves session/run evidence before removal. Each has its own reader and regression tests. |
| `GET /api/diagnostics/export` | Sidecar exports `metadata`, `config`, `sessions`, `modelUsage`, `reasoning`, `bridge`, `errors` and `generatedAt`. Session selection uses the explicit session, then the active session, then up to ten Inbox entries. The default time range is 24 hours. This endpoint does not collect process output, extension consoles, raw audit tables, JSONL files or the latest UI performance report. |

The export uses field-specific redaction. The fixture verifies that the private root is omitted. Session intent, coverage values, nested run error messages and the configured reasoning executable path can still appear. Treat these fields as potentially identifying when reviewing an export for sharing.

OTLP export is opt-in. The application supplies an empty configuration. The adapter ignores ambient `OTEL_*` variables and installs no global provider. Explicit programmatic configuration can select a library-approved loopback endpoint. Coverage is limited to UI Timeline read/write and the Instagram reader, bootstrap and corroboration operations. An unavailable exporter leaves the diagnostic response intact. See [telemetry configuration and contract tests](diagnostics-telemetry-alpha4-adoption.md).

## Remaining direct output

| Owner and source | Why the direct output remains |
| --- | --- |
| Sidecar `internal/diagnostics/{captureactivity,capturestartup,operational}`, Web UI and Headless adapters, AkuBridge `diagnostics/` and native host | These are the sinks that serialize records to their respective destinations. Sidecar's CLI owns its process logger and startup output. |
| Sidecar UI reader broker | Its extension console observes the browser-to-helper call before Sidecar can record a receipt. Development packaging and broker fixtures consume its phase names. |
| Sidecar native reader helper, Headless native-pilot owners and transports, screenshot worker, AkuBridge runtime native host | Their stdout carries framed native messages, JSON protocol replies or tool results. The child-process consumers depend on those bytes. |
| Headless Instagram history diagnostic | A bounded page-context console marker connects history changes to a CDP listener. The listener checks the marker and execution context. |
| AkuBrowser launcher and installer | The launcher forwards child streams. Database preflight writes a bounded diagnostic excerpt to `database-recovery.log` before Sidecar can start. Installer prompts, CLI errors and signing-tool output have operator or command-line consumers. |
| Chromium and HLS | Chromium owns `chrome-startup.log` when isolated startup diagnostics are explicitly enabled. The vendored HLS logger belongs to the upstream player. |
| Runtime state and artifacts | Sidecar writes startup readiness, activation holds, broker assets, database backups and version markers. Media provenance, screenshots and hash writers produce artifacts or digests. The AkuBridge native host owns downloaded archives, extracted files, locks and runtime metadata. These writers have file or protocol readers. |
| Development and build tools | Semantic replay, auth/process fixtures, QA staging, native-pilot fixtures, browser test data and the shared DOM build write results, assets or manifests for their harnesses. |

To repeat the source inventory, run the following search in each checkout. Inspect each hit with its caller and consumer, including build-tagged files. Check the supervisor and packaging scripts for consumers before retiring a prefix.

```sh
git grep -nE '([.]Printf\(|console\.(log|info|warn|error|debug)|os\.(Stdout|Stderr|WriteFile|OpenFile)|process\.(stdout|stderr)|fmt\.Fprint|log\.(Print|Fatal))' -- '*.go' '*.js' '*.mjs'
```

The review found no remaining emitter or executable consumer for the superseded split-action, native-reader timing, z-order and Headless warning formats checked. Historical examples in troubleshooting documents still describe older builds.

## Verification on 2026-11-28

- **Live, published Stage 07.** The listener process and binary digest matched the clean Sidecar build at `d7d5c39274dd9bd7ca0169cee402fe0fe922a307`. Health was `ok` and AkuBridge v142 was healthy and compatible. Between 18:59:00 and 19:05:47 WIB, the last 100 Supervisor stdout records contained 50 poll starts and 50 ends. There were 49 complete pairs inside the sample. All poll IDs had the expected format. The sample had zero raw HTTP(S) URL or Bearer matches. This sample covers background polling only.
- **Local candidate and fixtures.** Sidecar Go and Node tests cover broker success, rejection and transport error with a failing console, audit limits, ledger reads, Timeline retention, export selection and redaction, and default-off telemetry. AkuBridge native-host tests cover update audit rotation. Launcher tests cover the preflight diagnostic. Store limits and error branches were exercised against isolated fixtures. Live evidence for the Stage 08 broker change still requires an activated candidate and a forwarded click.
