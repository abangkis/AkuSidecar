//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
)

// FixtureCommandEvidence is a read-only row snapshot. Payload bytes are hashed,
// never emitted; nullable claim/completion fields retain null versus empty.
type FixtureCommandEvidence struct {
	ID, RunID, Type, Status, CreatedAt, PayloadSHA256 string
	ClaimedBy, ClaimedAt, CompletedAt, ErrorJSON      sql.NullString
	Collector, Driver                                 string
}

func (s *Store) FixtureRunCommandEvidence(ctx context.Context, runID string) ([]FixtureCommandEvidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,run_id,type,status,payload_json,created_at,claimed_by,claimed_at,completed_at,error_json FROM bridge_commands WHERE run_id=? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FixtureCommandEvidence{}
	for rows.Next() {
		var v FixtureCommandEvidence
		var raw string
		if err := rows.Scan(&v.ID, &v.RunID, &v.Type, &v.Status, &raw, &v.CreatedAt, &v.ClaimedBy, &v.ClaimedAt, &v.CompletedAt, &v.ErrorJSON); err != nil {
			return nil, err
		}
		payload, err := decodeCapturePayload(raw)
		if err != nil {
			return nil, err
		}
		v.Collector, err = CaptureCollector(payload)
		if err != nil {
			return nil, err
		}
		if stamp, ok := payload["captureRuntime"].(map[string]any); ok {
			v.Driver, _ = stamp["driver"].(string)
		}
		v.PayloadSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
		result = append(result, v)
	}
	return result, rows.Err()
}
