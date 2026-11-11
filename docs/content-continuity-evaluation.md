# Content continuity and successful evaluation

Smart resurfacing skips an unchanged native item within its cooldown only when
the same source and canonical evidence key have a completed candidate assessment
for the same content and context version. `CompleteRun` commits those assessments
and run completion atomically. Both selected and `not_selected` outcomes count.
An observation, a planner success, an empty completed run, or an assessment for a
different candidate does not count as successful Candidate Evaluation.

Existing successful history needs no schema migration. The guard reconstructs
the evaluated version from that run's retained, reconciled observations. It uses
the existing platform-ID-neutral content fingerprint and context comparison,
including verified X quote-parent hydration while rejecting changed quote text.
If retained evidence cannot prove a matching successful version, the candidate
remains eligible for evaluation.

A failed 503 evaluation therefore remains eligible on the next user/scheduler
Update. This change adds no provider retry, timer, or busy loop. Seen counts and
same-run continuity occurrences remain idempotent. Content changes and cooldown
expiry retain their existing evaluation behavior.

An unchanged version awaiting evaluation carries a typed `EvaluationPending`
decision. Engine replaces the `continuityEvaluationPending` run-coverage receipt
from authoritative classification on every pass, overriding capture input. This
allows a failed changed version to pass old-delivery exclusion on its next
evaluation. Semantic exact-replay bypass additionally requires a successful
assessment for that current run/source/key. Inbox labels only actual `fail_fast`
decisions as skipped; pending failures stay captured-only until assessed.

The regression fixture contains sanitized selected fields from three persisted
503 failures and two successful counterexamples (selected and not selected).
Replay timestamps use the current test clock; original timestamps remain fixture
provenance. All tests are offline. Separate Instagram vision-job deduplication
and retry behavior is unchanged; a ready vision job alone is not a candidate
assessment receipt.
