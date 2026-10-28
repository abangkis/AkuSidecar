# Daily X main-dev activation and rollback

## Current ordinary development procedure (2026-10-28)

The canonical daily executable is `runtime/dev/aku-sidecar.exe`. Keep the existing
build/restart entry points; concurrency is an optional capability delivered with
that ordinary runtime. The sealed-package migration procedure below is historical
reference and does not define a new service-registration step for every rebuild.

The ordinary helpers support Windows PowerShell 5.1 and PowerShell 7. Bundle
inventory uses a contained descendant path instead of the newer .NET
`Path.GetRelativePath` API, preserving the same target-layout hashes. JSON
manifests/configuration are read as UTF-8 on both shells, including after
promotion, so Unicode filenames do not corrupt provenance checks. Supervisor
status accepts the current `response.services` envelope and the legacy flat
`services` array. Ambiguous/malformed collections, non-unique service IDs and
missing/invalid ownership fields are rejected before shutdown or promotion;
missing data must never be interpreted as a stopped service.

Run `scripts/test-dev-runtime-compatibility.ps1` with both `powershell.exe` and
`pwsh.exe` for the synthetic compatibility checks. The existing
`test-dev-runtime-contract.ps1 -DiagnosticImage <development-diagnostic.exe>`
also exercises copied build/restart preparation, promotion, rollback and tamper
rejection. Its diagnostic processes use only synthetic config/database paths;
it does not restart the registered application. These checks do not authorize
an operational restart or session derivation.

- `build-dev.ps1` compiles the development capability tag and stages both X
  transports from reviewed source, without enabling either mode or granting
  consent. Release compilation is unchanged.
- A `.next` build stages its own Sidecar, broker helper, UI broker assets, native
  host manifest, headless worker and X transports. Provenance pins the complete
  target-layout file inventory; no active helper/transport is replaced.
- `restart-dev.ps1 -PlanOnly` resolves the actual service definition read-only.
  A sealed-experiment target is rejected before a build or registration change;
  it requires a separately approved one-time handoff to the stable path.
- `-PrepareOnly` builds and verifies the candidate, runs the early config/schema
  probe and read-only database inspection with the registered arguments/cwd,
  then exits without process control or registry writes. It reads the database
  selected by those arguments: the current authorized tests use only synthetic,
  absent database paths, never this option against the user runtime.
- A later authorized restart requires matching stable native-host registration,
  unchanged service/application config and graceful release. An existing runtime
  control token may authorize `shutdown-if-idle`; without it the user closes the
  UI gracefully. Timeout retains the candidate and aborts. Supervisor Stop is
  used only after independent no-owned-process, OS, port and reader-pipe release
  checks to normalize an already-exited service; no live-tree force-stop or
  Supervisor restart is permitted.
- Whole-bundle promotion retains the previous complete code bundle. A move
  failure restores all prior components and preserves the first failure;
  incomplete recovery is reported. Health/image/hash failures retain rollback
  evidence rather than force-closing a new process or restoring a DB snapshot.

Do not run these operational steps merely because the source/tests pass. The
current service/native-host still require an approved handoff; this increment
does not perform it. No config, login/profile, user database, Auto Update or
process derivation consent is changed by the synthetic acceptance.

Run `.\scripts\test-stable-dev.ps1` for the offline build/CLI/helper regression.
Its operational entry-point tests execute copied scripts in a synthetic
checkout with fake compilation/stagers and real diagnostic executables; no
registered service or signed-in browser is started.

The next increments adapt optional ownership and X routing to ordinary behavior,
then prove daily use under explicit operational/derivation authorization. See
the [current design sequence](x-daily-development-design.md).

## Historical sealed-package preparation and handoff

The preparer supports two explicit payload contracts: the historical v1
dedicated-reader package and the v2 [main-profile connection](x-main-profile-connection-design.md).
`--reader-source=main` selects v2, containing `main-browser-transport` and a
`capture.xReaderSource: main` proposal without xReaderProfile. Omitting the option
retains the dedicated-reader route; no profile is selected automatically.
Existing prepared artifacts remain immutable. Do not relabel a v1 package as v2.

## Prepared delivery, not operational approval

This package targets ordinary AkuBrowser development on port 11122, its existing
database, effective persisted provider/settings, Timeline and Auto Update. It is
not the isolated one-transfer QA package. Preparation builds `auth_session_dev`
main, the reader helper, pinned Node, ordinary non-X worker, sealed reader transport
and UI broker into `build/x-daily-development-prepared/<id>/bin/`. It does not
overwrite runtime/dev, launch Chrome, open a database/profile, connect to the
reader pipe, invoke inference, write registry/service configuration or change
Auto Update. Only the early config/schema probe executes the candidate; it exits
before Store, provider, HTTP, browser or daily factory startup.

From the checkout, using the actual Supervisor configuration path:

```powershell
.\runtime\dev\headless-worker\node.exe scripts/prepare-x-daily-development.mjs --supervisor-config=C:\Users\Force\AppData\Local\AkuSupervisor\services.json --reader-source=main
```

The manifest records `executionApproved:false`, `activationReady:false`, absent
process consent, payload/source hashes, ordinary resolved paths, a minimal
config proposal, complete proposed service and rollback copies of the on-disk
binary/helper/config/provenance and service definition. The provider declaration
is unchanged; the effective provider will be resolved from the same database at
startup. Preparation does not read the database or claim effective-provider
parity. No database or login/profile backup is made. Typed registry snapshots
cover HKCU/HKLM, 32/64 views, Chrome/Chromium; inaccessible reads stay unavailable.
Pipe presence is observed without connecting; ownership remains unverified.

Main packages carry `schema: aku.x.daily-development-package.v2` and
`readerSource: main`. They include
`bin/main-browser-transport/{manifest.json,main-browser-transport.cjs}` with
`mainLifecycle:true`, not the dedicated `bin/x-daily-owner/` launcher. The v2
verifier checks that source, entry, lifecycle, payload hash, config proposal and
rollback proposal agree. It does not certify account/session or operational
readiness. A newly prepared package is still default-off and grants no consent.

An explicit main refresh also accepts an existing `concurrent/main` baseline,
without a dedicated reader profile. Its config proposal is idempotent; rollback
retains the exact existing field presence/values rather than inventing an
`exclusive/dedicated` baseline. Preparation never applies either proposal.

Rollback provenance uses the ordinary executable's runtime-state file when
present. If absent, the main refresh requires the previous sealed main package
under this checkout's `build/x-daily-development-prepared/<id>/bin/`. Its manifest
must match the directory identity, source commit and build tag, and exactly one
record for each old Sidecar binary, reader helper and native-host manifest must
match the on-disk hash. The original manifest is copied into
`rollback/previous-daily-package.json`; no runtime-state file is fabricated.
The old package's handoff receipt is historical evidence only. New activation,
ownership and process consent remain unapproved, and a receipt on the new
candidate still blocks reuse. Rollback restores the recorded previous version;
it does not implicitly switch an already-concurrent config to exclusive mode.

Committed Go, schemas, broker, worker, fixture/bundle code and actual sibling
AkuBridge capture inputs are pinned. New preparation scripts are hashed
separately and must not be described as committed until published. Unrelated
dirty files are excluded. No temporary QA port/database is substituted.

## Read-only verification and reader selection

```powershell
<package>\bin\headless-worker\node.exe <package>\tools\verify-x-daily-development.mjs <package>\daily-package.json
pwsh -NoProfile -File <package>\preflight.ps1
```

Verification hashes explicit payload/code/assets and baseline files only. It
does not read databases, browser profiles, cookies, tokens or accounts. Stale
inputs or an existing `activation-receipt.json` reject reuse. That receipt is
written during a separately approved handoff, never by preparation.

For **main**, the existing ordinary `--browser-profile` is the reader resource;
the selected Chrome profile-directory is resolved and independently pinned only
at approved runtime startup. Preparation does not inspect Local State, profile
contents, accounts or locks, acquire a lease, or validate live ownership. It does
not launch another Chrome on that resource. `reader.selection: ordinary-main`
describes the proposal, with `ownershipVerified:false`.

The main proposal changes only `capture.xMode` to `concurrent` and
`capture.xReaderSource` to `main` in the original config. If an empty
`xReaderProfile` field exists, the proposal explicitly removes it. A nonempty
dedicated-profile baseline is rejected. The exact presence/value of all three
fields is recorded in `rollback-proposal.json`, including absent fields. Source
Stop revokes X and cleans collectors while the main Window/Manager retains its
reader; only graceful application shutdown releases that Chrome/profile.

Before operational approval, review effective non-X headless selections: they
conflict with the retained main reader. Quiet has no unrestricted evaluator in
main mode, and remains unavailable without typed recipes. Existing collector
policy may choose Bridge. Preparation does not read the settings database,
rewrite source modes or claim that all persisted selections are compatible.

For **dedicated** (`--reader-source=dedicated`, also the CLI default), the reader
profile is intentionally **unresolved**. Before activation, select
an existing canonical dedicated signed-in X user-data directory, distinct and
nonoverlapping with the ordinary capture and UI profiles, with no simultaneous
Chrome owner. Do not copy, adopt, rename, clear or log into the main profile as a
shortcut. Reusing a QA profile for daily use requires an explicit resource
decision. If no eligible profile exists, enrollment requires separate approval.
Finalize selection in a new reviewed package/receipt; never apply the `null`
template value blindly. Preparation does not inspect or create that profile.

The dedicated proposal changes only `capture.xMode` to `concurrent` and
`capture.xReaderProfile` to the approved path **in the original config file**.
The service proposal changes only the executable. Its cwd, arguments and actual
config path remain ordinary: config.Load derives its root from that path.
Launching with the copied rollback config would redirect relative state and is
forbidden. No manual/QA selector, provider override or consent flag is added.

## Later handoff, after explicit operational authorization

1. Reverify package SHA, sources and baseline. Confirm the explicit reader source
   and its resource prerequisites; refresh
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
4. After release, atomically apply the reviewed config proposal with a
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
   the approved Supervisor workflow. For main packages, follow
   `rollback-proposal.json`: restore the original presence/value of xMode,
   xReaderSource and xReaderProfile, and refuse changes conflicting with the
   switch receipt. Never leave `xReaderSource: main` with exclusive mode. Restore
   only these fields if other settings changed meanwhile; replacing the exact backup requires a matching
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

## Synthetic package acceptance (2026-10-27)

The daily/package regression command passed 56 tests (30 daily and 26 isolated
package checks):

```powershell
.\runtime\dev\headless-worker\node.exe --test scripts/x-daily-development-package.test.mjs scripts/x-development-pilot-package.test.mjs
```

Coverage includes legacy v1 compatibility, explicit main selection, actual sealed
transport generation and hash verification, wrong/mixed lifecycle rejection,
ordinary-root/service/provider preservation, absent/present config rollback,
stale baselines, receipt requirements and no implicit activation/consent. The
fixture databases/profiles remain absent; packaged binaries and Chrome are not
launched. Evidence:
`build/x-main-package-offline-f6c6d07b2dd84028be475c8afd9d289c/tests.log.txt`.
These are preparation-contract checks, not an executed runtime rollback or
signed-in main-profile acceptance.
