# AI4UDiagnostics alpha.4 telemetry adoption

## Published baseline and local adoption

The diagnostics extraction is published as
`96ad6d2bc1c90c4b7edda7291ec676cabd388be0`. The subsequent adoption uses the
official `github.com/abangkis/ai4u-diagnostics-go v0.1.0-alpha.4` module and its
`telemetry` package. There is no source-folder replace or workspace override.
The module checksum matches the shared library's immutable release record:
`h1:GiI7woBdGFp4dt7YlsWIVFE/smVZmv9a/WlLZl8Bt5I=`.

## Ownership and default-off behavior

`internal/diagnostics/observability` owns AkuSidecar's static signal vocabulary
and borrows official SDK interfaces from the shared telemetry runtime. The
application composition creates one adapter with `Config{}`, attaches it to HTTP
before Start, and owns its bounded shutdown. HTTP does not close providers.
Initialization or exporter failure does not replace diagnostic records or
prevent existing diagnostic HTTP behavior.

The application supplies no exporter endpoint. Ambient `OTEL_*` variables cannot
opt into export through this adapter. No Collector, CLI flag, Settings field,
global provider installation or automatic browser instrumentation is added.
Explicit programmatic configuration can select a library-approved loopback
endpoint. Exporter health is separate best-effort data, never authoritative
diagnostic state or proof of delivery.

## Instrumented boundary

| HTTP operation | Bounded telemetry operation |
| --- | --- |
| GET UI performance report | `ui_timeline_read` |
| POST UI performance report | `ui_timeline_write` |
| POST IG reader diagnostic | `instagram_reader` |
| POST IG bootstrap diagnostic | `instagram_bootstrap` |
| POST IG corroboration diagnostic | `instagram_corroboration` |

Each request produces a child span and request-count/duration metrics. Only
operation and outcome enums are recorded. HTTP status selects success, rejection
or unavailability, including handlers that write an error response but return
nil. Arbitrary route names, source/post identities, request/response bodies,
URLs, credentials and raw errors are not adapter inputs or signal attributes.
Existing request context is retained without copying its baggage into attributes.
Unselected routes retain the original response writer.

Per-scroll sampling, native-window observations and stored IG envelopes retain
their existing product contracts. This increment does not convert every
diagnostic into OTel or add trace fields to public JSON/headers.

## Compatibility evidence

- Existing generic diagnostic values, IG wire/catalog/privacy behavior, latest
  snapshot cloning, UI report schema, numeric limits and development gating pass.
- HTTP responses are compared byte-for-byte and header-for-header with and
  without instrumentation for accepted/rejected UI reports and production gates.
- The tagged development IG test verifies a handler-written 409 is counted as
  rejected rather than success.
- An explicitly failing local exporter leaves the authoritative saved report
  and diagnostic response intact. Default-off tests verify no requests despite
  configured environment variables, and no process-global provider changes.
- The isolated validation checkout passes Go tests across `internal/...`, the
  application CLI and native-observer helper, tagged diagnostic contract tests,
  module verification, and diagnostics/HTTP vet checks.

Dependency normalization was performed in a clean checkout because historical
Go fixture copies under ignored build/runtime directories contain retired import
paths. Those fixtures and unrelated X changes were preserved.

Publication and activation of this adoption remain separate from source and
fixture verification. No live model request or daily-runtime activation is part
of this increment.
