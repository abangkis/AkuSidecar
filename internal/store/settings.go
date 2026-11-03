package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

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
