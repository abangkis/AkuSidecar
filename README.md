# AkuSidecar

Current preview release: **`0.7.0-preview.1`**.

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
- AkuBridge `0.7.0-preview.1` / `source-fidelity-v60`
- AkuSupervisor for normal development and daily lifecycle ownership

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
`0.7.0-preview.1` package still assumes
the discovered installation is locally signed in; login assistance is
deferred. Settings shows the resolved full executable path beside the
Reasoning processes. A replacement path is provider-validated before it is
stored and hot-swapped into the shared runtime only while reasoning is idle;
**Use detected** reruns the same bounded platform discovery without saving the
result automatically. The default Go provider owns one managed `codex app-server` stdio
process, creates ephemeral read-only threads,
sends output schemas at turn start, rejects server callbacks, and stores
structured token telemetry. Acquisition planning, semantic event resolution,
and AI Deep Detection default to Luna `high`; candidate evaluation alone uses
Luna `xhigh`. Deep Detection runs only after Timeline delivery, while
local deterministic AI Fast Detection does not consume a model. The domain
adapters depend on a generic structured-inference contract rather than the
Codex transport, so another backend can replace App Server without changing
their schemas or authority rules. Settings exposes the active provider, model,
effort, and execution phase for each process. Each process can be tuned for the
next invocation through a backend-owned bounded catalog: Luna High, Luna XHigh,
Terra High, Terra XHigh, or Sol Medium. Free-form model IDs are never accepted.

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
component-level watcher or hidden replacement process. From a stopped
AkuWorkspace—including after deleting the generated `runtime` directory—start
the development stack through the workspace bootstrap:

```powershell
cd ..\AkuSupervisor
.\scripts\dev-akuworkspace.ps1 akusidecar
```

The bootstrap performs the incremental Go build before the generic Supervisor
validates its service configuration. After a source change while the stack is
already running, use the explicit rebuild/restart command from AkuSidecar:

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

Standard 1x is the checked-in fresh-database and full-reset default. A user's
persisted choice, including Expanded 2x or Custom, remains authoritative across
an ordinary rebuild or restart.

Capture visibility is independent of bounded-load depth. Quiet uses the
dedicated non-focused managed window. Adaptive fidelity directly uses the
newest eligible canonical source tab in an ordinary Chrome window; it does not
first create or try the Quiet managed window.

## Fresh database

The database defaults to `runtime/aku-sidecar.db`. Schema version 5 contains
only the active tables documented in
[`docs/go-rewrite-architecture.md`](docs/go-rewrite-architecture.md). The
narrow current-Go migration chain accepts v2 for the original detector tables,
v3 for typed AI assessed-object/signal-scope columns, and v4 for durable
evaluated-candidate and selection-correction state; this is not a Node
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
- `POST /api/timeline/{id}/feedback`
- `POST /api/timeline/{id}/ai-correction`
- `POST /api/ai-corrections/{id}/undo`
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
local event index; Codex App Server proposes only typed relationships. URL,
platform, and generic-language tokens cannot trigger the resolver. Without a
historical shortlist or strong intra-check event anchor, Go creates separate
event threads through a zero-token local fast path. Resolver prompts use
evaluated summaries and evidence excerpts capped at 600 characters. The
default collapses true duplicate reports while keeping them inspectable. Show
all bypasses the engine, and Hide removes duplicate reports from the Timeline.
Only `duplicate_report` is capacity-free; material updates, contradictions,
new consequences, and context remain unique. Automatic merging uses a bounded
confidence threshold: `0.92` by default, user-tunable from `0.85` to `0.95` in
`0.01` steps. User corrections create undoable local constraints.

Update Inbox records whether the local fast path or App Server ran, along with
the trigger reason, strongest overlap, retained-event count, duration, token
usage, and post-hoc user split/merge counts. It also exposes the asynchronous
Deep Detection job status, reviewed-post count, duration, token usage, and
non-fatal failure. Retained Timeline decisions appear with only More and Less
controls; a new choice supersedes the earlier source/evidence label during
fitting while preserving the append-only feedback audit trail. This makes
semantic, preference, and AI Detector cost and correction signals visible
without exposing raw database identities.

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
schema-bound Deep Detector reviews eligible bounded untrusted text
asynchronously over the shared App Server transport. It skips inadequate text,
direct platform/provenance evidence, and active user corrections because model
review cannot responsibly improve those higher-authority results. Failure
leaves the Fast result intact. If Deep
Detection overturns an earlier strong result, the UI keeps a corrected badge;
it never erases the assessment without explanation.

Drawer is the preview default and routes unseen strong-signal posts into the
generic Timeline side-pane host without moving posts the user already saw.
Inline remains available. Hide requires the exact phrase `HIDE STRONG AI SIGNALS` and applies only to
direct platform/provenance evidence, Deep-confirmed strong signals, or a user
`Mark as AI-generated` correction. Preliminary inferred signals are not
Hide-eligible. `Mark as AI-generated` and `Mark as not AI-generated` are
durable, undoable personal corrections and resolve above Fast or Deep output.
Every card keeps AI status, assessment detail, and corrections in one compact
expandable badge slot. A subtle `AI signal · Neutral` state exposes the same
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
