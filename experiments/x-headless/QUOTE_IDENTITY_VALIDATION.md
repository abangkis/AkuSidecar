# Quote identity recovery

Implementation is confined to the headless PoC; production AkuBridge and Sidecar collection are unchanged.

The existing X adapter remains the first DOM source. A bounded descriptor-only React resolver can accept explicit quote relations only from an object matching the primary tweet ID. It does not call getters or export raw React state. Conflicting IDs remain unresolved. Local fixtures cover explicit relation recovery, conflicting DOM/structured IDs, unrelated primary IDs and cyclic objects.

Native detail captures additionally support a navigation fallback when the quote identity is unknown. The PoC clicks only the adapter-selected quote root in the identified primary post, waits up to ten seconds for an X status destination, requires the destination's DOM identity and quoted opening text (or observed media URL) to match, and returns to the original primary post. Return readiness is also bounded to ten seconds. Failed matching remains unknown; conflicting identities are never overridden. Feed capture does not use this navigation fallback.

Resolved navigation identity is reapplied only when the same primary ID and quoted opening text remain observed. It sets both `quotedPost.permalink` and `parentPermalink`, with `observed_quote_navigation` provenance. Local tests verify content mismatch rejection and conflict handling. Video remains poster evidence, not full video acquisition.

## Live acceptance, 2026-10-01 Asia/Jakarta

Both previously unknown quote identities were recovered with `observed_quote_navigation` provenance. In each case the destination DOM post ID and opening text matched the selected quote, and return to the primary post was verified. A third, already identified quote retained its exact DOM permalink. No case remained unresolved in this bounded sample.

The structured-state path did not find matching Tweet objects in these live pages (166–167 bounded objects inspected); its success is fixture-only. The demonstrated live recovery path is navigation, not inferred IDs or response replay.

All three final captures observed authenticated UI without login requirements or detected challenges. Lifecycle receipts verify released profile ownership, healthy Sidecar and compatible Bridge after every run. An earlier attempt correctly refused to stop Sidecar while an Inbox session was active; testing resumed after the session completed.

Raw destination text/media stays in ignored `runs/` with the capture and lifecycle receipts. The aggregate is `runs/quote-identity-validation.json`. Run `node verify-quote-identity.mjs` to verify saved identity, destination-content and restore evidence; `node quote-identity-test.mjs` performs the live three-case suite with temporary Sidecar stop/restore; `node smoke.mjs` checks local fixtures.

Limitations: no general quote-coverage claim, no navigation fallback during feed capture, additional detail-navigation latency, and unknown/conflicting cases still fail closed. Full video acquisition, collector parity and a production browser lifecycle remain separate work.
