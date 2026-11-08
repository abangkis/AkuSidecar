package store

import (
	"context"
	"encoding/json"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/timelinecomposition"
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
	var items []compositionItem
	var candidates []timelinecomposition.Candidate
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
		items = append(items, item)
		candidates = append(candidates, timelinecomposition.Candidate{
			Source: string(item.source), EvidenceKey: item.evidence,
			SemanticEventID: item.semanticEventID, SemanticRelation: item.semanticRelation,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	plan := timelinecomposition.Plan(candidates, limit)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, decision := range plan {
		item := items[decision.Index]
		if decision.Keep {
			if _, err := tx.ExecContext(ctx, `UPDATE timeline_items SET rank=? WHERE id=?`, decision.Rank, item.id); err != nil {
				return err
			}
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
