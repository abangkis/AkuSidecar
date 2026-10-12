# Timeline conversation adjacency

Implemented in source on 13 October 2026. Activation requires a separately
authorized Sidecar rebuild/restart. Validation uses offline regressions and a
synthetic desktop/mobile preview, not installed or authenticated collection.

## Data contract and responsibility

Source adapters capture structural relations in `Block.directContext`. A
`replies_to` relation with a `post` target and validated captured provenance is
the input; quotes, comments, shared authors and similar text are not input.
X currently supplies this post-reply contract. A future source must explicitly
support and validate the same relation; a LinkedIn feed comment is not a reply
between two Timeline posts.

`ListTimeline` derives an optional response-only `conversation` projection:

```json
{"kind":"replies_to","targetTimelineId":"retained-parent-id","status":"resolved","provenance":"observed_response"}
```

The projection indexes only the selected, already-hydrated page by source and
native post identity, reusing existing identity helpers. It emits `not_in_view`
when no loaded parent matches and `ambiguous` for duplicate native identities
or conflicting parent targets. It ignores self references, invalid provenance,
unsupported relation kinds and conflicting own ID/permalink evidence. Multiple
valid observations of one edge corroborate it; they do not create ambiguity.
No additional query, capture, model call, database migration, enrollment or
stored rank/evidence rewrite is required.

The generic `timeline-conversations.js` module consumes local Timeline IDs and
relation kind/status. It contains no X URL parser or source-name branch. Other
adapters can enable the same Timeline behavior through the validated contract.

## Presentation rules

1. Apply normal Timeline selection, pagination, semantic-report policy and AI
   routing before grouping. Group only cards admitted to the inline view.
2. Place a conversation at the existing position of its first/newest-ranked
   member. Within the group, show the root first, then each reply after its
   parent, retaining existing order between sibling branches.
3. Keep each card once, preserve its own actions and Related Context trigger,
   and show a conversation line plus a `Reply to <author>` cue. The cue navigates
   to the immediate parent and respects reduced-motion preference.
4. Use the group's anchor member for batch separators. When another member came
   from a different check, label its original check time; moving a retained
   parent does not claim that it was newly captured. Unrelated groups retain
   their original ordering. Parent/reply cards remain direct children of the
   Timeline container so existing viewport cues and observation still work.
5. Keep a reply standalone when its parent is outside the page, excluded by AI
   routing, ambiguous, or cyclic. Explain the missing/matching state rather than
   render a synthetic parent or bypass visibility/retention policy. Older pages
   may resolve the relation when normal Timeline loading brings the parent into
   view; there is no automatic parent fetch.

Example: input order `Reply B, unrelated C, older parent A` becomes `A, B, C`.
A and B occupy B's original group position; A carries its original check date.
Existing snapshots without a captured reply relation retain their usual order.

## Regression coverage

- Store projection: exact aliases and ID-only reference, source isolation,
  own-identity validation, quote/comment/self rejection, duplicate/conflicting
  parents, corroborating provenance, page limits and both semantic modes.
- Pure layout: stable group anchoring, chains/siblings, duplicate IDs, unsupported
  or invented relations, cycle rejection, 1,000-card stack safety and immutability.
- Actual Timeline render path: one observed card per input, truthful batch
  separators/check times, parent navigation and authoritative AI routing.
- Cues use text nodes, so authored names cannot become executable markup.
