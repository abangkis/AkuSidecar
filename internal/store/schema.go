package store

const schemaVersion = "1"

const schemaSQL = `
PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  intent TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','partial','failed','cancelled')),
  active_source TEXT CHECK (active_source IN ('x','linkedin') OR active_source IS NULL),
  max_items_per_source INTEGER NOT NULL CHECK (max_items_per_source BETWEEN 1 AND 15),
  max_items_total INTEGER NOT NULL CHECK (max_items_total BETWEEN 1 AND 30),
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  coverage_json TEXT NOT NULL DEFAULT '{}',
  error_json TEXT
);

CREATE INDEX IF NOT EXISTS sessions_status_created ON sessions(status, created_at DESC);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  ordinal INTEGER NOT NULL CHECK (ordinal IN (0,1)),
  status TEXT NOT NULL CHECK (status IN ('queued','waiting_for_bridge','reasoning','completed','failed','cancelled')),
  stage TEXT NOT NULL,
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  summary TEXT NOT NULL DEFAULT '',
  coverage_json TEXT NOT NULL DEFAULT '{}',
  error_json TEXT,
  UNIQUE(session_id, source),
  UNIQUE(session_id, ordinal)
);

CREATE INDEX IF NOT EXISTS runs_session_ordinal ON runs(session_id, ordinal);
CREATE INDEX IF NOT EXISTS runs_status_created ON runs(status, created_at);

CREATE TABLE IF NOT EXISTS bridge_commands (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK (type IN ('collect_visible','release_capture')),
  status TEXT NOT NULL CHECK (status IN ('queued','claimed','completed','failed','cancelled')),
  payload_json TEXT NOT NULL,
  claimed_by TEXT,
  created_at TEXT NOT NULL,
  claimed_at TEXT,
  completed_at TEXT,
  error_json TEXT
);

CREATE INDEX IF NOT EXISTS bridge_commands_run_status ON bridge_commands(run_id, status, created_at);

CREATE TABLE IF NOT EXISTS observations (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  command_id TEXT NOT NULL UNIQUE REFERENCES bridge_commands(id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  observation_json TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS observations_run_created ON observations(run_id, created_at);

CREATE TABLE IF NOT EXISTS reasoning_invocations (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  phase TEXT NOT NULL CHECK (phase IN ('acquisition_planning','candidate_evaluation')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  effort TEXT NOT NULL,
  duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
  status TEXT NOT NULL CHECK (status IN ('completed','failed','cancelled')),
  input_tokens INTEGER,
  cached_input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_output_tokens INTEGER,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS reasoning_run_created ON reasoning_invocations(run_id, created_at);

CREATE TABLE IF NOT EXISTS candidate_assessments (
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  assessment_json TEXT NOT NULL,
  base_score REAL NOT NULL,
  preference_score REAL NOT NULL,
  final_score REAL NOT NULL,
  selected INTEGER NOT NULL CHECK (selected IN (0,1)),
  created_at TEXT NOT NULL,
  PRIMARY KEY(run_id, evidence_key)
);

CREATE TABLE IF NOT EXISTS timeline_items (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  evidence_key TEXT NOT NULL,
  rank INTEGER NOT NULL CHECK (rank >= 0),
  item_json TEXT NOT NULL,
  assessment_json TEXT NOT NULL,
  coverage_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  UNIQUE(run_id, evidence_key)
);

CREATE INDEX IF NOT EXISTS timeline_session_rank ON timeline_items(session_id, rank);
CREATE INDEX IF NOT EXISTS timeline_created ON timeline_items(created_at DESC);

CREATE TABLE IF NOT EXISTS feedback_events (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  direction TEXT NOT NULL CHECK (direction IN ('more','less')),
  reason TEXT CHECK (reason IN ('not_interested','already_knew','old_info','duplicate') OR reason IS NULL),
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS feedback_evidence_created ON feedback_events(evidence_key, created_at DESC);

CREATE TABLE IF NOT EXISTS preference_model (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  model_json TEXT NOT NULL,
  feedback_count INTEGER NOT NULL CHECK (feedback_count >= 0),
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS knowledge_events (
  id TEXT PRIMARY KEY,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  event_key TEXT NOT NULL,
  evidence_key TEXT NOT NULL,
  item_json TEXT NOT NULL,
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  UNIQUE(source, event_key)
);

CREATE INDEX IF NOT EXISTS knowledge_source_last_seen ON knowledge_events(source, last_seen_at DESC);
`
