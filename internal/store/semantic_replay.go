package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// SemanticReplaySession is a bounded, read-only projection of the durable
// semantic ledger. It deliberately contains only the fields needed by the
// assessment analyzer; the analyzer is responsible for never serializing the
// source evidence represented by the private text fields.
type SemanticReplaySession struct {
	Status               string
	InvocationStatus     string
	ResolverInvoked      bool
	DiagnosticsAvailable bool
	StrongestOverlap     int
	CandidateCount       int
	ShortlistCount       int
	TriggerReason        string
	SignalReceipt        *domain.SemanticSignalReceipt
	ActiveCorrections    int
	UndoneCorrections    int
	Reports              []SemanticReplayReport
}

type SemanticReplayReport struct {
	Relation string
}

type SemanticReplayEvent struct {
	CanonicalClaim string
	Actor          string
	Action         string
	Object         string
	EventKind      string
	Aliases        []string
}

const (
	semanticReplaySessionLimit = 500
	semanticReplayReportLimit  = 30
	semanticReplayEventLimit   = 1000
)

// ReadSemanticReplay returns deterministic, bounded projections of completed
// semantic history. It only executes SELECT statements and is safe to call on
// a Store opened with OpenReadOnly.
func (s *Store) ReadSemanticReplay(ctx context.Context, limit int) ([]SemanticReplaySession, []SemanticReplayEvent, error) {
	if limit <= 0 || limit > semanticReplaySessionLimit {
		return nil, nil, fmt.Errorf("semantic replay session limit must be between 1 and %d", semanticReplaySessionLimit)
	}
	receiptColumn, err := s.semanticReplayReceiptColumn(ctx)
	if err != nil {
		return nil, nil, err
	}
	receiptExpression := "'{}'"
	if receiptColumn {
		receiptExpression = "COALESCE(d.receipt_json,'{}')"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT s.status,
		       COALESCE(i.status,'not_recorded'),
		       CASE WHEN d.session_id IS NULL THEN 0 ELSE 1 END,
		       COALESCE(d.resolver_invoked,0),
		       COALESCE(d.strongest_overlap,0),
		       COALESCE(i.candidate_count,0),
		       COALESCE(i.shortlist_count,0),
		       COALESCE(d.trigger_reason,'unavailable'),
		       %s,
		       (SELECT COUNT(*) FROM semantic_event_corrections c
		          JOIN semantic_event_reports r ON r.id=c.report_id
		         WHERE r.session_id=s.id AND c.undone_at IS NULL),
		       (SELECT COUNT(*) FROM semantic_event_corrections c
		          JOIN semantic_event_reports r ON r.id=c.report_id
		         WHERE r.session_id=s.id AND c.undone_at IS NOT NULL)
		FROM sessions s
		LEFT JOIN event_resolution_invocations i ON i.session_id=s.id
		LEFT JOIN event_resolution_diagnostics d ON d.session_id=s.id
		WHERE s.status IN ('completed','partial','failed','cancelled')
		ORDER BY s.created_at DESC,s.id DESC
		LIMIT ?`, receiptExpression), limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	result := make([]SemanticReplaySession, 0, limit)
	for rows.Next() {
		var session SemanticReplaySession
		var diagnostics, invoked int
		var receiptRaw string
		if err := rows.Scan(&session.Status, &session.InvocationStatus, &diagnostics, &invoked, &session.StrongestOverlap, &session.CandidateCount, &session.ShortlistCount, &session.TriggerReason, &receiptRaw, &session.ActiveCorrections, &session.UndoneCorrections); err != nil {
			return nil, nil, err
		}
		session.DiagnosticsAvailable = diagnostics != 0
		var receipt domain.SemanticSignalReceipt
		if err := json.Unmarshal([]byte(receiptRaw), &receipt); err == nil && receipt.Version == "semantic-signal-receipt-v1" {
			session.SignalReceipt = &receipt
		}
		session.ResolverInvoked = invoked != 0
		result = append(result, session)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, nil, err
	}

	// The report query is intentionally independent of session IDs in its
	// returned shape. We use an internal ordered session-id query only to attach
	// reports, then discard those identifiers before returning.
	ids, err := s.replaySessionIDs(ctx, limit)
	if err != nil {
		return nil, nil, err
	}
	for index, sessionID := range ids {
		if index >= len(result) {
			break
		}
		reports, err := s.readSemanticReplayReports(ctx, sessionID)
		if err != nil {
			return nil, nil, err
		}
		result[index].Reports = reports
	}

	events, err := s.readSemanticReplayEvents(ctx)
	if err != nil {
		return nil, nil, err
	}
	return result, events, nil
}

func (s *Store) semanticReplayReceiptColumn(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('event_resolution_diagnostics') WHERE name='receipt_json'`).Scan(&count); err != nil {
		return false, err
	}
	return count == 1, nil
}

func (s *Store) replaySessionIDs(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id FROM sessions
		WHERE status IN ('completed','partial','failed','cancelled')
		ORDER BY created_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) readSemanticReplayReports(ctx context.Context, sessionID string) ([]SemanticReplayReport, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.relation
		FROM semantic_event_reports r
		WHERE r.session_id=?
		ORDER BY r.created_at,r.id
		LIMIT ?`, sessionID, semanticReplayReportLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SemanticReplayReport, 0, semanticReplayReportLimit)
	for rows.Next() {
		var value SemanticReplayReport
		if err := rows.Scan(&value.Relation); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) readSemanticReplayEvents(ctx context.Context) ([]SemanticReplayEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT canonical_claim,actor,action,object,aliases_json
		FROM semantic_events
		ORDER BY last_seen_at DESC,id DESC
		LIMIT ?`, semanticReplayEventLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SemanticReplayEvent, 0, semanticReplayEventLimit)
	for rows.Next() {
		var value SemanticReplayEvent
		var aliasesRaw string
		if err := rows.Scan(&value.CanonicalClaim, &value.Actor, &value.Action, &value.Object, &aliasesRaw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(aliasesRaw), &value.Aliases)
		result = append(result, value)
	}
	return result, rows.Err()
}
