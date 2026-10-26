# Unified native X QA: capture-start sequencing acceptance

## Scope and verdict

On 2026-10-26, one explicitly authorized attempt consumed preparation
`1617147ae968446cb6887b138d4edd7f`. The application used unchanged Go commit
`d24d882969a0817649c56b1a3d7345fbf6b8bc6e`, source SHA256
`74d4d5641662d336a307ec5086b21bc361383caef1aa5805e91e9d74aeb7fbf8`.
The new controller was uncommitted and independently pinned by SHA256
`bef20c9916b2dd30a558db5d50180487271fd6b0d2ef73916db2d1d4fbca565d`.
All six artifact hashes and the source graph were verified before execution.

**The full selected one-shot harness passed.** Startup was accepted, the
capture-start gate opened once, reader foreground and post navigation succeeded
during the in-flight capture backend RPC, one observation/two post blocks were
persisted, and stop/full cleanup succeeded. Exit code was zero, with no terminal
reason, operational first failure, parse error or stderr bytes.

Only `auth_token` and `ct0` could transfer from the retained QA reader to the new
disposable collector. The original maximum 60-second seed authorization was not
renewed. There was no retry or main login/database/runtime/Auto Update change.
Reasoning used the deterministic QA provider. No cookie values, account IDs or
raw bodies are included in this record.

## Timing evidence

Application-relative stage times:

| Event | Elapsed ms |
| --- | ---: |
| Startup accepted | 11,725 |
| Capture backend entered | 12,110 |
| Reader requested | 12,154 |
| Owned marker ready | 12,419 |
| Reader foreground verified | 13,664 |
| Post navigation requested | 13,664 |
| Post navigation completed | 14,164 |
| Capture backend returned | 16,599 |
| Capture settled / Stop requested | 16,826 |
| Parent cleanup started | 17,571 |

The reader request followed capture entry by 44 ms. Marker readiness, foreground
verification and navigation completion all fell inside the recorded backend RPC
interval. The existing `capture_backend_rpc_interval` overlap criterion reported
`observed`; neither that criterion nor account/Store/ownership guards changed.
This establishes overlap with a backend call, not continuous network activity or
continuous rendering. The controller completed in 17,813 ms without retry.

## Cleanup and evidence

Application cleanup reported `verified`. A separate scoped Windows process check
found no matching QA application/Node/Chrome process, including the controller.
The exact retained QA manifest was `clean`. The runtime-plan hash, exact new
collector path, ancestry and descendants were checked before removal; no reparse
points were allowed. Only this attempt's disposable collector root was removed.
The retained reader profile and older retained collector were preserved. QA
database and sanitized evidence remain under the consumed preparation root.

Main configuration SHA256 stayed
`52063d73180a8f6ab3e514d89cfaa81fb168e0b98381e58fa7ff1ae28d7aadae`.

Evidence under `build/x-application-qa-prepared/1617147ae968446cb6887b138d4edd7f/`:
`prepared-review.json`, `execution-events.jsonl`, `execution-result.json`,
`cleanup-verification.json`, `verify-cleanup.ps1` and consumed `execution.claim`.
Never reset or reuse this plan; its collector now refers to a removed directory.

## Acceptance boundary

The isolated application journey now has native capture, reader overlap and
cleanup evidence. The report's `nativeQualified:false` is preserved: a passing
QA harness does not issue general production/session authority. Main dev remains
default-off and was not activated. The next step is review/commit of this harness
and activation planning for the existing manual X-only pilot, not another
identity-renewal prerequisite or an automatic native retry.
