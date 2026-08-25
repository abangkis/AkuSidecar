# Development reasoning-provider evaluation

Status: active assessment. Groq remains an opt-in hidden assessment provider.
Gemini Flash Lite has passed the four bounded Sidecar live gates and is exposed
as a persisted application-level Settings choice, while Codex App Server
remains the checked-in development default and fallback.

This work seeks a free or materially cheaper provider for routine development
flows. It is separate from the completed payload-saving effort: model and route
trade-offs may be evaluated, but further prompt/token optimization is not the
current priority. Exceptional or quality-critical cases may continue to use
Codex App Server even if a development fallback is later accepted.

## Boundaries

- `reasoning.activeProvider` remains unchanged until a candidate passes the
  relevant SDK capability, Sidecar corpus, reliability, and shadow gates.
- An SDK model entry or successful synthetic request is not Sidecar
  integration approval.
- Assessment uses synthetic or source-controlled fixtures first. A live shadow
  run must not persist candidate-provider decisions to Timeline or model state.
- Provider credentials are never stored in the tracked Sidecar configuration,
  SQLite, receipts, diagnostics, or assessment artifacts. Provider entries
  carry only a namespaced `credentialRef` such as `groq.primary` or
  `gemini.primary`. AkuSidecar resolves that reference at provider composition
  time from the ignored centralized local store at
  `runtime/config/credentials.local.json`.
- OpenRouter development tooling uses `OPENROUTER_API_KEY`. The retired
  `OPENROUTER_CODEX_KEY` name is not part of the contract.
- ZDR enforcement is a separate SDK/provider-routing experiment. The Nemotron
  measurements below did not send `provider.zdr` or
  `provider.data_collection` and must not be cited as strict-privacy route
  availability evidence.

## Nemotron Ultra observation — 2026-08-25

Candidate:
`nvidia/nemotron-3-ultra-550b-a55b-free` in the SDK catalog, routed by
OpenRouter as `nvidia/nemotron-3-ultra-550b-a55b:free`, reasoning `high`.

The initial SDK availability calibration used text output because this route
does not currently declare native JSON Schema support. Two of three requests
completed in 2.42–2.90 seconds; one failed during response decoding. This was
enough to continue assessment, not enough to establish reliability.

The second gate exercised one synthetic case for each Sidecar workload using
the checked-in application schema:

- Acquisition Planning
- Candidate Evaluation
- Semantic Event Resolution
- AI Deep Detection

The model was asked for JSON in text mode. The evaluator parsed the response,
validated it locally against the corresponding Sidecar schema, and checked a
bounded expected invariant. It retained no raw prompt or output and did not
change the active Sidecar runtime or database.

Across three repetitions per workload:

- 11 of 12 requests completed (`91.7%`);
- every completed response parsed as JSON, passed its Sidecar schema, and met
  the simple expected invariant;
- one semantic-resolution request returned HTTP 200 with provider status
  `failed` and no usable output;
- seven successful repetitions had valid end-to-end timing: median `7.72s`,
  observed range `5.28–65.66s`, and three required `47–66s`;
- Candidate Evaluation took `47.12s` and `65.66s` in its two correctly timed
  repetitions, while AI Deep Detection reached `61.22s` once.

The first four successful outputs prove only simple prompt-to-JSON feasibility;
their initial evaluator measured time to response headers rather than complete
body delivery, so those four latency values were discarded. The evaluator was
corrected before the remaining measurements.

Conclusion: Nemotron Ultra remains an experimental candidate. Its successful
outputs were structurally strong on simple cases, but `91.7%` completion and
the observed tail latency are below the threshold for a default reasoning
provider. It may progress to a harder offline corpus or a non-authoritative
fallback experiment; it must not replace Codex App Server yet.

## Remaining evaluation gates

1. Run positive, negative, ambiguous, and boundary cases from source-controlled
   corpora for all four workloads. Measure schema validity separately from
   domain correctness.
2. Treat any false admission, unsafe semantic merge, transferred AI-origin
   evidence, fabricated candidate identity, or schema-invalid output as a
   critical failure rather than averaging it away.
3. Record request completion, typed provider failure, end-to-end p50/p95,
   token usage, invariant result, and output hash. Do not retain raw content.
4. Compare the same corpus and timeout budget against Codex App Server and the
   other shortlisted free providers. Do not compare calibration latency from
   different payloads as if it were equivalent workload performance.
5. Only after the offline gate passes, run a bounded development shadow check:
   Codex remains authoritative; the candidate receives equivalent bounded
   evidence, but its decisions cannot mutate Timeline, calibration, preference,
   semantic-event, or AI-detection state.
6. Groq provider composition is now available for explicit development tests.
   Keep it non-authoritative until it passes the same corpus and shadow gates.

## Groq integration baseline — 2026-08-25

AkuSidecar consumes Inference SDK v0.11.0 and exposes the curated Groq route
`openai/gpt-oss-120b` at low, medium, and high reasoning. All four Sidecar
workloads use provider-strict JSON Schema; conditional-free retries remain zero
by default. The checked-in active provider is still `codex-app-server`.

The centralized local JSON store is the development secret boundary. Its
resolver contract remains replaceable by an OS-backed implementation without
changing provider entries, which retain only an opaque namespaced
`credentialRef`.

SDK v0.11.0 fixes the Groq model preflight for provider model IDs containing a
slash. The opt-in `TestGroqLiveSidecarWorkloads` retains the Sidecar-level live
acceptance gate; its current result is recorded below.

### Live result after SDK v0.11.0

The SDK fix is verified: model preflight now succeeds. A synthetic SDK
calibration at high reasoning failed with a 128-token output budget, then
passed with 512 tokens in 1.70 seconds. The successful run reported 189 input,
109 output, 84 reasoning, and 298 total tokens with SDK schema validation
passed.

Sidecar-level results remain below the acceptance bar:

- the dotted workload identity had to be projected to a provider-safe schema
  name (`akusidecar-planning`) while retaining the stable dotted SchemaID;
- Planning completed successfully once after that correction, but subsequent
  equivalent requests returned provider HTTP 400;
- Evaluation once completed with zero items for one candidate, demonstrating
  that its former schema did not encode the one-result-per-candidate invariant;
- Evaluation now projects exact candidate counts into both result arrays and
  retains the local exact-count check;
- the former 16K output reservation triggered HTTP 413 on the free route, so
  Groq uses workload-scoped budgets of 512 for Planning and 4096 for the larger
  structured workloads; existing providers retain the 16K default;
- after reducing the Evaluation budget, the request reached the provider but
  still returned HTTP 400.

No raw prompt or model output was retained in these measurements. Groq remains
hidden from Settings and must not replace Codex App Server. The next Groq work
should be an offline schema/prompt compatibility study or a later provider
retest, not additional automatic retries in the active Sidecar flow.

## Gemini Flash Lite integration baseline — 2026-08-25

AkuSidecar consumes Inference SDK v0.12.0 and composes the native stateless
Gemini Interactions v1 adapter with `gemini-3.5-flash-lite`. Configuration
contains only `credentialRef: gemini.primary`; the key is resolved in memory
from the ignored centralized local store and sent by the SDK only in the
provider header.

Gemini accepts a narrower JSON Schema subset than AkuSidecar uses locally. The
composition removes the unsupported wire keywords (`$schema`, `pattern`,
`minLength`, and `maxLength`) plus `maxItems` from a cloned provider schema.
Although Gemini documents `maxItems`, its Interactions v1 endpoint consistently
rejected the Semantic Event schema when `decisions.maxItems` was 20; the same
schema was accepted when that one keyword was removed or reduced to 3. Every
successful response is then validated again against the complete unchanged
Sidecar schema, in addition to SDK validation of the provider projection.
Semantic resolution also projects an exact local `minItems`/`maxItems` count
from the candidate batch before invocation, requiring exactly one decision per
candidate regardless of provider behavior.

Initial bounded evidence:

- SDK calibration with high reasoning and 512 output tokens passed in 11.19
  seconds; usage was 22 input, 18 output, 173 reasoning, and 213 total tokens;
- final combined gate: Acquisition Planning passed in 4.78 seconds;
- final combined gate: Candidate Evaluation passed in 5.65 seconds with the required one
  item and one assessment;
- final combined gate: AI Deep Detection passed in 11.74 seconds across six synthetic positive,
  negative, quoted-context, external-artifact, and short-content controls;
- final combined gate: Semantic Event Resolution passed in 10.87 seconds after the bounded schema
  diagnostic and exact-count projection; its duplicate positive control and
  near-miss negative control both passed.

No raw prompt, output, provider error body, or credential was retained. Gemini
Flash Lite is selectable in Settings only when `gemini.primary` is populated
in `runtime/config/credentials.local.json`. Saving the choice persists it in
Sidecar state; the same Supervisor-managed application activates it after
restart. No Supervisor service parameters or lifecycle responsibilities are
added.

Create the ignored local store from the tracked secret-free example:

```powershell
New-Item -ItemType Directory -Force .\runtime\config | Out-Null
Copy-Item .\config\credentials.example.json .\runtime\config\credentials.local.json
# Edit credentialStore.values in credentials.local.json; never commit that file.
```

## Separate OpenRouter privacy gate

Strict OpenRouter assessment will be repeated separately with both of these
provider preferences on every initial and retried request:

```json
{
  "provider": {
    "zdr": true,
    "data_collection": "deny"
  }
}
```

`store:false` remains required but is not equivalent to either routing filter.
Strict-route failure must remain a failure: calibration, retry, fallback, or
model selection may not remove or weaken the two privacy preferences. Receipts
may state which policy the SDK requested, but must not claim that the SDK
independently audited the upstream provider. Availability under this filter is
unknown until the separate live gate is run.
