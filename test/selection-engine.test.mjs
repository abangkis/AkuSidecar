import assert from "node:assert/strict";
import test from "node:test";
import { selectReasoningCandidates } from "../src/core/selection-engine.mjs";

test("selection engine owns materiality admission and preserves platform order", () => {
  const result = selectReasoningCandidates({
    items: [item("x:a", "new_event"), item("x:b", "new_event"), item("x:c", "material_update")],
    candidateAssessments: [
      assessment("x:a", 0.2),
      assessment("x:b", 0.8),
      assessment("x:c", 0.1),
    ],
    deferredByBudget: 0,
  }, { maxItems: 2 });

  assert.deepEqual(result.items.map((entry) => entry.evidenceKey), ["x:b", "x:c"]);
  assert.equal(result.selection.decisions["x:a"].reasonCode, "below_materiality_threshold");
  assert.equal(result.selection.decisions["x:c"].reasonCode, "selected_mandatory_signal");
  assert.equal(result.selection.policyVersion, "selection-engine-v1");
});

test("selection engine uses one reliable fallback when every score is weak", () => {
  const result = selectReasoningCandidates({
    items: [item("x:a"), item("x:b")],
    candidateAssessments: [assessment("x:a", 0.1), assessment("x:b", 0.2)],
  }, { maxItems: 5 });
  assert.deepEqual(result.items.map((entry) => entry.evidenceKey), ["x:b"]);
  assert.equal(result.selection.fallbackApplied, true);
});

test("mandatory signals consume the finite budget before ordinary eligible candidates", () => {
  const result = selectReasoningCandidates({
    items: [item("x:a"), item("x:b", "material_update")],
    candidateAssessments: [{ ...assessment("x:a", 0.9), urgency: 0.1 }, assessment("x:b", 0.1)],
  }, { maxItems: 1 });

  assert.deepEqual(result.items.map((entry) => entry.evidenceKey), ["x:b"]);
  assert.equal(result.selection.decisions["x:a"].reasonCode, "deferred_by_attention_budget");
  assert.equal(result.selection.decisions["x:b"].reasonCode, "selected_mandatory_signal");
});

function item(evidenceKey, knowledgeDelta = "new_event") {
  return { evidenceKey, knowledgeDelta };
}

function assessment(evidenceKey, value) {
  return {
    evidenceKey,
    contentType: "news",
    topicTags: ["engineering"],
    novelty: value,
    urgency: value,
    actionability: value,
    materiality: value,
    evidenceStrength: value,
  };
}
