# IG diagnostic layering

The first migration uses published `github.com/abangkis/ai4u-diagnostics-go`
`v0.1.0-alpha.3`. It covers IG diagnostics and IG verifier observations only.
Existing X diagnostics, source admission, account proof, consent, retry policy,
Store authorization and operational cleanup authority are not migrated.

## Dependency boundaries

| Layer | Owner | Responsibility |
| --- | --- | --- |
| Generic values | Published AI4UDiagnostics | Operation/phase/location, knownness and bounded numeric projection |
| Observation recorder | `internal/diagnostictrace` | First observed cause, progress, independent cleanup/delivery and completeness |
| IG contract | `internal/igdiagnostic` | Closed catalogs, evidence states, validation, safe wire/public projection |
| Execution | Engine, owner binding, appshell and Node scope | Admission, clocks, deadlines, browser resources, retirement and shutdown |
| Presentation | HTTP and operator helper | Sanitized response and retained status observation |

The internal recorder is a future extraction candidate. It is deliberately not
published upstream in this change. It has no IG vocabulary, clocks, resource
authority, retry decisions or serialization policy. A second consumer must
demonstrate compatible semantics before extraction. Alpha.3 itself is unchanged;
no JavaScript package, OpenTelemetry exporter or shared runtime is introduced.

The IG adapter uses library `Location` values in actual recorder state and uses
`Observed[time.Duration]`, `Milliseconds` and bounded counts at its projection
boundary. Library values are not directly serialized as public JSON and are not
a sanitizer. Numeric projection limits do not enforce operation deadlines.

## Observations and authority

The diagnostic attempt identifier is random and separate from the nonce used by
the sealed browser operation. Neither identifier may be derived from an account,
cookie, target or frame. Build/contract provenance is application-owned.

Execution disposition distinguishes `not_started`, `start_unknown`, `started`,
`completed` and `result_lost`. A dispatched operation without an acknowledgement
has unknown execution, not proof of non-execution. Missing terminal data cannot
prove either account failure or successful cleanup.

The first recorded failure is immutable. It means the first locally observed
failure, not proven causal ordering between processes. Cleanup and delivery are
separate observations. A stream-close failure must not be described as a read
failure when reading succeeded. Releasing a Go lease does not prove that a
browser target or stream disappeared.

IG evidence uses `not_observed`, `passed`, `failed` and `invalidated`.
`Observed[bool]` alone cannot represent these states. A successful bootstrap
corroboration remains discovery with `qualified:false`; it does not replace the
production verifier or grant Store admission.

Go and Node keep their own elapsed-time origins. Observations do not compare
unsynchronized wall clocks. Remaining operation budget includes queue/transport
cost and reserves cleanup within the existing total deadline. Observability
must not extend consent, permit another attempt, release resources or select a
retry. Malformed or oversized telemetry marks diagnostics incomplete while
preserving the independent operational cause.

The IG dispatch budget is sampled again after acquiring the Go write lane, so
waiting for another writer cannot silently grant a fresh Node budget. Unknown
pipe/process transit delay remains an observation limit; the relative budget is
not proof of an absolute deadline shared by separate process clocks. The parent
deadline is unchanged. After cancellation, an opt-in passive RPC observer may
retain sanitized late progress for at most 30 seconds and 32 correlations. Its
timer expires the observation, and terminal delivery or transport closure clears
it. It cannot revive the operation or retire resources on its own.

## HTTP and operator observation

Existing diagnostic success bodies retain their operation-specific schemas.
Responses include `X-Aku-Diagnostic-ID`. Failed diagnostic responses include a
strict, bounded trace in `details.diagnostic` in addition to their existing API
error code.

`GET /api/diagnostics/instagram/status` returns the latest admitted attempt as
`{"diagnostic": ...}`, or `{"diagnostic": null}` before any admitted attempt.
It requires the current `X-Aku-Split-Epoch`, normal API security and the existing
development-build configuration. It accepts no query parameters. It never
dispatches a probe, resets one-shot permission or creates a collector. Only one
attempt snapshot is retained in memory for this Sidecar process. A rejected
parallel request or replay does not replace the admitted attempt.

HTTP `response_written` means the server write succeeded; it does not claim that
the client received the response. The snapshot included in the response can
still have pending delivery because it was serialized before that write. The
read-only status endpoint exposes the later local write observation.

`delivery` records RPC-result observation, while `httpDelivery` records the HTTP
response write. A successful error-response write cannot replace a missing RPC
terminal result. IG daily source status additionally includes `igDiagnostic`
when its existing operational failure carries a validated verifier trace. The
existing source failure code and retry classification remain independent.

The PowerShell operator helper reads the fixed, matched source catalog at
`internal/igdiagnostic/catalog.json`. If that catalog is unavailable, it falls
back to the existing code-only projection. Invalid trace metadata is omitted and
marked `diagnosticIncomplete`; it never replaces the independently accepted API
error code. Run the helper from the corresponding source checkout.

## Privacy and compatibility

Only application-owned catalogs, bounded numeric observations and structural
states may cross the diagnostic boundary. Raw bodies, cookie/token values,
account identifiers (including hashes), URLs, DOM fragments, CDP parameters,
exception messages and stacks are excluded. Validators reject duplicate fields,
unknown fields, invalid enum values and over-limit serialized representations.
No arbitrary exception properties are trusted as diagnostic categories.

The production verifier's `current_user` response and the bootstrap diagnostic
remain different operations. An HTTP 400 from that verifier must not be confused
with HTTP 409 from local diagnostic admission or transport. Offline tests do not
qualify either source against a live IG account.

## Validation requirements

The contract is tested with synthetic observations and actual Go/Node framing,
without browser or account access. Coverage includes early rejection, timeout,
EOF, malformed terminal/progress frames, independent cleanup, missing evidence,
privacy canaries and unchanged one-shot/permission behavior. Source changes must
not require a runtime restart for offline tests. Build artifacts and test
temporary directories belong under the isolated checkout's `build` directory.

Validation for this change passed the targeted IG/diagnostic Go tests in Engine,
HTTP and main, the generic recorder/IG contract/ownedrpc suites and vet, synthetic
Go-to-Node wire tests (including EOF and missing terminal results), affected Node
transport/diagnostic tests, existing IG provider/bootstrap regressions, and the
PowerShell response projection tests. Both ordinary and `auth_session_dev`
Sidecar binaries compiled into project-local `build/ig-diagnostic-layering`.
The source module was resolved from the existing module cache with
`GOWORK=off`, `GOPROXY=off`, no local replacement, and checksums matching the
alpha.3 publication record. This is not a fresh remote-publication audit.

Runtime activation, session transfer and real-account verification remain
separate operational steps. No such operation is part of this migration.
