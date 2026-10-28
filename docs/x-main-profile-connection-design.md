# Main-profile X reader connection

## Approved direction (2026-10-27)

Use the existing AkuBrowser source-sign-in profile in place, with the same
selected Chrome profile-directory. One Sidecar-owned headed browser keeps the
profile and reader windows. Each X batch gets its own disposable collector.
The UI profile remains separate. No profile copy, cookie-database reader,
external browser adoption or second launch on the same user-data directory is
part of this connection.

`capture.xMode: concurrent` plus `capture.xReaderSource: main` selects this
restart-required composition. The existing dedicated-reader composition remains
the default when xReaderSource is absent. Main mode rejects xReaderProfile to
avoid ambiguous resource selection. Configuration is not derivation consent.
The ordinary daily start request still grants only the running Sidecar process;
fresh evidence and Store authority are required for every <=60-second batch.

## Responsibility boundaries

1. **Main Window/Manager:** own the Chrome Job, actual profile and subprofile,
   generation and final shutdown. A process-retention policy prevents replacement
   by headless/native-reader intent; it is not a leaked capture lease.
2. **Private Node transport:** sole CDP byte reader/writer from startup; raw
   cookie/header processing stays in Node. Go receives only typed projections.
   Ordinary Go CDP clients never gain an unrestricted network/evaluation proxy.
3. **Borrowed X adapter:** validate independently pinned main-reader ownership,
   use a dedicated X control target, and own collector generations, startup
   evidence and scoped cleanup. Closing this adapter must not close the main
   browser, its profile lease or unrelated windows.
4. **Coordinator/Engine:** schedule ordinary work, expose backend conflicts and
   keep existing pacing, persistent challenge pause, command/result fences,
   reasoning and Timeline persistence.

The existing transport immediately starts a Go read loop and drops id-less
events. Main mode must replace that transport at process startup, before any
reader begins. Do not attach two readers to one pipe, move a running transport,
or call a raw proxy a sanitized boundary after it already crossed Go memory.
Separate browser/worker lifetime Jobs and retained raw endpoint copies must keep
a Node failure from implicitly closing the main browser through CDP EOF.

## Scope and compatibility

Only X gains the disposable concurrent headless collector. Non-X Browser routes
retain the main extension/host and existing parser contracts. Facebook Quiet is
usable only with an installed typed extraction recipe; arbitrary Runtime.evaluate
is not granted. Instagram/LinkedIn Browser/AkuBridge remain valid even though
the current Quiet target worker supports only X/Facebook. Quiet availability must
be reported accurately so the ordinary Bridge route can be selected explicitly
by the existing collector policy.

Non-X headless selections conflict with a retained main headed owner. Reject the
selection/admission explicitly; do not rewrite saved modes or silently substitute
Browser. Enabling all-source headless concurrency would require additional
source/session adapters and is outside this increment. Auto Update stays enabled
and uses the same scheduling/preferences; incompatible jobs must report the
backend conflict instead of replacing the reader.

Reader proof must use the main Chrome's independently verified Job and creation
identity. Collector proof uses the transport worker's independently owned Job.
Both retain the actual Sidecar parent identity. Do not manufacture a common Job
or waive process/profile/image checks to reuse the old dedicated-reader proof.
Root discovery/attachment/seed controls are serialized; distinct target reads
and existing reader windows remain separate. Capability loss/overflow revokes X.

## Delivery and acceptance

### Ordinary X window actions (offline, 2026-10-28)

In a supported development build, concurrent/main separates `open_source` and
`open_native_post` from batch consent. These actions borrow the existing Browser
through the ordinary Bridge queue and native broker, before batch consent and
after batch Stop. They require the exact retained main Manager shared by Engine
and Coordinator, a ready Browser driver and a current lease generation. Equal
generation numbers from different Managers do not establish ownership. A lost
owner is rejected; it cannot trigger browser replacement or an unowned fallback.

The ordinary native-reader path retains request/URL matching, epoch checks,
cancellation, lease cleanup and independent broker/foreground verification.
Opening a window grants no session derivation or capture permission. Legacy X
capture claims remain blocked; account proof, per-batch Store authority and the
60-second ceiling are unchanged. Dedicated mode retains its existing readiness
gates. Settings, source selection, scheduler state and Auto Update are unchanged.

The HTTP fixture exercises the old Bridge/broker conversation with fake process
and foreground adapters. It does not prove physical foreground or signed-in
capture overlap. The private Node transport still excludes ordinary operations
during exclusive seed phases; this patch changes no transport guard.

The code now composes this mode through main/broker/Coordinator and the daily
Engine/Store ports. It does not activate or alter the running service. The
packaged executable requires a sealed `bin/main-browser-transport/` with
`mainLifecycle:true`, plus the existing pinned `headless-worker/node.exe`.
`buildMainBrowserTransportPackage` in `scripts/build-x-native-owner.mjs` builds
that separate entry. The old dedicated-reader package cannot supply this port.

`scripts/prepare-x-daily-development.mjs --reader-source=main` now selects the
separate v2 delivery contract. Config proposals target the original ordinary
root and select concurrent/main, removing an explicit empty xReaderProfile if
present. Rollback records original field presence/value and requires a matching
operational switch receipt; it never restores a database/profile snapshot.
The preparation verifier checks the main entry/lifecycle/hash, ordinary layout,
minimal proposal, rollback and absent activation authority. The dedicated v1
default is retained for historical callers. Preparation is not main-profile
ownership verification or process consent. Follow the
[activation and rollback plan](x-daily-development-activation.md).

The main-only reader binding shares the existing containment registry; observed
owned marker windows are exempted from capture lowering without issuing an early
foreground ticket. Physical focus and the short-lived activation ticket still
require the normal independent checks. First typed failure and normalized code
are retained separately from cleanup state.

Run `scripts/test-x-main-profile.ps1` for the canonical offline suite. On this
Windows host it needs execution outside the restricted sandbox: the same-user
private-pipe ACL is retained, and the narrow test passed outside that sandbox.
The final run passed 30 Node tests, scoped regressions across seven Go packages
and four actual-main build modes. It exercised real Windows/Node IPC with fake
Chrome endpoints, separate lifetime pins, synthetic Store authority, repeated
batches, cancellation, scoped close and shared reader-registry contracts.
Evidence: `build/x-main-offline-c15a3ea73a2b4c9d93abb657ebaa1fb2/offline.log.txt`.

That offline gate did not qualify actual Chrome, physical foreground or signed-in
main-profile capture. The separately authorized local Chrome receipt below adds
real-process liveness/cleanup evidence without using any signed-in profile.

### Authorized empty-profile Chrome receipt (2026-10-27)

After explicit user approval, one execution of
`scripts/test-x-main-profile-chrome.ps1 -RunChrome` passed. The helper staged a
sealed Node transport and launched installed Chrome on about:blank with a new,
empty project-local profile, extensions/sync/background networking disabled and
DNS resolution blocked. It read only the bounded Browser.getVersion projection,
verified exact main Job/subprofile scope and rejected the wrong subprofile.
After terminating only its fixture Node worker, the same pinned Chrome process
and top-level window remained present for two seconds. Fixture-owned browser
shutdown, complete Job/transport cleanup and deletion of only the pinned empty
profile all passed. No source URL, login, X startup, cookie transfer or retry ran.

Evidence:
`build/x-main-chrome-local-76cc3af0942f4ac594d1c9d2709a7e4b/chrome-local.log.txt`
and its `attempt.json`. Main dev remained PID 39828 with its existing provider;
its config hash remained unchanged. No user database, native-host mapping,
main login, runtime or Auto Update setting was changed. The one-execution
permission is consumed. Physical foreground and signed-in capture are still
unqualified; this local fixture does not grant daily-use activation authority.

1. Land transport/lifecycle ports, main-mode selection and replacement fencing;
   synthetic tests must prove no Go raw network lane, no second owner, bounded
   responses, cancellation and scoped cleanup.
2. Bind borrowed startup/seed/capture to the existing verifier and Store through
   the real main/broker/Coordinator path. Test two batches, stale generation,
   wrong subprofile, reader/collector Job separation, Stop and worker failure.
3. Use new profiles and local synthetic Chrome to validate private endpoints,
   physical owner release and preservation of main windows after worker loss.
   Real account/profile activation and session transfer remain separately gated.

Keep the prepared dedicated-reader package immutable as historical evidence.
The main-profile candidate needs a newly prepared package and resource proposal;
never retrofit the old package or treat this document as runtime authorization.
Rollback revokes X, drains collectors/borrowers, then gracefully restarts the
whole process into exclusive mode. Preserve database, login, provider settings,
challenge state, user windows until shutdown and Auto Update. Uncertain cleanup
blocks handoff and database release; no forced main-runtime termination is added.

Implementation status must distinguish local fixture evidence from a qualified
signed-in main-profile connection. The authorized local fixture is complete;
main-dev activation and actual session transfer remain separately gated.
