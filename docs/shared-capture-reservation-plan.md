# Shared capture reservation: deferred architecture work

Status: proposed follow-up, not implemented or activated.
Date: 2026-10-28.

## Problem and current MVP correction

The daily-X scheduler branch could start a disposable collector and derive a
session while an earlier LinkedIn command in the same update session was still
claimed. Store correctly rejected the X authorization. A separate native browser
does not imply a separate Store capture lane.

The conservative correction calls the existing predecessor-drain rule before
daily-X worker registration, durable StartRun, native startup or seed. Earlier
runs must be terminal, their active/pending reasoning must have retired, and
their queued/claimed commands must be drained. A blocked X run stays queued;
ordinary reasoning retirement and the existing hybrid scheduler retry provide
wakeups. No waiting is performed while holding the scheduling mutex.

This corrects the demonstrated scheduler bypass. It deliberately waits for
predecessor reasoning as well as capture. The drain check is observational: it
does not reserve the lane against another entry point that races after the
check. Store remains authoritative and fails closed on contention. This is not
a claim that all concurrent admission races have been removed.

The reader remains independent and usable while X captures. This work does not
require simultaneous captures from multiple sources. Offline regressions do not
establish native or daily-use acceptance.

## Intended responsibility boundaries

| Component | Responsibility | Must not do |
| --- | --- | --- |
| Engine scheduler | Ordering, queued contention, wakeups, cancellation and worker joins | Treat an idle snapshot as a reservation or seed before admission |
| Store | Atomically reserve the session capture lane and enforce command/result eligibility | Treat a reservation as account/login authorization |
| Native owner/broker | Browser ownership, account/login evidence, approved derivation, original deadline and cleanup | Decide durable scheduling order or bypass Store admission |
| Reader actions | Open source/post through their existing authorized routes | Consume a capture reservation merely to open a window |

## Proposed reservation lifecycle

1. Engine requests a reservation in a Store transaction before collector startup
   or derivation. Store checks predecessor capture eligibility and contention.
   A busy result leaves the run queued; it is not an authentication failure.
2. A successful reservation is bound to session, run, attempt, owner generation
   and process incarnation. All ordinary, manual and daily claim entry points
   must honor the same reservation. No check-then-start alternative remains.
3. Only the reservation holder may start the native collector. Native ownership,
   account/login and seed checks then produce the existing separate authority.
   Reservation alone cannot claim or commit an authenticated result.
4. Capture claims/results retain existing replay, cancellation, account,
   challenge, source and deadline guards. Predecessor reasoning may overlap
   only when no predecessor capture or capture continuation remains eligible.
5. Retain the reservation until capture operations and cleanup have drained.
   Release is conditional on the exact reservation identity. Late cleanup from
   an older attempt cannot release a successor's reservation.
6. On cancellation, close admission, cancel and join work, reconcile commands,
   then release after confirmed cleanup. Uncertain cleanup keeps the lane
   blocked for explicit recovery; timeout is not proof that an owner stopped.
7. Restart invalidates old process authority and reconciles durable reservations
   before opening admission. Reconstruct scheduling state, never account consent
   or reusable authentication authority, from durable data.

The reservation wait/expiry budget is separate from native startup and the
original 60-second attempt clock. Queue waiting must not consume or reset that
clock; retries cannot renew a consumed attempt. These changes do not authorize
new transfers, hot switching, automatic fallback or renewal/rotation scope.

## Implementation sequence and decision gates

1. Inventory every claim/admission path and define typed reserved, busy, stale
   and recovery-required outcomes. Specify fencing and capture-continuation
   eligibility before selecting a schema migration. Do not assume current
   ordinals alone define every continuation.
2. Implement Store transactions and conditional release with synthetic databases.
   Define upgrade, downgrade and crash-recovery behavior explicitly; an older
   binary must not ignore an outstanding reservation and reopen capture.
3. Wire the scheduler to reserve before native work, queue contention and wake
   once capacity is released. Preserve stable Browser lifecycle and reader paths.
4. Make ordinary, manual and daily claims consume the same reservation contract.
   Keep account authorization separate and retain current strict rejection tests.
5. Validate deterministic races and recovery offline, then accountless native
   fixtures. Prepare rollback before requesting separate live/runtime approval.

Do not implement only the daily-X half of this contract: a reservation that
ordinary claim paths can ignore does not provide exclusivity.

## Acceptance criteria

- Barrier-controlled ordinary/manual/daily competitors have one winner; losers
  remain queued without creating collectors, deriving sessions or duplicating
  commands. No same-session capture overlap crosses the Store boundary.
- A completed predecessor capture can allow successor capture while predecessor
  reasoning continues, provided no further predecessor capture is eligible.
- Repeated mixed Instagram/LinkedIn/X batches progress without stranded runs;
  reader interaction remains independent of reservation ownership.
- Cancellation before reservation, before StartRun, during startup/seed, during
  capture and during cleanup cannot resurrect work or leak capacity.
- Old attempt cleanup and duplicate release cannot release a successor slot.
  Restart, failed cleanup and downgrade remain closed until safely reconciled.
- Existing account/login, ownership, challenge, exact claim/result, duplicate,
  deadline and stale-generation negative tests continue to reject invalid work.
- No diagnostic contains cookie values, tokens, raw bodies or account identity.

## Effort and rollout boundary

The Astra consultation estimated 6-10 engineering days for the bounded shared
reservation implementation and deterministic regression coverage, potentially
2-3 weeks if restart/cancellation contracts need substantial restructuring.
The conservative predecessor correction was estimated at 2-4 engineering days
including focused regression review. These are planning judgments, not measured
elapsed-time commitments; live provider validation and operational rollout are
excluded. Re-estimate after the claim-path inventory.

Current runtime, login, user database, Supervisor and Auto Update are outside
this offline implementation scope. Keep Store authorization strict. Native
acceptance and activation require their own explicitly authorized operations.
