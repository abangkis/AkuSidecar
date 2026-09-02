package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func libraryInput(id string, source domain.Source, title, summary, publishedAt string) domain.MemoryItemInput {
	return domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{
			Source:               source,
			CanonicalEvidenceKey: string(source) + ":library:" + id,
			CanonicalPlatformID:  id,
			ContentFingerprint:   "fingerprint-" + id,
		},
		Title: title, Summary: summary, Author: "Author " + id,
		PublishedAt: &publishedAt, Tags: []string{"library", "local"},
		Facets: []string{"memory"}, Reason: "library test",
	}
}

func TestMemoryLibrarySearchRanksFiltersAndPaginatesDeterministically(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	first, err := state.CreateMemoryRecallStub(ctx, libraryInput("2001", domain.SourceX, "Concurrency handbook", "A practical engineering note", "2026-08-01T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := state.CreateMemoryRecallStub(ctx, libraryInput("2002", domain.SourceX, "Engineering note", "Concurrency patterns for local systems", "2026-08-02T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	third, err := state.CreateMemoryRecallStub(ctx, libraryInput("2003", domain.SourceX, "Other note", "A separate topic", "2026-08-03T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, third.ID, domain.MemoryFullCopyInput{Content: "A private concurrency payload", CapturedAt: "2026-08-03T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	page, err := state.SearchMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "concurrency", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != first.ID || page.NextCursor == "" {
		t.Fatalf("title-weighted first page=%+v", page)
	}
	page2, err := state.SearchMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "concurrency", Limit: 1, Cursor: page.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 1 || page2.Items[0].ID == page.Items[0].ID {
		t.Fatalf("cursor page=%+v first=%+v", page2, page.Items[0])
	}

	filtered, err := state.ListMemoryLibrary(ctx, domain.MemoryLibraryQuery{
		Source: domain.SourceX, Tier: domain.MemoryTierFullCopy,
		PublishedFrom: "2026-08-03T00:00:00Z", PublishedTo: "2026-08-03T23:59:59Z", Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].ID != third.ID {
		t.Fatalf("filtered library=%+v", filtered)
	}
	if _, err := state.DeleteMemory(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	recent, err := state.ListMemoryLibrary(ctx, domain.MemoryLibraryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range recent.Items {
		if item.ID == second.ID || item.LifecycleState == domain.MemoryStateTombstone {
			t.Fatalf("recent listing exposed deleted item: %+v", recent)
		}
	}
}

func TestMemoryLibraryPaginationStableCursorForEqualTimestamps(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	for _, id := range []string{"2251", "2252", "2253"} {
		if _, err := state.CreateMemoryRecallStub(ctx, libraryInput(id, domain.SourceX, "Stable cursor "+id, "Equal timestamp pagination", "2026-08-10T00:00:00Z")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE memory_items SET updated_at=?`, "2026-08-10T12:00:00Z"); err != nil {
		t.Fatal(err)
	}

	all, err := state.ListMemoryLibrary(ctx, domain.MemoryLibraryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 3 {
		t.Fatalf("all items=%+v", all.Items)
	}

	var paged []string
	cursor := ""
	for pageNumber := 0; pageNumber < 4; pageNumber++ {
		page, err := state.ListMemoryLibrary(ctx, domain.MemoryLibraryQuery{Limit: 1, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 1 {
			t.Fatalf("page %d items=%+v cursor=%q", pageNumber, page.Items, page.NextCursor)
		}
		for _, id := range paged {
			if id == page.Items[0].ID {
				t.Fatalf("page %d repeated item %q", pageNumber, id)
			}
		}
		paged = append(paged, page.Items[0].ID)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
		if pageNumber == 3 {
			t.Fatal("stable cursor did not exhaust")
		}
	}
	if len(paged) != len(all.Items) {
		t.Fatalf("paged item count=%d all=%d paged=%v all=%v", len(paged), len(all.Items), paged, all.Items)
	}
	for index, item := range all.Items {
		if paged[index] != item.ID {
			t.Fatalf("paged order=%v all order=%v", paged, all.Items)
		}
	}
}

func TestMemorySearchIndexLifecycleAndRestartPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library.db")
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("2101", domain.SourceX, "Lifecycle marker", "Index lifecycle test", "2026-08-04T00:00:00Z"))
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, item.ID, domain.MemoryFullCopyInput{Content: "unique full payload token", CapturedAt: "2026-08-04T01:00:00Z"}); err != nil {
		state.Close()
		t.Fatal(err)
	}
	result, err := state.SearchMemory(ctx, domain.MemoryLibraryQuery{Query: "unique full payload token", Limit: 10})
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != item.ID {
		state.Close()
		t.Fatalf("full content search=%+v err=%v", result, err)
	}
	if _, err := state.ReleaseMemoryFullCopy(ctx, item.ID); err != nil {
		state.Close()
		t.Fatal(err)
	}
	result, err = state.SearchMemory(ctx, domain.MemoryLibraryQuery{Query: "unique full payload token", Limit: 10})
	if err != nil || len(result.Items) != 0 {
		state.Close()
		t.Fatalf("released full content remained indexed=%+v err=%v", result, err)
	}
	if _, err := state.DeleteMemory(ctx, item.ID); err != nil {
		state.Close()
		t.Fatal(err)
	}
	result, err = state.ListMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "lifecycle", Limit: 10})
	if err != nil || len(result.Items) != 0 {
		state.Close()
		t.Fatalf("deleted item remained indexed=%+v err=%v", result, err)
	}
	persisted, err := state.CreateMemoryRecallStub(ctx, libraryInput("2102", domain.SourceX, "Restart marker", "Index survives restart", "2026-08-04T02:00:00Z"))
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	result, err = state.SearchMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "restart marker", Limit: 10})
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != persisted.ID {
		t.Fatalf("active index did not survive restart=%+v err=%v", result, err)
	}
}

func TestSchema12MigrationBackfillsMemorySearchIndex(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schema12.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','12');`+memorySchemaSQL); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO memory_items(id,source,identity_digest,canonical_evidence_key,canonical_permalink,canonical_platform_id,content_fingerprint,title,summary,author,published_at,tags_json,facets_json,media_metadata_json,retention_tier,lifecycle_state,full_content_version_id,content_bytes,reason,created_at,updated_at) VALUES('memory_migration','x','digest','x:migration','https://x.com/aku/status/3001','3001','fp-migration','Migration marker','Backfilled searchable summary','Author','2026-08-05T00:00:00Z','["backfill"]','["migration"]','[]','recall','active','',0,'test','2026-08-05T00:00:00Z','2026-08-05T00:00:00Z')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := Open(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	result, err := state.SearchMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "backfilled searchable", Limit: 10})
	if err != nil || len(result.Items) != 1 || result.Items[0].ID != "memory_migration" {
		t.Fatalf("migration search=%+v err=%v", result, err)
	}
	var version string
	if err := state.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
		t.Fatalf("migration version=%q err=%v", version, err)
	}
}

func TestSchema12SearchMigrationIsAtomicOnConflict(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schema12-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','12');`+memorySchemaSQL+`CREATE TABLE memory_search_fts(existing TEXT);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := migrateSchema12To13(ctx, db); err == nil {
		db.Close()
		t.Fatal("conflicting search table must fail migration")
	}
	var version string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "12" {
		db.Close()
		t.Fatalf("failed migration changed version=%q err=%v", version, err)
	}
	var searchSQL string
	if err := db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name='memory_search_fts'`).Scan(&searchSQL); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(searchSQL), "using fts5") {
		db.Close()
		t.Fatalf("failed migration left virtual search table: %q", searchSQL)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryLibraryBoundsAndTombstoneDetail(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("2201", domain.SourceX, "Bounded item", "Bounded summary", "2026-08-06T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.DeleteMemory(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.MemoryLibraryItem(ctx, item.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("tombstone detail err=%v", err)
	}
	for _, query := range []domain.MemoryLibraryQuery{
		{Query: strings.Repeat("x", memoryLibraryMaxQuery+1)},
		{Query: "x", Limit: memoryLibraryMaxLimit + 1},
		{Query: "x", Cursor: strings.Repeat("x", memoryLibraryMaxCursor+1)},
		{PublishedFrom: "2026-08-07", PublishedTo: "2026-08-06"},
		{PublishedFrom: "not-a-date"},
		{PublishedTo: "2026-08-06T00:00:00"},
		{PublishedFrom: "2026-08-06T00:00:00.1Z", PublishedTo: "2026-08-06T00:00:00Z"},
	} {
		if _, err := state.ListMemoryLibrary(ctx, query); err == nil {
			t.Fatalf("unbounded/invalid library query unexpectedly succeeded: %+v", query)
		}
	}
}
