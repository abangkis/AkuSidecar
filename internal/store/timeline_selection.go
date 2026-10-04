package store

import (
	"context"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Select using identity/relation only. Detail hydration must not run on the
// history outside this page. The existing 1000-candidate horizon, ordering,
// unique-item offset and trailing collapsed reports are deliberately retained.
func (s *Store) timelinePageIDs(ctx context.Context, mode string, limit, offset int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,
		EXISTS(SELECT 1 FROM semantic_event_reports r JOIN semantic_events e ON e.id=r.event_id
		       WHERE r.timeline_id=timeline_items.id AND r.relation='duplicate_report')
		FROM timeline_items
		WHERE COALESCE((SELECT state FROM auto_update_batches b WHERE b.session_id=timeline_items.session_id),batch_state) IN ('','visible')`+timelinePresentationOrderSQL+` LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	uniqueSeen, uniqueIncluded := 0, 0
	for rows.Next() {
		var id string
		var duplicate bool
		if err := rows.Scan(&id, &duplicate); err != nil {
			return nil, err
		}
		if duplicate {
			if mode == "collapse" && uniqueSeen >= offset && uniqueIncluded <= limit {
				ids = append(ids, id)
			}
			continue
		}
		if uniqueSeen < offset {
			uniqueSeen++
			continue
		}
		if uniqueIncluded >= limit {
			break
		}
		ids = append(ids, id)
		uniqueSeen++
		uniqueIncluded++
	}
	return ids, rows.Err()
}

func (s *Store) hydrateTimelinePage(ctx context.Context, ids []string) ([]domain.TimelineItem, error) {
	if len(ids) == 0 {
		return []domain.TimelineItem{}, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i], args[i] = "?", id
	}
	items, err := s.listItems(ctx, `WHERE id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	// IN has no defined order. Restore the candidate ordering, including ties.
	byID := make(map[string]domain.TimelineItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]domain.TimelineItem, 0, len(ids))
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			result = append(result, item)
		}
	}
	return result, nil
}
