import assert from "node:assert/strict";
import test from "node:test";
import {
  composePreferenceOrder,
  ensureLocalPreferenceRuntime,
  preferenceRuntimeStatus,
  resetLocalPreferenceRuntime,
} from "../src/core/preference-runtime.mjs";
import { syntheticPreferenceReadyRuns } from "../test-support/preference-ready-dataset.mjs";

test("local preference runtime fits automatically without waiting for the diagnostic gate", () => {
  const complete = syntheticPreferenceReadyRuns();
  const runs = [complete[0], complete[1], complete[6], complete[7]];
  const store = memoryStore(runs);
  const before = preferenceRuntimeStatus(store);
  assert.equal(before.activationState, "baseline");
  assert.equal(before.eligibleForPersonalFit, true);

  const after = ensureLocalPreferenceRuntime(store, {}, {
    force: true,
    trigger: "test",
  });
  assert.equal(after.version, 2);
  assert.equal(after.activationState, "active");
  assert.equal(after.liveInfluence, true);
  assert.equal(after.currentSnapshot.origin, "local_runtime");
  assert.equal(after.currentSnapshot.proposedPolicy.influence, "bounded_selected_rerank");
});

test("bounded composition reorders only existing selected items by at most two positions", () => {
  const entries = [0, 1, 2, 3, 4].map((index) => ({
    sessionId: "session",
    runId: "run",
    item: { evidenceKey: `x:${index}` },
    preferenceCandidate: {
      source: "x",
      decision: "selected",
      assessment: {
        contentType: "opinion",
        topicTags: [],
        novelty: index / 4,
        urgency: 0.5,
        actionability: index / 4,
      },
    },
  }));
  const runtime = {
    liveInfluence: true,
    activationState: "active",
    policy: {
      version: "preference-runtime-v2",
      baseline: "source_platform_order",
      maxRankDisplacement: 2,
      minimumScoreDelta: 0.01,
    },
    currentSnapshot: {
      id: "snapshot",
      model: {
        intercept: 0,
        categorical: { source: {}, contentType: {}, topicTag: {} },
        continuous: {
          novelty: { weight: 2 },
          urgency: { weight: 0 },
          actionability: { weight: 2 },
        },
      },
    },
  };

  const composed = composePreferenceOrder(entries, runtime);
  assert.deepEqual(
    [...composed.map((entry) => entry.item.evidenceKey)].sort(),
    entries.map((entry) => entry.item.evidenceKey).sort(),
  );
  assert.ok(composed.some((entry) => entry.preference.displacement !== 0));
  assert.ok(composed.every((entry) => Math.abs(entry.preference.displacement) <= 2));
  assert.ok(composed.every((entry) => entry.preference.liveInfluence));
});

test("disabled personalization preserves the baseline order", () => {
  const entries = ["x:0", "linkedin:0", "x:1"].map((evidenceKey) => ({
    sessionId: "session",
    runId: "run",
    item: { evidenceKey },
    preferenceCandidate: null,
  }));
  const composed = composePreferenceOrder(entries, {
    liveInfluence: false,
    activationState: "baseline",
  });
  assert.deepEqual(
    composed.map((entry) => entry.item.evidenceKey),
    entries.map((entry) => entry.item.evidenceKey),
  );
  assert.ok(composed.every((entry) => entry.preference.liveInfluence === false));
});

test("reset returns to baseline without deleting the audit ledger", () => {
  const complete = syntheticPreferenceReadyRuns();
  const runs = [complete[0], complete[1], complete[6], complete[7]];
  const store = memoryStore(runs);
  const active = ensureLocalPreferenceRuntime(store, {}, { force: true });
  assert.equal(active.activationState, "active");

  const reset = resetLocalPreferenceRuntime(store);
  assert.equal(reset.activationState, "baseline");
  assert.equal(reset.suspended, true);
  assert.equal(reset.signalCounts.total, 4);
  assert.equal(ensureLocalPreferenceRuntime(store).activationState, "baseline");
  assert.equal(
    ensureLocalPreferenceRuntime(store, {}, { forceFit: true, trigger: "before_session" }).activationState,
    "baseline",
  );
});

test("an active champion remains live while newer feedback waits for a challenger decision", () => {
  const complete = syntheticPreferenceReadyRuns();
  const runs = [complete[0], complete[1], complete[6], complete[7]];
  const store = memoryStore(runs);
  const active = ensureLocalPreferenceRuntime(store, {}, { forceFit: true });
  const championId = active.activeSnapshot.id;
  runs.push(complete[8]);
  const stale = preferenceRuntimeStatus(store);
  assert.equal(stale.liveInfluence, true);
  assert.equal(stale.activeSnapshot.id, championId);
  assert.equal(stale.currentSnapshot, null);
});

function memoryStore(runs) {
  const settings = new Map();
  const snapshots = new Map();
  return {
    listRunsWithFeedback() { return runs; },
    getSetting(key) { return settings.get(key) ?? null; },
    setSetting(key, value) { settings.set(key, value); },
    getPreferenceModelSnapshotById(id) { return snapshots.get(id) ?? null; },
    savePreferenceModelSnapshot(snapshot) {
      snapshots.set(snapshot.id, snapshot);
      return snapshot;
    },
  };
}
