package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSchema13MigrationMaterializesLegacyFullCopyAsKeep(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schema13.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','13');`+memorySchemaSQL+memorySearchSchemaSQL); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO memory_items(
		  id,source,identity_digest,canonical_evidence_key,canonical_permalink,
		  canonical_platform_id,content_fingerprint,title,summary,author,published_at,
		  tags_json,facets_json,media_metadata_json,retention_tier,lifecycle_state,
		  full_content_version_id,content_bytes,reason,created_at,updated_at
		) VALUES('legacy-full','x','digest','x:legacy-full','https://x.com/aku/status/1','1','fp',
		  'Legacy full copy','Migrated text','Author','2026-08-30T00:00:00Z','[]','[]','[]',
		  'full_copy','active','legacy-content',12,'legacy','2026-08-30T00:00:00Z','2026-08-30T00:00:00Z');
		INSERT INTO memory_actions(id,memory_item_id,action,detail_json,created_at)
		VALUES('legacy-read-later','legacy-full','read_later','{}','2026-08-30T00:00:00Z'),
		  ('legacy-mark-read','legacy-full','mark_read','{}','2026-08-30T00:01:00Z');
		INSERT INTO memory_content_versions(
		  id,memory_item_id,version,content,content_fingerprint,media_metadata_json,
		  content_bytes,captured_at,created_at,released_at
		) VALUES('legacy-content','legacy-full',1,'legacy payload','fp-content','[]',12,
		  '2026-08-30T00:00:00Z','2026-08-30T00:00:00Z',NULL);`); err != nil {
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
	item, err := state.MemoryItem(ctx, "legacy-full")
	if err != nil {
		t.Fatal(err)
	}
	if !item.PermanentKeep || item.Saved || item.RetentionTier != domain.MemoryTierFullCopy || item.FullContent == nil || *item.FullContent != "legacy payload" {
		t.Fatalf("legacy full copy state=%+v", item)
	}
	var version string
	if err := state.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "17" {
		t.Fatalf("schema version=%q err=%v", version, err)
	}
	var claims int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_retention_claims WHERE memory_item_id='legacy-full' AND claim_kind='keep' AND resolved_at IS NULL`).Scan(&claims); err != nil || claims != 1 {
		t.Fatalf("legacy keep claims=%d err=%v", claims, err)
	}
}

func TestSchema13MigrationRejectsMalformedRetentionClaimsAtomically(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schema13-malformed.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','13');
		CREATE TABLE memory_retention_claims(bad TEXT);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("malformed v13 retention claims unexpectedly migrated")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "13" {
		t.Fatalf("failed migration changed schema version to %q", version)
	}
}

func TestReadLaterKeepAndDoneUseCurrentClaimsWithoutDuplicatingContent(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:saved-1001")

	first, alreadySaved, err := state.ReadLaterTimeline(ctx, fixture.Item.ID)
	if err != nil || alreadySaved || !first.Saved || first.PermanentKeep || first.RetentionTier != domain.MemoryTierFullCopy || first.FullContent == nil {
		t.Fatalf("first Read later item=%+v alreadySaved=%v err=%v", first, alreadySaved, err)
	}
	if *first.FullContent != fixture.Item.Evidence.Text {
		t.Fatalf("Read later content=%q want=%q", *first.FullContent, fixture.Item.Evidence.Text)
	}
	var versions, readLaterActions int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_content_versions WHERE memory_item_id=?`, first.ID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=? AND action='read_later'`, first.ID).Scan(&readLaterActions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || readLaterActions != 1 {
		t.Fatalf("initial versions=%d Read later actions=%d", versions, readLaterActions)
	}

	repeat, alreadySaved, err := state.ReadLaterTimeline(ctx, fixture.Item.ID)
	if err != nil || !alreadySaved || repeat.ID != first.ID {
		t.Fatalf("repeat Read later item=%+v alreadySaved=%v err=%v", repeat, alreadySaved, err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_content_versions WHERE memory_item_id=?`, first.ID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=? AND action='read_later'`, first.ID).Scan(&readLaterActions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || readLaterActions != 1 {
		t.Fatalf("repeat versions=%d Read later actions=%d", versions, readLaterActions)
	}

	saved, err := state.ListSavedMemory(ctx, domain.MemoryLibraryQuery{Limit: 10})
	if err != nil || len(saved.Items) != 1 || !saved.Items[0].Saved {
		t.Fatalf("Saved listing=%+v err=%v", saved, err)
	}
	kept, err := state.KeepMemoryInLibrary(ctx, first.ID)
	if err != nil || kept.Saved || !kept.PermanentKeep || kept.RetentionTier != domain.MemoryTierFullCopy || kept.FullContent == nil {
		t.Fatalf("Keep in Library item=%+v err=%v", kept, err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_content_versions WHERE memory_item_id=?`, first.ID).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || *kept.FullContent != *first.FullContent {
		t.Fatalf("Keep duplicated or changed content versions=%d item=%+v", versions, kept)
	}
	keptAgain, err := state.KeepMemoryInLibrary(ctx, first.ID)
	if err != nil || keptAgain.ID != first.ID || keptAgain.Saved || !keptAgain.PermanentKeep {
		t.Fatalf("idempotent Keep item=%+v err=%v", keptAgain, err)
	}
	finished, err := state.DoneSavedMemory(ctx, first.ID)
	if err != nil || finished.Saved || !finished.PermanentKeep || finished.FullContent == nil {
		t.Fatalf("Done after Keep item=%+v err=%v", finished, err)
	}
}

func TestDoneReleasesTemporaryReadLaterCopyAndLessUsesCurrentClaims(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:saved-1002")
	readLater, _, err := state.ReadLaterTimeline(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	finished, err := state.DoneSavedMemory(ctx, readLater.ID)
	if err != nil || finished.Saved || finished.PermanentKeep || finished.RetentionTier != domain.MemoryTierRecall || finished.FullContent != nil {
		t.Fatalf("Done temporary state=%+v err=%v", finished, err)
	}
	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	reason := "not_interested"
	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "less", Reason: &reason}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.MemoryItem(ctx, readLater.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("Less retained a Done Read later stub: %v", err)
	}
}

func TestLessConsultsCurrentSavedAndKeepClaims(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	reason := "not_interested"

	savedFixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:saved-1004")
	if _, err := state.AddFeedback(ctx, savedFixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	saved, _, err := state.ReadLaterTimeline(ctx, savedFixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, savedFixture.Item.ID, domain.Feedback{Direction: "less", Reason: &reason}); err != nil {
		t.Fatal(err)
	}
	stillSaved, err := state.MemoryItem(ctx, saved.ID)
	if err != nil || !stillSaved.Saved {
		t.Fatalf("Less retracted current Saved item=%+v err=%v", stillSaved, err)
	}

	keepFixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:saved-1005")
	if _, err := state.AddFeedback(ctx, keepFixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	keptItems, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	var kept domain.MemoryItem
	for _, item := range keptItems {
		if item.CanonicalEvidenceKey == keepFixture.Item.EvidenceKey {
			kept = item
			break
		}
	}
	if kept.ID == "" {
		t.Fatal("routine More did not create the keep fixture memory")
	}
	if _, err := state.KeepMemoryFullCopy(ctx, kept.ID, domain.MemoryFullCopyInput{Content: "independently kept"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, keepFixture.Item.ID, domain.Feedback{Direction: "less", Reason: &reason}); err != nil {
		t.Fatal(err)
	}
	stillKept, err := state.MemoryItem(ctx, kept.ID)
	if err != nil || !stillKept.PermanentKeep || stillKept.FullContent == nil {
		t.Fatalf("Less retracted permanent Keep item=%+v err=%v", stillKept, err)
	}
}

func TestReadLaterAllowsUnavailableTextAsSavedRecall(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:saved-1003")
	// Keep the persisted evidence row but make its local text unavailable.
	if _, err := state.db.ExecContext(ctx, `UPDATE observations SET observation_json=REPLACE(observation_json, 'A bounded source body retained only as recall metadata for saved-1003.', '') WHERE run_id=?`, fixture.Item.RunID); err != nil {
		t.Fatal(err)
	}
	item, _, err := state.ReadLaterTimeline(ctx, fixture.Item.ID)
	if err != nil || !item.Saved || item.RetentionTier != domain.MemoryTierRecall || item.FullContent != nil {
		t.Fatalf("unavailable-text Read later item=%+v err=%v", item, err)
	}
	if _, err := state.KeepMemoryInLibrary(ctx, item.ID); !errors.Is(err, ErrSavedMemoryTextUnavailable) {
		t.Fatalf("unavailable-text Keep err=%v", err)
	}
	finished, err := state.DoneSavedMemory(ctx, item.ID)
	if err != nil || finished.Saved || finished.RetentionTier != domain.MemoryTierRecall {
		t.Fatalf("unavailable-text Done item=%+v err=%v", finished, err)
	}
}
