# Unified native X QA on d24d882

## Scope and verdict

On 2026-10-26, one explicitly authorized attempt executed preparation
`856c7939903641b59641c591e8b3bd9d` on commit
`d24d882969a0817649c56b1a3d7345fbf6b8bc6e`, source digest
`74d4d5641662d336a307ec5086b21bc361383caef1aa5805e91e9d74aeb7fbf8`.
The source graph was committed and clean. The fixed recipe allowed only
`auth_token` and `ct0`, up to 60 seconds from the original one-use seed start.
There was no retry or main login/database/runtime/Auto Update change.

**All operational steps succeeded, but the full harness acceptance is false.**
Native startup, one persisted capture, reader foreground verification, post
navigation, settle, stop and final cleanup succeeded. No operational first
failure was reported. The harness correctly retained `passed:false` because
`overlap` was `not_observed` under its current point-event criterion. Application
exit code was zero, with no parse errors or stderr bytes.

## Timing evidence

Application-relative times:

| Event | Elapsed ms |
| --- | ---: |
| Startup accepted | 12,291 |
| Reader requested | 12,292 |
| Owned marker ready | 12,619 |
| Capture backend RPC entered | 12,647 |
| Capture backend RPC returned | 15,584 |
| Reader foreground verified | 16,012 |
| Post navigation requested | 16,012 |
| Post navigation completed | 16,497 |
| Capture settled / Stop requested | 16,499 |
| Parent cleanup started | 17,778 |

The marker became ready **28 ms before** the backend RPC entered. Foreground
verification and navigation occurred after the RPC returned. The evidence model
sets overlap only when a marker/foreground/navigation point event occurs while
the backend RPC is active. None did in this trial. It does not infer overlap
solely from the reader request being in progress or the pilot being active.

This ordering does not establish that the product serialized the two lanes or
cannot operate concurrently. It also must not be silently promoted into the
stricter selected overlap gate. A future harness can wait for the actual backend
entry before issuing `open_post`, fail if that interval is already missed, and
retain the same no-retry policy. An interval-based window-lifetime criterion
would be a separate reviewed evidence change, not a retrospective success flag.

## Persisted result and cleanup

Exactly one observation containing two post blocks was present in the new QA
database. The reader completed the canonical requested post navigation; this is
not a claim that a screenshot or loaded post DOM was inspected. The controller
completed in 17,937 ms. Start reported 51,316 ms remaining on the original
authorization; no lifetime was reset.

Application cleanup reported `verified`. A separate scoped check confirmed no
matching QA application/Node/Chrome process and that controller PID 43724 had
exited. The retained reader manifest was `clean`. After verifying runtime-plan
hash, the exact collector path under this fresh instance, and the absence of
reparse points, the disposable collector root was deleted. The older retained
collector was not read, modified or removed. The retained reader profile remains.

Main configuration SHA256 stayed:
`52063d73180a8f6ab3e514d89cfaa81fb168e0b98381e58fa7ff1ae28d7aadae`.
No cookie value, raw body or account ID is recorded here.

Evidence under `build/x-application-qa-prepared/856c7939903641b59641c591e8b3bd9d/`:
`execution-events.jsonl`, `execution-result.json`, `cleanup-verification.json`,
and the consumed `execution.claim`. The plan's collector identity now references
a removed disposable directory. This plan must not be reused or reset. Any new
native attempt requires fresh explicit authorization and a new preparation.

Main-dev activation remains default-off. The reader and graceful-close fixes
have native operational evidence; the selected overlap acceptance remains open.
