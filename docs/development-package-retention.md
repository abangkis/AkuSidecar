# Development package retention

Development preparation keeps a complete newest verified package for each contract:
main convergence, daily development, development pilot, and application QA.
Pilot and application QA use the same parent directory but have separate schemas
and retention lanes.

The shared implementation is in scripts/development-package-retention.mjs.
Preparation calls it only after its normal verifier or preparation-only checks
succeed. Loading the helper never starts a runtime or cleans a package.

## Preserved artifacts

Only these manifest-pinned rollback executables can be removed from superseded,
retention-owned packages:

- main: rollback/runtime/aku-sidecar.exe,
  rollback/runtime/aku-reader-broker.exe, and
  rollback/runtime/headless-worker/node.exe;
- daily and pilot: rollback/aku-sidecar.exe and
  rollback/aku-reader-broker.exe;
- application QA: no executable cleanup.

Source snapshots, candidate binaries, database files, profiles, configuration,
manifests, receipts, reports, logs, screenshots, and license files remain.
An old package becomes incomplete for activation/rollback after retirement.
Its original manifest is preserved. A retention-retired.json marker explains
the retirement, and updated verifiers reject it explicitly.

## Protected packages and uncertain state

Retention also keeps packages referenced by a running process, Supervisor
configuration, native-host registration, the main latest pointer, or another
retained package. A retention-pin.json file in a package preserves it
regardless of age. A consumed QA plan, unresolved execution claim, failed
QA result, or unverified cleanup prevents automatic retirement.

Only packages successfully registered by the updated preparation scripts are
eligible. Historical packages, including packages previously cleaned by hand,
are not automatically enrolled. Failed or incomplete preparations remain for
review. Retention failures defer cleanup and report the reason; they do not
authorize a service restart, forced shutdown, new activation, or QA retry.

## Concurrency and verification

An exclusive project-local operation.lock serializes preparation. A stale lock
is never reclaimed automatically; inspect the owning process and incomplete
preparation before any manual recovery.

The Windows guard holds the existing development runtime mutex throughout
cleanup, preventing overlap with the standard build/restart workflow and the
six updated custom activation entrypoints. It refreshes process/service/registry
references, checks all executable handles before the first deletion, and checks
hashes and process references again before each unlink. Unknown observation
state, reparse paths, changed manifests, or changed executables defer cleanup.

Retirement is recorded before the first unlink. Interrupted retirements remain
marked as retiring for manual review; they are never resumed blindly.

State and the latest result are stored in build/.development-package-retention.
The registry contains only artifact identities and hashes, not database contents,
cookies, login state, or full process command lines.

## Runtime rollback retention

The six local release activation helpers take the shared runtime mutex before
handoff. After health, process image, bundle, settings/configuration checks and
the activation receipt succeed, development-package-handoff.ps1 calls the
current source Remove-DevRuntimeRollbacksAfterSuccess implementation.

That implementation keeps the exact rollback created by the successful
promotion. Preparation alone never prunes runtime/dev rollback bundles.

## Entry points and source ownership

The three tracked preparation entrypoints are prepare-x-daily-development.mjs,
prepare-x-development-pilot.mjs, and prepare-x-application-qa.mjs.

The six main package preparation helpers under build/*release-tools and six
custom activation helpers are local, Git-ignored release tooling. Their local
hooks use the shared tracked source helpers. Existing tools copied into sealed
historical packages and source snapshots are unchanged. Any future replacement
of local release tooling must preserve the same verification and retention hooks.

## Validation

Run the focused Node suite from a fresh source checkout with Node.js 22 or newer:

    node --test scripts/development-package-retention.test.mjs

This default suite uses tracked source and disposable fixtures only. No existing
release helpers, prepared packages, runtime binaries, or service registration are
required. It creates its own test output directory under build/; it does not
build an application package. On Windows, the reparse-path fixture creates a
directory junction and requires permission to create that link.

Two integration tests for the local, Git-ignored release tools are explicitly
skipped by default, with a reason in the test output. They cover preparation and
activation hook ordering, and the six main-package retirement verifiers. To run
them on a development machine where those helpers already exist:

    $env:AKU_RETENTION_LOCAL_RELEASE_TEST = '1'
    node --test scripts/development-package-retention.test.mjs
    Remove-Item Env:AKU_RETENTION_LOCAL_RELEASE_TEST

Explicitly enabling these tests requires all six build/*release-tools preparation
and verification helpers, plus their six activation helpers (including
build/convergence-handoff-tools/activate.ps1). A missing helper fails the test;
it is never silently skipped. The tests read hook source and call verifiers only
on disposable fixtures. They never execute preparation or activation entrypoints.
These local tools remain ignored and are not generated or copied into the suite.

On Windows, set AKU_RETENTION_NATIVE_TEST=1 for the disposable-runtime test of
the actual Windows guard, IPC, handle preflight and named mutex. This separate
opt-in needs Windows PowerShell script execution and read access to the process,
Supervisor configuration, and native-host registry observations used by the guard.
It uses the tracked guard and does not require the local release helpers. The
suite creates and removes only UUID-named fixture directories under
build/development-package-retention-tests. Its native observation never starts,
stops, reloads, or activates a service.

Run the existing package verifier suites and scripts/test-dev-runtime-retention.ps1
when changing the associated contracts. Tests never activate the live runtime.


Run scripts/test-development-package-handoff.ps1 on Windows PowerShell 5.1 and
PowerShell 7 to validate the post-verification adapter and mutex interoperability
against disposable bundles. It does not control a live service.

Provenance verification normalizes inventory order before comparing entries.
This avoids culture-dependent ordering differences across PowerShell processes;
all file identity and SHA-256 checks remain required.
