# Living Topics lifecycle proof V5

V5 tightens lifecycle evidence without adding a provider invocation or changing
Memory retention. The original V4 temporal and relevance rules still apply.

Each lifecycle claim describes one event. A rollout completing does not establish
that a promised reset was issued, or that a credit expired. Terminal status
(`completed` or `cancelled`) requires specific evidence from the supplied retained
source text; generated Memory titles and summaries cannot provide that proof.
Missing proof leaves an explicit uncertainty, including in the claim prose and
material value, rather than merely changing a badge above an assertion of completion.

Accepted terminal claims retain a bounded proof excerpt and its evidence ID in
the snapshot. Their displayed assertion comes from that checked excerpt, not
an unchecked paraphrase about a different event. No additional full copy is
created for recall-only Memory.

The source text remains untrusted data. Proof validation does not grant it any
instruction authority, and matching a quotation does not by itself establish that
the real-world assertion is true. Conservative English lexical guards can leave
valid events, including other languages or unfamiliar phrasing, unconfirmed.
Source-language and semantic coverage must be measured separately
from deterministic regression checks.

## Existing topics

The projection contract is `current-projection-v5`; database schema remains 25.
Older snapshots remain immutable history and cannot inherit V5 current authority.
A previously evaluated topic without a V5 projection displays `Refresh needed`.
An explicit refresh or the next ordinary evidence/criteria change uses the new
input digest and proof contract. Installing this change does not itself queue a
provider call or fetch missing source text. Recall-only evidence therefore may
remain unable to establish completion until adequate evidence is available.

## Acceptance cases

- A source reporting a completed rollout and promising a later reset must yield
  separate lifecycle claims; the promise is not completion proof.
- A historical schedule remains an announcement unless completion is evidenced.
- Summary-only completion, invented quotations, and quotations from uncited
  evidence cannot establish terminal status.
- Source-supported cancellation and completion can be admitted independently.
- An older source arriving later does not prove a later event completed or that
  a credit expired.

These cases exercise the host boundary using controlled model responses. They
are not a measured live-provider accuracy rate. No new inference baseline is
claimed by this implementation.
