package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type Store struct {
	db    *sql.DB
	path  string
	clock Clock
}

type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func Open(path string, defaults domain.Settings) (*Store, error) {
	return OpenWithClock(path, defaults, systemClock{})
}

// OpenReadOnly opens an existing database without running initialization,
// migrations, retention, or any other write path. It is intended for bounded
// diagnostics and export tools. Callers must not use mutating Store methods
// on the returned handle.
func OpenReadOnly(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("read-only database path is required")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, clock: systemClock{}}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite read-only: %w", err)
	}
	return store, nil
}

func OpenWithClock(path string, defaults domain.Settings, clock Clock) (*Store, error) {
	if clock == nil {
		return nil, errors.New("store clock is required")
	}
	if err := validateDataRuntimeVersion(path, domain.ApplicationVersion); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, clock: clock}
	if err := store.initialize(defaults); err != nil {
		db.Close()
		return nil, err
	}
	if err := writeDataRuntimeVersion(path, domain.ApplicationVersion); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Now() time.Time {
	if s.clock == nil {
		return time.Now()
	}
	return s.clock.Now()
}

func (s *Store) initialize(defaults domain.Settings) error {
	ctx := context.Background()
	var metaTable int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='meta'`).Scan(&metaTable); err != nil {
		return fmt.Errorf("inspect database schema: %w", err)
	}
	if metaTable != 0 {
		var version string
		if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
		if version == "7" {
			if err := migrateSchema7To8(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 7 to 8: %w", err)
			}
			version = "8"
		}
		if version == "8" {
			if err := migrateSchema8To9(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 8 to 9: %w", err)
			}
			version = "9"
		}
		if version == "9" {
			if err := migrateSchema9To10(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 9 to 10: %w", err)
			}
			version = "10"
		}
		if version == "10" {
			if err := migrateSchema10To11(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 10 to 11: %w", err)
			}
			version = "11"
		}
		if version == "11" {
			if err := migrateSchema11To12(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 11 to 12: %w", err)
			}
			version = "12"
		}
		if version == "12" {
			if err := migrateSchema12To13(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 12 to 13: %w", err)
			}
			version = "13"
		}
		if version == "13" {
			if err := migrateSchema13To14(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 13 to 14: %w", err)
			}
			version = "14"
		}
		if version == "14" {
			if err := migrateSchema14To15(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 14 to 15: %w", err)
			}
			version = "15"
		}
		if version == "15" {
			if err := migrateSchema15To16(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 15 to 16: %w", err)
			}
			version = "16"
		}
		if version == "16" {
			if err := migrateSchema16To17(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 16 to 17: %w", err)
			}
			version = "17"
		}
		if version == "17" {
			if err := migrateSchema17To18(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 17 to 18: %w", err)
			}
			version = "18"
		}
		if version == "18" {
			if err := migrateSchema18To19(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 18 to 19: %w", err)
			}
			version = "19"
		}
		if version == "19" {
			if err := migrateSchema19To20(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 19 to 20: %w", err)
			}
			version = "20"
		}
		if version == "20" {
			if err := migrateSchema20To21(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 20 to 21: %w", err)
			}
			version = "21"
		}
		if version == "21" {
			if err := migrateSchema21To22(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 21 to 22: %w", err)
			}
			version = "22"
		}
		if version == "22" {
			if err := migrateSchema22To23(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 22 to 23: %w", err)
			}
			version = "23"
		}
		if version == "23" {
			if err := migrateSchema23To24(ctx, s.db); err != nil {
				return fmt.Errorf("migrate schema 23 to 24: %w", err)
			}
			version = schemaVersion
		}
		if version != schemaVersion {
			return fmt.Errorf("database schema %s is incompatible with required schema %s; start with a fresh database", version, schemaVersion)
		}
	}
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}
	if err := s.syncSourceDefinitions(ctx); err != nil {
		return err
	}
	if err := s.backfillSemanticEventDeltas(ctx); err != nil {
		return fmt.Errorf("backfill semantic event deltas: %w", err)
	}
	if err := s.backfillContentContinuity(ctx); err != nil {
		return fmt.Errorf("backfill content continuity: %w", err)
	}
	if err := s.backfillContentIdentityAliases(ctx); err != nil {
		return fmt.Errorf("backfill content identity aliases: %w", err)
	}
	if metaTable == 0 {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version',?)`, schemaVersion); err != nil {
			return fmt.Errorf("save schema version: %w", err)
		}
	}
	if _, err := s.bridgeToken(ctx); err != nil {
		return err
	}
	if _, err := s.memoryTombstoneKey(ctx); err != nil {
		return fmt.Errorf("initialize memory tombstone key: %w", err)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key='runtime'`).Scan(&count); err != nil {
		return err
	}
	fresh := count == 0
	if fresh {
		if err := s.SaveSettings(ctx, defaults); err != nil {
			return err
		}
	} else {
		settings, err := s.GetSettings(ctx)
		if err != nil {
			return err
		}
		adoptInstagram, err := s.shouldAdoptInstagramDefault(ctx, settings.ActiveSources)
		if err != nil {
			return err
		}
		if adoptInstagram {
			settings.ActiveSources = append(settings.ActiveSources, domain.SourceInstagram)
		}
		if err := s.SaveSettings(ctx, settings); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('source_default_instagram_v1','applied') ON CONFLICT(key) DO NOTHING`); err != nil {
		return fmt.Errorf("record Instagram default migration: %w", err)
	}
	if err := s.initializeOnboarding(ctx, fresh); err != nil {
		return err
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	_, err = s.EnforceRetention(ctx, settings)
	return err
}

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

func (s *Store) shouldAdoptInstagramDefault(ctx context.Context, active []domain.Source) (bool, error) {
	var marker string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='source_default_instagram_v1'`).Scan(&marker)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read Instagram default migration: %w", err)
	}
	return len(active) == 3 &&
		active[0] == domain.SourceX &&
		active[1] == domain.SourceLinkedIn &&
		active[2] == domain.SourceFacebook, nil
}

func (s *Store) syncSourceDefinitions(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source registry sync: %w", err)
	}
	defer tx.Rollback()
	for ordinal, descriptor := range domain.Sources() {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_definitions(id,display_name,ordinal,enabled)
			VALUES(?,?,?,1)
			ON CONFLICT(id) DO UPDATE SET
				display_name=excluded.display_name,
				ordinal=excluded.ordinal,
				enabled=excluded.enabled`, descriptor.ID, descriptor.DisplayName, ordinal); err != nil {
			return fmt.Errorf("sync source definition %s: %w", descriptor.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source registry sync: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }

func (s *Store) BridgeToken(ctx context.Context) (string, error) { return s.bridgeToken(ctx) }

func (s *Store) bridgeToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='bridge_token'`).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read bridge token: %w", err)
	}
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate bridge token: %w", err)
	}
	token = hex.EncodeToString(value[:])
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('bridge_token',?)`, token); err != nil {
		return "", fmt.Errorf("save bridge token: %w", err)
	}
	return token, nil
}

func (s *Store) MatchesBridgeToken(ctx context.Context, candidate string) bool {
	expected, err := s.bridgeToken(ctx)
	if err != nil || len(candidate) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1
}

func (s *Store) GetSettings(ctx context.Context) (domain.Settings, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value_json FROM settings WHERE key='runtime'`).Scan(&raw); err != nil {
		return domain.Settings{}, fmt.Errorf("read settings: %w", err)
	}
	settings := domain.Settings{
		AIDetectionEnabled:    domain.DefaultAIDetectionEnabled,
		ResurfaceMode:         domain.DefaultResurfaceMode,
		ResurfaceCooldownDays: domain.DefaultResurfaceCooldownDays,
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return domain.Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		return domain.Settings{}, fmt.Errorf("validate settings: %w", err)
	}
	return settings, nil
}

func (s *Store) SaveSettings(ctx context.Context, settings domain.Settings) error {
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value_json,updated_at) VALUES('runtime',?,?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, string(raw), domain.Now())
	return err
}

func (s *Store) initializeOnboarding(ctx context.Context, fresh bool) error {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='onboarding_status'`).Scan(&status)
	if err == nil {
		if status != "not_started" && status != "completed" {
			return fmt.Errorf("invalid onboarding status %q", status)
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read onboarding status: %w", err)
	}
	status = "completed"
	if fresh {
		status = "not_started"
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('onboarding_status',?)`, status); err != nil {
		return fmt.Errorf("save onboarding status: %w", err)
	}
	if status == "completed" {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('onboarding_completed_at',?)`, domain.Now()); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Onboarding(ctx context.Context) (domain.OnboardingState, error) {
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='onboarding_status'`).Scan(&status); err != nil {
		return domain.OnboardingState{}, fmt.Errorf("read onboarding status: %w", err)
	}
	if status == "not_started" {
		return domain.OnboardingState{Status: "not_started"}, nil
	}
	if status != "completed" {
		return domain.OnboardingState{}, fmt.Errorf("invalid onboarding status %q", status)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return domain.OnboardingState{}, err
	}
	var completedAt string
	_ = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='onboarding_completed_at'`).Scan(&completedAt)
	return domain.OnboardingState{Status: "completed", Profile: &domain.OnboardingProfile{
		Version:       1,
		Status:        "completed",
		Origin:        "explicit_onboarding",
		ActiveSources: append([]domain.Source(nil), settings.ActiveSources...),
		CompletedAt:   completedAt,
	}}, nil
}

func (s *Store) CompleteOnboarding(ctx context.Context, sources []domain.Source) (domain.OnboardingState, error) {
	var previousStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='onboarding_status'`).Scan(&previousStatus); err != nil {
		return domain.OnboardingState{}, err
	}
	if previousStatus != "not_started" && previousStatus != "completed" {
		return domain.OnboardingState{}, fmt.Errorf("invalid onboarding status %q", previousStatus)
	}
	firstCompletion := previousStatus != "completed"
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return domain.OnboardingState{}, err
	}
	settings.ActiveSources = append([]domain.Source(nil), sources...)
	settings.Normalize()
	if err := settings.Validate(); err != nil {
		return domain.OnboardingState{}, err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return domain.OnboardingState{}, err
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.OnboardingState{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE settings SET value_json=?,updated_at=? WHERE key='runtime'`, string(raw), now); err != nil {
		return domain.OnboardingState{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('onboarding_status','completed') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		return domain.OnboardingState{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('onboarding_completed_at',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, now); err != nil {
		return domain.OnboardingState{}, err
	}
	if firstCompletion {
		calibrationStatus := "disabled"
		if settings.CalibrationEnabled {
			calibrationStatus = "pending"
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('calibration_first_run_status',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, calibrationStatus); err != nil {
			return domain.OnboardingState{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.OnboardingState{}, err
	}
	return s.Onboarding(ctx)
}

func (s *Store) CreateUpdateSession(ctx context.Context, intent string, settings domain.Settings, policy domain.UpdatePolicy) (domain.Session, error) {
	if err := domain.ValidateIntent(intent); err != nil {
		return domain.Session{}, err
	}
	if err := policy.Validate(); err != nil {
		return domain.Session{}, err
	}
	open, err := s.ActiveSession(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if open != nil {
		return domain.Session{}, errors.New("an active session already exists")
	}
	now := s.Now().UTC().Format(time.RFC3339Nano)
	sessionID := domain.NewID("session")
	coverage, err := json.Marshal(map[string]any{
		"sourceWaitMode":  settings.SourceWaitMode,
		"trigger":         policy.Trigger,
		"delivery":        policy.Delivery,
		"budgetAuthority": policy.BudgetAuthority,
	})
	if err != nil {
		return domain.Session{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Session{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,coverage_json,created_at) VALUES(?,?,'queued',?,?,?,?)`, sessionID, strings.TrimSpace(intent), settings.MaxItemsPerSource, settings.MaxItemsTotal, string(coverage), now); err != nil {
		return domain.Session{}, err
	}
	if policy.Delivery == domain.UpdateDeliveryPrepared {
		if _, err := tx.ExecContext(ctx, `INSERT INTO auto_update_batches(session_id,state,created_at) VALUES(?,'preparing',?)`, sessionID, now); err != nil {
			return domain.Session{}, err
		}
	}
	for ordinal, source := range settings.ActiveSources {
		if _, err := tx.ExecContext(ctx, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES(?,?,?,?,'queued','queued',?)`, domain.NewID("run"), sessionID, source, ordinal, now); err != nil {
			return domain.Session{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Session{}, err
	}
	return s.GetSession(ctx, sessionID)
}

func (s *Store) ActiveSession(ctx context.Context) (*domain.Session, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM sessions WHERE status IN ('queued','running') ORDER BY created_at DESC LIMIT 1`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	session, err := s.GetSession(ctx, id)
	return &session, err
}

func (s *Store) GetSession(ctx context.Context, id string) (domain.Session, error) {
	var session domain.Session
	var active, started, completed, coverageRaw, errorRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id,intent,status,active_source,max_items_per_source,max_items_total,created_at,started_at,completed_at,coverage_json,error_json FROM sessions WHERE id=?`, id).Scan(&session.ID, &session.Intent, &session.Status, &active, &session.MaxItemsPerSource, &session.MaxItemsTotal, &session.CreatedAt, &started, &completed, &coverageRaw, &errorRaw)
	if err != nil {
		return domain.Session{}, err
	}
	if active.Valid {
		value := domain.Source(active.String)
		session.ActiveSource = &value
	}
	if started.Valid {
		session.StartedAt = &started.String
	}
	if completed.Valid {
		session.CompletedAt = &completed.String
	}
	decodeJSON(coverageRaw.String, &session.Coverage)
	if errorRaw.Valid {
		var failure domain.Failure
		decodeJSON(errorRaw.String, &failure)
		session.Error = &failure
	}
	policy, deliveryState, err := s.SessionPolicy(ctx, id, session.Coverage)
	if err != nil {
		return domain.Session{}, err
	}
	session.Trigger = policy.Trigger
	session.Delivery = policy.Delivery
	session.BudgetAuthority = policy.BudgetAuthority
	session.DeliveryState = deliveryState
	runs, err := s.listRuns(ctx, id)
	if err != nil {
		return domain.Session{}, err
	}
	session.Runs = runs
	session.ActiveSource = nil
	for _, run := range runs {
		if run.Status == "waiting_for_bridge" && run.BridgeCommandStatus == "claimed" {
			value := run.Source
			session.ActiveSource = &value
			break
		}
	}
	if session.ActiveSource == nil {
		for _, run := range runs {
			if run.Status == "reasoning" {
				value := run.Source
				session.ActiveSource = &value
				break
			}
		}
	}
	if session.ActiveSource == nil {
		for _, run := range runs {
			if run.Status == "waiting_for_bridge" || run.Status == "queued" {
				value := run.Source
				session.ActiveSource = &value
				break
			}
		}
	}
	items, err := s.ListSessionItems(ctx, id)
	if err != nil {
		return domain.Session{}, err
	}
	session.Items = items
	return session, nil
}

func (s *Store) SetSessionPipelineStage(ctx context.Context, sessionID, stage string) error {
	switch stage {
	case "semantic_event_resolution", "timeline_composition", "ai_fast_detection", "finalizing":
	default:
		return fmt.Errorf("invalid session pipeline stage %q", stage)
	}
	coverage := map[string]any{}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT coverage_json FROM sessions WHERE id=? AND status IN ('queued','running')`, sessionID).Scan(&raw); err != nil {
		return err
	}
	decodeJSON(raw, &coverage)
	coverage["pipelineStage"] = stage
	coverage["pipelineStageUpdatedAt"] = domain.Now()
	encoded, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET coverage_json=? WHERE id=? AND status IN ('queued','running')`, string(encoded), sessionID)
	return err
}

func (s *Store) SetRunPipelineStage(ctx context.Context, runID, stage string) error {
	switch stage {
	case "acquisition_planning", "candidate_evaluation":
	default:
		return fmt.Errorf("invalid run pipeline stage %q", stage)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET stage=? WHERE id=? AND status='reasoning'`, stage, runID)
	return err
}

func (s *Store) listRuns(ctx context.Context, sessionID string) ([]domain.Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT runs.id,runs.session_id,runs.source,runs.ordinal,runs.status,runs.stage,runs.created_at,runs.started_at,runs.completed_at,runs.summary,runs.coverage_json,runs.error_json,(SELECT bridge_commands.status FROM bridge_commands WHERE bridge_commands.run_id=runs.id ORDER BY bridge_commands.created_at DESC LIMIT 1) FROM runs WHERE runs.session_id=? ORDER BY runs.ordinal`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []domain.Run
	for rows.Next() {
		var run domain.Run
		var started, completed, coverageRaw, errorRaw, commandStatus sql.NullString
		if err := rows.Scan(&run.ID, &run.SessionID, &run.Source, &run.Ordinal, &run.Status, &run.Stage, &run.CreatedAt, &started, &completed, &run.Summary, &coverageRaw, &errorRaw, &commandStatus); err != nil {
			return nil, err
		}
		if started.Valid {
			run.StartedAt = &started.String
		}
		if completed.Valid {
			run.CompletedAt = &completed.String
		}
		if commandStatus.Valid {
			run.BridgeCommandStatus = commandStatus.String
		}
		decodeJSON(coverageRaw.String, &run.Coverage)
		if errorRaw.Valid {
			var failure domain.Failure
			decodeJSON(errorRaw.String, &failure)
			run.Error = &failure
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *Store) GetRun(ctx context.Context, id string) (domain.Run, error) {
	var run domain.Run
	var started, completed, coverageRaw, errorRaw, commandStatus sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT runs.id,runs.session_id,runs.source,runs.ordinal,runs.status,runs.stage,runs.created_at,runs.started_at,runs.completed_at,runs.summary,runs.coverage_json,runs.error_json,(SELECT bridge_commands.status FROM bridge_commands WHERE bridge_commands.run_id=runs.id ORDER BY bridge_commands.created_at DESC LIMIT 1) FROM runs WHERE runs.id=?`, id).Scan(&run.ID, &run.SessionID, &run.Source, &run.Ordinal, &run.Status, &run.Stage, &run.CreatedAt, &started, &completed, &run.Summary, &coverageRaw, &errorRaw, &commandStatus)
	if err != nil {
		return domain.Run{}, err
	}
	if started.Valid {
		run.StartedAt = &started.String
	}
	if completed.Valid {
		run.CompletedAt = &completed.String
	}
	if commandStatus.Valid {
		run.BridgeCommandStatus = commandStatus.String
	}
	decodeJSON(coverageRaw.String, &run.Coverage)
	if errorRaw.Valid {
		var failure domain.Failure
		decodeJSON(errorRaw.String, &failure)
		run.Error = &failure
	}
	return run, nil
}

func (s *Store) StartRun(ctx context.Context, runID string, payload map[string]any) (domain.BridgeCommand, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return domain.BridgeCommand{}, err
	}
	now := s.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.BridgeCommand{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET status='running',active_source=?,started_at=COALESCE(started_at,?) WHERE id=?`, run.Source, now, run.SessionID); err != nil {
		return domain.BridgeCommand{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='waiting_for_bridge',stage='capture',started_at=COALESCE(started_at,?) WHERE id=?`, now, runID); err != nil {
		return domain.BridgeCommand{}, err
	}
	command := domain.BridgeCommand{ID: domain.NewID("command"), RunID: runID, Type: "collect_visible", Status: "queued", Payload: payload, CreatedAt: now}
	raw, _ := json.Marshal(payload)
	if _, err = tx.ExecContext(ctx, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES(?,?,?,'queued',?,?)`, command.ID, runID, command.Type, string(raw), now); err != nil {
		return domain.BridgeCommand{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.BridgeCommand{}, err
	}
	return command, nil
}

func (s *Store) QueueFollowUp(ctx context.Context, runID string, payload map[string]any) (domain.BridgeCommand, error) {
	command := domain.BridgeCommand{ID: domain.NewID("command"), RunID: runID, Type: "collect_visible", Status: "queued", Payload: payload, CreatedAt: domain.Now()}
	raw, _ := json.Marshal(payload)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.BridgeCommand{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='waiting_for_bridge',stage='follow_up_capture' WHERE id=? AND status='reasoning'`, runID); err != nil {
		return domain.BridgeCommand{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES(?,?,?,'queued',?,?)`, command.ID, runID, command.Type, string(raw), command.CreatedAt); err != nil {
		return domain.BridgeCommand{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.BridgeCommand{}, err
	}
	return command, nil
}

func (s *Store) ClaimCommand(ctx context.Context, runID, bridgeID string) (*domain.BridgeCommand, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var claimed int
	if err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM bridge_commands claimed
		JOIN runs claimed_run ON claimed_run.id=claimed.run_id
		JOIN runs requested_run ON requested_run.id=?
		WHERE claimed.status='claimed' AND claimed_run.session_id=requested_run.session_id
	`, runID).Scan(&claimed); err != nil {
		return nil, err
	}
	if claimed > 0 {
		return nil, nil
	}
	var command domain.BridgeCommand
	var raw string
	err = tx.QueryRowContext(ctx, `SELECT id,run_id,type,status,payload_json,created_at FROM bridge_commands WHERE run_id=? AND status='queued' ORDER BY created_at LIMIT 1`, runID).Scan(&command.ID, &command.RunID, &command.Type, &command.Status, &raw, &command.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := domain.Now()
	result, err := tx.ExecContext(ctx, `UPDATE bridge_commands SET status='claimed',claimed_by=?,claimed_at=? WHERE id=? AND status='queued'`, bridgeID, now, command.ID)
	if err != nil {
		return nil, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return nil, nil
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	command.Status = "claimed"
	command.ClaimedAt = &now
	decodeJSON(raw, &command.Payload)
	return &command, nil
}

func (s *Store) ExpiredBridgeCommands(ctx context.Context, now time.Time) ([]domain.BridgeCommand, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,run_id,type,status,payload_json,created_at,claimed_at
		FROM bridge_commands
		WHERE status='claimed' AND claimed_at IS NOT NULL
		ORDER BY claimed_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var expired []domain.BridgeCommand
	for rows.Next() {
		var command domain.BridgeCommand
		var raw, claimedAt string
		if err := rows.Scan(&command.ID, &command.RunID, &command.Type, &command.Status, &raw, &command.CreatedAt, &claimedAt); err != nil {
			return nil, err
		}
		claimed, err := time.Parse(time.RFC3339Nano, claimedAt)
		if err != nil {
			continue
		}
		decodeJSON(raw, &command.Payload)
		if now.Before(claimed.Add(bridgeCommandLease(command.Payload))) {
			continue
		}
		command.ClaimedAt = &claimedAt
		expired = append(expired, command)
	}
	return expired, rows.Err()
}

func bridgeCommandLease(payload map[string]any) time.Duration {
	const (
		minimum = 60 * time.Second
		maximum = 3 * time.Minute
		grace   = 30 * time.Second
	)
	milliseconds := grace.Milliseconds()
	for _, key := range []string{"sourceHydrationTimeoutMs", "captureTimeoutMs", "pendingContentTimeoutMs"} {
		milliseconds += int64(numericPayloadValue(payload[key]))
	}
	retryBudget := int64(numericPayloadValue(payload["qualityRetryBudget"]))
	if retryBudget > 0 {
		milliseconds += retryBudget * int64(numericPayloadValue(payload["qualityRetrySettleMs"]))
	}
	lease := time.Duration(milliseconds) * time.Millisecond
	if lease < minimum {
		return minimum
	}
	if lease > maximum {
		return maximum
	}
	return lease
}

func numericPayloadValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		number, _ := typed.Float64()
		return number
	default:
		return 0
	}
}

func (s *Store) PendingBridgeRunID(ctx context.Context) (string, error) {
	var runID string
	err := s.db.QueryRowContext(ctx, `
		SELECT r.id FROM runs r
		JOIN bridge_commands c ON c.run_id=r.id
		WHERE r.status='waiting_for_bridge' AND c.status='queued'
		ORDER BY c.created_at LIMIT 1`).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return runID, err
}

func (s *Store) SaveObservation(ctx context.Context, commandID, runID string, observation domain.Observation) error {
	if !observation.Source.Valid() {
		return errors.New("observation source is invalid")
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var expectedRun string
	var status string
	var commandClaimedAt sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT run_id,status,claimed_at FROM bridge_commands WHERE id=?`, commandID).Scan(&expectedRun, &status, &commandClaimedAt); err != nil {
		return err
	}
	if expectedRun != runID || status != "claimed" {
		return errors.New("bridge command is not claimable for this run")
	}
	observedAt := strings.TrimSpace(observation.CapturedAt)
	if _, parseErr := time.Parse(time.RFC3339Nano, observedAt); parseErr != nil {
		observedAt = now
		observation.CapturedAt = observedAt
	}
	identity, err := s.resolveObservationContentIdentity(ctx, tx, runID, &observation, observedAt)
	if err != nil {
		return err
	}
	if observation.Coverage == nil {
		observation.Coverage = map[string]any{}
	}
	observation.Coverage["contentIdentity"] = identity
	raw, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES(?,?,?,?,?,?,?)`, domain.NewID("observation"), runID, commandID, observation.Source, string(raw), observation.CapturedAt, now); err != nil {
		return err
	}
	var coverageRaw string
	if err = tx.QueryRowContext(ctx, `SELECT coverage_json FROM runs WHERE id=?`, runID).Scan(&coverageRaw); err != nil {
		return err
	}
	coverage := durableCaptureCoverage(coverageRaw, observation.Coverage)
	durableCoverageRaw, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bridge_commands SET status='completed',completed_at=? WHERE id=?`, now, commandID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning',coverage_json=? WHERE id=?`, string(durableCoverageRaw), runID); err != nil {
		return err
	}
	captureDurationMS := int64(0)
	if claimedAt, parseErr := time.Parse(time.RFC3339Nano, commandClaimedAt.String); commandClaimedAt.Valid && parseErr == nil {
		captureDurationMS = time.Since(claimedAt).Milliseconds()
		if captureDurationMS < 0 {
			captureDurationMS = 0
		}
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO run_stage_timings(run_id,stage,duration_ms,completed_at) VALUES(?,'captured',?,?)
		ON CONFLICT(run_id,stage) DO UPDATE SET duration_ms=run_stage_timings.duration_ms+excluded.duration_ms,completed_at=excluded.completed_at`,
		runID, captureDurationMS, now); err != nil {
		return err
	}
	return tx.Commit()
}

func durableCaptureCoverage(raw string, next map[string]any) map[string]any {
	var current map[string]any
	decodeJSON(raw, &current)
	if current == nil {
		current = map[string]any{}
	}
	rounds := make([]any, 0, 2)
	if values, ok := current["rounds"].([]any); ok {
		rounds = append(rounds, values...)
	}
	rounds = append(rounds, next)
	current["acquisitionRounds"] = len(rounds)
	current["rounds"] = rounds
	return current
}

func (s *Store) SetRunCoverageField(ctx context.Context, runID, key string, value any) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("run coverage key is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT coverage_json FROM runs WHERE id=?`, runID).Scan(&raw); err != nil {
		return err
	}
	coverage := map[string]any{}
	decodeJSON(raw, &coverage)
	coverage[key] = value
	encoded, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET coverage_json=? WHERE id=?`, string(encoded), runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Observations(ctx context.Context, runID string) ([]domain.Observation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT observation_json FROM observations WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []domain.Observation
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value domain.Observation
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ResumableReasoningRunIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id
		FROM runs r
		WHERE r.status='reasoning'
		  AND EXISTS (SELECT 1 FROM observations o WHERE o.run_id=r.id)
		ORDER BY r.created_at,r.ordinal`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) FailCommand(ctx context.Context, commandID, runID string, failure domain.Failure) (bool, error) {
	raw, _ := json.Marshal(failure)
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var runStage string
	var observationCount int
	if err = tx.QueryRowContext(ctx, `SELECT r.stage,(SELECT COUNT(*) FROM observations o WHERE o.run_id=r.id) FROM runs r JOIN bridge_commands c ON c.run_id=r.id WHERE r.id=? AND c.id=?`, runID, commandID).Scan(&runStage, &observationCount); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE bridge_commands SET status='failed',completed_at=?,error_json=? WHERE id=? AND run_id=? AND status='claimed'`, now, string(raw), commandID, runID)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return false, errors.New("bridge command is not active")
	}
	if runStage == "follow_up_capture" && observationCount > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning' WHERE id=?`, runID); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=?`, failure.Stage, now, string(raw), runID); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Store) FailRun(ctx context.Context, runID string, failure domain.Failure) error {
	raw, _ := json.Marshal(failure)
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=? AND status NOT IN ('completed','failed','cancelled')`, failure.Stage, domain.Now(), string(raw), runID)
	return err
}

func (s *Store) FailQueuedRuns(ctx context.Context, sessionID string, failure domain.Failure) error {
	raw, err := json.Marshal(failure)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE session_id=? AND status='queued'`, failure.Stage, s.Now().UTC().Format(time.RFC3339Nano), string(raw), sessionID)
	return err
}

func (s *Store) SaveTelemetry(ctx context.Context, value domain.ReasoningTelemetry) error {
	callerLatency := value.CallerLatencyMS
	if callerLatency == 0 {
		callerLatency = value.DurationMS
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO reasoning_invocations(id,run_id,phase,provider,model,model_descriptor_version,model_maturity,effort,duration_ms,caller_latency_ms,queue_wait_ms,provider_execution_ms,response_total_ms,status,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.RunID, value.Phase, value.Provider, value.Model, value.ModelDescriptorVersion, value.ModelMaturity, value.Effort, value.DurationMS, callerLatency, value.QueueWaitMS, value.ProviderExecutionMS, value.ResponseTotalMS, value.Status, value.InputTokens, value.CachedInputTokens, value.OutputTokens, value.ReasoningOutputTokens, value.CreatedAt)
	return err
}

// ReasoningInvocationsBySession lists all provider invocations belonging to a
// session's runs, ordered chronologically. It is used to enrich diagnostics
// exports with per-run provider, model, effort, and elapsed timing.
func (s *Store) ReasoningInvocationsBySession(ctx context.Context, sessionID string) ([]domain.ReasoningTelemetry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id,i.run_id,i.phase,i.provider,i.model,i.model_descriptor_version,i.model_maturity,i.effort,i.duration_ms,i.caller_latency_ms,i.queue_wait_ms,i.provider_execution_ms,i.response_total_ms,i.status,
		       i.input_tokens,i.cached_input_tokens,i.output_tokens,i.reasoning_output_tokens,i.created_at
		FROM reasoning_invocations i JOIN runs r ON r.id=i.run_id
		WHERE r.session_id=? ORDER BY i.created_at,i.id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ReasoningTelemetry
	for rows.Next() {
		var t domain.ReasoningTelemetry
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&t.ID, &t.RunID, &t.Phase, &t.Provider, &t.Model, &t.ModelDescriptorVersion, &t.ModelMaturity, &t.Effort, &t.DurationMS, &t.CallerLatencyMS, &t.QueueWaitMS, &t.ProviderExecutionMS, &t.ResponseTotalMS, &t.Status, &input, &cached, &output, &reasoning, &t.CreatedAt); err != nil {
			return nil, err
		}
		if t.CallerLatencyMS == 0 {
			t.CallerLatencyMS = t.DurationMS
		}
		if input.Valid {
			t.InputTokens = &input.Int64
		}
		if cached.Valid {
			t.CachedInputTokens = &cached.Int64
		}
		if output.Valid {
			t.OutputTokens = &output.Int64
		}
		if reasoning.Valid {
			t.ReasoningOutputTokens = &reasoning.Int64
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type ScoredAssessment struct {
	Assessment                             domain.CandidateAssessment
	BaseScore, PreferenceScore, FinalScore float64
	Selected                               bool
}

func (s *Store) CompleteRun(ctx context.Context, run domain.Run, result domain.ReasoningResult, scored []ScoredAssessment, items []domain.TimelineItem, coverage map[string]any) error {
	now := domain.Now()
	coverageRaw, _ := json.Marshal(coverage)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	byKey := map[string]ScoredAssessment{}
	itemsByKey := map[string]domain.ReasonedItem{}
	for _, item := range result.Items {
		itemsByKey[item.EvidenceKey] = item
	}
	for _, value := range scored {
		byKey[value.Assessment.EvidenceKey] = value
		raw, _ := json.Marshal(value.Assessment)
		itemRaw, _ := json.Marshal(itemsByKey[value.Assessment.EvidenceKey])
		selected := 0
		if value.Selected {
			selected = 1
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, run.ID, value.Assessment.EvidenceKey, run.Source, string(raw), string(itemRaw), value.BaseScore, value.PreferenceScore, value.FinalScore, selected, now); err != nil {
			return err
		}
	}
	for index, item := range items {
		item.Rank = index
		item.CreatedAt = now
		itemRaw, _ := json.Marshal(item.Item)
		assessment := byKey[item.EvidenceKey].Assessment
		assessmentRaw, _ := json.Marshal(assessment)
		itemCoverage, _ := json.Marshal(item.Coverage)
		if _, err = tx.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, item.ID, item.SessionID, item.RunID, item.Source, item.EvidenceKey, item.Rank, string(itemRaw), string(assessmentRaw), string(itemCoverage), now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',completed_at=?,summary=?,coverage_json=? WHERE id=?`, now, result.Summary, string(coverageRaw), run.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AdvanceSession(ctx context.Context, sessionID string) (*domain.Run, error) {
	runs, err := s.listRuns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Status == "queued" {
			return &run, nil
		}
	}
	return nil, nil
}

// FinalizeSession publishes a terminal session only after Timeline composition
// and knowledge continuity have committed. Until this method succeeds, a new
// update remains blocked by the still-active session.
func (s *Store) FinalizeSession(ctx context.Context, sessionID string) error {
	runs, err := s.listRuns(ctx, sessionID)
	if err != nil {
		return err
	}
	now := s.Now().UTC().Format(time.RFC3339Nano)
	completed := 0
	failed := 0
	for _, run := range runs {
		switch run.Status {
		case "completed":
			completed++
		case "failed", "cancelled":
			failed++
		default:
			return fmt.Errorf("session %s cannot finalize while run %s is %s", sessionID, run.ID, run.Status)
		}
	}
	status := "failed"
	if completed == len(runs) {
		status = "completed"
	} else if completed > 0 {
		status = "partial"
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE sessions SET status=?,active_source=NULL,completed_at=? WHERE id=?`, status, now, sessionID); err != nil {
		return err
	}
	settings, settingsErr := s.GetSettings(ctx)
	if settingsErr != nil {
		return settingsErr
	}
	freshHours := settings.PreparedBatchMaxAgeHours
	var highestUrgency float64
	if urgencyErr := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(CAST(json_extract(assessment_json,'$.urgency') AS REAL)),0) FROM timeline_items WHERE session_id=?`, sessionID).Scan(&highestUrgency); urgencyErr != nil {
		return urgencyErr
	}
	switch {
	case highestUrgency >= 0.85:
		freshHours = 2
	case highestUrgency >= 0.75 && freshHours > 4:
		freshHours = 4
	case highestUrgency >= 0.50 && freshHours > 12:
		freshHours = 12
	}
	expiresAt := s.Now().UTC().Add(time.Duration(freshHours) * time.Hour).Format(time.RFC3339Nano)
	if _, err = s.db.ExecContext(ctx, `UPDATE auto_update_batches SET state=CASE WHEN EXISTS (SELECT 1 FROM timeline_items t WHERE t.session_id=?) THEN 'prepared' ELSE 'expired' END,prepared_at=?,expires_at=? WHERE session_id=? AND state='preparing'`, sessionID, now, expiresAt, sessionID); err != nil {
		return err
	}
	return nil
}

func (s *Store) Knowledge(ctx context.Context, source domain.Source, limit int) ([]domain.ReasonedItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_json FROM knowledge_events WHERE source=? ORDER BY last_seen_at DESC LIMIT ?`, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.ReasonedItem
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item domain.ReasonedItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PreviouslyDeliveredEvidence(ctx context.Context, source domain.Source, evidenceKeys []string) (map[string]bool, error) {
	result := map[string]bool{}
	if len(evidenceKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(evidenceKeys))
	args := make([]any, 0, len(evidenceKeys)+1)
	args = append(args, source)
	for index, key := range evidenceKeys {
		placeholders[index] = "?"
		args = append(args, key)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT evidence_key FROM timeline_items WHERE source=? AND evidence_key IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	return result, rows.Err()
}

func (s *Store) PreviouslyKnownEvents(ctx context.Context, source domain.Source, eventKeys []string) (map[string]bool, error) {
	result := map[string]bool{}
	if len(eventKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(eventKeys))
	args := make([]any, 0, len(eventKeys)+1)
	args = append(args, source)
	for index, key := range eventKeys {
		placeholders[index] = "?"
		args = append(args, key)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_key FROM knowledge_events WHERE source=? AND event_key IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	return result, rows.Err()
}

type compositionItem struct {
	id               string
	source           domain.Source
	evidence         string
	score            float64
	itemRaw          string
	item             domain.ReasonedItem
	created          string
	semanticEventID  string
	semanticRelation string
}

// ComposeSession establishes one global personalized order after every source
// has finished. It prevents more than two consecutive items from one source
// whenever another source is still available.
func (s *Store) ComposeSession(ctx context.Context, sessionID string) error {
	var limit int
	if err := s.db.QueryRowContext(ctx, `SELECT max_items_total FROM sessions WHERE id=?`, sessionID).Scan(&limit); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id,t.source,t.evidence_key,a.final_score,t.item_json,t.created_at,
		       COALESCE(r.event_id,''),COALESCE(r.relation,'')
		FROM timeline_items t
		JOIN candidate_assessments a ON a.run_id=t.run_id AND a.evidence_key=t.evidence_key
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE t.session_id=?
		ORDER BY a.final_score DESC,t.created_at,t.rank`, sessionID)
	if err != nil {
		return err
	}
	var remaining []compositionItem
	for rows.Next() {
		var item compositionItem
		if err := rows.Scan(&item.id, &item.source, &item.evidence, &item.score, &item.itemRaw, &item.created, &item.semanticEventID, &item.semanticRelation); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(item.itemRaw), &item.item); err != nil {
			rows.Close()
			return err
		}
		remaining = append(remaining, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	ordered := make([]compositionItem, 0, len(remaining))
	for len(remaining) > 0 {
		chosen := 0
		if len(ordered) >= 2 && ordered[len(ordered)-1].source == ordered[len(ordered)-2].source {
			for index, candidate := range remaining {
				if candidate.source != ordered[len(ordered)-1].source {
					chosen = index
					break
				}
			}
		}
		ordered = append(ordered, remaining[chosen])
		remaining = append(remaining[:chosen], remaining[chosen+1:]...)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	uniqueKept := 0
	duplicateKept := 0
	rank := 0
	duplicatesPerEvent := map[string]int{}
	for _, item := range ordered {
		keep := false
		if item.semanticRelation == "duplicate_report" {
			eventKey := item.semanticEventID
			if eventKey == "" {
				eventKey = item.evidence
			}
			if duplicateKept < limit && duplicatesPerEvent[eventKey] < 3 {
				keep = true
				duplicateKept++
				duplicatesPerEvent[eventKey]++
			}
		} else if uniqueKept < limit {
			keep = true
			uniqueKept++
		}
		if keep {
			if _, err := tx.ExecContext(ctx, `UPDATE timeline_items SET rank=? WHERE id=?`, rank, item.id); err != nil {
				return err
			}
			rank++
			if item.item.EventKey != "" {
				if _, err := tx.ExecContext(ctx, `INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(source,event_key) DO UPDATE SET evidence_key=excluded.evidence_key,item_json=excluded.item_json,last_seen_at=excluded.last_seen_at WHERE excluded.last_seen_at >= knowledge_events.last_seen_at`, domain.NewID("knowledge"), item.source, item.item.EventKey, item.evidence, item.itemRaw, item.created, item.created); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM timeline_items WHERE id=?`, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListSessionItems(ctx context.Context, sessionID string) ([]domain.TimelineItem, error) {
	return s.listItems(ctx, `WHERE session_id=? ORDER BY rank`, sessionID)
}
func (s *Store) TimelineItem(ctx context.Context, timelineID string) (domain.TimelineItem, error) {
	items, err := s.listItems(ctx, `WHERE id=?`, timelineID)
	if err != nil {
		return domain.TimelineItem{}, err
	}
	if len(items) != 1 {
		return domain.TimelineItem{}, sql.ErrNoRows
	}
	return items[0], nil
}

const timelinePresentationOrderSQL = `
	ORDER BY
	  CASE WHEN COALESCE((SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),'')='append' THEN 1 ELSE 0 END,
	  CASE WHEN COALESCE((SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),'')<>'append'
	    THEN COALESCE(
	      (SELECT b.revealed_at FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),
	      (SELECT s.completed_at FROM sessions s WHERE s.id=timeline_items.session_id)
	    )
	  END DESC,
	  CASE WHEN COALESCE((SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),'')='append'
	    THEN COALESCE(
	      (SELECT b.revealed_at FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),
	      (SELECT s.completed_at FROM sessions s WHERE s.id=timeline_items.session_id)
	    )
	  END ASC,
	  rank`

func (s *Store) ListTimeline(ctx context.Context, limit, offset int) ([]domain.TimelineItem, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.SemanticEventMode == "show_all" {
		items, err := s.listItems(ctx, `WHERE NOT EXISTS (SELECT 1 FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id AND b.state<>'visible')`+timelinePresentationOrderSQL+` LIMIT ? OFFSET ?`, limit, offset)
		for index := range items {
			items[index].SemanticEvent = nil
		}
		return items, err
	}
	items, err := s.listItems(ctx, `WHERE NOT EXISTS (SELECT 1 FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id AND b.state<>'visible')`+timelinePresentationOrderSQL+` LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	result := make([]domain.TimelineItem, 0, limit)
	uniqueSeen := 0
	uniqueIncluded := 0
	for _, item := range items {
		duplicate := item.SemanticEvent != nil && item.SemanticEvent.Relation == "duplicate_report"
		if duplicate {
			if settings.SemanticEventMode == "collapse" && uniqueSeen >= offset && uniqueIncluded <= limit {
				result = append(result, item)
			}
			continue
		}
		if uniqueSeen < offset {
			uniqueSeen++
			continue
		}
		if uniqueIncluded >= limit {
			break
		}
		result = append(result, item)
		uniqueSeen++
		uniqueIncluded++
	}
	return result, nil
}

func (s *Store) listItems(ctx context.Context, suffix string, args ...any) ([]domain.TimelineItem, error) {
	query := `SELECT id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at FROM timeline_items ` + suffix
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.TimelineItem
	for rows.Next() {
		var item domain.TimelineItem
		var itemRaw, assessmentRaw, coverageRaw string
		if err := rows.Scan(&item.ID, &item.SessionID, &item.RunID, &item.Source, &item.EvidenceKey, &item.Rank, &itemRaw, &assessmentRaw, &coverageRaw, &item.CreatedAt); err != nil {
			return nil, err
		}
		decodeJSON(itemRaw, &item.Item)
		decodeJSON(assessmentRaw, &item.Assessment)
		decodeJSON(coverageRaw, &item.Coverage)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	evidenceByRun := map[string]map[string]domain.Block{}
	for index := range items {
		item := &items[index]
		byKey, loaded := evidenceByRun[item.RunID]
		if !loaded {
			byKey = map[string]domain.Block{}
			observations, err := s.Observations(ctx, item.RunID)
			if err != nil {
				return nil, err
			}
			for _, observation := range observations {
				for _, snapshot := range observation.Snapshots {
					for _, block := range snapshot.Blocks {
						if block.EvidenceKey != "" {
							if _, exists := byKey[block.EvidenceKey]; !exists {
								byKey[block.EvidenceKey] = block
							}
						}
					}
				}
			}
			evidenceByRun[item.RunID] = byKey
		}
		if block, exists := byKey[item.EvidenceKey]; exists {
			copy := block
			item.Evidence = &copy
		}
		var overrideRaw string
		if err := s.db.QueryRowContext(ctx, `SELECT evidence_json FROM timeline_evidence_overrides WHERE timeline_id=?`, item.ID).Scan(&overrideRaw); err == nil {
			var override domain.Block
			decodeJSON(overrideRaw, &override)
			item.Evidence = &override
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if err := s.attachSemanticEvents(ctx, items); err != nil {
		return nil, err
	}
	if err := s.attachAIDetections(ctx, items); err != nil {
		return nil, err
	}
	if err := s.attachEffectivePreferenceDecision(ctx, items); err != nil {
		return nil, err
	}
	if err := s.attachTimelineMemoryProjections(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// attachTimelineMemoryProjections restores only the bounded retention state
// for a Timeline item. The lookup is intentionally best-effort for an item that has
// no valid memory identity yet; Timeline reads must remain useful for neutral
// or media-only entries that were never kept. Full text never crosses this
// projection boundary.
func (s *Store) attachTimelineMemoryProjections(ctx context.Context, items []domain.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin Timeline memory projection: %w", err)
	}
	defer tx.Rollback()
	for index := range items {
		normalized, normalizeErr := normalizeMemoryInput(routineMoreMemoryInput(items[index]))
		if normalizeErr != nil {
			continue
		}
		memoryID, resolveErr := resolveMemoryIdentity(ctx, tx, normalized.Identity)
		if resolveErr != nil {
			return fmt.Errorf("resolve Timeline memory projection: %w", resolveErr)
		}
		if memoryID == "" {
			continue
		}
		memory, memoryErr := memoryItemByQueryer(ctx, tx, memoryID)
		if errors.Is(memoryErr, ErrMemoryNotFound) {
			continue
		}
		if memoryErr != nil {
			return fmt.Errorf("read Timeline memory projection: %w", memoryErr)
		}
		if memory.LifecycleState != domain.MemoryStateActive {
			continue
		}
		items[index].PersonalMemory = &domain.TimelineMemoryProjection{
			RetentionTier: memory.RetentionTier,
			Saved:         memory.Saved,
			PermanentKeep: memory.PermanentKeep,
		}
	}
	return nil
}

func (s *Store) attachEffectivePreferenceDecision(ctx context.Context, items []domain.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	placeholders := make([]string, len(items))
	args := make([]any, len(items))
	itemsByID := make(map[string]*domain.TimelineItem, len(items))
	for index := range items {
		placeholders[index] = "(?)"
		args[index] = items[index].ID
		itemsByID[items[index].ID] = &items[index]
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH requested(timeline_id) AS (VALUES `+strings.Join(placeholders, ",")+`),
		decisions AS (
		  SELECT f.id,f.timeline_id,f.direction,f.reason,f.created_at,'routine' AS origin,2 AS authority
		  FROM feedback_events f
		  JOIN requested q ON q.timeline_id=f.timeline_id
		  UNION ALL
		  SELECT 'calibration:' || c.calibration_session_id || ':' || c.ordinal,t.id,
		    CASE c.label WHEN 'more_like_this' THEN 'more' ELSE 'less' END,
		    CASE c.label WHEN 'less_like_this' THEN 'not_interested' ELSE NULL END,
		    c.labeled_at,'calibration',1
		  FROM timeline_items t
		  JOIN requested q ON q.timeline_id=t.id
		  JOIN calibration_samples c ON c.run_id=t.run_id AND c.evidence_key=t.evidence_key
		  WHERE c.label IN ('more_like_this','less_like_this')
		), ranked AS (
		  SELECT decisions.*,
		    ROW_NUMBER() OVER (PARTITION BY timeline_id ORDER BY created_at DESC,authority DESC,id DESC) AS signal_rank
		  FROM decisions
		)
		SELECT id,timeline_id,direction,reason,origin,created_at
		FROM ranked WHERE signal_rank=1`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, timelineID, direction, origin, createdAt string
		var reason sql.NullString
		if err := rows.Scan(&id, &timelineID, &direction, &reason, &origin, &createdAt); err != nil {
			return err
		}
		item := itemsByID[timelineID]
		if item == nil {
			continue
		}
		feedback := domain.Feedback{
			ID: id, TimelineID: timelineID, SessionID: item.SessionID, RunID: item.RunID,
			EvidenceKey: item.EvidenceKey, Direction: direction, Origin: origin, CreatedAt: createdAt,
		}
		if reason.Valid {
			feedback.Reason = &reason.String
		}
		item.Feedback = &feedback
	}
	return rows.Err()
}

func (s *Store) AddFeedback(ctx context.Context, timelineID string, input domain.Feedback) (domain.Feedback, error) {
	var sessionID, runID, evidenceKey, sessionStatus string
	err := s.db.QueryRowContext(ctx, `
		SELECT t.session_id,t.run_id,t.evidence_key,s.status
		FROM timeline_items t JOIN sessions s ON s.id=t.session_id
		WHERE t.id=?`, timelineID).Scan(&sessionID, &runID, &evidenceKey, &sessionStatus)
	if err != nil {
		return domain.Feedback{}, err
	}
	// AddFeedback is the authoritative routine Timeline action. Origin is
	// normalized below so a stale or forged client hint cannot suppress the
	// guaranteed recall projection. Calibration and selection corrections use
	// their distinct canonical store paths and never call AddFeedback.
	input.ID = domain.NewID("feedback")
	input.TimelineID = timelineID
	input.SessionID = sessionID
	input.RunID = runID
	input.EvidenceKey = evidenceKey
	input.Origin = "routine"
	input.CreatedAt = domain.Now()
	if err = input.Validate(); err != nil {
		return domain.Feedback{}, err
	}

	// A Timeline row only becomes a Personal Memory candidate once it belongs
	// to a terminal, visible session. ComposeSession already removes
	// non-survivors; the existence check above therefore forms the survivor
	// boundary while the terminal status prevents projection during capture or
	// reasoning.
	projectMemory := input.Direction == "more" &&
		(sessionStatus == "completed" || sessionStatus == "partial")
	retractMemory := false
	var timelineItem domain.TimelineItem
	if projectMemory || (input.Direction == "less" && (sessionStatus == "completed" || sessionStatus == "partial")) {
		timelineItem, err = s.TimelineItem(ctx, timelineID)
		if err != nil {
			return domain.Feedback{}, err
		}
	}
	var normalized normalizedMemoryInput
	var tombstoneKey []byte
	if projectMemory {
		memoryInput := routineMoreMemoryInput(timelineItem)
		normalized, err = normalizeMemoryInput(memoryInput)
		if err != nil {
			return domain.Feedback{}, fmt.Errorf("prepare routine More memory projection: %w", err)
		}
		tombstoneKey, err = s.memoryTombstoneKey(ctx)
		if err != nil {
			return domain.Feedback{}, err
		}
		normalized.IdentityDigest = memoryIdentityDigest(tombstoneKey, normalized.Identity)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Feedback{}, fmt.Errorf("begin feedback transaction: %w", err)
	}
	defer tx.Rollback()
	if input.Direction == "less" && timelineItem.ID != "" {
		var previousDirection string
		err = tx.QueryRowContext(ctx, `
			SELECT direction FROM feedback_events
			WHERE timeline_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, timelineID).Scan(&previousDirection)
		if errors.Is(err, sql.ErrNoRows) {
			previousDirection = ""
		} else if err != nil {
			return domain.Feedback{}, fmt.Errorf("read previous routine feedback: %w", err)
		}
		retractMemory = previousDirection == "more"
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,reason,created_at) VALUES(?,?,?,?,?,?,?,?)`, input.ID, input.TimelineID, input.SessionID, input.RunID, input.EvidenceKey, input.Direction, input.Reason, input.CreatedAt); err != nil {
		return domain.Feedback{}, err
	}
	if projectMemory {
		if _, err = s.upsertMemoryRecallStubTx(ctx, tx, tombstoneKey, normalized); err != nil {
			// The feedback row and projection intentionally roll back together.
			return domain.Feedback{}, fmt.Errorf("project routine More memory: %w", err)
		}
	}
	if retractMemory {
		if _, err := retractRoutineMoreMemoryTx(ctx, tx, timelineItem); err != nil {
			return domain.Feedback{}, fmt.Errorf("retract routine More memory: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.Feedback{}, fmt.Errorf("commit feedback transaction: %w", err)
	}
	return input, nil
}

func (s *Store) CancelSession(ctx context.Context, id string) error {
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET status='cancelled',active_source=NULL,completed_at=? WHERE id=? AND status IN ('queued','running')`, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='cancelled',stage='cancelled',completed_at=? WHERE session_id=? AND status IN ('queued','waiting_for_bridge','reasoning')`, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bridge_commands SET status='cancelled',completed_at=? WHERE run_id IN (SELECT id FROM runs WHERE session_id=?) AND status IN ('queued','claimed')`, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RetryFailedRun(ctx context.Context, runID string) (domain.Run, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status != "failed" {
		return domain.Run{}, errors.New("only a failed run can be re-evaluated")
	}
	var observations, evaluations, timeline int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM observations WHERE run_id=?`, runID).Scan(&observations); err != nil {
		return domain.Run{}, err
	}
	if observations == 0 {
		return domain.Run{}, errors.New("failed run has no durable captured evidence to re-evaluate")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_assessments WHERE run_id=?`, runID).Scan(&evaluations); err != nil {
		return domain.Run{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, runID).Scan(&timeline); err != nil {
		return domain.Run{}, err
	}
	if evaluations != 0 || timeline != 0 {
		return domain.Run{}, errors.New("failed run already contains evaluated state and cannot be replayed safely")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Run{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET status='running',active_source=?,completed_at=NULL,error_json=NULL WHERE id=?`, run.Source, run.SessionID); err != nil {
		return domain.Run{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning',completed_at=NULL,summary='',error_json=NULL WHERE id=?`, runID); err != nil {
		return domain.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}

func (s *Store) ResetLearning(ctx context.Context) error {
	if err := s.requireIdle(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `
		DELETE FROM calibration_sessions;
		DELETE FROM feedback_events;
		DELETE FROM ai_feedback_events;
		DELETE FROM content_context_feedback_events;
		DELETE FROM preference_learning_ledger;
		DELETE FROM preference_model;
		DELETE FROM meta WHERE key='calibration_first_run_status';
		INSERT INTO meta(key,value) VALUES('preference_signal_reset_at',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value;`, domain.Now()); err != nil {
		return err
	}
	return tx.Commit()
}

type FullResetResult struct {
	CompletedAt string `json:"completedAt"`
	BackupFile  string `json:"backupFile"`
}

func (s *Store) FullReset(ctx context.Context, defaults domain.Settings) (FullResetResult, error) {
	if err := s.requireIdle(ctx); err != nil {
		return FullResetResult{}, err
	}
	defaults.Normalize()
	if err := defaults.Validate(); err != nil {
		return FullResetResult{}, err
	}
	backupPath, err := s.createVerifiedBackup(ctx)
	if err != nil {
		return FullResetResult{}, err
	}
	raw, err := json.Marshal(defaults)
	if err != nil {
		return FullResetResult{}, err
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FullResetResult{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM living_topic_activation_jobs; DELETE FROM living_topic_candidate_evaluations;`); err != nil {
		return FullResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM living_topic_model_invocations; DELETE FROM living_topic_understanding_jobs; DELETE FROM living_topic_routing_jobs; DELETE FROM living_topic_feedback_events; DELETE FROM living_topic_snapshots; DELETE FROM living_topic_membership_moves; DELETE FROM living_topic_memberships; DELETE FROM living_topics; DELETE FROM memory_search_fts; DELETE FROM memory_retention_claims; DELETE FROM memory_actions; DELETE FROM memory_provenance; DELETE FROM memory_content_versions; DELETE FROM memory_tombstone_aliases; DELETE FROM memory_identity_aliases; DELETE FROM memory_items; DELETE FROM content_context_feedback_events; DELETE FROM ai_feedback_events; DELETE FROM content_continuity_occurrences; DELETE FROM content_continuity; DELETE FROM content_identity_aliases; DELETE FROM sessions; DELETE FROM semantic_event_constraints; DELETE FROM semantic_events; DELETE FROM feedback_events; DELETE FROM preference_learning_ledger; DELETE FROM preference_model; DELETE FROM knowledge_events; DELETE FROM settings; DELETE FROM meta WHERE key IN ('calibration_first_run_status','preference_signal_reset_at','auto_update_budget_reset_day','auto_update_budget_reset_total','auto_update_budget_reset_automatic','auto_update_budget_reset_at','auto_update_scheduler_tick_at','auto_update_scheduler_receipts','auto_update_usage_limit_pause','pending_app_profile_reset','memory_tombstone_key_v1');`); err != nil {
		return FullResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE auto_update_state SET last_ui_access_at=NULL,last_attempt_at=NULL,last_success_at=NULL,last_error='' WHERE id=1`); err != nil {
		return FullResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value_json,updated_at) VALUES('runtime',?,?)`, string(raw), now); err != nil {
		return FullResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('onboarding_status','not_started') ON CONFLICT(key) DO UPDATE SET value=excluded.value`); err != nil {
		return FullResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM meta WHERE key='onboarding_completed_at'`); err != nil {
		return FullResetResult{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('last_full_reset',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, now); err != nil {
		return FullResetResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return FullResetResult{}, err
	}
	return FullResetResult{CompletedAt: now, BackupFile: filepath.Base(backupPath)}, nil
}

func (s *Store) requireIdle(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE status IN ('queued','running')`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return errors.New("reset is unavailable while an update is running")
	}
	return nil
}

// PendingAppProfileReset reports a legacy pre-0.8 profile-wipe marker. New
// resets never create one; startup consumes it without deleting the profile.
func (s *Store) PendingAppProfileReset(ctx context.Context) (bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='pending_app_profile_reset'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ConsumePendingAppProfileReset clears a legacy profile-wipe marker.
func (s *Store) ConsumePendingAppProfileReset(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM meta WHERE key='pending_app_profile_reset'`)
	return err
}

func (s *Store) createVerifiedBackup(ctx context.Context) (string, error) {
	directory := filepath.Join(filepath.Dir(s.path), "backups")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("create reset backup directory: %w", err)
	}
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	target := filepath.Join(directory, "pre-full-reset-"+stamp+".db")
	quoted := strings.ReplaceAll(target, "'", "''")
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO '`+quoted+`'`); err != nil {
		return "", fmt.Errorf("create reset backup: %w", err)
	}
	if err := verifySQLiteBackup(target); err != nil {
		return "", err
	}
	return target, nil
}

func verifySQLiteBackup(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open reset backup: %w", err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("inspect reset backup: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("reset backup integrity check returned %q", integrity)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("inspect reset backup foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("reset backup contains foreign key violations")
	}
	return rows.Err()
}

func decodeJSON(raw string, target any) {
	if raw == "" {
		return
	}
	_ = json.Unmarshal([]byte(raw), target)
}
