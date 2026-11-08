package store

import (
	"context"
	"database/sql"
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

type knowledgeWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Keep reasoning context keyed by source evidence. Semantic event identity is
// stored separately, and model event labels must not overwrite other posts.
func saveKnowledgeEvidence(ctx context.Context, writer knowledgeWriter, source domain.Source, evidenceKey, itemRaw, createdAt string) error {
	// The slash keeps new storage keys outside the legacy model-label format.
	key := "evidence/" + evidenceKey
	_, err := writer.ExecContext(ctx, `
		INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(source,event_key) DO UPDATE SET evidence_key=excluded.evidence_key,item_json=excluded.item_json,last_seen_at=excluded.last_seen_at
		WHERE excluded.last_seen_at >= knowledge_events.last_seen_at`, domain.NewID("knowledge"), source, key, evidenceKey, itemRaw, createdAt, createdAt)
	return err
}
