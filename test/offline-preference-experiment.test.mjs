import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  fitOfflinePreferenceExperiment,
  preferenceDatasetFingerprint,
  preferenceExperimentStatus,
  scorePreferenceCandidate,
} from "../src/core/offline-preference-experiment.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

test("offline preference fitting is hard-blocked until every replay gate passes", () => {
  const runs = [runFixture(0, "more_like_this")];
  const status = preferenceExperimentStatus(runs);
  const fit = fitOfflinePreferenceExperiment(runs);
  assert.equal(status.status, "blocked");
  assert.equal(status.liveInfluence, false);
  assert.equal(fit.status, "blocked");
  assert.equal(fit.snapshot, undefined);
});

test("fit creates a deterministic shadow-only snapshot after readiness", () => {
  const runs = readyRuns();
  const experiment = fitOfflinePreferenceExperiment(runs, {
    createdAt: "2026-07-11T00:00:00.000Z",
  });
  assert.equal(experiment.status, "fitted");
  assert.equal(experiment.snapshot.liveInfluence, false);
  assert.equal(experiment.snapshot.proposedPolicy.rankingInfluence, false);
  assert.equal(experiment.snapshot.proposedPolicy.explorationLane.active, false);
  assert.ok(experiment.snapshot.split.trainingSignals > 0);
  assert.ok(experiment.snapshot.split.holdoutSignals > 0);
  assert.equal(experiment.snapshot.evaluation.sufficientForActivationDecision, false);
  assert.equal(experiment.snapshot.shadow.appliedToLiveRanking, false);
  assert.equal(experiment.snapshot.datasetFingerprint, preferenceDatasetFingerprint([...runs].reverse()));

  const positiveScore = scorePreferenceCandidate(experiment.snapshot, {
    source: "x",
    decision: "excluded",
    assessment: assessment("release", ["engineering"], 0.9),
  });
  assert.ok(Number.isFinite(positiveScore));
  assert.ok(positiveScore >= 0 && positiveScore <= 1);
});

test("preference snapshots are persisted idempotently by dataset fingerprint", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-preference-snapshot-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  context.after(() => {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });
  const first = fitOfflinePreferenceExperiment(readyRuns(), {
    createdAt: "2026-07-11T00:00:00.000Z",
  }).snapshot;
  const duplicate = { ...first, id: "duplicate-id", createdAt: "2026-07-12T00:00:00.000Z" };
  assert.equal(store.savePreferenceModelSnapshot(first).id, first.id);
  assert.equal(store.savePreferenceModelSnapshot(duplicate).id, first.id);
  assert.equal(store.getLatestPreferenceModelSnapshot().datasetFingerprint, first.datasetFingerprint);
});

function readyRuns() {
  return Array.from({ length: 30 }, (_, index) =>
    runFixture(index, index < 6 ? "less_like_this" : "more_like_this"),
  );
}

function runFixture(index, kind) {
  const source = index % 2 === 0 ? "x" : "linkedin";
  const evidenceKey = `${source}:${index}`;
  const positive = kind === "more_like_this";
  return {
    id: `run-${index}`,
    source,
    candidateEvaluations: [{
      evidenceKey,
      decision: index % 3 === 0 ? "selected" : "excluded",
      assessment: assessment(
        positive ? "release" : "opinion",
        positive ? ["engineering"] : ["generic"],
        positive ? 0.9 : 0.2,
      ),
    }],
    preferenceFeedback: [{ evidenceKey, kind }],
  };
}

function assessment(contentType, topicTags, intentRelevance) {
  return {
    contentType,
    topicTags,
    recommendedPriority: intentRelevance > 0.5 ? "P1" : "P4",
    intentRelevance,
    novelty: intentRelevance,
    urgency: intentRelevance,
    actionability: intentRelevance,
    rationale: "Fixture assessment.",
  };
}
