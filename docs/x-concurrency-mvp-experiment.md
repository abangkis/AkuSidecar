# Minimal X concurrency experiment

Status: documentation-only plan, 2026-10-19. No implementation or test execution
is authorized by this document. Native state export/import requires a separate,
explicit authorization after the exact transfer manifest and implementation are
reviewable. Main login, Store data/policy and user-runtime lifecycle are excluded.
Planning approval must not be described as execution approval.

This is the MVP-1 experiment in the [concurrency-first roadmap](collection-modes-roadmap.md#session-reuse-mvp-concurrency-first-critical-path-2026-10-19).
It refines the [one-login design](auth-session-reuse-design.md), reusing the
reviewed baseline `d397cdd` and the original synthetic broker/central experiments.

## Question and minimum useful result

Can one QA X login support a visible native reader and a separate true-headless
collector at the same time, without a second login, reader disruption or accepted
work from an unverified session?

Separate profiles do not inherit each other's login. A one-way seed tests whether
the minimum approved native state is portable and sufficient for the same account.
It also tests whether ordinary collector activity changes or invalidates the
reader's session. It does not establish that copying cookies always works, that
the session can refresh indefinitely, or that a central vault can control every
site request.

The one-way direction establishes one authority for this experiment: reader to
derived collector, never collector to reader. It prevents automatic selection or
reverse-merging of conflicting state. It is a qualification order, not a final
choice against the owner's central-ticket preference. Central refresh/fanout is
a later candidate only if the native behavior supports it.

## Fixed scope and reusable evidence

- One enrolled QA account, one identity-pinned retained QA owner profile and one
  fresh disposable headless user-data-dir with an isolated derived context.
  Never copy a profile or attach to the user's main browser.
- One visible reader role and one actual headless collector role, not background
  tabs silently substituted for headless. Preserve existing QA reader windows.
- Use the retained-profile ownership lease. Reject active/foreign ownership,
  identity changes or incomplete prior cleanup. Reuse its valid login; do not ask
  for repeated sign-in. Expiry/challenge ends the run for a user decision.
- Both browser trees and helpers belong to the existing kill-on-close fixture
  Job before work begins. In the integrated pilot their parent is Sidecar. No
  child may survive its owning parent; do not change the user runtime to test it.
- Reuse synthetic overlap, refresh/conflict, stale-result, queue and Job evidence
  while inputs are unchanged. Add only future checks for changed boundaries.
- Early native bootstrap proves a self-ID candidate and matching entity in one
  sample, not authenticated semantics or continuity. Account-settings and absent
  login flags must not be promoted to stronger evidence.

## Transfer manifest and data boundary

The native allowlist is **not yet qualified**. A future reviewed manifest must
enumerate exact cookie names, domain/host-only scope, paths and partition context,
plus the owned source/destination context and expected account binding. No native
transfer may run with an empty, inferred, wildcard or dynamically expanded list.
Do not guess the required cookie set from synthetic cookie names.
Proposed transport ceilings are 16 tuples, 8 KiB per serialized tuple and 64 KiB
total. Enforce bounds before publication; overflow stops the run rather than
truncating credentials or raising limits automatically. These are experiment
limits to review with the exact manifest, not claims about X's required state.

| Data | Permitted boundary for a separately authorized run |
| --- | --- |
| Approved login cookies | Read only the enumerated tuples through the owned private browser connection; send values directly to the owned derived context in process memory/private IPC. Preserve Secure, HttpOnly, SameSite, expiry, domain/path and partition attributes; reject unsupported representation instead of broadening scope. |
| Origin scope | Start with explicitly reviewed X origin/domain tuples. No blanket subdomain, cross-site or all-cookie export. Any additional required origin needs manifest review and authorization before access. Google/Apple login-provider state is never part of the transfer. |
| Identity/authentication evidence | Read only the fixed, bounded source projection needed for current-self and authenticated semantics. Compare account values transiently; export only match/mismatch/unknown and qualified source/version categories. |
| Runtime binding | Carry private owner/context identity, observation freshness, one-use transfer nonce and local generation. These bind operations and reject replay; they do not create native session lineage. |
| Persistent evidence | Fixed diagnostics, bounds, enum outcomes, coarse lengths/counts, overlap timing and cleanup status only. No native account IDs/handles, cookie values or reversible/credential-derived fingerprints. |

Passwords, SMS codes, Google/Apple tokens, authorization/CSRF headers, raw request
or response bodies, browser history, localStorage, IndexedDB, sessionStorage,
device keys and whole-profile files are outside the transfer. If cookies alone
are insufficient, stop with `additional_state_required`; do not add storage,
headers or keys automatically. Ordinary website traffic remains Chrome-managed;
this restriction concerns helper extraction and transfer, not blocking X's own
requests. Main profiles and Chrome cookie database decryption are forbidden.

No credential values may appear in argv, environment, URLs, shell output, public
HTTP/status APIs, logs, screenshots, traces, receipts, Git or helper snapshots.
Do not persist a native vault/checkpoint for this experiment. Clear transfer
buffers/references and dispose the derived context on completion; do not promise
secure memory erasure. Chrome can write its own login state inside the authorized
QA profiles: memory-only helper transport is not proof of disk-free Chrome.

Keep the retained owner's profile under its existing retention authorization.
Remove only disposable task artifacts after proven Job/context cleanup. If cleanup
is unproven, retain that scoped root and report failure; never kill unrelated
Chrome processes. Local disposal/deletion does not prove server-side revocation.

## Proposed bounded procedure

These are future steps, not commands approved for execution now.

1. **Offline boundary preflight.** Review exact tuple/source manifests, ownership,
   redaction, body limits, replay rejection and failure cleanup. Reuse passing
   synthetic tests; run only necessary changed-boundary checks after separate
   implementation/test authorization. A `verified` capability input alone is
   not native qualification.
2. **Owner readiness.** Acquire the retained QA lease, prove the old Job released
   it, and open the QA reader. Use the existing login. Install the required
   observation before session-bearing work; any needed close/reopen for attaching
   to this QA profile is setup only, not a continuity claim across that interval.
   Pin a fresh observed baseline after setup. Qualify stable current-self and
   authenticated source semantics; a UI label or generic HTTP 200 is insufficient.
3. **Single transfer.** Create the fresh headless profile/context under the owned
   Job. Export/import the exact manifest once through the private connection.
   Validate the same account and authenticated semantics in the derived context.
   No second login, reverse sync, automatic reseed or retry loop is allowed.
4. **Actual overlap.** Observe at most 60 seconds, with two bounded pairs of
   ordinary read-only X operations. Use only reviewed natural page actions/routes,
   never guessed API or refresh endpoints. Record request start/completion timing
   and require at least one pair whose intervals overlap and whose qualified
   responses succeed for the same account. Merely keeping two processes alive
   does not prove concurrent authenticated work. Preserve reader target/window
   identities and verify readiness again after overlap. No like/post/follow,
   logout, account switch or forced refresh action is part of this minimal run.
5. **Mutation/coverage decision.** Independently record observed rotation,
   correlation barriers, target/startup coverage and detach/transport failures.
   Unexpected mutation or coverage loss closes admission and rejects affected
   results. Keep windows while the parent lives; do not inject derived state into
   the reader, automatically reconcile conflict or declare fallback resumed.
6. **Stop and report.** Dispose the derived context, stop all owned QA processes,
   prove Job empty, clean only disposable roots and release the retained lease.
   Keep the owner's retained profile. Emit the fixed non-authorizing result and
   stop; do not extend sampling to wait for a rotation or request another login.

The initial 60-second cap and two pairs are proposed transport/experiment bounds,
not proof of session durability. If rotation does not occur, report
`refresh_not_observed`. A source challenge or need for more state is a named
feasibility result, not an invitation to retry with broader access.

## Decisions and exit gates

| Outcome | Meaning and next decision |
| --- | --- |
| `overlap_qualified` | One-way portability, same-account authentication and overlapping reads were demonstrated within stated coverage. This alone does not authorize a main-dev pilot. |
| `overlap_partial` | Process overlap, UI readiness or identity candidate only; authentication, mutation coverage or another required claim remains unknown. Preserve partial evidence without accepting real work. |
| `refresh_not_observed` | No refresh claim. Do not silently release an unbounded pilot or infer durability from elapsed time. Qualify a later bounded refresh path or propose an explicit limited-session acceptance decision consistent with policy. |
| `additional_state_required` / `transfer_unsupported` | The approved cookie set is insufficient or credentials are nonportable/device-bound. Stop this transfer strategy; a vault is not a workaround for nonportable credentials. |
| `cross_browser_invalidation` | Collector activity or rotation invalidates the reader, or vice versa. Stop; central coordination needs source-specific evidence before it is considered a remedy. |
| `coverage_unqualified` | Required startup/target/worker coverage is absent or lost. No concurrent admission under the existing policy. Checkpoints do not substitute for required continuous observation. |

Service-worker targeted response observation and shared-worker checkpoints have
synthetic evidence, but initial script-fetch/startup coverage remains unqualified.
Do not assume disabling workers preserves X behavior. Scope may be bounded to
supported behavior, but accepting a known relevant observation gap requires an
explicit policy decision; no such change is approved here. Safe closure acts on
qualified events/barriers, not instantaneous server-wide logout detection.

Every receipt remains `canAuthorize:false`; this experiment does not open Store,
claim jobs, enroll a real provider or create an accepted revision. Native account,
authentication and session-generation provenance must ultimately meet the
unchanged Store contract. A nonce, local epoch, cookie equality or matching IDs
before/after a gap is not a replacement for native lineage.

The enrolled authority profile and the derived profile/context have distinct
identities. Future integration needs an explicit verified mapping between them,
including generation/revocation binding. The existing single-profile checkpoint
must not be satisfied by relabeling the derived profile as the authority. Define
any provider/version compatibility change separately; this plan changes no Store
schema, accepted revisions or claim policy.

Do not wait for the exclusive fallback's close/reopen continuity proof to attempt
this concurrency experiment. Do not drop native proof requirements merely because
both processes now remain open. A successful bounded feasibility result permits
planning concurrent integration; activation still requires qualified proof,
integrated acceptance and explicit authorization.

Before the pilot, define last-reader-close behavior: in the tested reader-owned
launch configuration the owner exits with its last window, so reseeding becomes
unavailable. Continuing an existing derived session requires qualified admission
scope; otherwise fence collection and retain the queue. Reopening an owner needs
a newly observed/verified generation. Central-vault has synthetic reader-absent
refresh evidence but no native qualification. Invisible owner lifetime, seamless
restart, broader platforms and media remain separate work, not implicit promises.

## Offline harness checkpoint (2026-10-19)

After the planning documents were reviewed and published as `d1db051`, the owner
authorized an offline harness and its tests only. The implementation is
`scripts/auth-session-fixture/x-concurrency-harness.mjs`, with the canonical
offline command documented in the [fixture README](../scripts/auth-session-fixture/README.md#offline-concurrency-harness-2026-10-19).

The targeted suite passed 16 tests, zero failures/skips, using the bundled Node
runtime. It covers one exact synthetic seed, two in-memory overlapping read
pairs, primitive/data-only manifest bounds, account/profile/generation binding,
replay, evidence freshness/deadlines, fixed error redaction, observation loss,
cancellation and cleanup after late effects. Reader inventory is reported as
retained, changed or unverified; pending/failed cleanup is separate from success.
Static owner evidence expires after five seconds and is rechecked before
dispatch; the 60-second experiment ceiling does not extend that evidence's life.

All adapters in this suite are trusted in-process synthetic test code. No Chrome,
network listener, native X transfer, retained-profile access, Store access or
user-runtime operation occurred. The receipt remains non-authorizing and native
unqualified, with `refresh_not_observed`. This is harness scheduling/contract
evidence; the native transfer allowlist, browser adapter and coverage qualification
remain unfinished. No existing synthetic capability flag is promoted to native
evidence by these tests.

## Synthetic Chrome adapter checkpoint (2026-10-19)

The offline harness delta was reviewed and published as
`9b62f12fd2d2844a08c23a561074575b4d9c3a3c`. The subsequent authorization covered
fresh synthetic Chrome profiles and loopback tests only, not native X transfer.
`x-concurrency-chrome-adapter.mjs` now connects the same harness to a normal
Chrome reader and a separate true-headless Chrome tree through private pipes.
The gated driver launches only after the Go harness has assigned Node to its
kill-on-close Job. Go verifies the Node/Chrome images and Job membership before
running the experiment; no Store or production controller is opened.

Canonical command: `scripts/test-x-concurrency-fixture.ps1`; `-NoChrome` selects
offline checks, while `-Scenario` can isolate one named browser case. Browser
execution on this host required an unsandboxed tool invocation; no permission or
authentication guard was removed. All test data stayed inside the project QA
root and all native account/login/runtime boundaries remained unchanged.

Final run `f8a1306db4b64b91b4c9a00ca9d8166e` passed 27 Node cases, one Go report
boundary case and three Chrome cases on Chrome 155.0.8059.84:

| Case | Observed result |
| --- | --- |
| Concurrent reads | One invented login, one exact seed, four authenticated same-account reads; independent Go server observed two simultaneous requests. Reader/bootstrap windows were retained. |
| Cookie mutation | After the server confirmed active overlap, the reader rotated its invented cookie. The response observer fenced the run with `unexpected_mutation`; no successful pair was published. Reader windows remained. |
| Collector target loss | After the same server barrier, the exact collector target was closed. Actual target/observer events fenced the run with `coverage_unqualified`; context disposal completed and reader windows remained. |

Each browser case used its own new pair of profiles and exactly one synthetic
login. The final Go browser suite completed in 2.80 seconds. Job cleanup was
verified and the helper removed the disposable root. Sanitized command output is
in `runtime/qa/auth-session-sidecar/x-concurrency-f8a1306db4b64b91b4c9a00ca9d8166e.log.txt`.
No raw credential/body artifact or additional acceptance archive was created.

Two compatibility findings were corrected with relevant coverage. CDP can
return fractional cookie-expiry seconds; both manifest validation and the adapter
now preserve the finite value exactly instead of requiring/rounding to an
integer. This follows the [CDP Cookie definition](https://github.com/ChromeDevTools/devtools-protocol/blob/master/pdl/domains/Network.pdl),
where expiry is a number. Destroying a target can leave its pending evaluation
without a response: the adapter now retires a collector read only after the exact
target's destruction event, while retaining tracking for creation/seed effects
and verifying physical context disposal. Neither correction changes Store policy.

These results qualify only this controlled, worker-free loopback page topology.
The target-loss case does not prove arbitrary transport-loss handling, startup
worker coverage or instantaneous global revocation. The deliberate synthetic
rotation proves failure closure, not a native refresh/reconciliation protocol.
Reports retain `canAuthorize:false`, `nativeQualified:false`, `realAuthTested:false`
and `refresh_not_observed` for the unqualified durable-refresh claim. Native X
allowlist/authentication/lineage and source-specific observation coverage remain
the unfinished MVP-1 gate. The production concurrent controller is not activated.
