# AkuSidecar Go rewrite

Status: implementation baseline for the `go-rewrite` branch.

## Decision

AkuSidecar is rewritten in place as a Go application. The
`pre-refactor-2026-07-15` tag is the complete Node-era rollback boundary. A
separate `AkuSidecarGo` repository would duplicate component identity, paths,
Supervisor registration, and release ownership without providing additional
safety.

This rewrite deliberately has no database, API-alias, configuration, or
reason-code compatibility with the Node implementation. The current product
behavior is re-expressed as a smaller new contract.

## Product invariants

- The service binds only to loopback and owns the local AkuBrowser UI.
- AkuBridge is read-only and receives bounded, versioned capture commands.
- X and LinkedIn are collected as one finite session, never an infinite feed.
- Capture admission remains truthful: trustworthy text may survive as
  `usable_degraded`, while missing evidence is never fabricated.
- Selection remains generic; preference may rerank and may fill one unused
  source slot, but default authority cannot hide or replace an admitted item.
- Source evidence is untrusted input. Reasoning runs read-only, offline, with
  approvals disabled and a required structured-output schema.
- AkuSupervisor owns the visible process lifecycle. AkuSidecar never starts a
  hidden replacement of itself.

## Kept surface

- loopback HTTP health, bootstrap, settings, session, run, timeline, feedback,
  Bridge heartbeat/command/result, and cooperative Bridge reload operations;
- bounded-load profiles (`standard`, `expanded`, `stress`, `custom`);
- X then LinkedIn unified sessions and capture-lease cleanup;
- source freshness, visibility, quality, media-degradation, and continuation
  evidence recorded as coverage;
- Selection Engine, Preference Runtime v2, unused-budget promotion, knowledge
  continuity, and reasoning telemetry;
- browser JavaScript/CSS served as embedded static assets. These assets run in
  Chrome and do not introduce a Node runtime or toolchain.

## Removed surface

- Node.js, npm, package-lock, Vite, MJS scripts, and Node tests;
- historical SQLite migrations and all existing runtime rows;
- calibration sessions and calibration snapshots;
- offline preference experiments, shadow comparisons, replay benchmarks,
  paired-model benchmarks, and pilot-review endpoints;
- `wrong_topic` and every other legacy reason alias;
- legacy environment-based settings and compatibility shims;
- inactive provider aliases and shadow eligibility paths.

## New component contract

- Application version begins at `1.0.0-dev.1`.
- Bridge contract is `aku-browser.bridge.v2`.
- The database schema is a single version-1 transaction. A schema mismatch is
  a startup error; it is never migrated automatically.
- Settings are served by `GET/PUT /api/settings`.
- New sessions use `POST /api/sessions`; session/run resources and Timeline are
  read-only except for cancellation and feedback.
- Bridge routes remain a dedicated token-authenticated namespace because the
  extension, not the browser UI, owns them.

## Go layout

```text
cmd/akusidecar/       production server
cmd/akuwatch/         zero-Node build/restart development loop
internal/config/      typed file and flag configuration
internal/domain/      contract types and validation
internal/store/       fresh SQLite schema and persistence
internal/engine/      session/run orchestration
internal/selection/   deterministic materiality selection
internal/preference/  bounded personalized reranking
internal/reasoning/   provider interface, Codex Exec, later App Server
internal/httpapi/     loopback API, security, embedded UI
schemas/              structured reasoning output contracts
```

## Reasoning transition

The provider interface is turn-oriented and transport-neutral. The default
provider owns one `codex app-server` stdio process, initializes the generated
JSON-RPC v2 protocol once, creates ephemeral read-only threads, constrains each
turn with its output schema, and records token notifications. A native live
smoke passed against Codex CLI 0.144.1.

`codex-exec` retains the former SDK-equivalent process-per-request boundary as
an explicit conformance transport; it is not the normal runtime provider.

## Fresh SQLite schema

The new database contains only:

1. `meta`
2. `settings`
3. `sessions`
4. `runs`
5. `bridge_commands`
6. `observations`
7. `reasoning_invocations`
8. `candidate_assessments`
9. `timeline_items`
10. `feedback_events`
11. `preference_model`
12. `knowledge_events`

Mutable domain payloads are stored as canonical JSON at bounded seams. Fields
used for lifecycle, ordering, filtering, or integrity remain typed columns.

## Cutover gates

1. `go test ./...`, `go vet ./...`, and a Windows production build pass.
2. The project contains no package manifest, Node source, Vite dependency, or
   Node command in its development and production workflows.
3. A fresh database starts, restarts, and resets deterministically.
4. AkuBridge v2 completes X and LinkedIn capture with truthful coverage.
5. One real Codex Exec acquisition/evaluation turn completes under Supervisor.
6. Timeline, feedback, preference reranking, cancellation, lease cleanup, and
   Bridge cooperative reload pass live validation.
7. AkuSupervisor starts the Go executable directly and reports it healthy.
