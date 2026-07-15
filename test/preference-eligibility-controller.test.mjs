import assert from "node:assert/strict";
import test from "node:test";
import {
  applyPreferenceEligibilityResult,
  buildPreferenceEligibilityReport,
  evaluatePreferenceEligibilityRun,
} from "../src/core/preference-eligibility-controller.mjs";

test("guarded eligibility applies one bounded promotion and suppression", () => {
  const run = { id: "run-1", source: "x", maxItems: 3 };
  const evaluation = evaluatePreferenceEligibilityRun({
    run,
    runtime: readyRuntime(),
    policy: { mode: "guarded_live" },
    candidates: [
      candidate("x:0123456789abcdef01234567", "excluded", "news", 0.35),
      candidate("x:1123456789abcdef01234567", "selected", "opinion", 0.7),
      candidate("x:2123456789abcdef01234567", "selected", "opinion", 0.7, "selected_mandatory_signal"),
    ],
  });

  const promote = evaluation.decisions[0];
  const suppress = evaluation.decisions[1];
  const mandatory = evaluation.decisions[2];
  assert.equal(promote.proposal, "promote");
  assert.equal(promote.boundedProposal, true);
  assert.equal(promote.budgetEffect, "unused_budget");
  assert.equal(promote.finalDecision, "selected");
  assert.equal(promote.reasonCode, "live_promotion_unused_budget");
  assert.equal(suppress.proposal, "suppress");
  assert.equal(suppress.boundedProposal, true);
  assert.equal(suppress.finalDecision, "excluded");
  assert.equal(suppress.reasonCode, "live_suppression_guarded");
  assert.equal(mandatory.proposal, "retain");
  assert.equal(mandatory.reasonCode, "suppression_blocked_mandatory_signal");
  assert.equal(evaluation.liveMutation, true);
  assert.equal(evaluation.summary.livePromotions, 1);
  assert.equal(evaluation.summary.liveSuppressions, 1);
  assert.equal(evaluation.summary.eligibilityChanged, true);
});

test("default authority fills unused capacity but keeps suppression disabled", () => {
  const evaluation = evaluatePreferenceEligibilityRun({
    run: { id: "run-live", source: "x", maxItems: 3 },
    runtime: readyRuntime(),
    candidates: [
      candidate("x:0123456789abcdef01234567", "excluded", "news", 0.35),
      candidate("x:1123456789abcdef01234567", "selected", "opinion", 0.7),
      candidate("x:2123456789abcdef01234567", "selected", "other", 0.7),
    ],
  });
  assert.equal(evaluation.authority, "promote_unused_budget");
  assert.equal(evaluation.decisions[0].finalDecision, "selected");
  assert.equal(evaluation.decisions[1].proposal, "retain");
  assert.equal(evaluation.decisions[1].reasonCode, "suppression_authority_disabled");
});

test("live decisions rebuild the displayed set in evaluated platform order", () => {
  const evaluation = evaluatePreferenceEligibilityRun({
    run: { id: "run-result", source: "x", maxItems: 2 },
    runtime: readyRuntime(),
    candidates: [
      candidate("x:0123456789abcdef01234567", "excluded", "news", 0.35),
      candidate("x:1123456789abcdef01234567", "selected", "other", 0.7),
    ],
  });
  const result = applyPreferenceEligibilityResult(
    { items: [{ id: "selected", evidenceKey: "x:1123456789abcdef01234567" }], selection: { selectedCount: 1 } },
    [
      { id: "promoted", evidenceKey: "x:0123456789abcdef01234567" },
      { id: "selected", evidenceKey: "x:1123456789abcdef01234567" },
    ],
    evaluation,
  );
  assert.deepEqual(result.items.map((item) => item.id), ["promoted", "selected"]);
  assert.equal(result.selection.baselineSelectedCount, 1);
  assert.equal(result.selection.preferenceEligibilityChanged, true);
});

test("controller protects the reliable floor and generic materiality floor", () => {
  const evaluation = evaluatePreferenceEligibilityRun({
    run: { id: "run-floor", source: "linkedin", maxItems: 1 },
    runtime: readyRuntime(),
    candidates: [
      candidate("linkedin:0123456789abcdef01234567", "selected", "opinion", 0.7),
      candidate("linkedin:1123456789abcdef01234567", "excluded", "news", 0.1),
    ],
  });
  assert.equal(evaluation.decisions[0].proposal, "retain");
  assert.equal(evaluation.decisions[0].reasonCode, "suppression_blocked_reliable_floor");
  assert.equal(evaluation.decisions[1].proposal, "retain");
  assert.equal(evaluation.decisions[1].reasonCode, "promotion_blocked_by_generic_floor");
});

test("same canonical assessment receives the same preference probability across sources", () => {
  const runtime = readyRuntime();
  const x = evaluatePreferenceEligibilityRun({
    run: { id: "x-run", source: "x", maxItems: 2 },
    runtime,
    candidates: [candidate("x:0123456789abcdef01234567", "excluded", "news", 0.35)],
  });
  const linkedin = evaluatePreferenceEligibilityRun({
    run: { id: "li-run", source: "linkedin", maxItems: 2 },
    runtime,
    candidates: [candidate("linkedin:0123456789abcdef01234567", "excluded", "news", 0.35)],
  });
  assert.equal(x.decisions[0].probability, linkedin.decisions[0].probability);
});

test("report exposes actionable candidates, evidence gates, and feedback context", () => {
  const xCandidate = candidate("x:0123456789abcdef01234567", "excluded", "news", 0.35);
  const liCandidate = candidate("linkedin:0123456789abcdef01234567", "selected", "opinion", 0.7);
  const liFloor = candidate("linkedin:1123456789abcdef01234567", "selected", "other", 0.7);
  const report = buildPreferenceEligibilityReport([
    run("x-run", "x", xCandidate, "more_like_this"),
    { ...run("li-run", "linkedin", liCandidate, "less_like_this"), candidateEvaluations: [liCandidate, liFloor] },
  ], readyRuntime(), { limit: 10 });

  assert.equal(report.mode, "preference_eligibility_live");
  assert.equal(report.authority.mode, "promote_unused_budget");
  assert.equal(report.authority.liveMutation, true);
  assert.equal(report.readiness.promotionReady, true);
  assert.equal(report.readiness.suppressionReady, true);
  assert.equal(report.summary.actionableCandidates, 1);
  assert.deepEqual(report.candidates.map((entry) => entry.proposal), ["promote"]);
  assert.equal(report.candidates[0].preferenceFeedback[0].kind, "more_like_this");
});

function readyRuntime() {
  return {
    liveInfluence: true,
    datasetFingerprint: "runtime-fixture",
    signalCounts: { total: 20, positive: 10, negative: 10 },
    activeSnapshot: {
      id: "snapshot-fixture",
      evaluation: { balancedAccuracy: 0.8, negativeRecall: 0.8 },
      model: {
        intercept: 0,
        categorical: {
          contentType: { news: 2, opinion: -2 },
          topicFacet: {},
        },
        continuous: Object.fromEntries([
          "novelty",
          "urgency",
          "actionability",
          "materiality",
          "evidenceStrength",
        ].map((field) => [field, { weight: 0 }])),
      },
    },
  };
}

function candidate(evidenceKey, decision, contentType, selectionScore, reasonCode = null) {
  return {
    evidenceKey,
    source: evidenceKey.startsWith("x:") ? "x" : "linkedin",
    decision,
    reasonCode: reasonCode ?? (decision === "selected" ? "selected_materiality" : "below_materiality_threshold"),
    selectionScore,
    author: "Fixture author",
    text: "Fixture candidate text",
    sourceUrl: evidenceKey.startsWith("x:") ? "https://x.com/example/status/1" : "https://www.linkedin.com/feed/update/1/",
    assessment: {
      contentType,
      topicTags: ["fixture"],
      topicFacets: ["other"],
      novelty: 0.5,
      urgency: 0.5,
      actionability: 0.5,
      materiality: 0.5,
      evidenceStrength: 0.8,
    },
  };
}

function run(id, source, value, kind) {
  return {
    id,
    source,
    maxItems: 3,
    createdAt: `2026-07-15T00:00:0${id.length}.000Z`,
    candidateEvaluations: [value],
    preferenceFeedback: [{ evidenceKey: value.evidenceKey, kind, origin: "routine" }],
  };
}
