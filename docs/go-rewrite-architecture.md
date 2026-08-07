# AkuSidecar Go boundary

Status: active runtime contract for `0.7.9`.

AkuSidecar was rewritten in place as one Go application. Tag `pre-refactor-2026-07-15` is the complete Node rollback boundary. The active line has no Node runtime, npm toolchain, historical migration chain, or API compatibility layer.

## Ownership

- `cmd/akusidecar` starts the loopback server.
- `internal/httpapi` serves the API and embedded UI.
- `internal/engine` owns bounded sessions, capture commands, reasoning, calibration, and finalization.
- `internal/store` owns the active SQLite v5 schema and rejects every other existing schema before mutating application tables.
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
- Source adapters declare their content family and supported evidence modalities. Generic Bridge admission, not an adapter-specific text-length rule, requires an author, stable identity, and at least one text, image, video, attachment, or quoted-post modality. AkuSidecar independently rejects identity-only evidence and resource or native-URL violations.
- A source descriptor may declare a bounded follow-up planning policy. Facebook and LinkedIn use the generic `local_frontier` capability: after at least one completed scroll produces zero new candidates and no explicit `has more` signal, AkuSidecar may finish acquisition locally instead of spending a model turn on a no-op follow-up decision. A descriptor may also declare an `initialRoundCandidateTarget`; LinkedIn uses a target of one so a complete, non-deadline-exhausted first round that already found one admissible candidate finishes the current finite check and defers older-feed coverage to a later check. Incomplete, degraded, deadline-exhausted, empty, or ambiguous LinkedIn evidence still falls back to the existing model planner. Candidate evaluation remains model-backed, and LinkedIn's continuation-overlap requirement remains unchanged when a follow-up round is actually requested.
- Native-content continuity promotes a strict fallback identity to a later stable native identity across sessions only when source, normalized author, and full normalized text agree. Short or missing text is not eligible. Content kind and exact publication time must also remain compatible. Source-relative timestamps marked as estimated are treated as unavailable; when either exact timestamp is unavailable, fallback reuse is bounded to 30 minutes. A stable platform ID is authoritative over harmless permalink spelling or tracking-query differences, while a canonical permalink remains authoritative when no platform ID is available. Two observations that expose different valid native identities are recorded as a conflict and never merged, even when their text matches. This promotion prevents a long-form LinkedIn post from spending acquisition and candidate-evaluation tokens again merely because its permalink became available later, without conflating an author's later repost; it is a generic Sidecar rule rather than LinkedIn adapter logic.
- Update Inbox projects one collapsed `Acquisition & identity` receipt alongside media-acquisition outcomes already stored for the run. It distinguishes local-frontier completion from model planning, reports follow-up yield, and counts native identity present, fallback identity, alias reuse, successful fallback-to-native promotion, identity conflict, and ambiguous fallback. Reading this telemetry adds no capture round and consumes no reasoning tokens.
- Model output describes every quality-admitted evidence candidate but cannot navigate, expand budgets, or select the Timeline.
- Direct user feedback outranks source-platform order once repeated evidence is sufficient.
- Trust protections outrank preference: an evidence-qualified material update or contradiction cannot be suppressed.
- One qualified discovery candidate remains available per source when it does not displace a protected update.
- No fallback item is fabricated. Zero additions is valid.
- Every active registered source is composed into one global personalized order with a maximum-two-consecutive-source guard.
- Event membership and information novelty are independent. A report may belong to an existing event while remaining a `material_update`; only `duplicate_report` means the factual payload is already represented by the canonical claim, a retained delta, or an earlier candidate in the same resolver turn.
- Every event retains a bounded local delta ledger of at most eight recent source-backed facts. Resolver prompts receive only the three newest compact deltas and process candidates sequentially, so repeated cross-author reports do not become false updates merely because they arrived in the same check. A one-time idempotent schema-7 backfill preserves each retained report's original chronology rather than stamping historical deltas with Sidecar startup time.
- Only a `duplicate_report` that reaches the configured confidence gate is capacity-free; related updates, contradictions, consequences, and context remain unique. Duplicate audit rows are retained under separate total and per-event bounds, so they never reduce the configured unique-information capacity. The gate defaults to `0.92` and is bounded to `0.85–0.95` in `0.01` steps.
- Semantic resolution is conditional: noisy lexical overlap cannot trigger the model, and unrelated reports use a deterministic local fast path.
- `show_all` bypasses event retrieval and resolution. User event corrections are local, persistent, and undoable. The correction contract records event membership separately from novelty, allowing `not the same event`, `same event · repeated report`, and `same event · meaningful update` without conflating those decisions.
- AI origin signals are presentation metadata only. Fast Detection runs after final composition, Deep Detection runs after Timeline delivery, and neither may change admission, order, event membership, or capacity. Image provenance is owned by a separate asynchronous media-provenance component backed by local `c2patool`: it begins after delivery, accepts only adapter-declared image hosts, keeps C2PA trust and AI-origin claims object-scoped to attached media, and never consumes reasoning tokens. Direct C2PA AI-media provenance routes the parent item to the AI Signals drawer until the user records a higher-authority personal correction.
- AI Detector binds every assessment to `assessedObject=social_post` and a typed signal scope. AI provenance for a quote, attached media, or an external artifact cannot be transferred into a strong social-post assessment.
- Deep Detection spends structured inference on a deterministic shortlist of at most five retained posts whose assessment can still change. Explicit personal `Unsure` feedback is ranked first, preliminary strong Fast findings follow, and explicit but phrasing-ambiguous authorship or agent-identity contexts may use remaining capacity. Style alone is never eligible. It skips inadequate text, direct platform/provenance evidence, ordinary neutral posts, and active personal AI/not-AI verdicts, and uses its own profile rather than inheriting candidate evaluation. The offline source-controlled corpus measures shortlist routing against positive and negative controls without consuming model tokens. Each model-backed process resolves an opaque, user-selectable profile through the active provider's bounded catalog; only candidate evaluation defaults to Luna `max`, while acquisition, semantic resolution, and AI Deep default to Luna `high`. The current backend is Codex App Server, but the domain contract, optional executable-runtime capability, and Settings renderer are provider-neutral.
- A Deep correction never silently removes an earlier strong badge. Direct platform/provenance evidence remains explicit, while deterministic Personal AI Policy gives scoped user feedback the highest personal presentation authority without entering selection.
- Drawer is the preview default and never abruptly removes a post already seen inline. Inline remains selectable. Hide requires exact typed confirmation and accepts only direct evidence, Deep-confirmed strong signals, or an explicit user AI verdict—not preliminary inference.
- Media recapture is item-scoped and quiet-first. A foreground attempt requires an unavailable background result plus explicit one-time user consent; neither path creates candidates or changes Timeline ordering.
- Media acquisition is one generic Bridge engine shared by every source adapter. Adapters declare media kinds, source-specific extractors, and visibility capability; quiet X recapture exhausts primary, structured-state, hydration, and alternate-DOM paths before requesting foreground permission.
- A bounded passive X media cache may complete presentation evidence after the Timeline is already usable. Its inputs include `x-response-evidence-v2`, which transiently inspects only already-requested X timeline/detail responses and exports no raw response or text. Sidecar revalidates the authoritative post identity and media allowlist, preserves `x_response_graphql` provenance in `passive-x-media-enrichment-v2`, records an evidence override, and never creates a browser job, reasoning call, candidate, or foreground action for this path. The same adapter can fill an unhydrated author avatar through a separate Bridge-only ephemeral cache; avatar evidence never reaches Sidecar as post media.
- Capture telemetry survives reasoning failure or process interruption. A failed model turn cannot erase the already accepted browser coverage.
- Candidate reasoning receives bounded media metadata (kind, alt text, dimensions, and provenance) without media URLs. A text-empty media post remains a candidate, but the evaluator must report visual limitations and may not invent details it was not given.
- AkuSidecar never launches a watcher or hidden replacement of itself.

## State

SQLite schema version 7 contains only active tables for metadata, source definitions, settings, sessions/runs, Bridge commands/observations, reasoning telemetry, durable assessments for every evaluated candidate, Timeline and append-only selection corrections, detector-owned AI assessment history and asynchronous jobs, canonical append-only personal AI feedback, item-scoped media recaptures and evidence overrides (including passive-enrichment provenance), calibration, preference/model state, source-scoped knowledge, native-content continuity and identity promotion, semantic event reports, bounded event deltas, membership constraints, novelty constraints, corrections, and resolver/trigger telemetry. Source-bearing rows reference the application registry through `source_definitions`; adding a source no longer requires editing every table constraint. Mutable bounded payloads use JSON; lifecycle, integrity, and ordering fields remain typed columns.

Bridge claims are leased rather than permanent. The lease is computed from the
command's bounded hydration and capture budgets plus a fixed grace window.
Heartbeat and command polling recover an expired claim as a retryable source
failure and release the session-wide capture lane for the next source.
Append-only AI feedback uses SQLite insertion order as its causal ordering;
timestamps are display telemetry and random IDs are never used as a
last-write-wins authority.

Semantic event memory is bounded by both age and total SQLite footprint. Cleanup runs on startup, Settings save, and terminal-session finalization. The default is 30 days or 100 MB, whichever is reached first.

There is no importer or migration path for an earlier Node or Go database. Schema v7 is the only accepted runtime contract; an existing database with any other schema version fails before AkuSidecar creates or alters application tables. The v7 boundary separates detector-owned `ai_assessments` from the user-owned `ai_feedback_events` ledger. Its deterministic Personal AI Policy may affect badge/drawer presentation and Deep-review priority only; preference fitting and selection do not read it. Reset learning preserves the historical Timeline/audit decision but clears preference and personal AI evidence; full reset is idle-only, creates and verifies a SQLite backup, clears product state, restores the current defaults including Standard 1x, Progressive wait, Smart resurfacing with a seven-day cooldown, Drawer AI signals, Luna High for acquisition/semantic/AI Deep, and Luna XHigh only for candidate evaluation, and preserves Bridge identity.

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

The offline acceptance suite also covers the object-scoped AI-origin contract: platform labels and direct C2PA AI provenance may route attached media to AI Signals, while quoted/external evidence, invalid manifests, missing manifests, and valid non-AI manifests remain inline. It does not consume Codex tokens:

```powershell
go test -count=1 ./internal/aidetector ./internal/mediaprovenance ./internal/store ./internal/httpapi
```

The workspace integration gate is `AkuBrowser/scripts/check.ps1`. Runtime replacement is explicit through `scripts/restart-dev.ps1`, which refuses to interrupt an active session and delegates stop/start to AkuSupervisor.
