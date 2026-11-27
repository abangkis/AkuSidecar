# Reply capture and Timeline conversation cues

## Presentation contract

An observed reply edge can bring its already-loaded parent closer in Timeline.
The group keeps the position of its highest-ranked member in the selected
view, reads parent-first, and renders each card once. Stored ranking, creation
times, published times, batch membership, and AI routing remain unchanged.
An older parent retains the `From another check` label. The grouping does not
fetch more pages or force filtered parents back into the view.

Source and Brief show a connector between adjacent cards when the second card
is a direct reply to the first. Siblings stay at the same indent level. The
conversation count appears on the root card. Unknown, conflicting and
out-of-view parents get a neutral status label.

## Tradeoffs

Grouping may move an older parent ahead of unrelated updates and a long thread
may occupy much of the selected page. These are presentation tradeoffs, not
new-event or freshness claims. Preserve the original timestamps and check
labels, keep content collapsed according to the existing text policy, and do
not duplicate parent cards. Quotes, text similarity and adjacency cannot create
reply edges.

## Validation boundary

The `0.9.3` / `source-adapters-v119` source candidate finalizes exact X reply
evidence before submitting an observation in both capture controllers. Windowed
capture performs its final cache lookup independently of deferred-media results.
Owned headless performs a bounded, reply-only lookup after its last snapshot
read and recovery work, immediately before observation serialization. Both use
the same Bridge-owned identity validation and reply merge rules. This is not
an authority to navigate, fetch another response, or guess a parent from text.

Repeated observations across scroll snapshots may corroborate the same child;
duplicate identities inside one snapshot remain ambiguous. Existing quote
semantics, relation limits, capture deadlines and headless evidence-size limits
still apply. Borrowed Quiet pages do not install or read an additional passive
observer. The final lookup cannot recover evidence that never reached the cache
or arrived after its deadline. Contradictions against retained reply evidence
remain ambiguous; this change does not add response-history storage.

The source candidate requires a separately authorized runtime activation. It
does not rewrite retained observations or repair previously captured posts.

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
older-check provenance and Source/Brief conversation cues.
The browser preview is synthetic; it does not confirm a specific relationship
on live X or rewrite already saved captures. Activate the compatible runtime
pair separately before checking newly captured real posts.

## Retained reply diagnostics

New X observations can include `coverage.xReplyDiagnostics`. Each row identifies
an exact captured post and snapshot and records the DOM marker outcome, final
reply-cache lookup, and resulting relation. A cache hit can bypass the DOM scan,
which is recorded as `not_checked`. The final lookup distinguishes a miss,
expiry, unavailable observer, error, and capture deadline. The relation outcome
records which evidence was retained, unresolved or ambiguous relations, and
Headless merge attempts skipped for the observation byte budget.

The producer retains up to 64 rows per observation. A truncated report or one
with unbound candidates has `complete: false`. Completeness describes the
recorded rows. The passive observer still reports its own coverage. Report rows
carry the captured candidate ID and outcome enums. They do not carry DOM text,
response bodies, or parent identifiers.

Sidecar binds the report to the retained X observation and omits it from
run-level coverage. Malformed reports are discarded while valid post evidence
is admitted. Bridge and worker transports omit the diagnostic report when
necessary to stay within their existing byte limits. Timeline grouping
continues to use the observed reply relation.
