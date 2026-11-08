package store

import (
	"context"
	"encoding/json"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type compositionItem struct {
	id               string
	source           domain.Source
	evidence         string
	score            float64
	itemRaw          string
	item             domain.ReasonedItem
	created          string
	semanticEventID  string
	semanticRelation string
}

// ComposeSession establishes one global personalized order after every source
// has finished. It prevents more than two consecutive items from one source
// whenever another source is still available.
func (s *Store) ComposeSession(ctx context.Context, sessionID string) error {
	var limit int
	if err := s.db.QueryRowContext(ctx, `SELECT max_items_total FROM sessions WHERE id=?`, sessionID).Scan(&limit); err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id,t.source,t.evidence_key,a.final_score,t.item_json,t.created_at,
		       COALESCE(r.event_id,''),COALESCE(r.relation,'')
		FROM timeline_items t
		JOIN candidate_assessments a ON a.run_id=t.run_id AND a.evidence_key=t.evidence_key
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE t.session_id=?
		ORDER BY a.final_score DESC,t.created_at,t.rank`, sessionID)
	if err != nil {
		return err
	}
	var remaining []compositionItem
	for rows.Next() {
		var item compositionItem
		if err := rows.Scan(&item.id, &item.source, &item.evidence, &item.score, &item.itemRaw, &item.created, &item.semanticEventID, &item.semanticRelation); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(item.itemRaw), &item.item); err != nil {
			rows.Close()
			return err
		}
		remaining = append(remaining, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	ordered := make([]compositionItem, 0, len(remaining))
	for len(remaining) > 0 {
		chosen := 0
		if len(ordered) >= 2 && ordered[len(ordered)-1].source == ordered[len(ordered)-2].source {
			for index, candidate := range remaining {
				if candidate.source != ordered[len(ordered)-1].source {
					chosen = index
					break
				}
			}
		}
		ordered = append(ordered, remaining[chosen])
		remaining = append(remaining[:chosen], remaining[chosen+1:]...)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	uniqueKept := 0
	duplicateKept := 0
	rank := 0
	duplicatesPerEvent := map[string]int{}
	for _, item := range ordered {
		keep := false
		if item.semanticRelation == "duplicate_report" {
			eventKey := item.semanticEventID
			if eventKey == "" {
				eventKey = item.evidence
			}
			if duplicateKept < limit && duplicatesPerEvent[eventKey] < 3 {
				keep = true
				duplicateKept++
				duplicatesPerEvent[eventKey]++
			}
		} else if uniqueKept < limit {
			keep = true
			uniqueKept++
		}
		if keep {
			if _, err := tx.ExecContext(ctx, `UPDATE timeline_items SET rank=? WHERE id=?`, rank, item.id); err != nil {
				return err
			}
			rank++
			if item.item.EventKey != "" {
				if err := saveKnowledgeEvidence(ctx, tx, item.source, item.evidence, item.itemRaw, item.created); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM timeline_items WHERE id=?`, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
