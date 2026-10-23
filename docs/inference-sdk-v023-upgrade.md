# Inference SDK v0.23 upgrade contract

Historical upgrade record. The [v0.24 contract](inference-sdk-v024-upgrade.md)
supersedes its dependency version and calibration-assurance behavior; the
validation and activation boundaries recorded here remain historical evidence.

## Dependency and activation boundary

AkuSidecar requires the published `github.com/abangkis/ai4u-inference-sdk-go`
`v0.23.0` module and Common Execution Profile `v0.2.0`. `go.sum` pins both;
there is no local replacement. The minimum Go version in `go.mod` remains `1.25.0`.

This source upgrade does not deploy or restart the development runtime. It does
not change stored settings, provider/model selections, authentication, session
integration, or the Store contract. Existing paused session work remains paused.

## Request and provider contracts

- **Codex output tokens:** the App Server protocol cannot enforce an output
  token limit. Codex requests therefore send `MaxOutputTokens: 0`. The positive
  execution-profile target remains present because Common Profile requires it;
  it is not an enforced Codex cap. Provider metadata reports
  `outputTokenLimitEnforced: false`. Other generative connectors retain their
  configured request limits.
- **Codex calibration:** SDK calibration defaults to a nonzero output budget,
  which Codex now rejects. Sidecar returns `ErrCalibrationUnsupported` before
  executable discovery or session startup. The development diagnostics API maps
  this to HTTP 422 / `calibration_unsupported`, without appending a report.
- **Ollama:** SDK v0.23 has no stable curated model catalog. Existing model IDs,
  including `qwen3.8-27b` and `nemotron-3.5-lightning`, remain unchanged and fail
  explicitly as unsupported. Sidecar does not select replacement models or opt
  into experimental SDK routes. Readiness returns `model_unsupported` before an
  endpoint probe; calibration maps the typed error to HTTP 422 /
  `model_unsupported`. Installing the old model does not restore SDK support.
- **System instructions and schemas:** preserve the structured JSON developer
  instruction and validate schemas before provider execution. Do not weaken
  schemas or coerce numeric identifiers to work around stricter validation.
- **Lifecycle:** stop admission and detach transports under a state lock, but do
  not hold that lock while waiting on external work. Close the adapter before
  waiting for binding work so blocked operations can be interrupted. Late
  bindings must release their leases and cannot revive a closed pool.

The source upgrade is compatible with the existing active Codex route, subject
to the explicit token-limit restriction. Configurations selecting a retired
Ollama route require an explicit user decision before that route can run again.

## Offline validation

The offline gate uses published modules already present in the project-local
module cache, disables live-provider test flags and module networking, and
redirects test/build temporary files into the project. Fake subprocesses and
loopback HTTP servers exercise provider contracts. No real inference is run.

The legacy `TestNewProviderRoutesGeminiEntries` factory test is excluded because
it consults the OS credential store before its synthetic JSON fallback. Gemini
request/schema tests still run with an injected static fake resolver. Live
Codex, Gemini and Groq tests remain disabled by their explicit environment gates.

Engine and HTTP checks run selected pure source files to exclude Store-backed
test setup. Full package test binaries and the Sidecar executable are compiled
into a separate build directory but never launched. User runtime binaries are
not overwritten. Full Store-backed integration suites and live-provider QA are
outside this validation scope.

Run the reproducible gate from the repository root:

```powershell
.\scripts\test-inference-sdk-offline.ps1
```

## Validation record (2026-10-23)

The canonical offline gate passed using the installed Go `1.26.10` toolchain:

- Config and reasoning suites passed with the credential-store factory test
  explicitly excluded and all three live-provider gates disabled.
- Pure Engine readiness and HTTP calibration-error tests passed without loading
  either package's Store-backed test setup.
- Full default Engine and HTTP test binaries compiled successfully; they were
  not executed.
- Default and combined `auth_session_dev auth_session_fixture` Sidecar builds
  passed in an isolated project-local directory. Build metadata confirmed SDK
  `v0.23.0` and Common Profile `v0.2.0` in the default binary.
- Offline module integrity verification passed. The SDK library checkout,
  checked-in user model defaults, Store implementation and managed runtime
  binaries were not changed.

The gate log and separate binaries are retained under
`.go-build/inference-sdk-v023/run-ccbd305f003f41a082c85f55d6ad03df/` for review.
This is offline compatibility evidence, not live-provider or deployment acceptance.
