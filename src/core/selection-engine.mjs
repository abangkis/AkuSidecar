import { genericMaterialityScore, normalizePreferenceAssessment } from "./preference-features.mjs";

export const SELECTION_POLICY = Object.freeze({
  version: "selection-engine-v1",
  minimumMateriality: 0.4,
  reliableFallback: true,
});

export function selectReasoningCandidates(result, run, policy = {}) {
  const effective = { ...SELECTION_POLICY, ...policy };
  const assessments = new Map((result.candidateAssessments ?? []).map((entry) => [
    entry.evidenceKey,
    normalizePreferenceAssessment(entry),
  ]));
  const ranked = (result.items ?? []).map((item, platformIndex) => {
    const assessment = assessments.get(item.evidenceKey) ?? normalizePreferenceAssessment();
    const score = genericMaterialityScore(assessment);
    const mandatory = item.knowledgeDelta === "contradiction" ||
      item.knowledgeDelta === "material_update" || assessment.urgency >= 0.85;
    return { item, platformIndex, score, mandatory };
  });
  let eligible = ranked.filter((entry) => entry.mandatory || entry.score >= effective.minimumMateriality);
  let fallbackApplied = false;
  if (eligible.length === 0 && ranked.length > 0 && effective.reliableFallback) {
    eligible = [[...ranked].sort((left, right) => right.score - left.score || left.platformIndex - right.platformIndex)[0]];
    fallbackApplied = true;
  }
  const eligibleKeys = new Set(eligible.map((entry) => entry.item.evidenceKey));
  const mandatory = eligible.filter((entry) => entry.mandatory);
  const selectedBudget = mandatory.length >= run.maxItems
    ? mandatory.slice(0, run.maxItems)
    : [
        ...mandatory,
        ...eligible.filter((entry) => !entry.mandatory).slice(0, run.maxItems - mandatory.length),
      ];
  const selectedBudgetKeys = new Set(selectedBudget.map((entry) => entry.item.evidenceKey));
  const selected = ranked.filter((entry) => selectedBudgetKeys.has(entry.item.evidenceKey));
  const selectedKeys = new Set(selected.map((entry) => entry.item.evidenceKey));
  const decisions = Object.fromEntries(ranked.map((entry) => [entry.item.evidenceKey, {
    decision: selectedKeys.has(entry.item.evidenceKey) ? "selected" : "excluded",
    reasonCode: selectedKeys.has(entry.item.evidenceKey)
      ? fallbackApplied && eligibleKeys.has(entry.item.evidenceKey)
        ? "selected_reliable_fallback"
        : entry.mandatory ? "selected_mandatory_signal" : "selected_materiality"
      : eligibleKeys.has(entry.item.evidenceKey) ? "deferred_by_attention_budget" : "below_materiality_threshold",
    materialityScore: entry.score,
  }]));
  return {
    ...result,
    items: selected.map((entry) => entry.item),
    deferredByBudget: Math.max(
      Number(result.deferredByBudget ?? 0),
      Math.max(0, eligible.length - run.maxItems),
    ),
    selection: {
      policyVersion: effective.version,
      candidateCount: ranked.length,
      eligibleCount: eligible.length,
      selectedCount: selected.length,
      excludedBelowThreshold: ranked.filter((entry) => !eligibleKeys.has(entry.item.evidenceKey)).length,
      fallbackApplied,
      threshold: effective.minimumMateriality,
      decisions,
    },
  };
}
