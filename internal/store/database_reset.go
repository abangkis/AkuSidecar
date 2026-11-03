package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

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
	if _, err = tx.ExecContext(ctx, `DELETE FROM living_topic_activation_jobs; DELETE FROM living_topic_candidate_evaluations; DELETE FROM timeline_items; DELETE FROM timeline_retention_receipts;`); err != nil {
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
