package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

const splitActionAuditLimit = 512
const splitActionAuditTimeLayout = "2006-01-02T15:04:05.000000000Z07:00"

const splitActionAuditSQL = `
CREATE TABLE IF NOT EXISTS split_action_audit (
  action_id TEXT NOT NULL,
  action_type TEXT NOT NULL CHECK (action_type IN ('open_source','open_native_post')),
  phase TEXT NOT NULL CHECK (phase IN ('queued','claimed','reader_broker_attached','reader_prepare','reader_foreground','result')),
  outcome TEXT NOT NULL CHECK (outcome IN ('pending','accepted','rejected')),
  occurred_at TEXT NOT NULL,
  PRIMARY KEY (action_id, phase, outcome, occurred_at)
);
CREATE INDEX IF NOT EXISTS split_action_audit_occurred ON split_action_audit(occurred_at DESC, action_id);
`

type SplitActionAudit struct {
	ActionID   string
	ActionType string
	Phase      string
	Outcome    string
	OccurredAt string
}

func (s *Store) RecordSplitActionAudit(ctx context.Context, value SplitActionAudit) error {
	if !validSplitActionAuditID(value.ActionID) || !validSplitActionAuditType(value.ActionType) ||
		!validSplitActionAuditPhase(value.Phase) || !validSplitActionAuditOutcome(value.Outcome) {
		return errors.New("split action audit metadata is invalid")
	}
	if value.Outcome == "" {
		value.Outcome = "pending"
	}
	if value.OccurredAt == "" {
		value.OccurredAt = time.Now().UTC().Format(splitActionAuditTimeLayout)
	} else {
		parsed, err := time.Parse(time.RFC3339Nano, value.OccurredAt)
		if err != nil {
			return errors.New("split action audit timestamp is invalid")
		}
		value.OccurredAt = parsed.UTC().Format(splitActionAuditTimeLayout)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO split_action_audit(action_id,action_type,phase,outcome,occurred_at) VALUES(?,?,?,?,?)`,
		value.ActionID, value.ActionType, value.Phase, value.Outcome, value.OccurredAt); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM split_action_audit WHERE (action_id,phase,outcome,occurred_at) IN (
		SELECT action_id,phase,outcome,occurred_at FROM split_action_audit
		ORDER BY occurred_at DESC,action_id DESC,phase DESC,outcome DESC LIMIT -1 OFFSET ?
	)`, splitActionAuditLimit); err != nil {
		return err
	}
	return tx.Commit()
}

// ReadSplitActionAudit returns a bounded recent projection with no payload
// fields. since may be empty; a supplied value must be RFC3339Nano.
func (s *Store) ReadSplitActionAudit(ctx context.Context, since string, limit int) ([]SplitActionAudit, error) {
	if limit < 1 || limit > splitActionAuditLimit {
		return nil, errors.New("split action audit limit is out of range")
	}
	if strings.TrimSpace(since) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, since)
		if err != nil {
			return nil, errors.New("split action audit timestamp is invalid")
		}
		since = parsed.UTC().Format(splitActionAuditTimeLayout)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT action_id,action_type,phase,outcome,occurred_at
		FROM split_action_audit WHERE occurred_at>=? ORDER BY occurred_at DESC,action_id DESC,phase DESC,outcome DESC LIMIT ?`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SplitActionAudit, 0, limit)
	for rows.Next() {
		var value SplitActionAudit
		if err := rows.Scan(&value.ActionID, &value.ActionType, &value.Phase, &value.Outcome, &value.OccurredAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func validSplitActionAuditID(value string) bool {
	if len(value) < 1 || len(value) > 200 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func validSplitActionAuditType(value string) bool {
	return value == "open_source" || value == "open_native_post"
}

func validSplitActionAuditPhase(value string) bool {
	switch value {
	case "queued", "claimed", "reader_broker_attached", "reader_prepare", "reader_foreground", "result":
		return true
	default:
		return false
	}
}

func validSplitActionAuditOutcome(value string) bool {
	return value == "" || value == "pending" || value == "accepted" || value == "rejected"
}
