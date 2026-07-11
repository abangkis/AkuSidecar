import fs from "node:fs";
import path from "node:path";
import { randomBytes, randomUUID, timingSafeEqual } from "node:crypto";
import { DatabaseSync } from "node:sqlite";
import { intentKeyForText, uniqueEvidenceKeys } from "../core/knowledge-continuity.mjs";

export class SqliteStateStore {
  constructor(databasePath) {
    fs.mkdirSync(path.dirname(databasePath), { recursive: true });
    this.databasePath = databasePath;
    this.database = new DatabaseSync(databasePath);
    this.database.exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;");
    this.#migrate();
    this.#backfillConfirmedExclusions();
    this.#initializePilotReviewStart();
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

      CREATE TABLE IF NOT EXISTS unified_sessions (
        id TEXT PRIMARY KEY,
        mode TEXT NOT NULL,
        intent TEXT NOT NULL,
        sources_json TEXT NOT NULL,
        max_items_per_source INTEGER NOT NULL,
        max_items_total INTEGER NOT NULL,
        status TEXT NOT NULL,
        active_source TEXT,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        started_at TEXT,
        completed_at TEXT,
        result_json TEXT,
        coverage_json TEXT
      );

      CREATE TABLE IF NOT EXISTS unified_session_children (
        session_id TEXT NOT NULL REFERENCES unified_sessions(id) ON DELETE CASCADE,
        source TEXT NOT NULL,
        ordinal INTEGER NOT NULL,
        run_id TEXT UNIQUE REFERENCES runs(id) ON DELETE SET NULL,
        status TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        PRIMARY KEY(session_id, source),
        UNIQUE(session_id, ordinal)
      );

      CREATE INDEX IF NOT EXISTS unified_sessions_status
        ON unified_sessions(status, updated_at);

      CREATE INDEX IF NOT EXISTS unified_session_children_run
        ON unified_session_children(run_id);

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

      CREATE INDEX IF NOT EXISTS feedback_run_created
        ON feedback(run_id, created_at);

      CREATE TABLE IF NOT EXISTS candidate_evaluations (
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        evidence_key TEXT NOT NULL,
        source TEXT NOT NULL,
        decision TEXT NOT NULL,
        reason_code TEXT NOT NULL,
        item_id TEXT,
        author TEXT NOT NULL,
        text TEXT NOT NULL,
        source_url TEXT NOT NULL,
        published_at TEXT,
        feed_position INTEGER,
        policy_version TEXT NOT NULL,
        preference_profile_version INTEGER NOT NULL,
        created_at TEXT NOT NULL,
        PRIMARY KEY(run_id, evidence_key)
      );

      CREATE INDEX IF NOT EXISTS candidate_evaluations_run_decision
        ON candidate_evaluations(run_id, decision, feed_position);

      CREATE TABLE IF NOT EXISTS preference_feedback_events (
        id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        evidence_key TEXT NOT NULL,
        kind TEXT NOT NULL,
        reason_code TEXT,
        note TEXT,
        created_at TEXT NOT NULL
      );

      CREATE INDEX IF NOT EXISTS preference_feedback_run_created
        ON preference_feedback_events(run_id, created_at);

      CREATE TABLE IF NOT EXISTS reasoning_invocations (
        id TEXT PRIMARY KEY,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        phase TEXT NOT NULL,
        provider TEXT NOT NULL,
        model TEXT,
        reasoning_effort TEXT,
        duration_ms INTEGER NOT NULL,
        status TEXT NOT NULL,
        input_tokens INTEGER,
        cached_input_tokens INTEGER,
        output_tokens INTEGER,
        reasoning_output_tokens INTEGER,
        created_at TEXT NOT NULL
      );

      CREATE INDEX IF NOT EXISTS reasoning_invocations_run_created
        ON reasoning_invocations(run_id, created_at);

      CREATE TABLE IF NOT EXISTS checkpoints (
        source TEXT NOT NULL,
        mode TEXT NOT NULL,
        run_id TEXT NOT NULL,
        observed_at TEXT NOT NULL,
        candidate_count INTEGER NOT NULL,
        result_count INTEGER NOT NULL,
        updated_at TEXT NOT NULL,
        PRIMARY KEY(source, mode)
      );

      CREATE TABLE IF NOT EXISTS knowledge_events (
        id TEXT PRIMARY KEY,
        source TEXT NOT NULL,
        mode TEXT NOT NULL,
        event_key TEXT NOT NULL,
        current_version_id TEXT,
        first_seen_at TEXT NOT NULL,
        updated_at TEXT NOT NULL,
        UNIQUE(source, mode, event_key)
      );

      CREATE TABLE IF NOT EXISTS knowledge_versions (
        id TEXT PRIMARY KEY,
        event_id TEXT NOT NULL REFERENCES knowledge_events(id) ON DELETE CASCADE,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        item_id TEXT NOT NULL,
        source TEXT NOT NULL,
        mode TEXT NOT NULL,
        evidence_key TEXT NOT NULL,
        source_url TEXT NOT NULL,
        source_url_kind TEXT NOT NULL,
        knowledge_delta TEXT NOT NULL,
        claim TEXT NOT NULL,
        published_at TEXT,
        observed_at TEXT NOT NULL,
        result_json TEXT NOT NULL,
        created_at TEXT NOT NULL,
        UNIQUE(source, mode, evidence_key)
      );

      CREATE INDEX IF NOT EXISTS knowledge_versions_event
        ON knowledge_versions(event_id, created_at);

      CREATE INDEX IF NOT EXISTS knowledge_versions_source_mode
        ON knowledge_versions(source, mode, created_at);

      CREATE TABLE IF NOT EXISTS evidence_dispositions (
        source TEXT NOT NULL,
        mode TEXT NOT NULL,
        intent_key TEXT NOT NULL,
        intent_text TEXT NOT NULL,
        evidence_key TEXT NOT NULL,
        disposition TEXT NOT NULL,
        run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
        created_at TEXT NOT NULL,
        PRIMARY KEY(source, mode, intent_key, evidence_key)
      );

      CREATE INDEX IF NOT EXISTS evidence_dispositions_scope
        ON evidence_dispositions(source, mode, intent_key, disposition);
    `);
    this.#ensureColumn("candidate_evaluations", "assessment_json", "TEXT");
    this.database
      .prepare("DELETE FROM preference_feedback_events WHERE kind NOT IN ('more_like_this', 'less_like_this')")
      .run();
  }

  #ensureColumn(table, column, definition) {
    const columns = this.database.prepare(`PRAGMA table_info(${table})`).all();
    if (!columns.some((entry) => entry.name === column)) {
      this.database.exec(`ALTER TABLE ${table} ADD COLUMN ${column} ${definition}`);
    }
  }

  #backfillConfirmedExclusions() {
    const rows = this.database
      .prepare("SELECT DISTINCT run_id, created_at FROM feedback WHERE kind = 'correct_empty'")
      .all();
    for (const row of rows) {
      this.#persistConfirmedExclusions(row.run_id, row.created_at);
    }
  }

  #persistConfirmedExclusions(runId, createdAt) {
    const run = this.getRun(runId);
    if (!run || run.status !== "completed" || (run.result?.items?.length ?? 0) !== 0) {
      return;
    }
    const intentKey = intentKeyForText(run.intent);
    const evidenceKeys = [
      ...new Set(run.observations.flatMap((entry) => uniqueEvidenceKeys(entry.payload))),
    ];
    const insert = this.database.prepare(`
      INSERT INTO evidence_dispositions(
        source, mode, intent_key, intent_text, evidence_key,
        disposition, run_id, created_at
      ) VALUES (?, ?, ?, ?, ?, 'confirmed_excluded', ?, ?)
      ON CONFLICT(source, mode, intent_key, evidence_key) DO UPDATE SET
        disposition = excluded.disposition,
        run_id = excluded.run_id,
        created_at = excluded.created_at
    `);
    for (const evidenceKey of evidenceKeys) {
      insert.run(
        run.source,
        run.mode,
        intentKey,
        run.intent,
        evidenceKey,
        runId,
        createdAt,
      );
    }
  }

  #initializePilotReviewStart() {
    const existing = this.getSetting("pilot_review_started_at");
    if (existing) return;
    const row = this.database
      .prepare(`
        SELECT MIN(r.created_at) AS started_at
        FROM runs r
        JOIN feedback f ON f.run_id = r.id
      `)
      .get();
    if (row?.started_at) this.setSetting("pilot_review_started_at", row.started_at);
  }

  getSetting(key) {
    return this.database.prepare("SELECT value FROM settings WHERE key = ?").get(key)?.value ?? null;
  }

  setSetting(key, value) {
    const now = new Date().toISOString();
    this.database
      .prepare(`
        INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
      `)
      .run(key, value, now);
  }

  getPilotReviewStartedAt() {
    return this.getSetting("pilot_review_started_at");
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

  createUnifiedSession(session) {
    const now = new Date().toISOString();
    this.database.exec("BEGIN IMMEDIATE");
    try {
      this.database
        .prepare(`
          INSERT INTO unified_sessions(
            id, mode, intent, sources_json, max_items_per_source, max_items_total,
            status, created_at, updated_at
          ) VALUES (?, ?, ?, ?, ?, ?, 'queued', ?, ?)
        `)
        .run(
          session.id,
          session.mode,
          session.intent,
          JSON.stringify(session.sources),
          session.maxItemsPerSource,
          session.maxItemsTotal,
          now,
          now,
        );
      const insertChild = this.database.prepare(`
        INSERT INTO unified_session_children(
          session_id, source, ordinal, status, created_at, updated_at
        ) VALUES (?, ?, ?, 'queued', ?, ?)
      `);
      session.sources.forEach((source, ordinal) => {
        insertChild.run(session.id, source, ordinal, now, now);
      });
      this.database.exec("COMMIT");
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
    return this.getUnifiedSession(session.id);
  }

  getUnifiedSession(id) {
    const row = this.database
      .prepare("SELECT * FROM unified_sessions WHERE id = ?")
      .get(id);
    if (!row) return null;
    const session = mapUnifiedSession(row);
    session.children = this.database
      .prepare("SELECT * FROM unified_session_children WHERE session_id = ? ORDER BY ordinal ASC")
      .all(id)
      .map((childRow) => {
        const child = mapUnifiedSessionChild(childRow);
        child.run = child.runId ? this.getRun(child.runId) : null;
        return child;
      });
    return session;
  }

  getUnifiedSessionByRunId(runId) {
    const row = this.database
      .prepare("SELECT session_id FROM unified_session_children WHERE run_id = ?")
      .get(runId);
    return row ? this.getUnifiedSession(row.session_id) : null;
  }

  listOpenUnifiedSessions() {
    return this.database
      .prepare("SELECT id FROM unified_sessions WHERE status IN ('queued', 'running') ORDER BY created_at ASC")
      .all()
      .map((row) => this.getUnifiedSession(row.id));
  }

  attachUnifiedSessionChild(sessionId, source, runId) {
    const now = new Date().toISOString();
    this.database.exec("BEGIN IMMEDIATE");
    try {
      this.database
        .prepare(`
          UPDATE unified_session_children
          SET run_id = ?, status = 'waiting_for_bridge', updated_at = ?
          WHERE session_id = ? AND source = ? AND status = 'queued'
        `)
        .run(runId, now, sessionId, source);
      this.database
        .prepare(`
          UPDATE unified_sessions
          SET status = 'running', active_source = ?, updated_at = ?,
              started_at = COALESCE(started_at, ?)
          WHERE id = ?
        `)
        .run(source, now, now, sessionId);
      this.database.exec("COMMIT");
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
    return this.getUnifiedSession(sessionId);
  }

  setUnifiedSessionChildStatus(sessionId, source, status) {
    const now = new Date().toISOString();
    this.database
      .prepare(`
        UPDATE unified_session_children SET status = ?, updated_at = ?
        WHERE session_id = ? AND source = ?
      `)
      .run(status, now, sessionId, source);
    return this.getUnifiedSession(sessionId);
  }

  completeUnifiedSession(id, status, result, coverage) {
    const now = new Date().toISOString();
    this.database
      .prepare(`
        UPDATE unified_sessions
        SET status = ?, active_source = NULL, updated_at = ?, completed_at = ?,
            result_json = ?, coverage_json = ?
        WHERE id = ?
      `)
      .run(status, now, now, JSON.stringify(result), JSON.stringify(coverage), id);
    return this.getUnifiedSession(id);
  }

  cancelUnifiedSession(id, status, result, coverage) {
    const now = new Date().toISOString();
    this.database.exec("BEGIN IMMEDIATE");
    try {
      this.database
        .prepare(`
          UPDATE unified_session_children
          SET status = 'cancelled', updated_at = ?
          WHERE session_id = ? AND status = 'queued'
        `)
        .run(now, id);
      this.database
        .prepare(`
          UPDATE unified_sessions
          SET status = ?, active_source = NULL, updated_at = ?, completed_at = ?,
              result_json = ?, coverage_json = ?
          WHERE id = ?
        `)
        .run(status, now, now, JSON.stringify(result), JSON.stringify(coverage), id);
      this.database.exec("COMMIT");
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
    return this.getUnifiedSession(id);
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

  listRunsWithFeedback(limit = 501) {
    const boundedLimit = Math.max(1, Math.min(501, Number.isFinite(limit) ? limit : 501));
    const runs = this.database
      .prepare("SELECT * FROM runs ORDER BY created_at DESC LIMIT ?")
      .all(boundedLimit)
      .map(mapRun);
    if (runs.length === 0) return runs;
    const placeholders = runs.map(() => "?").join(", ");
    const feedbackRows = this.database
      .prepare(`SELECT * FROM feedback WHERE run_id IN (${placeholders}) ORDER BY created_at ASC`)
      .all(...runs.map((run) => run.id));
    const feedbackByRun = new Map();
    for (const row of feedbackRows) {
      const entries = feedbackByRun.get(row.run_id) ?? [];
      entries.push(mapFeedback(row));
      feedbackByRun.set(row.run_id, entries);
    }
    for (const run of runs) run.feedback = feedbackByRun.get(run.id) ?? [];
    const candidateRows = this.database
      .prepare(`SELECT * FROM candidate_evaluations WHERE run_id IN (${placeholders}) ORDER BY feed_position ASC, created_at ASC`)
      .all(...runs.map((run) => run.id));
    const candidatesByRun = groupRows(candidateRows, "run_id", mapCandidateEvaluation);
    const preferenceRows = this.database
      .prepare(`SELECT * FROM preference_feedback_events WHERE run_id IN (${placeholders}) ORDER BY created_at ASC, id ASC`)
      .all(...runs.map((run) => run.id));
    const preferenceByRun = groupRows(preferenceRows, "run_id", mapPreferenceFeedback);
    const invocationRows = this.database
      .prepare(`SELECT * FROM reasoning_invocations WHERE run_id IN (${placeholders}) ORDER BY created_at ASC`)
      .all(...runs.map((run) => run.id));
    const invocationsByRun = groupRows(invocationRows, "run_id", mapReasoningInvocation);
    const sessionRows = this.database
      .prepare(`
        SELECT c.run_id, s.id AS session_id, s.created_at AS session_created_at
        FROM unified_session_children c
        JOIN unified_sessions s ON s.id = c.session_id
        WHERE c.run_id IN (${placeholders})
      `)
      .all(...runs.map((run) => run.id));
    const sessionByRun = new Map(sessionRows.map((row) => [row.run_id, row]));
    for (const run of runs) {
      run.candidateEvaluations = candidatesByRun.get(run.id) ?? [];
      run.preferenceFeedback = preferenceByRun.get(run.id) ?? [];
      run.reasoningInvocations = invocationsByRun.get(run.id) ?? [];
      run.unifiedSessionId = sessionByRun.get(run.id)?.session_id ?? null;
      run.unifiedSessionCreatedAt = sessionByRun.get(run.id)?.session_created_at ?? null;
    }
    return runs;
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
    run.candidateEvaluations = this.database
      .prepare("SELECT * FROM candidate_evaluations WHERE run_id = ? ORDER BY feed_position ASC, created_at ASC")
      .all(id)
      .map(mapCandidateEvaluation);
    run.preferenceFeedback = this.database
      .prepare("SELECT * FROM preference_feedback_events WHERE run_id = ? ORDER BY created_at ASC, id ASC")
      .all(id)
      .map(mapPreferenceFeedback);
    run.reasoningInvocations = this.database
      .prepare("SELECT * FROM reasoning_invocations WHERE run_id = ? ORDER BY created_at ASC")
      .all(id)
      .map(mapReasoningInvocation);
    const session = this.database
      .prepare(`
        SELECT s.id AS session_id, s.created_at AS session_created_at
        FROM unified_session_children c
        JOIN unified_sessions s ON s.id = c.session_id
        WHERE c.run_id = ?
      `)
      .get(id);
    run.unifiedSessionId = session?.session_id ?? null;
    run.unifiedSessionCreatedAt = session?.session_created_at ?? null;
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

  completeRunWithKnowledge(id, result, coverage, candidateEvaluations = []) {
    const run = this.getRun(id);
    if (!run) throw new Error("run not found");
    const now = new Date().toISOString();
    this.database.exec("BEGIN IMMEDIATE");
    try {
      this.database
        .prepare(`
          UPDATE runs
          SET status = 'completed', updated_at = ?, completed_at = ?,
              result_json = ?, coverage_json = ?, error_json = NULL
          WHERE id = ?
        `)
        .run(now, now, JSON.stringify(result), JSON.stringify(coverage), id);

      for (const item of result.items) {
        let event = this.database
          .prepare("SELECT * FROM knowledge_events WHERE source = ? AND mode = ? AND event_key = ?")
          .get(run.source, run.mode, item.eventKey);
        if (!event) {
          const eventId = randomUUID();
          this.database
            .prepare(`
              INSERT INTO knowledge_events(
                id, source, mode, event_key, first_seen_at, updated_at
              ) VALUES (?, ?, ?, ?, ?, ?)
            `)
            .run(eventId, run.source, run.mode, item.eventKey, now, now);
          event = { id: eventId };
        }

        const versionId = randomUUID();
        this.database
          .prepare(`
            INSERT INTO knowledge_versions(
              id, event_id, run_id, item_id, source, mode, evidence_key,
              source_url, source_url_kind, knowledge_delta, claim,
              published_at, observed_at, result_json, created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
          `)
          .run(
            versionId,
            event.id,
            id,
            item.id,
            run.source,
            run.mode,
            item.evidenceKey,
            item.sourceUrl,
            item.sourceUrlKind,
            item.knowledgeDelta,
            item.whatChanged,
            item.publishedAt,
            coverage.checkedThrough ?? now,
            JSON.stringify(item),
            now,
          );
        this.database
          .prepare("UPDATE knowledge_events SET current_version_id = ?, updated_at = ? WHERE id = ?")
          .run(versionId, now, event.id);
      }

      const insertCandidate = this.database.prepare(`
        INSERT INTO candidate_evaluations(
          run_id, evidence_key, source, decision, reason_code, item_id,
          author, text, source_url, published_at, feed_position,
          policy_version, preference_profile_version, assessment_json, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(run_id, evidence_key) DO UPDATE SET
          decision = excluded.decision,
          reason_code = excluded.reason_code,
          item_id = excluded.item_id,
          policy_version = excluded.policy_version,
          preference_profile_version = excluded.preference_profile_version,
          assessment_json = excluded.assessment_json
      `);
      for (const candidate of candidateEvaluations) {
        insertCandidate.run(
          id,
          candidate.evidenceKey,
          candidate.source,
          candidate.decision,
          candidate.reasonCode,
          candidate.itemId,
          candidate.author,
          candidate.text,
          candidate.sourceUrl,
          candidate.publishedAt,
          candidate.feedPosition,
          candidate.policyVersion,
          candidate.preferenceProfileVersion,
          candidate.assessment ? JSON.stringify(candidate.assessment) : null,
          now,
        );
      }

      this.database
        .prepare(`
          INSERT INTO checkpoints(
            source, mode, run_id, observed_at, candidate_count, result_count, updated_at
          ) VALUES (?, ?, ?, ?, ?, ?, ?)
          ON CONFLICT(source, mode) DO UPDATE SET
            run_id = excluded.run_id,
            observed_at = excluded.observed_at,
            candidate_count = excluded.candidate_count,
            result_count = excluded.result_count,
            updated_at = excluded.updated_at
        `)
        .run(
          run.source,
          run.mode,
          id,
          coverage.checkedThrough ?? now,
          coverage.candidateCount ?? 0,
          result.items.length,
          now,
        );
      this.database.exec("COMMIT");
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
    return this.getRun(id);
  }

  getCheckpoint(source, mode) {
    const row = this.database
      .prepare("SELECT * FROM checkpoints WHERE source = ? AND mode = ?")
      .get(source, mode);
    return row ? mapCheckpoint(row) : null;
  }

  getKnownEvidenceKeys(source, mode, evidenceKeys) {
    const unique = [...new Set(evidenceKeys)].filter(Boolean);
    if (unique.length === 0) return new Set();
    const placeholders = unique.map(() => "?").join(", ");
    const rows = this.database
      .prepare(`
        SELECT evidence_key FROM knowledge_versions
        WHERE source = ? AND mode = ? AND evidence_key IN (${placeholders})
      `)
      .all(source, mode, ...unique);
    return new Set(rows.map((row) => row.evidence_key));
  }

  getConfirmedExcludedEvidenceKeys(source, mode, intent, evidenceKeys) {
    const unique = [...new Set(evidenceKeys)].filter(Boolean);
    const intentKey = intentKeyForText(intent);
    if (unique.length === 0 || !intentKey) return new Set();
    const placeholders = unique.map(() => "?").join(", ");
    const rows = this.database
      .prepare(`
        SELECT evidence_key FROM evidence_dispositions
        WHERE source = ? AND mode = ? AND intent_key = ?
          AND disposition = 'confirmed_excluded'
          AND evidence_key IN (${placeholders})
      `)
      .all(source, mode, intentKey, ...unique);
    return new Set(rows.map((row) => row.evidence_key));
  }

  getKnowledgeContext(source, mode, limit = 20) {
    const numericLimit = Number.isFinite(limit) ? Math.trunc(limit) : 20;
    const boundedLimit = Math.max(1, Math.min(100, numericLimit));
    const rows = this.database
      .prepare(`
        SELECT e.event_key, e.first_seen_at, e.updated_at,
               v.evidence_key, v.knowledge_delta, v.claim, v.source_url,
               v.source_url_kind, v.published_at, v.observed_at
        FROM knowledge_events e
        JOIN knowledge_versions v ON v.id = e.current_version_id
        WHERE e.source = ? AND e.mode = ?
        ORDER BY e.updated_at DESC
        LIMIT ?
      `)
      .all(source, mode, boundedLimit);
    return {
      checkpoint: this.getCheckpoint(source, mode),
      events: rows.map(mapKnowledgeEvent),
    };
  }

  getKnowledgeEventHistory(source, mode, eventKey, limit = 50) {
    const numericLimit = Number.isFinite(limit) ? Math.trunc(limit) : 50;
    const boundedLimit = Math.max(1, Math.min(200, numericLimit));
    const rows = this.database
      .prepare(`
        SELECT v.* FROM knowledge_versions v
        JOIN knowledge_events e ON e.id = v.event_id
        WHERE e.source = ? AND e.mode = ? AND e.event_key = ?
        ORDER BY v.created_at ASC
        LIMIT ?
      `)
      .all(source, mode, eventKey, boundedLimit);
    return rows.map(mapKnowledgeVersion);
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

  getLatestBridgeCommandForRun(runId) {
    const row = this.database
      .prepare("SELECT * FROM bridge_commands WHERE run_id = ? ORDER BY created_at DESC LIMIT 1")
      .get(runId);
    return row ? mapBridgeCommand(row) : null;
  }

  getPendingBridgeCommandForRun(runId) {
    const row = this.database
      .prepare(`
        SELECT * FROM bridge_commands
        WHERE run_id = ? AND status IN ('queued', 'claimed')
        ORDER BY created_at DESC LIMIT 1
      `)
      .get(runId);
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
    this.database.exec("BEGIN IMMEDIATE");
    try {
      this.database
        .prepare(`
          INSERT INTO feedback(id, run_id, item_id, kind, note, created_at)
          VALUES (?, ?, ?, ?, ?, ?)
        `)
        .run(id, runId, feedback.itemId || null, feedback.kind, feedback.note || null, now);

      if (feedback.kind === "correct_empty") {
        this.#persistConfirmedExclusions(runId, now);
      }
      if (!this.getSetting("pilot_review_started_at")) {
        const run = this.getRun(runId);
        this.setSetting("pilot_review_started_at", run.createdAt);
      }
      this.database.exec("COMMIT");
    } catch (error) {
      this.database.exec("ROLLBACK");
      throw error;
    }
    return this.getRun(runId);
  }

  addPreferenceFeedback(runId, feedback) {
    const latest = this.database
      .prepare(`SELECT * FROM preference_feedback_events WHERE run_id = ? AND evidence_key = ? ORDER BY created_at DESC, id DESC LIMIT 1`)
      .get(runId, feedback.evidenceKey);
    if (
      latest &&
      latest.kind === feedback.kind &&
      (latest.reason_code ?? null) === feedback.reasonCode &&
      (latest.note ?? "") === feedback.note
    ) {
      return this.getRun(runId);
    }
    const now = new Date().toISOString();
    this.database
      .prepare(`INSERT INTO preference_feedback_events(id, run_id, evidence_key, kind, reason_code, note, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
      .run(randomUUID(), runId, feedback.evidenceKey, feedback.kind, feedback.reasonCode, feedback.note, now);
    return this.getRun(runId);
  }

  saveReasoningInvocation(telemetry) {
    this.database
      .prepare(`
        INSERT INTO reasoning_invocations(
          id, run_id, phase, provider, model, reasoning_effort, duration_ms,
          status, input_tokens, cached_input_tokens, output_tokens,
          reasoning_output_tokens, created_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      `)
      .run(
        randomUUID(), telemetry.runId, telemetry.phase, telemetry.provider,
        telemetry.model, telemetry.reasoningEffort, telemetry.durationMs,
        telemetry.status, telemetry.inputTokens, telemetry.cachedInputTokens,
        telemetry.outputTokens, telemetry.reasoningOutputTokens,
        new Date().toISOString(),
      );
  }

  getPreferenceProfile() {
    const row = this.database.prepare(`
      WITH ranked AS (
        SELECT p.*,
               ROW_NUMBER() OVER (
                 PARTITION BY p.run_id, p.evidence_key
                 ORDER BY p.created_at DESC, p.id DESC
               ) AS preference_rank
        FROM preference_feedback_events p
      ), effective AS (
        SELECT * FROM ranked WHERE preference_rank = 1
      )
      SELECT COUNT(*) AS total,
             SUM(CASE WHEN p.kind = 'more_like_this' THEN 1 ELSE 0 END) AS more_like_this,
             SUM(CASE WHEN p.kind = 'less_like_this' THEN 1 ELSE 0 END) AS less_like_this,
             SUM(CASE WHEN p.kind = 'more_like_this' AND c.decision = 'selected' THEN 1 ELSE 0 END) AS selected_more_like_this,
             SUM(CASE WHEN p.kind = 'more_like_this' AND c.decision = 'excluded' THEN 1 ELSE 0 END) AS excluded_more_like_this,
             MAX(p.created_at) AS updated_at
      FROM effective p
      LEFT JOIN candidate_evaluations c
        ON c.run_id = p.run_id AND c.evidence_key = p.evidence_key
    `).get();
    return {
      version: 0,
      status: "collecting",
      feedbackEventCount: Number(row.total ?? 0),
      moreLikeThisCount: Number(row.more_like_this ?? 0),
      lessLikeThisCount: Number(row.less_like_this ?? 0),
      selectedMoreLikeThisCount: Number(row.selected_more_like_this ?? 0),
      excludedMoreLikeThisCount: Number(row.excluded_more_like_this ?? 0),
      updatedAt: row.updated_at ?? null,
    };
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

function mapUnifiedSession(row) {
  return {
    id: row.id,
    mode: row.mode,
    intent: row.intent,
    sources: parseJson(row.sources_json) ?? [],
    maxItemsPerSource: row.max_items_per_source,
    maxItemsTotal: row.max_items_total,
    status: row.status,
    activeSource: row.active_source,
    createdAt: row.created_at,
    updatedAt: row.updated_at,
    startedAt: row.started_at,
    completedAt: row.completed_at,
    result: parseJson(row.result_json),
    coverage: parseJson(row.coverage_json),
    children: [],
  };
}

function mapUnifiedSessionChild(row) {
  return {
    source: row.source,
    ordinal: row.ordinal,
    runId: row.run_id,
    status: row.status,
    run: null,
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

function mapCandidateEvaluation(row) {
  return {
    runId: row.run_id,
    evidenceKey: row.evidence_key,
    source: row.source,
    decision: row.decision,
    reasonCode: row.reason_code,
    itemId: row.item_id,
    author: row.author,
    text: row.text,
    sourceUrl: row.source_url,
    publishedAt: row.published_at,
    feedPosition: row.feed_position,
    policyVersion: row.policy_version,
    preferenceProfileVersion: row.preference_profile_version,
    assessment: parseJson(row.assessment_json),
    createdAt: row.created_at,
  };
}

function mapPreferenceFeedback(row) {
  return {
    id: row.id,
    evidenceKey: row.evidence_key,
    kind: row.kind,
    reasonCode: row.reason_code,
    note: row.note,
    createdAt: row.created_at,
  };
}

function mapReasoningInvocation(row) {
  return {
    id: row.id,
    runId: row.run_id,
    phase: row.phase,
    provider: row.provider,
    model: row.model,
    reasoningEffort: row.reasoning_effort,
    durationMs: row.duration_ms,
    status: row.status,
    inputTokens: row.input_tokens,
    cachedInputTokens: row.cached_input_tokens,
    outputTokens: row.output_tokens,
    reasoningOutputTokens: row.reasoning_output_tokens,
    createdAt: row.created_at,
  };
}

function groupRows(rows, key, mapper) {
  const grouped = new Map();
  for (const row of rows) {
    const entries = grouped.get(row[key]) ?? [];
    entries.push(mapper(row));
    grouped.set(row[key], entries);
  }
  return grouped;
}

function mapCheckpoint(row) {
  return {
    source: row.source,
    mode: row.mode,
    runId: row.run_id,
    observedAt: row.observed_at,
    candidateCount: row.candidate_count,
    resultCount: row.result_count,
    updatedAt: row.updated_at,
  };
}

function mapKnowledgeEvent(row) {
  return {
    eventKey: row.event_key,
    firstSeenAt: row.first_seen_at,
    updatedAt: row.updated_at,
    evidenceKey: row.evidence_key,
    knowledgeDelta: row.knowledge_delta,
    claim: row.claim,
    sourceUrl: row.source_url,
    sourceUrlKind: row.source_url_kind,
    publishedAt: row.published_at,
    observedAt: row.observed_at,
  };
}

function mapKnowledgeVersion(row) {
  return {
    id: row.id,
    runId: row.run_id,
    itemId: row.item_id,
    evidenceKey: row.evidence_key,
    sourceUrl: row.source_url,
    sourceUrlKind: row.source_url_kind,
    knowledgeDelta: row.knowledge_delta,
    claim: row.claim,
    publishedAt: row.published_at,
    observedAt: row.observed_at,
    item: parseJson(row.result_json),
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
