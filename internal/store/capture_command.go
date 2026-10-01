package store

import (
	"context"
	"encoding/json"
)

// CaptureCommandPayload returns persisted owner evidence for the exact command
// and run. SaveObservation still atomically verifies the claimed command state.
func (s *Store) CaptureCommandPayload(ctx context.Context, commandID, runID string) (map[string]any, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM bridge_commands WHERE id=? AND run_id=?`, commandID, runID).Scan(&raw); err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Store) ActiveMediaRecaptureIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM media_recaptures WHERE status IN ('queued','claimed') ORDER BY created_at`)
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
