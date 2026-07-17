package store

const schemaVersion = "4"

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

CREATE TABLE IF NOT EXISTS ai_assessments (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  stage TEXT NOT NULL CHECK (stage IN ('fast','deep','user')),
  status TEXT NOT NULL CHECK (status IN ('strong_signals','insufficient_evidence','no_signal_detected','conflicting_evidence','user_marked_ai','user_marked_not_ai')),
  confidence_band TEXT NOT NULL CHECK (confidence_band IN ('low','medium','high')),
  evidence_json TEXT NOT NULL DEFAULT '[]',
  assessed_object TEXT NOT NULL CHECK (assessed_object IN ('social_post')),
  signal_scope TEXT NOT NULL CHECK (signal_scope IN ('social_post','quoted_post','external_artifact','attached_media','none','mixed')),
  provider TEXT NOT NULL,
  detector_version TEXT NOT NULL,
  content_fingerprint TEXT NOT NULL DEFAULT '',
  rationale TEXT NOT NULL DEFAULT '',
  supersedes_id TEXT REFERENCES ai_assessments(id),
  created_at TEXT NOT NULL,
  undone_at TEXT
);

CREATE INDEX IF NOT EXISTS ai_assessments_timeline_created ON ai_assessments(timeline_id, created_at, id);
CREATE INDEX IF NOT EXISTS ai_assessments_session_stage ON ai_assessments(session_id, stage, created_at);
CREATE INDEX IF NOT EXISTS ai_assessments_fingerprint ON ai_assessments(content_fingerprint, stage, created_at);

CREATE TABLE IF NOT EXISTS ai_detection_jobs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed','cancelled')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  effort TEXT NOT NULL,
  candidate_count INTEGER NOT NULL CHECK (candidate_count >= 0),
  duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  input_tokens INTEGER,
  cached_input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_output_tokens INTEGER,
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT
);

CREATE INDEX IF NOT EXISTS ai_detection_jobs_status_created ON ai_detection_jobs(status, created_at);

CREATE TABLE IF NOT EXISTS media_recaptures (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  target_url TEXT NOT NULL,
  evidence_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','claimed','completed','failed')),
  outcome TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL,
  result_json TEXT,
  claimed_by TEXT,
  created_at TEXT NOT NULL,
  claimed_at TEXT,
  completed_at TEXT,
  error_json TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS media_recaptures_one_active
  ON media_recaptures(timeline_id) WHERE status IN ('queued','claimed');

CREATE TABLE IF NOT EXISTS timeline_evidence_overrides (
  timeline_id TEXT PRIMARY KEY REFERENCES timeline_items(id) ON DELETE CASCADE,
  recapture_id TEXT NOT NULL REFERENCES media_recaptures(id) ON DELETE CASCADE,
  evidence_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS calibration_sessions (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL UNIQUE REFERENCES sessions(id) ON DELETE CASCADE,
  trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('first_run','manual','source_added','drift','random_audit')),
  status TEXT NOT NULL CHECK (status IN ('reviewing','completed')),
  max_items INTEGER NOT NULL CHECK (max_items BETWEEN 2 AND 10),
  sample_count INTEGER NOT NULL CHECK (sample_count BETWEEN 1 AND 10),
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  completed_at TEXT
);

CREATE TABLE IF NOT EXISTS calibration_samples (
  calibration_session_id TEXT NOT NULL REFERENCES calibration_sessions(id) ON DELETE CASCADE,
  ordinal INTEGER NOT NULL CHECK (ordinal BETWEEN 0 AND 9),
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  candidate_json TEXT NOT NULL,
  label TEXT CHECK (label IN ('more_like_this','neutral','less_like_this') OR label IS NULL),
  issue_code TEXT CHECK (issue_code IN ('capture_incomplete','wrong_source','duplicate','formatting') OR issue_code IS NULL),
  labeled_at TEXT,
  PRIMARY KEY(calibration_session_id, ordinal),
  UNIQUE(calibration_session_id, run_id, evidence_key),
  CHECK (label IS NULL OR issue_code IS NULL)
);

CREATE INDEX IF NOT EXISTS calibration_samples_resolution
  ON calibration_samples(calibration_session_id, label, issue_code, ordinal);

CREATE TABLE IF NOT EXISTS calibration_profile_snapshots (
  id TEXT PRIMARY KEY,
  calibration_session_id TEXT NOT NULL UNIQUE REFERENCES calibration_sessions(id) ON DELETE CASCADE,
  snapshot_json TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS feedback_events (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  direction TEXT NOT NULL CHECK (direction IN ('more','less')),
  reason TEXT CHECK (reason = 'not_interested' OR reason IS NULL),
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

CREATE TABLE IF NOT EXISTS semantic_events (
  id TEXT PRIMARY KEY,
  canonical_claim TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL DEFAULT '',
  object TEXT NOT NULL DEFAULT '',
  event_kind TEXT NOT NULL DEFAULT 'other',
  event_start TEXT,
  event_end TEXT,
  aliases_json TEXT NOT NULL DEFAULT '[]',
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS semantic_events_last_seen ON semantic_events(last_seen_at DESC);

CREATE TABLE IF NOT EXISTS semantic_event_reports (
  id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL REFERENCES semantic_events(id) ON DELETE CASCADE,
  timeline_id TEXT NOT NULL UNIQUE REFERENCES timeline_items(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  source TEXT NOT NULL CHECK (source IN ('x','linkedin')),
  relation TEXT NOT NULL CHECK (relation IN ('new_event','duplicate_report','material_update','contradiction','new_consequence','context_only')),
  confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  reason TEXT NOT NULL DEFAULT '',
  corrected INTEGER NOT NULL DEFAULT 0 CHECK (corrected IN (0,1)),
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS semantic_reports_event_created ON semantic_event_reports(event_id, created_at DESC);
CREATE INDEX IF NOT EXISTS semantic_reports_session_relation ON semantic_event_reports(session_id, relation);

CREATE TABLE IF NOT EXISTS semantic_event_constraints (
  evidence_key TEXT NOT NULL,
  event_id TEXT NOT NULL REFERENCES semantic_events(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('must_merge','must_not_merge')),
  created_at TEXT NOT NULL,
  PRIMARY KEY(evidence_key, event_id)
);

CREATE TABLE IF NOT EXISTS event_resolution_invocations (
  session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('completed','failed','bypassed')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  effort TEXT NOT NULL,
  candidate_count INTEGER NOT NULL CHECK (candidate_count >= 0),
  shortlist_count INTEGER NOT NULL CHECK (shortlist_count >= 0),
  unique_items INTEGER NOT NULL CHECK (unique_items >= 0),
  duplicate_reports INTEGER NOT NULL CHECK (duplicate_reports >= 0),
  duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
  input_tokens INTEGER,
  cached_input_tokens INTEGER,
  output_tokens INTEGER,
  reasoning_output_tokens INTEGER,
  error_json TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS event_resolution_diagnostics (
  session_id TEXT PRIMARY KEY REFERENCES event_resolution_invocations(session_id) ON DELETE CASCADE,
  historical_event_count INTEGER NOT NULL CHECK (historical_event_count >= 0),
  resolver_invoked INTEGER NOT NULL CHECK (resolver_invoked IN (0,1)),
  trigger_reason TEXT NOT NULL,
  strongest_overlap INTEGER NOT NULL CHECK (strongest_overlap >= 0),
  trigger_tokens_json TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS semantic_event_corrections (
  id TEXT PRIMARY KEY,
  report_id TEXT NOT NULL REFERENCES semantic_event_reports(id) ON DELETE CASCADE,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  action TEXT NOT NULL CHECK (action IN ('not_same_event','same_event')),
  from_event_id TEXT NOT NULL,
  from_relation TEXT NOT NULL,
  to_event_id TEXT NOT NULL,
  to_relation TEXT NOT NULL,
  created_at TEXT NOT NULL,
  undone_at TEXT
);

CREATE INDEX IF NOT EXISTS semantic_corrections_timeline_created ON semantic_event_corrections(timeline_id, created_at DESC);
`
