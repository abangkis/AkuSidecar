# Automatic Related Context admission

Admission policy version: `content-context-v6`. New feedback records identify
this policy; existing user feedback retains its context identity and history.

Related Context searches local saved content, then applies a stricter admission
rule than a user-entered Library search. Retrieval overlap alone cannot establish
a relationship. Empty results are valid when the saved collection does not
contain sufficient evidence.

For a named product or complete model identity, both records must bind the same
concrete issue to that owner in bounded text spans. For example, `Codex reset`
can match `Codex resets`; `Codex licensing. ChatGPT resets.` cannot establish a
Codex reset relationship. Ordinary prose such as `can`, `use`, `when`, or `think`
does not open admission or count as an issue in ranking.

The deterministic matcher uses an explicit issue vocabulary and supported
technical phrases, including reset, licensing, citation grounding, and error
analysis. New issue vocabulary requires positive examples and controls against
borrowing an issue from another subject. Unrecognized issues remain unresolved;
this rule is deliberately conservative rather than a general semantic parser.

The owner and issue must be attributable within their source field. Other
products and companies can delimit ownership without becoming admission
subjects themselves. Summary text can assist retrieval, while its restricted
technical evidence path requires a corroborated primary owner. Internal proof
records retain the field and rune spans; no new public API field is required.

Ranking and public match reasons use admitted issue evidence. General word
overlap cannot outrank a concrete issue. Complete model versions and qualifiers
remain distinct, and deliberate Library searches retain their broader policy.

Campaign identity and campaign aliases are a separate future change. The matcher
does not equate every OpenAI mention, sprint, or numbered day with one campaign.
Rebuild and restart AkuSidecar to activate admission changes; source-level and
offline database checks do not prove that the running service uses them.
