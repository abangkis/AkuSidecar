export function syntheticPreferenceReadyRuns() {
  return Array.from({ length: 30 }, (_, index) => syntheticRun(
    index,
    index < 6 ? "less_like_this" : "more_like_this",
  ));
}

function syntheticRun(index, kind) {
  const source = index % 2 === 0 ? "x" : "linkedin";
  const evidenceKey = `${source}:synthetic-${String(index).padStart(2, "0")}`;
  const positive = kind === "more_like_this";
  return {
    id: `synthetic-run-${String(index).padStart(2, "0")}`,
    source,
    candidateEvaluations: [{
      evidenceKey,
      decision: index % 3 === 0 ? "selected" : "excluded",
      assessment: {
        contentType: positive ? "release" : "opinion",
        topicTags: positive ? ["engineering", "ai"] : ["generic"],
        recommendedPriority: positive ? "P1" : "P4",
        intentRelevance: positive ? 0.9 : 0.2,
        novelty: positive ? 0.8 : 0.2,
        urgency: positive ? 0.7 : 0.1,
        actionability: positive ? 0.8 : 0.2,
        rationale: "Synthetic preference calibration fixture.",
      },
    }],
    preferenceFeedback: [{ evidenceKey, kind }],
  };
}
