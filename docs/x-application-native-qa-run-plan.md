# Native X application QA: preparation and operator plan

## Current readiness

The published base is `29d2d14726af9a22a9c433495c1c32a47af6202b`. One native
trial on its reviewed source committed one observation/two post blocks, but did
not accept reader foreground or final cleanup. The current working-tree increment
corrects foreground preparation and controlled Close ordering offline. It has
not executed natively; native execution remains separately gated.

`scripts/prepare-x-application-qa.mjs` compiles a development-only application
image, builds the sealed owner package and fingerprints those artifacts plus
the explicit Node/Chrome binaries. It invokes the image only with the early
`--dev-qa-pilot-prepare` exit to materialize a runtime plan. It does not select
the native pilot, execute the driver, inspect the retained manifest/profile,
acquire its lease, create a database or start a browser. Invoke it with:

```powershell
.\runtime\dev\headless-worker\node.exe .\scripts\prepare-x-application-qa.mjs
```

Output is under `build/x-application-qa-prepared/<id>`. Its `prepared-review.json`
uses `aku.x.application-qa-preparation.v2` and points to a separate, hashed
`aku.x.application-qa.v1` runtime plan. It records `runtimePlanMaterialized:true`,
`executionApproved:false`, `executionReady:false` and readiness
`offline_ready_pending_native_execution_authorization`. The implementation gap
is closed; actual profile/session validity is deliberately unobserved. Only the
nested runtime plan is suitable for the pilot selector. Git base, dirty-source
status, source digest and binary/package hashes identify the prepared artifacts.
Later code/package changes require new preparation. Preparing grants no consent.

Historical preparation `21edfb1e39c24536bf4a997a4016878e` was review-only before
the shared binding existed. Do not reuse it as a runtime plan.

Preparations `2bd6390be6b04ab98634ce0732e3c0b3` and
`496e9789604e4d57813453e43230466f` were each consumed by one explicitly authorized
native attempt. Never rerun them or reset their execution claims. The second
instance/collector remains retained because application cleanup was uncertain;
the new source correction does not retroactively upgrade its cleanup verdict.
See [the recorded outcome](x-unified-native-qa-fb36e37b.md). Prepare a fresh package
after the current source changes; no new native execution is authorized here.

## Shared ownership and one application journey

The plan selects `readerSource:"retained-qa"` and trusted checkout `projectRoot`,
with a zero/unresolved reader descriptor. Explicit native start resolves only
the fixed `x-retained-native-qa.json` manifest under its original manifest lock
to `run-x-two-phase-<32hex>/shared`. Legacy callers and the application owner
share `development_x_retained_profile_windows.go`, preserving the v1 manifest,
lock name, physical pins and active/clean protocol. Application acquisition is
existing-only; creation remains confined to bootstrap. Arbitrary external
profile selection is rejected.

The application owner acquires the retained lease before launch and owns its
sole finalization. Pre-launch rollback or verified whole-tree cleanup can publish
`clean`. Reader-preserving Stop keeps its `active` lease; uncertain cleanup keeps
fencing. Process death does not silently clear persisted `active` state. The
instance selector cannot release the reader lease. Collector and database remain
instance-local. There is no profile move/copy, login operation or Store change.

Combined-tag synthetic QA uses this same module against an invented manifest
inside its fresh instance (`retained-fixture`), never the real retained manifest.
Its evidence stays offline/non-authorizing for native startup. Tests cover both
caller orders, dirty/missing/replaced profiles, cleanup, Stop/parent release and
the actual-main retained-source capture/post journey.

## Evidence and time semantics

Bounded preflight initializes owners/Home documents. The authorization clock
starts immediately before the one-use seed RPC, after ownership revalidation.
The exact seed anchor supplies broker and Store deadlines; startup, reader and
capture cannot renew its 60-second lifetime. Existing startup/freshness limits
remain unchanged. This names the existing preflight/attempt distinction.

Per-attempt stages use relative times. Overlap describes an **in-flight capture
backend RPC interval** after claim/admission/payload checks, not queued work,
pilot status, reasoning, or individual network/render activity. Reports distinguish
`observed`, `not_observed` and `unknown`, without granting native authority.
The first operation/capture failure is immutable and terminates the one-shot
CLI. Its cause remains separate from the final `qa_closed` cleanup verdict.

Human waiting belongs to the existing bounded preparation phase, before the
five-second foreground capability is issued. The exact Job/PID-owned marker is
observed read-only and recorded as ready before waiting, preserving truthful
capture overlap. Once foreground is observed, revalidate ownership and mint the
short-lived capability. Reverify foreground before navigation; waiting cannot
extend preparation, the ticket, or the original 60-second attempt.

For controlled Close, Engine closes admission/work and drains broker cleanup
and owner disposal before parent cancellation. Real parent death retains its
immediate Job-containment path. Logical abort failure, physical disposal and
profile release remain distinct facts; no missing process alone proves cleanup.

## Proposed one-shot operator journey after explicit authorization

1. Prepare the corrected, pinned runtime plan with a fresh instance/database and
   disposable collector, referencing the retained QA reader in place. Verify
   executable/package and unchanged main configuration hashes. Actual retained
   manifest/lease acquisition is deferred to explicit native start. Do not
   start the native entry during preparation.
2. Obtain fresh permission for exactly one reader-to-collector transfer of only
   `auth_token` and `ct0` using the existing cookie manifest, at most 60 seconds
   from the original seed authorization start, with no retry. No other cookie, raw body,
   account ID or cookie value goes into the report. Cookie values stay in the
   existing private Node/browser path. No new login is requested in this plan.
3. Start only the isolated development binary. Disabled readiness/status must
   precede the one-use explicit `start` consent. Reuse the existing enrollment,
   seed, paired startup verification, Store permit and capture path. Do not
   reopen deferred identity renewal or rotation qualification.
4. Tell the operator before launch to click the marker once when visible. Issue
   one `open_post` for a reviewed canonical X post while capture is active. The
   owned marker becomes observable during preparation, before issuing a ticket.
   Wait for foreground within preparation and the original attempt's remaining
   time, then bind a fresh five-second capability, verify foreground and navigate.
   If insufficient time remains, reject without extending any lifetime. Never
   automate a focus bypass or hide/close the bootstrap.
5. Collect bounded status/phase evidence and QA observation counts. Require both
   browsers' accepted startup proof, a nonempty result committed once, owned
   post navigation and observed backend-execution overlap. An `active` pilot
   state alone is not proof that a backend call overlapped reader opening.
   If overlap was not observed, report it as unproven; do not retry automatically.
6. Stop on the first operational failure, challenge, invalidation or deadline.
   Preserve its first cause separately from cleanup errors. Stop revokes the
   collector while keeping the reader under Sidecar ownership; then close the
   QA parent through ordered Engine/broker stop and owner disposal, then cancel
   its parent context. Verify all owned processes before releasing leases/pins.
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
