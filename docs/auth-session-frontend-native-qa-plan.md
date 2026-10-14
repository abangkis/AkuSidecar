# Proposed full frontend/native QA gate

Status: one temporary QA host authorized, 2026-10-14; implementation/preparation
must finish before application. No host was registered, no native-capable QA UI
was launched and no user host was invoked at this approval checkpoint.
The current closed-native frontend fixture is partial evidence, not full acceptance.

## Pinned browser discovery boundary

The source review for CfT `152.0.7977.54` resolved tag commit
`24072c1aa400ec4a89dc738b6b6acd12a8589b6f`. Windows discovery is in
[`launch_context_win.cc`](https://chromium.googlesource.com/codesearch/chromium/src/+/refs/tags/152.0.7977.54/chrome/browser/extensions/api/messaging/launch_context_win.cc):
HKCU precedes HKLM, the 32-bit view precedes 64-bit, and a manifest must have an
absolute path. Chromium branding additionally checks Chromium's vendor path;
the Google Chrome path remains the fallback. No profile-local manifest lookup
or supported private manifest-directory override was found. Chrome's
[native messaging documentation](https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging)
also specifies Windows registry discovery.

Direct Windows EXE launch is controlled by `kLaunchWindowsNativeHostsDirectly`
and is disabled by default. A managed `NativeHostsExecutablesLaunchDirectly`
preference overrides the feature. The QA proposal uses the process-local flag
`--enable-features=LaunchWindowsNativeHostsDirectly`; it must still prove actual
direct Chrome parenting/Job membership. A disabling policy is a failure, not
authorization to change policy. See the pinned
[`native_process_launcher.cc`](https://chromium.googlesource.com/codesearch/chromium/src/+/refs/tags/152.0.7977.54/chrome/browser/extensions/api/messaging/native_process_launcher.cc)
and [feature definitions](https://chromium.googlesource.com/codesearch/chromium/src/+/refs/tags/152.0.7977.54/extensions/common/extension_features.cc).

## Exact proposed operation scope

The owner explicitly approved one temporary, uniquely named per-user QA native-
host registration with the answer "Izinkan satu host QA sementara". This is a
bounded exception to the previous no-registration boundary. It does not authorize
other global registration, installation, policy changes or user runtime/auth work.
Read-only parent checks confirmed the Google Chrome NativeMessagingHosts namespace
exists in both HKCU registry views. No parent creation is required.

- Generate one host name `com.akubrowser.qa_reader_<32 lowercase hex characters>`.
- New leaf only: `HKCU\Software\Google\Chrome\NativeMessagingHosts\<that exact host>`.
  Refuse any collision in HKCU/HKLM and either registry view. Never replace an
  existing host. Do not create or remove registry ancestors; if the required
  parent key does not exist, stop and report that additional scope.
- Default value: absolute path to that host's JSON manifest beneath one fresh
  direct `run-*` child of `AkuSidecar/runtime/qa/auth-session-sidecar`.
- Manifest: matching unique `name`, `type: stdio`, absolute root-local
  `aku-reader-broker.exe` path, and only
  `chrome-extension://dlibmmlopdahibfniinemhnghlifiple/` in `allowed_origins`.
- Root-local actual main remains `akusidecar.exe`. The manifest, both binary
  hashes, root, host name, registry view/value and private transport are recorded
  in a task-owned ownership receipt before applying the key.
- Private pipe: `\\.\pipe\AkuBrowser.fixture-reader.<32 lowercase hex characters>`.
  The fixture environment is inherited through the owned QA UI. Invalid/missing
  private pipe fails closed; no production pipe fallback is permitted.
- Stage native-capable UI broker source with existing key/content provenance,
  exact ephemeral loopback origin and the unique QA hostname. Its native call
  remains real; do not use the current denial substitution in positive mode.
- All profiles/login/SQLite/source pages are new synthetic QA data. No real
  authentication, runtime restart, installation or production key is changed.

`scripts/register-reader-broker-dev.ps1` must not be used: it targets the
production host name and multiple vendors. Any future QA helper must default to
preview, validate the entire receipt, and require explicit application approval.

## Positive acceptance required after an approved mechanism exists

1. Verify native host discovery and actual helper launch from the owned Chrome UI,
   with direct parent, exact root-local image, UI Job membership and creation age.
2. One physical click on the real Store-rendered frontend link correlates through
   original content/relay/API/native helper/private broker. Reuse source validation,
   native ticket/property/exact HWND/PID/foreground readback without weakening it.
3. Reader priority closes admission, drains active capture and related reasoning,
   persists pause, then releases the old profile owner before opening the reader.
4. Bootstrap and additional windows retain ownership until the final close.
   Resume requires matching synthetic account/profile/session and a new epoch/
   generation for never-claimed work. No second source login is permitted.
5. Exercise cancellation/stale/duplicate/session changes and parent shutdown;
   verify owned roots disappear and persisted queue/claim fences remain correct.
6. After all owned trees are empty, delete only the newly created QA leaf if its
   value and contents still match the receipt. Modified keys are retained and
   reported. Existing host keys, registry ancestors and user runtime remain intact.

After temporary-host approval, the next physical trial targets the positive
frontend/native path. The earlier closed-native physical trial is superseded;
its existing contract evidence continues to cover rejection before borrow.
The full goal remains open until every required boundary above has direct
evidence. Keep the native task root and receipt until the owned processes stop
and the unchanged QA registry leaf is removed, then remove staged files.

## Historical prepared operation — superseded, roots cleaned

Native preparation passed in `runtime/qa/auth-session-sidecar/frontend-native-preparation-ready.txt`.
The reviewed retained root is
`C:\WorkspaceCodex\AkuWorkspace\AkuSidecar\runtime\qa\auth-session-sidecar\run-067f89ec9bd5442799a7efcc50114043`;
receipt: `qa-host-receipt.json`; host:
`com.akubrowser.qa_reader_bee77dfa482c42dfaabf792bccdd429b`.
Manifest SHA256: `2a9260cbf93a87309fdf78cc7d5a1724802147b87da38f60055d71d730bc1118`;
helper SHA256: `3c77717d2f64c449434bae2301d197fac3d4c9d3b0159597eae2b9aa6c8d10c4`;
main SHA256: `3f4f592cfc43086523fedf459ecdf4ab97b244bf0ca5243d3f5b37122371e9d7`.
Root preview confirmed exact receipt/hash/collision scope and zero QA peers.

From the repo root, use the exact receipt/root above:

1. `scripts/qa-reader-host.ps1 -ReceiptPath <absolute receipt>` previews only.
2. After operator readiness, add `-Apply` for the single approved leaf.
3. `scripts/test-auth-session-sidecar.ps1 -VerifyDevelopmentFrontendNative -DevelopmentFrontendNativeTaskRoot <absolute root>` runs the physical/native journey.
4. After independently confirming the owned trees are empty, registrar
   `-Cleanup` verifies contents, canonical key identity and write epoch, then
   removes only the unchanged owned leaf. Finally remove the exact retained root.

The wrapper retains native files on success and failure, restores process-scoped
environment variables, and refuses replay of a physical attempt. The paths above
describe the earlier preparation, not a currently runnable root.

## Latest checkpoint — final limited trial completed, acceptance open

Monitor/presentation preparation passed in `frontend-native-monitor-preparation.txt`.
The last frozen root was `run-63094a8dbb934fc2bbdefb5740e59b77`, using the same
approved hostname. Manifest/helper/main SHA256 were respectively
`1b695712e148d59505150225e7a589d518a9f7b176037fef453e1b77c4719b84`,
`3c77717d2f64c449434bae2301d197fac3d4c9d3b0159597eae2b9aa6c8d10c4`,
`22fa6b7e9855044e5ba5ad4a8ec4644159a985e0c19ff67027d2433bd48a3bd0`.

The actual frontend click and Chrome-launched helper provenance passed, then the
combined pre-drain pause/owner predicate failed. Full reader handoff is not proven.
The authorized one additional physical trial was not repeated. Review the
harness's expectation of a durable marker before releasing active capture against
the binding's immediate admission fence and post-drain SQLite persistence before
proposing a focused repair.

The temporary leaf was removed after processes stopped. Four-view absence and
zero scoped peers were independently checked. All three attempted roots above
and the visibility retry root were deleted after exact-path/non-reparse/ownership
checks; stable logs and minimal synthetic failure evidence remain. No commands
referencing those removed roots should be replayed. See the acceptance audit for
the final failure boundary and partial budget accounting.

## Pause-order correction trial — 2026-10-14

The owner authorized the correction and a new physical trial. The fixture now
separates admission fencing from post-drain SQLite persistence and records
pause/read error/collector PID/Manager PID/driver/leases/work in three phases.
An observation inside the tagged pause binding captures the commit before the
existing session lease is released, with no added lease or blocking handoff.
Focused regression and fresh native preparation passed.

`frontend-native-pause-order-physical-trial.txt` proves all three corrected pause
phases and first actual frontend-to-reader foreground/API/UI completion with one
synthetic login. It then fails the separate remaining-window combined predicate
after bootstrap close. Inventory error/count/bootstrap flag versus Coordinator
pause have not been isolated; no lifecycle cause or full resume claim is justified.
Next scope is that remaining-window evidence, followed by final-close resume and
parent lifetime acceptance. No additional physical retry was run.

The prepared/trial root `run-a71bd2d9496a4ff1a94ae6e2d990e541` was removed after
owned registry cleanup, four-view absence and zero scoped peers. Stable logs and
minimal synthetic failure evidence remain. A future trial needs a fresh root and
operator readiness, using the same approved host namespace; original auth/user
runtime and publication remain excluded. The task continues without a budget;
historical partial accounting is preserved and current measurement unavailable.

## Multiwindow diagnostic receipts ready — 2026-10-14

Authorized step 1 adds typed before/after-bootstrap-close snapshots, separating
inventory error/count/bootstrap identity, Coordinator pause, SQLite read/pause,
current/Manager PID, driver/generation/leases/work, expected reader PID and close
error. The controller logs these receipts before applying the existing window
predicates. Unknown inventory is explicit (-1/no flags), distinct from known zero.
The after receipt survives a failed close operation; no lifecycle fence is relaxed.

Focused contracts and ordinary/dual-tag builds passed; no physical execution or
temporary registration was performed in this step. The actual failure cause is
still unknown. A subsequent authorized trial needs fresh preparation/profiles,
synthetic login and operator readiness, reusing the one approved QA host name.

## Corrected full synthetic journey passed — 2026-10-14

Diagnostic execution showed two windows/bootstrap still present after close while
pause/PID/driver remained valid. The fixture had closed only one page target from
a grouped window. `chrome-window-lifecycle.mjs` now closes the selected window's
known targets, refuses moved targets, verifies window disappearance and requires
natural root exit for final close. Its hash is included in prepared SourceInputs.
Six Node contracts, actual Chrome restart/multiwindow regression and fresh native
preparation passed. The one authorized causal retry passed the full physical
frontend/native journey: bootstrap 2→1 non-bootstrap with pause retained, final
close verified resume with one login, then the existing UI/headless parent-death
and SQLite-fence stage. Logs: `frontend-native-window-close-*.txt`.

Registry absence/zero peers and exact-root cleanup passed for both new trials;
staged profiles/binaries/private cues were removed. Delta remains uncommitted.
Next separate lifetime evidence is main death while the reader is still open;
the current parent-death stage happens after final-close/headless resume and does
not cover that state. All further execution stays synthetic and requires the
appropriate scope/operator readiness; original auth and user runtime stay excluded.

## Live-reader parent-death gate authorized — 2026-10-14

The owner explicitly approved implementing and testing the next fixture-only
case. It is a separate prepared scenario; the passing normal journey remains
the evidence for final-close resume. Scenario selection must be allowlisted,
pinned into prepared metadata and rejected on verification mismatch, with the
same dual-tag/--dev/fresh native receipt/private transport boundary.

The new case stops after actual frontend/native foreground/API/UI acceptance,
while the reader is still present and the original batch's second run remains
never-claimed. Before termination it must prove actual reader windows, browser
driver, expected reader/Manager PID, Coordinator and SQLite pause, one capture/
one synthetic login, normal helper exit, and the original queued run's claim fence.
It must not close reader windows or resume collection to construct this state.

The controller pins actual UI and reader process identities/handles and verifies
their root-local image/creation/Job membership while alive. It terminates only
the pinned actual fixture Sidecar main, then requires reader/UI handles signalled,
both HWNDs gone and the owned outer Job empty before fallback. Reopening SQLite
must retain the same original session/run and durable pause, reject claim, and
show no second capture/login. This is direct live-reader lifetime evidence;
it does not claim a still-blocked native helper was killed, because the helper
must have already completed normally before the death stage.

Use a fresh synthetic root/profile/login and the same single approved QA host;
operator readiness is required before the physical click. Root owns host apply/
cleanup, verifies exact ownership/empty trees/four-view absence, then removes
the staged profiles/binaries/private cues. No original auth, user runtime,
installation, extra registry namespace or publication is authorized.

Prepared scenario usage (both invocations must select the same scenario):

```powershell
.\scripts\test-auth-session-sidecar.ps1 -PrepareDevelopmentFrontendNative `
  -DevelopmentFrontendNativeScenario reader_open_parent_death `
  -DevelopmentFrontendNativeHostName '<the single explicitly approved QA host>'
# After owned Apply and operator readiness, with owned Cleanup in finally:
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopmentFrontendNative `
  -DevelopmentFrontendNativeScenario reader_open_parent_death `
  -DevelopmentFrontendNativeTaskRoot '<fresh retained prepared root>'
```

The second original run uses Store's zero-based ordinal 1. At live acceptance it
can already be `waiting_for_bridge`, with a prepared but never-claimed `queued`
command. The fixture requires that actual queued command and hashes its private
payload; the controller compares original run/session identity and all command
metadata after reopening SQLite and after a blocked claim. Do not introduce a
replacement session or a nonexistent command to manufacture the fence.
Preparation and an ordinal regression pass after a first physical attempt found
the incorrect ordinal-2 assumption. The next causal physical retry still needs
operator readiness and an explicit click cue; the prior accepted click alone
does not prove the parent-death stage.

Three physical attempts reached accepted native reader activation but failed new
fixture assumptions before the parent-death-ready stage: second-run ordinal 2,
never-started run status, then positive active request lease. All are corrected
with focused contracts. The completed reader request normally has zero leases;
known live reader windows and process/Manager identity plus replacement readiness
retain ownership. The final predicate also requires generation and drained work.
Actual pinned-handle/Job/HWND death and original command retention after only-main
termination remain unproven until a fresh physical execution succeeds. All prior
trial/build roots and the approved QA leaf are cleaned; no prepared runnable root
remains. A future execution needs fresh preparation and operator readiness.

## Owner-approved separation of lifetime and foreground gates — 2026-10-14

The owner approved separating automatic lifetime coverage from physical
foreground coverage, then continuing the physical test. Do not ask for a manual
click to debug SQLite, window inventory or parent/child cleanup assertions.

The automatic gate uses the actual tagged Sidecar main, its fresh synthetic
frontend UI, real fixture Chrome reader and the original batch/command. Native
activation stays closed: no native host registration/helper, no DOM/CDP click,
no foreground capability is manufactured. The scoped Engine borrow opens the
reader while active capture/reasoning drains and durable pause is established.
After releasing the short request lease, prove live bootstrap window/owner,
original never-claimed queued command, one capture/login, then terminate only
the pinned main. Pinned UI/reader handles and HWNDs must disappear and the outer
Job must become empty before fallback; reopening SQLite must retain the original
run/command/payload hash/claim fields and pause. No foreground result is claimed.

The physical gate retains the actual app/relay/content/configured worker/native
helper/private broker path and one real operator click. Prove pause/drain,
exact reader foreground/readback, UI/API completion, durable audit, bootstrap
reuse and one login. A dedicated prepared `foreground_only` scenario completes
after those proofs, with owned graceful fixture cleanup; it does not run the
automatic lifetime/multiwindow assertions. Existing `normal` full-journey evidence
remains separately scoped. Do not describe the separate gates as one physical
end-to-end parent-death execution.

Run automatic lifetime before requesting readiness for the new physical gate.
Both use new synthetic profiles/private transport and project-local artifacts;
only the physical gate reuses the single explicitly approved temporary QA host.
Production auth, user runtime, installation and commit/push remain excluded.

Both separate gates passed on 2026-10-14. Automatic actual-main lifetime took
3.29s with native activation closed/no gestures/helper/registration; the exact
owned hidden UI HWND and live reader were pinned and observed dead after only-main
termination, with original SQLite command/pause retained. Physical
`foreground_only` passed in 32.42s with one actual click, full foreground/API/UI
proof and graceful owned cleanup. All split execution roots and the temporary
leaf are cleaned. See the acceptance document for precise evidence and limitations.

Repeat lifetime independently, without requesting an operator click:

```powershell
.\scripts\test-auth-session-sidecar.ps1 -VerifyDevelopmentReaderLifetime
```

For a separately authorized physical foreground repeat, use the existing native
prepare/verify commands above with
`-DevelopmentFrontendNativeScenario foreground_only` on both invocations, fresh
profiles/root and the same explicitly approved QA host. Apply/cleanup remains
Coordinator-owned and readiness/click cue remains physical. The historical
combined scenario and `normal` full journey remain available for their distinct
coverage; do not automatically run them for every lifetime correction.
