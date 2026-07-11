# AkuSidecar

AkuSidecar is the local AkuBrowser runtime. It owns the pinned local UI, HTTP API, SQLite state, bounded job engine, provider-neutral browser-capture policy, and replaceable reasoning-provider adapters.

## Requirements

- Node.js 24 or newer
- A local Codex login when using the `codex-sdk` provider

## Development

```powershell
npm install
$env:AKU_REASONING_PROVIDER='codex-sdk'
npm run dev
```

Open `http://127.0.0.1:47821` in the same Chrome profile where AkuBridge is loaded.

Development uses one visible process and one port. Vite runs as middleware inside the Sidecar HTTP server and hot-reloads `public/` assets. Node's watcher automatically restarts that same process when backend modules change, so neither path requires a manual restart.

The default daily-use action creates one persisted Unified Session. AkuSidecar runs an X child followed by a LinkedIn child, keeps their checkpoints and feedback independent, and deterministically merges up to five validated items per source into one finite brief. Advanced/Pilot mode preserves the original single-source flow. Browser movement budgets remain unchanged.

For the production-style static server without file watching:

```powershell
npm start
```

## Configuration

Codex reasoning can be tuned without changing source code:

- `AKU_REASONING_PROVIDER=codex-sdk`
- `AKU_CODEX_MODEL=<model id>`; omit it to inherit the Codex CLI default
- `AKU_CODEX_PLANNING_MODEL=<model id>`; overrides only acquisition planning
- `AKU_CODEX_EVALUATION_MODEL=<model id>`; overrides only candidate evaluation
- `AKU_CODEX_PLANNING_EFFORT=minimal|low|medium|high|xhigh`
- `AKU_CODEX_EVALUATION_EFFORT=minimal|low|medium|high|xhigh`
- `AKU_CODEX_TIMEOUT_MS=<milliseconds>`

Committed defaults live in `config/reasoning.json`: Luna High for the narrow acquisition-planning fallback, Terra High for candidate evaluation, and `deterministic_sparse_gap` so planning tokens are spent only when one or two unseen candidates and an exhausted movement budget make one anchored follow-up plausible. `AKU_CODEX_MODEL` remains a convenient shared override; phase-specific environment variables take precedence.

The effective configured model and evaluation effort appear in the AkuBrowser header. Provider-reported input, cached-input, output, and reasoning-output tokens are stored per reasoning invocation for local performance and economic analysis. Token telemetry is not presented as a monetary cost unless a separate pricing contract is configured.

- `AKU_BROWSER_PORT` defaults to `47821`.
- `AKU_REASONING_PROVIDER` is `deterministic` or `codex-sdk`.
- `AKU_DATABASE_PATH` overrides the local SQLite path.
- `AKU_CODEX_PATH` overrides the packaged Codex CLI path.
- `AKU_CODEX_TIMEOUT_MS` defaults to `120000`.

Gate 0B uses a fixed native-capture budget: at most two 75%-viewport scrolls, three snapshots, and 45 seconds. AkuBridge restores the applicable capture baseline and reports the actual movement in coverage. Computer Use is not an implicit fallback.

Gate 0B.2 explicitly requests one allowlisted same-tab activation when `New posts`/`Show posts` is visible. Coverage distinguishes the pre-action position from the post-reveal baseline and never claims that the old feed view was restored.

Gate 0B.3 asks the configured ReasoningProvider only whether to finish or request one adjacent observation. A follow-up is capped at one scroll, locked to the same source, anchored to the last round-one viewport, and cannot activate fresh-content controls. Both rounds are stored and merged before the final result.

## Knowledge continuity

Every validated evidence block receives a deterministic identity. A completed run advances one checkpoint for its source and mode. Evidence previously delivered as a result is suppressed on later runs. When the user marks an empty result `Correctly empty`, the observed evidence is also stored as `confirmed_excluded` and suppressed only for the same source, mode, and normalized intent. A changed intent makes it eligible again. New semantic deltas are attached to stable event keys and stored as append-only versions.

If every evidence block in the initial bounded acquisition was already evaluated for the same intent, JobEngine finishes deterministically without asking the ReasoningProvider to plan another acquisition round.

The current frontier is inspectable through:

- `GET /api/knowledge?source=x&mode=catch_up`
- `GET /api/knowledge/events/{eventKey}?source=x&mode=catch_up`

## Pilot Review

The `Pilot Review` view is an evaluation surface, not another consumption mode. It summarizes the feedback-bearing pilot cohort and lets the user review completed empty runs or promoted items without losing the current Session result.

The cohort starts at the first run that receives feedback. Metrics are calculated over at most the latest 500 runs in that cohort and remain based on the selected source even when a verdict filter narrows the visible run list. The UI exposes the numerator and denominator for trust and item-quality rates so an unrated sample is not presented as success.

Feedback integrity is enforced by JobEngine:

- `Correctly empty` and `Missed something` are mutually exclusive run-level verdicts for completed empty runs;
- `Missed something` requires a non-empty note describing what should have appeared;
- `Useful`, `Correct lane`, `Wrong lane`, and `Duplicate` require an item that exists in the completed result; and
- repeated submission of the same verdict is idempotent.

The review API is `GET /api/pilot/review`. Optional query parameters are `source`, `verdict`, and `limit`. The response intentionally returns result, coverage, and feedback evidence but not raw browser observations.

## Unified Session API

- `POST /api/sessions` creates the bounded X + LinkedIn parent session.
- `GET /api/sessions/active` restores the latest non-terminal session after a page or Sidecar reload.
- `GET /api/sessions/{sessionId}` reconciles child status, starts the next sequential source when appropriate, and returns the persisted unified result.
- `POST /api/sessions/{sessionId}/cancel` cancels the active child and prevents queued sources from starting.

Existing `/api/runs` and bridge-command endpoints remain source-specific. The bridge still receives one run ID at a time.

## Verification

```powershell
npm run check
npm run smoke:codex
```

AkuSidecar does not import AkuBridge source. Their only runtime dependency is the versioned localhost bridge contract.
