# AkuSidecar Go boundary

Status: active runtime contract for `1.0.0-dev.10`.

AkuSidecar was rewritten in place as one Go application. Tag `pre-refactor-2026-07-15` is the complete Node rollback boundary. The active line has no Node runtime, npm toolchain, historical migration chain, or API compatibility layer.

## Ownership

- `cmd/akusidecar` starts the loopback server.
- `internal/httpapi` serves the API and embedded UI.
- `internal/engine` owns bounded sessions, capture commands, reasoning, calibration, and finalization.
- `internal/store` owns the active SQLite v4 schema and the narrow current-Go v2/v3-to-v4 migration.
- `internal/selection` owns generic trust/materiality admission, high-authority personalization, protected updates, exact-evidence exclusion, and the discovery lane.
- `internal/preference` fits the rebuildable local profile from canonical direct signals.
- `internal/reasoning` owns the single managed Codex App Server transport and candidate-evaluation adapter.
- `internal/eventengine` owns bounded event retrieval, the separate App Server resolver, high-precision merging, and safe degradation.
- `internal/aidetector` owns deterministic text-first Fast Detection and the separate asynchronous App Server resolver. It does not own selection or the generic side-pane UI primitive.
- AkuSupervisor starts and stops `runtime/dev/aku-sidecar.exe` directly.

Accepted observations are the durable boundary between browser acquisition and
Codex reasoning. AkuSidecar writes the bounded capture coverage to the run in
the same SQLite transaction that accepts an observation. A graceful restart
pauses an in-flight reasoning run without converting it to failure; the next
process resumes evaluation from the accepted observation without recapturing
or repeating acquisition planning.

## Product invariants

- AkuBridge capture is read-only, source-specific, and bounded.
- Model output describes every quality-admitted evidence candidate but cannot navigate, expand budgets, or select the Timeline.
- Direct user feedback outranks source-platform order once repeated evidence is sufficient.
- Trust protections outrank preference: an evidence-qualified material update or contradiction cannot be suppressed.
- One qualified discovery candidate remains available per source when it does not displace a protected update.
- No fallback item is fabricated. Zero additions is valid.
- X and LinkedIn are composed into one global personalized order with a maximum-two-consecutive-source guard.
- Only a `duplicate_report` that reaches the configured confidence gate is capacity-free; related updates, contradictions, consequences, and context remain unique. The gate defaults to `0.92` and is bounded to `0.85–0.95` in `0.01` steps.
- Semantic resolution is conditional: noisy lexical overlap cannot trigger the model, and unrelated reports use a deterministic local fast path.
- `show_all` bypasses event retrieval and resolution. User event corrections are local, persistent, and undoable.
- AI origin signals are presentation metadata only. Fast Detection runs after final composition, Deep Detection runs after Timeline delivery, and neither may change admission, order, event membership, or capacity.
- AI Detector binds every assessment to `assessedObject=social_post` and a typed signal scope. AI provenance for a quote, attached media, or an external artifact cannot be transferred into a strong social-post assessment.
- Deep Detection spends Codex only on retained posts whose assessment can still change. It skips inadequate text, direct platform/provenance evidence, and active user corrections, and uses a separate Terra `medium` model profile from high-effort selection evaluation.
- A Deep correction never silently removes an earlier strong badge. Direct platform/provenance evidence remains explicit, and the latest active user correction has the highest personal presentation authority.
- Inline is the default. Drawer never abruptly removes a post already seen inline. Hide requires exact typed confirmation and accepts only direct evidence, Deep-confirmed strong signals, or an explicit user AI verdict—not preliminary inference.
- Media recapture is item-scoped and quiet-first. A foreground attempt requires an unavailable background result plus explicit one-time user consent; neither path creates candidates or changes Timeline ordering.
- Media acquisition is one generic Bridge engine shared by every source adapter. Adapters declare media kinds, source-specific extractors, and visibility capability; quiet X recapture exhausts primary, structured-state, hydration, and alternate-DOM paths before requesting foreground permission.
- Capture telemetry survives reasoning failure or process interruption. A failed model turn cannot erase the already accepted browser coverage.
- AkuSidecar never launches a watcher or hidden replacement of itself.

## State

SQLite schema version 4 contains only active tables for metadata, settings, sessions/runs, Bridge commands/observations, reasoning telemetry, assessments/Timeline, append-only object-scoped AI assessment history and asynchronous jobs, item-scoped media recaptures and evidence overrides, calibration, feedback/model state, source-scoped knowledge, semantic event reports/constraints/corrections, and resolver/trigger telemetry. Mutable bounded payloads use JSON; lifecycle, integrity, and ordering fields remain typed columns.

Semantic event memory is bounded by both age and total SQLite footprint. Cleanup runs on startup, Settings save, and terminal-session finalization. The default is 30 days or 100 MB, whichever is reached first.

There is no importer for the Node database. Only current-Go v2 and v3 receive the narrow additive migrations into v4; every other schema mismatch fails startup. Full reset is idle-only, creates and verifies a SQLite backup, clears product state, restores strict defaults including Standard 1x and Inline AI signals, and preserves Bridge identity.

## Verification

```powershell
go test -p 1 ./...
go vet ./...
.\scripts\build-dev.ps1
```

The authenticated AI Detector acceptance canary is opt-in so ordinary tests never consume Codex:

```powershell
$env:AKU_CODEX_LIVE = "1"
go test -v -run TestLiveCodexAppServerAIDetectionAcceptanceCanary -count=1 ./internal/aidetector
```

The workspace integration gate is `AkuBrowser/scripts/check.ps1`. Runtime replacement is explicit through `scripts/restart-dev.ps1`, which refuses to interrupt an active session and delegates stop/start to AkuSupervisor.
