package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var allowedCaptureSurfaceEvents = map[string]bool{
	"created": true, "reused": true, "release_requested": true,
	"released": true, "preserved_user_owned": true,
	"focus_intervention": true, "reconciled": true,
}

func (s *Store) RecordCaptureSurfaceEvent(ctx context.Context, value domain.CaptureSurfaceEvent) (domain.CaptureSurfaceEvent, error) {
	if strings.TrimSpace(value.ID) == "" || len(value.ID) > 200 {
		return domain.CaptureSurfaceEvent{}, errors.New("capture surface event id is invalid")
	}
	if strings.TrimSpace(value.SessionID) == "" || len(value.SessionID) > 200 {
		return domain.CaptureSurfaceEvent{}, errors.New("capture surface session id is invalid")
	}
	if !allowedCaptureSurfaceEvents[value.Event] {
		return domain.CaptureSurfaceEvent{}, errors.New("capture surface event is unsupported")
	}
	if value.Source != "" && !value.Source.Valid() {
		return domain.CaptureSurfaceEvent{}, errors.New("capture surface source is invalid")
	}
	if len(value.Outcome) > 120 || len(value.OccurredAt) > 80 {
		return domain.CaptureSurfaceEvent{}, errors.New("capture surface event metadata is invalid")
	}
	if value.Detail == nil {
		value.Detail = map[string]any{}
	}
	detailRaw, err := json.Marshal(value.Detail)
	if err != nil || len(detailRaw) > 8*1024 {
		return domain.CaptureSurfaceEvent{}, errors.New("capture surface event detail is invalid")
	}
	if value.Source != "" && value.RunID == "" {
		_ = s.db.QueryRowContext(ctx,
			`SELECT id FROM runs WHERE session_id=? AND source=? ORDER BY ordinal LIMIT 1`,
			value.SessionID, value.Source,
		).Scan(&value.RunID)
	}
	if value.OccurredAt == "" {
		value.OccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO capture_surface_events(
		  id,session_id,run_id,source,event,outcome,detail_json,occurred_at
		) VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO NOTHING`,
		value.ID, value.SessionID, captureNullableString(value.RunID), captureNullableSource(value.Source),
		value.Event, value.Outcome, string(detailRaw), value.OccurredAt,
	)
	if err != nil {
		return domain.CaptureSurfaceEvent{}, err
	}
	return value, nil
}

func (s *Store) CaptureSurfaceEvents(ctx context.Context, runID string) ([]domain.CaptureSurfaceEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,session_id,COALESCE(run_id,''),COALESCE(source,''),event,outcome,detail_json,occurred_at
		FROM capture_surface_events
		WHERE run_id=?
		ORDER BY occurred_at,id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.CaptureSurfaceEvent{}
	for rows.Next() {
		var value domain.CaptureSurfaceEvent
		var detailRaw string
		if err := rows.Scan(
			&value.ID, &value.SessionID, &value.RunID, &value.Source,
			&value.Event, &value.Outcome, &detailRaw, &value.OccurredAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(detailRaw), &value.Detail); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func captureNullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func captureNullableSource(value domain.Source) any {
	if value == "" {
		return nil
	}
	return value
}
