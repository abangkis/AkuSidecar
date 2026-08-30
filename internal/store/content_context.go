package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	contentContextMaxTerms      = 8
	contentContextMaxFieldRunes = 1600
	contentContextMaxTermRunes  = 48
	contentContextDefaultLimit  = domain.ContentContextDefaultLimit
	contentContextMinLimit      = domain.ContentContextMinLimit
	contentContextMaxLimit      = domain.ContentContextMaxLimit
)

var ErrContentContextNotEligible = errors.New("Timeline item is not a final visible item")

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

	terms := contentContextTerms(timeline)
	if err := tx.Commit(); err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("commit content context read snapshot: %w", err)
	}
	if len(terms) == 0 {
		return domain.ContentContextResult{Matches: []domain.ContentContextMatch{}}, nil
	}

	items, err := s.searchMemoryContextTerms(ctx, terms, limit)
	if err != nil {
		return domain.ContentContextResult{}, err
	}
	result := domain.ContentContextResult{Matches: make([]domain.ContentContextMatch, 0, len(items))}
	for _, item := range items {
		if sameTimelineMemoryIdentity(timeline, item) {
			continue
		}
		result.Matches = append(result.Matches, domain.ContentContextMatch{
			Item:        item,
			MatchReason: contentContextMatchReason(item, terms),
		})
	}
	return result, nil
}

// searchMemoryContextTerms uses the existing FTS5 index with a bounded OR
// query. Context terms are deliberately less strict than the Library's user
// search (which uses AND), because a Timeline's title, text, summary, and
// tags do not all need to be present in an older memory item.
func (s *Store) searchMemoryContextTerms(ctx context.Context, terms []string, limit int) ([]domain.MemoryItem, error) {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		normalized, err := memorySearchTerms(term)
		if err != nil || len(normalized) != 1 {
			continue
		}
		quoted = append(quoted, normalized[0])
	}
	if len(quoted) == 0 {
		return []domain.MemoryItem{}, nil
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin content context FTS snapshot: %w", err)
	}
	defer tx.Rollback()
	rank := libraryBM25Expression()
	rows, err := tx.QueryContext(ctx, `
		SELECT mi.id
		FROM memory_search_fts
		JOIN memory_items mi ON mi.id=memory_search_fts.memory_item_id
		WHERE mi.lifecycle_state='active' AND memory_search_fts MATCH ?
		ORDER BY `+rank+` ASC,mi.updated_at DESC,mi.id DESC
		LIMIT ?`, strings.Join(quoted, " OR "), limit)
	if err != nil {
		return nil, fmt.Errorf("search local content context: %w", err)
	}
	defer rows.Close()
	items := make([]domain.MemoryItem, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		item, err := memoryItemByQueryer(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if item.LifecycleState == domain.MemoryStateActive {
			items = append(items, item)
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

// contentContextTerms returns at most eight lexical terms in a stable,
// field-balanced order. WhatChanged is the Timeline title-like field,
// Evidence.Text is source text, WhyItMatters is the summary, and assessment
// tags/facets provide topic labels.
func contentContextTerms(item domain.TimelineItem) []string {
	text := ""
	if item.Evidence != nil {
		text = item.Evidence.Text
	}
	fields := [][]string{
		contentContextTokenize(item.Item.WhatChanged),
		contentContextTokenize(text),
		contentContextTokenize(item.Item.WhyItMatters),
		contentContextTokenize(strings.Join(append(append([]string{}, item.Assessment.TopicTags...), item.Assessment.TopicFacets...), " ")),
	}
	terms := make([]string, 0, contentContextMaxTerms)
	seen := make(map[string]bool, contentContextMaxTerms)
	for round := 0; len(terms) < contentContextMaxTerms; round++ {
		added := false
		for _, field := range fields {
			if round >= len(field) {
				continue
			}
			term := field[round]
			if seen[term] {
				continue
			}
			seen[term] = true
			terms = append(terms, term)
			added = true
			if len(terms) == contentContextMaxTerms {
				break
			}
		}
		if !added {
			break
		}
	}
	return terms
}

func contentContextTokenize(value string) []string {
	runes := []rune(strings.ToLower(strings.TrimSpace(value)))
	if len(runes) > contentContextMaxFieldRunes {
		runes = runes[:contentContextMaxFieldRunes]
	}
	result := make([]string, 0, 12)
	current := make([]rune, 0, 24)
	flush := func() {
		if len(current) == 0 {
			return
		}
		if len(current) <= contentContextMaxTermRunes && !contentContextStopWords[string(current)] {
			result = append(result, string(current))
		}
		current = current[:0]
	}
	for _, value := range runes {
		if unicode.IsLetter(value) || unicode.IsNumber(value) {
			current = append(current, value)
			continue
		}
		flush()
	}
	flush()
	return result
}

var contentContextStopWords = map[string]bool{
	"a": true, "an": true, "and": true, "are": true, "as": true, "at": true,
	"be": true, "by": true, "for": true, "from": true, "in": true, "is": true,
	"it": true, "of": true, "on": true, "or": true, "that": true, "the": true,
	"their": true, "this": true, "to": true, "was": true, "with": true,
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

func contentContextMatchReason(item domain.MemoryItem, terms []string) string {
	fields := []struct {
		label string
		value string
	}{
		{label: "title", value: item.Title},
		{label: "summary", value: item.Summary},
		{label: "author", value: item.Author},
		{label: "tags", value: strings.Join(item.Tags, " ")},
		{label: "facets", value: strings.Join(item.Facets, " ")},
	}
	if item.FullContent != nil {
		fields = append(fields, struct {
			label string
			value string
		}{label: "retained text", value: *item.FullContent})
	}
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		if contentContextHasAnyTerm(field.value, terms) {
			labels = append(labels, field.label)
		}
	}
	if len(labels) == 0 {
		return "Matches a local lexical memory field"
	}
	return "Matches " + strings.Join(labels, ", ")
}

func contentContextHasAnyTerm(value string, terms []string) bool {
	set := make(map[string]bool)
	for _, token := range contentContextTokenize(value) {
		set[token] = true
	}
	for _, term := range terms {
		if set[term] {
			return true
		}
	}
	return false
}
