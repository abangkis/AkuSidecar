# Reply capture and Timeline conversation cues

## Presentation contract

An observed reply edge can bring its already-loaded parent closer in Timeline.
The group keeps the position of its highest-ranked member in the selected
view, reads parent-first, and renders each card once. Stored ranking, creation
times, published times, batch membership, and AI routing remain unchanged.
An older parent retains the `From another check` label. The grouping does not
fetch more pages or force filtered parents back into the view.

The chip beneath the author names the immediate parent, not the conversation
root. A muted 2px rail and small nodes identify the group; adjacent members use
a 6px gap within the existing 16px Timeline grid. Branches stay at one visual
indent level so sibling replies are not presented as replies to one another.
The chip also remains available in brief mode and compact report cards.

`View parent` focuses the exact loaded parent and adds a temporary outline.
Reduced-motion users get an immediate jump. On narrow screens the secondary
action hint is hidden, while the native button retains its descriptive
accessible name. Unknown, conflicting and out-of-view parents get a neutral
status label and no connecting rail or guessed navigation target.

## Tradeoffs

Grouping may move an older parent ahead of unrelated updates and a long thread
may occupy much of the selected page. These are presentation tradeoffs, not
new-event or freshness claims. Preserve the original timestamps and check
labels, keep content collapsed according to the existing text policy, and do
not duplicate parent cards. Quotes, text similarity and adjacency cannot create
reply edges.

## Validation boundary

The v116 worker retains exact parent IDs arriving through the existing passive
response observer after the DOM snapshot. A response-created relation carries
no parent body. A verified detail-page snapshot can also contribute its exact
reply edge when its text replaces the excerpt or when a more complete current
body is retained. Owner identity, parent identity, provenance and context bounds
must agree. Quote semantics stay separate. Malformed optional evidence cannot
erase an independently valid edge; conflicting valid parents produce an
ambiguous diagnostic and clear the chosen reply parent. The neutral UI label
for that diagnostic does not create a Store edge or a connector.

Synthetic regressions cover late response evidence, exact-owner binding,
conflicting parents, quote coexistence, detail-page reply preservation,
Observation serialization, grouping, branches, missing parents, AI filtering,
older-check provenance, source/brief cues and accessible parent navigation.
The browser preview is synthetic; it does not confirm a specific relationship
on live X or rewrite already saved captures. Activate the compatible runtime
pair separately before checking newly captured real posts.
