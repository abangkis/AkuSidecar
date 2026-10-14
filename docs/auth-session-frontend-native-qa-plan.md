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
