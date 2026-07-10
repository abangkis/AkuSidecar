# AkuSidecar

AkuSidecar is the local AkuBrowser runtime. It owns the pinned local UI, HTTP API, SQLite state, bounded job engine, and replaceable reasoning-provider adapters.

## Requirements

- Node.js 24 or newer
- A local Codex login when using the `codex-sdk` provider

## Run

```powershell
npm install
$env:AKU_REASONING_PROVIDER='codex-sdk'
npm start
```

Open `http://127.0.0.1:47821` in the same Chrome profile where AkuBridge is loaded.

## Configuration

- `AKU_BROWSER_PORT` defaults to `47821`.
- `AKU_REASONING_PROVIDER` is `deterministic` or `codex-sdk`.
- `AKU_DATABASE_PATH` overrides the local SQLite path.
- `AKU_CODEX_PATH` overrides the packaged Codex CLI path.
- `AKU_CODEX_TIMEOUT_MS` defaults to `120000`.

## Verification

```powershell
npm run check
npm run smoke:codex
```

AkuSidecar does not import AkuBridge source. Their only runtime dependency is the versioned localhost bridge contract.
