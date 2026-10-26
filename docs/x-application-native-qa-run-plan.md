# Native X application QA: preparation and operator plan

## Current readiness

The isolated application entry is published as
`9ce6a9e32ea852252170e77c19653298668727b2`. Reuse its accepted synthetic
evidence; no native selector, browser or transfer has run in this preparation.

`scripts/prepare-x-application-qa.mjs` compiles a development-only application
image, builds the sealed owner package and fingerprints those artifacts plus
the explicit Node/Chrome binaries. It does not execute the application image or
driver, inspect the retained manifest/profile, acquire its lease, create a
database, or start a browser. Invoke it with the project-owned Node executable:

```powershell
.\runtime\dev\headless-worker\node.exe .\scripts\prepare-x-application-qa.mjs
```

Output is under `build/x-application-qa-prepared/<id>`. Its `prepared-review.json`
is deliberately **not** an executable `aku.x.application-qa.v1` plan. It records
`executionApproved:false`, `executionReady:false` and the concrete blocker below.
It cannot be supplied as `--dev-qa-pilot-plan`. Artifact preparation grants no
permission to start or seed either browser. Changes to the compiled graph or
driver invalidate the artifact pins for a later run.

Preparation `21edfb1e39c24536bf4a997a4016878e` completed successfully on the
published application source. The artifact manifest is
`build/x-application-qa-prepared/21edfb1e39c24536bf4a997a4016878e/prepared-review.json`.
It reports no browser start, session transfer or retained-profile inspection.

## One remaining preparation gap

The new entry's reader descriptor must be instance-local. The retained reader
instead lives at `run-x-two-phase-<32hex>/shared` under the project QA parent,
selected by the fixed `x-retained-native-qa.json` manifest. Its existing harness
holds a **manifest-level lock** and a physical root/profile pin, and publishes
`active`/`clean` state. The new owner's `.aku-x-manual.lock` is a different lock.
Using it alone would permit the two harnesses to compete for the same profile.

The minimum next source increment must bind only that fixed retained manifest
to the reader role, share its existing lease and clean-state protocol, and keep
the collector/database in the fresh application instance. Validate the exact
project QA namespace and physical identities. Acquire the retained lease before
owner startup and retain it until verified complete reader-tree cleanup; an
uncertain cleanup must leave the profile fenced and must not publish `clean`.
No generic external-profile option, profile copying, credential copying, fresh
login or Store admission change is required. Extract/reuse the existing lease
mechanism rather than inventing a second competing ownership protocol.

Accountless regression acceptance for that increment: external arbitrary path,
missing/dirty manifest, wrong root/profile IDs, concurrent legacy lease, partial
startup and uncertain cleanup reject; clean in-place reuse and reacquisition
after verified cleanup pass. Use synthetic directories/manifests only. Preserve
the existing native startup proof, original attempt deadline and default-off
selector. The actual retained profile is not needed for these tests.

## Proposed one-shot operator journey after the gap is closed

1. Prepare the corrected, pinned runtime plan with a fresh instance/database and
   disposable collector, referencing the retained QA reader in place. Verify
   executable/package hashes and the clean retained manifest under its shared
   lease. Check that the original main configuration hash is unchanged. Do not
   start the native entry during this preparation step.
2. Obtain fresh permission for exactly one reader-to-collector transfer of only
   `auth_token` and `ct0` using the existing cookie manifest, at most 60 seconds
   from the original attempt start, with no retry. No other cookie, raw body,
   account ID or cookie value goes into the report. Cookie values stay in the
   existing private Node/browser path. No new login is requested in this plan.
3. Start only the isolated development binary. Disabled readiness/status must
   precede the one-use explicit `start` consent. Reuse the existing enrollment,
   seed, paired startup verification, Store permit and capture path. Do not
   reopen deferred identity renewal or rotation qualification.
4. Issue one `open_post` for a reviewed canonical X post while capture is active.
   After the marker window is visibly ready, instruct the operator to click that
   QA window once. The existing read-only foreground check must succeed before
   post navigation. Never automate a focus bypass or hide/close the bootstrap.
5. Collect bounded status/phase evidence and QA observation counts. Require both
   browsers' accepted startup proof, a nonempty result committed once, owned
   post navigation and evidence of actual capture overlap. An `active` pilot
   state alone is not proof that capture was still running at reader opening.
   If overlap was not observed, report it as unproven; do not retry automatically.
6. Stop on the first operational failure, challenge, invalidation or deadline.
   Preserve its first cause separately from cleanup errors. Stop revokes the
   collector while keeping the reader under Sidecar ownership; then close the
   QA parent and verify all owned processes before releasing leases/pins.
   Preserve the retained profile; remove only disposable instance artifacts
   after their cleanup is verified. Uncertain cleanup retains the fence/root.

## Evidence and activation boundary

Record only source/artifact hashes, closed stage/reason categories, durations,
operation counts, QA observation/block counts and cleanup verdicts. Do not
export profile contents or mutate the user database. Main login, runtime and
Auto Update remain untouched throughout.

Native QA success would qualify this specific isolated application journey.
It would not enable ordinary main dev automatically. The next activation review
is the already-selected manual X-only, default-off 60-second pilot with explicit
opt-in/stop and rollback; full production rollout and renewal remain separate.
