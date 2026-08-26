# Composable prompts

Status: development source of truth. This document defines how AkuSidecar
assembles prompts while preserving the canonical workload contract across
reasoning providers.

## Concept

AkuSidecar uses three terms:

```text
Canonical Workload Prompt + Provider Prompt Overlay = Composed Prompt
```

The Canonical Workload Prompt is the provider-neutral contract for a workload.
It contains the task, security boundary, evidence rules, ownership rules, and
output-behavior instructions that must hold for every provider. A Provider
Prompt Overlay is a small, provider-and-workload-scoped compatibility addition.
The Composed Prompt is the deterministic result sent to the selected adapter.

An overlay is an additive compatibility layer, not a second full prompt. It
may clarify how a provider should satisfy a known constraint, but it cannot
remove, replace, weaken, reorder, or contradict a canonical section.

## Goals

- Keep security, evidence, identity, and output-ownership rules canonical.
- Allow endpoint-specific compatibility instructions without changing other
  providers or workloads.
- Make provider behavior observable and testable as a composed artifact.
- Keep prompt assembly deterministic, reviewable, and easy to version.
- Preserve strict local schema validation after every provider response.

## Non-goals

- Creating a separately maintained full prompt for every provider.
- Moving schema enforcement into prose or trusting an overlay as validation.
- Normalizing, truncating, or silently repairing model output because a prompt
  asked for a bound.
- Changing the Supervisor lifecycle or provider selection policy.
- Applying a Candidate Evaluation overlay to Planning, Semantic Event
  Resolution, or AI Deep Detection.

## Composition contract

Each prompt workload has an ordered list of canonical sections. The composer
emits those sections in their declared order, followed by at most one overlay
for the exact `(provider, workload)` pair, followed by the dynamic bounded
input section. When no overlay is registered, the composed bytes must remain
byte-identical to the established provider-neutral prompt.

The composer must enforce these invariants:

1. Every composition includes the canonical security and evidence sections.
2. A provider overlay is selected by exact provider and workload identity; it
   never matches by substring or broad provider family.
3. An overlay appears at most once and has a deterministic position.
4. The overlay cannot supply a replacement for canonical text or dynamic
   evidence. Evidence remains bounded and application-owned.
5. Provider-specific constraints are compatibility guidance only. The
   application schema remains the enforcement source of truth and is applied
   locally to the complete response after inference.
6. Existing provider adapters that have no overlay receive the canonical
   composition unchanged.

Prompt text is not a security boundary. Untrusted evidence remains delimited
and must never be treated as instructions, even when an overlay is present.

## Current Gemini overlay

Gemini Candidate Evaluation has returned array values beyond the schema's
declared bounds during development (for example, six `topicTags` where the
contract permits five, and four `topicFacets` where it permits three). The
Gemini Candidate Evaluation overlay therefore states, explicitly:

- emit no more than five `topicTags` per assessment;
- emit no more than three `topicFacets` per assessment; and
- use only the `topicFacets` enum values declared by the response schema.

This overlay is scoped only to `(gemini, candidate_evaluation)`. Gemini
Planning, Gemini Semantic Event Resolution, and Gemini AI Deep Detection keep
their canonical prompts unchanged. Codex App Server, Groq, and Ollama also
keep their canonical Candidate Evaluation prompt unchanged.

The overlay is a compatibility experiment, not a relaxed validation policy.
If Gemini still emits invalid values, strict local validation must reject the
response. AkuSidecar must not truncate or normalize the arrays to force
acceptance.

The initial bounded live gate evaluated seven synthetic candidates through
the existing `6+1` Gemini chunk boundary. It returned seven items and seven
assessments in 18.64 seconds and passed the unchanged complete local schema.
This is initial compatibility evidence, not a reliability baseline; normal
development batches remain the authoritative follow-up signal.

The first subsequent scheduler batch also completed Candidate Evaluation. Its
four effective X assessments stayed within the contract (`topicTags` maximum
four and `topicFacets` maximum two). Semantic Event Resolution then identified
the selected report as a duplicate and prevented readmission. This is one
normal-batch observation and must still be followed across additional batches.

## Relationship to Semantic Event Resolution

Semantic Event Resolution already has a separate domain prompt because it
resolves relationships between selected candidates and historical events. It
is not a Candidate Evaluation overlay and must not inherit Candidate
Evaluation instructions. Future Semantic overlays, if needed, must be
registered independently under `(provider, semantic_event_resolution)` and
tested against the Semantic schema and duplicate-admission controls.

## Schema and contract testing

The complete schemas under `schemas/` are authoritative for cardinality,
enums, types, and local acceptance. A prompt overlay may repeat a small
constraint to improve provider compliance, but its numeric values and enum
list must be contract-tested against `schemas/reasoning-result.schema.json`.
Tests must fail if the overlay drifts from the schema. The wire projection
may continue to remove unsupported Gemini keywords, including `maxItems`,
but that projection does not alter the complete local schema.

Tests must cover:

- a golden default composition for the existing Candidate Evaluation prompt;
- exact absence of overlays for Codex, Groq, and Ollama;
- exact single appearance of the Gemini Candidate Evaluation overlay;
- preservation of canonical security and evidence sections;
- presence of the overlay in every Gemini evaluation chunk;
- overlay limits and enum values matching the complete schema; and
- strict rejection of schema-invalid model output.

## Versioning and evolution

Changes to canonical sections are workload-contract changes and require a
golden prompt review plus provider regression tests. Changes to an overlay
must identify the exact provider/workload scope, preserve the canonical
sections, and include a compatibility rationale and live or fixture evidence.
Changing a schema bound requires updating the schema first, then its contract
tests and any overlay generated from that contract. Do not change a prompt,
wire projection, and local acceptance rule in one opaque step.
