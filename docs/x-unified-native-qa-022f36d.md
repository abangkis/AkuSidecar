# Unified application native QA on 022f36d

## Scope and outcome

One explicitly authorized attempt executed the prepared application matching
`022f36d857513849b9e1331cce0bc5a86da0462b`. The retained QA reader stayed in
place; only the existing two-cookie recipe (`auth_token`, `ct0`) could seed a
fresh collector. The QA database was new. There was no retry, login action,
main/user database access, main-runtime operation or Auto Update change.

**Outcome: not accepted.** Native startup reached application publication, but
the capture worker returned `empty_unverified`; no observation/block was
committed. Reader marker preparation overlapped the in-flight capture RPC.
Foreground verification and post navigation were not completed.

## Bounded evidence

| Boundary | Observed result |
| --- | --- |
| Explicit start / strict paired startup / Store admission | Accepted; application stage at 12,874 ms |
| Remaining original authorization at start report | 50,645 ms |
| Capture backend RPC | Entered at 13,609 ms, returned at 16,116 ms (2,507 ms) |
| Owned reader marker | Ready at 13,652 ms, inside the capture RPC interval |
| First operational failure | `capture / failed`, detail `stage=worker reason=empty_unverified cleanup=completed` |
| Subsequent reader operation | `reader_foreground / unavailable`; it did not replace the first failure |
| Persisted result | Zero observations, zero blocks |
| Final parent cleanup | `verified`; controller completed at 18,672 ms |

The backend interval is evidence of concurrent application operations, not
proof of individual network/render activity. The top-level authorization still
had substantial time at startup; the reported failure is not an exhausted
60-second authorization deadline. This result alone does not establish why
source-post evidence was absent, whether page readiness or parser coverage was
responsible, or whether a longer wait would change it.

## Cleanup and retained state

The application reported verified complete owned-process/pipe cleanup. A
separate bounded metadata check then confirmed the retained manifest was
`clean`, its reader profile remained present, and the task-owned controller had
exited. After checking exact absolute scope and rejecting reparse points, the
entire disposable collector root was removed, including its derived session.
The new QA database and sanitized evidence remain for offline investigation.

The main configuration SHA256 remained:
`52063d73180a8f6ab3e514d89cfaa81fb168e0b98381e58fa7ff1ae28d7aadae`.
No cookie values, account IDs or raw response bodies appear in this report.

Evidence resides in the task-owned preparation root
`build/x-application-qa-prepared/2bd6390be6b04ab98634ce0732e3c0b3/`:
`execution-events.jsonl`, `execution-result.json`, and `cleanup-result.json`.
Its `execution.claim` is consumed. The collector identity now names a deleted
disposable directory; this runtime plan must not be rerun or reset.

## Next boundary

Investigate `empty_unverified` offline along the existing X worker/parser and
page-readiness path, preserving this first cause. Reuse the accepted ownership,
lease, startup and synthetic composition evidence. Do not change deadlines or
infer an authentication failure from this empty result without supporting
evidence. Any later native attempt requires a new plan and explicit permission.
Main-dev activation remains gated on a successful native capture/reader result.

## Subsequent offline correction (2026-10-26)

Read-only comparison found that both this run and the previously accepted
retained-capture route used the same one-second native hydration default. The
new application's scroll settings also differed from that older test. Therefore
neither a parser regression nor slow rendering is established as the native
root cause by the saved evidence. The collector captures Home; the supplied
post URL is for the separate reader operation.

The approved correction centralizes the native hydration default at 3,000 ms in
the shared timing policy, bounded by the original attempt. It also carries a
strictly projected structural worker summary to the final QA report. A delayed
synthetic post reproduces `empty_unverified` with the old wait and succeeds with
the new wait. A rejected-candidate fixture proves that missing identity counts
survive native diagnostic wrapping, cleanup failure, Node reply encoding, the
correlated Go pipe and the Engine result. A separate final-report test retains
worker metadata as the immutable first failure. No-snapshot telemetry is
distinguished from a snapshot containing no accepted posts.

Evidence:
- Engine/Store, Go timing, worker telemetry and owned-RPC tests:
  `runtime/qa/auth-session-sidecar/run-capture-worker-offline-436540057ef740a09dd7d3ed5a6c4cf6/go.log.txt`.
- Corrected accountless correlated pipe tests:
  `runtime/qa/auth-session-sidecar/run-capture-pipe-offline-0f742b3bca784c33b4ceb1e2c923d583/pipe.log.txt`.
- Actual-main synthetic journeys, four builds and legacy compile-only staging:
  `runtime/qa/auth-session-sidecar/x-qa-entry-50fcc2cc704f420d953194ca1fe3881e.log.txt`.

The first pipe-test revision exposed a test import accidentally entering the
driver's CLI guard; its sentinel argument correction passed both challenge and
worker-empty modes. This is test-harness evidence, not a native browser attempt.
No new session transfer or native test occurred. The original outcome above
remains not accepted, and its execution claim remains consumed.
