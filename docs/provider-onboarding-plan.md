# Provider selection during onboarding

Status: implemented, 27 August 2026. Stages 1–4 complete in AkuSidecar
(engine idle swap, onboarding dialog, tests, docs). Product contract updated
in AkuBrowser. Live E2E acceptance (dialog → Gemini selection with a real
key → first update on Gemini) remains open until the user provisions
`gemini.primary`.

## Problem

The first-run flow never asks which reasoning provider the user wants. The
provider select exists only in Settings, is disabled with "credential missing"
for Gemini/Ollama until someone hand-edits
`runtime/config/credentials.local.json`, and a provider switch requires a
Sidecar restart to take effect. The first onboarding update therefore always
runs on the checked-in default (Codex App Server) even when the user wants a
different provider, and the "Gemini · credential missing" wall appears with no
guidance about what to create or where.

## Decision (user, 2026-08-27)

First-run order becomes:

1. Choose sources, grant access, sign in — the existing readiness gate stays
   exactly as is. This is what the user already knows.
2. NEW provider selection dialog, shown after the readiness gate passes and
   before onboarding completes.
3. Onboarding completes; the first bounded update runs on the selected
   provider in the same process — no restart.

The visible catalog is three providers / four models:

| Provider | Model(s) | Positioning |
| --- | --- | --- |
| Codex App Server (default) | Codex Luna | Most compliant and reliable; local executable |
| Gemini | 3.5 Flash Lite | Needs a free Google AI Studio key; **privacy notice required: requests are processed by Google and free-tier data may be used by Google** |
| Ollama (local) | Nemotron 3.5 Lightning, Qwen 3.8 27B | Fully local; requires Ollama running with the model pulled |

Groq remains hidden (`hideFromSettings`) as an assessment provider.

## Current mechanics (verified)

- `GET /api/bootstrap` exposes `reasoningProviders[]` with
  `configured`/`configurationStatus`/`credentialName`; hidden providers are
  already excluded (`config.ProviderSummary`).
- `PUT /api/settings` rejects an unconfigured provider server-side
  (`Engine.SaveSettings`), persists `reasoningProvider`, and remembers
  per-provider profiles — but the running process keeps the startup provider
  until restart (`cmd/akusidecar/main.go` selects at boot).
- Executable path changes already apply immediately when idle via
  `executableRuntime.UseExecutable(...)`; the provider swap should follow the
  same idle-boundary precedent.
- The engine holds `provider`, `events *semanticengine.Engine` (wrapping a
  `resolver`), and `aiDeep aidetector.Resolver`; both structured resolvers are
  built from the provider at startup, so a swap must rebuild all three.

## Stages

### Stage 1 — Engine idle provider swap (Go)

New `internal/engine/provider_swap.go`:

- `SwapProvider(ctx, target)`:
  1. Copy `e.config`, `Reasoning.Select(target)` on the copy (flat fields
     only; the Providers map stays shared read-only), build
     `reasoning.NewProvider(cfgCopy)`.
  2. If the candidate is a `StructuredInvoker`, build replacement semantic
     (`semanticengine.NewStructuredResolver`) and AI Deep
     (`aidetector.NewStructuredResolver`) resolvers from the copy.
  3. Restore/migrate profiles exactly like `main.go`: activate the target
     provider's remembered profile set, migrate the four persisted profile
     slots through `EnsureResolvableProfile`.
  4. Take `operation.Lock`, verify idle (no active session, no `active` work,
     no pending Deep jobs), close the old provider, then swap `e.provider`,
     the event resolver (new `SetResolver` on `semanticengine.Engine`), and
     `e.aiDeep`.
  5. Log the swap, nudge `autoWake`.
- `SaveSettings` integration: when the target provider differs from the
  current one and is configured, perform the swap after persisting. When busy,
  fail with the same "finish the active update" contract the executable path
  uses. Settings persistence stays authoritative for the next restart.

### Stage 2 — Onboarding provider dialog (web UI)

- `index.html`: new `<dialog id="onboarding-provider-dialog">` beside the
  reset-confirmation dialog: provider cards container, description/privacy
  slot, setup-instructions slot, Re-check availability button, primary
  confirm, and a "Keep Codex App Server" secondary action.
- `app.js`:
  - `saveOnboarding` first-completion path: after `onboardingReadinessGate`
    passes, open the dialog instead of continuing directly.
  - Cards render from `state.bootstrap.reasoningProviders` with static
    per-provider copy keyed by name (Codex default/reliability; Gemini free
    key + Google data-use privacy warning; Ollama local/keep-model-running).
  - Unconfigured provider: inline setup instructions naming the exact local
    store path (`AkuSidecar\runtime\config\credentials.local.json`), the JSON
    shape from `config/credentials.example.json`, the AI Studio key URL for
    Gemini, and a Re-check button that refreshes
    `reasoningProviders` via `GET /api/settings`.
  - Confirm: `PUT /api/settings` with the unchanged settings plus the chosen
    `reasoningProvider` (engine hot-swaps), then continue into
    `PUT /api/onboarding` and the existing first-update flow.
  - "Keep Codex App Server": skips straight to onboarding.
  - Editing onboarding later never re-shows the dialog; Settings keeps its
    existing select, now taking effect immediately when idle.
- Copy updates: the Settings provider row no longer says "applies after the
  sidecar restarts" — it says the switch applies when no update is running.

### Stage 3 — Tests

- Go: swap success/failure fakes (close ordering, resolver replacement,
  profile migration), busy-rejection, unconfigured rejection.
- `server_test.go` UI content markers for the new dialog and privacy notice.
- Existing settings/onboarding suites stay green.

### Stage 4 — Documentation

- AkuBrowser `product-contract.md` first-run flow gains the provider step
  (after source readiness, before the first bounded update).
- AkuSidecar README: credential setup walkthrough for Gemini/Ollama in the
  development lane, and the idle-swap behavior note.

## Non-goals

- No UI for writing credential values: the local credential store remains a
  user-managed file; the dialog only detects and instructs.
- No change to Groq visibility, provider configs, model catalogs, or the
  composable-prompt contract.
- No automatic Sidecar restart flow; the swap happens in-process when idle.
