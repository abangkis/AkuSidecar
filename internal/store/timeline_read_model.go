package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) ListSessionItems(ctx context.Context, sessionID string) ([]domain.TimelineItem, error) {
	return s.listItems(ctx, `WHERE session_id=? ORDER BY rank`, sessionID)
}
func (s *Store) TimelineItem(ctx context.Context, timelineID string) (domain.TimelineItem, error) {
	items, err := s.listItems(ctx, `WHERE id=?`, timelineID)
	if err != nil {
		return domain.TimelineItem{}, err
	}
	if len(items) != 1 {
		return domain.TimelineItem{}, sql.ErrNoRows
	}
	return items[0], nil
}

const timelinePresentationOrderSQL = `
	ORDER BY
	  CASE WHEN COALESCE((SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),presentation)='append' THEN 1 ELSE 0 END,
	  CASE WHEN COALESCE((SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),presentation)<>'append'
	    THEN COALESCE(
	      (SELECT b.revealed_at FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),
	      (SELECT s.completed_at FROM sessions s WHERE s.id=timeline_items.session_id),presented_at,created_at
	    )
	  END DESC,
	  CASE WHEN COALESCE((SELECT json_extract(s.coverage_json,'$.timelinePresentation') FROM sessions s WHERE s.id=timeline_items.session_id),presentation)='append'
	    THEN COALESCE(
	      (SELECT b.revealed_at FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),
	      (SELECT s.completed_at FROM sessions s WHERE s.id=timeline_items.session_id),presented_at,created_at
	    )
	  END ASC,
	  rank`

func (s *Store) ListTimeline(ctx context.Context, limit, offset int) ([]domain.TimelineItem, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.SemanticEventMode == "show_all" {
		items, err := s.listItems(ctx, `WHERE COALESCE((SELECT state FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),batch_state) IN ('','visible')`+timelinePresentationOrderSQL+` LIMIT ? OFFSET ?`, limit, offset)
		if err != nil {
			return nil, err
		}
		for index := range items {
			items[index].SemanticEvent = nil
		}
		projectTimelineRelations(items)
		return items, nil
	}
	ids, err := s.timelinePageIDs(ctx, settings.SemanticEventMode, limit, offset)
	if err != nil {
		return nil, err
	}
	items, err := s.hydrateTimelinePage(ctx, ids)
	if err != nil {
		return nil, err
	}
	projectTimelineRelations(items)
	return items, nil
}

func (s *Store) listItems(ctx context.Context, suffix string, args ...any) ([]domain.TimelineItem, error) {
	query := `SELECT id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at,COALESCE(evidence_snapshot_json,''),EXISTS(SELECT 1 FROM sessions s WHERE s.id=timeline_items.session_id),EXISTS(SELECT 1 FROM runs r WHERE r.id=timeline_items.run_id) FROM timeline_items ` + suffix
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.TimelineItem
	for rows.Next() {
		var item domain.TimelineItem
		var itemRaw, assessmentRaw, coverageRaw, evidenceRaw string
		if err := rows.Scan(&item.ID, &item.SessionID, &item.RunID, &item.Source, &item.EvidenceKey, &item.Rank, &itemRaw, &assessmentRaw, &coverageRaw, &item.CreatedAt, &evidenceRaw, &item.OriginSessionAvailable, &item.OriginRunAvailable); err != nil {
			return nil, err
		}
		decodeJSON(itemRaw, &item.Item)
		decodeJSON(assessmentRaw, &item.Assessment)
		decodeJSON(coverageRaw, &item.Coverage)
		if evidenceRaw != "" {
			var evidence domain.Block
			decodeJSON(evidenceRaw, &evidence)
			item.Evidence = &evidence
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	evidenceByRun := map[string]map[string]domain.Block{}
	for index := range items {
		item := &items[index]
		byKey, loaded := evidenceByRun[item.RunID]
		if !loaded {
			byKey = map[string]domain.Block{}
			observations, err := s.Observations(ctx, item.RunID)
			if err != nil {
				return nil, err
			}
			for _, observation := range observations {
				for _, snapshot := range observation.Snapshots {
					for _, block := range snapshot.Blocks {
						if block.EvidenceKey != "" {
							if _, exists := byKey[block.EvidenceKey]; !exists {
								byKey[block.EvidenceKey] = block
							}
						}
					}
				}
			}
			evidenceByRun[item.RunID] = byKey
		}
		if block, exists := byKey[item.EvidenceKey]; exists {
			copy := block
			item.Evidence = &copy
		}
		var overrideRaw string
		if err := s.db.QueryRowContext(ctx, `SELECT evidence_json FROM timeline_evidence_overrides WHERE timeline_id=?`, item.ID).Scan(&overrideRaw); err == nil {
			var override domain.Block
			decodeJSON(overrideRaw, &override)
			item.Evidence = &override
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if err := s.attachSemanticEvents(ctx, items); err != nil {
		return nil, err
	}
	if err := s.attachAIDetections(ctx, items); err != nil {
		return nil, err
	}
	if err := s.attachEffectivePreferenceDecision(ctx, items); err != nil {
		return nil, err
	}
	if err := s.attachTimelineMemoryProjections(ctx, items); err != nil {
		return nil, err
	}
	return items, nil
}

// attachTimelineMemoryProjections restores only the bounded retention state
// for a Timeline item. The lookup is intentionally best-effort for an item that has
// no valid memory identity yet; Timeline reads must remain useful for neutral
// or media-only entries that were never kept. Full text never crosses this
// projection boundary.
func (s *Store) attachTimelineMemoryProjections(ctx context.Context, items []domain.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin Timeline memory projection: %w", err)
	}
	defer tx.Rollback()
	for index := range items {
		normalized, normalizeErr := normalizeMemoryInput(routineMoreMemoryInput(items[index]))
		if normalizeErr != nil {
			continue
		}
		memoryID, resolveErr := resolveMemoryIdentity(ctx, tx, normalized.Identity)
		if resolveErr != nil {
			return fmt.Errorf("resolve Timeline memory projection: %w", resolveErr)
		}
		if memoryID == "" {
			continue
		}
		memory, memoryErr := memoryItemByQueryer(ctx, tx, memoryID)
		if errors.Is(memoryErr, ErrMemoryNotFound) {
			continue
		}
		if memoryErr != nil {
			return fmt.Errorf("read Timeline memory projection: %w", memoryErr)
		}
		if memory.LifecycleState != domain.MemoryStateActive {
			continue
		}
		items[index].PersonalMemory = &domain.TimelineMemoryProjection{
			RetentionTier: memory.RetentionTier,
			Saved:         memory.Saved,
			PermanentKeep: memory.PermanentKeep,
		}
	}
	return nil
}

func (s *Store) attachEffectivePreferenceDecision(ctx context.Context, items []domain.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	placeholders := make([]string, len(items))
	args := make([]any, len(items))
	itemsByID := make(map[string]*domain.TimelineItem, len(items))
	for index := range items {
		placeholders[index] = "(?)"
		args[index] = items[index].ID
		itemsByID[items[index].ID] = &items[index]
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH requested(timeline_id) AS (VALUES `+strings.Join(placeholders, ",")+`),
		decisions AS (
		  SELECT f.id,f.timeline_id,f.direction,f.reason,f.created_at,'routine' AS origin,2 AS authority
		  FROM feedback_events f
		  JOIN requested q ON q.timeline_id=f.timeline_id
		  UNION ALL
		  SELECT 'calibration:' || c.calibration_session_id || ':' || c.ordinal,t.id,
		    CASE c.label WHEN 'more_like_this' THEN 'more' ELSE 'less' END,
		    CASE c.label WHEN 'less_like_this' THEN 'not_interested' ELSE NULL END,
		    c.labeled_at,'calibration',1
		  FROM timeline_items t
		  JOIN requested q ON q.timeline_id=t.id
		  JOIN calibration_samples c ON c.run_id=t.run_id AND c.evidence_key=t.evidence_key
		  WHERE c.label IN ('more_like_this','less_like_this')
		), ranked AS (
		  SELECT decisions.*,
		    ROW_NUMBER() OVER (PARTITION BY timeline_id ORDER BY created_at DESC,authority DESC,id DESC) AS signal_rank
		  FROM decisions
		)
		SELECT id,timeline_id,direction,reason,origin,created_at
		FROM ranked WHERE signal_rank=1`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, timelineID, direction, origin, createdAt string
		var reason sql.NullString
		if err := rows.Scan(&id, &timelineID, &direction, &reason, &origin, &createdAt); err != nil {
			return err
		}
		item := itemsByID[timelineID]
		if item == nil {
			continue
		}
		feedback := domain.Feedback{
			ID: id, TimelineID: timelineID, SessionID: item.SessionID, RunID: item.RunID,
			EvidenceKey: item.EvidenceKey, Direction: direction, Origin: origin, CreatedAt: createdAt,
		}
		if reason.Valid {
			feedback.Reason = &reason.String
		}
		item.Feedback = &feedback
	}
	return rows.Err()
}
