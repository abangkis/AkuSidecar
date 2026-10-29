# Bounded daily-source retries

The controller permits at most two consecutive additional attempts after a
confirmed transient timeout/network failure: cooldowns are 30 then 60 seconds.
The next normal manual or automatic update performs the retry. No update is
created automatically, no old run/command/result is replayed, and Auto Update
configuration is unchanged. A successful capture and verified cleanup reset the
consecutive failure allowance. Exhaustion stops the source in this process.

Admission requires all of the following:

- The collector and borrowed targets have completed verified cleanup.
- Store permit revocation and closed-run reconciliation succeeded.
- The provider independently confirms it can accept a fresh generation.
- The process grant, parent lifetime and selected profile remain valid.
- The failure has a trusted operational transient classification.

Every new attempt creates a fresh run, command, collector, nonce/generation and
account/session verification. Each batch still has its own 60-second maximum;
the cooldown occurs before that batch starts. Stop/opt-out/shutdown cancel a
pending cooldown without starting another collector or restoring permission.
Saved source/profile permission survives operational failure.

Account/login rejection, challenge, changed ownership, session mismatch,
uncertain cleanup, canceled work and unknown failures remain terminal. Generic
IG fetch errors and server rejections remain ambiguous: a login redirect or
challenge can produce those categories. They are not automatically retryable.
An IG fetch's own abort deadline is a known timeout; the next attempt must
verify account/session again. X exposes explicit private operational codes for
branded capture transport timeouts/network errors, separate from diagnostic
JSON. A peer rendezvous timeout or diagnostic text alone cannot authorize retry.

The source status exposes `retry-wait`, `retryCount`, `retryLimit`, and `retryAt`.
Settings and Timeline distinguish cooldown from a terminal failure requiring
restart. The first cause remains fixed through each attempt's cleanup; a new
attempt starts its own diagnostic record. Historical runs and bounded logs keep
the prior attempt's outcome.

Validation is offline/synthetic. No runtime activation or real session transfer
is implied by these checks.
