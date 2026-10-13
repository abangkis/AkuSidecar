//go:build auth_session_fixture

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strings"
)

const fixtureCapturePausePrefix = "fixture_capture_pause:"

func fixtureCaptureClaimPaused(ctx context.Context, tx *sql.Tx, runID string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta m JOIN runs r ON m.key=? || r.session_id WHERE r.id=?`, fixtureCapturePausePrefix, runID).Scan(&count)
	return count != 0, err
}

// FixtureCaptureSessionPaused is also used by the fixture claim admission gate.
func (s *Store) FixtureCaptureSessionPaused(ctx context.Context, sessionID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID).Scan(&count)
	return count != 0, err
}

func fixtureCaptureCommands(ctx context.Context, tx *sql.Tx, sessionID string) (map[string]map[string]any, error) {
	var claimed, media int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.session_id=? AND c.status='claimed'`, sessionID).Scan(&claimed); err != nil {
		return nil, err
	}
	if claimed != 0 {
		return nil, errors.New("fixture capture commands have not drained")
	}
	// Recaptures have independent profile leases; this bounded fixture cannot pause them.
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_recaptures WHERE status IN ('queued','claimed')`).Scan(&media); err != nil {
		return nil, err
	}
	if media != 0 {
		return nil, errors.New("fixture pause does not support active media recaptures")
	}
	rows, err := tx.QueryContext(ctx, `SELECT c.id,c.payload_json,c.claimed_at,c.claimed_by FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.session_id=? AND c.status='queued'`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	commands := map[string]map[string]any{}
	for rows.Next() {
		var id, raw string
		var claimedAt, claimedBy sql.NullString
		if err := rows.Scan(&id, &raw, &claimedAt, &claimedBy); err != nil {
			return nil, err
		}
		if (claimedAt.Valid && claimedAt.String != "") || (claimedBy.Valid && claimedBy.String != "") {
			return nil, errors.New("fixture pause requires never-claimed queued commands")
		}
		payload, err := decodeCapturePayload(raw)
		if err != nil {
			return nil, err
		}
		collector, err := CaptureCollector(payload)
		if err != nil || collector != "headless" {
			return nil, errors.New("fixture pause requires headless command routes")
		}
		stamp, ok := payload["captureRuntime"].(map[string]any)
		if !ok || stamp["driver"] != "headless" {
			return nil, errors.New("fixture capture runtime stamp is invalid")
		}
		epoch, ok := stamp["epoch"].(string)
		generation, numeric := stamp["generation"].(float64)
		if !ok || strings.TrimSpace(epoch) == "" || !numeric || math.IsNaN(generation) || math.IsInf(generation, 0) || generation < 1 || generation > 9007199254740991 || math.Trunc(generation) != generation {
			return nil, errors.New("fixture capture runtime stamp is invalid")
		}
		commands[id] = payload
	}
	return commands, rows.Err()
}

func (s *Store) PauseFixtureCaptureSession(ctx context.Context, sessionID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sessions WHERE id=?`, sessionID).Scan(&status); err != nil {
		return err
	}
	if status != "queued" && status != "running" {
		// Terminal work can leave a marker after cancellation during a reader.
		// Clearing it is safe; no command or session state is changed.
		if _, err := tx.ExecContext(ctx, `DELETE FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID); err != nil {
			return err
		}
		return tx.Commit()
	}
	if _, err := fixtureCaptureCommands(ctx, tx, sessionID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,'1') ON CONFLICT(key) DO UPDATE SET value='1'`, fixtureCapturePausePrefix+sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ResumeFixtureCaptureSession(ctx context.Context, sessionID, epoch string, generation uint64) error {
	if strings.TrimSpace(epoch) == "" || generation == 0 || generation > 9007199254740991 {
		return errors.New("fixture resume requires an owner epoch and generation")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var marker, status string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID).Scan(&marker); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT status FROM sessions WHERE id=?`, sessionID).Scan(&status); err != nil {
		return err
	}
	if status == "queued" || status == "running" {
		commands, err := fixtureCaptureCommands(ctx, tx, sessionID)
		if err != nil {
			return err
		}
		for id, payload := range commands {
			stamp := payload["captureRuntime"].(map[string]any)
			stamp["epoch"], stamp["generation"] = epoch, generation
			raw, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE bridge_commands SET payload_json=? WHERE id=? AND status='queued'`, string(raw), id); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListFixturePausedCaptureSessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM meta WHERE substr(key,1,?)=? ORDER BY key`, len(fixtureCapturePausePrefix), fixtureCapturePausePrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		ids = append(ids, strings.TrimPrefix(key, fixtureCapturePausePrefix))
	}
	return ids, rows.Err()
}
