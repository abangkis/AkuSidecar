# AkuSidecar

AkuSidecar is the local AkuBrowser runtime. It owns the pinned local UI, HTTP API, SQLite state, bounded job engine, provider-neutral browser-capture policy, and replaceable reasoning-provider adapters.

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

Gate 0B.1 uses a fixed native-capture budget: at most two 75%-viewport scrolls, three snapshots, and 45 seconds. AkuBridge restores the starting position and reports the actual movement in coverage. Computer Use is not an implicit fallback.

Coverage also distinguishes a detected platform fresh-content signal from an activated one. Gate 0B.1 records pending `New posts`/`Show posts` signals but does not click them.

## Verification

```powershell
npm run check
npm run smoke:codex
```

AkuSidecar does not import AkuBridge source. Their only runtime dependency is the versioned localhost bridge contract.
