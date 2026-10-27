# SQLite Go driver: canceled-query leak fix

## Scope

The application dependency changes from the committed `modernc.org/sqlite
v1.29.10` baseline to `v1.57.0`, with its matching `modernc.org/libc v1.74.4`.
The previously tested, uncommitted v1.40.1 selection is superseded. This aligns
the driver with the other current Go projects in the workspace; the driver
version is not a version number of the official SQLite engine.
The embedded engine changes from SQLite 3.45.3 to 3.53.3. The module's Go 1.25.0
minimum, inference SDK v0.24.0, account/Store policy and schema 30 stay unchanged.

The upstream fix closes already-created rows before discarding their reference
when cancellation wins the query-return race. That finalizes the orphan statement
instead of leaving a zombie SQLite connection holding its file. The source of
this race was independently reproduced before the upgrade; no application sleep,
deadline relaxation, unsafe statement sweep or Store cleanup override is used.

Primary upstream evidence:

- [Merge request 81](https://gitlab.com/cznic/sqlite/-/merge_requests/81).
- [v1.40.1 release tag, 2025-11-12](https://gitlab.com/cznic/sqlite/-/tags/v1.40.1).
- [Fixed query cancellation path](https://gitlab.com/cznic/sqlite/-/raw/v1.40.1/sqlite.go).
- [Matching dependency declarations](https://gitlab.com/cznic/sqlite/-/raw/v1.40.1/go.mod).
- [Selected v1.57.0 dependency declarations](https://gitlab.com/cznic/sqlite/-/raw/v1.57.0/go.mod).
- [v1.57.0 query cancellation cleanup](https://gitlab.com/cznic/sqlite/-/raw/v1.57.0/stmt.go).
- [Changes through v1.57.0](https://gitlab.com/cznic/sqlite/-/raw/v1.57.0/CHANGELOG.md).

Module resolution also updates `golang.org/x/sys` to v0.47.0 and the required
go-isatty, go-strftime, mathutil and memory versions. Obsolete transitive entries
are removed by `go mod tidy`. Public modules were obtained through Go's proxy/checksum
service into the project-local module cache. No model/provider selection or
runtime installation is part of this change.

Existing Store DSNs retain `_pragma=foreign_keys(1)` and
`_pragma=busy_timeout(5000)` without new driver options. The upstream statement
cache and cancellation cleanup are exercised through the public API. No
`_defensive`, timezone conversion, vector extension, custom Driver registration
or other optional feature is enabled by this dependency change.

## Regression and compatibility evidence

`scripts/test-sqlite-upgrade.ps1` uses project-owned temporary roots and an offline
module cache. It first compiles identical Store fixture source against the
immutable pre-upgrade module/checksum locks at
`ae97e52a1447da94c07ac28e0488ef89d6b5f0c5` and the upgraded locks. The same
synthetic database is created/written by the old driver, read/written by the new
driver, then reopened by the old driver. Its settings survive and schema remains
30. This proves that specific current-schema driver round trip, not compatibility
with arbitrary older application schemas or an authorization to downgrade users.

The maintained `TestSQLiteCancellationReleasesDatabase` uses public APIs only:
10,000 scheduled cancellations per run, accepting only cancellation/interrupt
outcomes or the correct successful result, followed by ordinary query reuse,
Close, rename, reopen/read and removal. Five focused runs plus the full Store
suite passed. Scheduling is stress-based, not deterministic proof of every race.
Windows file release is explicitly checked; other OS file semantics differ.

## Current v1.57.0 acceptance (2026-10-27)

- `go mod verify` passed for the selected module graph. Go minimum remains
  1.25.0; the test toolchain is go1.26.10/windows-amd64 with CGO disabled.
- `runtime/qa/auth-session-sidecar/sqlite-upgrade-85dc2bdfd7294c2e9388f4bc646cc5a9.log.txt`:
  old/new/old same-database compatibility reports SQLite 3.45.3 -> 3.53.3 ->
  3.45.3, preserved settings and schema 30 at every step. Five focused
  cancellation/file-release stress runs (50,000 attempts total), the full default
  Store suite, combined-tag synthetic guard checks and owned-process primitives
  passed. Specialized cross-build/live/performance helpers were not supplied;
  their conditional skips and unavailable Windows symlink privileges are not
  passed assertions.
- `runtime/qa/auth-session-sidecar/x-development-entry-f9b32fb29dda45139add90e72c2bec6d.log.txt`:
  67 top-level groups and 59 nested cases passed with no skips/failures. All four
  actual-main build modes compiled. Synthetic start/capture/reader/status/stop
  and ordinary loopback bootstrap were checked without a browser or native
  session transfer. Loopback bootstrap ran outside the sandbox. This is
  integration-worktree evidence: the ordinary development entry and its helper
  remain a separate, uncommitted increment, not part of the SQLite-only commit.
- Both test helpers removed their owned temporary run directories. No binary
  was promoted to `runtime/dev`, and the main config hash below is unchanged.
- The SQLite-only staged source was independently exported under
  `build/sqlite-commit-review-264e3fd42a614d6ca8caf32d4ce4143f/source/` before commit,
  excluding the unrelated development-entry and source-cache edits. Module
  verification, the full Store suite (273 top-level passes, 96 nested passes,
  five conditional skips) and all four actual-main builds passed offline.
  `../store.log` contains that scope's cancellation/file-release result. Its
  conditional live/cross-build/performance helpers were not executed.

## Historical cancellation investigation

The original investigation repro, retained under ignored `build/`, was also run
unchanged five times on the initial v1.40.1 upgrade. Its diagnostic-only inspection found
**zero orphan statements** in all five runs; Close and Windows rename succeeded.
That private-field inspection is not included in production or the maintained
regression suite. Before the upgrade it found an orphan `SELECT 42` and a sharing
violation despite Close reporting success.

Historical v1.40.1 evidence (2026-10-26; not v1.57.0 acceptance):

- `runtime/qa/auth-session-sidecar/sqlite-upgrade-0d307d16983747bc8f6ec3ea67b1fda3.log.txt`:
  old/new/old database round trip, five cancellation stress runs, full default
  Store suite, combined-tag synthetic guard checks and ownership/transport tests.
  Specialized cross-build/live/performance fixture probes remain skipped when
  their helper inputs are absent; symbolic-link alias checks remain unverified
  without Windows privilege. No skip is presented as a passed assertion.
- `runtime/qa/auth-session-sidecar/sqlite-upstream-proof-54e0cdc6a03e40beb784f470cef4b755.log.txt`:
  original repro five times, zero orphan statements, verified file release.
- `runtime/qa/auth-session-sidecar/x-development-entry-2c51506b60e44791831e52d0dca6640a.log.txt`:
  ordinary-development synthetic start/capture/reader/status/stop and four actual
  main build modes; bootstrap reaches readiness without native start. Loopback
  bootstrap ran outside the sandbox.
- `runtime/qa/auth-session-sidecar/x-qa-entry-d338b5255479471b8ec228d767545800.log.txt`:
  both legacy retained manual/capture modes compile; native tests not executed.

The SQLite driver fix does not replace the Engine's separate worker-drain fix.
Completed-flow tests still wait for actual reasoning completion; canceled-query
ownership now has its own driver/file-release regression.

## Old acceptance snapshot removal (2026-10-27)

The user separately authorized removal of the three old source snapshots that
declared v1.29.10. These exact ignored directories under
`AkuBrowser/acceptance-state/legacy-build/` were checked for path containment and
reparse points, removed, and verified absent:

- `rc7-clean-source-20260914/`
- `rc6-clean-source-20260914/`
- `hybrid-trial-20261004-clean/`

The live AkuBrowser/AkuBridge/AkuSidecar checkouts and their Git administration
directories were preserved. Other snapshots/builds and historical roadmap
evidence were not removed. This cleanup does not claim the removed artifacts
remain reproducible at those local paths.

## Runtime boundary

No browser, real session derivation, retained/main login, user database, registry,
main-dev binary/config, runtime restart or Auto Update setting was changed.
The main config hash remains
`52063d73180a8f6ab3e514d89cfaa81fb168e0b98381e58fa7ff1ae28d7aadae`.
The upgraded dependency is source/tested only; deployment and main-dev activation
remain separate operations requiring their existing explicit approval.
