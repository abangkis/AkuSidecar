# AkuSidecar

AkuSidecar is the Go local runtime for AkuBrowser. It owns the loopback HTTP
API, embedded browser UI, fresh SQLite state, bounded X and LinkedIn session
engine, deterministic selection and preference policies, AkuBridge v2
contract, and replaceable Codex reasoning transport.

The Node implementation ended at tag `pre-refactor-2026-07-15`. This line has
no Node runtime, npm/Vite toolchain, historical database migration, or
backward-compatibility layer.

## Requirements

- Go 1.21 or newer
- Windows x64 for the current local Codex bundle
- a valid local Codex login for the managed Codex App Server
- AkuBridge `0.6.0` / `source-fidelity-v48`
- AkuSupervisor for normal development and daily lifecycle ownership

## Local Codex runtime

The Codex executable is deliberately not committed. Place the official native
Codex distribution at:

```text
runtime/codex-cli/bin/codex.exe
runtime/codex-cli/codex-path/
runtime/codex-cli/codex-resources/
```

`config/sidecar.json` points to that location. The default Go provider owns one
managed `codex app-server` stdio process, creates ephemeral read-only threads,
sends output schemas at turn start, rejects server callbacks, and stores
structured token telemetry. Candidate evaluation and semantic event resolution
are separate schema-bound adapters over this one transport.

An explicit model-capacity failure retries the same model once through a fresh
App Server process, inside the original invocation deadline. Cancellation,
timeout, validation errors, and hidden model fallback are not retryable.

## Build and test

On this Windows workspace, keep Go caches outside the module so antivirus and
module discovery do not interfere with the repository:

```powershell
$env:GOCACHE = "C:\WorkspaceCodex\AkuWorkspace\.go-cache\build"
$env:GOMODCACHE = "C:\WorkspaceCodex\AkuWorkspace\.go-cache\mod"
$env:GOTMPDIR = "C:\WorkspaceCodex\AkuWorkspace\.go-cache\tmp"

go test -p 1 ./...
go vet ./...
.\scripts\build-dev.ps1
```

`-p 1` avoids transient Windows executable-cleanup locks observed when multiple
test binaries finish concurrently.

## Development

AkuSupervisor directly owns `runtime\dev\aku-sidecar.exe`; there is no
component-level watcher or hidden replacement process. After a source change,
run the explicit rebuild/restart command:

```powershell
.\scripts\restart-dev.ps1
```

The command first builds `aku-sidecar.next.exe`, refuses to interrupt an active
session, asks AkuSupervisor to stop the registered service, atomically promotes
the candidate to `aku-sidecar.exe`, and asks AkuSupervisor to start it again.
Use `build-dev.ps1` alone when only a stopped binary needs to be built.
Restarting the service directly through AkuSupervisor never rebuilds embedded
UI or Go source and must not be used while an update is active.

For an isolated production-style run:

```powershell
.\runtime\dev\aku-sidecar.exe --config .\config\sidecar.json
```

Normal workspace operation is owned by AkuSupervisor. Its canonical service
profile starts `runtime\dev\aku-sidecar.exe` directly with the strict Sidecar
configuration and `--dev` during development.

## Configuration

`config/sidecar.json` is strict and versioned. Unknown properties fail startup.
Runtime flags may override only process-local concerns:

- `--config`
- `--database`
- `--provider`
- `--codex-path`
- `--port`
- `--dev`

There are no environment-based compatibility settings. Product settings are
typed, stored in SQLite, and changed through `GET/PUT /api/settings`.

Built-in bounded-load profiles remain:

| Profile | Native scrolls | Items/source | Session items | Timeline |
| --- | ---: | ---: | ---: | ---: |
| Standard | 2 | 5 | 10 | 12 |
| Expanded | 4 | 10 | 20 | 24 |
| Stress | 6 | 15 | 30 | 36 |

## Fresh database

The database defaults to `runtime/aku-sidecar.db`. Schema version 2 is created
as one transaction and contains only the active tables documented in
[`docs/go-rewrite-architecture.md`](docs/go-rewrite-architecture.md).

There is no importer for the Node database. A mismatched schema fails closed;
delete or move the development database and start again.

## Active API

- `GET /api/health`
- `GET /api/bootstrap`
- `GET/PUT /api/onboarding`
- `GET /api/calibration/active`
- `POST /api/calibration/sessions`
- `GET /api/calibration/sessions/{id}`
- `PUT /api/calibration/sessions/{id}/samples/{ordinal}`
- `GET/PUT /api/settings`
- `POST /api/sessions`
- `GET /api/sessions/active`
- `GET /api/sessions/{id}`
- `POST /api/sessions/{id}/cancel`
- `GET /api/inbox`
- `GET /api/runs/{id}`
- `GET /api/timeline`
- `POST /api/timeline/{id}/feedback`
- `GET /api/timeline/{id}/event-suggestions`
- `POST /api/timeline/{id}/event-correction`
- `POST /api/event-corrections/{id}/undo`
- `POST /api/bridge/heartbeat`
- `GET /api/bridge/health`
- `GET /api/bridge/commands/next`
- `POST /api/bridge/commands/{id}/observation`
- `POST /api/bridge/commands/{id}/failure`
- `POST /api/operations/bridge/actions/reload-self`
- `GET /api/operations/bridge/actions/next`
- `POST /api/operations/bridge/actions/{id}/accept`
- `GET /api/operations/bridge/actions/{id}`
- `POST /api/operations/reset-learning`
- `POST /api/operations/full-reset`

All Bridge heartbeat, command/result, and cooperative-action routes require both the durable Bridge token and
`X-Aku-Bridge-Contract: aku-browser.bridge.v2`.

The embedded UI restores the source-first dark shell, first-run source
onboarding, editable active sources, bounded custom capture controls, persisted
Source/Brief and stream-width preferences, media inspection, the finite
Timeline finish line, and the back-to-top control. Reset operations require an
exact typed phrase and fail while an update is active. A full reset creates and
verifies a timestamped SQLite backup before clearing the fresh Go state,
preserves the Bridge identity, and returns directly to onboarding.

First-time onboarding starts one bounded update to acquire real source
candidates, then opens a forced calibration lane before the Timeline. The lane
round-robins pre-selection X and LinkedIn candidates, accepts More, Neutral,
Less, or a capture issue for every sample, and fits the local preference model
when the batch is complete. AkuSidecar creates this calibration as part of the
completed/partial session boundary; bootstrap also repairs a persisted pending
state, so the flow does not depend on one frontend polling callback.

Before reasoning, bounded snapshots are reconciled by stable source identity.
For LinkedIn, a repeated long-form entry that first appears without a permalink
and later exposes a native ID is enriched into one evidence candidate instead
of entering calibration and Timeline twice.

The fresh preference mode is `guarded_live`. Direct user labels become the
highest-authority relevance signal once repeated evidence is sufficient. They
may promote, replace, demote, and suppress ordinary candidates, while evidence
quality, contradictions, material updates, and one bounded discovery lane stay
protected. Exact delivered evidence is excluded, semantic context without a
material delta is not re-added, and a valid update may finish with zero
additions. Completed source runs are composed into one global personalized
order with a diversity guard rather than strict X/LinkedIn round-robin.

A separate Event Engine groups selected cross-author and cross-source reports
after source runs finish. Go retrieves a global bounded shortlist from the
local event index; Codex App Server proposes only typed relationships. The
default collapses true duplicate reports while keeping them inspectable. Show
all bypasses the engine, and Hide removes duplicate reports from the Timeline.
Only `duplicate_report` is capacity-free; material updates, contradictions,
new consequences, and context remain unique. Automatic merging requires 0.92
confidence, and user corrections create undoable local constraints.

The resolver shortlist is locked to 5, 10, or 15 event threads. Event memory
uses paired age and storage boundaries: 30/60/90 days and
100/200/300/400/500 MB or 1 GB. The defaults are 30 days and 100 MB; crossing
either boundary trims the oldest terminal history and orphaned event threads.

## Removed by design

Offline experiments, shadow comparison, replay benchmarks, paired-model
benchmarking, pilot review, legacy reason aliases, historical
schema migrations, and hidden provider fallbacks were not ported. Reintroduce
any of them only as a new Go-native product decision with a current contract
and tests.
