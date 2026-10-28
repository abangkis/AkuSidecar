# X concurrency: current dev readiness

This page is the current checkpoint. Historical experiments in the design and
roadmap are evidence, not a new list of prerequisites.

## Daily-use delivery checkpoint (2026-10-28)

The Settings controls from `058dda4` were published and activated through the
ordinary `runtime/dev` graceful handoff. Five served UI assets matched the
reviewed source, the UI window was present, and startup status was Consent
required with Start disabled until explicit permission. Settings, login,
Supervisor identity and Auto Update were preserved; activation did not grant
derivation. Receipt: `build/main-dev-handoff-058dda4-20261028/ui-verification.json`.

`ec32b4a` corrected PowerShell 5.1/7 inventory/UTF-8 and Supervisor response
compatibility. `d6dc397` added verified tokenless UI close to the ordinary restart
workflow, replacing the print-and-wait behavior. Compatibility checks passed
56 assertions per shell; graceful-phase checks passed 74 assertions in 17 cases
per shell, including the actual workflow call. The operator subsequently
confirmed that manual restart works. This last result is operator-reported, not
a new independently sampled runtime/hash receipt.

Next is daily operation through Settings > Live settings: grant permission and
click Start X in the current process, then use an ordinary manual check or Auto
Update. Verify the X result in Inbox/Timeline while the reader remains open.
Permission is not restored by restart, and Stop remains terminal for its process.
This is the existing main-dev backend and UI, not another isolated QA delivery.
Stability observation, full session rotation and shared reservation remain
follow-up work; they do not reopen the accepted two-batch MVP gate.

## Daily-use Settings controls: offline implementation (2026-10-28)

Settings > Live settings now includes an X concurrent capture panel for a
development process explicitly configured for concurrency. It is hidden in
ordinary exclusive mode and non-development UI contexts. Bootstrap exposes the
existing bounded X status; the existing runtime poll keeps it current.

Start requires an unchecked-by-default, process-only permission checkbox, then
an explicit Start X click. It sends the existing exact consent body and pins
the current process epoch. It does not save consent, change source selection or
Auto Update, or force a capture. Normal manual/automatic scheduling remains the
only batch entry. Stop uses the existing endpoint and retains terminal Stop and
one-start-per-process semantics: a normal restart and new consent are needed
before starting again. No UI control restarts a process automatically.

Polling/rendering do not grant permission. Mutation replies and poll tickets are
fenced against context changes and older reads. A failed or malformed status
closes Start and clears pending checkbox consent; previously authorized Stop
remains available for explicit cleanup. Failed actions cause one read-only
status reconciliation, never an automatic mutation retry. Consent is excluded
from the settings payload and is cleared on bootstrap/context reset.

Offline tests cover exact consent/epoch transport, double clicks, terminal Stop,
stale replies, failed polling, hidden/default-off states and unchanged settings.
An opt-in Go-owned Chrome fixture uses a fresh empty profile, only loopback
content and fake Start/Stop responses to test desktop/mobile interaction and
native form serialization. Screenshots and fixture receipts are retained under
`build/x-daily-ui-offline-20261028/`. This fixture makes zero real-session
requests and does not open the user's profile/database or control their runtime.

This increment was subsequently activated as recorded above. The earlier
native two-batch backend acceptance remains valid for its source and scope;
the first daily-use check should exercise consent and Start through the real UI
rather than the disposable operator. No new derivation or operational action
is authorized by this document. Shared reservation remains deferred.

## Main-dev two-batch acceptance passed (2026-10-28)

The user authorized the ordinary `runtime/dev` handoff of `0991fa2`, followed
by process-scoped derivation of only `auth_token` and `ct0`, at most 60 seconds
per X attempt. Two normal mixed Instagram/LinkedIn/X updates completed in the
same Sidecar process. X remained queued until LinkedIn capture and reasoning
finished, then used its separate disposable collector.

| X batch | Captured | Evaluated | Added to Timeline | Total X run |
| --- | ---: | ---: | ---: | ---: |
| 1 | 2 | 2 | 2 | 22,234 ms |
| 2 | 2 | 2 | 0 | 20,260 ms |

Both X runs completed without error. No Timeline addition in batch two is not a
capture failure. The ordinary predecessors also completed. These total run
durations include startup and reasoning; they are not isolated seed/capture
timings. The observed collector cleanup finished before reasoning completed,
so the attempt was bounded by the total X run in both cases. Predecessor queue
waiting is outside the original seed clock.

The same headed X source window remained visible across all 566 native window
samples. Independent process samples during batch two observed the disposable
headless collector and retained reader alive together. Stop X then revoked
derivation authorization and verified cleanup; no headless collector remained.
Sidecar epoch, Supervisor identity, application settings and Auto Update were
unchanged throughout the acceptance. There was no retry, login mutation or
runtime restart during the two-batch test.

Evidence: `build/main-dev-two-batches-0991fa2-20261028/acceptance-result.json`,
`reader-monitor.json`, `overlap-proof.json` and `after-stop-os.json`. Operational
receipts remain local; this document records their bounded, content-free result.
The reader evidence is the ordinary Open source X window. This run does not
establish a new native-post foreground action, long-running stability or full
session rotation qualification.

The predecessor blocker is closed for this accepted main-dev path. Daily-use
controls subsequently received implementation approval; their offline source
checkpoint is recorded above. This acceptance does not authorize a new native
run or imply long-running qualification. Shared reservation remains deferred.

## Latest offline increment: daily-X predecessor admission (2026-10-28)

Retained startup diagnostics from `ccb55b3` identified Store authorization
unavailability while an earlier LinkedIn capture command was still claimed.
The daily-X scheduler branch bypassed the ordinary predecessor admission rule;
Store correctly retained its capture exclusion.

The source correction now checks predecessor run, reasoning and command drain
before registering the daily worker, StartRun, collector startup or seed. A
blocked X run remains queued. The separate reader route, Store authority,
account checks and original 60-second attempt limit are unchanged. This
conservative boundary waits for predecessor reasoning as well as capture.

Offline validation passed five new top-level daily-X regressions (seven leaf
cases) covering claimed/queued predecessor commands, active/pending reasoning,
automatic worker-retirement wakeup, cancellation and retained reader ownership.
The claimed-LinkedIn regression fails against the pre-patch `engine.go` through
a Go source overlay (`X was started before predecessor drain`) and passes with
the correction. Evidence: `build/daily-predecessor-offline-20261028/` contains
`before-regression.log.txt` and `after-regression.log.txt`.

The scoped Engine/HTTP regressions also passed in default and `auth_session_dev`
builds (`TestXDaily`, `TestXManualStatusAndStopDefaultOff`, `TestHybrid` and
`TestProgressiveWait`), as did development Store authority regressions
(`TestDailyPilot` and `TestManualPilot`). Synthetic native-owner cases used the
accountless fake driver/test-process fixture; no real browser or transfer ran.

The separately authorized handoff and native mixed-source acceptance for this
correction subsequently passed as recorded above. Source validation alone did
not authorize those operations. Future operational runs retain their explicit
process-consent boundary and stop on failure without automatic retry.

The [shared capture reservation plan](shared-capture-reservation-plan.md) is a
deferred structural follow-up. It must not silently become a prerequisite for
testing the conservative MVP correction.

## Prior operational and diagnostic checkpoint (2026-10-28)

The ordinary `runtime/dev` handoff of `6105725` completed. Open source and native
post reader actions worked before process consent. In the subsequently approved
mixed-source acceptance, Instagram and LinkedIn completed; the first X batch
failed during `startup` after 6,830 ms. Batch two was not attempted. Stop revoked
X authority and verified collector cleanup while the reader/main Browser stayed
open. The retained evidence does not establish the failed startup predicate or
whether cookie transfer was reached; elapsed time alone is not a timeout finding.
Receipt: `build/main-dev-two-batches-6105725-20261028/acceptance-result.json`.

The next source increment adds closed `BeginBatch` stage/condition diagnostics,
first-cause retention before cleanup, and correlated bounded startup progress
through the main-profile transport. It does not change account evidence,
ownership, Store admission, deadlines, login or Auto Update. Telemetry-library
adoption/relocation is deferred at the user's request until its path is known.
See the [diagnostic contract](x-daily-development-design.md#startup-diagnostics-and-bounded-progress-2026-10-28).

The final `scripts/test-x-main-profile.ps1` run passed 32 Node tests, the scoped
seven-package Go suite in default and `auth_session_dev` builds, and all four
main build/tag combinations. Synthetic cases cover wrong/malformed startup
receipts, first cause surviving Stop plus cleanup failure, closed diagnostics,
the real inspect/inventory/acquire progress route, per-RPC caps, concurrent
reader-check correlation and late/terminal event rejection. Real Chrome is
explicitly skipped. Evidence:
`build/x-main-offline-3654273701784c5db4ffb94b8314a654/offline.log.txt`.

After offline validation and source publication, the next operational proposal
is an ordinary graceful development rebuild/restart with these diagnostics,
followed by separately authorized process derivation and the two-batch/reader
acceptance. On the first failure, preserve the new diagnostic, Stop and inspect
offline; do not automatically retry or reopen a QA-package delivery route.
No runtime restart or new transfer occurs in this source increment.

## Current daily-use MVP target (2026-10-27)

### Delivery correction: ordinary dev first (2026-10-28)

Daily concurrency must use the established `build-dev.ps1`/`restart-dev.ps1`
and `runtime/dev` contract. Prior sealed packages remain immutable historical
or QA evidence. They do not establish compatibility with the user's normal
manual rebuild/restart, and are no longer the repeated daily delivery route.

The current offline correction stages a complete tagged development candidate,
keeps candidate broker/worker/transport files separate from active files, pins
the full bundle, validates the actual registered service arguments/cwd, and
keeps config/schema diagnostics before optional capture composition. A wrong
registered image requires an explicit one-time handoff, without automatic
service/config/native-host rewriting. Graceful release and whole-bundle rollback
are required. Native ownership/transport adaptation and X-only capture routing
remain the following increments; no runtime activation is implied.

Validation executes the ordinary helper chain against copied fixture checkouts,
plus actual diagnostic CLIs on synthetic config/databases, in addition to the
existing component fixtures. Build success, file hashing and source publication
do not demonstrate main-dev acceptance.

The canonical offline command is `.\scripts\test-stable-dev.ps1`. It passed
four actual main builds, default/dev main-package tests, source-only native
capability staging, complete marker-bundle promotion/rollback/tamper cases and
legacy worker-retention regressions. Evidence:
`build/stable-dev-regression-f83eaa8faa654c04b1cc448a01463eed/offline.log.txt`.
After reusing the existing native-host manifest helper for candidate staging,
the copied build/restart regression passed again in
`build/stable-dev-offline-7cbd50c0111841f9bd8df628196d5e56/final-bundle-regression.log.txt`.
The copied helper fixture replaces compilation/stagers with synthetic tools,
while actual compiled diagnostic CLIs and real source-only native packaging
are validated separately. No real Supervisor, browser/profile, user DB or
runtime startup is exercised. This closes the offline toolchain increment;
it is not daily capture acceptance or an operational activation.

Release-scope verification also passed against a snapshot of the staged Git
index, with the unrelated source-cache changes and untracked cache helper
excluded. The copied ordinary build/restart fixture still passed both modes;
evidence is `build/stable-dev-release-review-8503aeb-28/staged-regression.log.txt`.

See [design](x-daily-development-design.md) and
[ordinary procedure](x-daily-development-activation.md).

### Ordinary X window actions (offline, not activated; 2026-10-28)

Concurrent/main now routes Open source and native post reader actions through
the existing Browser/Bridge/broker independently of batch consent. Engine and
Coordinator must share the exact retained Browser Manager; missing/dead or
mismatched owners reject admission without replacing the browser. Batch Stop
preserves ordinary reader availability. Dedicated mode remains gated.

Synthetic regressions cover successful Bridge/broker requests, rejected
foreground, invalid/missing/mismatched broker requests, owner loss, equal
generation numbers on different Managers and lease/queue cleanup. They also
prove that these UI actions grant neither derivation consent nor legacy X
capture claims, and preserve settings, scheduler state and Auto Update. Account
proof, Store authority and the 60-second batch ceiling remain unchanged.

The canonical `scripts/test-x-main-profile.ps1` passed 30 Node tests, selected
regressions in seven Go packages under default and development builds, and four
actual main builds. Evidence:
`build/x-main-offline-697d999fc00e4602938af20ed8d741d9/offline.log.txt`.
Existing HTTP split/reader/broker regressions also passed in both builds. Their
loopback tests require normal-user socket access outside the restricted sandbox.

These tests use synthetic browser/process/foreground adapters; real Chrome
tests are explicitly skipped. No activation, session transfer, main login,
user database, runtime or Auto Update change occurred. Private transport seed
phases still exclude ordinary operations. Actual window/collection overlap
remains an operational acceptance gate, not a result of these offline tests.

### Daily-X Browser admission correction (offline, not activated)

The first main-dev mixed update persisted its Instagram/LinkedIn/X queue, then
rejected Instagram before creating a capture command. Its frozen daily-X plan
uses Browser for ordinary sources but shares the legacy hybrid policy envelope;
the Engine incorrectly applied the Facebook-only temporary Browser exception.

The working-tree correction admits ordinary non-X Browser work when daily-X is
configured and the coordinator requests Browser. Common predecessor-drain,
lease/driver/generation checks remain in place. A requested headless backend
still uses the legacy Facebook exception, and native X/Store authorization is
unchanged. No schema or frozen-policy format changes are needed.

New synthetic Engine/SQLite regressions cover three Instagram/LinkedIn/X
orders, two batches per order, persistent-reader retention, Bridge stamps,
cross-driver and stale-owner rejection, predecessor drain, the legacy headless
exception and durable non-X queue recovery without X consent. The canonical
main-profile offline runner now includes the existing hybrid Facebook suite.
These tests use fake browser/native owners and deterministic reasoning.

Offline validation passed: 30 Node tests, the selected seven-package Go suite
under both default and `auth_session_dev` builds (including legacy Facebook
regressions), and four main build/tag combinations. Evidence:
`build/x-main-offline-c8797529e9d94bc38b1e99ea8597ea2d/offline.log.txt`.
The existing stop regression now waits for actual synthetic capture entry,
rather than the earlier batch-start marker, before testing cancellation; its
20-repeat check also passed. Real Chrome tests remain explicitly skipped.

This patch is not in the previously sealed runtime candidate. No real X batch
or reader-overlap acceptance is established by it. The earlier main-dev test
queue remains retained for supported cleanup during an approved recovery;
this offline increment does not edit the user database or runtime. Source code
must be reviewed/published and a new candidate prepared before activation.

The main-profile connection is approved for offline implementation; the dedicated
reader package is historical evidence, not proof that the main login is connected.
Track the [main-profile transport, ownership and compatibility gates](x-main-profile-connection-design.md).

Main-profile code/wiring now passes offline/synthetic regressions (30 Node
checks, seven Go packages, four main builds). The Windows fixture uses real
Node/private IPC and fake Chrome endpoints. A separately authorized one-run
empty-profile Chrome fixture also passed: real projected CDP calls, exact main
Job/subprofile scope, Chrome/window survival for two seconds after fixture Node
exit, verified owned cleanup and profile removal. Evidence:
`build/x-main-chrome-local-76cc3af0942f4ac594d1c9d2709a7e4b/chrome-local.log.txt`.
Physical foreground and signed-in main-profile capture remain unqualified. No
activation, real transfer, user-database access or Auto Update change occurred.

The daily package preparer now has an explicit `--reader-source=main` route.
Its v2 contract seals the main transport, proposes concurrent/main in the
ordinary config, and records exact field presence/values for rollback. The
verifier rejects mixed dedicated/main entries, missing transport, stale baselines,
changed rollback terms or implicit activation/consent. The dedicated v1 path
remains available. Package tests use synthetic baselines and the real sealed
transport builder without launching Chrome or opening a database/profile.
See [package preparation and rollback](x-daily-development-activation.md).

The user has confirmed the minimum main-profile backend plan: common
collectionMode=browser, with X still using its independent concurrent headless
collector after process consent. The decision is planning approval only; no
live settings, activation or transfer is authorized. The
[current handoff and phase-aware rollback](x-main-dev-handoff.md) records the
one-field settings change, fresh baseline/no-op handling and restoration of
the actual before value. It supersedes the preflight's pending backend question
without changing the sealed package or historical observations.

The target is repeated X capture in ordinary main dev, selectable with
`capture.xMode: exclusive | concurrent`, initially requiring graceful restart.
Normal Store, effective provider/settings, Timeline and non-X scheduling remain
in use. Derivation authorization lasts only for the running Sidecar process;
each batch remains limited to 60 seconds and requires fresh batch authority.
QA packages are supporting validation, not the delivered MVP.

Config/composition and the daily process/batch adapter are implemented and
validated offline. Without the required resource package and process consent,
X remains unavailable and cannot fall back to legacy capture. A new main-reader
candidate is prepared at
`build/x-daily-development-prepared/53c7a2a805044f0b9130d236b58bac2f/daily-package.json`
from runtime commit `ce9c24f`, with 92 verified payload files and separately
hashed, uncommitted preparation changes. Its manifest SHA-256 is
`922b5dbf1171586aaff3a4652599ccef7a372daffa00507a2f615ad0122d777a`.
It retains `executionApproved:false`, `activationReady:false` and absent process
consent. Remaining work is operational review/resource preflight and actual
daily-use acceptance under separate approval. Full renewal/rotation
qualification stays deferred.
See [daily design and three delivery increments](x-daily-development-design.md).

The old isolated package failed startup, was rolled back and was replaced by
the [schema-complete preparation](x-development-pilot-package-0e46923a.md).
Its old readiness and next-step statements below are time-specific history.

## Historical bounded pilot scope

- Manual, X-only attempt, at most 60 seconds from the original one-use seed
  authorization start; bounded owner/Home preflight remains separate.
- Strict account/login evidence for both owners at startup; identity renewal and
  complete refresh/rotation qualification remain deferred.
- Startup acquisition/publication has an 18-second deadline. Every identity and
  login sample must still be less than six seconds old at final acceptance.
  Both bounds are clipped by the original attempt's remaining time.
- Separate reader and collector profiles, with owned process-tree cleanup.
- Auto Update stays enabled. Source admission arbitrates work; it must not change
  the global scheduling preference.

## Current implementation

### Daily persistent reader and repeated batches: offline increment 2

Increment 1 was reviewed and pushed as `7f495c6`. The new working-tree increment
adds a separate daily process/batch owner protocol, process-start consent,
normal scheduled/manual X dispatch and run-scoped Store authority. The reader
survives successful collector disposal; each collector/profile/generation is new.
Stop and failed/uncertain cleanup prevent another batch. The ordinary source,
provider, Timeline and scheduling settings remain in use.

Startup recovery preserves accepted X reasoning in mixed sessions. Registration
of a cancel/join worker precedes durable command creation, preventing Stop from
orphaning a pending capture. Complete daily-worker drain, including the final
scheduling continuation, precedes database release. The two-batch test also
exposed and fixed an operation/schedule lock inversion during finalization;
the durable session is created before releasing operation for scheduling.

The canonical daily helper passed with 20 Node checks, legacy/daily Store
authority regressions, daily Engine/HTTP cases, synthetic native ownership and
all four main builds. Evidence:
`build/x-daily-offline-7ba0c07728c340439c823aa3fe7342f7/offline.log.txt`.
Unsupported default-build attachment is intentionally skipped; the supported
dev ownership test actually ran. Astra accepted the corrective shutdown/recovery
delta after review. The collector test uses fake child executables, not Chrome.

No runtime, login, user database or Auto Update change occurred. Activation still
requires the dedicated reader profile/resource package and process consent.
The first daily MVP has one acquisition round per X batch; configured command
budgets and evaluation remain ordinary. See [policy, resources and limits](x-daily-development-design.md#increment-2-persistent-process-reader-and-repeated-ordinary-batches).

### Daily mode selection and ordinary composition: offline increment 1

`capture.xMode` now defaults to `exclusive`; `concurrent` is Windows/dev-tag
only and takes effect on restart. Ordinary main keeps its Store, config,
effective provider and UI; the daily selection neither consumes a QA plan nor
starts a native owner. X capture/reader legacy fallback is blocked while the
daily batch adapter is not ready. The status endpoint reports this explicitly.

Synthetic recovery now covers both queued and claimed pre-restart X commands,
X-only finalization, mixed-source progress and accepted-observation follow-up
preservation. The recovery transaction keeps existing protected-pilot fences.
Configuration, daily Engine/HTTP tests and four actual-main builds passed in
`scripts/test-x-daily-development.ps1` (evidence under
`build/x-daily-offline-9555c38c309b426193d2368ec98ab163/`). Strengthened queued/
claimed and accepted-follow-up tests passed separately after that run.
The existing manual-entry suite also passed, including its synthetic HTTP
journey, Store/reader regressions, ordinary-main consent bootstrap and four
builds (log `runtime/qa/auth-session-sidecar/x-development-entry-ee3fe7e24dc742e2865a55586be401fd.log.txt`).

Astra accepted increment 1 after the recovery correction. No runtime, login,
Auto Update, user database, registration or session transfer was changed.
Daily native capture is not implemented by this increment: next is increment 2
in the [daily design](x-daily-development-design.md), including persistent reader
ownership and ordinary repeated batches under process-scoped authorization.

### Activation/rollback candidate prepared, not activated (2026-10-27)

Package `fd755aa1904d4f2e80a6c9d25c249b93` pins the reviewed development-tag
main/helper, owned Node, CfT UI, native driver and an isolated config/plan.
Eleven package tests, 331-file verification and the early config/schema probe
passed without Store, browser, retained-session or provider startup. Backup
files and current-user registration values were captured read-only. No runtime
or Auto Update changes were made; preparation source remains uncommitted.

Both current-user native-host mappings still select another runtime and the
exclusive reader pipe is present. Do not activate until the concrete host/pipe
handoff and candidate launch have separate operational approval; `/start` also
requires a fresh one-use transfer approval. See the [package checkpoint](x-development-pilot-package-fd755aa1.md)
and [activation/rollback runbook](x-development-pilot-activation.md).

### SQLite canceled-query ownership fixed by upstream driver (2026-10-27)

The separately authorized driver update now selects `modernc.org/sqlite v1.57.0`
and matching libc v1.74.4, superseding the tested but uncommitted v1.40.1 choice.
Schema 30, Store/account policy, deadlines and inference SDK v0.24.0 remain
unchanged. Current-version validation and the separately dated historical
v1.40.1 cancellation/pilot evidence are recorded in the linked upgrade note.
The v1.57.0 public-API stress and old/new/old synthetic database checks passed;
67 pilot groups, 59 nested cases and all four actual-main builds also passed.
See [dependency changes, evidence and limits](sqlite-driver-upgrade.md).
This fixes the old driver cancellation leak independently of the Engine's drain
correction. No native session transfer or main-dev runtime activation occurred.

### Ordinary development start wiring implemented; commit review (2026-10-27)

The Windows/dev-only `native-x-manual` selector now continues through ordinary
configuration, provider, Engine and HTTP startup with a pinned instance-local
config/database/profile plan. No selector leaves startup unchanged. The new
epoch/origin-checked one-use start API connects the accepted private factory;
status/stop and X source arbitration are reused. No Store policy/schema or
account verifier changes are introduced.

The optional UI has its own fresh profile and exact-origin broker assets. The
ordinary app-shell OS broker attaches to it; HTTP routes only X to the private
reader without borrowing the ordinary capture profile. Consent, deadline,
foreground and cleanup guards remain intact. The existing native helper/pipe
must be available at activation; no registry or runtime operation is performed
by the source/tests. See [entry, validation and activation boundaries](x-manual-development-entry.md).

The tests additionally found and fixed Engine's early-idle report during deferred
worker cleanup. Teardown now drains the real worker before closing SQLite and
releasing its pins. This is a shutdown-lifecycle fix, not an account/deadline
relaxation. Native transfer and main-dev activation remain unauthorized here.

Commit review additionally found a pending-startup drain gap on the HTTP shutdown
timeout path. Engine idle now includes startup completion, and main joins manual
selection cleanup before closing SQLite. Uncertain cleanup retains SQLite until
process exit. The exported staged payload passed 70 Go groups, 59 nested cases
and four main builds; 18 controller/gate cases and both legacy compile-only
staging modes passed. New synthetic cases cover the pre-route shutdown race,
ordered closure and uncertainty retention. No real browser/session was used.

Final accountless run `a2bc24a2cc804171a53b286d1781ee79` passed 67 Go groups,
59 nested cases and four build modes with no skips/failures. The real ordinary
dev image reached HTTP readiness without invoking native start and shut down
gracefully. Separately, 18 controller regressions and both legacy compile-only
staging modes passed. At that checkpoint the reproduced SQLite cancellation leak
was still deferred. The separately authorized driver upgrade above now fixes
and tests that path; completed-flow acceptance remains scoped to completed work.

### Native capture-start QA passes the complete selected gate (2026-10-26)

One explicitly authorized execution of preparation
`1617147ae968446cb6887b138d4edd7f` passed startup, persisted capture, reader
foreground/post navigation, observed capture-backend overlap and full cleanup.
It used unchanged d24d882 Go source plus the separately pinned, uncommitted
controller. Reader request followed capture entry by 44 ms; marker, foreground
and navigation completion all occurred before capture returned. The strict
overlap predicate was unchanged. One observation/two post blocks persisted.

The controller finished in 17.81 seconds without retry. Independent scoped
verification found no matching QA processes and a clean retained-reader manifest;
only the new disposable collector was removed. Reader profile, older retained
collector, main login/database/runtime and Auto Update were preserved. The
preparation/permission is consumed. See [the accepted bounded journey](x-unified-native-qa-1617147.md).
This closes the prior QA overlap gate, not general production qualification or
main-dev activation. No further native execution is authorized by this result.

### Capture-start sequencing implemented offline (2026-10-26)

Fresh preparation now renders and hashes a one-shot controller containing the
exact tested capture-start gate. After startup acceptance, it waits for
`capture_entered` with no `capture_returned` before sending one `open_post`.
Active status alone never opens the reader. A missed interval, failed/stalled
status read, failed attempt or cancellation ends the sequence without retry.
This is harness sequencing, not account, Store or reader authority.

`test-x-qa-entry.ps1 -ControllerOnly` passed 18 accountless Node regressions in
run `653d3b34ae2446e593766218dd06a9f5`. Preparation
`1617147ae968446cb6887b138d4edd7f` pins the unchanged d24d882 Go source and the
new uncommitted controller separately. It only parsed the generated controller;
it did not execute it, acquire the retained profile, open a database, start a
browser or transfer a session during preparation. The later, separately approved
execution above consumed its claim.

The final native overlap predicate remains unchanged. Capture may still finish
between a status sample and marker creation; only the recorded native events
can establish overlap. The preceding 856c7939 trial remains not accepted; the
later 1617147 trial above passed independently. The offline increment itself
granted no native execution authority.

### Native d24d882: capture, reader and cleanup succeed; overlap gate remains open (2026-10-26)

One authorized execution of preparation `856c7939903641b59641c591e8b3bd9d` on
committed `d24d882` accepted startup, persisted one observation/two post blocks,
verified the reader foreground, completed post navigation, settled, stopped and
verified full owned cleanup. No operational first failure occurred. The new
collector root was then safely removed after independent scoped verification;
the reader profile was preserved and the older retained collector was untouched.

The full harness verdict remains **not accepted**, solely because overlap was
`not_observed`: the marker-ready event preceded capture backend entry by 28 ms,
and the subsequent foreground/navigation events followed backend return. This
is an acceptance/evidence ordering gap, not a new capture, account, reader or
cleanup operational error. Do not silently reinterpret the point-event criterion
or infer concurrency from active status alone. The offline sequencing increment
above now waits for actual capture entry before requesting the reader, without
retries or changing guards. It has not been exercised natively. See the
[native timings and containment](x-unified-native-qa-d24d882.md).

The native trial completed in 17.94 seconds without retry. Main login, user
database/runtime and Auto Update were unchanged. This permission and plan are
consumed; any later native attempt requires a new plan and explicit permission.

### Foreground preparation and graceful-close ordering corrected offline (2026-10-26)

Commit d24d882 addresses the two reviewed ordering gaps.
The native post preparer first observes a unique marker belonging to the exact
reader PID in the held Job, then waits for its foreground during the existing
preparation lifetime. This observation issues no property/capability and never
writes foreground state. A changed, lost, ambiguous or no-longer-owned marker
rejects. Only after foreground is observed and ownership rechecked does the
existing binder mint its unchanged five-second, one-use capability. Preparation
must retain enough time for that capability; the QA operation and reader borrow
are also clipped to the original attempt's remaining time. Account/session proof
and the 60-second attempt policy are unchanged. Stop cancels an in-flight human
preparation wait promptly without canceling the caller's own context.

`reader_marker_ready` is recorded when the owned marker is actually observed,
before the human wait. Waiting does not erase an already observed marker/capture
backend overlap or fabricate one if capture completed earlier. The real
foreground verification and post navigation still run under the fresh capability.

Controlled QA Close now shuts down Engine/admission/work and drains broker abort
and owner disposal while the parent remains live. Parent cancellation follows.
Actual external parent death still triggers the existing immediate Job closure;
its failure path cannot manufacture successful disposal or profile release.
Broker-abort failure remains an uncertain application result even if physical
owner disposal succeeds; physical disposal must still be attempted so children
cannot outlive the parent. Selector/state fencing remains on uncertain Close.
No process-absence heuristic upgrades that verdict.

`test-x-qa-entry.ps1` run `27eb2200fe344d1f9bf3c42f919d6266` passed 37 Go groups
and 83 nested cases, four actual-main builds and both legacy compile-only modes.
Two unchanged hardlink/alias cases were skipped because this environment denied
link creation. New accountless regressions cover a simulated 12-second human delay before
a fresh five-second ticket, marker/capture overlap, changed ownership, expiry,
deadline headroom, cancellation, ordered abort/disposal, genuine abort failure,
and external cancellation with failed disposal retaining an invented profile lease.
The first test revision exposed a teardown-only database-close race; bounded
capture/Engine draining corrected the fixture without changing failure assertions.

Routing/Store/HTTP regression run `dc56f8e9903e4011a2e93b235d30c93e` passed 35
groups and 35 nested cases without skips, plus four builds. Its first invocation
could not read a shared Go cache; a process-local project cache resolved that
environment issue without editing cache settings or helper changes from other work.
The routing regression also covers Stop canceling blocked reader preparation.

At offline acceptance, this increment had no native execution or transfer. Those
synthetic checks alone do not establish Windows
foreground/native cleanup acceptance, dispose the previously retained collector,
or authorize another native trial. Main login, user database/runtime and Auto
Update are unchanged. The next native attempt needs a new prepared package and
explicit one-shot authorization; consumed plans remain unusable.

### Latest unified native QA: capture persisted, reader and cleanup pending (2026-10-26)

One authorized run of preparation `496e9789604e4d57813453e43230466f`, source
digest `fb36e37b92dece3f72c52a3de9753fa2417be24cc3235a4f258f31eeae506f7f`
(reviewed changes on base `022f36d`, not a new committed revision), accepted
startup and committed one observation with two post blocks. The owned reader
marker overlapped the capture backend RPC. `empty_unverified` did not recur.

The complete journey remains **not accepted**: foreground verification ended
before a physical click, no post navigation completed, and application cleanup
reported `uncertain`. The operator confirmed the marker was visible but had not
yet been clicked. A bounded independent check found no process matching this QA
root, the controller had exited, and the retained manifest was `clean`; those
facts do not upgrade the final cleanup verdict. The disposable collector/instance
is retained pending offline cleanup review. Do not reuse its session or plan.

There was no retry, main login/database/runtime operation or Auto Update change.
Offline review identified two ordering gaps. QA binds the five-second foreground
capability before waiting for operator focus. Its controlled Close cancels the
shared parent before Engine shutdown, allowing owner tree termination and
broker abort RPCs to race. Native outcome logs do not identify the exact winning
callback order. Existing synthetic abort/focus ports do not model this race.

The proposed correction waits for the owned marker foreground during preparation
before minting the unchanged short-lived capability, while retaining the original
attempt deadline and recording overlap when the marker becomes ready. Controlled
shutdown should close admission/work, complete broker cleanup while the owner
transport remains usable, then dispose the owned tree and verify profile release.
Real parent death must still trigger immediate Job containment. Logical abort,
physical disposal and release evidence must remain distinguishable; absence of a
process is insufficient to clear uncertain state. These corrections are reviewed,
not implemented in this increment.

This does not block committing the default-off code or continued offline work.
Cleanup and reader acceptance still gate activation for actual main-dev use.
Preserve the demonstrated native capture result; do not reopen deferred renewal
as a prerequisite. Further native execution needs new explicit permission. See the
[scoped result and containment](x-unified-native-qa-fb36e37b.md).

### Capture readiness and structural diagnostics corrected offline (2026-10-26)

The private native capture adapter now takes its default hydration wait from
`captureHydrationMS` in `internal/engine/x-manual-timing-policy.json`: 3,000 ms,
validated in Go and Node within 1,000–45,000 ms. This is a capture readiness
budget, independent of startup evidence age. Each request clips it to the
original authorization's remaining time; neither a reader action nor telemetry
can reset the 60-second attempt. Explicit payload types are validated before
numeric clamping. Login/account/ownership and protected-cookie rules are unchanged.

The worker's empty/invalid capture evidence now survives the adapter, correlated
pipe, Engine result and final QA first-failure report. Optional `worker` metadata
distinguishes `snapshot_unavailable`, `no_posts` and `observation_invalid`, with
hydration/elapsed durations and at most two structural samples: candidate and
rejection counts, nullable readiness/UI flags, and four fixed rejection categories.
UI flags are observations, never login/account authority. URLs, IDs, post text,
cookie values, tokens and arbitrary exception strings are excluded. The terminal
diagnostic envelope is capped at 2 KiB (worker section 1.5 KiB); progress retains
its 512-byte bound. Malformed optional telemetry cannot replace the known
operational cause, grant a retry or enter observation content. Cleanup stays
independent and report snapshots do not expose mutable first-failure state.

Offline regressions reproduce a post appearing after 2.3 seconds: the former
one-second wait fails, while the configured wait captures it. A separate real
worker/parser fixture reports rejected identity candidates. No-snapshot,
privacy, malformed metadata, deadline clipping, first-cause, Engine/Store replay
and actual Node-to-Go pipe tests pass. Actual-main synthetic journeys and four
build variants pass in `x-qa-entry-50fcc2cc704f420d953194ca1fe3881e.log.txt`;
both legacy native staging modes compile without executing native tests.
The accountless pipe regression also uses a sentinel argument so importing a
driver cannot activate its direct-CLI entry.

These results establish the corrections and diagnostic propagation, not the
cause of the previous native failure or native acceptance of this patch. No QA
profile, session transfer, main login/database/runtime or Auto Update setting was
accessed or changed. A fresh native attempt still requires explicit permission
and a new runtime plan; the consumed plan remains unusable.

### First unified native QA: startup accepted, capture rejected (2026-10-26)

The 30-file unified increment was reviewed and pushed as
`022f36d857513849b9e1331cce0bc5a86da0462b`; origin/main matched. One newly
authorized retained-QA attempt then accepted native startup and observed owned
reader-marker/capture-backend overlap. Capture failed first with
`worker / empty_unverified`; no observation was committed, and foreground/post
navigation remained unverified. The subsequent foreground failure did not
overwrite the capture cause. This trial does not close native acceptance.

Complete owned cleanup was verified, the retained manifest returned to `clean`,
the reader profile was preserved and the disposable collector root was removed.
There was no retry or main login/database/runtime/Auto Update change. The next
work was offline worker/parser/page-readiness diagnosis using the
[bounded trial evidence](x-unified-native-qa-022f36d.md), without reopening the
completed shared-ownership integration or renewal scope. A new native run needs
new permission; the consumed runtime plan must not be reused.

### Unified retained ownership and complete offline journey (2026-10-26)

The working-tree increment implements Astra's cohesive integration unit. Legacy
and application callers share the exact retained manifest/lock/state module;
the application owner resolves the existing reader only at explicit start and
releases its lease only before launch or after verified whole-tree cleanup.
Stop preserves the reader and active lease. Uncertain cleanup retains fencing.
The collector, QA database, strict startup evidence and Store policy are unchanged.

Both local and invented retained-source actual-main journeys now compose the
same application path. Tests verify once-committed capture, reader operation,
backend-RPC overlap, terminal first cause, separate cleanup and legacy lease
reacquisition after parent close. The real retained manifest/profile is never
used by these tests. Both legacy staging modes compile without executing their
native binaries. The next boundary is one separately authorized native QA run,
not another retained-binding implementation or renewal investigation.

The [updated operator plan](x-application-native-qa-run-plan.md) describes the
implemented lifetime and clock semantics. The preparation helper now invokes
only the image's early preparation exit and emits a hashed runtime plan with
the retained source deferred, without a database open or browser launch. The
review envelope still reports execution approval/readiness false pending native
permission and observation. Older gap descriptions below are historical.

Acceptance `611740f32903415ca786b3c57877d64f`: 29 Go groups/73 nested cases,
four main build variants, and both legacy compile-only staging modes passed.
Owner regression `ddb78b51d6f04998b1939cdd30d49f01`: 30 groups/21 cases passed;
existing Windows symlink cases require privileges unavailable here. Five Node
seed-clock regressions passed. Current preparation
`2bd6390be6b04ab98634ce0732e3c0b3` includes the exact current source fingerprint
and hashed runtime plan, without real profile/SQLite/browser access.

### Isolated application QA entry: synthetic acceptance (2026-10-26)

The preceding 35-file driver/routing increment was reviewed and pushed as
`e6713edd0cc0c10ebf6bc997fc7d19b2f7014d29`. The isolated QA entry increment
is published as `9ce6a9e32ea852252170e77c19653298668727b2`. It adds the
[isolated QA entry](x-application-qa-entry.md), explicit default-off
selectors, pinned instance/profile/database validation and owned reader post
preparation. The native selector is now implemented under Windows/dev tags
after explicit source-only approval; it has **not been executed**. Ordinary
runtime has no start control or configuration opt-in for it.

Synthetic run `ae6c9295edd7400383dbe8d8caa80ad9` passed eight Engine/post
groups, three actual-main groups and four build variants. It executed the
accountless CLI start/post/capture/status/stop journey and database reopening,
including Windows hardlink and SQLite sidecar rejection. Store marker/version
and schema-classification regressions passed separately in
`8fb0760356074539a91e7ef40cbd2fed`. Foreground must verify successfully before
the one-use navigation; failure retires the temporary capability.

Preparation review found that v1's instance-local reader layout cannot yet reuse
the legacy retained QA profile in place. Its manifest-level lease must also be
preserved; substituting an external path or using only the new owner's root lock
is insufficient. First add/test a fixed retained-reader binding without copying
the profile or credentials, then prepare/review the exact native plan and obtain
execution authorization before any real transfer. Native foreground,
Chrome composition and captured X results through this new entry remain
unqualified. Main login, database/runtime and Auto Update remain untouched.
The dated sections below describe prior increments and do not override this
source/activation distinction.

The [native preparation/operator plan](x-application-native-qa-run-plan.md)
describes this exact remaining binding and the one-shot acceptance journey.
Preparation `21edfb1e39c24536bf4a997a4016878e` compiled the development-only
application and sealed owner package without executing either, inspecting the
retained profile or creating a database. Its artifact manifest deliberately
reports `executionReady:false`; it is not an executable runtime plan.

### Packaged driver and application routing (2026-10-26)

The application-owner/profile-lease increment was reviewed and pushed as
`42111c1bc3e99f7986fb822e33da37bef4bd60c4`. The next, uncommitted increment
packages the minimum private pipe entry and adds the Engine's X-only source
reservation, ordinary dispatch exclusion, reader preparation hook, read-only
status and explicit stop. It reuses the existing broker, private Coordinator,
Store issuer, claim and result path. No Store schema or admission policy changes
are required. The two new Store methods are candidate selection/preflight reads.

The sealed CJS bundle embeds its ESM dependency graph, capture/parser assets and
timing policy. Runtime imports cannot read mutable sibling sources. Build with
`node scripts/build-x-native-owner.mjs`; the output manifest records source/asset
hashes and requires Node 22.15 or later. The default output is project-local
`build/x-native-owner`. Do not interpret a reproducible bundle as account proof.

**Native application activation remains unavailable.** After explicit source-only
approval, the package implements a separate private application seed grant and
declares `sessionSeed=true` as an implementation capability. It does not grant
execution consent. The private Go composition requires a one-use selection bound
to the pinned driver and both physical profiles. Existing QA callers retain their
environment gates; application mode never sets or borrows those flags. No public
seed RPC bypass, native selector or start control is enabled. Any later real
execution still requires its own bounded authorization.

The private factory now composes owned role initialization, the existing one-use
enrollment/two-cookie recipe, strict private receipt validation, broker startup,
then exact capture-runtime construction and Store issuance. Runtime construction
occurs after document pins exist, rather than assuming a prebuilt synthetic
backend. Unpublished startup always disposes the full owner tree independently
of its preserved first diagnostic cause. Normal opt-out preserves the reader;
parent shutdown disposes all owned roles. Native UI integration must supply an
owned post preparer through the existing broker contract before activation.

The focused Node grant/protocol/retained-setup command passed 22 cases using
synthetic values only. Application-owner run
`2660a0dba10d49149c880ac59df71c98` passed the Windows
factory/role/seed-receipt regressions using fake browser processes only.
Routing run `6ac54e44a9be49c699f7c5887c1c0541` passed sixteen synthetic route
groups, existing Store and HTTP/reader broker regressions, and all four actual
main build variants. Its synthetic database and build root were removed. This
is application composition evidence, not native Chrome/session acceptance.

The sealed driver has 59 embedded modules, 25 capture assets and the shared
timing policy. Its SHA256 is
`d36f09c6eb5ff3a9c60994d4a0630e779e70ccf756ad862faeb88b555a72a687`.

The follow-up lifecycle audit preserves an earlier broker cause even when stop
arrives without a prior status read. Shutdown marks/cancels the Engine before
external owner disposal, without changing saved Auto Update preferences. A stop
deadline reports uncertain cleanup and retains the X gate until late resource
cleanup actually completes; it does not trigger a second startup or transfer.
Preflight and paired sampling use separate envelopes from the shared timing
policy while retaining the existing absolute attempt deadline.

Pilot capture is a one-use asynchronous task, so a held X capture cannot block
the ordinary collector pump or non-X selection. Stop and autonomous expiry keep
source occupancy until both browser cleanup and result reconciliation finish.
The synthetic tests hold each boundary to verify this ordering; committed
observations survive expiry and cannot be captured a second time.

The default route has no factory call or browser/profile access. Status cannot
start it. The stop API requires development mode and the current instance epoch
under the existing loopback/origin middleware. The normal reader helper retains
request matching and foreground verification. No native start control is exposed.
Main login, user database/runtime and Auto Update remain untouched.

### Private application owner construction (2026-10-26)

The next provider increment adds the Windows/dev-only application owner factory
and physically pinned existing-profile lease. Asset verification precedes lease
acquisition; disabled/invalid construction cannot start the worker. The factory
reuses the application's suspended launcher and Job lifetime guarantees, answers
a strict nonce handshake through the shared pipe, isolates Node startup
environment overrides, and verifies announced role PIDs against actual OS facts.
No account receipt, Store authority, native provider registration or runtime
activation follows from constructing those resources.

Startup request lifetime is separate from owner lifetime, so a completed request
cannot close a reader that is still owned by Sidecar. Full-owner disposal first
closes admission, then verifies owned process/pipe cleanup before releasing pins
and leases. Unknown cleanup keeps the profiles fenced; it is not a retry signal.
See the [construction contract and limits](x-concurrency-dev-provider-design.md#application-owner-factory-and-profile-lease-2026-10-26).

Accountless run `482eb41b1c304dffa8bc3adc94af383a` passed four lease groups,
eight application-factory groups (ten nested invalid-start/RPC cases) and twelve
existing application session/worker-command groups. It covers wrong asset/pins,
wrong/extra/duplicate handshake fields, stalled readiness, parent cancellation,
effective Node preload rejection, separate startup/owner lifetime, private
operation limits, role binding, protocol loss and verified lease reacquisition.
The fake-browser helper is deliberately skipped outside its child invocation.
Symlink rejection subcases could not execute without Windows symlink privilege.
Evidence: `runtime/qa/auth-session-sidecar/x-application-owner-482eb41b1c304dffa8bc3adc94af383a.log.txt`.

Main regression run `93a092c5157f4255bb645285fb454532` passed four build variants,
all ten actual-main synthetic scenarios, early config/database rejection and
Coordinator regressions. No browser, retained/main profile, X network, real
transfer, user database/runtime or Auto Update setting was accessed or changed.
These are construction/regression results, not another native capture run.

Shared-core compatibility run `15ef359855404f8eb3b7e520450e1999` also passed the
seventeen RPC/OS ownership cases, synthetic Engine/Store scenarios, accountless
Node/Go diagnostics and both named-file staging modes. Staging now fingerprints
the imported profile-lease package as well. The additional RPC regression orders
terminal publication and correlation retirement atomically against concurrent
close, so an accepted operational cause cannot disappear during owner teardown.
Its boolean delivery result is local bookkeeping and grants no session authority.

### Shared Windows ownership and private transport (2026-10-25)

The first provider integration increment extracts reusable Windows OS facts and
private pipe correlation from the QA bridge. Fixtures use the shared
`internal/ownedprocess` and `internal/ownedrpc` implementations through thin
compatibility wrappers. Fixture grants, loopback/root policy, fault injection,
handshake and teardown ordering remain outside the shared correlation package.

The Windows X owner adapter now depends on a private trusted transport/profile
scope port and builds with `auth_session_dev` alone. It retains image/creation,
process-parent, Job membership, physical directory, context/document and reader
inventory checks. No ordinary-build constructor, native factory registration,
CLI activation, public owner RPC, Store policy/schema change or saved opt-in is
added. The existing native provider name still rejects before normal state access.

This completes the shared primitive/adapter extraction, not the application
provider. Remaining construction must supply the owned launcher, packaged
private driver and held-profile lease, then attach X-specific routing/status/stop
to main. See the [provider checkpoint](x-concurrency-dev-provider-design.md#extraction-checkpoint-2026-10-25).

Offline acceptance `4a1e9d33007147d998fa092c6f90142b` passed twelve RPC tests,
four Windows ownership tests, six Engine/Store capture test groups, seven actual
accountless Node/Go diagnostic test groups, and both source-pinned staging modes.
The ownership suite checks kill-on-Job-close using only its own hidden test
subprocess. The symlink rejection subcase could not run without Windows symlink
privilege; it remains unverified rather than counted as a successful alias test.
The subprocess helper is intentionally skipped outside its private child invocation.
Evidence:
`runtime/qa/auth-session-sidecar/owned-browser-core-4a1e9d33007147d998fa092c6f90142b.log.txt`
and its two staging provenance files. A delivered terminal response followed by
EOF and a failed write with another writer queued have focused regressions.
Correlation close is not physical process/pipe termination; that remains the
owner's responsibility and must be proved before source/profile reuse.

Main integration run `e452a73c896647ce85a01d86049cf976` passed four main build
variants, all ten actual-main synthetic scenarios, early state-access rejection
and Coordinator regressions. The native selector remained unavailable and
configuration/database sentinels stayed absent. Its log is under the same QA root.
The unchanged Node capture/parser suites also passed 80 and 36 checks in the
Node portions of `x-manual-native-capture-ba976ee6d99749e19c691285d64463ac.log.txt`;
the corrected Go/staging acceptance is the dedicated owned-core run above.
No browser, retained profile, real transfer or user runtime was accessed. Owned
synthetic databases, staging binaries and temporary build roots were removed.

### Native X capture accepted (2026-10-25)

One explicitly authorized retained-QA attempt on source `5964a24` passed the
native capture scenario. Run `7ec6e926a3344597867582923101e948` completed in
14.37 seconds, without retry. Initial account/login evidence for both owners and
the final ownership check passed; startup publication took 2,319 ms. The oldest
identity sample at publication was 2,163 ms old, within the six-second bound.

The production X worker/parser returned nonempty posts through the owned Chrome
collector, Engine and guarded Store path. Exactly one observation committed to
a newly created disposable QA database. Reader window inventory stayed at two
or more, the collector PID remained bound, and replay returned the existing
commit without recapture. This closes the native capture acceptance gate; do not
reopen deferred renewal or ordinary-traffic completeness as MVP prerequisites.

The separate ordinary-traffic report was `coverage=partial`, with
`orphan_extra`, `redirect`, `response_pending` and `completion_pending` across
173 diagnostic records and zero dropped records. Those conditions no longer
reject a valid post result. This run does not establish complete cookie-writer
coverage or exercise a malicious cookie write; the strict negative guards retain
their offline regression evidence.

Job/lease cleanup was verified and the owned collector/database/build root was
removed. The dedicated retained QA reader profile was preserved. No main-login,
user-database, main-dev runtime or Auto Update change occurred; the existing local
configuration hash and unrelated checkout changes were unchanged. The single
transfer authorization is consumed and cannot authorize a later execution.

Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-7ec6e926a3344597867582923101e948.run.log.txt`,
with matching preparation log and provenance JSON. This proves the bounded QA
composition, not ordinary main-dev provider registration or foreground activation.
Opening another reader *during* native in-flight capture remains covered by the
separate synthetic Chrome scenario, rather than claimed from this ordering.

### Capture traffic domains (2026-10-25)

The native capture target now follows the same separation between observed
header safety and ordinary request diagnostics as startup. Header inspection
runs first for every observed event from the exact capture session. Candidate
`auth_token`/`ct0` writes, blocked attempts, malformed/ambiguous header inspection,
inspection exceptions and target/session loss still reject and invalidate the
owned capture lane. They cannot be skipped by diagnostic capacity or hidden by
an earlier ordinary correlation issue. Actual worker failure, cancellation,
deadline, owner mismatch and cleanup failure retain the existing one-use
fail-closed policy; no retry or new session authority is introduced.

`CaptureTrafficDiagnostics` owns only ordinary request/response accounting.
Unknown/orphan IDs, absent/nonboolean ExtraInfo flags, redirects/reused IDs,
unfinished responses, missing/unexpected ExtraInfo, ordinary loading failure,
record/event saturation and diagnostic observer/sink failures no longer call
`onUnsafe`, reject a post result or revoke the collector. The report is bounded
to 256 diagnostic records, 16,384 observed events, saturating drop counts and
closed condition flags. It retains no URL, frame/owner/account ID, header value,
cookie or body in exported metadata. Saturation cannot stop header inspection.

The private result envelope may include an optional `traffic` report
(`aku.capture-traffic.v1`). `complete` describes that bounded diagnostic ledger,
not complete browser/session coverage. `partial` is also non-authoritative.
Go validates and retains the small report separately after observation, digest,
scope, owner and deadline checks. Missing or semantically invalid diagnostics
become `coverage=unavailable` without accepting an invalid post or rejecting an
otherwise valid one. Wire framing/duplicate-key integrity and global result
bounds remain strict. Optional metadata is omitted if it would exceed the
existing result byte limit. The retained QA entry logs this report separately
from operational capture errors; it is not stored in observation content or
used to select pause, admission, account proof or Store authority.

Related-path audit:

| Site | Decision |
| --- | --- |
| Capture ordinary request cap and final completion checks | Moved to diagnostic-only accounting, including orphan IDs and ExtraInfo mismatches. |
| Capacity branch before header inspection | Removed; security inspection precedes all ordinary accounting and its early returns. |
| Diagnostic observer, finalizer, sink or result-byte pressure | Isolated from operational success/failure; absent reports remain unavailable. |
| Owner-port and Go revocation after actual capture failure | Retained as the selected single-use cancellation/failure policy. Ordinary telemetry now produces no such error. |
| HeaderSafetyGuard and selected account/root proof | Already separated; kept strict inspection and pinned proof rules. |
| Shared driver/collector dispatch | No additional concrete ordinary-diagnostic throw was established; transport/lifecycle failure remains strict. |
| Historical controlled-refresh observer | Its nonce, origin and correlation requirements qualify a different proof; intentionally unchanged. |

Offline validation passed `scripts/test-x-manual-native-capture.ps1`, including
ordinary traffic variants, diagnostic saturation followed by cookie attempts,
blocked/malformed headers, lifecycle and inspection failures, immutable challenge
causes, actual production parser/worker capture and synthetic Engine/Store
results with partial/invalid/legacy metadata. The staged Windows entry and all
four tag gates passed. One additional focused test passed through the real
owner port with an orphan ExtraInfo event and subsequent usable inspection.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-native-capture-f3799d4da9ac4db39f256f822f377037.log.txt`.
No retained QA, Chrome/profile launch, session transfer, login, user database or
runtime operation ran. Auto Update and the existing local configuration remain
unchanged. The exact event behind the historical native `coverage_shape` log
still cannot be identified retrospectively; these regressions qualify the
corrected mechanism offline only.

### Central timing policy and flow trace (2026-10-25)

`internal/engine/x-manual-timing-policy.json` is the single source of timeout
values for this manual startup lane. Go embeds it; Node reads the same asset.
Both reject duplicate/unknown keys, missing or invalid values, inconsistent
local envelopes, and an enlarged transfer-authorization ceiling. Staging copies
and fingerprints the JSON so a prepared executable and Node cannot silently
use different policies. A source change requires rebuilding/preparing again;
there is no hot reload, runtime opt-in or automatic retry.

The approved tuning multiplies operational timeouts by three:

| Bound | Previous | Current |
| --- | ---: | ---: |
| Shared startup acquisition/publication | 6 s | 18 s |
| Maximum age of each identity/login sample | 2 s | 6 s |
| CDP command | 4 s | 12 s |
| Early command wrapper | 4.2 s | 12.6 s |
| Bootstrap observation | 5 s | 15 s |
| Fixed bootstrap evaluation | 1.5 s | 4.5 s |
| Manual reader/collector rendezvous | 1 s | 3 s |
| Settings fallback in shared QA recipe | 2.5 s | 7.5 s |
| Collector page readiness before acquisition | 15 s | 45 s |
| Collector exit | 5 s | 15 s |
| Owner/Store-revoke cleanup | 2 s | 6 s |
| Capture command / capture cancellation cleanup | 1.5 / 1 s | 4.5 / 3 s |

The 60-second pilot/transfer authorization ceiling is a separate permission
bound and remains unchanged. Historical probe observation windows, polling
cadence, test-runner limits and unrelated production collection are outside
this startup timeout tuning. No existing enrollment or session evidence is
reused or granted a new lifetime by changing this file.

The original six-second total is incompatible with the enlarged local command
envelopes (12 and 12.6 seconds) and observation envelope (15 seconds); the
configured shared total is therefore 18 seconds. Reader and collector execute
in parallel under that one original Go deadline. Settings receives only its
remaining wait budget; returning a buffered peer receipt or entering the final
ownership check cannot reset the deadline. Shorter caller/attempt limits still
win. Local timeout maxima are not additive latency estimates: a worst-case
series can exhaust the shared deadline and must reject. Cleanup has its own
bounded retirement budget after admission stops, rather than extending startup
or granting more collection time. No offline test establishes native wall time.

The bounded private flow trace records entry/exit of ownership checks, receipt
delivery, each role's identity/settings validation and final publication.
Each relevant checkpoint includes elapsed duration, remaining startup time,
both sample ages and the remaining sample-validity horizon, without absolute
timestamps or account/process/profile/request values. Time rejection categories
distinguish samples predating the challenge, future timestamps, invalid validity
order, source expiry, age expiry and expiry at publication. The trace is kept
on both successful and rejected startup. Completed Node phase reports remain
available as per-phase maximum durations after the active operation becomes
idle; they must not be summed with enclosing RPC durations. At most 24 Go flow
checkpoints and the existing bounded Node progress records are retained.
Operational first-cause diagnostics remain separately immutable.

Offline validation passed the canonical startup, owner, broker and native
capture composition helpers. Their Node suites passed 79, 193, 57, and 59/36
cases respectively (overlapping suites, not additive unique coverage). The Go
suites cover policy rejection/parity, receipt age at delivery/publication,
future timestamps, original shorter attempt limits, completed Node phase
retention, parallel owner admission and synthetic Engine/Store capture/results.
Four build-tag gates and the staged Windows/retained entry compilation passed.
A synthetic replay with 2,684 ms of post-sample work is accepted under the new
six-second age limit; exact six-second age and 18-second startup boundaries
still reject. These are simulated clocks, not a new native X measurement.
The retained setup's eight accountless tests also pass with the centralized
authorization cap. Owned temporary build/database roots were removed.

Evidence under `runtime/qa/auth-session-sidecar/`:
`native-manual-startup-7b5eaae6e67e41068702e8a3f68c98d2.log.txt`,
`x-manual-owner-5ce7e6b22bd94d0a8508b1524f1bdd8f.log.txt`,
`native-manual-broker-8fa00c37b66e45d585857356e3e6bff6.log.txt`, and
`x-manual-native-capture-88f33be194b546e58d4035078b90fcac.log.txt`.
No retained-profile read, browser launch, X request or session transfer was
performed for this tuning. Main login/runtime, Auto Update and user database
were unchanged. No commit, push or runtime activation is included.

### Preparation diagnostics (2026-10-25)

Private startup diagnostics now include closed `prepare_stage` and
`prepare_condition` fields. Go distinguishes admission/replay, OS verification
before preparation, request binding, RPC, reply decoding/handle validation, OS
verification afterward and document binding. The Windows backend retains closed
categories for process identity, parent links, Job membership and physical
directory checks instead of collapsing every failure into OS-unavailable.
Node distinguishes actor readiness/pins, driver setup, prepared reply shape,
observer snapshot commands, snapshot predicates and distinct reader/collector
ownership. These categories describe the failed check; a failed OS query alone
does not prove that the owner actually changed.

The Node encoder emits V9; Go still accepts the original closed codes and
V2-V8, with absent preparation fields remaining `unknown`. All trace codes
remain at most 80 bytes. Only trusted call sites select categories; exception
text, tokens, paths, IDs and document contents cannot become diagnostic values.
An earlier branded source/guard failure keeps its original role, phase and
cause. Node records the cause before aborting. Go also freezes the first
observed preparation cause before cleanup can block, so a later peer reply,
cancellation or cleanup error cannot replace it. Cleanup uncertainty remains
separate from that first cause. Admission, account checks, Store policy,
deadlines and retry behavior are unchanged.

Offline validation: `scripts/test-x-manual-pilot-owner.ps1` passed 192 Node
tests, the standalone owner/pair Go suite, all four build-tag gates and Windows
backend compilation. Regressions cover 32 Go preparation predicate cases across
both roles, Node actor/reply/snapshot failures, a held reader cleanup with a
later collector reply, first-cause privacy/immutability, settings-wait reporting,
V2-V9 compatibility and actual Node-to-Go preparation catalog decoding.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-owner-c404684f436344af832adc4084338a7c.log.txt`.
Owned accountless build artifacts were removed. No Chrome/profile, native
transfer, user database/runtime, login or Auto Update setting was accessed.
No native run was repeated; the latest historical prepare failure below still
has no recorded internal predicate and cannot be retrospectively classified.

### Earlier timing policy update (2026-10-25, superseded)

The approved timing change separates startup work from evidence age. The private
Go adapter gives the complete initial acquisition (ownership checks, transport,
both role receipts and final pair inspection) less than six seconds after both
owners are prepared. Node receives only the remaining rounded-down wait budget,
capped at 6,000 ms; waits cannot reset or extend it. A shorter caller or absolute
attempt deadline still wins. The source's own elapsed duration is bounded by
six seconds, independently of sample timestamps.

All four identity/login samples must remain less than two seconds old at final
publication; an earlier source expiry also wins. Node rejects locally stale
samples and Go rechecks the earliest sample expiry after final ownership. This
also keeps the maximum time between the two browsers' accepted observations
below two seconds. The admitted manual vector still ends at the original
attempt start plus at most 60 seconds, rather than at sample expiry. No renewal,
retry, cookie-guard relaxation, Store policy or runtime activation is introduced.

Earlier two-second startup and five-second sample-age references on this page
describe the previous timing policy. The latest native attempt below passed
initial startup admission under the new timing policy but failed at capture;
the historical failed-startup records remain unchanged.

Offline validation passed: `test-native-manual-pilot-startup.ps1` (76 Node and
33 top-level Go tests, four tag gates, both Engine builds),
`test-native-manual-pilot-broker.ps1` (55 Node and eight top-level Go tests,
four tag gates, both Engine builds), and `test-x-manual-pilot-owner.ps1`
(151 Node and 22 top-level Go tests, four tag gates, staged Windows compilation).
These suites overlap and their Node counts are not a unique total. Regression
cases accept slow startup with fresh samples, reject exact freshness/startup
limits, preserve shorter caller/attempt deadlines, and exercise the shared
Go/Node private transport. Owned temporary build roots were removed.

Evidence logs:

- `runtime/qa/auth-session-sidecar/native-manual-startup-ab916c30e2f54ce2a972bf0b0d055a8c.log.txt`
- `runtime/qa/auth-session-sidecar/native-manual-broker-74421c65a1144719bed5c35e3c426db5.log.txt`
- `runtime/qa/auth-session-sidecar/x-manual-owner-d461230dcb4c4d268423e07c17bf48e0.log.txt`

No browser, retained QA profile, transfer, user database, runtime or Auto Update
setting was accessed by these tests. The existing local dev configuration and
untracked experiments remain outside this change.

The private native capture adapter uses the existing parser/worker, Coordinator,
Engine and guarded Store commit. Synthetic acceptance covers actual parser output,
replay, cancellation, expiry and ownership rejection. Main/broker routing has
synthetic acceptance. This is not yet an ordinary main-dev native provider.

The new preparation mode is:

```powershell
.\scripts\prepare-x-manual-retained-qa.ps1 -Capture
```

Preparation compiles the real Engine sources and the bounded retained-QA capture
continuation. It excludes the package-wide browser TestMain, pins source and
binary hashes, and checks the disabled entry before any retained-profile access.
The plan distinguishes `capture-store` from the older `startup-memory` scenario;
an old startup plan cannot execute capture. Preparation is not transfer approval.

## Native acceptance attempt

### Offline capture rejection investigation (2026-10-25)

The latest native log below cannot establish the failed capture predicate. The
previous code collapsed local adapter predicates, Node setup/worker errors and
transport errors into generic rejection; the retained QA entry printed only
the broad Engine phase/outcome. Node's final response/header coverage check or
cleanup exception could also replace an earlier worker/challenge error. These
are confirmed observability defects, not proof of the native rejection's cause.

The private capture path now carries closed `stage`, `reason` and `cleanup`
diagnostics. Engine behavior admission and Coordinator fallback are distinct
from backend owner/payload/transport checks, Node target setup, navigation,
evaluation, worker login/challenge/empty-evidence errors, response/header
coverage, wire/digest/observation validation and final ownership. Unknown errors
stay coarse; no arbitrary error message is promoted into a diagnostic. The QA
entry prints only these sanitized fields, never its raw persistence error.

Node uses private WeakMap branding. Operational failure codes and observational
metadata are now independent: the correlated failure reply carries its existing
`code` plus an optional `diagnostic` envelope with schema
`aku.capture-diagnostic.v1`, `stage`, `reason` and `cleanup`. The generic fixture
bridge only carries a bounded opaque payload (originally 512 bytes, now 2 KiB
for the optional structural worker summary); progress remains limited to 512
bytes. The capture adapter validates its version, unique fields, types and closed
catalogs. The optional worker section is validated independently. Unknown/malformed metadata is ignored without changing the operational
failure. A diagnostic claiming challenge cannot create challenge/pause authority;
a contradictory diagnostic cannot change a challenge's independently decoded
category. The old `manual_capture_detail_<stage>__<reason>__<cleanup>` code reader
remains for compatibility, but new producers no longer encode diagnostics there.

The Go error wrapper keeps the domain-owned operational cause independently
discoverable through `errors.As`, alongside its safe diagnostic. Challenge plus
failed cleanup therefore retains both the normal persistent challenge handling
and `cleanup=failed` through Node, bridge, Windows RPC adapter and cached Store
result. A later pause-persistence error may remain the operational error but
cannot replace the stored capture diagnostic. Seed/startup reply behavior and
the existing code sanitizer are unchanged.

Inspection failure and proven mismatch are also distinct. Go reports an
unavailable/canceled/deadline inspection before considering owner comparison;
the zero/unknown result of a failed inspection cannot become `owner_changed`.
Node uses trusted inspection-condition evidence to distinguish confirmed pinned
mismatch from a failed snapshot. Rejection and revocation rules remain the same.
These are local implementations of reusable separation and evidence principles;
no shared library, OTel exporter, AG migration or product-wide schema is added.

Terminal primary failure wins over cleanup/coverage errors; cleanup status is
reported separately. `cleanup=completed` means the capture target's CDP close
command completed, not proof of Windows Job/lease or profile retirement. Actual
owned-process cleanup remains the existing separate QA gate. Capture cleanup
awaits a previously started close instead of dispatching another close. Errors
that the existing worker treats as transient retain their original in-process
classification; a recovered transient does not become a fatal diagnostic.

No cookie/token value, page text, URL, account/owner/request identifier, local
path or raw exception is included in these diagnostics. Startup freshness,
attempt deadlines, guards, result admission, retry rules and Store policies are
unchanged. No browser, retained profile, X request, transfer, user database,
main runtime or Auto Update preference was accessed for this investigation.

Offline validation: `scripts/test-x-manual-native-capture.ps1` passed 44 capture/
owner Node tests and 36 worker/parser/transport Node tests; five top-level Go
tests include 12 Engine/Store integration subcases and ten inspection-classifier
subcases. The retained native entry compiled through the canonical staging
helper. Its separate Windows sanitizer test and accountless Node-to-Go stdio
test passed; the latter uses the actual Node encoder, bridge reader and Windows
RPC error adapter without OS ownership assertions, Chrome or a profile. All
four build-tag gates passed. Covered failures include setup/navigation, pending
ExtraInfo, late cookie writes, first challenge plus failed cleanup, cancellation,
invalid wire/digest/source, malformed/contradictory sidebands, metadata that cannot
manufacture operational authority, unknown private errors, replayed diagnostics and
unchanged transient context recovery. Only synthetic databases and fake CDP/DOM
were used. Owned temporary roots were removed.

Evidence:
`runtime/qa/auth-session-sidecar/x-manual-native-capture-d4831f08908a4a20905f6dc4ec01fc28.log.txt`.
No native attempt was repeated; identifying the latest real capture failure
still requires a separately authorized run with these diagnostics.

### Latest native attempt

Latest attempt: source commit `27e0d7e`, explicitly authorized run
`7d78bf4a93d347529f89e39b18d56d57` on 2026-10-25. All 45 centralized timing
and bounded flow-trace files were committed/pushed before execution. Preparation
passed 283 and 47 Node cases, four tag gates, full staged Engine compilation
and accountless seed/startup/trace checks. The prepared plan pinned the shared
policy asset and the original one-shot, two-cookie, maximum 60-second scope.

Exactly one native execution passed startup admission in 2,201 ms, below even
the earlier six-second total. All four reader/collector identity/login checks,
the final pair ownership check and publication were accepted. Reader identity
age at publication was 2,055 ms (settings age 561 ms), with 3,944 ms remaining
on its sample horizon. This run therefore needed more than the old two-second
sample-age allowance, while its total startup did not need the enlarged
18-second allowance. It does not retrospectively identify the exact failing
sample/check in earlier logs that lacked these fields.

The new trace measured reader/collector settings-wait maxima of 1,478/1,746 ms,
acquire RPC totals of 1,573/1,947 ms and final pair ownership from elapsed
2,125 to 2,201 ms. These overlapping durations must not be summed. Both receipts
were available at elapsed 2,125 ms; the buffered collector read took no further
measurable whole millisecond. Completed internal phases remain visible after
both active role operations became idle.

The complete test still failed (12.60 seconds) after entering capture:
`phase=capture outcome=rejected_no_mutation detail=(stage=coverage reason=coverage_shape cleanup=completed)`.
This category belongs to the capture target's CDP network response/ExtraInfo
coverage checks, not to post-content parsing or account identity. The current
category can describe a missing/nonboolean response ExtraInfo flag, a response
or ExtraInfo event without a tracked request, or unresolved coverage metadata;
the log does not select an exact predicate. No result was committed. The fresh
synthetic capture database and all owned collector/build artifacts were removed
after helper-verified Job/lease cleanup. The retained reader QA profile was
preserved. No retry occurred.

Main login/runtime, user database and Auto Update were unchanged; the local dev
configuration SHA-256 remained
`52063D73180A8F6AB3E514D89CFAA81FB168E0B98381E58FA7FF1AE28D7AADAE`.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-7d78bf4a93d347529f89e39b18d56d57.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-7d78bf4a93d347529f89e39b18d56d57.run.log.txt`.

Previous attempt: source commit `b110d26`, explicitly authorized run
`6b23993bb7c740ddb58d8e80b1cc1067` on 2026-10-25. The 16 preparation-diagnostic
files were committed/pushed as `ef565a4`; `b110d26` aligns two retained seed
test expectations with the appended V9 fields. The first preparation stopped
on those stale expected strings, before lease/profile access or native execution.
After that test-only correction, preparation passed 283 and 47 Node tests,
all four tag gates, standalone full Engine compilation and accountless retained
seed/startup/trace diagnostics. The plan pinned 921 source files, only
`auth_token`/`ct0` and the original maximum 60-second attempt.

Exactly one native execution rejected initial admission with `evidence-expired`
(full test 10.59 seconds). Both owners passed preparation, so this run progressed
beyond the previous reader-prepare failure. The private diagnostic recorded
`role=pair phase=joint reason=unavailable kind=primary elapsed_ms=2684`;
reader/collector acquisition durations were 1,892/1,955 ms. This is not a
six-second startup timeout. The adapter can report this terminal reason during
receipt freshness/validity checks or its final publication check. The log does
not distinguish which sample or which of those checks expired; do not claim
that both account/login receipts were accepted or that final inspection alone
caused the rejection. No initial vector was published, and the capture
continuation never entered, so no capture QA database or observation was created.

Final owned Job/lease cleanup was verified by the helper. The collector/build
root was removed and the dedicated retained reader QA profile was preserved.
The failed preparation-only build root was also removed after confirming that
no execution claim or derived collector profile existed. No native retry
occurred. Main login, user database/runtime and Auto Update were unchanged;
the local dev configuration SHA-256 remained
`52063D73180A8F6AB3E514D89CFAA81FB168E0B98381E58FA7FF1AE28D7AADAE`.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-6b23993bb7c740ddb58d8e80b1cc1067.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-6b23993bb7c740ddb58d8e80b1cc1067.run.log.txt`.

Previous attempt: source commit `bc3914d`, explicitly authorized run
`8063debc56b54e29b6c257d6740dc666` on 2026-10-25. Preparation passed 279 and
33 Node cases, all four retained tag gates, full staged Engine compilation and
accountless Go diagnostics. The prepared capture plan pinned 921 source files,
the original maximum 60-second attempt and only `auth_token`/`ct0` transfer.

Exactly one native execution rejected before initial admission (full test 6.57
seconds). The private seed acknowledgement and scope checks passed; the broker
then recorded `role=reader phase=prepare reason=unavailable kind=primary`.
The outward terminal reason was `cleanup-uncertain`, reflecting uncertain
cleanup during broker failure handling. The primary prepare cause remained
separately available. This is not evidence of an account mismatch, challenge,
cookie write, startup deadline failure or a repeated capture-stage rejection.
The current prepare category does not establish its failed internal predicate.

The capture continuation was never entered, so no capture QA database or
observation was created. The new capture diagnostic path was not exercised by
this native attempt. The earlier startup/reader acceptance remains historical
evidence; this run does not qualify native capture or establish that the patch
caused the earlier-stage failure.

Final owned Job/lease cleanup was verified by the helper, despite the broker's
earlier uncertain cleanup status. The collector/build root was removed and the
dedicated retained reader QA profile was preserved. No retry occurred. Main
login, user database/runtime and Auto Update were unchanged; the local dev
configuration SHA-256 remained
`52063D73180A8F6AB3E514D89CFAA81FB168E0B98381E58FA7FF1AE28D7AADAE`.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-8063debc56b54e29b6c257d6740dc666.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-8063debc56b54e29b6c257d6740dc666.run.log.txt`.

Previous attempt: source commit `ad865b4`, explicitly authorized run
`581ff3a1757546fdbfe84afd2369bfad` on 2026-10-25. Preparation passed 279 and
33 Node cases, all four retained tag gates, staged full Engine compilation and
accountless Go diagnostic checks. The prepared plan pinned 918 source files,
only `auth_token`/`ct0`, and the original maximum 60-second attempt.

Exactly one native execution passed initial broker startup admission, entered
the capture continuation, created its disposable QA database and opened the
native reader. Runtime binding, dispatch, durable Store claim and pre-capture
owner validation also passed, as established by the reached `capture` phase.
The strict account/login receipts were therefore accepted within the separated
six-second startup/two-second final sample-age policy in this attempt. No second
login was requested. No precise startup duration or general timing reliability
claim is available from this log.

The complete test still failed (12.26 seconds) with
`phase=capture outcome=rejected_no_mutation`. This phase contains both behavior
admission and Coordinator/backend capture. The test logs neither the retained
error nor a safe substage/category, so it does not establish whether pacing,
ownership, request validation, the worker, a challenge, cancellation or another
backend predicate caused rejection. Do not infer the cause from the generic
outcome. The phase did not reach result validation or result commit;
`rejected_no_mutation` is not a claim that the earlier durable claim did not
mutate the disposable QA database.

Owned Job/lease cleanup was verified. The collector/database/build root was
removed, and the dedicated retained reader profile was preserved. No retry or
main-dev activation occurred. Main login, user database/runtime and Auto Update
were unchanged; the local dev configuration SHA-256 remained
`52063D73180A8F6AB3E514D89CFAA81FB168E0B98381E58FA7FF1AE28D7AADAE`.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-581ff3a1757546fdbfe84afd2369bfad.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-581ff3a1757546fdbfe84afd2369bfad.run.log.txt`.

Previous attempt: source commit `94d8e2f`, explicitly authorized run
`90d1b8373b2d4f548eb3a99c3c4da2ac` on 2026-10-25. Preparation passed 279 and
33 Node cases, retained tag gates, staged compilation and accountless diagnostic
checks. Exactly one native execution rejected at the unchanged 2,000 ms joint
startup deadline (full test 11.10 seconds).

At the frozen boundary collector was in an OS ownership snapshot for 4 ms;
its acquire RPC had accumulated 1,866 ms, OS snapshots 130 ms and inspect RPCs
2 ms. Reader had no tracked operation active, with acquire 1,698 ms, OS snapshots
171 ms and inspect RPCs 13 ms. These per-role totals overlap and are not additive.
The snapshot is an ownership boundary after the collector acquire RPC, but
does not identify whether its remaining OS work is the role's local post-check
or a later pair check. Full Go receipt/pair acceptance is not established.
No specific cookie, account or settings rejection was recorded. A single
uncontrolled attempt does not establish the timing effect of parallelization.

The capture continuation was not entered; no QA capture database/observation
was created. Owned Job/lease cleanup was verified, the collector/build root was
removed and the dedicated retained reader profile was preserved. No retry or
main-dev activation occurred. Main login and Auto Update were unchanged; the
local dev configuration SHA-256 remained unchanged. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-90d1b8373b2d4f548eb3a99c3c4da2ac.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-90d1b8373b2d4f548eb3a99c3c4da2ac.run.log.txt`.

Previous attempt: source commit `fbf9aae`, explicitly authorized run
`8364848408cb47879bd7b83bf9590471` on 2026-10-25. Preparation passed 279 and
33 Node cases, retained tag gates, staged compilation and accountless diagnostic
checks. Exactly one native execution then rejected at the unchanged 2,000 ms
joint startup deadline (full test 10.59 seconds).

Collector's acquire RPC had returned; it was in the subsequent ownership inspect
RPC for 6 ms when the frozen boundary was taken. Collector cumulative acquire
was 1,763 ms, OS snapshots 157 ms and inspection RPCs 8 ms. Reader had no tracked
operation active; its acquire totaled 1,505 ms, OS snapshots 180 ms, inspection
RPCs 6 ms and inventory RPCs 1 ms. Per-role times overlap and are not additive.
The owner port enters this inspection only after a successful acquire RPC, but
Go wire decoding and final pair acceptance still follow; no full account/startup
acceptance is claimed. The previous `settings_wait` boundary was not active in
this attempt. No settings category remains on that role's new inspection RPC,
because per-RPC timing resets it deliberately.

The capture continuation was not entered, so no QA capture database/observation
was created. Owned Job/lease cleanup was verified, the collector/build root was
removed and the dedicated retained reader profile was preserved. No retry or
main-dev activation occurred. The main login and Auto Update were unchanged;
the local dev configuration SHA-256 also remained unchanged. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-8364848408cb47879bd7b83bf9590471.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-8364848408cb47879bd7b83bf9590471.run.log.txt`.

Previous attempt: source commit `52e1a6e` (startup correction `45adc58` plus offline
fixture wiring), explicitly authorized run `a68c471222af4c71aab3682109348f6f` on
2026-10-25. Preparation passed 279 and 33 Node cases, all retained tag gates,
staged compilation and accountless diagnostic tests. The broader preparation
first exposed a missing wait-budget callback in the in-memory driver fixture;
that fixture now uses the same bounded wait helper, and its 72 tests pass.
The failed preparation never launched Chrome or transferred a session.

Exactly one native execution then rejected at the unchanged 2,000 ms startup
deadline (full test 11.93 seconds). At the frozen boundary no tracked reader
operation was active; reader cumulative acquire was 1,570 ms, OS snapshots
198 ms, inspection RPCs 48 ms and inventory RPCs 1 ms. Collector was still in
acquire (1,763 ms), with `settings_wait` the last reported Node stage (age
1,575 ms), OS snapshots 150 ms and inspection RPCs 1 ms. These per-role timings
overlap and must not be added together. They identify the settings completion
wait as the current observed boundary, not a specific missing response, body
projection failure or account mismatch. Reader idle alone is not proof that
the full pair's final validation completed.

The capture continuation was not entered; no QA capture database/observation
was created. Owned Job/lease cleanup was verified and the collector/build root
was removed; the dedicated retained reader profile was preserved. No retry,
main-login change, main-dev activation or Auto Update change occurred. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-a68c471222af4c71aab3682109348f6f.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-a68c471222af4c71aab3682109348f6f.run.log.txt`.

Previous attempt: source commit `20011da`, explicitly authorized run
`10f3fd8b7d4d4e1f922cbb64bb02e69f` on 2026-10-25. Startup rejected at the
unchanged joint deadline (2,000 ms; full test 9.56 seconds). At the frozen
diagnostic boundary reader was in `inspect_rpc` for 15 ms; its cumulative
`acquire_rpc` time was 1,733 ms, OS snapshots 174 ms and inspection RPCs 19 ms.
Collector recorded 1,606 ms in acquire, 201 ms in OS snapshots and 2 ms in
inspection RPCs; no tracked operation was active for it at the boundary.
Idle does not establish that collector validation had finished.

The measured boundary is a reader inspection after acquisition, not a new
cookie/challenge rejection. Acquisition consumed most of the shared window;
these aggregate counters do not locate its internal longest suboperation.
No capture continuation, QA database or observation was created. Owned cleanup
was verified, the collector/build root was removed, and the retained QA reader
profile was preserved. No retry or main-dev activation occurred. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-10f3fd8b7d4d4e1f922cbb64bb02e69f.run.log.txt`.

Earlier attempt, before timing diagnostics:

One explicitly authorized attempt ran on 2026-10-25, run
`f5eec35fe324475f8584e5553d3e927e`. Seed acknowledgement and scope validation
completed; startup admission then rejected with
`role=pair phase=joint reason=deadline kind=primary` (test duration 9.43 seconds).
The capture continuation was not entered: no QA database or capture observation
was created. This is a failed acceptance, not a capture success.

The joint startup evidence deadline is two seconds, separate from the overall
60-second attempt ceiling. The retained diagnostic does not identify which role
or operation consumed that window; no specific cause is established yet. Do not
extend either limit or infer a cookie/challenge failure from this result.

Owned Job/lease cleanup was verified, the collector/build root was removed, and
the retained reader profile was preserved. No retry was performed. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-f5eec35fe324475f8584e5553d3e927e.run.log.txt`.

## Acceptance scenario

### Combined pair ownership checkpoint

The X owner port supplies a private combined checkpoint to the broker. It reads
policy/consent before inspection, runs the complete reader and collector
inspections in parallel, and rereads policy/consent after both finish. Each role
keeps its OS-before/RPC/OS-after checks, exact prepared handle/document binding,
image/creation/Job/directory validation, headless/window checks and deadline.
The final state uses the inventory already validated by the reader inspection;
it does not trust the policy callback's cached window count. Policy completeness,
parent/channel/observer flags and all enrollment/attempt/session-policy pins must
still pass. No ownership result is cached between checkpoints.

The native pair checkpoint performs four OS snapshots and two inspect RPCs,
replacing the previous six snapshots, two inspect RPCs and extra inventory RPC.
The optimization applies to initial/final pair checks and subsequent guarded
uses. Per-role pre/post-acquire inspections remain mandatory. Existing generic
ports retain their original path only when no combined callback is installed;
a failed installed callback never falls back. Account receipt validation still
applies, using the current shared startup/freshness policy above and the
original 60-second attempt ceiling.

Offline evidence: `test-native-manual-pilot-broker.ps1` passed 53 Node and eight
top-level Go cases, plus both Engine builds (log
`runtime/qa/auth-session-sidecar/native-manual-broker-dd126684e41b489681d5ca9fe7fd28f2.log.txt`).
`test-x-manual-pilot-owner.ps1` passed 151 Node and 22 top-level Go cases, tag
gates and staged Windows compilation (log
`runtime/qa/auth-session-sidecar/x-manual-owner-6c53dc77e695403d9fbc8288c56cb060.log.txt`).
The suites overlap. Focused pair tests also passed after making their late-reply
barrier cleanup robust. They prove parallel entry, exact work counts, fresh
inventory, policy rejection on both sides, peer cancellation, cause retention
and rejection of late inspections/policy reads. Owned temporary roots were
removed. Implementation validation used no Chrome/profile/session transfer or
user runtime. The subsequent single native attempt is recorded above and still
fails. Native timing improvement and successful capture acceptance remain
unproven.

### Settings completion and wait diagnostics

The direct capture and the bound observation now settle metadata completion with
closed `complete`/`failed` results. A selected `loadingFailed`, a rejected
candidate or a scope invalidation releases the waiter as failed immediately;
it does not wait for the joint timeout to reveal a known failure. Explicit
finish also retires incomplete waiters without changing their source verdict.
The manual driver rejects a failed result, retains the original cause and still
cleans up Runtime/Debugger resources. `settings_capture` identifies a terminal
settings-capture failure; an earlier cookie, lifecycle or other cause wins.

Metadata completion still requires the selected response, loading completion and
required ExtraInfo. It does not authorize an account or await the projected body.
Final acceptance still validates the body, account, pinned document, freshness,
cookie guard, cleanup and unchanged two-second/60-second deadlines.

Private progress can include one optional closed `settingsWait` category:
request pending or excluded before identity/by frame/document/history, response
pending, loading completion pending, ExtraInfo pending, metadata complete,
network/candidate/scope failure, or body pending/complete/failed. No request ID,
URL, account, cookie/header value, raw body or exception text is included.
The existing 32-record/512-byte bounds remain; category updates do not reset
the operation age. Go rejects unknown, duplicate or malformed fields/categories,
isolates the roles and freezes the last received category when admission fails
or times out. The first failure carries its category in the same progress record;
later role events cannot rewrite it. Diagnostics remain nonauthorizing and cannot
extend the deadline.

Implementation validation was offline. The subsequent one-shot native attempt
is recorded above and remains a failed acceptance; it reached the ownership
inspection after acquire instead of stopping in the settings wait.

Offline evidence: startup regression passed 73 Node and 31 top-level Go cases,
plus both Engine build variants (`native-manual-startup-692ece5f29d74440a088179651d6ffd5.log.txt`).
Owner regression passed 151 Node and 17 top-level Go cases, all tag gates and
staged Windows compilation (`x-manual-owner-437b048a8f254004962c1fe3f226a92c.log.txt`).
These suites overlap. An additional preparation-only transport check exercised
the closed failure/category across Node stdio into Go
(`x-manual-retained-8f3bb6b536b4466ebb19ed0d153671ef.prepare.log.txt`); its native
entries skipped before profile/lease access. Evidence logs are under
`runtime/qa/auth-session-sidecar/`. All owned temporary build/preparation roots
were removed; no prepared transfer plan remains for this correction.

### Offline startup overhead correction

The Windows owner checkpoint now resolves Node and Chrome parent links from one
process-list snapshot. This snapshot is local to that checkpoint; process
creation, image, handle liveness, Job membership and physical directory checks
remain in place and are repeated at later checkpoints.

The adapter checks the whole pair before acquisition and once again after both
receipts pass validation, immediately before publication. It no longer repeats
the pair inspection between the composed source's buffered receipts. Per-role
checks before and after acquisition remain mandatory, and the final publication
still checks cancellation, source provenance, evidence expiry and policy state.

After the pre-acquisition inspection, Go sends a private integer wait hint
(`startupBudgetMS`, 1 through the configured 18,000 ms), rounded down from the remaining context
deadline. Each Node owner spends that single monotonic budget across identity,
rendezvous and settings waiting; a settings wait cannot reset it. The manual
startup route rejects a missing/invalid hint instead of using the standalone
QA recipe's configured fallback wait. Go remains authoritative over the entire configured
startup, including transport, cleanup and final ownership checks. The hint
cannot extend it or authorize evidence; receipt/account/document guards and
the 60-second attempt ceiling are unchanged.

Implementation validation was offline/accountless. The subsequent single native
acceptance is recorded above and still fails; these changes do not establish
that native startup fits within the configured deadline.

Offline evidence: `test-native-manual-pilot-startup.ps1` passed 55 Node tests,
30 top-level Go tests and both development Engine build variants (log
`runtime/qa/auth-session-sidecar/native-manual-startup-a17b9c92619a4b1fa0432342578b81d7.log.txt`).
`test-x-manual-pilot-owner.ps1` passed 79 Node tests, 17 top-level Go tests,
all four tag gates and staged Windows backend compilation (log
`runtime/qa/auth-session-sidecar/x-manual-owner-6a7f76c74fee45628537bf120524d1c6.log.txt`).
The Node suites overlap; their counts are not an aggregate unique-test count.
Both helpers verified removal of their owned temporary build roots.

Startup diagnostics now retain a frozen, bounded snapshot for both roles when
admission fails: total local elapsed time, the active Go operation and its age,
cumulative Go operation durations, and the last reported Node stage. Node stages
include identity, rendezvous, settings wait, cleanup and root-control restoration.
Node stage age combines its reported duration with local time since reception;
it is not a synchronized cross-process timestamp or proof of the current browser
state. Missing progress stays idle/unknown rather than blaming a browser.

Private progress uses the existing pending RPC correlation, at most 32 records
per RPC, each at most 512 bytes. Only closed operation/cause categories and
bounded milliseconds are accepted. It carries no cookie, account ID, page body,
URL or document pins, and it cannot authorize a result. Late events and events
for completed RPCs are ignored. The first observed failure is retained across a
subsequent deadline; an unattributed timeout remains pair-scoped. Frozen timing
cannot be rewritten by late callbacks or cleanup.

The two-second startup deadline and 60-second attempt ceiling are unchanged.
The latest native failure remains the failed attempt recorded above; offline
timing/transport tests do not reconstruct unrecorded internal operation timing.
The six preflight fixture regressions now register their synthetic root-control
session before exercising guard binding, so they reach their intended predicates.

The capture scenario reuses the existing retained-profile lease and startup
recipe exactly once. It opens an additional reader window, captures one bounded
round through the existing X worker, commits to a newly created QA database,
checks a nonempty observation and exactly-once replay, and checks that the reader
windows and collector PID remain. Reasoning uses the deterministic QA provider.
Post content stays in the disposable QA database and is not printed to logs.
Reports expose fixed phase/outcome values rather than raw errors or account data.

It does not prove opening a new reader window *during* a native in-flight capture;
that ordering currently has separate synthetic Chrome acceptance. It does not
prove foreground Windows activation, long-running renewal, or main-dev activation.

Execution requires a fresh, explicit one-shot transfer approval for the concrete
prepared plan. Only `auth_token` and `ct0`, the existing manifest and 60-second
deadline are permitted. No retry, login fallback or main-profile access is added.
Cleanup retains the dedicated reader QA profile and deletes the owned collector
profile/database/build root only after Job/lease cleanup is verified.

## Shared capture behavior

- Pilot claims and ordinary claims use the same transactional source occupancy.
  The pilot/ordinary exclusion remains effective with optional pacing disabled.
- Configured spacing/backoff applies before the pilot's durable claim.
- Persistent X challenge pause gates both pilot claims and result commits.
- A challenge from an exact active pilot claim pauses X and cancels registered
  work. A stale claim cannot relatch X after explicit recovery.
- Closed challenge categories survive Node and Go transport. Capture responses
  get a separate bounded envelope; startup evidence limits are unchanged.

## Remaining activation work

The isolated actual-main QA selector now has native startup, capture persistence,
reader overlap and owned cleanup evidence. Packaged driver and QA application
routing/status/stop are implemented; the earlier statements that they remained
unwired are historical. Ordinary main-dev activation remains separate:

1. Harness/evidence are committed at `ae97e52`; the ordinary-dev wiring and
   worker-drain increment above is reviewed and accountlessly validated. Prepare
   a concrete development-tag build with its
   reviewed instance-local config, fresh plan and compatible native helper. Keep
   the existing manual X-only, default-off start/status/stop and 60-second limit.
   No new identity source or deferred renewal/rotation qualification is required.
2. Prepare the concrete build/activation and rollback package before obtaining
   explicit main-dev activation permission. Preserve the existing main login and
   Auto Update. Rollback closes pilot admission/owners and returns source routing
   to the ordinary scheduler without clearing challenge state. No QA success
   silently authorizes user-profile/database access or a runtime restart.

No main-dev configuration, migration, build replacement or restart was performed
by this QA. The [provider design](x-concurrency-dev-provider-design.md) remains
architectural background; current implementation/acceptance is recorded here.
