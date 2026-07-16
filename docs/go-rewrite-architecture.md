# AkuSidecar Go boundary

Status: active runtime contract for `1.0.0-dev.5`.

AkuSidecar was rewritten in place as one Go application. Tag `pre-refactor-2026-07-15` is the complete Node rollback boundary. The active line has no Node runtime, npm toolchain, historical migration chain, or API compatibility layer.

## Ownership

- `cmd/akusidecar` starts the loopback server.
- `internal/httpapi` serves the API and embedded UI.
- `internal/engine` owns bounded sessions, capture commands, reasoning, calibration, and finalization.
- `internal/store` owns the fresh SQLite v2 schema.
- `internal/selection` owns generic trust/materiality admission, high-authority personalization, protected updates, exact-evidence exclusion, and the discovery lane.
- `internal/preference` fits the rebuildable local profile from canonical direct signals.
- `internal/reasoning` owns the managed Codex App Server and explicit Codex Exec conformance transport.
- AkuSupervisor starts and stops `runtime/dev/aku-sidecar.exe` directly.

## Product invariants

- AkuBridge capture is read-only, source-specific, and bounded.
- Model output describes every candidate but cannot navigate, expand budgets, or select the Timeline.
- Direct user feedback outranks source-platform order once repeated evidence is sufficient.
- Trust protections outrank preference: an evidence-qualified material update or contradiction cannot be suppressed.
- One qualified discovery candidate remains available per source when it does not displace a protected update.
- No fallback item is fabricated. Zero additions is valid.
- X and LinkedIn are composed into one global personalized order with a maximum-two-consecutive-source guard.
- AkuSidecar never launches a watcher or hidden replacement of itself.

## State

SQLite schema version 2 contains only the active fifteen tables for metadata, settings, sessions/runs, Bridge commands/observations, reasoning telemetry, assessments/Timeline, calibration, feedback/model state, and knowledge events. Mutable bounded payloads use JSON; lifecycle, integrity, and ordering fields remain typed columns.

There is no importer for the Node database. Schema mismatch fails startup. Full reset is idle-only, creates and verifies a SQLite backup, clears product state, restores strict defaults, and preserves Bridge identity.

## Verification

```powershell
go test -p 1 ./...
go vet ./...
.\scripts\build-dev.ps1
```

The workspace integration gate is `AkuBrowser/scripts/check.ps1`. Runtime replacement is explicit through `scripts/restart-dev.ps1`, which refuses to interrupt an active session and delegates stop/start to AkuSupervisor.
