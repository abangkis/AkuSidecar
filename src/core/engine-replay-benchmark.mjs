import { fitOfflinePreferenceExperiment, scorePreferenceCandidate } from "./offline-preference-experiment.mjs";
import { buildPreferenceSignals } from "./preference-replay.mjs";
import { feedbackLaneForReason } from "./preference-features.mjs";

export function buildEngineReplayBenchmark(runs) {
  const { signals } = buildPreferenceSignals(runs);
  const assessed = signals.filter((signal) => signal.assessment && signal.polarity !== "neutral");
  const experiment = fitOfflinePreferenceExperiment(runs, { runtime: true, trigger: "benchmark" });
  const snapshot = experiment.snapshot ?? null;
  const predictions = snapshot ? assessed.map((signal) => ({
    signal,
    probability: scorePreferenceCandidate(snapshot, {
      source: signal.source,
      assessment: signal.assessment,
      decision: signal.decision,
    }),
  })) : [];
  const confusion = confusionFor(predictions);
  const bySource = Object.fromEntries([...new Set(assessed.map((signal) => signal.source))]
    .sort().map((source) => [source, confusionFor(predictions.filter((entry) => entry.signal.source === source))]));
  const reasoningProfiles = summarizeReasoningProfiles(runs);
  return {
    version: 1,
    generatedAt: new Date().toISOString(),
    dataset: {
      runs: runs.length,
      assessedSignals: assessed.length,
      positive: assessed.filter((signal) => signal.polarity === "positive").length,
      negative: assessed.filter((signal) => signal.polarity === "negative").length,
      neutral: signals.filter((signal) => signal.polarity === "neutral").length,
      sources: [...new Set(assessed.map((signal) => signal.source))].sort(),
    },
    preference: {
      available: Boolean(snapshot),
      policyVersion: snapshot?.policyVersion ?? null,
      snapshotId: snapshot?.id ?? null,
      metrics: confusion,
      bySource,
      sourceFeatureUsed: Boolean(snapshot?.model?.categorical?.source &&
        Object.keys(snapshot.model.categorical.source).length),
      feedbackRoutes: counts(signals.map((signal) =>
        feedbackLaneForReason(signal.kind, signal.reasonCode),
      )),
    },
    selection: selectionMetrics(runs),
    reasoningProfiles,
    guardrails: {
      eligibilityOwnedBySelectionEngine: true,
      preferenceChangesEligibility: false,
      sourceIsPreferenceFeature: false,
      benchmarkPerformsModelCalls: false,
    },
  };
}

function confusionFor(entries) {
  const confusion = { truePositive: 0, trueNegative: 0, falsePositive: 0, falseNegative: 0 };
  for (const { signal, probability } of entries) {
    const predicted = probability >= 0.5;
    const positive = signal.polarity === "positive";
    if (predicted && positive) confusion.truePositive += 1;
    else if (!predicted && !positive) confusion.trueNegative += 1;
    else if (predicted) confusion.falsePositive += 1;
    else confusion.falseNegative += 1;
  }
  const positiveRecall = ratio(confusion.truePositive, confusion.truePositive + confusion.falseNegative);
  const negativeRecall = ratio(confusion.trueNegative, confusion.trueNegative + confusion.falsePositive);
  return {
    signals: entries.length,
    agreement: ratio(confusion.truePositive + confusion.trueNegative, entries.length),
    balancedAccuracy: positiveRecall === null || negativeRecall === null ? null : (positiveRecall + negativeRecall) / 2,
    positiveRecall,
    negativeRecall,
    confusion,
  };
}

function selectionMetrics(runs) {
  const candidates = runs.flatMap((run) => run.candidateEvaluations ?? []);
  const selected = candidates.filter((entry) => entry.decision === "selected");
  return {
    evaluatedCandidates: candidates.length,
    selectedCandidates: selected.length,
    selectionRate: ratio(selected.length, candidates.length),
    policyVersions: [...new Set(candidates.map((entry) => entry.policyVersion).filter(Boolean))].sort(),
    reasonCodes: counts(candidates.map((entry) => entry.reasonCode)),
  };
}

function summarizeReasoningProfiles(runs) {
  const profiles = new Map();
  for (const invocation of runs.flatMap((run) => run.reasoningInvocations ?? [])) {
    const key = [invocation.phase, invocation.provider, invocation.model ?? "default", invocation.reasoningEffort ?? "default"].join("|");
    const current = profiles.get(key) ?? {
      phase: invocation.phase,
      provider: invocation.provider,
      model: invocation.model ?? null,
      reasoningEffort: invocation.reasoningEffort ?? null,
      invocations: 0,
      completed: 0,
      failed: 0,
      durationMs: 0,
      inputTokens: 0,
      outputTokens: 0,
    };
    current.invocations += 1;
    current[invocation.status === "completed" ? "completed" : "failed"] += 1;
    current.durationMs += invocation.durationMs ?? 0;
    current.inputTokens += invocation.inputTokens ?? 0;
    current.outputTokens += invocation.outputTokens ?? 0;
    profiles.set(key, current);
  }
  return [...profiles.values()].map((entry) => ({
    ...entry,
    averageDurationMs: ratio(entry.durationMs, entry.invocations),
    averageInputTokens: ratio(entry.inputTokens, entry.invocations),
    averageOutputTokens: ratio(entry.outputTokens, entry.invocations),
  })).sort((left, right) => `${left.phase}:${left.model}`.localeCompare(`${right.phase}:${right.model}`));
}

function counts(values) {
  const result = {};
  for (const value of values) result[value ?? "unknown"] = (result[value ?? "unknown"] ?? 0) + 1;
  return result;
}

function ratio(numerator, denominator) {
  return denominator > 0 ? numerator / denominator : null;
}
