import { explainPreferenceCandidate } from "./offline-preference-experiment.mjs";

export const PREFERENCE_ELIGIBILITY_POLICY = Object.freeze({
  version: "preference-eligibility-v2",
  mode: "promote_unused_budget",
  promotionThreshold: 0.75,
  suppressionThreshold: 0.2,
  minimumPromotionSelectionScore: 0.25,
  minimumPromotionEvidenceStrength: 0.5,
  maxPromotionProposalsPerRun: 1,
  maxSuppressionProposalsPerRun: 1,
  minimumPositiveSignals: 8,
  minimumNegativeSignals: 8,
  minimumSourceCoverage: 2,
  minimumBalancedAccuracy: 0.65,
  minimumNegativeRecall: 0.75,
});

export function evaluatePreferenceEligibilityRun({
  run,
  candidates,
  runtime,
  policy = {},
  datasetRuns = [],
}) {
  const effective = { ...PREFERENCE_ELIGIBILITY_POLICY, ...policy };
  const snapshot = runtime?.activeSnapshot ?? runtime?.currentSnapshot ?? null;
  const readiness = eligibilityReadiness(runtime, effective, datasetRuns);
  const selectedCount = candidates.filter((candidate) => candidate.decision === "selected").length;
  const availableBudget = Math.max(0, Number(run?.maxItems ?? 0) - selectedCount);
  const decisions = candidates.map((candidate) => evaluateCandidate({
    candidate,
    run,
    snapshot,
    selectedCount,
    availableBudget,
    readiness,
    policy: effective,
  }));
  boundProposals(decisions, effective);
  applyAuthority(decisions, readiness, effective);
  const liveMutation = decisions.some((decision) => decision.eligibilityChanged);
  return {
    version: 2,
    policyVersion: effective.version,
    authority: effective.mode,
    liveMutation,
    snapshotId: snapshot?.id ?? null,
    datasetFingerprint: runtime?.datasetFingerprint ?? snapshot?.datasetFingerprint ?? null,
    readiness,
    summary: summarizeDecisions(decisions),
    decisions,
  };
}

export function buildPreferenceEligibilityReport(runs, runtime, options = {}) {
  const limit = boundedInteger(options.limit, 20, 1, 100);
  const offset = boundedInteger(options.offset, 0, 0, 10_000);
  const policy = { ...PREFERENCE_ELIGIBILITY_POLICY, ...(options.policy ?? {}) };
  const latest = deduplicateCandidates(runs);
  const grouped = new Map();
  for (const entry of latest) {
    const values = grouped.get(entry.run.id) ?? { run: entry.run, candidates: [] };
    values.candidates.push(entry.candidate);
    grouped.set(entry.run.id, values);
  }
  const decisions = [...grouped.values()].flatMap(({ run, candidates }) => {
    const evaluation = evaluatePreferenceEligibilityRun({
      run,
      candidates,
      runtime,
      policy,
      datasetRuns: runs,
    });
    return evaluation.decisions.map((decision) => {
      const candidate = candidates.find((entry) => entry.evidenceKey === decision.evidenceKey);
      return {
        ...decision,
        runId: run.id,
        source: run.source,
        author: candidate?.author ?? null,
        text: candidate?.text ?? null,
        sourceUrl: candidate?.sourceUrl ?? null,
        publishedAt: candidate?.publishedAt ?? null,
        contentType: candidate?.assessment?.contentType ?? "other",
        topicFacets: candidate?.assessment?.topicFacets ?? [],
        preferenceFeedback: (run.preferenceFeedback ?? []).filter(
          (feedback) => feedback.evidenceKey === decision.evidenceKey,
        ),
      };
    });
  });
  decisions.sort(compareReportDecision);
  const readiness = eligibilityReadiness(runtime, policy, runs);
  const actionable = decisions.filter((decision) => decision.proposal !== "retain");
  return {
    version: 2,
    mode: "preference_eligibility_live",
    policyVersion: policy.version,
    authority: {
      mode: policy.mode,
      liveMutation: policy.mode !== "rank_only",
      allowedMutations: policy.mode === "guarded_live"
        ? ["promote_unused_budget", "suppress_guarded"]
        : policy.mode === "promote_unused_budget" ? ["promote_unused_budget"] : [],
      explanation:
        policy.mode === "guarded_live"
          ? "The controller may add one bounded unused-budget item or suppress one guarded item when its evidence gates pass."
          : policy.mode === "promote_unused_budget"
            ? "The controller may add one bounded item only when unused attention capacity and promotion evidence are available."
            : "Selection Engine eligibility is preserved; personalized eligibility authority is disabled.",
    },
    snapshotId: runtime?.activeSnapshot?.id ?? runtime?.currentSnapshot?.id ?? null,
    readiness,
    summary: {
      ...summarizeDecisions(decisions),
      uniqueCandidates: decisions.length,
      actionableCandidates: actionable.length,
      repeatAppearancesCollapsed: runs.reduce(
        (sum, run) => sum + (run.candidateEvaluations?.length ?? 0),
        0,
      ) - decisions.length,
    },
    candidates: actionable.slice(offset, offset + limit),
    pagination: {
      total: actionable.length,
      offset,
      limit,
      returned: Math.max(0, Math.min(limit, actionable.length - offset)),
      hasNext: offset + limit < actionable.length,
    },
  };
}

export function parseStoredEligibilityDecision(value) {
  try {
    const parsed = typeof value === "string" ? JSON.parse(value) : value;
    return parsed?.policyVersion === PREFERENCE_ELIGIBILITY_POLICY.version ? parsed : null;
  } catch {
    return null;
  }
}

function evaluateCandidate({
  candidate,
  run,
  snapshot,
  selectedCount,
  availableBudget,
  readiness,
  policy,
}) {
  const explanation = explainPreferenceCandidate(snapshot, {
    ...candidate,
    source: run?.source ?? candidate.source,
  });
  const probability = explanation?.probability ?? null;
  const baselineDecision = candidate.decision === "selected" ? "selected" : "excluded";
  const protectedMandatory = candidate.reasonCode === "selected_mandatory_signal";
  const protectedReliableFloor = baselineDecision === "selected" && selectedCount <= 1;
  const evidenceStrength = candidate.assessment?.evidenceStrength ?? 0;
  const genericFloorPassed =
    Number(candidate.selectionScore) >= policy.minimumPromotionSelectionScore &&
    Number(evidenceStrength) >= policy.minimumPromotionEvidenceStrength;
  let proposal = "retain";
  let reasonCode = explanation ? "preference_inside_neutral_band" : "preference_snapshot_unavailable";
  let budgetEffect = "none";

  if (
    baselineDecision === "excluded" &&
    Number.isFinite(probability) &&
    probability >= policy.promotionThreshold
  ) {
    if (!genericFloorPassed) {
      reasonCode = "promotion_blocked_by_generic_floor";
    } else {
      proposal = "promote";
      reasonCode = readiness.promotionReady
        ? "promotion_candidate_ready"
        : "promotion_candidate_gate_pending";
      budgetEffect = availableBudget > 0 ? "unused_budget" : "would_require_swap";
    }
  } else if (
    baselineDecision === "selected" &&
    Number.isFinite(probability) &&
    probability <= policy.suppressionThreshold
  ) {
    if (protectedMandatory) {
      reasonCode = "suppression_blocked_mandatory_signal";
    } else if (protectedReliableFloor) {
      reasonCode = "suppression_blocked_reliable_floor";
    } else {
      proposal = "suppress";
      reasonCode = readiness.suppressionReady
        ? "suppression_candidate_ready"
        : "suppression_candidate_gate_pending";
      budgetEffect = "reduce_visible_set";
    }
  }

  return {
    runId: run?.id ?? null,
    evidenceKey: candidate.evidenceKey,
    policyVersion: policy.version,
    snapshotId: snapshot?.id ?? null,
    authority: policy.mode,
    baselineDecision,
    proposal,
    proposalRank: null,
    boundedProposal: false,
    finalDecision: baselineDecision,
    eligibilityChanged: false,
    reasonCode,
    probability,
    preferenceConfidence: Number.isFinite(probability)
      ? Math.abs(probability - 0.5) * 2
      : null,
    protectedMandatory,
    protectedReliableFloor,
    genericFloorPassed,
    budgetEffect,
    readiness: {
      promotionReady: readiness.promotionReady,
      suppressionReady: readiness.suppressionReady,
    },
  };
}

function applyAuthority(decisions, readiness, policy) {
  for (const decision of decisions) {
    if (decision.proposal === "promote") {
      if (policy.mode === "rank_only") {
        decision.reasonCode = "promotion_authority_disabled";
      } else if (decision.budgetEffect !== "unused_budget") {
        decision.reasonCode = "promotion_requires_swap_disabled";
      } else if (!decision.boundedProposal) {
        decision.reasonCode = "promotion_outside_bounded_limit";
      } else if (!readiness.promotionReady) {
        decision.reasonCode = "promotion_evidence_pending";
      } else {
        decision.finalDecision = "selected";
        decision.eligibilityChanged = true;
        decision.reasonCode = "live_promotion_unused_budget";
      }
      continue;
    }
    if (decision.proposal !== "suppress") continue;
    if (policy.mode !== "guarded_live") {
      decision.proposal = "retain";
      decision.reasonCode = "suppression_authority_disabled";
    } else if (!decision.boundedProposal) {
      decision.reasonCode = "suppression_outside_bounded_limit";
    } else if (!readiness.suppressionReady) {
      decision.reasonCode = "suppression_evidence_pending";
    } else {
      decision.finalDecision = "excluded";
      decision.eligibilityChanged = true;
      decision.reasonCode = "live_suppression_guarded";
    }
  }
}

export function applyPreferenceEligibilityResult(baselineResult, evaluatedItems, evaluation) {
  if (!evaluation?.liveMutation) return baselineResult;
  const finalDecisionByEvidence = new Map(
    evaluation.decisions.map((decision) => [decision.evidenceKey, decision.finalDecision]),
  );
  const items = (evaluatedItems ?? []).filter(
    (item) => finalDecisionByEvidence.get(item.evidenceKey) === "selected",
  );
  return {
    ...baselineResult,
    items,
    selection: {
      ...(baselineResult.selection ?? {}),
      baselineSelectedCount: baselineResult.items?.length ?? 0,
      selectedCount: items.length,
      preferenceEligibilityPolicyVersion: evaluation.policyVersion,
      preferenceEligibilityAuthority: evaluation.authority,
      preferenceEligibilityChanged: true,
    },
  };
}

function boundProposals(decisions, policy) {
  const promotions = decisions
    .filter((decision) => decision.proposal === "promote")
    .sort((left, right) => right.probability - left.probability || left.evidenceKey.localeCompare(right.evidenceKey));
  const suppressions = decisions
    .filter((decision) => decision.proposal === "suppress")
    .sort((left, right) => left.probability - right.probability || left.evidenceKey.localeCompare(right.evidenceKey));
  for (const [index, decision] of promotions.entries()) {
    decision.proposalRank = index + 1;
    decision.boundedProposal = index < policy.maxPromotionProposalsPerRun;
  }
  for (const [index, decision] of suppressions.entries()) {
    decision.proposalRank = index + 1;
    decision.boundedProposal = index < policy.maxSuppressionProposalsPerRun;
  }
}

function eligibilityReadiness(runtime, policy, runs = []) {
  const snapshot = runtime?.activeSnapshot ?? runtime?.currentSnapshot ?? null;
  const positive = Number(runtime?.signalCounts?.positive ?? 0);
  const negative = Number(runtime?.signalCounts?.negative ?? 0);
  const sources = new Set(runs
    .filter((run) => (run.candidateEvaluations ?? []).some((candidate) => candidate.assessment))
    .map((run) => run.source));
  const sourceCount = runs.length > 0 ? sources.size : policy.minimumSourceCoverage;
  const balancedAccuracy = snapshot?.evaluation?.balancedAccuracy ?? null;
  const negativeRecall = snapshot?.evaluation?.negativeRecall ?? null;
  const gates = [
    gate("preference_runtime_active", runtime?.liveInfluence === true, true),
    gate("snapshot_available", Boolean(snapshot?.model), true),
    gate("positive_support", positive, policy.minimumPositiveSignals),
    gate("negative_support", negative, policy.minimumNegativeSignals),
    gate("source_coverage", sourceCount, policy.minimumSourceCoverage),
    gate("balanced_accuracy", balancedAccuracy, policy.minimumBalancedAccuracy),
    gate("negative_recall", negativeRecall, policy.minimumNegativeRecall),
  ];
  const passed = (id) => gates.find((entry) => entry.id === id)?.passed === true;
  const promotionReady = [
    "preference_runtime_active",
    "snapshot_available",
    "positive_support",
    "source_coverage",
  ].every(passed);
  const suppressionReady = [
    "preference_runtime_active",
    "snapshot_available",
    "negative_support",
    "source_coverage",
    "balanced_accuracy",
    "negative_recall",
  ].every(passed);
  return {
    promotionReady,
    suppressionReady,
    liveAuthorityReady: policy.mode === "guarded_live"
      ? promotionReady && suppressionReady
      : policy.mode === "promote_unused_budget" ? promotionReady : false,
    liveAuthorityBlock: policy.mode === "rank_only"
      ? "Personalized eligibility authority is disabled by configuration."
      : policy.mode === "guarded_live" && !(promotionReady && suppressionReady)
        ? "Guarded live authority is waiting for promotion and suppression evidence gates."
        : policy.mode === "promote_unused_budget" && !promotionReady
          ? "Unused-budget promotion is waiting for positive evidence gates."
          : null,
    gates,
  };
}

function gate(id, observed, required) {
  const passed = typeof required === "boolean"
    ? observed === required
    : Number.isFinite(observed) && observed >= required;
  return { id, observed, required, passed };
}

function summarizeDecisions(decisions) {
  const changed = decisions.filter((decision) => decision.eligibilityChanged);
  return {
    candidateCount: decisions.length,
    promotionProposals: decisions.filter((decision) => decision.proposal === "promote").length,
    suppressionProposals: decisions.filter((decision) => decision.proposal === "suppress").length,
    boundedPromotionProposals: decisions.filter(
      (decision) => decision.proposal === "promote" && decision.boundedProposal,
    ).length,
    boundedSuppressionProposals: decisions.filter(
      (decision) => decision.proposal === "suppress" && decision.boundedProposal,
    ).length,
    mandatoryProtected: decisions.filter((decision) => decision.protectedMandatory).length,
    reliableFloorProtected: decisions.filter((decision) => decision.protectedReliableFloor).length,
    livePromotions: changed.filter((decision) => decision.finalDecision === "selected").length,
    liveSuppressions: changed.filter((decision) => decision.finalDecision === "excluded").length,
    eligibilityChanged: changed.length > 0,
  };
}

function deduplicateCandidates(runs) {
  const latest = new Map();
  const ordered = [...runs].sort((left, right) =>
    String(left.createdAt ?? "").localeCompare(String(right.createdAt ?? "")) ||
    String(left.id ?? "").localeCompare(String(right.id ?? ""))
  );
  for (const run of ordered) {
    for (const candidate of run.candidateEvaluations ?? []) {
      latest.set(`${run.source}:${candidate.evidenceKey}`, { run, candidate });
    }
  }
  return [...latest.values()];
}

function compareReportDecision(left, right) {
  const order = { suppress: 0, promote: 1, retain: 2 };
  return order[left.proposal] - order[right.proposal] ||
    Number(right.boundedProposal) - Number(left.boundedProposal) ||
    right.preferenceConfidence - left.preferenceConfidence ||
    `${left.runId}:${left.evidenceKey}`.localeCompare(`${right.runId}:${right.evidenceKey}`);
}

function boundedInteger(value, fallback, minimum, maximum) {
  const parsed = Number.parseInt(value, 10);
  return Number.isInteger(parsed) ? Math.max(minimum, Math.min(maximum, parsed)) : fallback;
}
