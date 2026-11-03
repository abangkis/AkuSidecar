package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) Knowledge(ctx context.Context, source domain.Source, limit int) ([]domain.ReasonedItem, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_json FROM knowledge_events WHERE source=? ORDER BY last_seen_at DESC LIMIT ?`, source, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []domain.ReasonedItem
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item domain.ReasonedItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PreviouslyDeliveredEvidence(ctx context.Context, source domain.Source, evidenceKeys []string) (map[string]bool, error) {
	result := map[string]bool{}
	if len(evidenceKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(evidenceKeys))
	args := make([]any, 0, len(evidenceKeys)+1)
	args = append(args, source)
	for index, key := range evidenceKeys {
		placeholders[index] = "?"
		args = append(args, key)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT evidence_key FROM timeline_items WHERE source=? AND evidence_key IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	return result, rows.Err()
}

func (s *Store) PreviouslyKnownEvents(ctx context.Context, source domain.Source, eventKeys []string) (map[string]bool, error) {
	result := map[string]bool{}
	if len(eventKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(eventKeys))
	args := make([]any, 0, len(eventKeys)+1)
	args = append(args, source)
	for index, key := range eventKeys {
		placeholders[index] = "?"
		args = append(args, key)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_key FROM knowledge_events WHERE source=? AND event_key IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = true
	}
	return result, rows.Err()
}
