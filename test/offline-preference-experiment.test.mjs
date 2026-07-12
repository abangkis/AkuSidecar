import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  buildShadowComparison,
  explainPreferenceCandidate,
  fitOfflinePreferenceExperiment,
  preferenceDatasetFingerprint,
  preferenceExperimentStatus,
  scorePreferenceCandidate,
} from "../src/core/offline-preference-experiment.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";
import { syntheticPreferenceReadyRuns } from "../test-support/preference-ready-dataset.mjs";

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
  const runs = syntheticPreferenceReadyRuns();
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
  const explanation = explainPreferenceCandidate(experiment.snapshot, {
    source: "x",
    decision: "excluded",
    assessment: assessment("release", ["engineering"], 0.9),
  });
  assert.equal(explanation.probability, positiveScore);
  assert.ok(explanation.contributions.length > 0);

  const comparison = buildShadowComparison(experiment.snapshot, runs);
  assert.equal(comparison.available, true);
  assert.equal(comparison.liveInfluence, false);
  assert.equal(comparison.summary.scoredCandidates, 30);
  assert.deepEqual(comparison.pagination, {
    total: 30,
    offset: 0,
    limit: 50,
    returned: 30,
    hasNext: false,
  });
  assert.equal(
    comparison.summary.wouldMoveUp + comparison.summary.wouldMoveDown + comparison.summary.unchanged,
    30,
  );
  assert.ok(comparison.candidates.every((entry) => entry.contributions.length > 0));
});

test("shadow comparison remains unavailable without a current snapshot", () => {
  const comparison = buildShadowComparison(null, syntheticPreferenceReadyRuns());
  assert.equal(comparison.available, false);
  assert.equal(comparison.reason, "no_current_snapshot");
  assert.equal(comparison.liveInfluence, false);
  assert.equal(comparison.pagination.total, 0);
});

test("shadow promotion keeps provider priority as a conservative eligibility guardrail", () => {
  const baseRuns = syntheticPreferenceReadyRuns();
  const experiment = fitOfflinePreferenceExperiment(baseRuns, {
    createdAt: "2026-07-11T00:00:00.000Z",
  });
  const strongAssessment = assessment("release", ["engineering"], 0.95);
  const comparison = buildShadowComparison(experiment.snapshot, [{
    id: "guardrail-run",
    source: "x",
    candidateEvaluations: [
      {
        evidenceKey: "x:eligible-p2",
        decision: "excluded",
        assessment: { ...strongAssessment, recommendedPriority: "P2" },
      },
      {
        evidenceKey: "x:blocked-p4",
        decision: "excluded",
        assessment: { ...strongAssessment, recommendedPriority: "P4" },
      },
    ],
  }]);

  assert.equal(
    comparison.candidates.find((entry) => entry.evidenceKey === "x:eligible-p2").movement,
    "would_move_up",
  );
  assert.equal(
    comparison.candidates.find((entry) => entry.evidenceKey === "x:blocked-p4").movement,
    "unchanged",
  );
});

test("shadow comparison exposes every scored candidate for complete evaluation", () => {
  const baseRuns = syntheticPreferenceReadyRuns();
  const experiment = fitOfflinePreferenceExperiment(baseRuns, {
    createdAt: "2026-07-11T00:00:00.000Z",
  });
  const repeatedRuns = Array.from({ length: 4 }, (_, batch) =>
    baseRuns.map((run) => ({
      ...run,
      id: `${run.id}-batch-${batch}`,
    })),
  ).flat();
  const repeatedComparison = buildShadowComparison(experiment.snapshot, repeatedRuns);
  assert.equal(repeatedComparison.summary.scoredCandidates, 30);
  assert.equal(repeatedComparison.summary.duplicateCandidatesCollapsed, 90);

  const expandedRuns = repeatedRuns.map((run) => ({
    ...run,
    candidateEvaluations: run.candidateEvaluations.map((candidate) => ({
      ...candidate,
      evidenceKey: `${candidate.evidenceKey}:${run.id}`,
    })),
  }));

  const firstPage = buildShadowComparison(experiment.snapshot, expandedRuns, {
    limit: 100,
    offset: 0,
  });
  const secondPage = buildShadowComparison(experiment.snapshot, expandedRuns, {
    limit: 100,
    offset: 100,
  });

  assert.equal(firstPage.summary.scoredCandidates, 120);
  assert.equal(firstPage.candidates.length, 100);
  assert.equal(firstPage.pagination.hasNext, true);
  assert.equal(secondPage.summary.scoredCandidates, 120);
  assert.equal(secondPage.candidates.length, 20);
  assert.equal(secondPage.pagination.hasNext, false);
  assert.equal(
    new Set([...firstPage.candidates, ...secondPage.candidates].map((entry) =>
      `${entry.runId}:${entry.evidenceKey}`,
    )).size,
    120,
  );
});

test("preference snapshots are persisted idempotently by dataset fingerprint", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-preference-snapshot-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  context.after(() => {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });
  const first = fitOfflinePreferenceExperiment(syntheticPreferenceReadyRuns(), {
    createdAt: "2026-07-11T00:00:00.000Z",
  }).snapshot;
  const duplicate = { ...first, id: "duplicate-id", createdAt: "2026-07-12T00:00:00.000Z" };
  assert.equal(store.savePreferenceModelSnapshot(first).id, first.id);
  assert.equal(store.savePreferenceModelSnapshot(duplicate).id, first.id);
  assert.equal(store.getLatestPreferenceModelSnapshot().datasetFingerprint, first.datasetFingerprint);
});

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
