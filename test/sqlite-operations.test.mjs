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
    preferenceFeedback: [],
    reasoningInvocations: [],
  }];
  const exported = buildPilotDatasetExport(runs, { createdAt: "2026-01-01T00:00:00.000Z" });
  assert.equal(exported.containsRawObservations, false);
  assert.equal("observations" in exported.runs[0], false);
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
