# Isolated X application QA entry

## Scope and activation

The Windows `auth_session_dev` build recognizes `native-x-manual-qa` through
`--dev-session-reuse`, together with `--dev` and `--dev-qa-pilot-plan`. The unified
retained-source integration remains default-off: its native selector has not
been executed or qualified here. Ordinary builds reject it before configuration or
database initialization. No normal configuration setting enables the pilot.

The isolated dispatcher runs before ordinary configuration, application shell,
providers, scheduler and native-host setup. It rejects ordinary profile/database
overrides and shell modes. It neither reads nor changes the main Auto Update
preference. Its only HTTP listener uses the plan's explicit loopback port,
excluding the ordinary port 11122; the listener exposes read-only status and a
fixed reader marker, with no HTTP start, settings or mutation endpoint.

Native start additionally requires a one-use stdin `start` control with both
`acknowledgeLimitations` and `allowOneSessionDerivation` true. Selecting a build,
preparing a plan, reading status or reopening the process grants no start consent.
The original manual X-only 60-second attempt, strict startup account/login
checks, Store claim/result fences and cleanup rules remain in force. An actual
native run or real session transfer still needs separate scoped authorization.

## Explicit isolation plan

`XApplicationQAPlan` uses schema `aku.x.application-qa.v1`. It must be an
absolute canonical `plan.json` directly inside `qa-x-pilot_<32 lowercase hex>`.
There is no lookup of a default profile or database and no fallback to main dev.

| Selection | Required binding |
| --- | --- |
| Instance | Matching `instance.json` schema `aku.x.qa-isolation.v1`, instance and physical root identity |
| State | Exact `state` child and its physical directory identity, held by an exclusive instance lease |
| Database | Exact `state/pilot.db`; exclusive creation, or explicit physical file identity and current schema for reuse |
| Reader | Native: fixed retained-QA manifest and trusted checkout, resolved/leased only at start. Synthetic: local profile or an invented instance-local retained manifest. |
| Collector | Exact `profiles/collector/profile`, independently pinned and leased at native owner acquisition |
| Assets | Explicit Node, Chrome and sealed driver paths and SHA256 hashes, held against replacement |
| Listener | Explicit loopback port 1024–65535 excluding 11122 |

Native plans specify `readerSource:"retained-qa"`, trusted `projectRoot` and an
empty reader descriptor. They cannot supply arbitrary external profile paths.
Legacy and application callers share the original manifest lock/state protocol;
the application owner finalizes its lease only after complete verified cleanup.

Aliases/reparse points, hardlinked state files, URI-special paths and incorrect
identities are rejected. Existing WAL/SHM/journal files require separate offline
recovery rather than automatic opening. A current-version runtime marker is
created exclusively if absent and pinned for the database lifetime. The tagged
QA Store opener validates that exact held marker and retains normal schema,
version and foreign-key checks without replacing the pinned marker. Ordinary
Store open behavior and admission policy are unchanged. There is no automatic
migration or downgrade in this entry.

## Reader and lifecycle

Bounded strict stdin controls are `start`, `open_post`, `settle`, `status` and
`stop`. Only canonical X post URLs are accepted. A post open creates a separate
marker window in the already-owned reader; the original Home bootstrap remains
counted and preserved. The marker binds the exact reader PID, Job, HWND,
document and one-use capability. The operator must foreground it manually;
verification must succeed before the sole post navigation. The entry does not
force focus or close user windows on preparation failure. Cancellation, expiry,
failed verification and failed navigation retire the temporary capability.

Stop revokes collection authority while preserving the reader and its active
retained lease under Sidecar. EOF/process shutdown disposes the complete owned
tree. Unknown cleanup
retains instance/profile fences; it never means retry or permission to start
again. Reopening the same clean QA database starts disabled with new consent
required. `settle` rejects an unsuccessful startup without waiting on a capture
that never began.

## Synthetic acceptance

Run from the repository root:

```powershell
.\scripts\test-x-qa-entry.ps1
# Only ordinary Store version/compatibility checks:
.\scripts\test-x-qa-entry.ps1 -StoreOnly
# Compile both legacy staging modes without executing their native tests:
.\scripts\test-x-qa-entry.ps1 -StagingOnly
```

The helper compiles four actual-main variants and executes only
`synthetic-x-manual-qa` in the combined `auth_session_dev auth_session_fixture`
build. Its fresh accountless plan contains a deliberately non-executable driver
stub. A private offline adapter preserves offline provenance and reports
`synthetic:true`, `nativeQualified:false`; it cannot turn public JSON flags into
native authority. The journey covers disabled readiness, explicit start,
owned-reader preparation, exactly-once nonempty observation persistence,
status/stop and clean database reopening. Neither Chrome nor the native selector
is executed. Disposable test roots/binaries are removed; logs remain project-local.

Earlier acceptance `ae6c9295edd7400383dbe8d8caa80ad9` passed eight Engine/post groups
with 24 nested cases, three actual-main groups with nine nested cases, and all
four builds. Windows hardlink rejection cases executed successfully outside the
filesystem sandbox. The ordinary Store regression run
`8fb0760356074539a91e7ef40cbd2fed` passed four groups and ten schema-classification
cases. The unchanged JavaScript actor/package tests previously passed 16 cases.
Evidence is under `runtime/qa/auth-session-sidecar/x-qa-entry-<run>.log.txt`.

This proves synthetic composition and injected ownership/foreground behavior.
It does not prove real Chrome profile release, Windows foreground interaction,
native X capture or frontend/native-message integration through this new entry.
Those require a separately prepared native QA plan and an explicitly authorized
bounded run. Main login, user database/runtime and Auto Update were not changed.

## Unified retained-source composition (2026-10-26)

The earlier layout/lease gap is closed in the working tree. The shared retained
lease module is used by legacy adapters and the application owner. Synthetic
actual-main tests exercise both profile sources through the same application
journey, preserving offline provenance. They verify capture/post overlap at the
backend execution interval, once-committed observations, terminal first failure,
separate cleanup reporting and lease retention through reader-preserving Stop.
Owner and staging regressions protect compatibility with the older callers.

The preparation helper now compiles the dev image and invokes only its early
`--dev-qa-pilot-prepare` exit. This materializes a hashed runtime plan, collector
directory and empty state directory without opening SQLite, resolving the real
retained manifest or starting a browser. The review envelope's execution flags
remain false until separate permission. See the updated
[native run plan](x-application-native-qa-run-plan.md) for evidence and the exact
seed authorization time anchor. No new renewal, timing tuning or Store policy
gate is introduced. Native Chrome/X acceptance remains unexecuted.

Final unified run `611740f32903415ca786b3c57877d64f` passed **29 Go groups and
73 nested cases**, with no skipped cases, all four actual-main build variants,
and both compile-only legacy staging modes. This includes original retained
lease tests, cross-caller exclusion, reader-preserving Stop/parent release,
local/retained actual-main journeys, safe terminal errors and asynchronous first
failure. The source path is complete for this offline acceptance unit.

Owner regression `ddb78b51d6f04998b1939cdd30d49f01` passed 30 groups and
21 nested cases across profile leases, suspended fake-worker Jobs, seed receipts
and existing app-shell/session tests. Existing symlink-privilege cases remain
unavailable on this Windows host; the unified run executed its hardlink cases.
Five application-seed Node fake-clock tests also passed, protecting exact expiry,
no deadline extension, replay rejection and pending-write behavior.
