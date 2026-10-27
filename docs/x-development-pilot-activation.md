# X concurrency development pilot: activation and rollback package

## Preparation boundary

Run `node scripts/prepare-x-development-pilot.mjs` from AkuSidecar. It builds
`auth_session_dev` main and the compatible reader helper into a new project-local
package under `build/x-application-qa-prepared/<id>/`, copies the existing pinned
Chrome for Testing UI and owned Node distribution, packages the native driver,
and invokes only the existing early plan-materialization exit. No database,
browser, retained profile, provider or session transfer is opened. No downloads,
registry writes, main runtime replacement, scheduling changes or login access
are performed. A read-only HKCU host-registration snapshot is retained when
available; unavailable reads remain explicit.

The new instance config explicitly trusts only the packaged UI broker extension
identity; it does not depend on a main-runtime CLI origin override. An existing
early candidate probe loads and validates the config and database schema version,
then exits before Store/provider/browser startup. The experimental selector is
not passed to that probe and no session authority follows from it.

The package contains `activation-package.json`, immutable payload hashes, an
isolated plan/config, `bin/aku-sidecar.exe`, `bin/aku-reader-broker.exe`,
`bin/chromium/`, `bin/node/`, `owner/`, `preflight.ps1`, and `rollback/` backups
of the on-disk main executable/helper/config/provenance. The reasoning provider,
model and assurance settings are copied unchanged. User databases, browser
profiles, cookies and credentials are never copied into this package. Stored
user Auto Update preferences are not read or written; the fresh pilot database
retains the existing enabled default. The baseline is files on disk, not a
claim that the running process matches those files.

Paths and directory identities in the plan are physical pins. Do not move the
package, edit its plan/assets or reuse a consumed instance. Port selection during
preparation is not a reservation. Regenerate preparation if a pin or prerequisite
changes; a passing preparation does not authorize a retry or native execution.

## Read-only verification before approval

From the checkout: `node scripts/verify-x-development-pilot.mjs <package>/activation-package.json`.
Or use the packaged Node and `tools/verify-x-development-pilot.mjs` with that
same manifest. This checks packaged file hashes, the external capture Chrome,
unchanged model configuration and absent/unconsumed database. It opens no DB.
The runtime root is derived from the config path, exactly as in `config.Load`.
All six startup schema assets must be present and inventoried under
`<instance-root>/schemas/`: acquisition-plan, reasoning-result,
semantic-event-resolution, ai-deep-detection, living-topic-snapshot and
living-topic-routing (each with `.schema.json`). Their source bytes are copied
unchanged, pinned, and covered by the clean source gate and source digest.
Verification independently requires this catalog even if a missing schema is
omitted from the file inventory. It checks JSON syntax and root contracts;
it does not compile recursive JSON Schema semantics or construct providers.
Run `<package>/preflight.ps1` to read host-registration and pipe-presence
categories without connecting to the reader pipe or writing registry.

The package is always `executionApproved:false`, `activationReady:false`.
Operational approval remains necessary for all of these separate actions:

1. Confirm the exact package ID/hash, isolated port, preserved models and pinned
   retained QA source. Approve opening the candidate UI/process and isolated DB.
2. Ensure the exclusive `AkuBrowser.reader-activation.11122` pipe is free. Do not
   stop the existing main runtime under preparation authority.
3. Confirm both relevant native-host mappings select the packaged manifest/helper.
   If a mapping points elsewhere, approve the exact registry/runtime switch first
   and retain the prior mapping. `LaunchWindowsNativeHostsDirectly` does not
   replace native-host registration. HKLM/other registry views are not claimed
   covered by the current-user snapshot and must be reviewed if applicable.
4. Separately approve one `auth_token`/`ct0` reader-to-collector derivation, at most
   60 seconds from seed authorization, with cleanup and no retry. No approval is
   inherited from prior QA, this package, HTTP readiness or a process restart.

## Activation after explicit approval only

Use the exact executable and argument array under `launch` in the manifest.
This starts ordinary main-dev composition with the selected real reasoning
provider, an isolated database, fresh UI profile and packaged UI broker. It does
not replace the existing main runtime or import its database/login. Starting the
candidate itself writes isolated state and opens the UI Chrome before manual
`/start`; do not run the launch command during preparation.

After the approved launch, verify health version/instance epoch and
`GET /api/collection/runtime`: X must report disabled, available, and
`explicit-start-required`. Registration/helper/foreground failures stop the
acceptance; do not silently fall back to the old exclusive collector.

Only after the separate transfer approval, send same-origin JSON to
`POST /api/development/x-manual/start` with current `X-Aku-Split-Epoch` and exact
body `{"acknowledgeLimitations":true,"allowOneSessionDerivation":true}`. Observe
strict startup evidence, one persisted capture, reader opening during capture,
status and explicit stop. Use an actual UI broker click for foreground evidence.
No like/reply/post, automatic renewal, reseed, fallback or retry is authorized.

## Rollback after explicit operational approval

1. With the candidate's origin/current epoch, call
   `POST /api/development/x-manual/stop` with `{}`. Verify collector cleanup and
   X admission status. Stop does not implicitly close a user's reader windows.
2. Gracefully shut down the candidate and verify its entire owned process tree
   has exited before releasing files/registrations. Pending or uncertain cleanup
   retains fencing/pins; do not delete the instance or announce success.
3. If registration was changed during the approved activation, restore the exact
   prior values from `rollback/native-host-registration.json` (or the additional
   pre-activation system-view snapshot), then verify readback. Do not overwrite
   a mapping that another runtime has acquired since the approved switch.
4. The current package does not overwrite main-dev files, so an unchanged main
   runtime needs no binary/config restoration. If a later approved promotion
   replaces them, restore only the recorded executable/helper/config/provenance
   backups after all affected processes have exited and hashes are verified.
   Never restore/copy/reset a user database, clear challenge/pause state, change
   login, toggle Auto Update or automatically reopen the one-use pilot plan.

Preparation and rollback-file creation do not grant permission for these actions.

## Failed package retained for evidence (2026-10-27)

Package `fd755aa1904d4f2e80a6c9d25c249b93` failed ordinary startup before
HTTP/UI readiness because its instance root omitted required schemas. Its
successful historical early probe did not exercise provider construction.
The approved handoff was rolled back and the original supervised service,
native-host mappings and reader pipe ownership were verified restored.
No session derivation or manual `/start` occurred. Preserve that package and
its launch/outcome receipts; do not retrofit or retry it. The corrected
preparation creates a fresh package requiring a new operational approval.
Only graceful shutdown is authorized; do not invoke a stop path that can
escalate to forced process-tree termination.
