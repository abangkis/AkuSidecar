import { createHash } from "node:crypto";
import { buildPreferenceReplay, buildPreferenceSignals } from "./preference-replay.mjs";

const SCORE_FIELDS = ["novelty", "urgency", "actionability"];
const SNAPSHOT_VERSION = 3;
const PROMOTION_THRESHOLD = 0.6;
const DEMOTION_THRESHOLD = 0.25;

export function preferenceExperimentStatus(runs, latestSnapshot = null) {
  const replay = buildPreferenceReplay(runs);
  const datasetFingerprint = preferenceDatasetFingerprint(runs);
  const ready = replay.readiness.status === "ready_for_offline_fit";
  const currentSnapshot = latestSnapshot?.version === SNAPSHOT_VERSION &&
    latestSnapshot.datasetFingerprint === datasetFingerprint
    ? latestSnapshot
    : null;
  return {
    version: SNAPSHOT_VERSION,
    mode: "offline_preference_experiment",
    status: ready ? (currentSnapshot ? "fitted" : "ready_to_fit") : "blocked",
    liveInfluence: currentSnapshot?.liveInfluence === true,
    datasetFingerprint,
    readiness: replay.readiness,
    dataset: replay.dataset,
    latestSnapshot,
    currentSnapshot,
    limitations: [
      "Preference snapshots never alter candidate eligibility or attention budgets.",
      ready ? null : "Every replay readiness gate must pass before fitting is allowed.",
      latestSnapshot && !currentSnapshot
        ? "The latest persisted snapshot belongs to an older feedback dataset."
        : null,
    ].filter(Boolean),
  };
}

export function fitOfflinePreferenceExperiment(runs, options = {}) {
  const status = preferenceExperimentStatus(runs);
  const runtime = options.runtime === true;
  if (status.status === "blocked" && !runtime) return status;
  const { candidates, signals } = buildPreferenceSignals(runs);
  const assessedSignals = deduplicateSignals(signals.filter((signal) => signal.assessment));
  const positiveSignals = assessedSignals.filter((signal) => signal.polarity === "positive").length;
  const negativeSignals = assessedSignals.filter((signal) => signal.polarity === "negative").length;
  if (runtime && (assessedSignals.length < 4 || positiveSignals < 1 || negativeSignals < 1)) {
    return {
      ...status,
      status: "baseline",
      liveInfluence: false,
      limitations: [
        ...status.limitations,
        "Local fitting requires at least four assessed signals with both preference polarities.",
      ],
    };
  }
  const { training, holdout } = splitByRun(assessedSignals);
  const evaluationModel = trainModel(training.length > 0 ? training : assessedSignals);
  const evaluation = evaluateModel(evaluationModel, holdout);
  const model = trainModel(runtime ? assessedSignals : training);
  const shadow = evaluateShadow(model, deduplicateCandidateEntries(candidates));
  const createdAt = options.createdAt ?? new Date().toISOString();
  const snapshot = {
    id: `${runtime ? "preference-runtime" : "preference-diagnostic"}-v${SNAPSHOT_VERSION}-${status.datasetFingerprint.slice(0, 16)}`,
    version: SNAPSHOT_VERSION,
    policyVersion: runtime ? "preference-runtime-v1" : "preference-diagnostic-v1",
    datasetFingerprint: status.datasetFingerprint,
    createdAt,
    origin: runtime ? "local_runtime" : "manual_diagnostic",
    fitTrigger: options.trigger ?? (runtime ? "automatic" : "manual"),
    liveInfluence: runtime,
    dataset: {
      assessedFeedback: assessedSignals.length,
      positiveSignals,
      negativeSignals,
    },
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
      rankingInfluence: runtime,
      influence: runtime ? "bounded_selected_rerank" : "diagnostic_only",
      maxRankDisplacement: runtime ? 2 : 0,
      activationRequiresDecision: true,
      explorationLane: {
        active: runtime,
        proposedBudgetFraction: 0.1,
        purpose: "Preserve bounded discovery outside learned preference tendencies.",
      },
      comeback: {
        active: false,
        purpose: "Allow a weakened topic to return under materially stronger future evidence.",
      },
      movementThresholds: {
        promoteAtOrAbove: PROMOTION_THRESHOLD,
        demoteAtOrBelow: DEMOTION_THRESHOLD,
      },
      guardrails: {
        duplicateEvidenceCollapsed: true,
      },
    },
  };
  return {
    ...status,
    status: runtime ? "active" : "fitted",
    liveInfluence: runtime,
    snapshot,
    currentSnapshot: snapshot,
    latestSnapshot: snapshot,
  };
}

export function scorePreferenceCandidate(snapshot, candidate) {
  return explainPreferenceCandidate(snapshot, candidate)?.probability ?? null;
}

export function explainPreferenceCandidate(snapshot, candidate) {
  if (!snapshot?.model || !candidate?.assessment) return null;
  const model = snapshot.model;
  const assessment = candidate.assessment;
  const contributions = [{ feature: "intercept", label: "base", value: model.intercept }];
  addCategoricalContribution(contributions, "source", candidate.source, model.categorical.source);
  addCategoricalContribution(
    contributions,
    "content_type",
    assessment.contentType,
    model.categorical.contentType,
  );
  for (const tag of assessment.topicTags ?? []) {
    addCategoricalContribution(contributions, "topic_tag", tag, model.categorical.topicTag);
  }
  for (const field of SCORE_FIELDS) {
    if (Number.isFinite(assessment[field])) {
      contributions.push({
        feature: field,
        label: String(assessment[field]),
        value: (assessment[field] - 0.5) * model.continuous[field].weight,
      });
    }
  }
  const rawScore = contributions.reduce((sum, entry) => sum + entry.value, 0);
  return {
    rawScore,
    probability: sigmoid(rawScore),
    contributions: contributions
      .filter((entry) => Number.isFinite(entry.value) && entry.value !== 0)
      .sort((left, right) => Math.abs(right.value) - Math.abs(left.value)),
  };
}

export function buildShadowComparison(snapshot, runs, options = {}) {
  const requestedLimit = Math.trunc(options.limit ?? 50);
  const requestedOffset = Math.trunc(options.offset ?? 0);
  const limit = Number.isFinite(requestedLimit)
    ? Math.max(1, Math.min(100, requestedLimit))
    : 50;
  const offset = Number.isFinite(requestedOffset) ? Math.max(0, requestedOffset) : 0;
  if (!snapshot?.model) {
    return {
      version: SNAPSHOT_VERSION,
      available: false,
      liveInfluence: false,
      reason: "no_current_snapshot",
      summary: emptyShadowSummary(),
      candidates: [],
      pagination: shadowPagination(0, offset, limit, 0),
    };
  }
  const candidates = [];
  const uniqueCandidates = deduplicateCandidates(runs);
  let insufficientEvidence = 0;
  for (const { run, candidate } of uniqueCandidates) {
      const explanation = explainPreferenceCandidate(snapshot, {
        ...candidate,
        source: run.source,
      });
      if (!explanation) {
        insufficientEvidence += 1;
        continue;
      }
      const promotionEligible = explanation.probability >= PROMOTION_THRESHOLD;
      const demotionEligible = explanation.probability <= DEMOTION_THRESHOLD;
      const movement = candidate.decision === "selected"
        ? demotionEligible ? "would_move_down" : "unchanged"
        : promotionEligible ? "would_move_up" : "unchanged";
      candidates.push({
        runId: run.id,
        evidenceKey: candidate.evidenceKey,
        source: run.source,
        author: candidate.author ?? null,
        text: candidate.text ?? null,
        sourceUrl: candidate.sourceUrl ?? null,
        publishedAt: candidate.publishedAt ?? null,
        originalDecision: candidate.decision,
        movement,
        probability: explanation.probability,
        distanceFromNeutral: Math.abs(explanation.probability - 0.5),
        contentType: candidate.assessment.contentType,
        topicTags: candidate.assessment.topicTags ?? [],
        contributions: explanation.contributions.slice(0, 8),
      });
  }
  candidates.sort((left, right) =>
    movementOrder(left.movement) - movementOrder(right.movement) ||
    right.distanceFromNeutral - left.distanceFromNeutral ||
    `${left.runId}:${left.evidenceKey}`.localeCompare(`${right.runId}:${right.evidenceKey}`),
  );
  const summary = {
    scoredCandidates: candidates.length,
    wouldMoveUp: candidates.filter((entry) => entry.movement === "would_move_up").length,
    wouldMoveDown: candidates.filter((entry) => entry.movement === "would_move_down").length,
    unchanged: candidates.filter((entry) => entry.movement === "unchanged").length,
    insufficientEvidence,
    duplicateCandidatesCollapsed: runs.reduce(
      (sum, run) => sum + (run.candidateEvaluations?.length ?? 0),
      0,
    ) - uniqueCandidates.length,
    sources: Object.fromEntries(["x", "linkedin"].map((source) => {
      const entries = candidates.filter((entry) => entry.source === source);
      return [source, {
        scoredCandidates: entries.length,
        wouldMoveUp: entries.filter((entry) => entry.movement === "would_move_up").length,
        wouldMoveDown: entries.filter((entry) => entry.movement === "would_move_down").length,
      }];
    })),
  };
  return {
    version: SNAPSHOT_VERSION,
    available: true,
    liveInfluence: false,
    snapshotId: snapshot.id,
    datasetFingerprint: snapshot.datasetFingerprint,
    summary,
    candidates: candidates.slice(offset, offset + limit),
    pagination: shadowPagination(candidates.length, offset, limit, candidates.length),
  };
}

export function preferenceDatasetFingerprint(runs) {
  const { signals } = buildPreferenceSignals(runs);
  const stable = deduplicateSignals(signals.filter((signal) => signal.assessment))
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
  return createHash("sha256")
    .update(`preference-model-v${SNAPSHOT_VERSION}:`)
    .update(JSON.stringify(stable))
    .digest("hex");
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
    type: "regularized_additive_preference_v1_1",
    classBalance: "equal_polarity",
    intercept: 0,
    categorical: {
      source: categoricalWeights(signals, (signal) => [signal.source]),
      contentType: categoricalWeights(signals, (signal) => [signal.assessment.contentType]),
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
    const support = count.positive + count.negative;
    if (support < 3) return [label, 0];
    const positiveRate = (count.positive + 1) / (positiveTotal + 2);
    const negativeRate = (count.negative + 1) / (negativeTotal + 2);
    const supportShrinkage = Math.min(1, (support - 2) / 8);
    const polarityShrinkage = count.positive > 0 && count.negative > 0 ? 1 : 0.2;
    return [
      label,
      clamp(
        Math.log(positiveRate / negativeRate) * supportShrinkage * polarityShrinkage,
        -0.5,
        0.5,
      ),
    ];
  }));
}

function deduplicateSignals(signals) {
  const latest = new Map();
  for (const signal of [...signals].sort(compareSignalRecency)) {
    latest.set(`${signal.source}:${signal.evidenceKey}`, signal);
  }
  return [...latest.values()];
}

function compareSignalRecency(left, right) {
  return String(left.createdAt ?? "").localeCompare(String(right.createdAt ?? "")) ||
    String(left.runId).localeCompare(String(right.runId));
}

function deduplicateCandidates(runs) {
  return deduplicateCandidateEntries(runs.flatMap((run) =>
    (run.candidateEvaluations ?? []).map((candidate) => ({ run, candidate })),
  ));
}

function deduplicateCandidateEntries(entries) {
  const latest = new Map();
  const ordered = [...entries].sort((left, right) =>
    String(left.run.createdAt ?? "").localeCompare(String(right.run.createdAt ?? "")) ||
    String(left.run.id).localeCompare(String(right.run.id)),
  );
  for (const entry of ordered) {
    latest.set(`${entry.run.source}:${entry.candidate.evidenceKey}`, entry);
  }
  return [...latest.values()];
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

function addCategoricalContribution(target, feature, label, weights) {
  const value = categoricalWeight(weights, label);
  if (label && value !== 0) target.push({ feature, label, value });
}

function movementOrder(value) {
  return value === "would_move_up" ? 0 : value === "would_move_down" ? 1 : 2;
}

function emptyShadowSummary() {
  return {
    scoredCandidates: 0,
    wouldMoveUp: 0,
    wouldMoveDown: 0,
    unchanged: 0,
    insufficientEvidence: 0,
    duplicateCandidatesCollapsed: 0,
    sources: {},
  };
}

function shadowPagination(total, offset, limit, available) {
  const returned = Math.max(0, Math.min(limit, available - offset));
  return {
    total,
    offset,
    limit,
    returned,
    hasNext: offset + returned < total,
  };
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
