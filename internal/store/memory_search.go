package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	memoryLibraryDefaultLimit = 24
	memoryLibraryMaxLimit     = 50
	memoryLibraryMaxQuery     = 200
	memoryLibraryMaxCursor    = 512
	memoryLibraryCursorV      = 1
)

// ErrMemoryLibraryQuery marks a caller-supplied Library query/cursor that
// violates the read contract. HTTP callers can map it to a 400 without
// misclassifying an underlying SQLite failure.
var ErrMemoryLibraryQuery = errors.New("invalid memory library query")

// FTS5 ranks lower BM25 scores first. Keep the field order in sync with the
// memory_search_fts declaration and document the weights in the Browser
// contract: title is strongest, followed by summary, tags/facets, author, and
// the optional full copy.
const (
	memorySearchTitleWeight       = 10.0
	memorySearchSummaryWeight     = 5.0
	memorySearchAuthorWeight      = 2.0
	memorySearchTagsWeight        = 3.0
	memorySearchFacetsWeight      = 3.0
	memorySearchFullContentWeight = 1.0
)

type memoryLibraryCursor struct {
	Version   int     `json:"v"`
	QueryKey  string  `json:"k"`
	Score     float64 `json:"s,omitempty"`
	HasScore  bool    `json:"h,omitempty"`
	UpdatedAt string  `json:"u"`
	ID        string  `json:"i"`
}

type memoryLibraryRow struct {
	ID        string
	UpdatedAt string
	Score     float64
	HasScore  bool
}

type memoryLibraryQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// backfillMemorySearchIndexTx builds the v13 FTS index from active v12
// memory rows. It is deliberately invoked inside the migration transaction so
// a malformed row or index failure leaves schema_version at v12.
func backfillMemorySearchIndexTx(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_search_fts`); err != nil {
		return fmt.Errorf("clear memory search index: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM memory_items WHERE lifecycle_state='active' ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list memories for search backfill: %w", err)
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := syncMemorySearchIndexTx(ctx, tx, id); err != nil {
			return fmt.Errorf("backfill memory %s search index: %w", id, err)
		}
	}
	return nil
}

// syncMemorySearchIndexTx replaces one logical item in the FTS index. A
// delete-first update makes the operation idempotent even though FTS5 does
// not provide a normal UNIQUE constraint for the unindexed item id.
func syncMemorySearchIndexTx(ctx context.Context, tx *sql.Tx, memoryID string) error {
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return errors.New("memory search index requires an item id")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_search_fts WHERE memory_item_id=?`, memoryID); err != nil {
		return fmt.Errorf("remove memory search index row: %w", err)
	}
	var lifecycle, title, summary, author, tagsJSON, facetsJSON, tier, versionID string
	if err := tx.QueryRowContext(ctx, `
		SELECT lifecycle_state,title,summary,author,tags_json,facets_json,retention_tier,full_content_version_id
		FROM memory_items WHERE id=?`, memoryID).Scan(
		&lifecycle, &title, &summary, &author, &tagsJSON, &facetsJSON, &tier, &versionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("read memory for search index: %w", err)
	}
	if lifecycle != string(domain.MemoryStateActive) {
		return nil
	}
	tags, err := decodeMemorySearchLabels(tagsJSON)
	if err != nil {
		return fmt.Errorf("decode memory tags for search index: %w", err)
	}
	facets, err := decodeMemorySearchLabels(facetsJSON)
	if err != nil {
		return fmt.Errorf("decode memory facets for search index: %w", err)
	}
	fullContent := ""
	if tier == string(domain.MemoryTierFullCopy) && versionID != "" {
		if err := tx.QueryRowContext(ctx, `
			SELECT content FROM memory_content_versions
			WHERE id=? AND memory_item_id=? AND released_at IS NULL`, versionID, memoryID).Scan(&fullContent); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read memory full content for search index: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memory_search_fts(memory_item_id,title,summary,author,tags,facets,full_content)
		VALUES(?,?,?,?,?,?,?)`, memoryID, title, summary, author, strings.Join(tags, " "), strings.Join(facets, " "), fullContent); err != nil {
		return fmt.Errorf("insert memory search index row: %w", err)
	}
	return nil
}

func decodeMemorySearchLabels(raw string) ([]string, error) {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

// normalizeMemoryLibraryQuery validates public bounds and turns the user
// query into quoted AND terms. Quoting prevents FTS operators or column names
// from becoming an injection surface while retaining deterministic lexical
// matching.
func normalizeMemoryLibraryQuery(input domain.MemoryLibraryQuery) (domain.MemoryLibraryQuery, string, string, error) {
	query := input
	query.Query = strings.TrimSpace(query.Query)
	if len([]rune(query.Query)) > memoryLibraryMaxQuery {
		return domain.MemoryLibraryQuery{}, "", "", fmt.Errorf("%w: query cannot exceed %d characters", ErrMemoryLibraryQuery, memoryLibraryMaxQuery)
	}
	if query.Source != "" && !query.Source.Valid() {
		return domain.MemoryLibraryQuery{}, "", "", fmt.Errorf("%w: unsupported source %q", ErrMemoryLibraryQuery, query.Source)
	}
	if query.Tier != "" && !query.Tier.Valid() {
		return domain.MemoryLibraryQuery{}, "", "", fmt.Errorf("%w: unsupported retention tier %q", ErrMemoryLibraryQuery, query.Tier)
	}
	var err error
	query.PublishedFrom, err = normalizeMemoryLibraryDate("publishedFrom", query.PublishedFrom, false)
	if err != nil {
		return domain.MemoryLibraryQuery{}, "", "", err
	}
	query.PublishedTo, err = normalizeMemoryLibraryDate("publishedTo", query.PublishedTo, true)
	if err != nil {
		return domain.MemoryLibraryQuery{}, "", "", err
	}
	if query.PublishedFrom != "" && query.PublishedTo != "" {
		from, _ := time.Parse(time.RFC3339Nano, query.PublishedFrom)
		to, _ := time.Parse(time.RFC3339Nano, query.PublishedTo)
		if from.After(to) {
			return domain.MemoryLibraryQuery{}, "", "", fmt.Errorf("%w: publishedFrom must not be after publishedTo", ErrMemoryLibraryQuery)
		}
	}
	if query.Limit == 0 {
		query.Limit = memoryLibraryDefaultLimit
	}
	if query.Limit < 1 || query.Limit > memoryLibraryMaxLimit {
		return domain.MemoryLibraryQuery{}, "", "", fmt.Errorf("%w: limit must be between 1 and %d", ErrMemoryLibraryQuery, memoryLibraryMaxLimit)
	}
	terms, err := memorySearchTerms(query.Query)
	if err != nil {
		return domain.MemoryLibraryQuery{}, "", "", fmt.Errorf("%w: %v", ErrMemoryLibraryQuery, err)
	}
	searchQuery := strings.Join(terms, " AND ")
	key := memoryLibraryQueryKey(query, searchQuery)
	return query, searchQuery, key, nil
}

func normalizeMemoryLibraryDate(label, value string, upperBound bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > 100 {
		return "", fmt.Errorf("%w: %s cannot exceed 100 characters", ErrMemoryLibraryQuery, label)
	}
	var parsed time.Time
	var err error
	if len(value) == len("2006-01-02") {
		parsed, err = time.Parse("2006-01-02", value)
		if err == nil && upperBound {
			parsed = parsed.Add(24*time.Hour - time.Nanosecond)
		}
	} else {
		parsed, err = time.Parse(time.RFC3339Nano, value)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s must be RFC3339 or YYYY-MM-DD", ErrMemoryLibraryQuery, label)
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func memorySearchTerms(query string) ([]string, error) {
	if query == "" {
		return nil, nil
	}
	runes := []rune(strings.ToLower(query))
	terms := make([]string, 0, 8)
	current := make([]rune, 0, 32)
	flush := func() {
		if len(current) == 0 {
			return
		}
		terms = append(terms, `"`+strings.ReplaceAll(string(current), `"`, `""`)+`"`)
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
	if len(terms) == 0 {
		return nil, errors.New("library query must contain letters or numbers")
	}
	return terms, nil
}

func memoryLibraryQueryKey(query domain.MemoryLibraryQuery, searchQuery string) string {
	value := strings.Join([]string{
		searchQuery,
		string(query.Source), string(query.Tier), query.PublishedFrom, query.PublishedTo,
	}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func encodeMemoryLibraryCursor(cursor memoryLibraryCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > memoryLibraryMaxCursor {
		return "", errors.New("library cursor exceeds its bounded length")
	}
	return encoded, nil
}

func decodeMemoryLibraryCursor(raw, queryKey string) (memoryLibraryCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return memoryLibraryCursor{}, nil
	}
	if len(raw) > memoryLibraryMaxCursor {
		return memoryLibraryCursor{}, fmt.Errorf("%w: cursor cannot exceed %d characters", ErrMemoryLibraryQuery, memoryLibraryMaxCursor)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return memoryLibraryCursor{}, fmt.Errorf("%w: cursor is invalid", ErrMemoryLibraryQuery)
	}
	var cursor memoryLibraryCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != memoryLibraryCursorV || cursor.QueryKey != queryKey || cursor.ID == "" || cursor.UpdatedAt == "" {
		return memoryLibraryCursor{}, fmt.Errorf("%w: cursor is invalid or does not match this query", ErrMemoryLibraryQuery)
	}
	if cursor.HasScore && (cursor.Score != cursor.Score) {
		return memoryLibraryCursor{}, fmt.Errorf("%w: cursor score is invalid", ErrMemoryLibraryQuery)
	}
	return cursor, nil
}

func libraryBM25Expression() string {
	return fmt.Sprintf("bm25(memory_search_fts,0.0,%.1f,%.1f,%.1f,%.1f,%.1f,%.1f)",
		memorySearchTitleWeight, memorySearchSummaryWeight, memorySearchAuthorWeight,
		memorySearchTagsWeight, memorySearchFacetsWeight, memorySearchFullContentWeight)
}

// ListMemoryLibrary is the canonical local Library read method. It never
// invokes a provider and never returns tombstones; an empty query is a
// bounded recent-items listing.
func (s *Store) ListMemoryLibrary(ctx context.Context, input domain.MemoryLibraryQuery) (domain.MemoryLibraryResult, error) {
	query, searchQuery, queryKey, err := normalizeMemoryLibraryQuery(input)
	if err != nil {
		return domain.MemoryLibraryResult{}, err
	}
	cursor, err := decodeMemoryLibraryCursor(query.Cursor, queryKey)
	if err != nil {
		return domain.MemoryLibraryResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.MemoryLibraryResult{}, fmt.Errorf("begin library read snapshot: %w", err)
	}
	defer tx.Rollback()
	rows, err := s.queryMemoryLibraryRows(ctx, tx, query, searchQuery, cursor)
	if err != nil {
		return domain.MemoryLibraryResult{}, err
	}
	if len(rows) == 0 {
		if err := tx.Commit(); err != nil {
			return domain.MemoryLibraryResult{}, fmt.Errorf("commit empty library read snapshot: %w", err)
		}
		return domain.MemoryLibraryResult{Items: []domain.MemoryItem{}}, nil
	}
	hasMore := len(rows) > query.Limit
	if hasMore {
		rows = rows[:query.Limit]
	}
	items := make([]domain.MemoryItem, 0, len(rows))
	returnedRows := make([]memoryLibraryRow, 0, len(rows))
	for _, row := range rows {
		item, err := memoryItemByQueryer(ctx, tx, row.ID)
		if err != nil {
			return domain.MemoryLibraryResult{}, err
		}
		if item.LifecycleState != domain.MemoryStateActive {
			continue
		}
		items = append(items, item)
		returnedRows = append(returnedRows, row)
	}
	result := domain.MemoryLibraryResult{Items: items}
	if hasMore && len(returnedRows) > 0 {
		last := returnedRows[len(returnedRows)-1]
		result.NextCursor, err = encodeMemoryLibraryCursor(memoryLibraryCursor{
			Version: memoryLibraryCursorV, QueryKey: queryKey, Score: last.Score,
			HasScore: last.HasScore, UpdatedAt: last.UpdatedAt, ID: last.ID,
		})
		if err != nil {
			return domain.MemoryLibraryResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryLibraryResult{}, fmt.Errorf("commit library read snapshot: %w", err)
	}
	return result, nil
}

// SearchMemoryLibrary is a descriptive alias for ListMemoryLibrary.
func (s *Store) SearchMemoryLibrary(ctx context.Context, input domain.MemoryLibraryQuery) (domain.MemoryLibraryResult, error) {
	return s.ListMemoryLibrary(ctx, input)
}

// SearchMemory is retained as a concise store call for local callers.
func (s *Store) SearchMemory(ctx context.Context, input domain.MemoryLibraryQuery) (domain.MemoryLibraryResult, error) {
	return s.ListMemoryLibrary(ctx, input)
}

// ListLibraryItems is a descriptive alias for the canonical Library listing.
func (s *Store) ListLibraryItems(ctx context.Context, input domain.MemoryLibraryQuery) (domain.MemoryLibraryResult, error) {
	return s.ListMemoryLibrary(ctx, input)
}

func (s *Store) queryMemoryLibraryRows(ctx context.Context, q memoryLibraryQueryer, query domain.MemoryLibraryQuery, searchQuery string, cursor memoryLibraryCursor) ([]memoryLibraryRow, error) {
	where := []string{"mi.lifecycle_state='active'"}
	args := make([]any, 0, 8)
	if query.Source != "" {
		where = append(where, "mi.source=?")
		args = append(args, query.Source)
	}
	if query.Tier != "" {
		where = append(where, "mi.retention_tier=?")
		args = append(args, query.Tier)
	}
	if query.PublishedFrom != "" {
		where = append(where, "julianday(mi.published_at)>=julianday(?)")
		args = append(args, query.PublishedFrom)
	}
	if query.PublishedTo != "" {
		where = append(where, "julianday(mi.published_at)<=julianday(?)")
		args = append(args, query.PublishedTo)
	}
	limitArg := query.Limit + 1
	if searchQuery == "" {
		if cursor.ID != "" {
			where = append(where, "(mi.updated_at<? OR (mi.updated_at=? AND mi.id<?))")
			args = append(args, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
		}
		args = append(args, limitArg)
		rows, err := q.QueryContext(ctx, `SELECT mi.id,mi.updated_at FROM memory_items mi WHERE `+strings.Join(where, " AND ")+` ORDER BY mi.updated_at DESC,mi.id DESC LIMIT ?`, args...)
		if err != nil {
			return nil, fmt.Errorf("list library items: %w", err)
		}
		return scanMemoryLibraryRows(rows, false)
	}

	where = append(where, "memory_search_fts MATCH ?")
	args = append(args, searchQuery)
	rank := libraryBM25Expression()
	if cursor.HasScore {
		where = append(where, fmt.Sprintf("(%s>? OR (%s=? AND (mi.updated_at<? OR (mi.updated_at=? AND mi.id<?))))", rank, rank))
		args = append(args, cursor.Score, cursor.Score, cursor.UpdatedAt, cursor.UpdatedAt, cursor.ID)
	}
	args = append(args, limitArg)
	querySQL := `SELECT mi.id,mi.updated_at,` + rank + ` AS score FROM memory_search_fts JOIN memory_items mi ON mi.id=memory_search_fts.memory_item_id WHERE ` + strings.Join(where, " AND ") + ` ORDER BY score ASC,mi.updated_at DESC,mi.id DESC LIMIT ?`
	rows, err := q.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("search library items: %w", err)
	}
	return scanMemoryLibraryRows(rows, true)
}

func scanMemoryLibraryRows(rows *sql.Rows, hasScore bool) ([]memoryLibraryRow, error) {
	defer rows.Close()
	result := make([]memoryLibraryRow, 0)
	for rows.Next() {
		var row memoryLibraryRow
		if hasScore {
			if err := rows.Scan(&row.ID, &row.UpdatedAt, &row.Score); err != nil {
				return nil, err
			}
			row.HasScore = true
		} else if err := rows.Scan(&row.ID, &row.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// MemoryLibraryItem returns only active items for the read-only Library
// surface. Tombstones deliberately look like a missing item.
func (s *Store) MemoryLibraryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	item, err := s.MemoryItem(ctx, strings.TrimSpace(id))
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if item.LifecycleState != domain.MemoryStateActive {
		return domain.MemoryItem{}, sql.ErrNoRows
	}
	return item, nil
}

func (s *Store) GetMemoryLibraryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	return s.MemoryLibraryItem(ctx, id)
}
