# Codex discovery through LibraryGo

AkuSidecar uses the published `github.com/abangkis/ai4u-library-go v0.4.0`
`pkg/codexruntime` package. The duplicated internal scanner and version-selection
implementation have been removed. The Inference SDK remains at v0.28.0; no local
module replacement is used.

## Selection and persistence

`reasoningExecutableMode` records `automatic` or `explicit` independently of
`reasoningExecutablePath`. In automatic mode, the path is a cached display value,
not an override. Startup, inactive-provider readiness, and provider switching
share the application policy in `internal/codexselection`. Discovery chooses the
highest capable installed version, subject to the caller's deadline.

An executable supplied by configuration or `--codex-path` takes precedence, then
`AKU_CODEX_PATH`, then an explicit saved selection. These are strict overrides:
missing or invalid executables fail without silently selecting another runtime.
Relative paths containing directory separators are resolved against the project
root. A plain command name is resolved through PATH.

Before selection mode existed, the application persisted automatic discovery
results in the same field as manually selected executables. Legacy paths matching
the known Codex App-managed Windows or macOS installation layouts are upgraded
to automatic, even when their version directory has been deleted. Other legacy
paths remain explicit. This is a bounded application migration policy, not proof
of the original user's intent. A newly stored `explicit` mode always preserves
the pin, including a path inside a managed installation directory. LibraryGo's
`IsManagedPath` is not used to classify deleted historical executables.

Saving an edited executable path records explicit mode; clearing it or using
Detect requests automatic mode. An unrelated settings save preserves the mode.
The provider switch persists the resolved path and mode together before swapping
the active provider. Readiness checks do not persist discoveries or switch providers.
Failure retains the previous provider and settings.

Discovery checks capabilities and version only. The existing SDK preflight checks
the initialize handshake without starting an inference turn; neither result is
proof that a later model request will succeed or that account quota is available.

## Local validation

Run `scripts/test-codex-discovery.ps1` with the published modules already cached.
The gate runs synthetic readiness/switch tests, the affected package suites,
frontend selection tests, and default/development builds. Artifacts stay under
`.go-build/codex-discovery/`; the gate does not replace managed runtime binaries,
start AkuBrowser, or open the user's database. The discovery integration fixture
uses copied test executables and rejects any RPC except initialize/initialized.

Key regressions include missing legacy managed paths, automatic caches,
strict explicit/config/environment overrides, read-only readiness, switching
from a remote provider, and preserving selection mode in the settings form.

### Validation on 2026-11-11

- Full suites passed for selection policy, domain, configuration, Store,
  reasoning, Engine, HTTP API, and the application command. The initial Engine
  run exposed a nil-provider fixture; the new selection guard was corrected and
  the full Engine suite then passed. The other passing package evidence was reused.
- The selection regressions passed with `auth_session_dev auth_session_fixture`.
  Additional default/development tests verified that Detect previews an explicit
  pin replacement without applying it and that automatic repair retains its mode.
- All 11 selected frontend tests passed. Both application variants built into
  the isolated validation directory. Embedded dependency metadata confirms
  LibraryGo v0.4.0 and Inference SDK v0.28.0.
- The separate build's `--discover-codex` found installed Codex
  `0.167.0-alpha.4.1`. An explicit missing path returned `not_found` and exit 2.
- Final live readback still showed Gemini as the active provider, the original
  stored path, and the original runtime process. No source activation occurred.

Evidence is retained locally under
`.go-build/codex-discovery/run-aba50a5f6fae4af3ad6ea56a52281398/`:
`go-tests.jsonl`, `engine-final.jsonl`, `development-tests.log`,
`discovery-mode-tests.jsonl`, `node-tests.log`, `build-modules.log`,
`installed-discovery.json`, `missing-explicit-discovery.json`, and
`unchanged-runtime.json`. The initial failed Engine result remains in the
original log; `engine-final.jsonl` records its corrected full-suite outcome.
