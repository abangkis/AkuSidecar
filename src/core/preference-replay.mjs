const POSITIVE_KINDS = new Set(["more_like_this"]);
const NEGATIVE_KINDS = new Set(["less_like_this"]);
const SCORE_FIELDS = ["intentRelevance", "novelty", "urgency", "actionability"];

export const PREFERENCE_REPLAY_THRESHOLDS = Object.freeze({
  feedbackEvents: 30,
  moreLikeThis: 15,
  lessLikeThis: 5,
  assessedFeedback: 20,
  feedbackRuns: 10,
});

export function buildPreferenceReplay(runs) {
  const candidates = runs.flatMap((run) =>
    (run.candidateEvaluations ?? []).map((candidate) => ({ run, candidate })),
  );
  const candidateByRunEvidence = new Map(
    candidates.map(({ run, candidate }) => [key(run.id, candidate.evidenceKey), { run, candidate }]),
  );
  const signals = [];
  const latestByCandidate = new Map();
  for (const run of runs) {
    for (const feedback of run.preferenceFeedback ?? []) {
      latestByCandidate.set(key(run.id, feedback.evidenceKey), { run, feedback });
    }
  }
  for (const { run, feedback } of latestByCandidate.values()) {
      const polarity = POSITIVE_KINDS.has(feedback.kind)
        ? "positive"
        : NEGATIVE_KINDS.has(feedback.kind)
          ? "negative"
          : null;
      if (!polarity) continue;
      const matched = candidateByRunEvidence.get(key(run.id, feedback.evidenceKey));
      signals.push({
        runId: run.id,
        source: run.source,
        evidenceKey: feedback.evidenceKey,
        kind: feedback.kind,
        polarity,
        decision: matched?.candidate.decision ?? null,
        assessment: matched?.candidate.assessment ?? null,
        matchedCandidate: Boolean(matched),
      });
  }

  const positive = signals.filter((signal) => signal.polarity === "positive");
  const negative = signals.filter((signal) => signal.polarity === "negative");
  const assessed = signals.filter((signal) => signal.assessment);
  const feedbackRuns = new Set(signals.map((signal) => signal.runId)).size;
  const gates = [
    gate("feedback_events", signals.length, PREFERENCE_REPLAY_THRESHOLDS.feedbackEvents),
    gate("more_like_this", positive.length, PREFERENCE_REPLAY_THRESHOLDS.moreLikeThis),
    gate("less_like_this", negative.length, PREFERENCE_REPLAY_THRESHOLDS.lessLikeThis),
    gate("assessed_feedback", assessed.length, PREFERENCE_REPLAY_THRESHOLDS.assessedFeedback),
    gate("feedback_runs", feedbackRuns, PREFERENCE_REPLAY_THRESHOLDS.feedbackRuns),
  ];
  const passed = gates.filter((entry) => entry.passed).length;

  return {
    version: 0,
    mode: "offline_replay",
    liveInfluence: false,
    readiness: {
      status: passed === gates.length ? "ready_for_offline_fit" : "collecting",
      passedGates: passed,
      totalGates: gates.length,
      progress: passed / gates.length,
      gates,
    },
    dataset: {
      runs: runs.length,
      feedbackRuns,
      evaluatedCandidates: candidates.length,
      assessedCandidates: candidates.filter(({ candidate }) => candidate.assessment).length,
      feedbackEvents: signals.length,
      matchedFeedback: signals.filter((signal) => signal.matchedCandidate).length,
      assessedFeedback: assessed.length,
      moreLikeThis: positive.length,
      lessLikeThis: negative.length,
      selectedSignals: signals.filter((signal) => signal.decision === "selected").length,
      excludedSignals: signals.filter((signal) => signal.decision === "excluded").length,
      sources: [...new Set(signals.map((signal) => signal.source))].sort(),
    },
    tendencies: {
      contentTypes: aggregateLabels(assessed, (assessment) => [assessment.contentType]),
      topicTags: aggregateLabels(assessed, (assessment) => assessment.topicTags ?? []),
      scoreAverages: Object.fromEntries(
        SCORE_FIELDS.map((field) => [field, {
          positive: averageScore(assessed, field, "positive"),
          negative: averageScore(assessed, field, "negative"),
        }]),
      ),
    },
    limitations: [
      "Replay observations do not alter live selection, ordering, attention limits, or comeback policy.",
      negative.length < PREFERENCE_REPLAY_THRESHOLDS.lessLikeThis
        ? "Contextual negative-interest feedback is still too sparse for balanced fitting."
        : null,
      assessed.length < signals.length
        ? "Some historical feedback predates structured candidate assessment."
        : null,
    ].filter(Boolean),
  };
}

function aggregateLabels(signals, labelsForAssessment) {
  const counts = new Map();
  for (const signal of signals) {
    for (const label of labelsForAssessment(signal.assessment).filter(Boolean)) {
      const current = counts.get(label) ?? { label, positive: 0, negative: 0, total: 0 };
      current[signal.polarity] += 1;
      current.total += 1;
      counts.set(label, current);
    }
  }
  return [...counts.values()]
    .sort((left, right) => right.total - left.total || left.label.localeCompare(right.label))
    .slice(0, 12);
}

function averageScore(signals, field, polarity) {
  const values = signals
    .filter((signal) => signal.polarity === polarity)
    .map((signal) => signal.assessment?.[field])
    .filter(Number.isFinite);
  if (values.length === 0) return null;
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function gate(id, observed, required) {
  return { id, observed, required, passed: observed >= required };
}

function key(runId, evidenceKey) {
  return `${runId}:${evidenceKey}`;
}
