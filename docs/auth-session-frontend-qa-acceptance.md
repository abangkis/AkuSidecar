# Frontend/bridge QA acceptance audit

Checkpoint: 2026-10-14, source base `1c7cff9`, uncommitted continuation.

The full current physical synthetic frontend/native journey now passes. Its
scope and the additional live-reader/production boundaries are recorded below.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| Actual frontend and split UI relay | Real Server assets, Store-seeded Timeline and owned CfT DOM/readback in `frontend-preparation-final.txt`; reviewed app-only PNG | Proven at preparation scope |
| Separate QA instance, fresh profiles, synthetic login | Corrected physical trial used actual tagged main/CfT, fresh source/UI profiles, isolated Store/authority and one synthetic login | Proven at synthetic physical scope |
| Ordinary builds stay closed | Three actual-main builds, four early CLI rejections before ordinary startup, strict development UI guard; ordinary reader/broker regressions | Proven at recorded contract scope |
| Typed relay/authentication/cancellation | Real contracts plus corrected physical app/relay/native completion; subsequent duplicate/stale/cancelled API denials ran before the multiwindow failure | Proven at those boundaries; session-change physical scope remains open |
| Trusted human click on rendered link | Corrected physical trial passed actual click correlation and terminal app/relay/API acceptance after the explicit cue; no simulated input | Proven at synthetic physical scope |
| Frontend invokes Chrome-launched native helper | Final monitor trial passed exact helper image, direct UI parent, Job, creation-age, retained handle and topmost-release checks | Proven at helper invocation scope |
| Actual frontend reader foreground and ownership | Corrected close-window trial accepted exact PID/HWND foreground, bootstrap reuse, terminal app/API, durable audit and remaining-window ownership | Proven at synthetic physical scope |
| Frontend pause/drain/verified resume | Three pause phases, remaining non-bootstrap window held pause, final natural close resumed collection with one login and capture count two | Proven at synthetic physical scope |
| Failure handling and cleanup | Duplicate/stale/cancelled API denials; abrupt actual-main death after resume killed owned UI/headless roots before fallback, retained SQLite pause/claim fence; registry/root cleanup verified | Proven at those boundaries; parent death while reader still open remains a separate gate |
| No original auth/runtime/install changes | One explicitly approved temporary QA registry leaf was applied and removed; final four-view absence, zero scoped QA peers, exact disposable roots removed | Preserved with the recorded QA registration exception |
| No commit/push without another authorization | Current working changes are not published | Preserved |

Evidence lives beneath `runtime/qa/auth-session-sidecar`:

- `frontend-preparation-final.txt`: 29 Node cases; actual-main CLI/build,
  staging, closed admission, real relay/server/cancel, Store seed and ordinary
  reader contracts; headless UI readiness passed in 10.23 seconds.
- `frontend-preparation-viewport.png`: actual Timeline, active synthetic capture
  panel and the real **Open source evidence** link beneath the synthetic card.
- Earlier failed transcripts remain diagnostic evidence, not passing runs.

The passive headless ping/probe results are typed skips. They are not collector
extension or real-source authentication parity. The browser shell is a normal
owned CfT window, not ordinary app-mode startup parity.

## Remaining gates

1. Prepare the native-capable QA fixture and obtain operational readiness for
   the real frontend scroll/click trial. The earlier closed-native physical
   trial is superseded by this positive trial after the temporary-host approval;
   its passing contract evidence remains limited to rejection before borrow.
   The positive trial must correlate the actual content/relay/helper/broker
   request and prove reader foreground, pause/drain/resume and owned cleanup.
2. Establish a permitted native-host discovery mechanism for the isolated QA
   Chrome. Windows documentation/source currently points to registry discovery;
   no reliable portable profile-local override was found. Existing registered
   host names/paths must not be invoked or replaced.
3. The owner has now approved one uniquely named temporary QA-only HKCU host as
   specified in `auth-session-frontend-native-qa-plan.md`. Finish and review its
   exact manifest/helper/registry/private-pipe receipt before application. No
   other registry scope, installation, policy change or user host is authorized.
4. Only after an approved native mechanism exists, test actual frontend click to
   native helper/broker/reader, active capture/reasoning drain, multiple windows,
   last-close verified resume, cancellation/session changes and owned cleanup.

Registration preparation is still read-only. The reviewed helper defaults to
preview, preserves the newly created key handle through value write/readback,
and records canonical key identity/write time to distinguish shared registry
views from a second registration. The native task root must remain present until
the owned processes stop and Root verifies/removes the single unchanged QA leaf;
only then may its staged manifest and binaries be removed.

No completion claim is justified while any required positive frontend/native
gate is unverified. Do not aggregate separate green tests into an end-to-end claim.

## Native preparation checkpoint, 2026-10-14

`scripts/test-auth-session-sidecar.ps1 -PrepareDevelopmentFrontendNative` passed.
The authoritative transcript is `runtime/qa/auth-session-sidecar/frontend-native-preparation-ready.txt`.
It covers configured original worker/content provenance, four early CLI variants,
read-only registry identity/write-time ABI, manifest/hash/tamper rejection, actual
Server result metadata, truthful Store seed, and actual-main rejection of missing
receipt, foreign pipe and unregistered host before SQLite/Node/Chrome/UI. Three
main variants, the unchanged private-transport helper and scoped controller built.

The frozen prepared root is
`runtime/qa/auth-session-sidecar/run-067f89ec9bd5442799a7efcc50114043`.
Its receipt selects only
`com.akubrowser.qa_reader_bee77dfa482c42dfaabf792bccdd429b` in HKCU Registry64,
with the sole allowed origin `chrome-extension://dlibmmlopdahibfniinemhnghlifiple/`.
Root independently re-ran preview: all four exact-name collision checks were
clear, receipt/file hashes matched, `registry_written=false`, and scoped QA peers
were zero. No native UI or registry application has happened at this checkpoint.

The operator-readiness question is pending. After readiness, Root applies the
already authorized leaf and runs the physical native command with this exact
retained root. Window: **AkuBrowser QA - Synthetic native reader**; card:
**QA native frontend: synthetic reader**; target: **Open source evidence**.
The operator may scroll the actual page, then click once on the explicit cue.
Native preparation passing does not prove physical opening, pause/drain/resume,
multiple-window ownership or parent-death cleanup.

Source/evidence audit after preparation: HEAD remains
`1812d983f40389b35ebfe02755198ea0695e7fe7`; prepared extension/driver input hashes
still match; neither an applied-host receipt nor a physical-attempt marker exists.
`dev3-final-preparation-validation.txt` proves the shared bound-authority guard
rejects unauthenticated authority, foreign account, empty revision, wrong profile
and stale owner before a reader marker. `dev2-final-contract-validation.txt`
records verified same-profile/account/revision resume with one login after graceful
and abrupt process restart. The relevant authority tests/driver remain unchanged.
These are evidence at the shared authority/recovery boundaries; physical frontend
behavior during a changed session has not been tested. Preserve this distinction
when evaluating the remaining native acceptance gates.

## Physical attempts and presentation gap

Operator readiness was received. The first actual native attempt failed before
READY because private CDP launch sets `HideWindow=true`; a one-shot visible-only
HWND lookup could not establish the loaded QA window. Evidence:
`frontend-native-physical-validation.txt`. The exact QA leaf was applied and
removed by the Root-owned finally block; `frontend-native-registry-cleanup.txt`
records unchanged owned-leaf deletion after tree emptiness.

After a tagged HWND/presentation correction, the fresh-root trial reached actual
READY/ARMED with PID/Job/class/QA-title checks and `Visible=true`,
`Minimized=false`. Root sent the explicit operator cue, but the test timed out
without an authorized helper click. Evidence:
`frontend-native-physical-visibility-retry.txt`; cleanup:
`frontend-native-visibility-registry-cleanup.txt`. The operator explicitly reported
that no window was shown to click. Therefore WS_VISIBLE/readback is not proof of
operator-visible presentation. Native opening, drain/resume and parent-death
acceptance remain unproven; do not classify this as a successful physical trial.

The implemented causal correction establishes owned-window placement on the active
monitor, safe desktop/session/cloak and intersection evidence, and temporary QA
presentation above other windows without activation. Temporary topmost state must
be released on the real authorized helper path before reader activation. Native
clocks, actual input requirements and ordinary launcher behavior remain intact.
The same approved host name is reused; no production host is invoked or changed.

Native accounting limitation: after operator readiness, the available goal read
still reported `blocked`, `tokensUsed=921237`, `tokenBudget=1000000`; available
tools do not resume that state. That snapshot excludes resumed work and is not
the current aggregate usage. Preserve the initial ceiling and report partial
measurement rather than reset the goal or infer exact totals.

## Final limited physical trial and cleanup — 2026-10-14

The owner authorized one additional trial and cleanup with partial accounting.
`frontend-native-monitor-preparation.txt` passed. The frozen final root was
`run-63094a8dbb934fc2bbdefb5740e59b77`; the same approved QA hostname was reused.
The owned UI reached READY/ARMED with exact PID/Job/class/title, monitor work-area,
cloaking and input-desktop/session checks. Temporary topmost presentation did not
activate the window and was released on the authorized helper path.

`frontend-native-physical-monitor-trial.txt` failed after the actual physical
click/helper receipt passed. The controller validated the real Chrome-launched
helper's exact image, direct UI parent, Job, age, retained handle, topmost release
and frontend request correlation. It then received `dev_frontend_native_failed`
instead of `dev_frontend_native_draining`: `native frontend did not persist pause
before capture drain`. The failing predicate combines SQLite read/error/paused,
collector PID and owner driver checks; no individual-condition telemetry exists.
Reader foreground, composed drain/resume, multiwindow and parent-death gates
were not reached and remain open.

The source exposes a specific ordering mismatch to review next: the new harness
requires the durable marker before `ReleaseCapture`, whereas
`exclusiveSessionBinding.pause` fences admission immediately and writes the
SQLite marker only after active capture/reasoning work is no longer busy. This
supports a harness-ordering hypothesis; it does not establish every failed
predicate or authorize weakening ownership/claim fences. No repair or further
physical retry was performed within the one-trial scope.

Root finally removed the unchanged owned QA leaf after tree emptiness;
`frontend-native-monitor-registry-cleanup.txt` records the result. Independent
cleanup confirmed absence in all four registry views and zero peers in the three
exact attempted roots, then removed those roots after path/reparse/receipt checks.
Stable logs, `frontend-native-monitor-final-checkpoint.json` and the synthetic
failed SQLite remain under `runtime/qa/auth-session-sidecar`; profiles, binaries,
private cue/nonce files and staged manifests were removed. No original auth,
user runtime, production registry, commit or push was touched.

The latest native `get_goal` returned null. The last historical measured count
is 921,237 tokens at the earlier blocked cutoff; resumed work and worker coverage
are unmeasured. The owner subsequently requested no budget for this task and a
reset. Resolution with `--task-budget none` confirmed a task override with no
ceiling, leaving the saved default 1,000,000 intact. Native counter reset is not
available; historical usage is retained, and new-phase actual usage is unknown,
not measured zero. No replacement goal/counter was created.

Publication review (2026-10-14): the owner authorized committing and pushing the
current checkpoint. Go sources were formatted; the focused staging/presentation/
native metadata/receipt contracts passed across main, readerbroker, HTTP, Engine
and appshell. The read-only registry ABI test needed execution outside the sandbox
after a sandbox access denial; it passed with zero registry writes. Thirteen
development-priority/native availability/routing Node tests passed, and the
ordinary Sidecar build passed. These checks do not replace the failed physical
pause/drain acceptance. Runtime evidence/profiles/binaries remain untracked and
excluded from publication.

## Authorized pause-order correction — 2026-10-14

The owner authorized correcting the fixture, adding separate pause/PID/driver
evidence and repeating the physical synthetic frontend trial. Admission and
reasoning-blocked receipts now require the original headless collector/Manager
PID, a held lease, closed admission and no premature SQLite marker. After
reasoning is released, a buffered fixture observer records the actual SQLite
commit inside the tagged binding before its original session lease is released.
That receipt requires SQLite pause, zero active capture/work, the original
headless owner/PID and a live lease. It neither adds a replacement lease nor
blocks the handoff; ordinary bindings have no observer installed.

Each phase reports SQLite read errors separately from pause, collector/Manager
PID, driver, generation, active leases and active work. The controller verifies
the expected collector against the READY receipt, then consumes the three phase
receipts before accepting foreground/terminal UI. Native clocks, physical input,
identity/ownership and resume verification remain intact. The focused Engine
drain test proves no SQLite marker before capture/reasoning drain and a committed
marker while the original lease is still held. The new pause-evidence validator
rejects read errors, PID/driver changes, missing leases and undrained work.

Both focused tests passed in `frontend-native-pause-order-contract.txt`. Physical
acceptance is pending new preparation and operator readiness. Reuse only the
previously approved QA hostname with new root/profiles/synthetic login; do not
touch original authentication or user runtime, and do not commit/push this delta
without another authorization. The inherited task override remains no budget;
historical partial usage is retained rather than reset.

### Corrected physical trial result

`frontend-native-pause-order-preparation.txt` passed with the new phase contract,
five affected packages, three main build variants and actual-main rejection of
missing/foreign/unregistered startup. Broader synthetic Engine/Store/HTTP checks
ran in `frontend-native-pause-order-regression.txt`: Store/HTTP passed; two Engine
cases hit a sandbox loopback denial and SQLite cleanup lock, respectively. Both
passed when specifically re-run outside the sandbox in
`frontend-native-pause-order-regression-environment-retry.txt`.

The operator confirmed readiness and performed one actual link click after the
explicit cue. `frontend-native-pause-order-physical-trial.txt` proves:

| Phase | SQLite pause | Collector/Manager PID | Driver | Held leases | Active capture/work |
| --- | --- | --- | --- | --- | --- |
| Capture blocked, admission fenced | false; no read error | 32720 / 32720 | headless | 2 | 1 / 0 |
| Reasoning blocked | false; no read error | 32720 / 32720 | headless | 1 | reasoning work 1 |
| After drain, before original lease release | true; no read error | 32720 / 32720 | headless | 1 | 0 / 0 |

The same physical click then completed actual app/relay/content/worker/native
helper/private broker/reader. Helper PID 24912 had the exact direct UI parent,
image/Job/age and retained-handle proof. Reader PID 22132, HWND 9438718 passed
exact foreground/readback; actual API and terminal UI completed, durable audit
was present, bootstrap was reused, and synthetic login count remained one.
No extra/warm click or simulated input was used. This directly closes the earlier
pause-order and first physical frontend handoff gaps at synthetic QA scope.

The full test still failed after closing the bootstrap ownership window:
`native frontend remaining window lost pause`, before `dev_frontend_native_resumed`.
That separate combined predicate checks inventory error, remaining window count,
bootstrap flag and Coordinator pause; individual values were not recorded.
Do not infer that SQLite pause cleared or that the extra window disappeared.
Multiwindow retention, final-close verified resume and abrupt-parent lifetime
were not accepted in this frontend composition. Next investigation must isolate
these values and the reader PID/driver before changing lifecycle behavior; no
automatic additional physical trial was attempted.

The same approved temporary QA host was removed by Root finally after tree
emptiness (`frontend-native-pause-order-registry-cleanup.txt`). Independent
four-view absence, zero scoped peers, exact-root/non-reparse/REMOVED-receipt checks
passed, then the root `run-a71bd2d9496a4ff1a94ae6e2d990e541` was deleted. Stable
logs, redacted HWND evidence in `frontend-native-pause-order-final-checkpoint.json`
and the synthetic failed SQLite remain; profiles/binaries/private cue files do
not. Source remains uncommitted, base `da58e68`; original auth and user runtime
were untouched. Latest goal read remains null: no budget is the task override,
921,237 is historical partial usage, and current actual usage is unavailable.

## Multiwindow evidence only — 2026-10-14

The owner authorized step 1: add evidence before/after bootstrap close, with new
synthetic profiles/login for any future physical execution. This step adds two
receipts after the previously accepted frontend handoff:
`dev_frontend_native_windows_before_close` and
`dev_frontend_native_windows_after_close`. The controller prints each full typed
snapshot before asserting its existing window predicates.

Snapshots record inventory known/error/count/bootstrap flags, Coordinator pause,
SQLite known/pause/read error, current process and Manager PID, driver, generation,
active leases and capture/work counts. Expected reader PID comes from accepted
foreground evidence. Unknown inventory uses count -1 and no flags; known empty
inventory has count zero and an empty array. SQLite and Coordinator pause remain
independent observations. Flags are copied to preserve the sampled values.

The after-close receipt also records selected bootstrap index and close error,
and is emitted even if close failed, before propagating failure. The original
inventory/count/bootstrap/Coordinator admission predicates remain intact; errors
name the failed predicates instead of claiming a generic loss of pause. These
are sequential reads, not an atomic ownership snapshot or a new lifecycle gate.
No credentials, URLs or profile contents are added to the receipt schema.

Focused contracts passed in `frontend-native-window-evidence-contract.txt`: exact
window-failure classification, unknown/known-empty distinction, independent
SQLite/Coordinator values and copied flags, plus prior pause-evidence validation.
Ordinary and dual-tag Sidecar builds passed. Go reported nonfatal denied writes
to its external module stat cache; exit codes were zero and output binaries were
created under project QA. No physical browser trial, synthetic login, registry
write or user-runtime action occurred in this evidence-only step. The earlier
live failure still has unisolated values; next scope is a fresh physical trial
that consumes these receipts. This delta is uncommitted, base `1c7cff9`.

## Multiwindow cause, correction and passing physical trial — 2026-10-14

The new diagnostic trial in `frontend-native-window-evidence-physical-trial.txt`
isolated the failure. Before and after a successful close request, inventory
remained known with two windows and flags `[true,false]`. Coordinator and SQLite
pause were true with no read error; reader/Manager PID stayed 28212, driver
`browser`, generation 2. Failed predicates were `expected_one_remaining_window`
and `remaining_window_is_bootstrap`, not `coordinator_not_paused`.

The old driver grouped page targets by window but retained only the first target
ID while OR-ing bootstrap identity across all targets. It closed that one target
and accepted target disappearance as window closure. The corrected fixture
retains every target per selected window, rechecks each target's window identity
before closing it, and verifies the selected window ID disappears. Targets that
moved are rejected; new/replacement targets do not turn a surviving window into
a release proof. Final-window success also requires observed Chrome root exit,
not only private-pipe closure. No whole-browser forced close is used for this
actor. The Node module is hash-pinned in prepared SourceInputs.

Six Node tests passed in `frontend-native-window-close-contract.txt`, including
an explicit reproduction of the old one-tab false success. The actual Chrome
Engine/SQLite restart/multiwindow/resume regression passed in
`frontend-native-window-close-chrome-regression.txt`. Fresh preparation passed in
`frontend-native-window-close-preparation.txt`, using the same approved temporary
host name with new source/UI profiles and synthetic login.

After operator readiness and one actual physical click, the corrected full trial
passed in `frontend-native-window-close-physical-trial.txt` (26.19s):

- Source collector PID 11332 retained its headless ownership through capture and
  reasoning; SQLite commit occurred with its original lease before release.
- Chrome-launched helper PID 632, reader PID 39424/HWND 3410336: actual app/relay/
  original content/configured worker/native/private broker foreground completion,
  exact readback, durable audit, bootstrap reuse and one source login passed.
- Before bootstrap close: two windows, `[true,false]`; afterward: one window,
  `[false]`. Both pause views remained true, reader/Manager PID stayed 39424 and
  driver `browser`/generation 2. Both predicate lists were empty.
- Final natural close produced source-account/profile-verified headless resume,
  capture count two and login count one, plus actual runtime API readback.
  Duplicate, stale-epoch and cancelled API actions were denied.
- The existing lifetime stage then terminated only actual main. Pinned UI and
  source Chrome handles signalled, UI HWND disappeared and outer Job was empty
  before fallback; reopened SQLite retained pause and rejected never-claimed work.

Lifetime scope is precise: that stage runs after reader windows have closed and
collection has resumed, so it covers owned UI plus paused headless Chrome. The
native helper already exited normally. It does not prove abrupt main death with
a live reader or a blocked native helper; those remain separate evidence gates.
Real-source auth, ordinary installed/runtime parity, active media recapture and
non-headless batch support remain excluded. Passing this synthetic journey does
not activate the feature in the user's runtime.

Both approved-leaf finally cleanups passed. Independent cleanup confirmed exact
host absence in four registry views and zero scoped peers, validated REMOVED
receipts/non-reparse paths and removed both diagnostic/corrected trial roots.
Stable logs, redacted HWND/receipt hashes and minimal synthetic failed SQLite
remain. No original authentication/user runtime, installation or publication was
changed. The task now uses an active native 1,000,000-token goal from its new
measurement baseline; historical 921,237 and the unmeasured interval stay separate.

Completion audit identified a distinct remaining lifetime state: the passing
normal journey kills main only after reader close and headless resume. The owner
subsequently authorized implementing/testing main death with reader still open.
The concrete gate is recorded in `auth-session-frontend-native-qa-plan.md`:
separate pinned scenario, accepted live reader/paused original queue, pinned UI/
reader process and HWND death before fallback, retained original SQLite run/claim
fence and one login/capture. Until that evidence exists, lifetime completion is
unproven at live-reader scope; do not substitute the post-resume result.

## Live-reader implementation and causal retry preparation — 2026-10-14

The separate allowlisted scenario `reader_open_parent_death` is implemented and
pinned in prepared metadata. Verification with `normal` against this prepared
scenario was rejected before registration/UI or the physical-start marker.
Unknown scenarios are also rejected by actual main before SQLite/Node/Chrome.
The normal path still defaults to `normal` and retains its prior passing evidence.

The first physical attempt (`frontend-native-reader-open-physical-trial.txt`)
accepted the actual frontend/native click: main 12068, UI 4820, collector 37368,
helper 6240, reader 39764/HWND 8586774. Capture/reasoning drain, committed pause
before original lease release, exact foreground/API/durable audit, bootstrap
reuse and one synthetic login passed. The new branch then failed before its
parent-death-ready receipt: it expected the second original run's ordinal to be
2, but Store run ordinals start at 0 and that run's ordinal is 1. Consequently
this attempt does not prove live-reader parent death. Its finally-owned registry
cleanup passed; independent four-view absence, zero scoped peers and exact-root
removal passed (`frontend-native-reader-open-artifact-cleanup.txt`). Redacted
checkpoint and logs remain, not the profiles/private cues/binaries.

The causal correction accepts only the original session's queued second run at
ordinal 1. A focused contract accepts this zero-based run and rejects ordinal 2,
a replacement session, completed status and a missing second run. Fresh
canonical preparation passed in
`frontend-native-reader-open-corrected-preparation.txt`: those contracts, live
window/pause/owner checks, scenario/startup rejection, the existing five-package
contracts, ordinary/dev/dual-tag/helper builds and controller compilation. The
new root `run-e04fdea9ff7b496dacf268df59bf90f3` is PREPARED, with no applied host
or physical-start marker yet; the causal physical retry awaits operator readiness.

At that preparation point the continuation was assumed to remain never-started.
The next physical attempt contradicted this assumption: the actual run was
`waiting_for_bridge` with a prepared, never-claimed `queued` command. The original
batch receipt was an earlier snapshot, not a current command-state read. The
subsequent correction below supersedes the never-started-only expectation.

Git metadata became unavailable to the tools during this step: `.git` was not
visible in either sandbox or escalated inspection. Earlier delta review and
worker whitespace checks were completed while it was available; a final current
Git check remains unavailable. Source files remain in place. No Git initialization,
restore/reset, commit or push was attempted. The last verified source base remains
`1c7cff9`, not a fresh HEAD verification.

## Live-reader command proof and lease-contract correction — 2026-10-14

The ordinal-corrected physical attempt failed after accepted frontend/native
activation (`frontend-native-reader-open-corrected-physical-trial.txt`, 28.29s).
Main 25608/UI 29476, collector 41572, helper 24300 and reader 41656/HWND 4000156
passed capture/reasoning drain, committed pause before lease release, actual
foreground/API/durable audit and one source login. The branch rejected the actual
second run's `waiting_for_bridge` status even though its command was `queued`.
Read-only inspection of that scoped synthetic SQLite confirmed durable pause,
ordinal 1, a single queued headless command, null claimed-by/claimed-at/completed/
error fields and the payload hash. The redacted evidence is retained in
`frontend-native-reader-command-failed-sqlite-proof.json`; no raw payload/cookie
was emitted.

The fixture now reads all command rows for that original run via the tagged
read-only `FixtureRunCommandEvidence`. It records stable ID/run/type/status/time,
SHA-256 of exact payload bytes, route/driver and nullable claim/completion/error
fields. The live gate requires one actual `collect_visible` queued headless
command, with null claim/completion/error fields, attached to the original
ordinal-1 `waiting_for_bridge` run. Thus its blocked claim cannot pass merely
because no command exists. Compare the original run and complete command-row
snapshot after the pre-death blocked claim, SQLite reopen and post-death blocked
claim. Null and empty metadata remain distinct; payload bytes remain private.
Store reopen/fence/hash-change contracts and queue rejection contracts passed;
fresh six-package preparation and default/dev/dual-tag/helper/controller builds
passed in `frontend-native-reader-command-preparation.txt`.

The owner explicitly authorized one further physical test with partial token
measurement, plus cleanup. That test reached the real reader state but failed
the fixture's lease predicate before any parent-death-ready receipt
(`frontend-native-reader-command-physical-trial.txt`, 35.48s): main 32644/UI
33332, collector 38264, helper 6380, reader/Manager 25924/HWND 3803574, browser
generation 2, known one bootstrap window, Coordinator/SQLite/admission paused,
capture/work zero, one login. ActiveLeases was zero. The earlier passing normal
multiwindow snapshots also showed zero leases; the request lease is temporary,
while window/process replacement readiness retains profile ownership. Requiring
a positive request lease after the completed action was an incorrect fixture
assertion, not evidence that ownership had been lost.

The final predicate accepts this zero-lease live-window state while still requiring
the expected reader/Manager PID, browser driver, nonzero generation, known live
bootstrap window, independent pause proofs and zero capture/work. The controller
retains its process creation/image/Job/HWND checks before only-main termination.
`frontend-native-reader-window-lease-contract.txt` passed across cmd, Engine,
Store and Coordinator, including the existing borrow-release test that retains
the open reader and returns only after natural exit. This validates the predicate
correction; it does not prove abrupt live-reader parent death. No additional
physical attempt was automatically launched.

Owned registry cleanup passed for all three attempts. Independent four-view host
absence, zero scoped peers, REMOVED receipts and non-reparse exact-root checks
passed; all seven new trial/build roots were removed. Logs, redacted checkpoints
and SQL evidence remain. No original auth, user runtime, installation, Git reset/
initialization, commit or push was performed. Git metadata remains unavailable.

Completion is still unproven: the live-reader lifetime-ready, only-main kill,
pinned UI/reader death, empty Job before fallback and post-death original command
retention stage has not run successfully. A newly prepared, separately approved
physical attempt is required. Do not substitute the previous normal post-resume
UI/headless lifetime result or these passing contracts for this missing state.

The numeric task budget remains 1,000,000. Native goal status stayed `blocked`
after operator readiness; available tools cannot resume its counter. The native
cutoff is 465,295 goal tokens from the new baseline. The owner approved one bounded
physical test/cleanup with subsequent usage reported unmeasured. Do not reset the
goal or present that cutoff as the current whole-task total; worker coverage is
unknown, and historical 921,237 plus the older unmeasured interval remain separate.

## Separation authorized, implementation underway — 2026-10-14

The owner approved automatic lifetime coverage plus a separate physical
foreground test. The concrete plan is recorded in
`auth-session-frontend-native-qa-plan.md`. The automatic actual-main/Chrome/UI
test must keep native activation closed and use no operator gesture, helper or
registration; the physical `foreground_only` scenario retains the original
real click/native foreground evidence and ends before lifetime assertions.
Pending results are not passes. The old combined gate has not passed and must
not be relabeled as completed; the separate gates will report their own scope.

The latest native goal read is now null. The last known cutoff 465,295 and the
unmeasured continuation remain distinct; the one-million task ceiling is not
reset or raised. Preserving accounting is not supported by creating a new goal
with a fresh allowance. The owner explicitly approved continued partial
measurement for the split and one physical test. No new goal/counter is created.

## Separate lifetime and foreground gates passed — 2026-10-14

Both owner-approved gates now pass independently. The historical combined
`reader_open_parent_death` physical scenario is retained; it has not been run to
success and is not relabeled as a single physical end-to-end parent-death proof.

Automatic lifetime: canonical
`.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopmentReaderLifetime`
passed in `frontend-native-separated-lifetime-automatic-window-identity.txt`
(actual-main test 3.29s). It ran the real tagged main, real frontend DOM/worker,
fresh UI/source profiles and real fixture Chrome reader with native activation
closed, zero helpers and zero observed click/request/open/rejection/API native
signals. The UI HWND was initially hidden by private CDP: a read-only exact PID/
Job/class/title lookup supplied its lifetime identity without presentation or
focus. The reader retained a live bootstrap window and paused browser ownership
after its short request lease was released. Active capture/reasoning drained;
SQLite pause was observed with the original collector lease before release.

The controller pinned the actual UI/reader process creation times, root-local
images, outer Job membership and HWND ownership. It terminated only the verified
actual main; both process handles signalled, both HWNDs disappeared and the whole
outer Job was empty before any fallback. Reopened SQLite retained the original
session/run and original queued command
`command_8c5a1f91a60b8704bcd168cb2c6eac94`, exact payload SHA-256
`9226a5e4b6bd0299b3fe2711e6439d5f341fe578cd93e73bbeb092d21c1154e2`,
queued status and null claim fields. Post-death claim was blocked without changing
run/command metadata; capture/login counts stayed one and no headless fallback
occurred. This is real process/window/SQLite lifetime evidence, not foreground
or helper-lifetime evidence.

Startup plumbing failures were isolated before this pass: controller cwd must be
`internal/engine`; sandbox Node scope checks return EPERM for the absent new
profile, while the same fresh-scope validator outside sandbox succeeds. Two
no-Chrome probes document that difference in
`frontend-native-separated-lifetime-sandbox-probe.json`. No scope guard was
relaxed. Escalated execution then exposed the incorrect visible-UI requirement;
the automatic lane now reads an owned hidden HWND, while the physical lane still
requires visible, non-minimized, presented UI. Startup/queue/null-versus-empty/
payload/reopen contracts and actual main/controller builds passed. Log files
retain the failures separately; no failure is reported as a pass.

Physical foreground: fresh preparation for pinned `foreground_only` passed in
`frontend-native-separated-foreground-preparation.txt`, including six Node window
contracts, six-package tagged contracts, ordinary/dev/dual-tag/helper/controller
builds and actual missing/foreign/unregistered/unknown startup rejection. After
operator readiness and one actual physical click, the separate test passed in
`frontend-native-separated-foreground-physical.txt` (32.42s): main 39048/UI 39080
HWND 1902320, collector 22512 retained through drain/SQLite commit, helper 40320
with real Chrome UI parent, reader 31340/HWND 4851136. Original app/relay/content,
configured worker, native call and private broker passed exact foreground/readback,
UI/API completion, durable audit, bootstrap reuse and one source login/capture,
without warm retry. This branch ends before lifetime/multiwindow assertions.
Its `stop` control awaited actual main exit 0 and empty outer Job/both HWNDs gone;
it did not fabricate a generic runner's `stopped` receipt.

Finally-owned cleanup removed the same single approved QA registry leaf. Independent
four-view absence, zero scoped processes, REMOVED receipt and exact/non-reparse
scope checks passed. Four remaining split-test roots (launcher-only, automatic
failed/passing, physical passing) were removed in
`frontend-native-separated-artifact-cleanup.txt`; the other two automatic startup
attempt roots were cleaned by the canonical wrapper after its Job marker. No
profiles, staged binaries or private cues remain from these executions. Stable
logs, redacted HWND/hash checkpoints and synthetic SQL evidence remain. No original
auth, user runtime, installation, commit or push was touched; Git metadata remains
unavailable. Last verified Git base remains 1c7cff9 rather than a fresh HEAD claim.

Completion audit for the revised scope: before/after bootstrap diagnostics and
the normal final-close verified resume have their prior passing physical/Chrome
evidence; the new automatic gate proves live-reader main death; the separate
physical gate proves genuine frontend/native foreground. Preserve these scopes.
Production/installed parity, real-source auth, active media recapture, non-headless
batch and a blocked native-helper death remain excluded. The feature is still
default-disabled and these tests do not activate it in the user runtime.

Budget remains the one-million task override. Latest native goal is unavailable
(null), measured cutoff 465,295 belongs to the former new-baseline goal, and this
approved continuation plus worker coverage is unmeasured. Historical 921,237 and
the earlier unmeasured interval remain separate; no counter/reset/new allowance
was created and no exact total or native enforcement is claimed.

## Git metadata recovered for authorized publication — 2026-10-14

The owner authorized committing and pushing the completed fixture delta. Local
Git metadata was absent, while non-interactive origin readback still reported
`main` at `1c7cff90fd6b5b8ce52a221a00fa0d295c5c414e`. A separate no-checkout clone
at that exact baseline supplied only Git metadata and the baseline index. Its
metadata was installed into the absent `.git`; no checkout/reset/clean touched
working files. Recovered status shows the expected fixture/source/docs delta
and no tracked deletions. Existing unrelated `experiments/` remains local and
unstaged; runtime evidence, profiles and binaries remain excluded. Code hashes
from the successful split-test audit are reused unchanged. Final remote commit
readback is reported with publication rather than inferred from the local commit.
