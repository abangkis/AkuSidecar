# Frontend/bridge QA acceptance audit

Checkpoint: 2026-10-14, source base `1812d98`, uncommitted continuation.

The full goal remains open. A closed native transport is useful preparation and
failure-path evidence; it does not replace a frontend-to-reader handoff.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| Actual frontend and split UI relay | Real Server assets, Store-seeded Timeline and owned CfT DOM/readback in `frontend-preparation-final.txt`; reviewed app-only PNG | Proven at preparation scope |
| Separate QA instance, fresh profiles, synthetic login | Corrected physical trial used actual tagged main/CfT, fresh source/UI profiles, isolated Store/authority and one synthetic login | Proven at synthetic physical scope |
| Ordinary builds stay closed | Three actual-main builds, four early CLI rejections before ordinary startup, strict development UI guard; ordinary reader/broker regressions | Proven at recorded contract scope |
| Typed relay/authentication/cancellation | Real contracts plus corrected physical app/relay/native completion; subsequent duplicate/stale/cancelled API denials ran before the multiwindow failure | Proven at those boundaries; session-change physical scope remains open |
| Trusted human click on rendered link | Corrected physical trial passed actual click correlation and terminal app/relay/API acceptance after the explicit cue; no simulated input | Proven at synthetic physical scope |
| Frontend invokes Chrome-launched native helper | Final monitor trial passed exact helper image, direct UI parent, Job, creation-age, retained handle and topmost-release checks | Proven at helper invocation scope |
| Actual frontend reader foreground and ownership | Corrected physical trial accepted exact PID/HWND foreground, bootstrap reuse, terminal app/API and durable audit | First handoff proven; multiwindow retention remains open |
| Frontend pause/drain/verified resume | Three pause/PID/driver/lease/work phases and actual foreground after drain passed; trial failed at remaining-window predicate | Pause/drain proven; final-close verified resume remains open |
| Failure handling and cleanup | Closed-admission owner/leases/queue contract; graceful headless main exit and outer tree empty before fallback | Proven at these boundaries; physical terminal UI and abrupt frontend-parent death remain pending |
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
