import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";
import {
  buildPilotDatasetExport,
  createSqliteBackup,
  inspectSqliteDatabase,
  previewRetention,
  resetSqliteForOnboarding,
} from "../src/store/sqlite-operations.mjs";

test("SQLite health and backup validate a consistent temporary database", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-sqlite-ops-"));
  const databasePath = path.join(directory, "state.db");
  const backupPath = path.join(directory, "backup", "state-backup.db");
  const quotedBackupPath = path.join(directory, "backup", "state's-backup.db");
  const store = new SqliteStateStore(databasePath);
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  store.setSetting("fixture", "value");
  store.close();

  const health = inspectSqliteDatabase(databasePath);
  assert.equal(health.status, "healthy");
  assert.equal(health.foreignKeyViolations, 0);
  assert.ok(health.counts.runs >= 0);

  const backup = createSqliteBackup(databasePath, backupPath);
  assert.equal(backup.health.status, "healthy");
  assert.equal(fs.existsSync(backupPath), true);
  assert.throws(() => createSqliteBackup(databasePath, backupPath), /already exists/);
  assert.equal(createSqliteBackup(databasePath, quotedBackupPath).health.status, "healthy");
});

test("pilot export excludes raw observations and retention stays preview-only", () => {
  const runs = [{
    id: "run-1",
    source: "x",
    mode: "catch_up",
    intent: "Engineering",
    status: "completed",
    provider: "fixture",
    createdAt: "2025-01-01T00:00:00.000Z",
    result: { items: [] },
    observations: [{ payload: { secret: "must-not-export" } }],
    candidateEvaluations: [{ evidenceKey: "x:a", text: "Source content" }],
    preferenceEligibilityDecisions: [{
      evidenceKey: "x:a",
      proposal: "suppress",
      finalDecision: "selected",
      eligibilityChanged: false,
    }],
    preferenceFeedback: [],
    reasoningInvocations: [],
  }];
  const exported = buildPilotDatasetExport(runs, { createdAt: "2026-01-01T00:00:00.000Z" });
  assert.equal(exported.containsRawObservations, false);
  assert.equal("observations" in exported.runs[0], false);
  assert.deepEqual(exported.runs[0].preferenceEligibilityDecisions, runs[0].preferenceEligibilityDecisions);
  assert.doesNotMatch(JSON.stringify(exported), /must-not-export/);

  const preview = previewRetention(runs, {
    olderThanDays: 30,
    now: "2026-01-01T00:00:00.000Z",
  });
  assert.equal(preview.olderRuns, 1);
  assert.equal(preview.deletionExecuted, false);
});

test("onboarding reset is confirmation-gated and backup-first", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-onboarding-reset-"));
  const databasePath = path.join(directory, "state.db");
  const backupPath = path.join(directory, "backup", "before-onboarding.db");
  const store = new SqliteStateStore(databasePath);
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  store.setSetting("fixture", "preserved-in-backup");
  store.close();

  assert.throws(
    () => resetSqliteForOnboarding(databasePath, backupPath, "no"),
    /exact confirmation/,
  );
  assert.equal(fs.existsSync(databasePath), true);
  const result = resetSqliteForOnboarding(
    databasePath,
    backupPath,
    "RESET_ONBOARDING",
  );
  assert.equal(result.sourceRemoved, true);
  assert.equal(result.backup.health.status, "healthy");
  assert.equal(fs.existsSync(backupPath), true);
  assert.equal(fs.existsSync(databasePath), false);
});

test("settings resets require typed confirmation, reject active work, and preserve a verified backup", (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-settings-reset-"));
  const databasePath = path.join(directory, "state.db");
  const backupPath = path.join(directory, "backups", "before-full-reset.db");
  const store = new SqliteStateStore(databasePath);
  context.after(() => {
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });

  store.createRun({
    id: "reset-run",
    mode: "catch_up",
    source: "x",
    intent: "latest",
    maxItems: 1,
    scrolls: 0,
  }, "fixture");
  assert.throws(() => store.resetLearningData("RESET LEARNING"), /while an update is running/);
  store.completeRunWithKnowledge("reset-run", { items: [] }, {}, [], [{
    evidenceKey: "x:reset",
    policyVersion: "preference-eligibility-v2",
    proposal: "retain",
    baselineDecision: "selected",
    finalDecision: "selected",
    eligibilityChanged: false,
  }]);
  store.setSetting("user.onboarding_profile_v0", JSON.stringify({
    version: 0,
    status: "completed",
    activeSources: ["x"],
  }));
  store.setSetting("ui.timeline_capacity", "20");
  store.setSetting("preference.runtime.active_snapshot_id", "snapshot-1");
  store.addPreferenceFeedback("reset-run", {
    evidenceKey: "x:reset",
    kind: "more_like_this",
    reasonCode: null,
    note: "",
  });
  store.savePreferenceModelSnapshot({
    id: "snapshot-1",
    version: 1,
    datasetFingerprint: "reset-fixture",
    createdAt: "2026-07-15T00:00:00.000Z",
  });

  assert.throws(() => store.resetLearningData("DELETE"), /RESET LEARNING/);
  const learning = store.resetLearningData("RESET LEARNING");
  assert.equal(learning.deleted.preferenceFeedbackEvents, 1);
  assert.equal(learning.deleted.preferenceModelSnapshots, 1);
  assert.equal(learning.deleted.preferenceEligibilityDecisions, 1);
  assert.deepEqual(learning.remaining, {
    preferenceFeedbackEvents: 0,
    preferenceEligibilityDecisions: 0,
    preferenceModelSnapshots: 0,
    calibrationSessions: 0,
    calibrationSamples: 0,
    calibrationProfileSnapshots: 0,
  });
  assert.ok(store.getRun("reset-run"));
  assert.equal(store.getSetting("ui.timeline_capacity"), "20");
  assert.match(store.getSetting("user.onboarding_profile_v0"), /completed/);

  const bridgeToken = store.getOrCreateBridgeToken();
  const backup = createSqliteBackup(databasePath, backupPath);
  assert.throws(() => store.resetForOnboarding("DELETE", backup), /RESET AKUBROWSER/);
  const full = store.resetForOnboarding("RESET AKUBROWSER", backup);
  assert.equal(full.backup.status, "healthy");
  assert.equal(store.listRuns().length, 0);
  assert.equal(store.getSetting("user.onboarding_profile_v0"), null);
  assert.equal(store.getSetting("ui.timeline_capacity"), null);
  assert.equal(store.getOrCreateBridgeToken(), bridgeToken);
  assert.match(store.getSetting("operations.last_full_reset"), /before-full-reset/);
});
