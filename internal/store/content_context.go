package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/contentcontext"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	contentContextDefaultLimit = domain.ContentContextDefaultLimit
	contentContextMinLimit     = domain.ContentContextMinLimit
	contentContextMaxLimit     = domain.ContentContextMaxLimit
)

var ErrContentContextNotEligible = errors.New("Timeline item is not a final visible item")

var contentContextEngine = contentcontext.NewEngine()

// ContentContext reads one final Timeline item and searches only the local
// Personal Memory FTS index. It intentionally has no write path, provider
// invocation, Bridge call, or Timeline/feedback side effect.
func (s *Store) ContentContext(ctx context.Context, timelineID string, limit int) (domain.ContentContextResult, error) {
	if limit == 0 {
		limit = contentContextDefaultLimit
	}
	if limit < contentContextMinLimit || limit > contentContextMaxLimit {
		return domain.ContentContextResult{}, fmt.Errorf("content context limit must be between %d and %d", contentContextMinLimit, contentContextMaxLimit)
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("begin content context read snapshot: %w", err)
	}
	defer tx.Rollback()

	timeline, err := timelineItemForRetentionTx(ctx, tx, strings.TrimSpace(timelineID), false)
	if err != nil {
		if errors.Is(err, ErrTimelineMemoryNotEligible) {
			return domain.ContentContextResult{}, ErrContentContextNotEligible
		}
		return domain.ContentContextResult{}, err
	}
	var visible int
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM timeline_items t
			JOIN sessions s ON s.id=t.session_id
			LEFT JOIN auto_update_batches b ON b.session_id=s.id
			WHERE t.id=?
			  AND s.status IN ('completed','partial')
			  AND s.completed_at IS NOT NULL
			  AND (b.state IS NULL OR b.state='visible')
		)`, strings.TrimSpace(timelineID)).Scan(&visible); err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("check content context Timeline visibility: %w", err)
	}
	if visible == 0 {
		return domain.ContentContextResult{}, ErrContentContextNotEligible
	}

	query := contentContextEngine.Extract(timeline)
	if err := tx.Commit(); err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("commit content context read snapshot: %w", err)
	}
	if len(query.Terms) == 0 {
		return domain.ContentContextResult{Matches: []domain.ContentContextMatch{}}, nil
	}

	items, err := s.searchMemoryContextCandidates(ctx, query.Terms, contentContextEngine.CandidatePool)
	if err != nil {
		return domain.ContentContextResult{}, err
	}
	candidates := make([]contentcontext.Candidate, 0, len(items))
	for _, candidate := range items {
		if sameTimelineMemoryIdentity(timeline, candidate.Item) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	return domain.ContentContextResult{Matches: contentContextEngine.Match(query, candidates, limit)}, nil
}

// searchMemoryContextCandidates uses the existing FTS5 index with a bounded
// OR query. It intentionally overfetches so the relevance engine can reject
// weak generic-token matches without filling the public result quota from the
// first BM25 rows.
func (s *Store) searchMemoryContextCandidates(ctx context.Context, terms []string, limit int) ([]contentcontext.Candidate, error) {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		normalized, err := memorySearchTerms(term)
		if err != nil || len(normalized) != 1 {
			continue
		}
		quoted = append(quoted, normalized[0])
	}
	if len(quoted) == 0 {
		return []contentcontext.Candidate{}, nil
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin content context FTS snapshot: %w", err)
	}
	defer tx.Rollback()
	rank := libraryBM25Expression()
	rows, err := tx.QueryContext(ctx, `
		SELECT mi.id,`+rank+` AS score
		FROM memory_search_fts
		JOIN memory_items mi ON mi.id=memory_search_fts.memory_item_id
		WHERE mi.lifecycle_state='active' AND memory_search_fts MATCH ?
		ORDER BY `+rank+` ASC,mi.updated_at DESC,mi.id DESC
		LIMIT ?`, strings.Join(quoted, " OR "), limit)
	if err != nil {
		return nil, fmt.Errorf("search local content context: %w", err)
	}
	defer rows.Close()
	items := make([]contentcontext.Candidate, 0, limit)
	for rows.Next() {
		var id string
		var bm25 float64
		if err := rows.Scan(&id, &bm25); err != nil {
			return nil, err
		}
		item, err := memoryItemByQueryer(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if item.LifecycleState == domain.MemoryStateActive {
			items = append(items, contentcontext.Candidate{Item: item, BM25: bm25})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit content context FTS snapshot: %w", err)
	}
	return items, nil
}

func sameTimelineMemoryIdentity(timeline domain.TimelineItem, memory domain.MemoryItem) bool {
	if timeline.Source != memory.Source {
		return false
	}
	if evidenceKey := strings.TrimSpace(timeline.EvidenceKey); evidenceKey != "" && evidenceKey == strings.TrimSpace(memory.CanonicalEvidenceKey) {
		return true
	}
	if sourceURL := strings.TrimSpace(timeline.Item.SourceURL); sourceURL != "" && sourceURL == strings.TrimSpace(memory.CanonicalPermalink) {
		return true
	}
	if timeline.Evidence == nil {
		return false
	}
	if permalink := strings.TrimSpace(timeline.Evidence.Permalink); permalink != "" && permalink == strings.TrimSpace(memory.CanonicalPermalink) {
		return true
	}
	if platformID := strings.TrimSpace(timeline.Evidence.PlatformID); platformID != "" && platformID == strings.TrimSpace(memory.CanonicalPlatformID) {
		return true
	}
	return false
}
