# Living Topics Current Projection V4

V4 separates the latest state supported by local evidence from historical facts.
It does not claim that local capture covers every development in the world.

- Claims have independent `temporalStatus` (`current`, `historical`, `unknown`)
  and `eventStatus` (`announced`, `ongoing`, `completed`, `cancelled`, `unknown`).
  A completed event can be the latest known state. Completion requires cited
  evidence; a deadline passing or an omitted claim does not prove completion.
- Relevant uncertain developments remain visible, independently of source
  strength. Rollout completion does not establish compensation-credit expiry.
- Synthesis receives sources ordered by publication time, with unknown times
  preserved. Host-derived `latestEvidenceAt` and snapshot `evidenceAsOf` are
  publication timestamps, not event dates or an assurance of complete coverage.
- The overview includes only current, central, supported claims. Historical
  evidence remains visible below the latest state and uncertainties. With no
  supported latest state, the overview explicitly says so.
- Temporal and event-status transitions create material deltas. Claim removal
  is `removed`; legacy `resolved` is presented as no longer in the projection.
  Neither label means the event completed. New corroboration alone need not
  create material history.
- Semantic routing receives at most five recent attached source summaries to
  recognize concrete event continuations despite changing terminology. Topic
  criteria and explicit exclusions retain authority. Previous generated prose
  is never routing or synthesis evidence, and source text remains untrusted.
- Topics accept at most 30 active members. Manual, automatic, move, citation,
  and UI boundaries share this capacity. History still has its separate limit.

Schema 25 adds `evidence_as_of` and queues a coalesced V4 rebaseline for existing
topics with evidence. Older projections remain immutable historical records,
including the latest prior-contract projection even when non-material. Old
rejected routing receipts remain unchanged; a local activation scan can propose
additional evidence for review using the new continuity context.

Acceptance examples: an older milestone reset is historical when superseded
by a new reset episode; banked-reset policy remains relevant even with weak
attribution; a sourced rollout completion leads the latest state without
inventing credit expiry; missing dates remain unknown; the 30th member succeeds
and the 31st is rejected without losing existing memberships.
