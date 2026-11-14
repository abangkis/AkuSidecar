# AkuBrowser internal diagnostics

This package family separates reusable recording mechanisms from AkuBrowser
contracts, collection authority, transport, and presentation. It is an internal
extraction, not a new published library or a new telemetry backend.

| Package | Responsibility | Reuse boundary |
| --- | --- | --- |
| `recording` | Concurrent progress snapshots, first observed cause, independent cleanup/delivery, explicit completeness | Reusable candidate; caller supplies vocabulary and observations |
| `snapshot` | Single latest value with copy-on-save/copy-on-read and concurrency control | Reusable candidate; the value supplies its deep-clone operation |
| `timeline` | Closed browser-performance report schema, numeric/shape validation, cloning, and latest-report composition | AkuBrowser adapter; stage names, bounds, UTC receipt time and privacy policy stay here |
| `instagram` | Existing IG catalogs, attempt ownership, wire/public projection and validation | AkuBrowser adapter; does not grant capture, retry or mutation authority |
| `nativecapture` | Existing bounded platform-specific native-window observations and classification | AkuBrowser/Windows adapter; process classification and capture lifecycle stay here |

`recording` continues to use the generic values in the pinned AI4UDiagnostics
package. `snapshot` depends only on Go synchronization and its clone contract.
Neither package knows HTTP, browser selectors, native profiles, source identity,
timing acquisition, collector configuration, UI labels, or storage databases.
Independent-consumer tests use unrelated operations/value types rather than
claiming another application has already adopted these candidates.

## Browser boundary

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
- OpenTelemetry initialization/export, module-version upgrades, runtime
  activation, and migration to a published shared library are separate changes.

AG now imports the published AI4UDiagnostics `telemetry` package from alpha.4.
This extraction follows the same mechanism-versus-product ownership principle;
it preserves AkuSidecar's existing alpha.3 pin. The root diagnostics value package
and OTel provider lifecycle are distinct concerns.
