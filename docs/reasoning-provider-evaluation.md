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

## Current tuning focus

The current phase tunes endpoint behavior and reliability: request acceptance,
structured-output completion, endpoint-specific output budgets, latency, and
typed failures. Provider-type tuning is deliberately deferred. Codex App
Server and Ollama still inherit AkuSidecar's historical 16,384-token fallback
where a workload-specific value is absent; that fallback is not evidence that
their Planning, Evaluation, Semantic Event, or AI Detection budgets are
optimal. A later provider-type pass will derive separate workload budgets from
completed-run telemetry and quality gates for Codex, Ollama, Gemini, Groq, and
any other accepted provider. Do not copy one endpoint's budget into another
without endpoint-specific evidence.

Prompt compatibility changes follow the composable-prompt contract in
[`composable-prompts.md`](composable-prompts.md). The current Gemini
Candidate Evaluation overlay is scoped to that workload only. Its initial
seven-candidate bounded live gate passed the complete local response schema;
the first subsequent scheduler batch also completed with four effective X
assessments inside the schema bounds. Additional normal batches are still
required before treating it as reliable.

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

### Development runtime output-budget result — 2026-08-26

Seven development sessions using the former 4,096-token Candidate Evaluation
budget produced two completed sessions, four partial sessions, and one failed
session. Across their fourteen source runs, eight Candidate Evaluation
invocations completed, five returned the typed non-retryable
`incomplete_response`, and one eight-candidate X request returned HTTP 400
`invalid_request`. Successful invocations remained materially faster than the
recent Codex App Server comparison, but the session completion rate was not
acceptable.

A controlled change raised only Gemini Candidate Evaluation to 8,192 output
tokens. Profile selection was also corrected to retain the workload-owned
output budget and structured-output assurance instead of replacing them with
a provider-wide profile default. Planning remains 512 tokens; Semantic Event
Resolution and AI Deep Detection remain 4,096 tokens.

The first post-change visible update completed both sources and the semantic
pass in 60 seconds. LinkedIn evaluated four captured candidates in 17.58
seconds and reported 1,526 output plus 4,865 reasoning tokens. X evaluated six
of seven captured candidates in 19.19 seconds and reported 2,126 output plus
4,867 reasoning tokens. Their combined output and reasoning totals, 6,391 and
6,993 respectively, exceeded the former 4,096 ceiling while remaining below
8,192. Semantic resolution completed in 9.14 seconds under its unchanged
4,096-token budget. This confirms that the former ceiling caused at least the
observed incomplete responses for these batch sizes; it does not yet explain
or remediate the separate HTTP 400 seen with an eight-candidate X request.

No raw prompt or model output was retained. More completed sessions are needed
before treating the new budget as a reliability baseline.

### Duplicate-admission observation after the evaluation change

The next completed visible update confirmed that Candidate Evaluation remained
healthy at the new 8,192-token budget, but Semantic Event Resolution retained
its separate 4,096-token budget and returned a non-retryable
`incomplete_response`. The semantic stage had five selected candidates, ten
shortlisted historical events, a strongest lexical overlap of fifteen, and was
invoked rather than bypassed. Because semantic resolution is currently a
degraded-availability stage, the engine recorded the semantic failure and then
continued Timeline composition. All five selected candidates were consequently
added as unique reports and the session itself remained `completed`.

This behavior provides a direct path for an observed duplicate to pass through:
the immediate failure is semantic endpoint completion followed by the existing
fail-open composition policy, not proven Gemini misclassification. Earlier
Gemini semantic calls did complete and one detected duplicate report, so model
classification quality must be evaluated separately from resolver availability.
The next endpoint-focused experiment raised only Gemini Semantic Event to
8,192 tokens. Planning remains 512, Candidate Evaluation remains 8,192, and AI
Deep Detection remains 4,096. The first post-change canary completed in 89.5
seconds with ten captured candidates, four evaluated candidates, and three
items admitted. Semantic resolution compared three selected candidates with a
ten-event historical shortlist and completed in 9.44 seconds, reporting 718
output plus 2,352 reasoning tokens. This confirms that the larger budget is
wired through and provides headroom for the previously incomplete workload; a
single success does not yet establish a reliability baseline.

Four additional pre-change semantic invocations also completed under 4,096 in
6.18 to 8.44 seconds and detected three duplicate reports in total. The
semantic failure therefore depends on workload shape rather than occurring on
every invocation. Whether semantic failure should remain fail-open is a
separate product-policy decision and is not changed during endpoint tuning.

Two independent Candidate Evaluation issues remain. X batches with eight
captured candidates twice returned provider HTTP 400 `invalid_request`, while
the post-change canary completed with seven captured and four evaluated X
candidates. One LinkedIn invocation also returned structurally valid JSON that
failed the complete local schema because `topicFacets` contained four entries
where the contract permits at most three. Neither issue is addressed by the
Semantic Event budget change; both require focused reproduction before a fix.

### Gemini candidate-evaluation cardinality boundary — 2026-08-26

A focused endpoint canary isolated the X `invalid_request`: six effective
Candidate Evaluation candidates completed in 12.5 seconds, while seven were
rejected in 1.14 seconds before any model usage was reported. Three production
failures had exactly seven effective candidates. This boundary is independent
of prompt content, output budget, and timeout.

Gemini's Sidecar adapter now evaluates candidates in ordered chunks of at most
six when the effective set exceeds that boundary. The adapter preserves the
original evidence order and rebinds each returned item and assessment to its
source evidence key by position. It merges chunk items and assessments in
order, combines non-duplicate limitations deterministically, joins distinct
chunk summaries, and sums repeated-claim, deferred-budget, token, timing, and
duration telemetry. Any chunk failure remains a failed evaluation and returns
no partial result. Other providers retain their existing full-batch contract.

The post-fix live gate evaluated the same seven-candidate synthetic control
successfully through two bounded chunks in 19.35 seconds. The merged result
contained seven items and seven assessments in source order, with aggregate
usage of 1,061 input, 2,132 output, and 3,914 reasoning tokens. The rebuilt
development runtime remained healthy under Supervisor ownership.

No raw prompt, output, or provider error body is retained by this mitigation.
The separate local-schema issue (`topicFacets` exceeding three entries) remains
an independent provider-conformance problem.

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
