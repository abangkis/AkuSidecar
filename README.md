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

For the production-style static server without file watching:

```powershell
npm start
```

## Configuration

- `AKU_BROWSER_PORT` defaults to `47821`.
- `AKU_REASONING_PROVIDER` is `deterministic` or `codex-sdk`.
- `AKU_DATABASE_PATH` overrides the local SQLite path.
- `AKU_CODEX_PATH` overrides the packaged Codex CLI path.
- `AKU_CODEX_TIMEOUT_MS` defaults to `120000`.

Gate 0B uses a fixed native-capture budget: at most two 75%-viewport scrolls, three snapshots, and 45 seconds. AkuBridge restores the applicable capture baseline and reports the actual movement in coverage. Computer Use is not an implicit fallback.

Gate 0B.2 explicitly requests one allowlisted same-tab activation when `New posts`/`Show posts` is visible. Coverage distinguishes the pre-action position from the post-reveal baseline and never claims that the old feed view was restored.

Gate 0B.3 asks the configured ReasoningProvider only whether to finish or request one adjacent observation. A follow-up is capped at one scroll, locked to the same source, anchored to the last round-one viewport, and cannot activate fresh-content controls. Both rounds are stored and merged before the final result.

## Knowledge continuity

Every validated evidence block receives a deterministic identity. A completed run advances one checkpoint for its source and mode; only evidence previously delivered as a result is suppressed on later runs. New semantic deltas are attached to stable event keys and stored as append-only versions.

The current frontier is inspectable through:

- `GET /api/knowledge?source=x&mode=catch_up`
- `GET /api/knowledge/events/{eventKey}?source=x&mode=catch_up`

## Verification

```powershell
npm run check
npm run smoke:codex
```

AkuSidecar does not import AkuBridge source. Their only runtime dependency is the versioned localhost bridge contract.
