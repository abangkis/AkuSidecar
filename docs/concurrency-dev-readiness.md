# X concurrency: current dev readiness

This page is the current checkpoint. Historical experiments in the design and
roadmap are evidence, not a new list of prerequisites.

## Selected scope

- Manual, X-only attempt, at most 60 seconds from the original attempt start.
- Strict account/login evidence for both owners at startup; identity renewal and
  complete refresh/rotation qualification remain deferred.
- Startup acquisition/publication has a six-second deadline. Every identity and
  login sample must still be less than two seconds old at final acceptance.
  Both bounds are clipped by the original attempt's remaining time.
- Separate reader and collector profiles, with owned process-tree cleanup.
- Auto Update stays enabled. Source admission arbitrates work; it must not change
  the global scheduling preference.

## Current implementation

### Timing policy update (2026-10-25)

The approved timing change separates startup work from evidence age. The private
Go adapter gives the complete initial acquisition (ownership checks, transport,
both role receipts and final pair inspection) less than six seconds after both
owners are prepared. Node receives only the remaining rounded-down wait budget,
capped at 6,000 ms; waits cannot reset or extend it. A shorter caller or absolute
attempt deadline still wins. The source's own elapsed duration is bounded by
six seconds, independently of sample timestamps.

All four identity/login samples must remain less than two seconds old at final
publication; an earlier source expiry also wins. Node rejects locally stale
samples and Go rechecks the earliest sample expiry after final ownership. This
also keeps the maximum time between the two browsers' accepted observations
below two seconds. The admitted manual vector still ends at the original
attempt start plus at most 60 seconds, rather than at sample expiry. No renewal,
retry, cookie-guard relaxation, Store policy or runtime activation is introduced.

Earlier two-second startup and five-second sample-age references on this page
describe the previous timing policy. The latest native attempt below passed
initial startup admission under the new timing policy but failed at capture;
the historical failed-startup records remain unchanged.

Offline validation passed: `test-native-manual-pilot-startup.ps1` (76 Node and
33 top-level Go tests, four tag gates, both Engine builds),
`test-native-manual-pilot-broker.ps1` (55 Node and eight top-level Go tests,
four tag gates, both Engine builds), and `test-x-manual-pilot-owner.ps1`
(151 Node and 22 top-level Go tests, four tag gates, staged Windows compilation).
These suites overlap and their Node counts are not a unique total. Regression
cases accept slow startup with fresh samples, reject exact freshness/startup
limits, preserve shorter caller/attempt deadlines, and exercise the shared
Go/Node private transport. Owned temporary build roots were removed.

Evidence logs:

- `runtime/qa/auth-session-sidecar/native-manual-startup-ab916c30e2f54ce2a972bf0b0d055a8c.log.txt`
- `runtime/qa/auth-session-sidecar/native-manual-broker-74421c65a1144719bed5c35e3c426db5.log.txt`
- `runtime/qa/auth-session-sidecar/x-manual-owner-d461230dcb4c4d268423e07c17bf48e0.log.txt`

No browser, retained QA profile, transfer, user database, runtime or Auto Update
setting was accessed by these tests. The existing local dev configuration and
untracked experiments remain outside this change.

The private native capture adapter uses the existing parser/worker, Coordinator,
Engine and guarded Store commit. Synthetic acceptance covers actual parser output,
replay, cancellation, expiry and ownership rejection. Main/broker routing has
synthetic acceptance. This is not yet an ordinary main-dev native provider.

The new preparation mode is:

```powershell
.\scripts\prepare-x-manual-retained-qa.ps1 -Capture
```

Preparation compiles the real Engine sources and the bounded retained-QA capture
continuation. It excludes the package-wide browser TestMain, pins source and
binary hashes, and checks the disabled entry before any retained-profile access.
The plan distinguishes `capture-store` from the older `startup-memory` scenario;
an old startup plan cannot execute capture. Preparation is not transfer approval.

## Native acceptance attempt

### Offline capture rejection investigation (2026-10-25)

The latest native log below cannot establish the failed capture predicate. The
previous code collapsed local adapter predicates, Node setup/worker errors and
transport errors into generic rejection; the retained QA entry printed only
the broad Engine phase/outcome. Node's final response/header coverage check or
cleanup exception could also replace an earlier worker/challenge error. These
are confirmed observability defects, not proof of the native rejection's cause.

The private capture path now carries closed `stage`, `reason` and `cleanup`
diagnostics. Engine behavior admission and Coordinator fallback are distinct
from backend owner/payload/transport checks, Node target setup, navigation,
evaluation, worker login/challenge/empty-evidence errors, response/header
coverage, wire/digest/observation validation and final ownership. Unknown errors
stay coarse; no arbitrary error message is promoted into a diagnostic. The QA
entry prints only these sanitized fields, never its raw persistence error.

Node uses private WeakMap branding. Operational failure codes and observational
metadata are now independent: the correlated failure reply carries its existing
`code` plus an optional `diagnostic` envelope with schema
`aku.capture-diagnostic.v1`, `stage`, `reason` and `cleanup`. The generic fixture
bridge only carries a bounded opaque payload (at most 512 bytes); the capture
adapter validates its version, exact unique fields, string types and closed
catalogs. Unknown/malformed metadata is ignored without changing the operational
failure. A diagnostic claiming challenge cannot create challenge/pause authority;
a contradictory diagnostic cannot change a challenge's independently decoded
category. The old `manual_capture_detail_<stage>__<reason>__<cleanup>` code reader
remains for compatibility, but new producers no longer encode diagnostics there.

The Go error wrapper keeps the domain-owned operational cause independently
discoverable through `errors.As`, alongside its safe diagnostic. Challenge plus
failed cleanup therefore retains both the normal persistent challenge handling
and `cleanup=failed` through Node, bridge, Windows RPC adapter and cached Store
result. A later pause-persistence error may remain the operational error but
cannot replace the stored capture diagnostic. Seed/startup reply behavior and
the existing code sanitizer are unchanged.

Inspection failure and proven mismatch are also distinct. Go reports an
unavailable/canceled/deadline inspection before considering owner comparison;
the zero/unknown result of a failed inspection cannot become `owner_changed`.
Node uses trusted inspection-condition evidence to distinguish confirmed pinned
mismatch from a failed snapshot. Rejection and revocation rules remain the same.
These are local implementations of reusable separation and evidence principles;
no shared library, OTel exporter, AG migration or product-wide schema is added.

Terminal primary failure wins over cleanup/coverage errors; cleanup status is
reported separately. `cleanup=completed` means the capture target's CDP close
command completed, not proof of Windows Job/lease or profile retirement. Actual
owned-process cleanup remains the existing separate QA gate. Capture cleanup
awaits a previously started close instead of dispatching another close. Errors
that the existing worker treats as transient retain their original in-process
classification; a recovered transient does not become a fatal diagnostic.

No cookie/token value, page text, URL, account/owner/request identifier, local
path or raw exception is included in these diagnostics. Startup freshness,
attempt deadlines, guards, result admission, retry rules and Store policies are
unchanged. No browser, retained profile, X request, transfer, user database,
main runtime or Auto Update preference was accessed for this investigation.

Offline validation: `scripts/test-x-manual-native-capture.ps1` passed 44 capture/
owner Node tests and 36 worker/parser/transport Node tests; five top-level Go
tests include 12 Engine/Store integration subcases and ten inspection-classifier
subcases. The retained native entry compiled through the canonical staging
helper. Its separate Windows sanitizer test and accountless Node-to-Go stdio
test passed; the latter uses the actual Node encoder, bridge reader and Windows
RPC error adapter without OS ownership assertions, Chrome or a profile. All
four build-tag gates passed. Covered failures include setup/navigation, pending
ExtraInfo, late cookie writes, first challenge plus failed cleanup, cancellation,
invalid wire/digest/source, malformed/contradictory sidebands, metadata that cannot
manufacture operational authority, unknown private errors, replayed diagnostics and
unchanged transient context recovery. Only synthetic databases and fake CDP/DOM
were used. Owned temporary roots were removed.

Evidence:
`runtime/qa/auth-session-sidecar/x-manual-native-capture-d4831f08908a4a20905f6dc4ec01fc28.log.txt`.
No native attempt was repeated; identifying the latest real capture failure
still requires a separately authorized run with these diagnostics.

### Latest native attempt

Latest attempt: source commit `ad865b4`, explicitly authorized run
`581ff3a1757546fdbfe84afd2369bfad` on 2026-10-25. Preparation passed 279 and
33 Node cases, all four retained tag gates, staged full Engine compilation and
accountless Go diagnostic checks. The prepared plan pinned 918 source files,
only `auth_token`/`ct0`, and the original maximum 60-second attempt.

Exactly one native execution passed initial broker startup admission, entered
the capture continuation, created its disposable QA database and opened the
native reader. Runtime binding, dispatch, durable Store claim and pre-capture
owner validation also passed, as established by the reached `capture` phase.
The strict account/login receipts were therefore accepted within the separated
six-second startup/two-second final sample-age policy in this attempt. No second
login was requested. No precise startup duration or general timing reliability
claim is available from this log.

The complete test still failed (12.26 seconds) with
`phase=capture outcome=rejected_no_mutation`. This phase contains both behavior
admission and Coordinator/backend capture. The test logs neither the retained
error nor a safe substage/category, so it does not establish whether pacing,
ownership, request validation, the worker, a challenge, cancellation or another
backend predicate caused rejection. Do not infer the cause from the generic
outcome. The phase did not reach result validation or result commit;
`rejected_no_mutation` is not a claim that the earlier durable claim did not
mutate the disposable QA database.

Owned Job/lease cleanup was verified. The collector/database/build root was
removed, and the dedicated retained reader profile was preserved. No retry or
main-dev activation occurred. Main login, user database/runtime and Auto Update
were unchanged; the local dev configuration SHA-256 remained
`52063D73180A8F6AB3E514D89CFAA81FB168E0B98381E58FA7FF1AE28D7AADAE`.
Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-581ff3a1757546fdbfe84afd2369bfad.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-581ff3a1757546fdbfe84afd2369bfad.run.log.txt`.

Previous attempt: source commit `94d8e2f`, explicitly authorized run
`90d1b8373b2d4f548eb3a99c3c4da2ac` on 2026-10-25. Preparation passed 279 and
33 Node cases, retained tag gates, staged compilation and accountless diagnostic
checks. Exactly one native execution rejected at the unchanged 2,000 ms joint
startup deadline (full test 11.10 seconds).

At the frozen boundary collector was in an OS ownership snapshot for 4 ms;
its acquire RPC had accumulated 1,866 ms, OS snapshots 130 ms and inspect RPCs
2 ms. Reader had no tracked operation active, with acquire 1,698 ms, OS snapshots
171 ms and inspect RPCs 13 ms. These per-role totals overlap and are not additive.
The snapshot is an ownership boundary after the collector acquire RPC, but
does not identify whether its remaining OS work is the role's local post-check
or a later pair check. Full Go receipt/pair acceptance is not established.
No specific cookie, account or settings rejection was recorded. A single
uncontrolled attempt does not establish the timing effect of parallelization.

The capture continuation was not entered; no QA capture database/observation
was created. Owned Job/lease cleanup was verified, the collector/build root was
removed and the dedicated retained reader profile was preserved. No retry or
main-dev activation occurred. Main login and Auto Update were unchanged; the
local dev configuration SHA-256 remained unchanged. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-90d1b8373b2d4f548eb3a99c3c4da2ac.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-90d1b8373b2d4f548eb3a99c3c4da2ac.run.log.txt`.

Previous attempt: source commit `fbf9aae`, explicitly authorized run
`8364848408cb47879bd7b83bf9590471` on 2026-10-25. Preparation passed 279 and
33 Node cases, retained tag gates, staged compilation and accountless diagnostic
checks. Exactly one native execution then rejected at the unchanged 2,000 ms
joint startup deadline (full test 10.59 seconds).

Collector's acquire RPC had returned; it was in the subsequent ownership inspect
RPC for 6 ms when the frozen boundary was taken. Collector cumulative acquire
was 1,763 ms, OS snapshots 157 ms and inspection RPCs 8 ms. Reader had no tracked
operation active; its acquire totaled 1,505 ms, OS snapshots 180 ms, inspection
RPCs 6 ms and inventory RPCs 1 ms. Per-role times overlap and are not additive.
The owner port enters this inspection only after a successful acquire RPC, but
Go wire decoding and final pair acceptance still follow; no full account/startup
acceptance is claimed. The previous `settings_wait` boundary was not active in
this attempt. No settings category remains on that role's new inspection RPC,
because per-RPC timing resets it deliberately.

The capture continuation was not entered, so no QA capture database/observation
was created. Owned Job/lease cleanup was verified, the collector/build root was
removed and the dedicated retained reader profile was preserved. No retry or
main-dev activation occurred. The main login and Auto Update were unchanged;
the local dev configuration SHA-256 also remained unchanged. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-8364848408cb47879bd7b83bf9590471.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-8364848408cb47879bd7b83bf9590471.run.log.txt`.

Previous attempt: source commit `52e1a6e` (startup correction `45adc58` plus offline
fixture wiring), explicitly authorized run `a68c471222af4c71aab3682109348f6f` on
2026-10-25. Preparation passed 279 and 33 Node cases, all retained tag gates,
staged compilation and accountless diagnostic tests. The broader preparation
first exposed a missing wait-budget callback in the in-memory driver fixture;
that fixture now uses the same bounded wait helper, and its 72 tests pass.
The failed preparation never launched Chrome or transferred a session.

Exactly one native execution then rejected at the unchanged 2,000 ms startup
deadline (full test 11.93 seconds). At the frozen boundary no tracked reader
operation was active; reader cumulative acquire was 1,570 ms, OS snapshots
198 ms, inspection RPCs 48 ms and inventory RPCs 1 ms. Collector was still in
acquire (1,763 ms), with `settings_wait` the last reported Node stage (age
1,575 ms), OS snapshots 150 ms and inspection RPCs 1 ms. These per-role timings
overlap and must not be added together. They identify the settings completion
wait as the current observed boundary, not a specific missing response, body
projection failure or account mismatch. Reader idle alone is not proof that
the full pair's final validation completed.

The capture continuation was not entered; no QA capture database/observation
was created. Owned Job/lease cleanup was verified and the collector/build root
was removed; the dedicated retained reader profile was preserved. No retry,
main-login change, main-dev activation or Auto Update change occurred. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-a68c471222af4c71aab3682109348f6f.prepare.log.txt`
and `runtime/qa/auth-session-sidecar/x-manual-retained-a68c471222af4c71aab3682109348f6f.run.log.txt`.

Previous attempt: source commit `20011da`, explicitly authorized run
`10f3fd8b7d4d4e1f922cbb64bb02e69f` on 2026-10-25. Startup rejected at the
unchanged joint deadline (2,000 ms; full test 9.56 seconds). At the frozen
diagnostic boundary reader was in `inspect_rpc` for 15 ms; its cumulative
`acquire_rpc` time was 1,733 ms, OS snapshots 174 ms and inspection RPCs 19 ms.
Collector recorded 1,606 ms in acquire, 201 ms in OS snapshots and 2 ms in
inspection RPCs; no tracked operation was active for it at the boundary.
Idle does not establish that collector validation had finished.

The measured boundary is a reader inspection after acquisition, not a new
cookie/challenge rejection. Acquisition consumed most of the shared window;
these aggregate counters do not locate its internal longest suboperation.
No capture continuation, QA database or observation was created. Owned cleanup
was verified, the collector/build root was removed, and the retained QA reader
profile was preserved. No retry or main-dev activation occurred. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-10f3fd8b7d4d4e1f922cbb64bb02e69f.run.log.txt`.

Earlier attempt, before timing diagnostics:

One explicitly authorized attempt ran on 2026-10-25, run
`f5eec35fe324475f8584e5553d3e927e`. Seed acknowledgement and scope validation
completed; startup admission then rejected with
`role=pair phase=joint reason=deadline kind=primary` (test duration 9.43 seconds).
The capture continuation was not entered: no QA database or capture observation
was created. This is a failed acceptance, not a capture success.

The joint startup evidence deadline is two seconds, separate from the overall
60-second attempt ceiling. The retained diagnostic does not identify which role
or operation consumed that window; no specific cause is established yet. Do not
extend either limit or infer a cookie/challenge failure from this result.

Owned Job/lease cleanup was verified, the collector/build root was removed, and
the retained reader profile was preserved. No retry was performed. Evidence:
`runtime/qa/auth-session-sidecar/x-manual-retained-f5eec35fe324475f8584e5553d3e927e.run.log.txt`.

## Acceptance scenario

### Combined pair ownership checkpoint

The X owner port supplies a private combined checkpoint to the broker. It reads
policy/consent before inspection, runs the complete reader and collector
inspections in parallel, and rereads policy/consent after both finish. Each role
keeps its OS-before/RPC/OS-after checks, exact prepared handle/document binding,
image/creation/Job/directory validation, headless/window checks and deadline.
The final state uses the inventory already validated by the reader inspection;
it does not trust the policy callback's cached window count. Policy completeness,
parent/channel/observer flags and all enrollment/attempt/session-policy pins must
still pass. No ownership result is cached between checkpoints.

The native pair checkpoint performs four OS snapshots and two inspect RPCs,
replacing the previous six snapshots, two inspect RPCs and extra inventory RPC.
The optimization applies to initial/final pair checks and subsequent guarded
uses. Per-role pre/post-acquire inspections remain mandatory. Existing generic
ports retain their original path only when no combined callback is installed;
a failed installed callback never falls back. Account receipt validation still
applies, using the six-second startup/two-second freshness policy above and the
original 60-second attempt ceiling.

Offline evidence: `test-native-manual-pilot-broker.ps1` passed 53 Node and eight
top-level Go cases, plus both Engine builds (log
`runtime/qa/auth-session-sidecar/native-manual-broker-dd126684e41b489681d5ca9fe7fd28f2.log.txt`).
`test-x-manual-pilot-owner.ps1` passed 151 Node and 22 top-level Go cases, tag
gates and staged Windows compilation (log
`runtime/qa/auth-session-sidecar/x-manual-owner-6c53dc77e695403d9fbc8288c56cb060.log.txt`).
The suites overlap. Focused pair tests also passed after making their late-reply
barrier cleanup robust. They prove parallel entry, exact work counts, fresh
inventory, policy rejection on both sides, peer cancellation, cause retention
and rejection of late inspections/policy reads. Owned temporary roots were
removed. Implementation validation used no Chrome/profile/session transfer or
user runtime. The subsequent single native attempt is recorded above and still
fails. Native timing improvement and successful capture acceptance remain
unproven.

### Settings completion and wait diagnostics

The direct capture and the bound observation now settle metadata completion with
closed `complete`/`failed` results. A selected `loadingFailed`, a rejected
candidate or a scope invalidation releases the waiter as failed immediately;
it does not wait for the joint timeout to reveal a known failure. Explicit
finish also retires incomplete waiters without changing their source verdict.
The manual driver rejects a failed result, retains the original cause and still
cleans up Runtime/Debugger resources. `settings_capture` identifies a terminal
settings-capture failure; an earlier cookie, lifecycle or other cause wins.

Metadata completion still requires the selected response, loading completion and
required ExtraInfo. It does not authorize an account or await the projected body.
Final acceptance still validates the body, account, pinned document, freshness,
cookie guard, cleanup and unchanged two-second/60-second deadlines.

Private progress can include one optional closed `settingsWait` category:
request pending or excluded before identity/by frame/document/history, response
pending, loading completion pending, ExtraInfo pending, metadata complete,
network/candidate/scope failure, or body pending/complete/failed. No request ID,
URL, account, cookie/header value, raw body or exception text is included.
The existing 32-record/512-byte bounds remain; category updates do not reset
the operation age. Go rejects unknown, duplicate or malformed fields/categories,
isolates the roles and freezes the last received category when admission fails
or times out. The first failure carries its category in the same progress record;
later role events cannot rewrite it. Diagnostics remain nonauthorizing and cannot
extend the deadline.

Implementation validation was offline. The subsequent one-shot native attempt
is recorded above and remains a failed acceptance; it reached the ownership
inspection after acquire instead of stopping in the settings wait.

Offline evidence: startup regression passed 73 Node and 31 top-level Go cases,
plus both Engine build variants (`native-manual-startup-692ece5f29d74440a088179651d6ffd5.log.txt`).
Owner regression passed 151 Node and 17 top-level Go cases, all tag gates and
staged Windows compilation (`x-manual-owner-437b048a8f254004962c1fe3f226a92c.log.txt`).
These suites overlap. An additional preparation-only transport check exercised
the closed failure/category across Node stdio into Go
(`x-manual-retained-8f3bb6b536b4466ebb19ed0d153671ef.prepare.log.txt`); its native
entries skipped before profile/lease access. Evidence logs are under
`runtime/qa/auth-session-sidecar/`. All owned temporary build/preparation roots
were removed; no prepared transfer plan remains for this correction.

### Offline startup overhead correction

The Windows owner checkpoint now resolves Node and Chrome parent links from one
process-list snapshot. This snapshot is local to that checkpoint; process
creation, image, handle liveness, Job membership and physical directory checks
remain in place and are repeated at later checkpoints.

The adapter checks the whole pair before acquisition and once again after both
receipts pass validation, immediately before publication. It no longer repeats
the pair inspection between the composed source's buffered receipts. Per-role
checks before and after acquisition remain mandatory, and the final publication
still checks cancellation, source provenance, evidence expiry and policy state.

After the pre-acquisition inspection, Go sends a private integer wait hint
(`startupBudgetMS`, 1 through 6,000 ms), rounded down from the remaining context
deadline. Each Node owner spends that single monotonic budget across identity,
rendezvous and settings waiting; a settings wait cannot reset it. The manual
startup route rejects a missing/invalid hint instead of using the standalone
QA recipe's 2,500 ms wait. Go remains authoritative over the entire six-second
startup, including transport, cleanup and final ownership checks. The hint
cannot extend it or authorize evidence; receipt/account/document guards and
the 60-second attempt ceiling are unchanged.

Implementation validation was offline/accountless. The subsequent single native
acceptance is recorded above and still fails; these changes do not establish
that native startup fits within the current six-second deadline.

Offline evidence: `test-native-manual-pilot-startup.ps1` passed 55 Node tests,
30 top-level Go tests and both development Engine build variants (log
`runtime/qa/auth-session-sidecar/native-manual-startup-a17b9c92619a4b1fa0432342578b81d7.log.txt`).
`test-x-manual-pilot-owner.ps1` passed 79 Node tests, 17 top-level Go tests,
all four tag gates and staged Windows backend compilation (log
`runtime/qa/auth-session-sidecar/x-manual-owner-6a7f76c74fee45628537bf120524d1c6.log.txt`).
The Node suites overlap; their counts are not an aggregate unique-test count.
Both helpers verified removal of their owned temporary build roots.

Startup diagnostics now retain a frozen, bounded snapshot for both roles when
admission fails: total local elapsed time, the active Go operation and its age,
cumulative Go operation durations, and the last reported Node stage. Node stages
include identity, rendezvous, settings wait, cleanup and root-control restoration.
Node stage age combines its reported duration with local time since reception;
it is not a synchronized cross-process timestamp or proof of the current browser
state. Missing progress stays idle/unknown rather than blaming a browser.

Private progress uses the existing pending RPC correlation, at most 32 records
per RPC, each at most 512 bytes. Only closed operation/cause categories and
bounded milliseconds are accepted. It carries no cookie, account ID, page body,
URL or document pins, and it cannot authorize a result. Late events and events
for completed RPCs are ignored. The first observed failure is retained across a
subsequent deadline; an unattributed timeout remains pair-scoped. Frozen timing
cannot be rewritten by late callbacks or cleanup.

The two-second startup deadline and 60-second attempt ceiling are unchanged.
The latest native failure remains the failed attempt recorded above; offline
timing/transport tests do not reconstruct unrecorded internal operation timing.
The six preflight fixture regressions now register their synthetic root-control
session before exercising guard binding, so they reach their intended predicates.

The capture scenario reuses the existing retained-profile lease and startup
recipe exactly once. It opens an additional reader window, captures one bounded
round through the existing X worker, commits to a newly created QA database,
checks a nonempty observation and exactly-once replay, and checks that the reader
windows and collector PID remain. Reasoning uses the deterministic QA provider.
Post content stays in the disposable QA database and is not printed to logs.
Reports expose fixed phase/outcome values rather than raw errors or account data.

It does not prove opening a new reader window *during* a native in-flight capture;
that ordering currently has separate synthetic Chrome acceptance. It does not
prove foreground Windows activation, long-running renewal, or main-dev activation.

Execution requires a fresh, explicit one-shot transfer approval for the concrete
prepared plan. Only `auth_token` and `ct0`, the existing manifest and 60-second
deadline are permitted. No retry, login fallback or main-profile access is added.
Cleanup retains the dedicated reader QA profile and deletes the owned collector
profile/database/build root only after Job/lease cleanup is verified.

## Shared capture behavior

- Pilot claims and ordinary claims use the same transactional source occupancy.
  The pilot/ordinary exclusion remains effective with optional pacing disabled.
- Configured spacing/backoff applies before the pilot's durable claim.
- Persistent X challenge pause gates both pilot claims and result commits.
- A challenge from an exact active pilot claim pauses X and cancels registered
  work. A stale claim cannot relatch X after explicit recovery.
- Closed challenge categories survive Node and Go transport. Capture responses
  get a separate bounded envelope; startup evidence limits are unchanged.

## Remaining activation work

1. Obtain native capture acceptance using the prepared QA scenario. Do not reopen
   deferred identity renewal merely to expand this bounded pilot. Initial native
   startup and reader opening passed in the latest attempt; the current failure
   is at capture. Closed behavior/Coordinator/backend substage/reason diagnostics
   now pass offline validation. Review/commit this delta before a separately
   authorized one-shot native acceptance attempt.
   Another real transfer needs fresh one-shot authorization. Keep account guards,
   the six-second startup/two-second freshness policy and 60-second attempt cap.
2. Promote the owned Windows/profile lifecycle from QA-only construction into a
   development provider, with explicit opt-in, status, stop and source-route
   arbitration. The current native Windows owner remains dual-tag/fixture scoped;
   a boolean alone cannot activate it safely.
3. Verify that provider against an isolated dev instance before separately
   approving main-dev runtime activation. Keep Auto Update enabled and preserve
   the existing main login. Rollback closes pilot admission/owners and returns
   routing to the ordinary scheduler without clearing challenge state.

No ordinary config switch or UI button is exposed until it has a working provider.
The current source changes do not restart, migrate or write the user's runtime.
