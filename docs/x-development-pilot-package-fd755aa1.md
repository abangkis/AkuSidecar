# Prepared X development activation package (2026-10-27)

Package: `fd755aa1904d4f2e80a6c9d25c249b93`.
Root: `build/x-application-qa-prepared/fd755aa1904d4f2e80a6c9d25c249b93/`.
Manifest: `activation-package.json`.
Manifest SHA-256: `8d0dbac83dda29c4f1a8a2cef550dfdadf9456eb96514802e158613bf367d77e`.

## Payload and identity

- The Windows main binary is built with `auth_session_dev` only at source
  revision `867c54e963bf9c8e8b9e13d91893babf5baca2ad`, using SQLite driver v1.57.0.
- Go/UI input paths were checked clean against that revision before building.
  The full checkout remains dirty because unrelated config/source-cache work was
  preserved; Go build metadata correctly reports `vcs.modified=true`. Binary
  hashes and the scoped source digest, not the commit alone, identify this build.
- The native driver has its own embedded module/capture-asset hash manifest.
  Packaged Node is 24.16.0; the existing UI Chrome for Testing is 152.0.7977.54.
  No download or browser process was performed.
- The pinned isolated port is 60241. The manifest carries the exact instance
  plan/config/database paths and launch argument array. The port is not reserved;
  recheck availability at activation.
- The same reasoning provider/models/assurance and pacing config were preserved.
  No user settings/database/profile or credentials were copied. Four on-disk
  main executable/helper/config/provenance backups and a read-only HKCU
  registration snapshot are present under `rollback/`.

## Verified preparation boundary

Eleven accountless package regressions passed. Both the source verifier and the
independent packaged verifier checked 331 payload files and the pinned external
capture Chrome. The four baseline main files remained byte-identical. The pilot
database and UI profile are absent; retained QA session/profile access was not
performed. Candidate invocations were limited to the existing early
`--dev-qa-pilot-prepare` and config/schema-probe exits, not ordinary startup or
`/start`. The ordinary config/schema probe passed with config version 1 and
schema 30, while leaving the database absent. It found a missing UI-origin
override in the first config candidate; the final isolated config now explicitly
trusts the packaged reader-broker identity. Main configuration was unchanged.

Read-only preflight outside the sandbox reported:

- Google Chrome HKCU native-host mapping: `other_runtime_registered`.
- Chromium HKCU native-host mapping: `other_runtime_registered`.
- Reader pipe name present: `true`; its owner/process identity was not inspected.
- Rollback registration snapshot: `observed`, two current-user entries.

These observations are time-specific prerequisites, not authority to change the
registration or stop its owner. Sandbox registry reads had been unavailable;
the superseded preparations were discarded after checking their owned paths,
absence of database/UI startup and ordinary-directory boundary. The current
package retains the successful read-only snapshot and validated config.

The package remains `executionApproved:false`, `activationReady:false` and
default-off. No user runtime, login, Auto Update, user database, registration or
model selection was changed. Preparation files are not committed/pushed by this
task. Existing pre-change synthetic main-dev acceptance remains separate from
this native package; this task does not rerun or extend real-X acceptance.

## Next operational decision

Review the package and [activation/rollback runbook](x-development-pilot-activation.md).
Before running it, approve the concrete candidate UI/database launch and the
coordinated native-host/pipe handoff. A separate fresh one-use session-transfer
approval is required for `/start`, followed by the existing bounded native
acceptance. Do not infer either approval from preparation success.
