package store

import (
	"context"
	"database/sql"
	"fmt"
)

func migrateSchema7To8(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"reasoning_invocations", "event_resolution_invocations", "ai_detection_jobs"} {
		for _, column := range []string{"caller_latency_ms", "queue_wait_ms", "provider_execution_ms", "response_total_ms"} {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s INTEGER NOT NULL DEFAULT 0 CHECK (%s >= 0)", table, column, column)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='8' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema8To9(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"reasoning_invocations", "event_resolution_invocations", "ai_detection_jobs"} {
		for _, column := range []string{"model_descriptor_version", "model_maturity"} {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s TEXT NOT NULL DEFAULT ''", table, column)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='9' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema9To10(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var tableCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='event_resolution_diagnostics'`).Scan(&tableCount); err != nil {
		return err
	}
	if tableCount > 0 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE event_resolution_diagnostics ADD COLUMN receipt_json TEXT NOT NULL DEFAULT '{}'`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='10' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema10To11(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
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
CREATE INDEX IF NOT EXISTS vision_evaluation_queue_order ON vision_evaluation_jobs(source,status,queued_at,id);
CREATE INDEX IF NOT EXISTS vision_evaluation_run ON vision_evaluation_jobs(run_id,created_at);`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='11' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema11To12(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, memorySchemaSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='12' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema12To13(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, memorySearchMigrationSQL); err != nil {
		return err
	}
	if err := backfillMemorySearchIndexTx(ctx, tx); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='13' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema13To14(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, memoryRetentionMigrationSQL); err != nil {
		return err
	}
	// Existing full copies were already an explicit durable retention choice
	// before current Saved membership existed. Preserve that ownership without
	// inferring Saved from historical read_later/mark_read actions.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at,resolved_at)
		SELECT id,'keep',COALESCE(updated_at,created_at),NULL
		FROM memory_items
		WHERE lifecycle_state='active' AND retention_tier='full_copy'
		ON CONFLICT(memory_item_id,claim_kind) DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='14' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema14To15(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, contentContextFeedbackMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='15' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema15To16(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='16' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema16To17(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsRoutingMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='17' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema17To18(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsUnderstandingMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='18' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema18To19(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsActivationMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='19' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema19To20(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsNewEvidenceMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='20' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema20To21(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsMembershipMoveMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='21' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema21To22(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, livingTopicsModelUsageMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='22' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema22To23(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	columns := []struct{ name, definition string }{
		{"evidence_roles_json", "TEXT NOT NULL DEFAULT '[]'"},
		{"contract_version", "TEXT NOT NULL DEFAULT 'legacy-v1'"},
		{"material_change", "INTEGER NOT NULL DEFAULT 1 CHECK (material_change IN (0,1))"},
		{"coverage_state", "TEXT NOT NULL DEFAULT 'legacy'"},
	}
	for _, column := range columns {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('living_topic_snapshots') WHERE name=?`, column.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE living_topic_snapshots ADD COLUMN %s %s", column.name, column.definition)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, livingTopicsCurrentProjectionMigrationSQL); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='23' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateSchema23To24(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	columns := []struct{ name, definition string }{
		{"source_diversity_state", "TEXT NOT NULL DEFAULT 'unknown' CHECK (source_diversity_state IN ('unknown','single_platform','limited','diverse'))"},
		{"source_platform_count", "INTEGER NOT NULL DEFAULT 0 CHECK (source_platform_count >= 0)"},
		{"independent_source_count", "INTEGER NOT NULL DEFAULT 0 CHECK (independent_source_count >= 0)"},
		{"independent_cluster_count", "INTEGER NOT NULL DEFAULT 0 CHECK (independent_cluster_count >= 0)"},
	}
	for _, column := range columns {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('living_topic_snapshots') WHERE name=?`, column.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			if _, err := tx.ExecContext(ctx, fmt.Sprintf("ALTER TABLE living_topic_snapshots ADD COLUMN %s %s", column.name, column.definition)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE living_topics
		SET understanding_status='pending', understanding_input_digest='', understanding_trigger='migration_rebaseline_v3', understanding_last_error=''
		WHERE EXISTS (SELECT 1 FROM living_topic_memberships m WHERE m.topic_id=living_topics.id);
		INSERT INTO living_topic_understanding_jobs(id,topic_id,status,trigger,queued_at)
		SELECT 'topic_understanding_rebaseline_v3:' || t.id,t.id,'pending','migration_rebaseline_v3',strftime('%Y-%m-%dT%H:%M:%fZ','now')
		FROM living_topics t
		WHERE EXISTS (SELECT 1 FROM living_topic_memberships m WHERE m.topic_id=t.id)
		  AND NOT EXISTS (SELECT 1 FROM living_topic_understanding_jobs j WHERE j.topic_id=t.id AND j.status IN ('pending','running'))
		ON CONFLICT(id) DO NOTHING`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET value='24' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}

// V4 stores publication coverage separately from evaluation time. Previous
// snapshots are immutable; pending work is coalesced into one rebaseline.
func migrateSchema24To25(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('living_topic_snapshots') WHERE name='evidence_as_of'`).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE living_topic_snapshots ADD COLUMN evidence_as_of TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE living_topic_understanding_jobs SET trigger='migration_rebaseline_v4'
		WHERE status='pending';
		UPDATE living_topics
		SET understanding_status='pending',understanding_input_digest='',understanding_trigger='migration_rebaseline_v4',understanding_last_error=''
		WHERE EXISTS (SELECT 1 FROM living_topic_memberships m WHERE m.topic_id=living_topics.id);
		INSERT INTO living_topic_understanding_jobs(id,topic_id,status,trigger,queued_at)
		SELECT 'topic_understanding_rebaseline_v4:' || t.id,t.id,'pending','migration_rebaseline_v4',strftime('%Y-%m-%dT%H:%M:%fZ','now')
		FROM living_topics t
		WHERE EXISTS (SELECT 1 FROM living_topic_memberships m WHERE m.topic_id=t.id)
		  AND NOT EXISTS (SELECT 1 FROM living_topic_understanding_jobs j WHERE j.topic_id=t.id AND j.status IN ('pending','running'))
		ON CONFLICT(id) DO NOTHING;
		UPDATE meta SET value='25' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}
