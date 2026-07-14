import assert from "node:assert/strict";
import test from "node:test";
import { CalibrationEngine, sampleCandidates } from "../src/core/calibration-engine.mjs";

test("calibration sampler round-robins raw candidates while preserving source order", () => {
  const children = [child("x", 6), child("linkedin", 3)];
  const samples = sampleCandidates(children, { maxItems: 8, maxItemsPerSource: 5 });
  assert.deepEqual(samples.map((sample) => sample.evidenceKey), [
    "x:0", "linkedin:0", "x:1", "linkedin:1", "x:2", "linkedin:2", "x:3", "x:4",
  ]);
});

test("calibration lifecycle feeds directional labels into local fitting", () => {
  const store = memoryStore([child("x", 2), child("linkedin", 1)]);
  const engine = new CalibrationEngine({ store, maxItems: 10 });
  let session = engine.createFromUnifiedSession("unified-1");
  assert.equal(session.sampleCount, 3);
  assert.equal(session.status, "reviewing");
  session = engine.decide(session.id, 0, { label: "more_like_this" });
  session = engine.decide(session.id, 1, { label: "neutral" });
  session = engine.decide(session.id, 2, { label: "less_like_this" });
  assert.equal(session.status, "completed");
  assert.equal(session.snapshot.labels.moreLikeThis, 1);
  assert.equal(session.snapshot.labels.neutral, 1);
  assert.equal(session.snapshot.labels.lessLikeThis, 1);
  assert.equal(session.snapshot.labels.captureIssues, 0);
  assert.equal(session.snapshot.liveInfluence, false);
  assert.equal(session.snapshot.activationState, "feeds_local_fit");
  assert.deepEqual(store.preferenceFeedback.map((entry) => entry.kind), [
    "more_like_this",
    "less_like_this",
  ]);
  assert.equal(engine.createFromUnifiedSession("unified-1").id, session.id);
});

test("first-run calibration is unique even when another unified session completes", () => {
  const store = memoryStore([child("x", 2), child("linkedin", 1)]);
  const engine = new CalibrationEngine({ store, maxItems: 10 });
  const first = engine.createFromUnifiedSession("unified-1", { triggerKind: "first_run" });
  const repeated = engine.createFromUnifiedSession("unified-2", { triggerKind: "first_run" });
  assert.equal(repeated.id, first.id);
  assert.equal(repeated.unifiedSessionId, "unified-1");
  assert.equal(store.createdCount, 1);
});

function child(source, count) {
  return {
    source,
    runId: `run-${source}`,
    run: {
      status: "completed",
      candidateEvaluations: Array.from({ length: count }, (_, index) => ({
        runId: `run-${source}`,
        evidenceKey: `${source}:${index}`,
        source,
        feedPosition: index,
        author: `${source} author ${index}`,
        text: `${source} candidate ${index}`,
        sourceUrl: `https://example.test/${source}/${index}`,
        media: [],
      })),
    },
  };
}

function memoryStore(children) {
  let calibration = null;
  let createdCount = 0;
  const settings = new Map();
  const preferenceFeedback = [];
  return {
    setSetting(key, value) { settings.set(key, value); },
    getUnifiedSession(id) {
      return ["unified-1", "unified-2"].includes(id) ? { id, status: "completed", children } : null;
    },
    getCalibrationSessionByUnifiedSession(id) {
      return calibration?.unifiedSessionId === id ? calibration : null;
    },
    getCalibrationSessionByTriggerKind(triggerKind) {
      return calibration?.triggerKind === triggerKind ? calibration : null;
    },
    createCalibrationSession(input, samples) {
      createdCount += 1;
      calibration = project(input, samples);
      return calibration;
    },
    getCalibrationSession(id) {
      return calibration?.id === id ? calibration : null;
    },
    getActiveCalibrationSession() {
      return calibration?.status === "reviewing" ? calibration : null;
    },
    addPreferenceFeedback(runId, feedback) {
      preferenceFeedback.push({ runId, ...feedback });
    },
    recordCalibrationDecision(id, ordinal, decision) {
      if (calibration?.id !== id || !calibration.samples[ordinal]) return null;
      Object.assign(calibration.samples[ordinal], decision);
      calibration.resolvedCount = calibration.samples.filter((sample) => sample.label || sample.issueCode).length;
      calibration.currentOrdinal = calibration.samples.find((sample) => !sample.label && !sample.issueCode)?.ordinal ?? null;
      return calibration;
    },
    completeCalibrationSession(id, snapshot) {
      calibration.status = "completed";
      calibration.snapshot = snapshot;
      return calibration;
    },
    settings,
    preferenceFeedback,
    get createdCount() { return createdCount; },
  };
}

function project(input, samples) {
  return {
    ...input,
    status: "reviewing",
    sampleCount: samples.length,
    resolvedCount: 0,
    currentOrdinal: 0,
    samples: samples.map((sample, ordinal) => ({ ...sample, ordinal, label: null, issueCode: null })),
    snapshot: null,
    liveInfluence: false,
  };
}
