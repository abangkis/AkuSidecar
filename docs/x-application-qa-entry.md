# Isolated X application QA entry

## Scope and activation

The Windows `auth_session_dev` build recognizes `native-x-manual-qa` through
`--dev-session-reuse`, together with `--dev` and `--dev-qa-pilot-plan`. This is
source implementation only: the native selector has not been executed or
qualified by this increment. Ordinary builds reject it before configuration or
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
| Reader | Exact `profiles/reader/profile`, with physical root and profile directory identities |
| Collector | Exact `profiles/collector/profile`, independently pinned and leased at native owner acquisition |
| Assets | Explicit Node, Chrome and sealed driver paths and SHA256 hashes, held against replacement |
| Listener | Explicit loopback port 1024–65535 excluding 11122 |

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

Stop revokes collection authority while preserving the reader under its Sidecar
parent. EOF/process shutdown disposes the complete owned tree. Unknown cleanup
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

Acceptance `ae6c9295edd7400383dbe8d8caa80ad9` passed eight Engine/post groups
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

## Retained-profile preparation finding (2026-10-26)

The v1 layout accepts only the instance-local `profiles/reader/profile`. The
retained QA harness instead uses the fixed `x-retained-native-qa.json` manifest,
`run-x-two-phase-<32hex>` owned root and its `shared` profile, with a
manifest-level lease.
The application owner's root lease is not a replacement for that shared lease.
Consequently, v1 cannot yet reuse the retained login in place. A plan that simply
substitutes that external path is correctly rejected before opening the database.

Before a retained native run, add a narrowly scoped reader binding to the fixed
retained-QA manifest and the existing exclusive lease/clean-state protocol. Keep
the collector and database instance-local, retain physical identity checks, and
hold the retained lease until complete owned-reader cleanup. This is preparation
work, not a request to copy/move a Chrome profile, copy credentials, create a new
login, or relax arbitrary-path admission. The retained binding is not implemented
or qualified by this increment; do not run the native selector against that
profile. A source-only synthetic increment must close this specific gap first.
