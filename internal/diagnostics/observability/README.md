# Diagnostic request observability adapter

The application composition root owns `Runtime` and calls `Shutdown`. HTTP
handlers borrow it and call `Begin` once for a selected diagnostic request,
then complete it once with `Success`, `Rejected`, or `Unavailable`. Completion
is idempotent. Unknown operations are no-ops; unknown outcomes become
`unavailable`. A nil runtime is safe.

`New` uses the published AI4UDiagnostics `telemetry` runtime, with explicit
`AkuSidecar` service identity and optional build metadata. It installs no global
providers or error handlers. `Config` reads no environment variables. An empty
`OTLPEndpoint` enables no exporter, including when OTLP environment variables
are set. Explicit endpoint validation, provider ownership, bounded transport,
exporter health, and idempotent shutdown belong to the shared runtime.

The adapter owns its timer and the following product signals:

| Signal | Contract |
| --- | --- |
| Request span | `akubrowser.diagnostics.<operation>` |
| Counter | `akubrowser.diagnostics.requests`, unit `{request}` |
| Duration histogram | `akubrowser.diagnostics.duration`, unit `ms` |

All signals use scope
`github.com/abangkis/AkuSidecar/internal/diagnostics/observability`.
Their only attributes are `operation` and `outcome`. Operations are limited to
`ui_timeline_read`, `ui_timeline_write`, `instagram_reader`,
`instagram_bootstrap`, and `instagram_corroboration`. Outcomes are limited to
`success`, `rejected`, and `unavailable`.

The API accepts no raw paths, URLs, source/post IDs, worker output, tokens, or
errors. It does not create per-scroll observations, modify diagnostic reports,
or add response fields. Existing report persistence and diagnostic snapshots
remain authoritative. Export is best-effort; bounded exporter health is
available through `Diagnostics` and never determines a business response.
