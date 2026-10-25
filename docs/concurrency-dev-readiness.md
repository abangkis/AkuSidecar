# X concurrency: current dev readiness

This page is the current checkpoint. Historical experiments in the design and
roadmap are evidence, not a new list of prerequisites.

## Selected scope

- Manual, X-only attempt, at most 60 seconds from the original attempt start.
- Strict account/login evidence for both owners at startup; identity renewal and
  complete refresh/rotation qualification remain deferred.
- Separate reader and collector profiles, with owned process-tree cleanup.
- Auto Update stays enabled. Source admission arbitrates work; it must not change
  the global scheduling preference.

## Current implementation

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

Latest attempt: source commit `52e1a6e` (startup correction `45adc58` plus offline
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

This correction has offline validation only. No new native attempt or transfer
was performed; the last native acceptance remains the failed attempt above.

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
(`startupBudgetMS`, 1 through 2,000 ms), rounded down from the remaining context
deadline. Each Node owner spends that single monotonic budget across identity,
rendezvous and settings waiting; a settings wait cannot reset it. The manual
startup route rejects a missing/invalid hint instead of using the standalone
QA recipe's 2,500 ms wait. Go remains authoritative over the entire two-second
startup, including transport, cleanup and final ownership checks. The hint
cannot extend it or authorize evidence; receipt/account/document guards and
the 60-second attempt ceiling are unchanged.

Implementation validation was offline/accountless. The subsequent single native
acceptance is recorded above and still fails; these changes do not establish
that native startup now fits within two seconds.

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
   deferred identity renewal merely to expand this bounded pilot. Review the
   collector settings-completion wait offline next; another real transfer needs
   fresh one-shot authorization. Keep the current account guards and deadlines.
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
