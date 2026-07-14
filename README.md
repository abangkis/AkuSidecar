# AkuSidecar

AkuSidecar is the local AkuBrowser runtime. It owns the pinned local UI, HTTP API, SQLite state, bounded job engine, provider-neutral browser-capture policy, and replaceable reasoning-provider adapters.

## Requirements

- Node.js 24 or newer
- A local Codex login when using the `codex-sdk` provider

## Development

Install dependencies once:

```powershell
npm install
```

For normal AkuWorkspace development, let the user-visible AkuSupervisor own the
Sidecar watcher and its complete process tree:

```powershell
cd ..\AkuSupervisor
.\scripts\dev.ps1 akusidecar
```

Use `npm run dev` directly only for isolated Sidecar development when
AkuSupervisor is intentionally not in use. Do not set a reasoning-provider
environment variable for normal startup; the committed default and persisted
AkuBrowser Settings select `codex-sdk`.

Open `http://127.0.0.1:47821` in the same Chrome profile where AkuBridge is loaded.

Development uses one visible process and one port. Vite runs as middleware inside the Sidecar HTTP server and hot-reloads `public/` assets. Node's watcher automatically restarts that same process when backend modules change, so neither path requires a manual restart.

The default daily-use action creates one persisted Unified Session. AkuSidecar runs an X child followed by a LinkedIn child, keeps their checkpoints and feedback independent, and deterministically merges up to five validated items per source into one finite brief. Advanced/Pilot mode preserves the original single-source flow. Browser movement budgets remain unchanged.

Source layout reconstructs each item from persisted candidate text, provenance, and at most four validated source images or video posters. Images are presentation-only, lazy-loaded without a referrer, and omitted from reasoning prompts. A cache miss may request the original allowlisted source CDN; AkuBrowser does not reopen or recapture the source page.

For the production-style static server without file watching:

```powershell
npm start
```

## Configuration

The normal configuration surface is AkuBrowser **Settings**. Settings are
allowlisted, persisted in SQLite, survive Sidecar restarts, and report their
effective value, persisted value, source, and apply mode.

Settings that apply live or to the next run include source activation,
presentation, Timeline capacity, stream width, telemetry behavior, calibration,
missing-source-tab policy, per-source item budget, native scrolls, acquisition
rounds, and knowledge-context size. Reasoning provider, planning/evaluation
model, effort, planning policy, and timeout are startup settings: saving them
marks a visible restart as required. Restart AkuSidecar through AkuSupervisor;
the application never hot-swaps a provider or starts a hidden replacement.

Committed reasoning defaults live in `config/reasoning.json`: `codex-sdk`, Luna
High for acquisition planning, Terra High for candidate evaluation, and
`deterministic_sparse_gap`. A fresh installation can therefore run without any
environment setup.

The effective configured model and evaluation effort appear in the AkuBrowser header. Provider-reported input, cached-input, output, and reasoning-output tokens are stored per reasoning invocation for local performance and economic analysis. Token telemetry is not presented as a monetary cost unless a separate pricing contract is configured.

### Legacy and recovery environment overrides

Environment variables remain implemented for compatibility, packaging, and
short-lived recovery diagnostics, but they are not the recommended install or
daily-run workflow. An active override takes precedence over SQLite and locks
the corresponding Settings control, which can make the visible configuration
misleading if it is left behind. Remove the override after the diagnostic run.

The supported compatibility overrides are `AKU_REASONING_PROVIDER`,
`AKU_CODEX_MODEL`, `AKU_CODEX_PLANNING_MODEL`,
`AKU_CODEX_EVALUATION_MODEL`, `AKU_CODEX_PLANNING_EFFORT`,
`AKU_CODEX_EVALUATION_EFFORT`, `AKU_CODEX_PLANNING_POLICY`,
`AKU_CODEX_TIMEOUT_MS`, and `AKU_MISSING_SOURCE_TAB_POLICY`. Low-level process
overrides `AKU_BROWSER_PORT`, `AKU_DATABASE_PATH`, and `AKU_CODEX_PATH` are not
dashboard settings and should be reserved for packaging or explicit recovery.
The normal port is `47821`, database path is `runtime/aku-browser.db`, and
reasoning timeout is `120000` ms.

Gate 0B uses a fixed native-capture budget: at most two 75%-viewport scrolls, three snapshots, and 45 seconds. AkuBridge restores the applicable capture baseline and reports the actual movement in coverage. Computer Use is not an implicit fallback.

Every current capture also requires the generic `social-post-v1` quality
report. Sidecar pre-authorizes one same-candidate Bridge retry with a 300 ms
settle time, validates candidate/snapshot/coverage report consistency, and
rejects a final `retryable` result. `complete` and `usable_degraded` blocks are
admitted; `invalid` blocks are removed. If none remain, the source fails before
acquisition planning or final reasoning. `coverage.qualityAdmission` records
admitted, degraded, and rejected counts plus bounded issue/retry totals. The
ReasoningProvider never receives rejected parser output.

Media uses `media-recovery-v1` inside that same one-retry budget. Sidecar
requires a per-block outcome and aggregate coverage, verifies recovered media
against the allowlisted values, and rejects mismatched outcome/fallback counts.
If recovery is exhausted, trustworthy text remains `usable_degraded`; Source
layout shows a media-unavailable notice with the native-post link. Recovery
metadata is presentation/diagnostic context and is not sent to text reasoning.

When the initial acquisition cannot find the requested source, `open_missing_tab` lets AkuBridge create one inactive canonical feed tab (`https://x.com/home` or `https://www.linkedin.com/feed/`) and wait for it to load before capture. `fail_fast` preserves the earlier behavior. A follow-up round never opens a replacement tab because it must remain anchored to the original observation frontier.

Gate 0B.2 explicitly requests one allowlisted same-tab activation when `New posts`/`Show posts` is visible. Coverage distinguishes the pre-action position from the post-reveal baseline and never claims that the old feed view was restored.

Source freshness precedes Gate 0B.2 capture. AkuBridge applies one generic
`wake -> observe -> reveal/prove -> capture` state machine to both X and
LinkedIn, while each source adapter supplies only its versioned wake and
pending-control knowledge. Sidecar requires a ready `coverage.sourceFreshness`
outcome. A failed reveal stops at `source_freshness`; Sidecar does not retry a
stale feed under detect-only policy or present that failure as zero additions.

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

`GET /api/preferences/replay` provides historical dataset-maturity diagnostics. Automatic local fitting does not wait for these manual experiment gates. More, Neutral, and reason-aware Less events share an append-only ledger with calibration/routine origin and context metadata.

Selection Engine v1 owns generic materiality admission and the finite display budget. Preference Runtime v2 uses canonical source-neutral features only, keeps an active champion while evaluating a challenger, and applies confidence-scaled zero-to-two-position reranking without changing eligibility. Reset is durable suspension; only explicit manual refit resumes fitting.

`GET /api/preferences/benchmark` runs the local read-only replay benchmark. It reports polarity, source-sliced bias, selection, latency, token, model, and effort metrics without invoking a reasoning model. With Sidecar running, `npm run benchmark:engine` prints the same payload.

`GET /api/preferences/experiment` and `POST /api/preferences/experiment/fit` remain optional legacy-compatible diagnostics; they are not onboarding or production prerequisites.

## Unified Session API

- `POST /api/sessions` creates the bounded X + LinkedIn parent session.
- `GET /api/sessions/active` restores the latest non-terminal session after a page or Sidecar reload.
- `GET /api/sessions/{sessionId}` reconciles child status, starts the next sequential source when appropriate, and returns the persisted unified result.
- `POST /api/sessions/{sessionId}/cancel` cancels the active child and prevents queued sources from starting.

Existing `/api/runs` and bridge-command endpoints remain source-specific. The bridge still receives one run ID at a time.

`GET /api/operations/bridge/health` combines the latest successful adapter
observation with a rolling five-run terminal window per source. A successful
observation cannot hide newer failures: one recent failure below 90% completion
degrades the source, while two failures below 70% completion or a two-run
failure streak marks it unhealthy. The response exposes bounded counts and
timestamps, never captured post content or raw failure messages.

## Verification

```powershell
npm run check
npm run check:provider
npm run smoke:codex
```

`check:provider` is offline and quota-free. It validates the deterministic fallback against the same candidate coverage, provenance, planning, and telemetry envelope required from future local or open-source ReasoningProvider adapters. Passing this structural harness does not make the deterministic fallback suitable for pilot-quality ranking.

Local data operations are explicit and never delete pilot state:

```powershell
npm run db:health
npm run db:retention-preview -- --days 90
npm run db:backup -- --output C:\path\to\new-backup.db
npm run db:export -- --output C:\path\to\new-pilot-export.json
```

Backup uses SQLite `VACUUM INTO`, refuses to overwrite an existing target, and verifies the resulting database. The analysis export contains source content, candidate assessments, feedback, outcomes, and reasoning telemetry but excludes raw browser observations. Retention is preview-only; no delete operation exists.

AkuSidecar does not import AkuBridge source. Their only runtime dependency is the versioned localhost bridge contract.
