import fs from "node:fs";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";

const COUNTED_TABLES = [
  "runs",
  "unified_sessions",
  "observations",
  "candidate_evaluations",
  "preference_feedback_events",
  "reasoning_invocations",
  "preference_model_snapshots",
  "calibration_sessions",
  "calibration_samples",
  "calibration_profile_snapshots",
  "knowledge_events",
  "knowledge_versions",
];

export function inspectSqliteDatabase(databasePath) {
  const resolved = path.resolve(databasePath);
  const database = new DatabaseSync(resolved, { readOnly: true });
  try {
    const integrityRows = database.prepare("PRAGMA integrity_check").all();
    const foreignKeyRows = database.prepare("PRAGMA foreign_key_check").all();
    const journalMode = database.prepare("PRAGMA journal_mode").get()?.journal_mode ?? null;
    const pageCount = Number(database.prepare("PRAGMA page_count").get()?.page_count ?? 0);
    const pageSize = Number(database.prepare("PRAGMA page_size").get()?.page_size ?? 0);
    const counts = Object.fromEntries(COUNTED_TABLES.map((table) => [
      table,
      Number(database.prepare(`SELECT COUNT(*) AS count FROM ${table}`).get()?.count ?? 0),
    ]));
    return {
      version: 1,
      status:
        integrityRows.length === 1 && integrityRows[0].integrity_check === "ok" &&
        foreignKeyRows.length === 0
          ? "healthy"
          : "degraded",
      databasePath: resolved,
      fileBytes: fs.statSync(resolved).size,
      allocatedBytes: pageCount * pageSize,
      journalMode,
      integrity: integrityRows.map((row) => row.integrity_check),
      foreignKeyViolations: foreignKeyRows.length,
      counts,
    };
  } finally {
    database.close();
  }
}

export function createSqliteBackup(databasePath, targetPath) {
  const source = path.resolve(databasePath);
  const target = path.resolve(targetPath);
  if (source === target) throw new Error("backup target must differ from the source database");
  if (fs.existsSync(target)) throw new Error("backup target already exists");
  fs.mkdirSync(path.dirname(target), { recursive: true });
  const database = new DatabaseSync(source);
  try {
    database.exec(`VACUUM INTO '${target.replaceAll("'", "''")}'`);
  } finally {
    database.close();
  }
  const health = inspectSqliteDatabase(target);
  if (health.status !== "healthy") {
    throw new Error("created backup did not pass SQLite integrity checks");
  }
  return { source, target, health };
}

export function resetSqliteForOnboarding(databasePath, targetPath, confirmation) {
  if (confirmation !== "RESET_ONBOARDING") {
    throw new Error("reset requires the exact confirmation RESET_ONBOARDING");
  }
  const source = path.resolve(databasePath);
  const before = inspectSqliteDatabase(source);
  if (before.status !== "healthy") {
    throw new Error("source database must be healthy before onboarding reset");
  }
  const backup = createSqliteBackup(source, targetPath);
  for (const candidate of [source, `${source}-wal`, `${source}-shm`, `${source}-journal`]) {
    if (fs.existsSync(candidate)) fs.rmSync(candidate);
  }
  return {
    version: 1,
    reset: "onboarding",
    source,
    backup,
    previousCounts: before.counts,
    sourceRemoved: !fs.existsSync(source),
  };
}

export function buildPilotDatasetExport(runs, options = {}) {
  const createdAt = options.createdAt ?? new Date().toISOString();
  return {
    version: 1,
    createdAt,
    containsSourceContent: true,
    containsRawObservations: false,
    liveInfluence: false,
    runs: runs.map((run) => ({
      id: run.id,
      source: run.source,
      mode: run.mode,
      intent: run.intent,
      status: run.status,
      provider: run.provider,
      createdAt: run.createdAt,
      completedAt: run.completedAt,
      coverage: run.coverage,
      result: run.result,
      error: run.error,
      candidateEvaluations: run.candidateEvaluations ?? [],
      preferenceEligibilityDecisions: run.preferenceEligibilityDecisions ?? [],
      preferenceFeedback: run.preferenceFeedback ?? [],
      reasoningInvocations: run.reasoningInvocations ?? [],
    })),
  };
}

export function previewRetention(runs, options = {}) {
  const olderThanDays = Math.max(1, Math.trunc(options.olderThanDays ?? 90));
  const now = new Date(options.now ?? Date.now()).valueOf();
  const cutoff = now - olderThanDays * 86_400_000;
  const older = runs.filter((run) => new Date(run.createdAt).valueOf() < cutoff);
  const protectedRuns = older.filter((run) =>
    (run.preferenceFeedback?.length ?? 0) > 0 ||
    (run.feedback?.length ?? 0) > 0 ||
    (run.result?.items?.length ?? 0) > 0,
  );
  return {
    version: 1,
    previewOnly: true,
    olderThanDays,
    cutoff: new Date(cutoff).toISOString(),
    totalRuns: runs.length,
    olderRuns: older.length,
    protectedRuns: protectedRuns.length,
    unprotectedRuns: older.length - protectedRuns.length,
    deletionExecuted: false,
  };
}
