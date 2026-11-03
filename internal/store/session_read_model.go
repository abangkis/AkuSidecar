package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

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

func (s *Store) listRuns(ctx context.Context, sessionID string) ([]domain.Run, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT runs.id,runs.session_id,runs.source,runs.ordinal,runs.status,runs.stage,runs.created_at,runs.started_at,runs.completed_at,runs.summary,runs.coverage_json,runs.error_json,(SELECT bridge_commands.status FROM bridge_commands WHERE bridge_commands.run_id=runs.id ORDER BY bridge_commands.created_at DESC LIMIT 1),(SELECT payload_json FROM bridge_commands WHERE bridge_commands.run_id=runs.id ORDER BY created_at,rowid LIMIT 1) FROM runs WHERE runs.session_id=? ORDER BY runs.ordinal`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var runs []domain.Run
	for rows.Next() {
		var run domain.Run
		var started, completed, coverageRaw, errorRaw, commandStatus, commandPayload sql.NullString
		if err := rows.Scan(&run.ID, &run.SessionID, &run.Source, &run.Ordinal, &run.Status, &run.Stage, &run.CreatedAt, &started, &completed, &run.Summary, &coverageRaw, &errorRaw, &commandStatus, &commandPayload); err != nil {
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
		run.CaptureCollector = projectedCaptureCollector(commandPayload)
		decodeJSON(coverageRaw.String, &run.Coverage)
		if !commandPayload.Valid && run.Coverage["captureLifecycleOwner"] == "native_daily_v1" {
			run.CaptureCollector = "headless"
		}
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
	var started, completed, coverageRaw, errorRaw, commandStatus, commandPayload sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT runs.id,runs.session_id,runs.source,runs.ordinal,runs.status,runs.stage,runs.created_at,runs.started_at,runs.completed_at,runs.summary,runs.coverage_json,runs.error_json,(SELECT bridge_commands.status FROM bridge_commands WHERE bridge_commands.run_id=runs.id ORDER BY bridge_commands.created_at DESC LIMIT 1),(SELECT payload_json FROM bridge_commands WHERE bridge_commands.run_id=runs.id ORDER BY created_at,rowid LIMIT 1) FROM runs WHERE runs.id=?`, id).Scan(&run.ID, &run.SessionID, &run.Source, &run.Ordinal, &run.Status, &run.Stage, &run.CreatedAt, &started, &completed, &run.Summary, &coverageRaw, &errorRaw, &commandStatus, &commandPayload)
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
	run.CaptureCollector = projectedCaptureCollector(commandPayload)
	decodeJSON(coverageRaw.String, &run.Coverage)
	if !commandPayload.Valid && run.Coverage["captureLifecycleOwner"] == "native_daily_v1" {
		run.CaptureCollector = "headless"
	}
	if errorRaw.Valid {
		var failure domain.Failure
		decodeJSON(errorRaw.String, &failure)
		run.Error = &failure
	}
	return run, nil
}

// projectedCaptureCollector is lifecycle metadata, never proof or authority.
func projectedCaptureCollector(raw sql.NullString) string {
	if !raw.Valid {
		return ""
	}
	payload, err := decodeCapturePayload(raw.String)
	if err != nil {
		return "unknown"
	}
	route, err := CaptureCollector(payload)
	if err != nil {
		return "unknown"
	}
	return route
}
