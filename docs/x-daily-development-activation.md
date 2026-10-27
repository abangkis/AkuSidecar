# Daily X main-dev activation and rollback

This preparer and its historical payload describe the dedicated-reader route.
The newly implemented [main-profile connection](x-main-profile-connection-design.md)
requires a new candidate containing `main-browser-transport` and an explicit
`capture.xReaderSource: main` proposal without xReaderProfile. Do not retrofit,
activate or infer main-profile readiness from the earlier dedicated package.

## Prepared delivery, not operational approval

This package targets ordinary AkuBrowser development on port 11122, its existing
database, effective persisted provider/settings, Timeline and Auto Update. It is
not the isolated one-transfer QA package. Preparation builds `auth_session_dev`
main, the reader helper, pinned Node, ordinary non-X worker, sealed daily owner
and UI broker into `build/x-daily-development-prepared/<id>/bin/`. It does not
overwrite runtime/dev, launch Chrome, open a database/profile, connect to the
reader pipe, invoke inference, write registry/service configuration or change
Auto Update. Only the early config/schema probe executes the candidate; it exits
before Store, provider, HTTP, browser or daily factory startup.

From the checkout, using the actual Supervisor configuration path:

```powershell
.\runtime\dev\headless-worker\node.exe scripts/prepare-x-daily-development.mjs --supervisor-config=C:\Users\Force\AppData\Local\AkuSupervisor\services.json
```

The manifest records `executionApproved:false`, `activationReady:false`, absent
process consent, payload/source hashes, ordinary resolved paths, a two-field
config proposal, complete proposed service and rollback copies of the on-disk
binary/helper/config/provenance and service definition. The provider declaration
is unchanged; the effective provider will be resolved from the same database at
startup. Preparation does not read the database or claim effective-provider
parity. No database or login/profile backup is made. Typed registry snapshots
cover HKCU/HKLM, 32/64 views, Chrome/Chromium; inaccessible reads stay unavailable.
Pipe presence is observed without connecting; ownership remains unverified.

Committed Go, schemas, broker, worker, fixture/bundle code and actual sibling
AkuBridge capture inputs are pinned. New preparation scripts are hashed
separately and must not be described as committed until published. Unrelated
dirty files are excluded. No temporary QA port/database is substituted.

## Read-only verification and unresolved reader selection

```powershell
<package>\bin\headless-worker\node.exe <package>\tools\verify-x-daily-development.mjs <package>\daily-package.json
pwsh -NoProfile -File <package>\preflight.ps1
```

Verification hashes explicit payload/code/assets and baseline files only. It
does not read databases, browser profiles, cookies, tokens or accounts. Stale
inputs or an existing `activation-receipt.json` reject reuse. That receipt is
written during a separately approved handoff, never by preparation.

The reader profile is intentionally **unresolved**. Before activation, select
an existing canonical dedicated signed-in X user-data directory, distinct and
nonoverlapping with the ordinary capture and UI profiles, with no simultaneous
Chrome owner. Do not copy, adopt, rename, clear or log into the main profile as a
shortcut. Reusing a QA profile for daily use requires an explicit resource
decision. If no eligible profile exists, enrollment requires separate approval.
Finalize selection in a new reviewed package/receipt; never apply the `null`
template value blindly. Preparation does not inspect or create that profile.

The proposal changes only `capture.xMode` to `concurrent` and
`capture.xReaderProfile` to the approved path **in the original config file**.
The service proposal changes only the executable. Its cwd, arguments and actual
config path remain ordinary: config.Load derives its root from that path.
Launching with the copied rollback config would redirect relative state and is
forbidden. No manual/QA selector, provider override or consent flag is added.

## Later handoff, after explicit operational authorization

1. Reverify package SHA, sources and baseline. Resolve the reader choice; refresh
   service revision, actual running image/tree and typed native-host mappings.
   Retain exact pre-switch observations. Database compatibility is checked only
   under the separately authorized runtime gate, not preparation.
2. Coordinate the development watcher so it cannot rebuild/restore the service
   during handoff. Preserve its prior supervision state. Do not change Auto
   Update: source scheduling and process supervision are separate. If watcher
   coordination is unavailable, stop the handoff.
3. Request graceful application shutdown of the identified old main tree. Wait
   for verified UI/capture/worker/reader exit, port 11122 and
   `AkuBrowser.reader-activation.11122` release. Supervisor Stop may escalate to
   forced termination and is not authorized. If graceful exit stalls, abort;
   never force-kill or remove locks/profiles.
4. After release, atomically apply the reviewed two-field config change with a
   matching baseline hash. Register the proposed service using the Supervisor
   human approval workflow. Switch required native-host mappings to candidate
   `bin/com.akubrowser.reader_activation.json`, whose absolute helper path names
   the packaged helper. Preserve key presence, value kind and registry view.
   Do not overwrite mappings acquired by another runtime since the snapshot.
   A switch affecting another runtime requires an explicit conflict decision.
5. Start through the approved Supervisor path, retaining ordinary arguments,
   DB/provider/settings and Auto Update. Verify actual image/epoch, compatible
   helper and sole pipe owner. Before consent, `/api/collection/runtime` must
   show `selectedMode:concurrent`, `process-consent-required` and
   `derivationAuthorized:false`.
6. With current same-origin `X-Aku-Split-Epoch`, explicitly grant this running
   process via `POST /api/development/x-daily/start`:

   ```json
   {"acknowledgeLimitations":true,"allowProcessSessionDerivation":true}
   ```

   The grant ends with Sidecar. Every batch gets fresh verified account/startup
   evidence and Store authority; its original seed clock is capped at 60 seconds.
   Failure/uncertainty stops admission without automatic retry, reseed or
   fallback. Successful batch cleanup retains the reader. The initial MVP uses
   one acquisition round per X batch, preserving item/scroll budgets/evaluation.

## Daily-use acceptance

Prove two ordinary X batches in one process with the same reader and different
disposed collectors; verify real normal Timeline results. Prove a native-post
click can use the reader while capture is active. Observe a normal scheduled
batch without changing Auto Update. Confirm non-X routing/effective provider
stay unchanged. Daily Stop must block a subsequent X batch; graceful exclusive
restart must restore ordinary routing. Offline tests do not prove live outcomes.

## Rollback after operational authorization

1. POST `{}` to `/api/development/x-daily/stop` using current origin/epoch. Verify
   revoked derivation and complete collector cleanup. Keep first failure and
   cleanup uncertainty separate. Stop preserves the reader; repeating start in
   the same process is forbidden.
2. Gracefully shut down the candidate. Verify the entire owned tree, profile
   leases, port and reader pipe are released. Uncertainty retains fencing; never
   delete locks, collectors, profiles or database files manually.
3. Restore mappings only if still matching the switch receipt, preserving exact
   previous value/kind/key-presence/view. Restore the original service through
   the approved Supervisor workflow. Restore only the two config fields if other
   settings changed meanwhile; replacing the exact backup requires a matching
   post-switch hash. Refuse conflicting changes.
4. Restart original service/binary/ordinary arguments. Side-by-side delivery
   leaves the original binary/helper/worker untouched. Verify exclusive X,
   health, sole pipe ownership and effective provider. Restore the watcher's
   prior state only after stable ownership.
5. Preserve the database/observations, challenge/pause records, login, profiles,
   provider preferences and Auto Update. Never restore a DB snapshot or clear
   fences to force progress. If the old binary rejects current Store state,
   stop for a compatibility decision.

Preparation, verification and publication authorize none of these runtime steps.
