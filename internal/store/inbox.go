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
		entry.EventResolution, err = s.EventResolutionSummary(ctx, session.ID)
		if err != nil {
			return nil, 0, err
		}
		entry.AIDetection, err = s.AIDetectionJob(ctx, session.ID)
		if err != nil {
			return nil, 0, err
		}
		if entry.EventResolution != nil {
			entry.DuplicateReports = entry.EventResolution.DuplicateReports
			entry.AddedItems = entry.EventResolution.UniqueItems
		}
		sessions = append(sessions, entry)
	}
	return sessions, total, nil
}

func (s *Store) LatestTimelineCheck(ctx context.Context) (*domain.TimelineCheckSummary, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	var value domain.TimelineCheckSummary
	if settings.SemanticEventMode == "show_all" {
		err = s.db.QueryRowContext(ctx,
			"SELECT s.id,s.status,s.completed_at,COUNT(t.id),0 "+
				"FROM sessions s LEFT JOIN timeline_items t ON t.session_id=s.id "+
				"WHERE s.status IN ('completed','partial') AND s.completed_at IS NOT NULL "+
				"GROUP BY s.id,s.status,s.completed_at ORDER BY s.completed_at DESC LIMIT 1").
			Scan(&value.SessionID, &value.Status, &value.CompletedAt, &value.AddedItems, &value.DuplicateReports)
	} else {
		err = s.db.QueryRowContext(ctx,
			"SELECT s.id,s.status,s.completed_at,"+
				"SUM(CASE WHEN t.id IS NULL OR r.relation='duplicate_report' THEN 0 ELSE 1 END),"+
				"SUM(CASE WHEN t.id IS NOT NULL AND r.relation='duplicate_report' THEN 1 ELSE 0 END) "+
				"FROM sessions s LEFT JOIN timeline_items t ON t.session_id=s.id "+
				"LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id "+
				"WHERE s.status IN ('completed','partial') AND s.completed_at IS NOT NULL "+
				"GROUP BY s.id,s.status,s.completed_at ORDER BY s.completed_at DESC LIMIT 1").
			Scan(&value.SessionID, &value.Status, &value.CompletedAt, &value.AddedItems, &value.DuplicateReports)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
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
