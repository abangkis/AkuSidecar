package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestPersonalMemorySchemaContract(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	wantColumns := map[string][]string{
		"memory_items": {
			"id", "source", "identity_digest", "canonical_evidence_key", "canonical_permalink",
			"canonical_platform_id", "content_fingerprint", "title", "summary", "author", "published_at",
			"tags_json", "facets_json", "media_metadata_json", "retention_tier", "lifecycle_state",
			"full_content_version_id", "content_bytes", "reason", "created_at", "updated_at",
		},
		"memory_identity_aliases":  {"source", "alias_kind", "alias_value", "memory_item_id", "created_at", "last_seen_at"},
		"memory_tombstone_aliases": {"memory_item_id", "alias_kind", "alias_digest", "created_at"},
		"memory_content_versions":  {"id", "memory_item_id", "version", "content", "content_fingerprint", "media_metadata_json", "content_bytes", "captured_at", "created_at", "released_at"},
		"memory_provenance":        {"id", "memory_item_id", "provenance_kind", "source", "canonical_evidence_key", "source_url", "capture_context_json", "reason", "created_at"},
		"memory_actions":           {"id", "memory_item_id", "action", "detail_json", "created_at"},
		"memory_search_fts":        {"memory_item_id", "title", "summary", "author", "tags", "facets", "full_content"},
	}
	for table, want := range wantColumns {
		rows, err := state.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var ordinal, notNull, primaryKey int
			var name, kind string
			var defaultValue any
			if err := rows.Scan(&ordinal, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			got = append(got, name)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s columns=%v want=%v", table, got, want)
		}
	}
	for _, table := range []string{"memory_items", "memory_identity_aliases", "memory_tombstone_aliases", "memory_content_versions", "memory_provenance", "memory_actions"} {
		rows, err := state.db.QueryContext(ctx, `PRAGMA foreign_key_list(`+table+`)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id, ordinal int
			var tableName, from, to, onUpdate, onDelete, match string
			if err := rows.Scan(&id, &ordinal, &tableName, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if tableName == "sessions" || tableName == "runs" || tableName == "timeline_items" {
				t.Fatalf("%s unexpectedly references operational table %s", table, tableName)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	var version string
	if err := state.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "13" || SchemaVersion != 13 {
		t.Fatalf("schema version=%q constant=%d", version, SchemaVersion)
	}
	indexes := map[string]bool{}
	rows, err := state.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='index' AND name LIKE 'memory_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		indexes[name] = true
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"memory_items_identity_digest", "memory_items_lifecycle_updated", "memory_items_source_updated",
		"memory_identity_aliases_lookup", "memory_identity_aliases_strong_unique", "memory_identity_aliases_item",
		"memory_tombstone_aliases_lookup", "memory_tombstone_aliases_item",
		"memory_content_versions_item_created", "memory_content_versions_active", "memory_provenance_item_created",
		"memory_actions_item_created", "memory_actions_action_created",
	} {
		if !indexes[name] {
			t.Errorf("missing memory index %s", name)
		}
	}
	var searchTable int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_search_fts'`).Scan(&searchTable); err != nil {
		t.Fatal(err)
	}
	if searchTable != 1 {
		t.Fatalf("memory search table count=%d", searchTable)
	}
}

func TestSchema11MigratesPersonalMemoryFoundation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema11.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','11')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var version string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "13" {
		t.Fatalf("migrated schema version=%q err=%v", version, err)
	}
	var count int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('memory_items','memory_identity_aliases','memory_tombstone_aliases','memory_content_versions','memory_provenance','memory_actions')`).Scan(&count); err != nil || count != 6 {
		t.Fatalf("memory tables=%d err=%v", count, err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_search_fts'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("memory search table=%d err=%v", count, err)
	}
	var integrity string
	if err := state.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q err=%v", integrity, err)
	}
}

func TestSchema11MemoryMigrationIsAtomicOnSchemaConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema11-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','11');
		CREATE TABLE memory_items(bad INTEGER);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := migrateSchema11To12(context.Background(), db); err == nil {
		db.Close()
		t.Fatal("conflicting memory schema must fail migration")
	}
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if version != "11" {
		db.Close()
		t.Fatalf("failed migration changed schema version=%q", version)
	}
	var created int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('memory_identity_aliases','memory_tombstone_aliases','memory_content_versions','memory_provenance','memory_actions')`).Scan(&created); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if created != 0 {
		db.Close()
		t.Fatalf("failed migration left partial memory tables=%d", created)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func memoryStubInput() domain.MemoryItemInput {
	published := "2026-08-29T10:00:00Z"
	return domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{
			Source: domain.SourceX, CanonicalEvidenceKey: "x:status:1001",
			CanonicalPermalink: "https://x.com/aku/status/1001", ContentFingerprint: "fp-1001",
		},
		Title: "A useful memory", Summary: "A bounded summary", Author: "aku",
		PublishedAt: &published, Tags: []string{"go", "memory"}, Facets: []string{"engineering"},
		Media:  []domain.MemoryMediaReference{{Kind: "image", URL: "https://pbs.twimg.com/media/1001.jpg", AltText: "preview"}},
		Reason: "explicit More signal",
	}
}

func TestMemoryIdentityDeduplicatesAliasesAndKeepsFingerprintAsFallback(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	first, err := state.UpsertMemoryRecallStub(ctx, memoryStubInput())
	if err != nil {
		t.Fatal(err)
	}
	second, err := state.UpsertMemoryRecallStub(ctx, domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{Source: domain.SourceX, CanonicalPermalink: "https://x.com/aku/status/1001/?utm_source=feed"},
		Summary:  "updated summary",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Summary != "updated summary" {
		t.Fatalf("alias did not update same item: first=%+v second=%+v", first, second)
	}
	third, err := state.UpsertMemoryRecallStub(ctx, domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{Source: domain.SourceX, ContentFingerprint: "fp-1001"},
		Title:    "fingerprint update",
	})
	if err != nil {
		t.Fatal(err)
	}
	if third.ID != first.ID {
		t.Fatalf("fingerprint fallback did not resolve existing item: first=%s third=%s", first.ID, third.ID)
	}
	var aliases int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_identity_aliases WHERE memory_item_id=?`, first.ID).Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if aliases != 3 {
		t.Fatalf("aliases=%d want 3", aliases)
	}
}

func TestMemoryKeepReleaseFullCopyAndLogicalStorage(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	item, err := state.CreateMemoryRecallStub(ctx, memoryStubInput())
	if err != nil {
		t.Fatal(err)
	}
	if item.RetentionTier != domain.MemoryTierRecall || item.FullContent != nil {
		t.Fatalf("new memory=%+v", item)
	}
	if _, err := state.RecordMemoryProvenance(ctx, domain.MemoryProvenance{
		MemoryItemID: item.ID, ProvenanceKind: "captured", Source: domain.SourceX,
		CanonicalEvidenceKey: item.CanonicalEvidenceKey, SourceURL: item.CanonicalPermalink,
		CaptureContext: map[string]any{"surface": "timeline"},
	}); err != nil {
		t.Fatal(err)
	}
	kept, err := state.KeepMemoryFullCopy(ctx, item.ID, domain.MemoryFullCopyInput{
		Content: "the complete text retained locally", Media: item.Media,
		CapturedAt: "2026-08-29T10:01:00Z", Reason: "read later",
	})
	if err != nil {
		t.Fatal(err)
	}
	if kept.RetentionTier != domain.MemoryTierFullCopy || kept.FullContent == nil || *kept.FullContent != "the complete text retained locally" || kept.ContentBytes == 0 {
		t.Fatalf("kept memory=%+v", kept)
	}
	usage, err := state.MemoryStorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.FullCopyItems != 1 || usage.ContentBytes != int64(len("the complete text retained locally")) || usage.LogicalBytes <= usage.ContentBytes {
		t.Fatalf("usage after keep=%+v", usage)
	}
	released, err := state.ReleaseMemoryFullCopy(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if released.RetentionTier != domain.MemoryTierRecall || released.FullContent != nil || released.ContentBytes != 0 {
		t.Fatalf("released memory=%+v", released)
	}
	var storedContent, releasedAt string
	if err := state.db.QueryRow(`SELECT content,released_at FROM memory_content_versions WHERE memory_item_id=?`, item.ID).Scan(&storedContent, &releasedAt); err != nil {
		t.Fatal(err)
	}
	if storedContent != "" || releasedAt == "" {
		t.Fatalf("released version content=%q releasedAt=%q", storedContent, releasedAt)
	}
	usage, err = state.MemoryStorage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.ContentBytes != 0 || usage.FullCopyItems != 0 || usage.RecallItems != 1 {
		t.Fatalf("usage after release=%+v", usage)
	}
}

func TestMemoryDeleteClearsPrivacyAndLeavesOpaqueTombstone(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	item, err := state.CreateMemoryRecallStub(ctx, memoryStubInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordMemoryProvenance(ctx, domain.MemoryProvenance{
		MemoryItemID: item.ID, ProvenanceKind: "captured", Source: domain.SourceX,
		SourceURL: item.CanonicalPermalink, Reason: "private provenance",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, item.ID, domain.MemoryFullCopyInput{Content: "private complete text", Reason: "keep https://x.com/aku/status/1001"}); err != nil {
		t.Fatal(err)
	}
	deleted, err := state.DeleteMemory(ctx, item.ID, "remove https://x.com/aku/status/1001 and private text")
	if err != nil {
		t.Fatal(err)
	}
	if deleted.LifecycleState != domain.MemoryStateTombstone || deleted.Source != "" || deleted.CanonicalPermalink != "" || deleted.Title != "" || deleted.FullContent != nil {
		t.Fatalf("deleted memory retained identifying data=%+v", deleted)
	}
	var source, permalink, tags, detail string
	var provenanceCount, aliasCount, versionCount, tombstoneAliasCount int
	if err := state.db.QueryRow(`SELECT source,canonical_permalink,tags_json FROM memory_items WHERE id=?`, item.ID).Scan(&source, &permalink, &tags); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, item.ID).Scan(&provenanceCount); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_identity_aliases WHERE memory_item_id=?`, item.ID).Scan(&aliasCount); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_tombstone_aliases WHERE memory_item_id=?`, item.ID).Scan(&tombstoneAliasCount); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_content_versions WHERE memory_item_id=?`, item.ID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT detail_json FROM memory_actions WHERE memory_item_id=? AND action='delete'`, item.ID).Scan(&detail); err != nil {
		t.Fatal(err)
	}
	var actionDetails string
	if err := state.db.QueryRow(`SELECT GROUP_CONCAT(detail_json,' ') FROM memory_actions WHERE memory_item_id=?`, item.ID).Scan(&actionDetails); err != nil {
		t.Fatal(err)
	}
	if source != "" || permalink != "" || tags != "[]" || provenanceCount != 0 || aliasCount != 0 || tombstoneAliasCount != 3 || versionCount != 0 || detail != "{}" {
		t.Fatalf("delete privacy fields source=%q permalink=%q tags=%q provenance=%d aliases=%d tombstoneAliases=%d versions=%d detail=%q", source, permalink, tags, provenanceCount, aliasCount, tombstoneAliasCount, versionCount, detail)
	}
	for _, input := range []domain.MemoryItemInput{
		{Identity: domain.MemoryIdentity{Source: domain.SourceX, CanonicalEvidenceKey: "x:status:1001"}},
		{Identity: domain.MemoryIdentity{Source: domain.SourceX, CanonicalPermalink: "https://x.com/aku/status/1001"}},
		{Identity: domain.MemoryIdentity{Source: domain.SourceX, ContentFingerprint: "fp-1001"}},
	} {
		if _, err := state.UpsertMemoryRecallStub(ctx, input); !errors.Is(err, ErrMemoryTombstoned) {
			t.Fatalf("recapture input=%+v err=%v want tombstone", input.Identity, err)
		}
	}
	if strings.Contains(actionDetails, "https://") || strings.Contains(actionDetails, "private") {
		t.Fatalf("action details retained private data=%q", actionDetails)
	}
	var rawAliasCount int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_tombstone_aliases WHERE alias_digest LIKE '%x.com%' OR alias_digest LIKE '%1001%'`).Scan(&rawAliasCount); err != nil {
		t.Fatal(err)
	}
	if rawAliasCount != 0 {
		t.Fatalf("tombstone aliases contain raw identity material: %d", rawAliasCount)
	}
}

func TestMemoryRemovePhysicallyClearsAndAllowsRecapture(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	input := memoryStubInput()
	item, err := state.CreateMemoryRecallStub(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.RecordMemoryProvenance(ctx, domain.MemoryProvenance{
		MemoryItemID: item.ID, ProvenanceKind: "captured", Source: domain.SourceX,
		SourceURL: item.CanonicalPermalink, Reason: "local removal fixture",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, item.ID, domain.MemoryFullCopyInput{Content: "local content to remove"}); err != nil {
		t.Fatal(err)
	}
	if err := state.RemoveMemory(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.MemoryItem(ctx, item.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("removed memory still readable: %v", err)
	}
	var active, search, actions, provenance, aliases, versions, tombstones int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_items WHERE id=?`, item.ID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_search_fts WHERE memory_item_id=?`, item.ID).Scan(&search); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=?`, item.ID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, item.ID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_identity_aliases WHERE memory_item_id=?`, item.ID).Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_content_versions WHERE memory_item_id=?`, item.ID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_tombstone_aliases WHERE memory_item_id=?`, item.ID).Scan(&tombstones); err != nil {
		t.Fatal(err)
	}
	if active != 0 || search != 0 || actions != 0 || provenance != 0 || aliases != 0 || versions != 0 || tombstones != 0 {
		t.Fatalf("physical removal left rows item=%d search=%d actions=%d provenance=%d aliases=%d versions=%d tombstones=%d", active, search, actions, provenance, aliases, versions, tombstones)
	}
	recreated, err := state.CreateMemoryRecallStub(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if recreated.ID == item.ID {
		t.Fatalf("physical removal reused deleted item id=%q", recreated.ID)
	}
}

func TestMemorySurvivesOperationalDeletionAndFullResetRemovesIt(t *testing.T) {
	ctx := context.Background()
	clock := &mutableStoreClock{now: time.Date(2026, time.August, 29, 12, 0, 0, 0, time.UTC)}
	state := openTestStoreWithClock(t, clock)
	item, err := state.CreateMemoryRecallStub(ctx, memoryStubInput())
	if err != nil {
		t.Fatal(err)
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := createVisibleUpdateSession(state, ctx, "operational fixture", settings); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.MemoryItem(ctx, item.ID); err != nil {
		t.Fatalf("memory removed with operational sessions: %v", err)
	}
	if _, err := state.DeleteMemory(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	reset, err := state.FullReset(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if reset.BackupFile == "" {
		t.Fatal("full reset did not create verified backup")
	}
	if _, err := state.MemoryItem(ctx, item.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("memory survived FullReset: %v", err)
	}
	var count int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("memory item count after reset=%d", count)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_tombstone_aliases`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tombstone alias count after reset=%d", count)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_search_fts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("search index count after reset=%d", count)
	}
	var tombstoneKeyCount int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM meta WHERE key='memory_tombstone_key_v1'`).Scan(&tombstoneKeyCount); err != nil {
		t.Fatal(err)
	}
	if tombstoneKeyCount != 0 {
		t.Fatalf("memory tombstone key survived reset")
	}
	var integrity string
	if err := state.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("post-reset integrity=%q err=%v", integrity, err)
	}
}
