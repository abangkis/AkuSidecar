# AkuSidecar

Current release candidate: **`0.8.0`**.

AkuSidecar is the Go local runtime for AkuBrowser. It owns the loopback HTTP
API, embedded browser UI, fresh SQLite state, bounded X, LinkedIn, Facebook, and Instagram session
engine, deterministic selection and preference policies, AkuBridge v2
contract, and replaceable Codex reasoning transport.

The Node implementation ended at tag `pre-refactor-2026-07-15`. This line has
no Node runtime, npm/Vite toolchain, historical database migration, or
backward-compatibility layer.

## OpenAI Build Week role

During OpenAI Build Week, AkuSidecar was rewritten from Node.js to Go and moved
from the Codex SDK to one managed Codex App Server process. GPT-5.6 now serves
four schema-bound roles: Acquisition Planning, Candidate Evaluation,
cross-source Semantic Event Resolution, and asynchronous AI Deep Detection.
Codex also accelerated the rewrite, preference and semantic engines, recovery
work, tests, documentation, and release preparation.

AkuSidecar remains the application authority. It owns browser budgets,
untrusted-evidence separation, response validation, preference eligibility,
SQLite state, continuity, correction rules, and final Timeline composition.

## Optional C2PA image provenance

AI Detector can inspect captured images asynchronously with the official
`c2patool` executable. Discovery checks `AKU_C2PATOOL_PATH`, the directory
beside AkuSidecar, and then `PATH`. AkuSidecar remains operational when the
tool is unavailable; only C2PA image provenance is skipped. The adapter is
image-only, uses embedded manifests without remote-manifest or OCSP fetching,
and never treats a missing manifest as proof that media is human-created.
See the
[final project story](https://github.com/abangkis/AkuBrowser/blob/main/docs/openai-build-week-submission.md)
and [Build Week evidence](https://github.com/abangkis/AkuBrowser/blob/main/BUILD_WEEK.md).

## Requirements

- Go 1.21 or newer
- Windows x64 or macOS x64/arm64 for the current portable preview
- a valid local Codex login for the managed Codex App Server
- AkuBridge `0.8.0` / `source-adapters-v102`
- AkuSupervisor is recommended for normal Windows development and daily
  lifecycle ownership; it is not part of the portable runtime or a macOS
  prerequisite

## Local Codex runtime

The Codex executable and every generated runtime artifact are deliberately not
committed. A developer may keep a temporary native Codex distribution under:

```text
runtime/codex-cli/bin/codex.exe
runtime/codex-cli/codex-path/
runtime/codex-cli/codex-resources/
```

That directory is ignored as a whole and is never a source dependency.
`config/sidecar.json` leaves the executable unset, so development and packaged
runtimes discover an explicit `--codex-path`,
`AKU_CODEX_PATH`, `PATH`, managed Codex App runtimes, and common platform CLI
locations in that order. `AkuSidecar --discover-codex` exposes the same JSON
probe to launchers and installers, and accepts a candidate only after its
`app-server` capability succeeds. When a Codex App exposes multiple managed
runtimes, discovery selects the highest semantic version and uses file time
only as a tie-breaker; the stable app `bin/codex` entry remains a fallback. The
`0.8.0` package still assumes
the discovered installation is locally signed in; login assistance is
deferred. Settings shows the resolved full executable path beside the
Reasoning processes. A replacement path is provider-validated before it is
stored and hot-swapped into the shared runtime only while reasoning is idle;
**Use detected** reruns the same bounded platform discovery without saving the
result automatically. The default Go provider owns one managed `codex app-server` stdio
process, creates ephemeral read-only threads,
sends output schemas at turn start, rejects server callbacks, and stores
structured token telemetry. On Windows, each managed App Server is assigned to
its own Job Object so forced timeout/recycle also cleans its descendant tool
processes; the Supervisor remains the outer ownership boundary. Acquisition planning, semantic event resolution,
and AI Deep Detection default to Luna `high`; candidate evaluation also defaults
to Luna `high` so routine checks stay within the bounded reasoning deadline.
Luna XHigh and Luna Max remain explicit tuning options. Deep Detection runs only after Timeline delivery, while
local deterministic AI Fast Detection does not consume a model. The domain
adapters depend on a generic structured-inference contract rather than the
Codex transport, so another backend can replace App Server without changing
their schemas or authority rules. Settings exposes the active provider, model,
effort, and execution phase for each process. Each process can be tuned for the
next invocation through a backend-owned bounded catalog: Luna High, Luna XHigh,
Luna Max, Terra High, Terra XHigh, or Sol Medium. Free-form model IDs are never
accepted.

An explicit model-capacity failure retries the same model once through a fresh
App Server process, inside the original invocation deadline. Cancellation,
timeout, validation errors, and hidden model fallback are not retryable.

## Optional pinned-Chromium app shell

`--app-shell` opens AkuSidecar's UI inside a locally pinned Chromium window in
app mode, turning the loopback server into a desktop-like application without
changing the acquisition or reasoning architecture. Discovery checks the
explicit `--chromium-path`, `AKU_CHROMIUM_PATH`, `PATH`, the sidecar-relative
`runtime/chromium/bin` directory, and known installed locations, in that
order, and accepts a candidate only after its `--version` capability probe
succeeds; `--discover-chromium` exposes the same JSON probe to launchers.
Closing the window requests a graceful Sidecar shutdown, and on Windows the
window belongs to its own Job Object so Sidecar exit also closes descendant
renderer processes. `--bridge-extension-path` loads an unpacked AkuBridge
directory into the window through `--load-extension`; the browser profile is
separate from any user Chrome profile and persists under
`runtime/app-profile`. `scripts/fetch-chromium.ps1` downloads a pinned
Chrome-for-Testing build into `runtime/chromium` and records its version,
source URL, and SHA-256 in `pin.json`; the pinned engine is a local runtime
artifact and is never committed. Engine freshness is the application's own
responsibility: the recorded pin is the patch channel, and a stale pin is a
product defect rather than an operating-system concern. An automatic engine
patch path that swaps `runtime/chromium` independently of application releases
is a deliberate nice-to-have and is deferred until after the first packaged
app-shell release.

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

### Inference SDK dependency

`go.mod` requires `github.com/abangkis/ai4u-inference-sdk-go` at the published
`v0.7.0` tag and `github.com/abangkis/ai4u-common-execution-profile-go` at
`v0.1.0` on GitHub; there is no local `replace` directive. The exact module
hash is locked in `go.sum`. Bump the version when a newer SDK release is needed.
Because the module is private, `GOPRIVATE=github.com/abangkis/*` must be set so
`go` fetches directly from GitHub instead of the public proxy and checksum DB.

The legacy inference model fields `model` (provider wire-model name) and
`effort` are deprecated as of AkuSidecar v0.8.0. New configuration must use the
provider-owned stable `modelId`, client-owned `minReasoningTier`, and optional
exact `reasoningOptionId`. v0.8.5 is the migration-warning milestone: remaining
legacy values should produce actionable diagnostics while still loading. The
aliases are scheduled for removal with the breaking configuration contract in
v0.9.0; configurations that still use them will then fail validation.

## Development

AkuSupervisor directly owns `runtime\dev\aku-sidecar.exe`; there is no
component-level watcher or hidden replacement process. From a stopped
AkuWorkspace—including after deleting the generated `runtime` directory—start
the development stack through the workspace bootstrap:

```powershell
cd ..\AkuSupervisor
.\scripts\dev.ps1 akusidecar
```

The bootstrap resolves AkuBrowser's `development` profile from the single
Bridge identity registry, writes its exact origin as a generated argument in
the active Supervisor configuration, and performs the incremental Go build
before the generic Supervisor validates its service configuration. The
checked-in Sidecar base configuration intentionally contains no trusted Bridge
origin. After a source change while the stack is
already running, use the explicit rebuild/restart command from AkuSidecar:

```powershell
.\scripts\restart-dev.ps1
```

The command first builds `aku-sidecar.next.exe`, refuses to interrupt an active
session or other runtime-owned background work, waits up to 15 minutes by
default for update readiness, asks AkuSupervisor to stop the registered
service, atomically promotes the candidate to `aku-sidecar.exe`, and asks
AkuSupervisor to start it again.
Every development build writes an adjacent
`aku-sidecar.exe.runtime-state.json` provenance receipt containing the
application version, source commit, dirty state, build time, and binary
SHA-256. Candidate provenance is promoted atomically with the executable, and
the restart succeeds only after the new health endpoint reports the recorded
version.
Use `build-dev.ps1` alone when only a stopped binary needs to be built.
Restarting the service directly through AkuSupervisor never rebuilds embedded
UI or Go source and must not be used while an update is active.

For an isolated direct development run, derive the exact origin from the same
registry rather than copying an ID into another file:

```powershell
$identity = Get-Content ..\AkuBrowser\config\bridge-identities.json -Raw |
  ConvertFrom-Json |
  Select-Object -ExpandProperty profiles |
  Select-Object -ExpandProperty development
$origin = "chrome-extension://$($identity.extensionId)/"
.\runtime\dev\aku-sidecar.exe --config .\config\sidecar.json `
  --bridge-extension-origin $origin
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
- `--chromium-path`
- `--bridge-extension-path`
- `--app-shell`
- `--discover-chromium`
- `--port`
- `--dev`
- `--bridge-extension-origin`

`--bridge-extension-origin` is a launcher-owned trust projection. Normal
development receives it from `AkuSupervisor\scripts\dev.ps1`; production
packages receive it from AkuBrowser release tooling. It is not a user-tunable
product setting and must resolve to one exact identity declared by
`AkuBrowser\config\bridge-identities.json`.

There are no environment-based compatibility settings. Product settings are
typed, stored in SQLite, and changed through `GET/PUT /api/settings`.

`config/sidecar.json` declares every inference backend under
`reasoning.providers` and picks the first-boot default with
`reasoning.activeProvider`. A legacy single `reasoning.provider` block still
loads and is projected into one declared provider. Provider names are the
transport switch: `codex-app-server` routes to the managed Codex app-server
stdio runtime, while every `ollama-*` name routes to the local Ollama endpoint
with that entry's own model catalog (for example `ollama-nemotron` runs
`nemotron-3.5-lightning:latest` and `ollama-qwen` runs `qwen3.8:27b`). The
selected provider is a typed product setting (`reasoningProvider`): switching
it in Settings takes effect the next time the sidecar starts, and the boot-time
profile migration re-resolves saved reasoning profiles against the new
provider's catalog. A `--provider` flag overrides the Settings selection for
one process launch.

Provider entries are provider-specific, so cold-start handling stays local to
the backend that has it. An Ollama runtime evicts a loaded model after a few
minutes of idle, which forces every scheduler-backed session to pay the model
load cost again; that load time is invisible to the model but inflates the
invocation timeout and can trigger a false "awaiting headers" deadline.
`ollama-*` providers therefore expose three independent controls:

- `timeoutMs` bounds only the generation/invocation phase of a warmed request.
- `warmupTimeoutMs` grants a separate, isolated budget spent loading the model
  (the Ollama warm primitive issues a minimal chat request and waits only for
  headers); if the model is already resident this step returns at once, so it
  never inflates `timeoutMs`.
- `keepAliveMinutes` sets Ollama's `keep_alive` option on every request so the
  model stays resident between scheduler ticks. Raise it (e.g. `120`–`300`) to
  suppress recurring cold starts, or set `0` to fall back to the runtime
  default and accept per-session warm-up. `numCtx` raises a model's default
  context window to avoid response truncation without deriving a custom model.

Warm-up, keep-alive, and `numCtx` only apply to `ollama-*` providers;
declaring them elsewhere fails startup validation.

The SDK descriptor's `modelDescriptorVersion` and `modelMaturity` receipt
fields are retained in reasoning, event-resolution, and AI Deep Detection
telemetry.

Ollama invocation capacity remains one by default. Set `maxConcurrentInvocations`
only after measuring the local runtime;
values are bounded and validated at startup. Codex App Server remains a single
managed session unless `codexSessionPoolSize` is set to a positive value. That
option explicitly creates an SDK session pool; it does not use the SDK pool's
multi-session default implicitly. Invocation `durationMs` is the caller-observed
end-to-end latency, including binding. The API also reports `callerLatencyMs`,
`queueWaitMs`, `providerExecutionMs`, and `responseTotalMs` separately when the
provider returns typed timing.

Auto Update is also a typed product setting. One Sidecar-owned scheduler can
prepare hidden finite batches while the process is alive. Adaptive demand is
the default: while the Timeline is idle it maintains one low-frequency standby
batch, then recent batch-reveal pace selects a larger ready-buffer target beneath
the user's queue ceiling after reading resumes. Revealing the standby batch marks
fresh demand and receives one immediate priority refill after a batch reveal;
later refills return to the normal demand cadence. A decaying replenishment-pressure
score can lower the active target and space ordinary refills when recent consumption
and generation are already intense or yield is weak, but it does not suppress the
single standby floor or the first post-reveal refill. Adaptive alone also requires a
small retained-item runway, so a one-item batch can receive one bounded supplemental
attempt. A rolling
generation allowance bounds throughput, and empty prepared results back off
exhausted supply. Continuous and explicit update admission are unchanged. The queue defaults to two,
and local invocation telemetry gates automatic work. The fresh daily boundary
is 2M tokens with 25% unavailable to
automatic work for user-visible updates. A user-authorized daily quota reset preserves
invocation history while establishing a new local allowance baseline. Prepared
batches do not enter the Timeline until revealed. An account-level Codex usage
limit creates a durable scheduler stopper before another tick is admitted. The
Timeline and Settings expose the pause, and automatic work resumes only after
the user explicitly confirms that Codex usage has been restored.

Adaptive supply telemetry classifies every completed user or scheduler update:
an update with retained items is productive, an all-source successful update
with no retained items is valid-empty, and a timeout, capacity, Bridge, or
other failed source is technical. Only valid-empty outcomes increment the
supply streak. Technical outcomes use a separate short retry backoff, while a
productive manual update clears a stale supply cooldown. Scheduler generation
allowance remains scheduler-only and is never consumed by user-visible work.

Built-in bounded-load profiles remain:

| Profile | Native scrolls | Items/source | Session items | Timeline |
| --- | ---: | ---: | ---: | ---: |
| Standard | 2 | 5 | 10 | 12 |
| Expanded | 4 | 10 | 20 | 24 |
| Stress | 6 | 15 | 30 | 36 |

Standard 1x is the checked-in fresh-database and full-reset default. A user's
persisted choice, including Expanded 2x or Custom, remains authoritative across
an ordinary rebuild or restart.

Capture visibility is independent of bounded-load depth. The default
single-window Quiet mode shares one non-focused managed window across sources.
Multi-window Quiet remains available as an experimental per-source isolation
mode. Adaptive fidelity directly uses the
newest eligible canonical source tab in an ordinary Chrome window; it does not
first create or try the Quiet managed window.

## Fresh database

The database defaults to `runtime/aku-sidecar.db`. Schema version 7 contains
only the active tables documented in
[`docs/go-rewrite-architecture.md`](docs/go-rewrite-architecture.md). The
current preview accepts only that schema version. Additive tables within the
same preview line are created idempotently at startup; this is not a Node
compatibility path.

There is no importer for the Node database. A mismatched schema fails closed;
delete or move the development database and start again.

AI Detector strong results are version-bound to the current object-scope
contract. The App Server response is schema-validated and then independently
checked against captured source evidence before it can route or hide a post.
An AI-created external artifact or attached medium does not establish that AI
authored the social post text, and stale strong results are presented as
corrected instead of retaining authority indefinitely.

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
- `GET /api/auto-update/status`
- `POST /api/auto-update/budget/reset`
- `POST /api/auto-update/batches/{sessionId}/reveal`
- `POST /api/timeline/{id}/feedback`
- `POST /api/timeline/{id}/ai-feedback`
- `GET /api/timeline/{id}/ai-feedback`
- `POST /api/ai-feedback/{id}/undo`
- `POST /api/timeline/{id}/recapture`
- `GET /api/timeline/{id}/event-suggestions`
- `POST /api/timeline/{id}/event-correction`
- `POST /api/event-corrections/{id}/undo`
- `POST /api/bridge/heartbeat`
- `GET /api/bridge/health`
- `GET /api/bridge/commands/next`
- `POST /api/bridge/commands/{id}/observation`
- `POST /api/bridge/commands/{id}/failure`
- `GET /api/bridge/media-recaptures/{id}/claim`
- `POST /api/bridge/media-recaptures/{id}/observation`
- `POST /api/bridge/media-recaptures/{id}/failure`
- `POST /api/bridge/timeline/{id}/media-evidence`
- `POST /api/operations/bridge/actions/reload-self`
- `GET /api/operations/bridge/actions/next`
- `POST /api/operations/bridge/actions/{id}/accept`
- `GET /api/operations/bridge/actions/{id}`
- `POST /api/operations/reset-learning`
- `POST /api/operations/full-reset`

All Bridge heartbeat, capture-command, media-recapture, passive-media-evidence,
and cooperative-action routes require both the durable Bridge token and
`X-Aku-Bridge-Contract: aku-browser.bridge.v2`.

Bridge usability is negotiated independently from product release numbers.
Heartbeat protocol major versions must match, the Bridge minor version must
meet Sidecar's minimum, and its declared actions, sources, and adapter entries
must contain Sidecar's required capability subsets. Extension version, build,
runtime revision, and present adapter-version differences are reported as
degraded advisories rather than blocking otherwise compatible work. A legacy
v2 heartbeat without explicit protocol fields is accepted as protocol 2.0
during the transition. `/api/health` and `/api/bootstrap` expose the Sidecar
software-update protocol, database schema, Bridge protocol range, and supported
update-handoff capabilities. Candidate probe schema 1 keeps the frozen
five-field response required by deployed strict hosts; a current host
explicitly requests schema 2, which also reports the database schema version
for pre-activation validation.

The embedded UI restores the source-first dark shell, first-run source
onboarding, editable active sources, bounded custom capture controls, persisted
Source/Brief and stream-width preferences, collapsed long text with Show more,
media inspection, generic source attachments and external LinkedIn link cards,
unique/duplicate latest-check counts, quiet history
boundaries, the finite Timeline finish line, and the boundary-aware back-to-top
control. Reset operations require an exact typed phrase and fail while an
update is active. A full reset creates and verifies a timestamped SQLite backup
before clearing the fresh Go state, preserves the Bridge identity, restores
Standard 1x, and returns directly to onboarding.

First-time onboarding starts one bounded update to acquire real source
candidates, then opens a forced calibration lane before the Timeline. The lane
round-robins pre-selection candidates across every active source, accepts More, Neutral,
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
local event index; Codex App Server proposes only typed relationships. URL,
platform, and generic-language tokens cannot trigger the resolver. Without a
historical shortlist or strong intra-check event anchor, Go creates separate
event threads through a zero-token local fast path. Resolver prompts use
evaluated summaries and evidence excerpts capped at 600 characters. Event
membership and information novelty are separate decisions: the same occurrence
may be a repeated report or a meaningful update. Each event keeps at most eight
recent source-backed deltas locally, while only the three newest compact deltas
enter a resolver prompt. Candidates are resolved sequentially so a fact accepted
from an earlier report is already known when later authors repeat it. The
default collapses true duplicate reports while keeping them inspectable. Show
all bypasses the engine, and Hide removes duplicate reports from the Timeline.
Only `duplicate_report` is capacity-free; material updates, contradictions,
new consequences, and context remain unique. Automatic merging uses a bounded
confidence threshold: `0.92` by default, user-tunable from `0.85` to `0.95` in
`0.01` steps. Duplicate audit rows are bounded separately and never displace
the configured unique-information capacity. User corrections create undoable
local constraints for both event membership and report novelty.

An exact native-source replay is resolved before semantic inference. When the
same opaque evidence identity already belongs to a retained event, Go emits a
deterministic duplicate report at confidence `1.0`, excludes it from the Codex
shortlist, and labels it `Already captured` in Inbox diagnostics. This is an
identity lookup, not a semantic similarity decision. A user's explicit
`must_not_merge` correction remains higher authority and forces a new event,
including when the native source identity is repeated.

Update Inbox records whether the local fast path or App Server ran, along with
the trigger reason, strongest overlap, retained-event count, duration, token
usage, and post-hoc user split/merge counts. It also exposes the asynchronous
Deep Detection job status, reviewed-post count, duration, token usage, and
non-fatal failure. Retained Timeline decisions appear with only More and Less
controls; a new choice supersedes the earlier source/evidence label during
fitting while preserving the append-only feedback audit trail. This makes
semantic, preference, and AI Detector cost and correction signals visible
without exposing raw database identities.

Adapter performance is reported separately from raw feed coverage. A bounded
viewport observation remains `partial` because AkuBridge never claims full-feed
coverage; Inbox derives `capturePerformance.outcome` as `complete`, `degraded`,
or `unavailable` from the latest capture-quality verdict. This prevents a
healthy bounded capture from appearing as an adapter failure while preserving
the narrower raw-coverage claim for diagnostics.

The live update indicator is a monotonic view of the complete synchronous
pipeline: source capture, acquisition planning, follow-up capture, candidate
evaluation, semantic-event resolution, Timeline composition, local AI Fast
Detection, and final publication. AkuSidecar persists the active stage instead
of requiring the UI to infer it from source status. AI Deep Detection begins
after the Timeline is usable, so it is disclosed during finalization and in the
Inbox but never holds the blocking update bar open.

Source scheduling is a typed Setting. The default `progressive_wait` mode keeps
one browser capture lane but starts the next source capture as soon as the
previous source enters reasoning. `full_wait` keeps the original serial
behavior and does not start another source until the current source run is
terminal. Both modes preserve the same global barrier: semantic-event
resolution, Timeline composition, AI Fast Detection, and publication begin
only after every source run is terminal. The selected mode is snapshotted into
the session so changing Settings cannot alter a check already in progress.

Source availability is also typed. A source adapter may report a temporary
site outage before feed discovery; AkuBridge preserves `source_unavailable`
instead of misclassifying the page as a selector, visibility, or reasoning
failure. The remaining sources continue, the completed evidence stays
durable, and the final session is presented as a retryable warning rather than
an AkuSidecar failure.

Each source-run card also offers a lazy `Inspect flow` drill-down. It derives
one row per captured evidence identity from existing observations,
assessments, Timeline items, and semantic reports, then filters that bounded
view by Captured, Evaluated, Selected, or Added. The compact rows expose only
author, excerpt, source link, final outcome, and one-line rationale. Duplicate
snapshots are folded together, semantic duplicate reports are named rather
than counted as unique additions, and the main Inbox response remains light.
An evaluated candidate below the automatic selection line exposes `Should have
selected`. That explicit, undoable correction restores the item to the current
Timeline, resolves its semantic-event relation, runs AI Fast Detection, queues
item-scoped AI Deep Detection, and becomes the strongest positive taste signal.
A later More or Less decision for the same canonical evidence becomes the
newest learning authority without rewriting historical Timeline membership.
Captured-only evidence cannot be selected directly; a failed reasoning run can
instead reuse its durable capture through `Re-evaluate run` without another
browser acquisition.
No raw observation JSON, prompts, media, or heavy telemetry enters this path.

AI Detector is a separate presentation-only domain. Its text-first Fast
Detector runs locally after global composition and recognizes only explicit
evidence: platform labels, author declarations, and prompt/instruction residue.
It does not use stylistic regularity as proof and cannot change selection,
ranking, semantic grouping, or capacity. After session finalization, the
schema-bound Deep Detector reviews a deterministic shortlist of at most five
bounded untrusted posts asynchronously over the shared App Server transport.
Preliminary strong Fast findings have first priority; explicit but
phrasing-ambiguous authorship or agent-identity disclosures may use remaining
capacity. Style alone is never eligible. It skips inadequate text, direct
platform/provenance evidence, ordinary neutral posts, and active user
corrections because model review cannot responsibly improve those results.
The source-controlled offline corpus measures shortlist routing without
consuming model tokens. Failure leaves the Fast result intact. If Deep
Detection overturns an earlier strong result, the UI keeps a corrected badge;
it never erases the assessment without explanation.

Drawer is the preview default and routes unseen strong-signal posts into the
generic Timeline side-pane host without moving posts the user already saw.
Inline remains available. Hide requires the exact phrase `HIDE STRONG AI SIGNALS` and applies only to
direct platform/provenance evidence, Deep-confirmed strong signals, or a user
`Mark as AI-generated` feedback. Preliminary inferred signals are not
Hide-eligible. `Mark as AI-generated`, `Mark as not AI-generated`, and
`Unsure · Review more deeply` are
durable, undoable personal corrections and resolve above Fast or Deep output.
Every card keeps AI status, assessment detail, and corrections in one compact
expandable badge slot. Feedback records explicit post, media, quote, or account
scope plus an optional bounded reason in the append-only `ai_feedback_events`
ledger. Undo appends a `clear`; it does not mutate history. A subtle
`AI signal · Neutral` state exposes the same
controls without claiming that absence of a strong signal proves human origin.

The resolver shortlist is locked to 5, 10, or 15 event threads. Event memory
uses paired age and storage boundaries: 30/60/90 days and
100/200/300/400/500 MB or 1 GB. The defaults are 30 days and 100 MB; crossing
either boundary trims the oldest terminal history and orphaned event threads.

Unavailable X media first has a passive completion path. AkuBridge v60 can
relay evidence from its DOM observers or from the bounded
`x-response-evidence-v2` adapter, which inspects only X's already-requested
`HomeTimeline`, `HomeLatestTimeline`, and `TweetDetail` responses. Raw response
payloads and post text never reach Sidecar. The relay contains only a
sanitized, short-lived cache entry keyed by the authoritative
`x:status:<id>` identity. Sidecar revalidates that identity, accepts at most
four allowlisted `pbs.twimg.com`/`video.twimg.com` post-media records, preserves
`x_response_graphql` provenance when applicable, and writes a completed
`passive-x-media-enrichment-v2` row plus an evidence override without a browser
operation. The enrichment consumes no reasoning call or Timeline capacity and
cannot add, rerank, or semantically regroup an item.

When an accepted X video record contains `playbackMode=inline` and an
allowlisted `https://video.twimg.com/` MP4 URL, the Timeline keeps rendering
the poster until the user explicitly selects **Play video**. Only that action
assigns the URL to a native `<video controls>` element; ordinary Timeline
rendering does not preload the video. The application CSP limits media loading
to `video.twimg.com` and pauses another inline player when a new one starts.
The poster never shows inline and native-play actions at the same time. If the
playback URL is absent or native playback fails, the poster returns to the
previous **Play on native post** behavior. An active inline player pauses when
it leaves the viewport completely and remains paused when the user scrolls
back; playback resumes only through an explicit user action.

The same response adapter may expose the owning Tweet author's allowlisted X
avatar URL to AkuBridge's isolated runtime. Avatar evidence is held only in a
separate bounded in-memory cache and fills presentation when Quiet DOM
hydration omits the image. It is never relayed to this Sidecar endpoint,
persisted as post media, or used by reasoning and selection.

If passive evidence never becomes available, the item keeps its explicit
Recapture action. The first job is always quiet and zero-scroll inside the
managed capture window. If that attempt completes without media, the UI may offer a separate foreground job;
Sidecar permits it only after explicit per-item consent and a completed
unavailable background attempt. This one-time authorization does not change the
persisted Quiet setting. A successful job replaces presentation evidence only;
it never adds, reranks, or semantically regroups a Timeline item.

## Removed by design

Offline experiments, shadow comparison, replay benchmarks, paired-model
benchmarking, pilot review, legacy reason aliases, historical
schema migrations, and hidden provider fallbacks were not ported. Reintroduce
any of them only as a new Go-native product decision with a current contract
and tests.
