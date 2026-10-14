# Frontend/bridge QA acceptance audit

Checkpoint: 2026-10-14, source base `1812d98`, uncommitted continuation.

The full goal remains open. A closed native transport is useful preparation and
failure-path evidence; it does not replace a frontend-to-reader handoff.

| Requirement | Current evidence | Status |
| --- | --- | --- |
| Actual frontend and split UI relay | Real Server assets, Store-seeded Timeline and owned CfT DOM/readback in `frontend-preparation-final.txt`; reviewed app-only PNG | Proven at preparation scope |
| Separate QA instance, fresh profiles, synthetic login | Recorded headless preparation used an actual tagged main/CfT run, fresh UI profile and isolated Store/authority; one synthetic login | Proven at that scope; native physical run pending |
| Ordinary builds stay closed | Three actual-main builds, four early CLI rejections before ordinary startup, strict development UI guard; ordinary reader/broker regressions | Proven at recorded contract scope |
| Typed relay/authentication/cancellation | Actual relay bytes and real Server contract; credentials/epoch/source/request validation, native 409, rejected audit and correlated cancel before borrow | Proven by contract; physical browser request still pending |
| Trusted human click on rendered link | Final monitor trial passed the actual-click receipt and correlation checks after the explicit cue; no simulated input | Proven through native helper, later drain gate failed |
| Frontend invokes Chrome-launched native helper | Final monitor trial passed exact helper image, direct UI parent, Job, creation-age, retained handle and topmost-release checks | Proven at helper invocation scope |
| Actual frontend reader foreground and ownership | Separate DEV-3 Win32 native trial exists, but it is not this frontend composition | Not proven |
| Frontend pause/drain/verified resume | Native denial runs before borrow; old native/Engine proof remains separate | Not proven |
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
