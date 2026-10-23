# Inference SDK v0.24 upgrade contract

## Dependency and scope

AkuSidecar requires the published
`github.com/abangkis/ai4u-inference-sdk-go v0.24.0` module, tag commit
`323ae3e15db5cd74f2e30f888b8b11094539201d`. The module was downloaded directly
from the official Git repository into the project-local module cache, with
noninteractive cached Git authentication. `go.sum` pins the downloaded module;
there is no local replacement or workspace override. Common Execution Profile
remains `v0.2.0`, and the minimum Go version remains `1.25.0`.

This upgrade changes source dependencies and calibration policy propagation.
It does not deploy or restart the managed runtime, change stored settings or
model selections, enable prompt fallback, enroll an account, run native Chrome
QA, or modify Store. The preceding [v0.23 contract](inference-sdk-v023-upgrade.md)
remains the record of that earlier upgrade.

## Calibration assurance

`RunCalibration` now passes the evaluation workload's selected assurance into
the SDK's calibration request. Whitespace is normalized; omission explicitly
selects `sdk_validated`, and `provider_strict` retains its stricter resolution
requirement. `prompt_fallback` and unknown policies reject before constructing
an adapter, resolving credentials or contacting an endpoint, including direct
calls that bypass general config validation. Existing Settings/config policy is
unchanged and does not offer or permit fallback.

The unexported adapter factory permits offline tests to exercise the real SDK
calibration resolver and synthetic-schema validator with a fake adapter, without
accessing the OS credential store. Production construction still uses the
existing Ollama, Groq and Gemini adapters with their existing cleanup and output
budget rules. Reports retain the SDK's `requestedAssurancePolicy` evidence.
Optional fallback evidence remains nullable; absence is not proof of `false`.
No calibration report is appended to the user's live ledger during verification.

Codex calibration remains unsupported because calibration requires a positive
output-token budget that App Server cannot enforce. Generation still sends
`MaxOutputTokens: 0` for Codex while keeping the workload profile's positive
target. Retired Ollama models still fail before endpoint access; no model is
substituted or opted into an experimental route.

## Timeout behavior and regression boundary

SDK v0.24 counts direct Codex admission and pooled queue time within the same
configured invocation timeout. An earlier caller deadline still wins. Queued
cancellation must not interrupt the active session owner; active cancellation
must release its slot/transport so a later invocation can use a fresh session.
Single-session and opt-in pooled regression cases exercise this through the
Sidecar provider with the existing fake App Server subprocess, not the user's
Codex runtime. Existing tests retain Close, malformed-schema, instruction,
output-budget and process-recovery contracts.

Startup callbacks must still return promptly, and cleanup can extend beyond the
invocation deadline. Local cancellation does not prove remote generation or
billing was canceled. This SDK's App Server admission concerns inference; it
does not resolve the separate X browser/CDP identity-acquisition blocker.

## Offline validation

```powershell
.\scripts\test-inference-sdk-offline.ps1
```

The gate requires the exact published SDK v0.24 and Common Profile v0.2 modules
already cached, verifies module integrity and disables module networking during
tests. All live-provider flags are disabled, and cache, temporary files and
standalone outputs stay under project-local `.go-build` directories. Process
environment variables are restored when the gate exits.

The config/reasoning suites exclude only the pre-existing
`TestNewProviderRoutesGeminiEntries` factory test, which consults the OS credential
store. Other provider/schema tests use injected credentials and loopback mocks.
Engine readiness and HTTP calibration-error tests use named source files to
exclude Store-backed test setup. Full Engine/HTTP test binaries and default/
fixture Sidecar executables are compiled separately and never launched. Runtime
binaries are not overwritten. This is offline compatibility acceptance, not
deployment or live-provider acceptance.

## Validation record (2026-10-23)

The canonical gate passed with Go `1.26.10` against the published module, with
module networking disabled during testing. Config and reasoning tests passed,
including calibration default/explicit/strict assurance, pre-adapter fallback
rejection, synthetic response-schema rejection and the single/pool queued-caller
deadline regressions. Named-file Engine readiness and HTTP calibration-error
tests passed without loading Store-backed test setup. Full Engine/HTTP test
binaries and default/fixture Sidecar binaries compiled without being launched.
`go mod verify` passed. Default binary build metadata records SDK `v0.24.0` and
Common Profile `v0.2.0`, with no replacement.

The first sandboxed run exposed blocked loopback mock connections and a missing
native-assurance declaration in the new synthetic calibration catalog. The
fixture declaration was corrected; the unchanged offline gate then passed
outside the sandbox with all live-provider gates still disabled. No production
policy or schema was relaxed to make the tests pass.

Evidence and isolated binaries are retained under
`.go-build/inference-sdk-v024/run-133fafcc3fe74847a3abbb9faf2fc384/`; its
`offline-results.log` records the commands, results and build metadata. The
source model defaults, config assurance allowlist, Store, main authentication,
managed runtime binaries and SDK library checkout were not changed. There was
no live inference, native browser QA, deployment or runtime restart.
