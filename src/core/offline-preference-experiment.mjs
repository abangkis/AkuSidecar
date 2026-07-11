import { createHash } from "node:crypto";
import { buildPreferenceReplay, buildPreferenceSignals } from "./preference-replay.mjs";

const SCORE_FIELDS = ["intentRelevance", "novelty", "urgency", "actionability"];
const SNAPSHOT_VERSION = 1;

export function preferenceExperimentStatus(runs, latestSnapshot = null) {
  const replay = buildPreferenceReplay(runs);
  const datasetFingerprint = preferenceDatasetFingerprint(runs);
  const ready = replay.readiness.status === "ready_for_offline_fit";
  const currentSnapshot = latestSnapshot?.datasetFingerprint === datasetFingerprint
    ? latestSnapshot
    : null;
  return {
    version: SNAPSHOT_VERSION,
    mode: "offline_preference_experiment",
    status: ready ? (currentSnapshot ? "fitted" : "ready_to_fit") : "blocked",
    liveInfluence: false,
    datasetFingerprint,
    readiness: replay.readiness,
    dataset: replay.dataset,
    latestSnapshot,
    currentSnapshot,
    limitations: [
      "No experiment output can alter live candidate eligibility, ranking, or attention budgets.",
      ready ? null : "Every replay readiness gate must pass before fitting is allowed.",
      latestSnapshot && !currentSnapshot
        ? "The latest persisted snapshot belongs to an older feedback dataset."
        : null,
    ].filter(Boolean),
  };
}

export function fitOfflinePreferenceExperiment(runs, options = {}) {
  const status = preferenceExperimentStatus(runs);
  if (status.status === "blocked") return status;
  const { candidates, signals } = buildPreferenceSignals(runs);
  const assessedSignals = signals.filter((signal) => signal.assessment);
  const { training, holdout } = splitByRun(assessedSignals);
  const model = trainModel(training);
  const evaluation = evaluateModel(model, holdout);
  const shadow = evaluateShadow(model, candidates);
  const createdAt = options.createdAt ?? new Date().toISOString();
  const snapshot = {
    id: `preference-v${SNAPSHOT_VERSION}-${status.datasetFingerprint.slice(0, 16)}`,
    version: SNAPSHOT_VERSION,
    policyVersion: "offline-preference-v1",
    datasetFingerprint: status.datasetFingerprint,
    createdAt,
    liveInfluence: false,
    split: {
      strategy: "stable_run_holdout_20_percent",
      trainingSignals: training.length,
      holdoutSignals: holdout.length,
      trainingRuns: new Set(training.map((signal) => signal.runId)).size,
      holdoutRuns: new Set(holdout.map((signal) => signal.runId)).size,
    },
    model,
    evaluation,
    shadow,
    proposedPolicy: {
      rankingInfluence: false,
      activationRequiresDecision: true,
      explorationLane: {
        active: false,
        proposedBudgetFraction: 0.1,
        purpose: "Preserve bounded discovery outside learned preference tendencies.",
      },
      comeback: {
        active: false,
        purpose: "Allow a weakened topic to return under materially stronger future evidence.",
      },
    },
  };
  return {
    ...status,
    status: "fitted",
    snapshot,
    currentSnapshot: snapshot,
    latestSnapshot: snapshot,
  };
}

export function scorePreferenceCandidate(snapshot, candidate) {
  if (!snapshot?.model || !candidate?.assessment) return null;
  const model = snapshot.model;
  const assessment = candidate.assessment;
  let score = model.intercept;
  score += categoricalWeight(model.categorical.source, candidate.source);
  score += categoricalWeight(model.categorical.decision, candidate.decision);
  score += categoricalWeight(model.categorical.contentType, assessment.contentType);
  score += categoricalWeight(model.categorical.priority, assessment.recommendedPriority);
  for (const tag of assessment.topicTags ?? []) {
    score += categoricalWeight(model.categorical.topicTag, tag);
  }
  for (const field of SCORE_FIELDS) {
    if (Number.isFinite(assessment[field])) {
      score += (assessment[field] - 0.5) * model.continuous[field].weight;
    }
  }
  return sigmoid(score);
}

export function preferenceDatasetFingerprint(runs) {
  const { signals } = buildPreferenceSignals(runs);
  const stable = signals
    .filter((signal) => signal.assessment)
    .map((signal) => ({
      runId: signal.runId,
      source: signal.source,
      evidenceKey: signal.evidenceKey,
      kind: signal.kind,
      decision: signal.decision,
      assessment: signal.assessment,
    }))
    .sort((left, right) =>
      `${left.runId}:${left.evidenceKey}`.localeCompare(`${right.runId}:${right.evidenceKey}`),
    );
  return createHash("sha256").update(JSON.stringify(stable)).digest("hex");
}

function splitByRun(signals) {
  const runIds = [...new Set(signals.map((signal) => signal.runId))]
    .sort((left, right) => stableNumber(left) - stableNumber(right) || left.localeCompare(right));
  const holdoutRunCount = Math.max(1, Math.round(runIds.length * 0.2));
  const holdoutRuns = new Set(runIds.slice(0, holdoutRunCount));
  return {
    training: signals.filter((signal) => !holdoutRuns.has(signal.runId)),
    holdout: signals.filter((signal) => holdoutRuns.has(signal.runId)),
  };
}

function trainModel(signals) {
  const positive = signals.filter((signal) => signal.polarity === "positive");
  const negative = signals.filter((signal) => signal.polarity === "negative");
  return {
    type: "smoothed_additive_preference_v1",
    classBalance: "equal_polarity",
    intercept: 0,
    categorical: {
      source: categoricalWeights(signals, (signal) => [signal.source]),
      decision: categoricalWeights(signals, (signal) => [signal.decision]),
      contentType: categoricalWeights(signals, (signal) => [signal.assessment.contentType]),
      priority: categoricalWeights(signals, (signal) => [signal.assessment.recommendedPriority]),
      topicTag: categoricalWeights(signals, (signal) => signal.assessment.topicTags ?? []),
    },
    continuous: Object.fromEntries(SCORE_FIELDS.map((field) => {
      const positiveMean = average(positive.map((signal) => signal.assessment[field]));
      const negativeMean = average(negative.map((signal) => signal.assessment[field]));
      return [field, {
        positiveMean,
        negativeMean,
        weight: clamp(((positiveMean ?? 0.5) - (negativeMean ?? 0.5)) * 2, -2, 2),
      }];
    })),
  };
}

function categoricalWeights(signals, labelsForSignal) {
  const positiveTotal = signals.filter((signal) => signal.polarity === "positive").length;
  const negativeTotal = signals.filter((signal) => signal.polarity === "negative").length;
  const counts = new Map();
  for (const signal of signals) {
    for (const label of labelsForSignal(signal).filter(Boolean)) {
      const value = counts.get(label) ?? { positive: 0, negative: 0 };
      value[signal.polarity] += 1;
      counts.set(label, value);
    }
  }
  return Object.fromEntries([...counts.entries()].map(([label, count]) => {
    const positiveRate = (count.positive + 1) / (positiveTotal + 2);
    const negativeRate = (count.negative + 1) / (negativeTotal + 2);
    return [label, clamp(Math.log(positiveRate / negativeRate), -2, 2)];
  }));
}

function evaluateModel(model, signals) {
  const confusion = { truePositive: 0, trueNegative: 0, falsePositive: 0, falseNegative: 0 };
  for (const signal of signals) {
    const probability = scorePreferenceCandidate({ model }, {
      source: signal.source,
      decision: signal.decision,
      assessment: signal.assessment,
    });
    const predictedPositive = probability >= 0.5;
    const actualPositive = signal.polarity === "positive";
    if (predictedPositive && actualPositive) confusion.truePositive += 1;
    else if (!predictedPositive && !actualPositive) confusion.trueNegative += 1;
    else if (predictedPositive) confusion.falsePositive += 1;
    else confusion.falseNegative += 1;
  }
  const total = signals.length;
  const positiveTotal = confusion.truePositive + confusion.falseNegative;
  const negativeTotal = confusion.trueNegative + confusion.falsePositive;
  const positiveRecall = ratio(confusion.truePositive, positiveTotal);
  const negativeRecall = ratio(confusion.trueNegative, negativeTotal);
  return {
    holdoutSignals: total,
    agreement: ratio(confusion.truePositive + confusion.trueNegative, total),
    balancedAccuracy:
      positiveRecall === null || negativeRecall === null
        ? null
        : (positiveRecall + negativeRecall) / 2,
    positiveRecall,
    negativeRecall,
    confusion,
    sufficientForActivationDecision: false,
  };
}

function evaluateShadow(model, candidates) {
  const scored = candidates
    .filter(({ candidate }) => candidate.assessment)
    .map(({ run, candidate }) => ({
      decision: candidate.decision,
      source: run.source,
      probability: scorePreferenceCandidate({ model }, { ...candidate, source: run.source }),
    }));
  return {
    scoredCandidates: scored.length,
    selectedMean: average(scored.filter((entry) => entry.decision === "selected").map((entry) => entry.probability)),
    excludedMean: average(scored.filter((entry) => entry.decision === "excluded").map((entry) => entry.probability)),
    preferredSelected: scored.filter(
      (entry) => entry.decision === "selected" && entry.probability >= 0.5,
    ).length,
    preferredExcluded: scored.filter(
      (entry) => entry.decision === "excluded" && entry.probability >= 0.5,
    ).length,
    sources: [...new Set(scored.map((entry) => entry.source))].sort(),
    appliedToLiveRanking: false,
  };
}

function categoricalWeight(weights, label) {
  return label && Number.isFinite(weights?.[label]) ? weights[label] : 0;
}

function average(values) {
  const finite = values.filter(Number.isFinite);
  return finite.length ? finite.reduce((sum, value) => sum + value, 0) / finite.length : null;
}

function ratio(numerator, denominator) {
  return denominator > 0 ? numerator / denominator : null;
}

function sigmoid(value) {
  return 1 / (1 + Math.exp(-clamp(value, -20, 20)));
}

function clamp(value, minimum, maximum) {
  return Math.min(maximum, Math.max(minimum, value));
}

function stableNumber(value) {
  let hash = 2166136261;
  for (const character of String(value)) {
    hash ^= character.charCodeAt(0);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}
