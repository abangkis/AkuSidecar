import fs from "node:fs";
import path from "node:path";
import { randomBytes, randomUUID, timingSafeEqual } from "node:crypto";
import { DatabaseSync } from "node:sqlite";

export class SqliteStateStore {
  constructor(databasePath) {
    fs.mkdirSync(path.dirname(databasePath), { recursive: true });
    this.databasePath = databasePath;
    this.database = new DatabaseSync(databasePath);
    this.database.exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;");
    this.#migrate();
  }

  close() {
    this.database.close();
  }

  #migrate() {
    this.database.exec(`
      CREATE TABLE IF NOT EXISTS settings (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL,
        updated_at TEXT NOT NULL
      );

      CREATE TABLE IF NOT EXISTS runs (
        id TEXT PRIMARY KEY,
        mode TEXT NOT NULL,
        source TEXT NOT NULL,
        intent TEXT NOT NULL,
        max_items INTEGER NOT NULL,
        scrolls INTEGER NOT NULL,
        status TEXT NOT NULL,
        provider TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        started_at TEXT,
        completed_at TEXT,
        coverage_json TEXT,
        result_json TEXT,
        error_json TEXT
      );

      CREATE TABLE IF NOT EXISTS bridge_commands (
        id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        type TEXT NOT NULL,
        payload_json TEXT NOT NULL,
        status TEXT NOT NULL,
        created_at TEXT NOT NULL,
        claimed_at TEXT,
        completed_at TEXT,
        bridge_id TEXT,
        error_json TEXT
      );

      CREATE INDEX IF NOT EXISTS bridge_commands_run_status
        ON bridge_commands(run_id, status, created_at);

      CREATE TABLE IF NOT EXISTS observations (
        id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        source TEXT NOT NULL,
        source_url TEXT NOT NULL,
        published_at TEXT,
        observed_at TEXT NOT NULL,
        payload_json TEXT NOT NULL,
        created_at TEXT NOT NULL
      );

      CREATE INDEX IF NOT EXISTS observations_run
        ON observations(run_id, created_at);

      CREATE TABLE IF NOT EXISTS feedback (
        id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        item_id TEXT,
        kind TEXT NOT NULL,
        note TEXT,
        created_at TEXT NOT NULL
      );
    `);
  }

  getOrCreateBridgeToken() {
    const existing = this.database
      .prepare("SELECT value FROM settings WHERE key = ?")
      .get("bridge_token");
    if (existing?.value) return existing.value;

    const token = randomBytes(32).toString("base64url");
    const now = new Date().toISOString();
    this.database
      .prepare("INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)")
      .run("bridge_token", token, now);
    return token;
  }

  matchesBridgeToken(candidate) {
    if (typeof candidate !== "string") return false;
    const expected = Buffer.from(this.getOrCreateBridgeToken());
    const received = Buffer.from(candidate);
    return expected.length === received.length && timingSafeEqual(expected, received);
  }

  createRun(run, provider) {
    const now = new Date().toISOString();
    this.database
      .prepare(`
        INSERT INTO runs(
          id, mode, source, intent, max_items, scrolls, status, provider,
          created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `)
      .run(
        run.id,
        run.mode,
        run.source,
        run.intent,
        run.maxItems,
        run.scrolls,
        "waiting_for_bridge",
        provider,
        now,
        now,
      );
    return this.getRun(run.id);
  }

  listRuns(limit = 20) {
    const rows = this.database
      .prepare("SELECT * FROM runs ORDER BY created_at DESC LIMIT ?")
      .all(limit);
    return rows.map(mapRun);
  }

  getRun(id) {
    const row = this.database.prepare("SELECT * FROM runs WHERE id = ?").get(id);
    if (!row) return null;
    const run = mapRun(row);
    run.observations = this.database
      .prepare("SELECT * FROM observations WHERE run_id = ? ORDER BY created_at ASC")
      .all(id)
      .map(mapObservation);
    run.feedback = this.database
      .prepare("SELECT * FROM feedback WHERE run_id = ? ORDER BY created_at ASC")
      .all(id)
      .map(mapFeedback);
    return run;
  }

  setRunStatus(id, status, extra = {}) {
    const now = new Date().toISOString();
    const startedAt = extra.startedAt ?? null;
    const completedAt = extra.completedAt ?? null;
    this.database
      .prepare(`
        UPDATE runs
        SET status = ?, updated_at = ?,
            started_at = COALESCE(?, started_at),
            completed_at = COALESCE(?, completed_at)
        WHERE id = ?
      `)
      .run(status, now, startedAt, completedAt, id);
    return this.getRun(id);
  }

  completeRun(id, result, coverage) {
    const now = new Date().toISOString();
    this.database
      .prepare(`
        UPDATE runs
        SET status = 'completed', updated_at = ?, completed_at = ?,
            result_json = ?, coverage_json = ?, error_json = NULL
        WHERE id = ?
      `)
      .run(now, now, JSON.stringify(result), JSON.stringify(coverage), id);
    return this.getRun(id);
  }

  failRun(id, stage, error) {
    const now = new Date().toISOString();
    const safeError = {
      stage,
      name: error?.name ?? "Error",
      message: String(error?.message ?? error ?? "Unknown error").slice(0, 2_000),
    };
    this.database
      .prepare(`
        UPDATE runs
        SET status = 'failed', updated_at = ?, completed_at = ?, error_json = ?
        WHERE id = ?
      `)
      .run(now, now, JSON.stringify(safeError), id);
    return this.getRun(id);
  }

  cancelRun(id) {
    const run = this.getRun(id);
    if (!run || ["completed", "failed", "cancelled"].includes(run.status)) return run;
    const now = new Date().toISOString();
    this.database.exec("BEGIN IMMEDIATE");
    try {
      this.database
        .prepare("UPDATE runs SET status = 'cancelled', updated_at = ?, completed_at = ? WHERE id = ?")
        .run(now, now, id);
      this.database
        .prepare("UPDATE bridge_commands SET status = 'cancelled', completed_at = ? WHERE run_id = ? AND status IN ('queued', 'claimed')")
        .run(now, id);
      this.database.exec("COMMIT");
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
    return this.getRun(id);
  }

  enqueueBridgeCommand(runId, type, payload) {
    const id = randomUUID();
    const now = new Date().toISOString();
    this.database
      .prepare(`
        INSERT INTO bridge_commands(
          id, run_id, type, payload_json, status, created_at
        ) VALUES (?, ?, ?, ?, 'queued', ?)
      `)
      .run(id, runId, type, JSON.stringify(payload), now);
    return this.getBridgeCommand(id);
  }

  getBridgeCommand(id) {
    const row = this.database
      .prepare("SELECT * FROM bridge_commands WHERE id = ?")
      .get(id);
    return row ? mapBridgeCommand(row) : null;
  }

  claimBridgeCommand(runId, bridgeId) {
    this.database.exec("BEGIN IMMEDIATE");
    try {
      const row = this.database
        .prepare(`
          SELECT * FROM bridge_commands
          WHERE run_id = ? AND status = 'queued'
          ORDER BY created_at ASC LIMIT 1
        `)
        .get(runId);
      if (!row) {
        this.database.exec("COMMIT");
        return null;
      }
      const now = new Date().toISOString();
      this.database
        .prepare(`
          UPDATE bridge_commands
          SET status = 'claimed', claimed_at = ?, bridge_id = ?
          WHERE id = ? AND status = 'queued'
        `)
        .run(now, bridgeId, row.id);
      this.database
        .prepare("UPDATE runs SET status = 'capturing', updated_at = ?, started_at = COALESCE(started_at, ?) WHERE id = ?")
        .run(now, now, runId);
      this.database.exec("COMMIT");
      return this.getBridgeCommand(row.id);
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
  }

  completeBridgeCommand(commandId) {
    const now = new Date().toISOString();
    this.database
      .prepare("UPDATE bridge_commands SET status = 'completed', completed_at = ? WHERE id = ?")
      .run(now, commandId);
    return this.getBridgeCommand(commandId);
  }

  failBridgeCommand(commandId, error) {
    const now = new Date().toISOString();
    const payload = JSON.stringify({ message: String(error?.message ?? error).slice(0, 2_000) });
    this.database
      .prepare("UPDATE bridge_commands SET status = 'failed', completed_at = ?, error_json = ? WHERE id = ?")
      .run(now, payload, commandId);
    return this.getBridgeCommand(commandId);
  }

  saveObservation(runId, observation) {
    const id = randomUUID();
    const now = new Date().toISOString();
    const firstPublishedAt = observation.snapshots
      .flatMap((snapshot) => snapshot.blocks)
      .map((block) => block.publishedAt)
      .find(Boolean);
    this.database
      .prepare(`
        INSERT INTO observations(
          id, run_id, source, source_url, published_at, observed_at,
          payload_json, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      `)
      .run(
        id,
        runId,
        observation.source,
        observation.pageUrl,
        firstPublishedAt ?? null,
        observation.capturedAt,
        JSON.stringify(observation),
        now,
      );
    return this.database
      .prepare("SELECT * FROM observations WHERE id = ?")
      .get(id);
  }

  addFeedback(runId, feedback) {
    const id = randomUUID();
    const now = new Date().toISOString();
    this.database
      .prepare(`
        INSERT INTO feedback(id, run_id, item_id, kind, note, created_at)
        VALUES (?, ?, ?, ?, ?, ?)
      `)
      .run(id, runId, feedback.itemId || null, feedback.kind, feedback.note || null, now);
    return this.getRun(runId);
  }
}

function mapRun(row) {
  return {
    id: row.id,
    mode: row.mode,
    source: row.source,
    intent: row.intent,
    maxItems: row.max_items,
    scrolls: row.scrolls,
    status: row.status,
    provider: row.provider,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
    startedAt: row.started_at,
    completedAt: row.completed_at,
    coverage: parseJson(row.coverage_json),
    result: parseJson(row.result_json),
    error: parseJson(row.error_json),
  };
}

function mapObservation(row) {
  return {
    id: row.id,
    source: row.source,
    sourceUrl: row.source_url,
    publishedAt: row.published_at,
    observedAt: row.observed_at,
    payload: parseJson(row.payload_json),
    createdAt: row.created_at,
  };
}

function mapFeedback(row) {
  return {
    id: row.id,
    itemId: row.item_id,
    kind: row.kind,
    note: row.note,
    createdAt: row.created_at,
  };
}

function mapBridgeCommand(row) {
  return {
    id: row.id,
    runId: row.run_id,
    type: row.type,
    payload: parseJson(row.payload_json),
    status: row.status,
    createdAt: row.created_at,
    claimedAt: row.claimed_at,
    completedAt: row.completed_at,
    bridgeId: row.bridge_id,
    error: parseJson(row.error_json),
  };
}

function parseJson(value) {
  if (!value) return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}
