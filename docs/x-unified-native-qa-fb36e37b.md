# Unified native X QA: source fb36e37b

## Scope and verdict

On 2026-10-26, one explicitly authorized execution used preparation
`496e9789604e4d57813453e43230466f`, based on commit `022f36d` plus the reviewed
uncommitted source digest:
`fb36e37b92dece3f72c52a3de9753fa2417be24cc3235a4f258f31eeae506f7f`.
The one-shot recipe permitted only `auth_token` and `ct0`, at most 60 seconds
from the original seed authorization start. There was no retry.

**Capture succeeded; the complete native journey was not accepted.** One
observation containing two post blocks committed to the fresh QA database.
Reader foreground verification did not complete, and the application reported
cleanup as `uncertain`. The operator confirmed that the marker was visible but
they had not clicked before it closed. No navigation-completed event occurred.
This attempt therefore does not establish foreground failure after a valid
physical click. It does not grant ordinary main-dev activation.

## Bounded evidence

| Boundary | Result |
| --- | --- |
| Startup publication | Accepted at application-relative 9,008 ms |
| Remaining authorization at start report | 53,582 ms |
| Capture backend RPC | 9,460–13,006 ms; 3,546 ms interval |
| Owned reader marker ready | 9,514 ms, inside the capture RPC interval |
| Persisted result | One observation, two post blocks |
| First failure | `reader_foreground / unavailable` |
| Operator observation | Marker visible; no click before closure |
| Final application cleanup | `uncertain`; not promoted to verified |
| Controller completion | 14,598 ms, failed overall, no retry |

The overlap is evidence of the capture backend RPC interval, not a claim about
individual render/network activity. The previous `worker / empty_unverified`
failure did not recur in this attempt. That single result does not prove which
earlier environmental condition caused the old empty capture, or establish
reliability across later runs.

## Cleanup containment

The application exited. A separate read-only process check found zero matching
application/Node/Chrome processes tied to this exact prepared root and confirmed
controller PID 20816 had exited. The retained manifest read `clean`. These
observations do not override the application's `uncertain` cleanup verdict or
reconstruct the complete missing Job/pipe/semantic-cleanup proof.

The disposable collector directory and QA instance are retained. They may still
contain the derived session on disk; do not reuse, copy or transfer from them.
Do not reset the consumed `execution.claim`, force-clear profile state, or delete
the collector based only on process-name/PID absence. Offline review must first
separate logical stop failure from physical owner cleanup and determine the
safe disposal/recovery procedure. The retained reader profile is preserved.

Main configuration SHA256 remained:
`52063d73180a8f6ab3e514d89cfaa81fb168e0b98381e58fa7ff1ae28d7aadae`.
Main login, user database/runtime and Auto Update were unchanged.

Evidence is under `build/x-application-qa-prepared/496e9789604e4d57813453e43230466f/`:
`execution-events.jsonl`, `execution-result.json`, `cleanup-verification.json`,
and the consumed `execution.claim`. No cookie value, raw body or account ID is
exported by this report.

## Next work

Review the QA human-click timing separately from the five-second foreground
capability. Also trace parent cancellation, broker stop, owned disposal and
cleanup publication offline. Do not infer a foreground implementation defect
from an attempt with no physical click, weaken account/ownership guards, or
repeat native transfer under this consumed permission.
