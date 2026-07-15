import assert from "node:assert/strict";
import test from "node:test";
import {
  buildPreferenceReplay,
  PREFERENCE_REPLAY_THRESHOLDS,
} from "../src/core/preference-replay.mjs";

test("offline replay matches preference signals to structured candidate assessments", () => {
  const replay = buildPreferenceReplay([
    run("run-1", "x", [
      candidate("x:a", "excluded", assessment("tutorial", ["codex"], 0.8)),
      candidate("x:b", "selected", assessment("opinion", ["ai"], 0.4)),
    ], [
      feedback("x:a", "more_like_this"),
      feedback("x:b", "less_like_this"),
    ]),
  ]);

  assert.equal(replay.mode, "offline_replay");
  assert.equal(replay.liveInfluence, false);
  assert.equal(replay.readiness.status, "collecting");
  assert.equal(replay.dataset.feedbackEvents, 2);
  assert.equal(replay.dataset.matchedFeedback, 2);
  assert.equal(replay.dataset.assessedFeedback, 2);
  assert.equal(replay.dataset.moreLikeThis, 1);
  assert.equal(replay.dataset.lessLikeThis, 1);
  assert.deepEqual(replay.dataset.sources, ["x"]);
  assert.equal(replay.tendencies.contentTypes[0].total, 1);
  assert.equal(replay.tendencies.scoreAverages.novelty.positive, 0.8);
  assert.equal(replay.tendencies.scoreAverages.novelty.negative, 0.4);
});

test("offline replay becomes fit-ready only after every calibration gate passes", () => {
  const runs = [];
  const total = PREFERENCE_REPLAY_THRESHOLDS.feedbackEvents;
  for (let index = 0; index < total; index += 1) {
    const negative = index < PREFERENCE_REPLAY_THRESHOLDS.lessLikeThis;
    const evidenceKey = `x:${index}`;
    runs.push(run(
      `run-${index}`,
      index % 2 === 0 ? "x" : "linkedin",
      [candidate(evidenceKey, negative ? "selected" : "excluded", assessment("release", ["engineering"], 0.7))],
      [feedback(evidenceKey, negative ? "less_like_this" : "more_like_this")],
    ));
  }
  const replay = buildPreferenceReplay(runs);
  assert.equal(replay.readiness.status, "ready_for_offline_fit");
  assert.equal(replay.readiness.passedGates, replay.readiness.totalGates);
  assert.equal(replay.liveInfluence, false);
});

test("offline replay uses the latest contextual signal while preserving append-only history", () => {
  const replay = buildPreferenceReplay([
    run("run-reversal", "x", [
      candidate("x:reversed", "selected", assessment("opinion", ["ai"], 0.5)),
    ], [
      feedback("x:reversed", "more_like_this"),
      feedback("x:reversed", "less_like_this"),
    ]),
  ]);
  assert.equal(replay.dataset.feedbackEvents, 1);
  assert.equal(replay.dataset.moreLikeThis, 0);
  assert.equal(replay.dataset.lessLikeThis, 1);
});

test("offline replay excludes a diagnostic Less refinement from preference readiness", () => {
  const replay = buildPreferenceReplay([
    run("run-diagnostic", "x", [
      candidate("x:known", "selected", assessment("news", ["science"], 0.7)),
    ], [
      feedback("x:known", "less_like_this"),
      { ...feedback("x:known", "less_like_this"), reasonCode: "already_known" },
    ]),
  ]);

  assert.equal(replay.dataset.feedbackEvents, 0);
  assert.equal(replay.dataset.lessLikeThis, 0);
});

test("offline replay preserves historical wrong_topic as full preference evidence", () => {
  const replay = buildPreferenceReplay([
    run("run-legacy-reason", "x", [
      candidate("x:legacy", "selected", assessment("opinion", ["culture"], 0.4)),
    ], [
      { ...feedback("x:legacy", "less_like_this"), reasonCode: "wrong_topic" },
    ]),
  ]);

  assert.equal(replay.dataset.feedbackEvents, 1);
  assert.equal(replay.dataset.lessLikeThis, 1);
});

function run(id, source, candidateEvaluations, preferenceFeedback) {
  return { id, source, candidateEvaluations, preferenceFeedback };
}

function candidate(evidenceKey, decision, value) {
  return { evidenceKey, decision, assessment: value };
}

function feedback(evidenceKey, kind) {
  return { evidenceKey, kind };
}

function assessment(contentType, topicTags, preferenceScore) {
  return {
    contentType,
    topicTags,
    novelty: preferenceScore,
    urgency: 0.3,
    actionability: 0.5,
    rationale: "Fixture assessment.",
  };
}
