import assert from "node:assert/strict";
import test from "node:test";
import { buildEngineReplayBenchmark } from "../src/core/engine-replay-benchmark.mjs";
import { syntheticPreferenceReadyRuns } from "../test-support/preference-ready-dataset.mjs";

test("engine replay benchmark is local, source-neutral, and model-call free", () => {
  const runs = syntheticPreferenceReadyRuns().map((run, index) => ({
    ...run,
    reasoningInvocations: [{
      phase: "candidate_evaluation",
      provider: "codex-sdk",
      model: "fixture-model",
      reasoningEffort: index % 2 ? "low" : "medium",
      durationMs: 100,
      status: "completed",
      inputTokens: 200,
      outputTokens: 20,
    }],
  }));
  const benchmark = buildEngineReplayBenchmark(runs);
  assert.equal(benchmark.version, 1);
  assert.equal(benchmark.preference.available, true);
  assert.equal(benchmark.preference.sourceFeatureUsed, false);
  assert.equal(benchmark.guardrails.benchmarkPerformsModelCalls, false);
  assert.equal(benchmark.reasoningProfiles.length, 2);
  assert.equal(benchmark.selection.evaluatedCandidates, 30);
});
