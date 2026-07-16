package store

import (
	"context"
	"database/sql"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) ListInboxSessions(ctx context.Context, limit, offset int) ([]domain.InboxSession, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM sessions ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	sessions := make([]domain.InboxSession, 0, len(ids))
	for _, id := range ids {
		session, err := s.GetSession(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		entry := domain.InboxSession{ID: session.ID, Intent: session.Intent, Status: session.Status, CreatedAt: session.CreatedAt, StartedAt: session.StartedAt, CompletedAt: session.CompletedAt, Error: session.Error, Runs: make([]domain.InboxRun, 0, len(session.Runs))}
		for _, run := range session.Runs {
			diagnostic, err := s.inboxRun(ctx, run)
			if err != nil {
				return nil, 0, err
			}
			entry.CapturedCandidates += diagnostic.CapturedCandidates
			entry.EvaluatedCandidates += diagnostic.EvaluatedCandidates
			entry.SelectedCandidates += diagnostic.SelectedCandidates
			entry.AddedItems += diagnostic.AddedItems
			entry.Runs = append(entry.Runs, diagnostic)
		}
		sessions = append(sessions, entry)
	}
	return sessions, total, nil
}

func (s *Store) inboxRun(ctx context.Context, run domain.Run) (domain.InboxRun, error) {
	entry := domain.InboxRun{ID: run.ID, Source: run.Source, Status: run.Status, Stage: run.Stage, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Summary: run.Summary, Error: run.Error}
	observations, err := s.Observations(ctx, run.ID)
	if err != nil {
		return domain.InboxRun{}, err
	}
	evidence := map[string]bool{}
	entry.AcquisitionRounds = len(observations)
	for _, observation := range observations {
		entry.SnapshotCount += len(observation.Snapshots)
		entry.PerformedScrolls += integerValue(observation.Coverage["performedScrolls"])
		for _, snapshot := range observation.Snapshots {
			for _, block := range snapshot.Blocks {
				if block.EvidenceKey != "" {
					evidence[block.EvidenceKey] = true
				}
			}
		}
	}
	entry.CapturedCandidates = len(evidence)
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(selected),0) FROM candidate_assessments WHERE run_id=?`, run.ID).Scan(&entry.EvaluatedCandidates, &entry.SelectedCandidates); err != nil {
		return domain.InboxRun{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, run.ID).Scan(&entry.AddedItems); err != nil {
		return domain.InboxRun{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(duration_ms),0) FROM reasoning_invocations WHERE run_id=?`, run.ID).Scan(&entry.ReasoningDurationMS); err != nil && err != sql.ErrNoRows {
		return domain.InboxRun{}, err
	}
	var fallbackRaw sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT error_json FROM bridge_commands WHERE run_id=? AND status='failed' AND error_json IS NOT NULL ORDER BY created_at DESC LIMIT 1`, run.ID).Scan(&fallbackRaw); err != nil && err != sql.ErrNoRows {
		return domain.InboxRun{}, err
	}
	if fallbackRaw.Valid && run.Status == "completed" {
		var failure domain.Failure
		decodeJSON(fallbackRaw.String, &failure)
		entry.FollowUpFallback = &failure
	}
	return entry, nil
}

func integerValue(value any) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	case int64:
		return int(number)
	default:
		return 0
	}
}
