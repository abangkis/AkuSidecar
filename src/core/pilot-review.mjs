const REVIEW_VERDICTS = new Set([
  "all",
  "unreviewed",
  "correct",
  "correct_empty",
  "correct_lane",
  "missed",
  "useful",
  "wrong_lane",
  "duplicate",
  "failed",
]);
const EMPTY_VERDICTS = new Set(["correct_empty", "missed"]);
const ITEM_VERDICTS = new Set(["correct_lane", "wrong_lane", "duplicate", "useful"]);

export function buildPilotReview(runs, options = {}) {
  const source = options.source ?? "all";
  const verdict = options.verdict ?? "all";
  if (!REVIEW_VERDICTS.has(verdict)) throw new TypeError("unsupported pilot review verdict");
  const limit = Math.max(1, Math.min(100, Number.isFinite(options.limit) ? options.limit : 50));
  const sourceRuns = source === "all" ? runs : runs.filter((run) => run.source === source);
  const filtered = sourceRuns.filter((run) => matchesVerdict(run, verdict));
  return {
    summary: summarizePilotRuns(sourceRuns),
    filters: { source, verdict },
    window: options.window ?? null,
    totalMatching: filtered.length,
    runs: filtered.slice(0, limit).map(toReviewRun),
  };
}

export function summarizePilotRuns(runs) {
  const completed = runs.filter((run) => run.status === "completed");
  const failed = runs.filter((run) => run.status === "failed");
  const reviewable = completed.filter(isReviewableRun);
  const emptyRuns = reviewable.filter((run) => (run.result?.items?.length ?? 0) === 0);
  const feedback = completed.flatMap((run) =>
    validFeedback(run).map((entry) => ({ ...entry, runId: run.id })),
  );
  const runLevelVerdicts = feedback.filter(
    (entry) => !entry.itemId && ["correct_empty", "missed"].includes(entry.kind),
  );
  const correctlyEmpty = new Set(
    runLevelVerdicts.filter((entry) => entry.kind === "correct_empty").map((entry) => entry.runId),
  );
  const missed = new Set(
    runLevelVerdicts.filter((entry) => entry.kind === "missed").map((entry) => entry.runId),
  );
  const reviewedItems = new Set(
    feedback.filter((entry) => entry.itemId).map((entry) => `${entry.runId}:${entry.itemId}`),
  );
  const positiveItems = new Set(
    feedback
      .filter((entry) => entry.itemId && ["useful", "correct_lane"].includes(entry.kind))
      .map((entry) => `${entry.runId}:${entry.itemId}`),
  );
  const completedDurations = completed
    .map(durationMs)
    .filter((value) => Number.isFinite(value) && value >= 0);
  const reviewedRuns = reviewable.filter((run) => isReviewed(run)).length;
  const emptyVerdictCount = correctlyEmpty.size + missed.size;

  return {
    totalRuns: runs.length,
    completedRuns: completed.length,
    failedRuns: failed.length,
    reviewableRuns: reviewable.length,
    reviewedRuns,
    reviewCoverage: ratio(reviewedRuns, reviewable.length),
    emptyRuns: emptyRuns.length,
    correctlyEmptyRuns: correctlyEmpty.size,
    missedRuns: missed.size,
    emptyTrustRate: ratio(correctlyEmpty.size, emptyVerdictCount),
    promotedItems: completed.reduce((sum, run) => sum + (run.result?.items?.length ?? 0), 0),
    reviewedItems: reviewedItems.size,
    positiveReviewedItems: positiveItems.size,
    positiveItemRate: ratio(positiveItems.size, reviewedItems.size),
    wrongLaneFeedback: feedback.filter((entry) => entry.kind === "wrong_lane").length,
    duplicateFeedback: feedback.filter((entry) => entry.kind === "duplicate").length,
    missRate: ratio(missed.size, emptyVerdictCount),
    duplicateEscapeRate: ratio(
      new Set(
        feedback
          .filter((entry) => entry.itemId && entry.kind === "duplicate")
          .map((entry) => `${entry.runId}:${entry.itemId}`),
      ).size,
      reviewedItems.size,
    ),
    medianDurationMs: median(completedDurations),
    averageAcquisitionRounds: average(
      completed.map((run) => run.coverage?.acquisitionRounds).filter(Number.isFinite),
    ),
    evidenceSuppressed: completed.reduce(
      (sum, run) => sum + (run.coverage?.exactDuplicatesSuppressed ?? 0),
      0,
    ),
    deliveredEvidenceSuppressed: completed.reduce(
      (sum, run) => sum + (run.coverage?.deliveredEvidenceSuppressed ?? 0),
      0,
    ),
    confirmedExcludedSuppressed: completed.reduce(
      (sum, run) => sum + (run.coverage?.confirmedExcludedSuppressed ?? 0),
      0,
    ),
    restorationFailures: completed.filter(
      (run) => run.coverage?.restoreAttempted && run.coverage?.restored === false,
    ).length,
    followUpRate: ratio(
      completed.filter((run) => run.coverage?.providerFollowUpExecuted).length,
      completed.length,
    ),
  };
}

function matchesVerdict(run, verdict) {
  if (verdict === "all") return true;
  if (verdict === "failed") return run.status === "failed";
  if (verdict === "unreviewed") return isReviewableRun(run) && !isReviewed(run);
  const feedback = validFeedback(run);
  if (verdict === "correct") {
    return feedback.some((entry) =>
      ["correct_empty", "correct_lane", "useful"].includes(entry.kind),
    );
  }
  return feedback.some((entry) => entry.kind === verdict);
}

function isReviewed(run) {
  if (!isReviewableRun(run)) return false;
  return validFeedback(run).length > 0;
}

function isReviewableRun(run) {
  if (run.status !== "completed") return false;
  if ((run.result?.items?.length ?? 0) > 0) return true;
  return run.coverage?.status !== "unavailable" && run.coverage?.observedBlockCount !== 0;
}

function validFeedback(run) {
  if (run.status !== "completed") return [];
  const items = run.result?.items ?? [];
  const itemIds = new Set(items.map((item) => item.id));
  return (run.feedback ?? []).filter((entry) => {
    if (EMPTY_VERDICTS.has(entry.kind)) {
      if (entry.itemId || items.length !== 0 || !isReviewableRun(run)) return false;
      return entry.kind !== "missed" || Boolean(entry.note?.trim());
    }
    return ITEM_VERDICTS.has(entry.kind) && Boolean(entry.itemId) && itemIds.has(entry.itemId);
  });
}

function toReviewRun(run) {
  return {
    id: run.id,
    mode: run.mode,
    source: run.source,
    intent: run.intent,
    status: run.status,
    provider: run.provider,
    createdAt: run.createdAt,
    startedAt: run.startedAt,
    completedAt: run.completedAt,
    durationMs: durationMs(run),
    coverage: run.coverage,
    result: run.result,
    error: run.error,
    feedback: validFeedback(run),
    reviewed: isReviewed(run),
  };
}

function durationMs(run) {
  if (!run.startedAt || !run.completedAt) return null;
  const value = new Date(run.completedAt).valueOf() - new Date(run.startedAt).valueOf();
  return Number.isFinite(value) ? value : null;
}

function ratio(numerator, denominator) {
  return denominator > 0 ? numerator / denominator : null;
}

function average(values) {
  return values.length > 0
    ? values.reduce((sum, value) => sum + value, 0) / values.length
    : null;
}

function median(values) {
  if (values.length === 0) return null;
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 0
    ? (sorted[middle - 1] + sorted[middle]) / 2
    : sorted[middle];
}
