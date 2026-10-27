# Main-dev X handoff: approved backend plan

## Decision and authority (2026-10-27)

The user confirmed Browser as the common collection backend and independent
headless concurrency for X. The backend choice is settled; it does not need to
be asked again while this scope remains unchanged. This instruction authorizes
alignment of the handoff/rollback plan only. Activation, live settings writes,
session derivation/transfer, runtime restart and registry/service mutation remain
unapproved and unexecuted. Login and Auto Update must remain unchanged.

The planned settings change is only `collectionMode: headless -> browser`.
The planned restart-required config remains `capture.xMode: concurrent` and
`capture.xReaderSource: main`, without xReaderProfile. Instagram/LinkedIn use
Browser/AkuBridge; X uses the independent disposable headless collector after
explicit process consent. Keep the active sources and all other preferences.
The mode selection does not authorize account access or issue Store authority.

## Artifacts and baseline freshness

The immutable candidate remains:
`C:\WorkspaceCodex\AkuWorkspace\AkuSidecar\build\x-daily-development-prepared\53c7a2a805044f0b9130d236b58bac2f\daily-package.json`.

Its manifest SHA-256 remains
`922b5dbf1171586aaff3a4652599ccef7a372daffa00507a2f615ad0122d777a`.
Do not edit the sealed package, its copied activation guide or pinned preparation
inputs to record this decision. This document supplements the
[package activation guide](x-daily-development-activation.md).

The original [preflight plan](C:/WorkspaceCodex/AkuWorkspace/AkuSidecar/build/x-main-handoff-preflight-3b5d5053136e4604955618dd7304a736/HANDOFF.md)
and its hashed observations remain historical and unchanged. Its pending backend
question is superseded by this approved choice. Its PIDs, active-session state,
settings, native-host mappings and service revision are snapshots, not current
operational authority. Refresh those facts before any approved action.

Decision/settings/rollback records:
`C:\WorkspaceCodex\AkuWorkspace\AkuSidecar\build\x-main-handoff-plan-814a7951224146f4b9d550263ce397fb`.
The records distinguish backendChoiceConfirmed from applyApproved and
executionApproved; the latter two remain false. No source code or runtime
configuration is changed by these records.

## Ordered handoff after explicit operational approval

1. Reverify the package, ordinary config and service baselines. Refresh current
   process/creation identities, watcher state, settings, native-host mappings and
   port/pipe state. Inspect candidate/database compatibility under the approved
   operational gate before any live setting change. Stop on rejected or unknown
   compatibility; never reset the database to pass the check.
2. Reserve a quiet Supervisor source-change window. Verify no pending/active
   rebuild or watcher/Supervisor identity change. Do not stop the Supervisor
   watcher, which can shut down other services. Auto Update stays enabled.
3. At an idle boundary, read fresh full settings through the supported API.
   If collectionMode is headless, record its actual before value and change only
   that field to browser. If already browser, record a no-op. Stop and refresh
   the plan for an unexpected mode. Never PUT a partial settings projection:
   preserve every other fresh field, check for observed concurrent edits, and
   re-read after the write. The earlier snapshot is not a settings write body.
4. Recheck idle and identify the current UI window. The existing graceful route
   is an operator close of that AkuBrowser UI. Wait for verified root/descendant
   exit, profile release, port 11122 release and reader-pipe absence. Do not use
   Supervisor Stop, restart-dev.ps1, forced process termination or lock removal.
   If any release is uncertain, stop before further changes.
5. Apply the reviewed config proposal against the unchanged baseline. Update
   the service executable only, preserving ordinary cwd/args/config root and
   database. Supervisor registration requires a fresh, stopped-state,
   revision-bound draft and the human's interactive approval command.
6. Compare and switch only approved effective native-host mappings to the
   packaged helper manifest. Record exact pre/post key/default presence, kind,
   value and view. Use the fresh complete HKCU/HKLM snapshot, not an unavailable
   historical observation; leave absent HKLM mappings absent. Reject conflicts.
7. Start the candidate through Supervisor and verify its image/epoch, sole
   ownership, same profile/database, effective Browser setting, same active
   sources/provider and unchanged Auto Update. X must remain
   process-consent-required with derivationAuthorized=false. Do not transfer
   merely to test startup.
8. Only after explicit running-process derivation consent, enable X's daily
   controller. Fresh batch account/login/ownership/Store checks and the original
   60-second limit per batch remain. Prove two X batches, reader overlap, normal
   Timeline persistence, non-X Browser capture, scheduled Auto Update and Stop.
   Failure, challenge or uncertain cleanup must not trigger an automatic retry.

Capture exact actual settings and every successful mutation in an activation
receipt. Record whether this handoff actually changed collectionMode. This
supplement does not add a live settings applier or pretend an API transaction
exists. If drift cannot be excluded, stop for a conflict decision.

## Rollback by reached phase

- **Before the old runtime exits:** if only collectionMode changed, use that
  original running runtime's supported API to restore the actual before value
  when the current value still matches the receipt. No service restart is needed.
- **After old exit, before candidate startup:** restore only config/service/host
  changes that actually completed, with receipt comparisons. Restore/start the
  original runtime, then restore collectionMode if this handoff changed it.
- **Candidate started without process consent:** gracefully close it and verify
  all releases, then restore the original runtime and setting. No consent/start
  request or session transfer is needed merely to roll back.
- **Candidate with process consent:** revoke derivation and join collector
  cleanup first. Preserve first failure separately from cleanup uncertainty.
  Only then gracefully close, verify release and restore the original runtime
  and setting. Uncertain cleanup blocks old-owner startup on that profile.

For config, use the package rollback-proposal.json to restore exact original
presence/value of xMode, xReaderSource and xReaderProfile. Never leave
xReaderSource=main with exclusive mode. Restore registry/service only against
their actual post-switch receipt and the fresh complete before snapshot.

For settings, re-read the original runtime's full current settings, compare the
changed field, and restore only the recorded actual before value. The earlier
headless snapshot is the expected baseline, not an unconditional rollback value.
Skip mode restoration if the handoff recorded a no-op or the value is already
restored. Stop on conflicting edits and preserve all unrelated preferences.

Never restore a database/profile snapshot, clear challenge/pause state, alter
login or toggle Auto Update. If the original binary rejects current Store state,
stop for a compatibility decision.
