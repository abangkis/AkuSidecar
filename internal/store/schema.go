package store

const SchemaVersion = 16

const schemaVersion = "16"

// memorySchemaSQL is deliberately kept separate from the operational schema.
// Personal Memory has no foreign keys into sessions, runs, or Timeline rows;
// retention and Full Reset therefore cannot accidentally orphan or resurrect
// a memory through an operational cascade.
const memorySchemaSQL = `
CREATE TABLE IF NOT EXISTS memory_items (
  id TEXT PRIMARY KEY,
  source TEXT NOT NULL DEFAULT '',
  identity_digest TEXT NOT NULL DEFAULT '',
  canonical_evidence_key TEXT NOT NULL DEFAULT '',
  canonical_permalink TEXT NOT NULL DEFAULT '',
  canonical_platform_id TEXT NOT NULL DEFAULT '',
  content_fingerprint TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  author TEXT NOT NULL DEFAULT '',
  published_at TEXT,
  tags_json TEXT NOT NULL DEFAULT '[]',
  facets_json TEXT NOT NULL DEFAULT '[]',
  media_metadata_json TEXT NOT NULL DEFAULT '[]',
  retention_tier TEXT NOT NULL CHECK (retention_tier IN ('recall','full_copy')),
  lifecycle_state TEXT NOT NULL CHECK (lifecycle_state IN ('active','tombstone')),
  full_content_version_id TEXT NOT NULL DEFAULT '',
  content_bytes INTEGER NOT NULL DEFAULT 0 CHECK (content_bytes >= 0),
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memory_items_identity_digest
  ON memory_items(identity_digest);
CREATE INDEX IF NOT EXISTS memory_items_lifecycle_updated
  ON memory_items(lifecycle_state, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS memory_items_source_updated
  ON memory_items(source, updated_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS memory_identity_aliases (
  source TEXT NOT NULL,
  alias_kind TEXT NOT NULL CHECK (alias_kind IN ('canonical_evidence_key','canonical_permalink','canonical_platform_id','content_fingerprint')),
  alias_value TEXT NOT NULL,
  memory_item_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  UNIQUE(source, alias_kind, alias_value, memory_item_id)
);

CREATE INDEX IF NOT EXISTS memory_identity_aliases_lookup
  ON memory_identity_aliases(source, alias_kind, alias_value);
CREATE UNIQUE INDEX IF NOT EXISTS memory_identity_aliases_strong_unique
  ON memory_identity_aliases(source, alias_kind, alias_value)
  WHERE alias_kind IN ('canonical_evidence_key','canonical_permalink','canonical_platform_id');
CREATE INDEX IF NOT EXISTS memory_identity_aliases_item
  ON memory_identity_aliases(memory_item_id, alias_kind);

-- Deleted-memory suppression keeps only keyed per-alias digests. Raw source,
-- URL, evidence key, and content are deliberately absent from this table.
CREATE TABLE IF NOT EXISTS memory_tombstone_aliases (
  memory_item_id TEXT NOT NULL,
  alias_kind TEXT NOT NULL CHECK (alias_kind IN ('canonical_evidence_key','canonical_permalink','canonical_platform_id','content_fingerprint')),
  alias_digest TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(memory_item_id, alias_kind, alias_digest)
);

CREATE INDEX IF NOT EXISTS memory_tombstone_aliases_lookup
  ON memory_tombstone_aliases(alias_kind, alias_digest);
CREATE INDEX IF NOT EXISTS memory_tombstone_aliases_item
  ON memory_tombstone_aliases(memory_item_id, alias_kind);

CREATE TABLE IF NOT EXISTS memory_content_versions (
  id TEXT PRIMARY KEY,
  memory_item_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version >= 1),
  content TEXT NOT NULL DEFAULT '',
  content_fingerprint TEXT NOT NULL DEFAULT '',
  media_metadata_json TEXT NOT NULL DEFAULT '[]',
  content_bytes INTEGER NOT NULL DEFAULT 0 CHECK (content_bytes >= 0),
  captured_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  released_at TEXT,
  UNIQUE(memory_item_id, version)
);

CREATE INDEX IF NOT EXISTS memory_content_versions_item_created
  ON memory_content_versions(memory_item_id, created_at DESC, version DESC);
CREATE INDEX IF NOT EXISTS memory_content_versions_active
  ON memory_content_versions(memory_item_id, released_at, version DESC);

CREATE TABLE IF NOT EXISTS memory_provenance (
  id TEXT PRIMARY KEY,
  memory_item_id TEXT NOT NULL,
  provenance_kind TEXT NOT NULL CHECK (provenance_kind IN ('captured','explicit_feedback','imported','manual','unknown')),
  source TEXT NOT NULL DEFAULT '',
  canonical_evidence_key TEXT NOT NULL DEFAULT '',
  source_url TEXT NOT NULL DEFAULT '',
  capture_context_json TEXT NOT NULL DEFAULT '{}',
  reason TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memory_provenance_item_created
  ON memory_provenance(memory_item_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS memory_actions (
  id TEXT PRIMARY KEY,
  memory_item_id TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('create_stub','update_stub','keep_full_copy','release_full_copy','read_later','mark_read','import','delete')),
  detail_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS memory_actions_item_created
  ON memory_actions(memory_item_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS memory_actions_action_created
  ON memory_actions(action, created_at DESC, id DESC);
`

// memoryRetentionSchemaSQL materializes current retention state separately
// from the append-only memory_actions audit. A resolved claim remains as a
// compact state row so repeated Read later/Done/Keep calls are idempotent,
// while action history is never consulted as current ownership.
const memoryRetentionSchemaSQL = `
CREATE TABLE IF NOT EXISTS memory_retention_claims (
  memory_item_id TEXT NOT NULL,
  claim_kind TEXT NOT NULL CHECK (claim_kind IN ('saved','keep')),
  claimed_at TEXT NOT NULL,
  resolved_at TEXT,
  PRIMARY KEY(memory_item_id, claim_kind)
);

CREATE INDEX IF NOT EXISTS memory_retention_claims_active
  ON memory_retention_claims(claim_kind, resolved_at, claimed_at DESC, memory_item_id);
CREATE INDEX IF NOT EXISTS memory_retention_claims_item
  ON memory_retention_claims(memory_item_id, claim_kind, resolved_at);
`

// memoryRetentionMigrationSQL intentionally omits IF NOT EXISTS. A v13
// database must not silently accept an object with the wrong retention-claim
// definition; the enclosing transaction must fail and preserve schema_version=13.
const memoryRetentionMigrationSQL = `
CREATE TABLE memory_retention_claims (
  memory_item_id TEXT NOT NULL,
  claim_kind TEXT NOT NULL CHECK (claim_kind IN ('saved','keep')),
  claimed_at TEXT NOT NULL,
  resolved_at TEXT,
  PRIMARY KEY(memory_item_id, claim_kind)
);

CREATE INDEX memory_retention_claims_active
  ON memory_retention_claims(claim_kind, resolved_at, claimed_at DESC, memory_item_id);
CREATE INDEX memory_retention_claims_item
  ON memory_retention_claims(memory_item_id, claim_kind, resolved_at);
`

// contentContextFeedbackSchemaSQL stores append-only, pairwise relevance
// decisions. It deliberately does not foreign-key Timeline or Memory rows:
// feedback is learning/audit evidence, while current retrieval still requires
// both authoritative objects to exist and remain eligible.
const contentContextFeedbackSchemaSQL = `
CREATE TABLE IF NOT EXISTS content_context_feedback_events (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL,
  context_key TEXT NOT NULL,
  memory_item_id TEXT NOT NULL,
  verdict TEXT NOT NULL CHECK (verdict IN ('relevant','not_relevant','clear')),
  engine_version TEXT NOT NULL,
  result_rank INTEGER NOT NULL CHECK (result_rank >= 1 AND result_rank <= 5),
  match_reason TEXT NOT NULL DEFAULT '',
  supersedes_id TEXT REFERENCES content_context_feedback_events(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS content_context_feedback_pair_created
  ON content_context_feedback_events(context_key,memory_item_id,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS content_context_feedback_timeline_created
  ON content_context_feedback_events(timeline_id,created_at DESC,id DESC);
`

// The v14-to-v15 migration intentionally omits IF NOT EXISTS so a conflicting
// object cannot be accepted while advancing the schema marker.
const contentContextFeedbackMigrationSQL = `
CREATE TABLE content_context_feedback_events (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL,
  context_key TEXT NOT NULL,
  memory_item_id TEXT NOT NULL,
  verdict TEXT NOT NULL CHECK (verdict IN ('relevant','not_relevant','clear')),
  engine_version TEXT NOT NULL,
  result_rank INTEGER NOT NULL CHECK (result_rank >= 1 AND result_rank <= 5),
  match_reason TEXT NOT NULL DEFAULT '',
  supersedes_id TEXT REFERENCES content_context_feedback_events(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX content_context_feedback_pair_created
  ON content_context_feedback_events(context_key,memory_item_id,created_at DESC,id DESC);
CREATE INDEX content_context_feedback_timeline_created
  ON content_context_feedback_events(timeline_id,created_at DESC,id DESC);
`

// livingTopicsSchemaSQL stores the bounded manual topic, membership, and
// append-only snapshot boundary. It never references operational session,
// run, Timeline, preference, or knowledge-event tables.
const livingTopicsSchemaSQL = `
CREATE TABLE IF NOT EXISTS living_topics (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS living_topics_updated
  ON living_topics(updated_at DESC,id DESC);

CREATE TABLE IF NOT EXISTS living_topic_memberships (
  topic_id TEXT NOT NULL,
  memory_item_id TEXT NOT NULL,
  added_at TEXT NOT NULL,
  PRIMARY KEY(topic_id,memory_item_id)
);

CREATE INDEX IF NOT EXISTS living_topic_memberships_memory
  ON living_topic_memberships(memory_item_id,topic_id);

CREATE TABLE IF NOT EXISTS living_topic_snapshots (
  id TEXT PRIMARY KEY,
  topic_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version >= 1),
  status TEXT NOT NULL CHECK (status IN ('ready','insufficient_evidence','no_change')),
  overview TEXT NOT NULL DEFAULT '',
  claims_json TEXT NOT NULL DEFAULT '[]',
  deltas_json TEXT NOT NULL DEFAULT '[]',
  evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  input_digest TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  effort TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  usage_json TEXT NOT NULL DEFAULT '{}',
  previous_snapshot_id TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(topic_id,version)
);

CREATE INDEX IF NOT EXISTS living_topic_snapshots_topic_created
  ON living_topic_snapshots(topic_id,created_at DESC,id DESC);
`

// A v15 database must fail closed when any canonical object already exists
// with an incompatible definition.
const livingTopicsMigrationSQL = `
CREATE TABLE living_topics (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX living_topics_updated ON living_topics(updated_at DESC,id DESC);
CREATE TABLE living_topic_memberships (
  topic_id TEXT NOT NULL,
  memory_item_id TEXT NOT NULL,
  added_at TEXT NOT NULL,
  PRIMARY KEY(topic_id,memory_item_id)
);
CREATE INDEX living_topic_memberships_memory ON living_topic_memberships(memory_item_id,topic_id);
CREATE TABLE living_topic_snapshots (
  id TEXT PRIMARY KEY,
  topic_id TEXT NOT NULL,
  version INTEGER NOT NULL CHECK (version >= 1),
  status TEXT NOT NULL CHECK (status IN ('ready','insufficient_evidence','no_change')),
  overview TEXT NOT NULL DEFAULT '',
  claims_json TEXT NOT NULL DEFAULT '[]',
  deltas_json TEXT NOT NULL DEFAULT '[]',
  evidence_ids_json TEXT NOT NULL DEFAULT '[]',
  input_digest TEXT NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  effort TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  usage_json TEXT NOT NULL DEFAULT '{}',
  previous_snapshot_id TEXT,
  created_at TEXT NOT NULL,
  UNIQUE(topic_id,version)
);
CREATE INDEX living_topic_snapshots_topic_created ON living_topic_snapshots(topic_id,created_at DESC,id DESC);
`

// memorySearchSchemaSQL is a local FTS5 index over active Personal Memory
// fields. It intentionally stores the item id as an unindexed lookup value;
// lifecycle state and public filtering remain authoritative in memory_items.
// The index is rebuilt transactionally on every memory lifecycle mutation.
const memorySearchSchemaSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS memory_search_fts USING fts5(
  memory_item_id UNINDEXED,
  title,
  summary,
  author,
  tags,
  facets,
  full_content,
  tokenize='unicode61'
);
`

// memorySearchMigrationSQL intentionally omits IF NOT EXISTS. A v12
// database must not silently accept an object with the wrong definition under
// the canonical FTS name; the enclosing transaction must fail and preserve
// schema_version=12 instead.
const memorySearchMigrationSQL = `
CREATE VIRTUAL TABLE memory_search_fts USING fts5(
  memory_item_id UNINDEXED,
  title,
  summary,
  author,
  tags,
  facets,
  full_content,
  tokenize='unicode61'
);
`

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

CREATE TABLE IF NOT EXISTS source_definitions (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL,
  ordinal INTEGER NOT NULL UNIQUE CHECK (ordinal >= 0),
  enabled INTEGER NOT NULL CHECK (enabled IN (0,1))
);

CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  intent TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','partial','failed','cancelled')),
  active_source TEXT REFERENCES source_definitions(id),
  max_items_per_source INTEGER NOT NULL CHECK (max_items_per_source BETWEEN 1 AND 15),
  max_items_total INTEGER NOT NULL CHECK (max_items_total BETWEEN 1 AND 30),
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  coverage_json TEXT NOT NULL DEFAULT '{}',
  error_json TEXT
);

CREATE INDEX IF NOT EXISTS sessions_status_created ON sessions(status, created_at DESC);

CREATE TABLE IF NOT EXISTS auto_update_batches (
  session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
  state TEXT NOT NULL CHECK (state IN ('preparing','prepared','visible','expired')),
  created_at TEXT NOT NULL,
  prepared_at TEXT,
  revealed_at TEXT,
  expires_at TEXT
);

CREATE INDEX IF NOT EXISTS auto_update_batches_state_prepared
  ON auto_update_batches(state, prepared_at);

CREATE TABLE IF NOT EXISTS auto_update_state (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_ui_access_at TEXT,
  last_attempt_at TEXT,
  last_success_at TEXT,
  last_error TEXT NOT NULL DEFAULT ''
);

INSERT OR IGNORE INTO auto_update_state(id) VALUES(1);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  source TEXT NOT NULL REFERENCES source_definitions(id),
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
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
  source TEXT NOT NULL REFERENCES source_definitions(id),
  observation_json TEXT NOT NULL,
  captured_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS observations_run_created ON observations(run_id, created_at);

CREATE TABLE IF NOT EXISTS capture_surface_events (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES runs(id) ON DELETE CASCADE,
  source TEXT REFERENCES source_definitions(id),
  event TEXT NOT NULL CHECK (event IN (
    'created','reused','release_requested','released',
    'preserved_user_owned','focus_intervention','reconciled'
  )),
  outcome TEXT NOT NULL DEFAULT '',
  detail_json TEXT NOT NULL DEFAULT '{}',
  occurred_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS capture_surface_events_run_occurred
  ON capture_surface_events(run_id, occurred_at);
CREATE INDEX IF NOT EXISTS capture_surface_events_session_source
  ON capture_surface_events(session_id, source, occurred_at);

CREATE TABLE IF NOT EXISTS content_continuity (
  source TEXT NOT NULL REFERENCES source_definitions(id),
  evidence_key TEXT NOT NULL,
  content_fingerprint TEXT NOT NULL,
  context_fingerprint TEXT NOT NULL DEFAULT '',
  engagement_score INTEGER NOT NULL DEFAULT 0 CHECK (engagement_score >= 0),
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  last_run_id TEXT NOT NULL,
  seen_count INTEGER NOT NULL CHECK (seen_count >= 1),
  PRIMARY KEY(source,evidence_key)
);

CREATE INDEX IF NOT EXISTS content_continuity_last_seen
  ON content_continuity(last_seen_at);

CREATE TABLE IF NOT EXISTS content_identity_aliases (
  source TEXT NOT NULL REFERENCES source_definitions(id),
  identity_fingerprint TEXT NOT NULL,
  canonical_evidence_key TEXT NOT NULL,
  canonical_platform_id TEXT NOT NULL DEFAULT '',
  canonical_permalink TEXT NOT NULL DEFAULT '',
  canonical_content_kind TEXT NOT NULL DEFAULT '',
  canonical_published_at TEXT NOT NULL DEFAULT '',
  ambiguous INTEGER NOT NULL DEFAULT 0 CHECK (ambiguous IN (0,1)),
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  last_run_id TEXT NOT NULL,
  seen_count INTEGER NOT NULL CHECK (seen_count >= 1),
  PRIMARY KEY(source,identity_fingerprint)
);

CREATE INDEX IF NOT EXISTS content_identity_aliases_last_seen
  ON content_identity_aliases(last_seen_at);

CREATE TABLE IF NOT EXISTS content_continuity_occurrences (
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('fresh','resurfaced_unchanged','resurfaced_changed','resurfaced_after_cooldown')),
  action TEXT NOT NULL CHECK (action IN ('evaluate','fail_fast')),
  previous_seen_at TEXT,
  observed_at TEXT NOT NULL,
  reason TEXT NOT NULL,
  PRIMARY KEY(run_id,evidence_key)
);

CREATE INDEX IF NOT EXISTS content_continuity_occurrences_status
  ON content_continuity_occurrences(run_id,status,action);

CREATE TABLE IF NOT EXISTS run_stage_timings (
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  stage TEXT NOT NULL CHECK (stage IN ('captured','evaluated','selected','added')),
  duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
  completed_at TEXT NOT NULL,
  PRIMARY KEY(run_id,stage)
);

CREATE TABLE IF NOT EXISTS reasoning_invocations (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  phase TEXT NOT NULL CHECK (phase IN ('acquisition_planning','candidate_evaluation')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  model_descriptor_version TEXT NOT NULL DEFAULT '',
  model_maturity TEXT NOT NULL DEFAULT '',
  effort TEXT NOT NULL,
  duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
  caller_latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (caller_latency_ms >= 0),
  queue_wait_ms INTEGER NOT NULL DEFAULT 0 CHECK (queue_wait_ms >= 0),
  provider_execution_ms INTEGER NOT NULL DEFAULT 0 CHECK (provider_execution_ms >= 0),
  response_total_ms INTEGER NOT NULL DEFAULT 0 CHECK (response_total_ms >= 0),
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
  source TEXT NOT NULL REFERENCES source_definitions(id),
  assessment_json TEXT NOT NULL,
  item_json TEXT NOT NULL DEFAULT '{}',
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
  source TEXT NOT NULL REFERENCES source_definitions(id),
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
  stage TEXT NOT NULL CHECK (stage IN ('fast','deep')),
  status TEXT NOT NULL CHECK (status IN ('strong_signals','insufficient_evidence','no_signal_detected','conflicting_evidence')),
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

CREATE TABLE IF NOT EXISTS ai_feedback_events (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  source TEXT NOT NULL REFERENCES source_definitions(id),
  target_type TEXT NOT NULL CHECK (target_type IN ('post','media','quote','account')),
  target_key TEXT NOT NULL,
  verdict TEXT NOT NULL CHECK (verdict IN ('ai','not_ai','unsure','clear')),
  signal_scope TEXT NOT NULL CHECK (signal_scope IN ('social_post','attached_media','quoted_post','author_account')),
  reason TEXT NOT NULL DEFAULT '' CHECK (reason IN ('','author_disclosed_ai','platform_label','content_credentials','account_identifies_as_agent','repeated_automated_pattern','signal_applies_to_another_object','known_human_authored','insufficient_evidence')),
  supersedes_id TEXT REFERENCES ai_feedback_events(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS ai_feedback_target_created
  ON ai_feedback_events(target_type,target_key,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ai_feedback_timeline_created
  ON ai_feedback_events(timeline_id,created_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS ai_feedback_session_created
  ON ai_feedback_events(session_id,created_at DESC,id DESC);

CREATE TABLE IF NOT EXISTS ai_detection_jobs (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed','cancelled')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  model_descriptor_version TEXT NOT NULL DEFAULT '',
  model_maturity TEXT NOT NULL DEFAULT '',
  effort TEXT NOT NULL,
  candidate_count INTEGER NOT NULL CHECK (candidate_count >= 0),
  duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  caller_latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (caller_latency_ms >= 0),
  queue_wait_ms INTEGER NOT NULL DEFAULT 0 CHECK (queue_wait_ms >= 0),
  provider_execution_ms INTEGER NOT NULL DEFAULT 0 CHECK (provider_execution_ms >= 0),
  response_total_ms INTEGER NOT NULL DEFAULT 0 CHECK (response_total_ms >= 0),
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

CREATE TABLE IF NOT EXISTS media_provenance_assessments (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  source TEXT NOT NULL REFERENCES source_definitions(id),
  media_index INTEGER NOT NULL CHECK (media_index >= 0),
  media_kind TEXT NOT NULL CHECK (media_kind IN ('image')),
  target_url TEXT NOT NULL,
  target_url_hash TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('queued','running','completed','failed','cancelled')),
  manifest_state TEXT NOT NULL CHECK (manifest_state IN ('pending','no_manifest','valid','invalid','unsupported','unavailable')),
  trust_state TEXT NOT NULL CHECK (trust_state IN ('pending','not_applicable','not_evaluated','trusted','untrusted')),
  ai_origin TEXT NOT NULL CHECK (ai_origin IN ('unknown','none','generated','edited')),
  evidence_json TEXT NOT NULL DEFAULT '[]',
  asset_sha256 TEXT NOT NULL DEFAULT '',
  provider TEXT NOT NULL,
  verifier_version TEXT NOT NULL,
  rationale TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
  error TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  UNIQUE(timeline_id,target_url_hash,verifier_version)
);

CREATE INDEX IF NOT EXISTS media_provenance_status_created
  ON media_provenance_assessments(status, created_at);
CREATE INDEX IF NOT EXISTS media_provenance_timeline_completed
  ON media_provenance_assessments(timeline_id, completed_at);

CREATE TABLE IF NOT EXISTS media_recaptures (
  id TEXT PRIMARY KEY,
  timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,
  source TEXT NOT NULL REFERENCES source_definitions(id),
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

-- Media-only social posts are held here until a pixel-capable evaluator is
-- available. The queue is source-scoped and intentionally independent from
-- Inbox presentation: Inbox only projects these durable states.
CREATE TABLE IF NOT EXISTS vision_evaluation_jobs (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  source TEXT NOT NULL REFERENCES source_definitions(id),
  evidence_key TEXT NOT NULL,
  canonical_identity TEXT NOT NULL,
  media_fingerprint TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','deferred','evaluating','retry_wait','ready','failed')),
  reason TEXT NOT NULL DEFAULT '',
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0 AND attempt_count <= 2),
  queued_at TEXT NOT NULL,
  next_attempt_at TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  candidate_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  started_at TEXT,
  completed_at TEXT,
  UNIQUE(source,canonical_identity,media_fingerprint)
);

CREATE INDEX IF NOT EXISTS vision_evaluation_queue_order
  ON vision_evaluation_jobs(source,status,queued_at,id);
CREATE INDEX IF NOT EXISTS vision_evaluation_run
  ON vision_evaluation_jobs(run_id,created_at);

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
  source TEXT NOT NULL REFERENCES source_definitions(id),
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
  timeline_id TEXT REFERENCES timeline_items(id) ON DELETE SET NULL,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  direction TEXT NOT NULL CHECK (direction IN ('more','less')),
  reason TEXT CHECK (reason = 'not_interested' OR reason IS NULL),
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS feedback_evidence_created ON feedback_events(evidence_key, created_at DESC);
CREATE INDEX IF NOT EXISTS feedback_timeline_created ON feedback_events(timeline_id, created_at DESC);

CREATE TABLE IF NOT EXISTS selection_corrections (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
  run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
  evidence_key TEXT NOT NULL,
  timeline_id TEXT REFERENCES timeline_items(id) ON DELETE SET NULL,
  action TEXT NOT NULL CHECK (action = 'should_select'),
  created_at TEXT NOT NULL,
  undone_at TEXT
);

CREATE INDEX IF NOT EXISTS selection_corrections_evidence_created
  ON selection_corrections(run_id,evidence_key,created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS selection_corrections_one_active
  ON selection_corrections(run_id,evidence_key) WHERE undone_at IS NULL;

CREATE TABLE IF NOT EXISTS preference_model (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  model_json TEXT NOT NULL,
  feedback_count INTEGER NOT NULL CHECK (feedback_count >= 0),
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS preference_learning_ledger (
  event_id TEXT PRIMARY KEY,
  source TEXT NOT NULL,
  evidence_key TEXT NOT NULL,
  direction TEXT NOT NULL CHECK (direction IN ('more','neutral','less')),
  reason TEXT,
  origin TEXT NOT NULL CHECK (origin IN ('routine','calibration','selection_correction')),
  created_at TEXT NOT NULL,
  assessment_json TEXT NOT NULL,
  active INTEGER NOT NULL DEFAULT 1 CHECK (active IN (0,1))
);

CREATE INDEX IF NOT EXISTS preference_learning_effective
  ON preference_learning_ledger(active,source,evidence_key,created_at);

CREATE TABLE IF NOT EXISTS knowledge_events (
  id TEXT PRIMARY KEY,
  source TEXT NOT NULL REFERENCES source_definitions(id),
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
  source TEXT NOT NULL REFERENCES source_definitions(id),
  relation TEXT NOT NULL CHECK (relation IN ('new_event','duplicate_report','material_update','contradiction','new_consequence','context_only')),
  confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  reason TEXT NOT NULL DEFAULT '',
  corrected INTEGER NOT NULL DEFAULT 0 CHECK (corrected IN (0,1)),
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS semantic_reports_event_created ON semantic_event_reports(event_id, created_at DESC);
CREATE INDEX IF NOT EXISTS semantic_reports_session_relation ON semantic_event_reports(session_id, relation);

CREATE TABLE IF NOT EXISTS semantic_event_deltas (
  id TEXT PRIMARY KEY,
  event_id TEXT NOT NULL REFERENCES semantic_events(id) ON DELETE CASCADE,
  report_id TEXT REFERENCES semantic_event_reports(id) ON DELETE SET NULL,
  fingerprint TEXT NOT NULL,
  claim TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('material_update','contradiction','new_consequence','context_only')),
  source TEXT NOT NULL REFERENCES source_definitions(id),
  evidence_key TEXT NOT NULL,
  confidence REAL NOT NULL CHECK (confidence BETWEEN 0 AND 1),
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  UNIQUE(event_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS semantic_event_deltas_event_seen ON semantic_event_deltas(event_id, last_seen_at DESC);

CREATE TABLE IF NOT EXISTS semantic_event_constraints (
  evidence_key TEXT NOT NULL,
  event_id TEXT NOT NULL REFERENCES semantic_events(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('must_merge','must_not_merge')),
  created_at TEXT NOT NULL,
  PRIMARY KEY(evidence_key, event_id)
);

CREATE TABLE IF NOT EXISTS semantic_novelty_constraints (
  evidence_key TEXT NOT NULL,
  event_id TEXT NOT NULL REFERENCES semantic_events(id) ON DELETE CASCADE,
  relation TEXT NOT NULL CHECK (relation IN ('duplicate_report','material_update')),
  created_at TEXT NOT NULL,
  PRIMARY KEY(evidence_key, event_id)
);

CREATE TABLE IF NOT EXISTS event_resolution_invocations (
  session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
  status TEXT NOT NULL CHECK (status IN ('completed','failed','bypassed')),
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  model_descriptor_version TEXT NOT NULL DEFAULT '',
  model_maturity TEXT NOT NULL DEFAULT '',
  effort TEXT NOT NULL,
  candidate_count INTEGER NOT NULL CHECK (candidate_count >= 0),
  shortlist_count INTEGER NOT NULL CHECK (shortlist_count >= 0),
  unique_items INTEGER NOT NULL CHECK (unique_items >= 0),
  duplicate_reports INTEGER NOT NULL CHECK (duplicate_reports >= 0),
  duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
  caller_latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (caller_latency_ms >= 0),
  queue_wait_ms INTEGER NOT NULL DEFAULT 0 CHECK (queue_wait_ms >= 0),
  provider_execution_ms INTEGER NOT NULL DEFAULT 0 CHECK (provider_execution_ms >= 0),
  response_total_ms INTEGER NOT NULL DEFAULT 0 CHECK (response_total_ms >= 0),
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
  trigger_tokens_json TEXT NOT NULL DEFAULT '[]',
  receipt_json TEXT NOT NULL DEFAULT '{}'
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
` + memorySchemaSQL + memorySearchSchemaSQL + memoryRetentionSchemaSQL + contentContextFeedbackSchemaSQL + livingTopicsSchemaSQL
