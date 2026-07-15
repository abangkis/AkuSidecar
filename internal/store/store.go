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
	db   *sql.DB
	path string
}

func Open(path string, defaults domain.Settings) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path}
	if err := store.initialize(defaults); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) initialize(defaults domain.Settings) error {
	ctx := context.Background()
	if _, err := s.db.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("initialize schema: %w", err)
	}
	var version string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version',?)`, schemaVersion); err != nil {
			return fmt.Errorf("save schema version: %w", err)
		}
	case err != nil:
		return fmt.Errorf("read schema version: %w", err)
	case version != schemaVersion:
		return fmt.Errorf("database schema %s is incompatible with required schema %s; start with a fresh database", version, schemaVersion)
	}
	if _, err := s.bridgeToken(ctx); err != nil {
		return err
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
		if err := s.SaveSettings(ctx, settings); err != nil {
			return err
		}
	}
	return s.initializeOnboarding(ctx, fresh)
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
	var settings domain.Settings
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
	if status != "completed" {
		return domain.OnboardingState{Status: "not_started"}, nil
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

func (s *Store) CreateSession(ctx context.Context, intent string, settings domain.Settings) (domain.Session, error) {
	if err := domain.ValidateIntent(intent); err != nil {
		return domain.Session{}, err
	}
	open, err := s.ActiveSession(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if open != nil {
		return domain.Session{}, errors.New("an active session already exists")
	}
	now := domain.Now()
	sessionID := domain.NewID("session")
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Session{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES(?,?,'queued',?,?,?)`, sessionID, strings.TrimSpace(intent), settings.MaxItemsPerSource, settings.MaxItemsTotal, now); err != nil {
		return domain.Session{}, err
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
	runs, err := s.listRuns(ctx, id)
	if err != nil {
		return domain.Session{}, err
	}
	session.Runs = runs
	items, err := s.ListSessionItems(ctx, id)
	if err != nil {
		return domain.Session{}, err
	}
	session.Items = items
	return session, nil
}

func (s *Store) listRuns(ctx context.Context, sessionID string) ([]domain.Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,session_id,source,ordinal,status,stage,created_at,started_at,completed_at,summary,coverage_json,error_json FROM runs WHERE session_id=? ORDER BY ordinal`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []domain.Run
	for rows.Next() {
		var run domain.Run
		var started, completed, coverageRaw, errorRaw sql.NullString
		if err := rows.Scan(&run.ID, &run.SessionID, &run.Source, &run.Ordinal, &run.Status, &run.Stage, &run.CreatedAt, &started, &completed, &run.Summary, &coverageRaw, &errorRaw); err != nil {
			return nil, err
		}
		if started.Valid {
			run.StartedAt = &started.String
		}
		if completed.Valid {
			run.CompletedAt = &completed.String
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
	var started, completed, coverageRaw, errorRaw sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id,session_id,source,ordinal,status,stage,created_at,started_at,completed_at,summary,coverage_json,error_json FROM runs WHERE id=?`, id).Scan(&run.ID, &run.SessionID, &run.Source, &run.Ordinal, &run.Status, &run.Stage, &run.CreatedAt, &started, &completed, &run.Summary, &coverageRaw, &errorRaw)
	if err != nil {
		return domain.Run{}, err
	}
	if started.Valid {
		run.StartedAt = &started.String
	}
	if completed.Valid {
		run.CompletedAt = &completed.String
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
	now := domain.Now()
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

func (s *Store) SaveObservation(ctx context.Context, commandID, runID string, observation domain.Observation) error {
	if !observation.Source.Valid() {
		return errors.New("observation source is invalid")
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var expectedRun string
	var status string
	if err = tx.QueryRowContext(ctx, `SELECT run_id,status FROM bridge_commands WHERE id=?`, commandID).Scan(&expectedRun, &status); err != nil {
		return err
	}
	if expectedRun != runID || status != "claimed" {
		return errors.New("bridge command is not claimable for this run")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES(?,?,?,?,?,?,?)`, domain.NewID("observation"), runID, commandID, observation.Source, string(raw), observation.CapturedAt, now); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bridge_commands SET status='completed',completed_at=? WHERE id=?`, now, commandID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning' WHERE id=?`, runID); err != nil {
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

func (s *Store) FailCommand(ctx context.Context, commandID, runID string, failure domain.Failure) error {
	raw, _ := json.Marshal(failure)
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE bridge_commands SET status='failed',completed_at=?,error_json=? WHERE id=? AND run_id=? AND status='claimed'`, now, string(raw), commandID, runID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return errors.New("bridge command is not active")
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=?`, failure.Stage, now, string(raw), runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailRun(ctx context.Context, runID string, failure domain.Failure) error {
	raw, _ := json.Marshal(failure)
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=? AND status NOT IN ('completed','failed','cancelled')`, failure.Stage, domain.Now(), string(raw), runID)
	return err
}

func (s *Store) SaveTelemetry(ctx context.Context, value domain.ReasoningTelemetry) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO reasoning_invocations(id,run_id,phase,provider,model,effort,duration_ms,status,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.RunID, value.Phase, value.Provider, value.Model, value.Effort, value.DurationMS, value.Status, value.InputTokens, value.CachedInputTokens, value.OutputTokens, value.ReasoningOutputTokens, value.CreatedAt)
	return err
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
	for _, value := range scored {
		byKey[value.Assessment.EvidenceKey] = value
		raw, _ := json.Marshal(value.Assessment)
		selected := 0
		if value.Selected {
			selected = 1
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, value.Assessment.EvidenceKey, run.Source, string(raw), value.BaseScore, value.PreferenceScore, value.FinalScore, selected, now); err != nil {
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
		if item.Item.EventKey != "" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(source,event_key) DO UPDATE SET evidence_key=excluded.evidence_key,item_json=excluded.item_json,last_seen_at=excluded.last_seen_at`, domain.NewID("knowledge"), item.Source, item.Item.EventKey, item.EvidenceKey, string(itemRaw), now, now); err != nil {
				return err
			}
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
	now := domain.Now()
	completed := 0
	failed := 0
	for _, run := range runs {
		if run.Status == "completed" {
			completed++
		}
		if run.Status == "failed" || run.Status == "cancelled" {
			failed++
		}
	}
	status := "failed"
	if completed == len(runs) {
		status = "completed"
	} else if completed > 0 {
		status = "partial"
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE sessions SET status=?,active_source=NULL,completed_at=? WHERE id=?`, status, now, sessionID); err != nil {
		return nil, err
	}
	return nil, nil
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

func (s *Store) ListSessionItems(ctx context.Context, sessionID string) ([]domain.TimelineItem, error) {
	return s.listItems(ctx, `WHERE session_id=? ORDER BY source,rank`, sessionID)
}
func (s *Store) ListTimeline(ctx context.Context, limit, offset int) ([]domain.TimelineItem, error) {
	return s.listItems(ctx, `ORDER BY created_at DESC,rank LIMIT ? OFFSET ?`, limit, offset)
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
	}
	return items, nil
}

func (s *Store) AddFeedback(ctx context.Context, timelineID string, input domain.Feedback) (domain.Feedback, error) {
	var sessionID, runID, evidenceKey string
	err := s.db.QueryRowContext(ctx, `SELECT session_id,run_id,evidence_key FROM timeline_items WHERE id=?`, timelineID).Scan(&sessionID, &runID, &evidenceKey)
	if err != nil {
		return domain.Feedback{}, err
	}
	input.ID = domain.NewID("feedback")
	input.TimelineID = timelineID
	input.SessionID = sessionID
	input.RunID = runID
	input.EvidenceKey = evidenceKey
	input.CreatedAt = domain.Now()
	if err = input.Validate(); err != nil {
		return domain.Feedback{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,reason,created_at) VALUES(?,?,?,?,?,?,?,?)`, input.ID, input.TimelineID, input.SessionID, input.RunID, input.EvidenceKey, input.Direction, input.Reason, input.CreatedAt)
	return input, err
}

type PreferenceSignal struct {
	Direction  string
	Reason     *string
	Origin     string
	Assessment domain.CandidateAssessment
}

func (s *Store) PreferenceSignals(ctx context.Context) ([]PreferenceSignal, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH signals AS (
		  SELECT run_id,evidence_key,direction,reason,'routine' AS origin,created_at
		  FROM feedback_events
		  UNION ALL
		  SELECT run_id,evidence_key,
		    CASE label WHEN 'more_like_this' THEN 'more' WHEN 'less_like_this' THEN 'less' ELSE 'neutral' END,
		    NULL,'calibration',labeled_at
		  FROM calibration_samples
		  WHERE label IS NOT NULL
		), ranked AS (
		  SELECT signals.*,ROW_NUMBER() OVER (
		    PARTITION BY run_id,evidence_key ORDER BY created_at DESC,origin DESC
		  ) AS signal_rank
		  FROM signals
		)
		SELECT ranked.direction,ranked.reason,ranked.origin,a.assessment_json
		FROM ranked
		JOIN candidate_assessments a
		  ON a.run_id=ranked.run_id AND a.evidence_key=ranked.evidence_key
		WHERE ranked.signal_rank=1
		ORDER BY ranked.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var signals []PreferenceSignal
	for rows.Next() {
		var signal PreferenceSignal
		var reason sql.NullString
		var raw string
		if err := rows.Scan(&signal.Direction, &reason, &signal.Origin, &raw); err != nil {
			return nil, err
		}
		if reason.Valid {
			signal.Reason = &reason.String
		}
		if err := json.Unmarshal([]byte(raw), &signal.Assessment); err != nil {
			return nil, err
		}
		signals = append(signals, signal)
	}
	return signals, rows.Err()
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

func (s *Store) ResetLearning(ctx context.Context) error {
	if err := s.requireIdle(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM calibration_sessions; DELETE FROM feedback_events; DELETE FROM preference_model; DELETE FROM meta WHERE key='calibration_first_run_status';`); err != nil {
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
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions; DELETE FROM feedback_events; DELETE FROM preference_model; DELETE FROM knowledge_events; DELETE FROM settings; DELETE FROM meta WHERE key='calibration_first_run_status';`); err != nil {
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
