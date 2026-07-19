package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

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
		entry := domain.InboxSession{ID: session.ID, Intent: session.Intent, Status: session.Status, CreatedAt: session.CreatedAt, StartedAt: session.StartedAt, CompletedAt: session.CompletedAt, Error: session.Error, PreferenceDecisions: []domain.InboxPreferenceDecision{}, Runs: make([]domain.InboxRun, 0, len(session.Runs))}
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
		entry.PreferenceDecisions, err = s.inboxPreferenceDecisions(ctx, session.ID)
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

func (s *Store) inboxPreferenceDecisions(ctx context.Context, sessionID string) ([]domain.InboxPreferenceDecision, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH ranked AS (
		  SELECT f.timeline_id,f.evidence_key,t.source,f.direction,f.created_at,
		    ROW_NUMBER() OVER (
		      PARTITION BY t.source,f.evidence_key
		      ORDER BY f.created_at DESC,f.id DESC
		    ) AS signal_rank
		  FROM feedback_events f
		  JOIN timeline_items t ON t.id=f.timeline_id
		)
		SELECT t.id,t.evidence_key,t.source,t.item_json,r.direction,r.created_at
		FROM timeline_items t
		JOIN ranked r ON r.timeline_id=t.id AND r.signal_rank=1
		WHERE t.session_id=?
		ORDER BY t.rank,t.created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	decisions := []domain.InboxPreferenceDecision{}
	for rows.Next() {
		var decision domain.InboxPreferenceDecision
		var itemRaw string
		if err := rows.Scan(
			&decision.TimelineID,
			&decision.EvidenceKey,
			&decision.Source,
			&itemRaw,
			&decision.Direction,
			&decision.UpdatedAt,
		); err != nil {
			return nil, err
		}
		var item domain.ReasonedItem
		if err := json.Unmarshal([]byte(itemRaw), &item); err != nil {
			return nil, err
		}
		decision.Author = item.Author
		decision.Summary = item.WhatChanged
		decision.SourceURL = item.SourceURL
		decisions = append(decisions, decision)
	}
	return decisions, rows.Err()
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
	entry := domain.InboxRun{ID: run.ID, Source: run.Source, Status: run.Status, Stage: run.Stage, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt, Summary: run.Summary, Error: run.Error, StageDurationsMS: map[string]int64{}}
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
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),COALESCE(SUM(CASE WHEN a.selected=1 OR EXISTS (
		  SELECT 1 FROM selection_corrections c
		  WHERE c.run_id=a.run_id AND c.evidence_key=a.evidence_key AND c.undone_at IS NULL
		) THEN 1 ELSE 0 END),0)
		FROM candidate_assessments a WHERE a.run_id=?`, run.ID).Scan(&entry.EvaluatedCandidates, &entry.SelectedCandidates); err != nil {
		return domain.InboxRun{}, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM timeline_items t
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE t.run_id=? AND COALESCE(r.relation,'') != 'duplicate_report'`, run.ID).Scan(&entry.AddedItems); err != nil {
		return domain.InboxRun{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(duration_ms),0) FROM reasoning_invocations WHERE run_id=?`, run.ID).Scan(&entry.ReasoningDurationMS); err != nil && err != sql.ErrNoRows {
		return domain.InboxRun{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT stage,duration_ms FROM run_stage_timings WHERE run_id=?`, run.ID)
	if err != nil {
		return domain.InboxRun{}, err
	}
	for rows.Next() {
		var stage string
		var duration int64
		if err := rows.Scan(&stage, &duration); err != nil {
			rows.Close()
			return domain.InboxRun{}, err
		}
		entry.StageDurationsMS[stage] = duration
	}
	if err := rows.Close(); err != nil {
		return domain.InboxRun{}, err
	}
	if run.StartedAt != nil && run.CompletedAt != nil {
		started, startedErr := time.Parse(time.RFC3339Nano, *run.StartedAt)
		completed, completedErr := time.Parse(time.RFC3339Nano, *run.CompletedAt)
		if startedErr == nil && completedErr == nil && !completed.Before(started) {
			entry.TotalDurationMS = completed.Sub(started).Milliseconds()
		}
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),COALESCE(SUM(CASE WHEN action='fail_fast' THEN 1 ELSE 0 END),0)
		FROM content_continuity_occurrences WHERE run_id=? AND status!='fresh'`, run.ID).
		Scan(&entry.ResurfacedItems, &entry.SkippedResurfaces); err != nil {
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
