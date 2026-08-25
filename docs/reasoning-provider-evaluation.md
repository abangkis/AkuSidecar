# Development reasoning-provider evaluation

Status: active assessment; no OpenRouter provider is integrated into
AkuSidecar and Codex App Server remains the authoritative development baseline.

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
- Provider credentials are never stored in Sidecar configuration, SQLite,
  receipts, diagnostics, or assessment artifacts.
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
6. Add a Sidecar provider composition and Settings option only after the SDK
   exposes the required model capability and failure/receipt contract.

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
