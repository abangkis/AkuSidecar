package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/batchbehavior"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// SessionCaptureCommandsDrained reports whether every command for a session
// is terminal. A bounded ordinal supports safe hybrid-driver handoff after all
// sources preceding the next run have fully drained.
func (s *Store) SessionCaptureCommandsDrained(ctx context.Context, sessionID string, beforeOrdinal *int) (bool, error) {
	query := `SELECT COUNT(*) FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.session_id=? AND c.status IN ('queued','claimed')`
	args := []any{sessionID}
	if beforeOrdinal != nil {
		query += ` AND r.ordinal<?`
		args = append(args, *beforeOrdinal)
	}
	var outstanding int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&outstanding); err != nil {
		return false, err
	}
	return outstanding == 0, nil
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
	if err := requireSourceCaptureAllowed(ctx, tx, run.Source); err != nil {
		return domain.BridgeCommand{}, err
	}
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
	var source domain.Source
	if err := tx.QueryRowContext(ctx, `SELECT source FROM runs WHERE id=?`, runID).Scan(&source); err != nil {
		return domain.BridgeCommand{}, err
	}
	if err := requireSourceCaptureAllowed(ctx, tx, source); err != nil {
		return domain.BridgeCommand{}, err
	}
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
	return s.claimCommand(ctx, runID, bridgeID, "")
}

// ClaimCommandForCollector fences the immutable route before changing durable
// claim state. A command belonging to another consumer remains queued.
func (s *Store) ClaimCommandForCollector(ctx context.Context, runID, bridgeID, collector string) (*domain.BridgeCommand, error) {
	return s.claimCommand(ctx, runID, bridgeID, collector)
}

func (s *Store) claimCommand(ctx context.Context, runID, bridgeID, collector string) (*domain.BridgeCommand, error) {
	return s.claimCommandWithBehavior(ctx, runID, bridgeID, collector, batchbehavior.Policy{})
}

func (s *Store) claimCommandWithBehavior(ctx context.Context, runID, bridgeID, collector string, policy batchbehavior.Policy) (*domain.BridgeCommand, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var captureSource domain.Source
	if err := tx.QueryRowContext(ctx, `SELECT source FROM runs WHERE id=?`, runID).Scan(&captureSource); err != nil {
		return nil, err
	}
	if err := requireSourceCaptureAllowed(ctx, tx, captureSource); err != nil {
		return nil, err
	}
	gate, err := s.captureCommandClaimGate(ctx, tx, runID)
	if err != nil || gate.paused || gate.protected && !gate.allowed {
		return nil, err
	}
	if err := rejectManualPilotRun(ctx, tx, runID); err != nil {
		return nil, err
	}
	if allowed, err := s.captureBehaviorAllowed(ctx, tx, captureSource, policy); err != nil || !allowed {
		return nil, err
	}
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
	command.Payload, err = decodeCapturePayload(raw)
	if err != nil {
		return nil, err
	}
	if gate.protected {
		var source string
		if err := tx.QueryRowContext(ctx, `SELECT source FROM runs WHERE id=?`, command.RunID).Scan(&source); err != nil {
			return nil, errors.New("capture checkpoint route lookup failed")
		}
		if !accountCheckpointCommandEligible(gate.snapshot, domain.Source(source), command.Payload) {
			return nil, nil
		}
	}
	if collector != "" {
		route, err := CaptureCollector(command.Payload)
		if err != nil {
			return nil, err
		}
		if route != collector {
			return nil, nil
		}
	}
	if expiredQueuedBridgeCommand(command, s.Now()) {
		return nil, nil
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
	return &command, nil
}

// CaptureCollector preserves legacy routing while rejecting unknown stamped
// contracts. The runtime driver identifies the profile owner, not its consumer.
func CaptureCollector(payload map[string]any) (string, error) {
	if value, exists := payload["captureCollector"]; exists {
		stamp, ok := value.(map[string]any)
		if !ok || (stamp["version"] != 1 && stamp["version"] != float64(1)) {
			return "", errors.New("unsupported capture collector stamp")
		}
		backend, _ := stamp["backend"].(string)
		switch backend {
		case "bridge", "browser_quiet_hidden", "headless":
			return backend, nil
		default:
			return "", errors.New("unsupported capture collector backend")
		}
	}
	if runtime, ok := payload["captureRuntime"].(map[string]any); ok && runtime["driver"] == "headless" {
		return "headless", nil
	}
	return "bridge", nil
}

func decodeCapturePayload(raw string) (map[string]any, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, errors.New("invalid capture payload JSON")
	}
	return payload, nil
}

// FirstCommandCollector pins all acquisition rounds to the first durable
// command, including queued work restored before managed runtime attachment.
func (s *Store) FirstCommandCollector(ctx context.Context, runID string) (string, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM bridge_commands WHERE run_id=? ORDER BY created_at,rowid LIMIT 1`, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	payload, err := decodeCapturePayload(raw)
	if err != nil {
		return "", true, err
	}
	route, err := CaptureCollector(payload)
	return route, true, err
}

// FirstCommandCaptureDriver reads the first command's durable runtime stamp.
// It never derives a replacement from current settings or the attached owner.
func (s *Store) FirstCommandCaptureDriver(ctx context.Context, runID string) (string, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT payload_json FROM bridge_commands WHERE run_id=? ORDER BY created_at,rowid LIMIT 1`, runID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	payload, err := decodeCapturePayload(raw)
	if err != nil {
		return "", true, err
	}
	stamp, exists := payload["captureRuntime"]
	if !exists {
		return "", false, nil
	}
	identity, ok := stamp.(map[string]any)
	if !ok {
		return "", true, errors.New("invalid durable capture runtime stamp")
	}
	driver, ok := identity["driver"].(string)
	if !ok || (driver != "browser" && driver != "headless") {
		return "", true, errors.New("invalid durable capture runtime driver")
	}
	return driver, true, nil
}

func (s *Store) ExpiredBridgeCommands(ctx context.Context, now time.Time) ([]domain.BridgeCommand, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,run_id,type,status,payload_json,created_at,claimed_at
		FROM bridge_commands
		WHERE (status='queued' OR status='claimed' AND claimed_at IS NOT NULL)
		AND NOT EXISTS (SELECT 1 FROM runs r JOIN meta m ON m.key='manual_pilot_session_v1:'||r.session_id WHERE r.id=bridge_commands.run_id)
		ORDER BY claimed_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var expired []domain.BridgeCommand
	for rows.Next() {
		var command domain.BridgeCommand
		var raw string
		var claimedAt sql.NullString
		if err := rows.Scan(&command.ID, &command.RunID, &command.Type, &command.Status, &raw, &command.CreatedAt, &claimedAt); err != nil {
			return nil, err
		}
		decodeJSON(raw, &command.Payload)
		if command.Status == "queued" {
			if expiredQueuedBridgeCommand(command, now) {
				expired = append(expired, command)
			}
			continue
		}
		claimed, err := time.Parse(time.RFC3339Nano, claimedAt.String)
		if err != nil {
			continue
		}
		decodeJSON(raw, &command.Payload)
		if now.Before(claimed.Add(bridgeCommandLease(command.Payload))) {
			continue
		}
		command.ClaimedAt = &claimedAt.String
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

func (s *Store) PendingRunIDForCollector(ctx context.Context, collector string) (string, error) {
	return s.PendingRunIDForCollectorExcept(ctx, collector, "")
}

// PendingRunIDForCollectorExcept selects an ordinary route without changing
// admission policy. The Engine may reserve X for its private manual route.
func (s *Store) PendingRunIDForCollectorExcept(ctx context.Context, collector string, excluded domain.Source) (string, error) {
	return s.PendingRunIDForCollectorExceptSources(ctx, collector, []domain.Source{excluded})
}

func (s *Store) PendingRunIDForCollectorExceptSources(ctx context.Context, collector string, excluded []domain.Source) (string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id,c.payload_json,r.source FROM runs r JOIN bridge_commands c ON c.run_id=r.id WHERE r.status='waiting_for_bridge' AND c.status='queued' ORDER BY c.created_at,c.rowid`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		var source domain.Source
		if err := rows.Scan(&id, &raw, &source); err != nil {
			return "", err
		}
		skip := false
		for _, v := range excluded {
			if v == source {
				skip = true
			}
		}
		if skip {
			continue
		}
		payload, err := decodeCapturePayload(raw)
		if err != nil {
			continue // Corrupt work is never admitted as a legacy Bridge route.
		}
		route, routeErr := CaptureCollector(payload)
		if routeErr == nil && route == collector {
			return id, nil
		}
	}
	return "", rows.Err()
}

// Browser dispatch has a bounded pre-claim budget independent of capture time.
// Native collectors have their own owner deadline and must not be expired here.
const bridgeDispatchWait = 2 * time.Minute

func expiredQueuedBridgeCommand(command domain.BridgeCommand, now time.Time) bool {
	if command.Status != "queued" {
		return false
	}
	route, err := CaptureCollector(command.Payload)
	if err != nil || route != "bridge" {
		return false
	}
	created, err := time.Parse(time.RFC3339Nano, command.CreatedAt)
	return err == nil && !now.Before(created.Add(bridgeDispatchWait))
}
