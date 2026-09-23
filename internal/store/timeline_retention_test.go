package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type timelineRetentionClock struct{ now time.Time }

func (c timelineRetentionClock) Now() time.Time { return c.now }

func openTimelineRetentionStore(t *testing.T, now time.Time) *Store {
	t.Helper()
	state, err := OpenWithClock(filepath.Join(t.TempDir(), "sidecar.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true), timelineRetentionClock{now: now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	return state
}

func insertTimelineRetentionItem(t *testing.T, state *Store, id string, presentedAt *time.Time, batchState string, payloadBytes int) {
	t.Helper()
	created := state.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339Nano)
	sessionID, runID := "session-"+id, "run-"+id
	if _, err := state.db.Exec(`INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,coverage_json,created_at,completed_at) VALUES(?,?,'completed',1,1,'{"delivery":"visible"}',?,?)`, sessionID, "fixture", created, created); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.Exec(`INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES(?,?,?,0,'completed','completed',?)`, runID, sessionID, domain.SourceX, created); err != nil {
		t.Fatal(err)
	}
	var presented any
	if presentedAt != nil {
		presented = presentedAt.UTC().Format(time.RFC3339Nano)
	}
	if _, err := state.db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,origin_status,presentation,presented_at,batch_state,created_at) VALUES(?,?,?,?,?,0,?,'{}','{}','completed','prepend',?,?,?)`, id, sessionID, runID, domain.SourceX, "evidence-"+id, `{"text":"`+strings.Repeat("x", payloadBytes)+`"}`, presented, batchState, created); err != nil {
		t.Fatal(err)
	}
}

func TestTimelineRetentionActiveProtectsYoungAndHiddenCards(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	young := now.AddDate(0, 0, -3)
	eligible := now.AddDate(0, 0, -20)
	expired := now.AddDate(0, 0, -40)
	insertTimelineRetentionItem(t, state, "young", &young, "visible", 20)
	insertTimelineRetentionItem(t, state, "eligible", &eligible, "visible", 30)
	insertTimelineRetentionItem(t, state, "expired", &expired, "visible", 40)
	insertTimelineRetentionItem(t, state, "hidden", nil, "prepared", 50)

	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	status := result.Timeline
	if status.Mode != "active" || status.TotalItems != 3 || status.ProtectedItems != 2 || status.EligibleItems != 1 || status.RoutineExpiredItems != 0 || status.HiddenItems != 1 || status.MissingPresentationItems != 0 || status.WouldRemoveItems != 0 || status.RemovedItems != 1 || status.RemovedBytes <= 0 {
		t.Fatalf("status=%+v", status)
	}
	var items, receipts int
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_retention_receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if items != 3 || receipts == 0 || status.ReceiptID == "" {
		t.Fatalf("items=%d receipts=%d receipt=%q", items, receipts, status.ReceiptID)
	}
}

func TestTimelineRetentionProtectsQueuedAndClaimedMediaRecapture(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -40)
	for _, status := range []string{"queued", "claimed"} {
		insertTimelineRetentionItem(t, state, status, &old, "visible", 1)
		if _, err := state.db.Exec(`INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,payload_json,created_at) VALUES(?,?,?,?,?,?,'{}',?)`,
			"recapture-"+status, status, domain.SourceX, "https://example.invalid/post", "evidence-"+status, status, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.TotalItems != 2 || result.Timeline.ProcessingProtectedItems != 2 || result.Timeline.RemovedItems != 0 {
		t.Fatalf("in-flight recaptures not protected: %+v", result.Timeline)
	}
	var jobs int
	if err := state.db.QueryRow(`SELECT count(*) FROM media_recaptures WHERE status IN ('queued','claimed')`).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatalf("in-flight jobs=%d err=%v", jobs, err)
	}
	if _, err := state.db.Exec(`UPDATE media_recaptures SET status='completed' WHERE id='recapture-queued'`); err != nil {
		t.Fatal(err)
	}
	result, err = state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.TotalItems != 1 || result.Timeline.ProcessingProtectedItems != 1 || result.Timeline.RemovedItems != 1 {
		t.Fatalf("completed recapture did not release its card: %+v", result.Timeline)
	}
	if err := state.db.QueryRow(`SELECT count(*) FROM media_recaptures WHERE status='claimed'`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("claimed job=%d err=%v", jobs, err)
	}
}

func TestTimelineRetentionProtectsQueuedAndRunningMediaProvenance(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -40)
	for _, status := range []string{"queued", "running"} {
		insertTimelineRetentionItem(t, state, status, &old, "visible", 1)
		if _, err := state.db.Exec(`INSERT INTO media_provenance_assessments(id,timeline_id,session_id,source,media_index,media_kind,target_url,target_url_hash,status,manifest_state,trust_state,ai_origin,provider,verifier_version,created_at) VALUES(?,?,?,? ,0,'image','https://example.invalid/image',?,?,'pending','pending','unknown','local','v1',?)`,
			"provenance-"+status, status, "session-"+status, domain.SourceX, "hash-"+status, status, now.Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.TotalItems != 2 || result.Timeline.ProcessingProtectedItems != 2 || result.Timeline.RemovedItems != 0 {
		t.Fatalf("in-flight provenance not protected: %+v", result.Timeline)
	}
	if _, err := state.db.Exec(`UPDATE media_provenance_assessments SET status='completed' WHERE id='provenance-queued'`); err != nil {
		t.Fatal(err)
	}
	result, err = state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.TotalItems != 1 || result.Timeline.ProcessingProtectedItems != 1 || result.Timeline.RemovedItems != 1 {
		t.Fatalf("completed provenance did not release its card: %+v", result.Timeline)
	}
	var active int
	if err := state.db.QueryRow(`SELECT count(*) FROM media_provenance_assessments WHERE status='running'`).Scan(&active); err != nil || active != 1 {
		t.Fatalf("running provenance=%d err=%v", active, err)
	}
}

func TestTimelineRetentionMigrationBackfillsOnlyVisibleCards(t *testing.T) {
	ctx := context.Background()
	db := lifecycleLegacyDB(t)
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,coverage_json,created_at,completed_at) VALUES('hidden-session','hidden','completed',1,1,'{"delivery":"prepared"}','2026-01-02','2026-01-03');
		INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('hidden-run','hidden-session','x',0,'completed','completed','2026-01-02');
		INSERT INTO auto_update_batches(session_id,state,created_at,prepared_at) VALUES('hidden-session','prepared','2026-01-02','2026-01-03');
		INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,presented_at,batch_state,created_at) VALUES('hidden-post','hidden-session','hidden-run','x','hidden-evidence',0,'{}','{}','2026-01-03','prepared','2026-01-02');`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema27To28(ctx, db); err != nil {
		t.Fatal(err)
	}
	var version, visiblePresented string
	var hiddenPresented sql.NullString
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT presented_at FROM timeline_items WHERE id='post'`).Scan(&visiblePresented); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT presented_at FROM timeline_items WHERE id='hidden-post'`).Scan(&hiddenPresented); err != nil {
		t.Fatal(err)
	}
	if version != "28" || visiblePresented != "2026-01-01" || hiddenPresented.Valid {
		t.Fatalf("version=%q visible=%q hidden=%+v", version, visiblePresented, hiddenPresented)
	}
	if _, err := db.Exec(`DELETE FROM sessions WHERE id='hidden-session'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT presented_at FROM timeline_items WHERE id='hidden-post'`).Scan(&hiddenPresented); err != nil || hiddenPresented.Valid {
		t.Fatalf("hidden card gained presentation after origin retention: %+v %v", hiddenPresented, err)
	}
}

func TestTimelineRetentionMigrationIsAtomicOnConflict(t *testing.T) {
	ctx := context.Background()
	db := lifecycleLegacyDB(t)
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE timeline_retention_receipts(existing TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema27To28(ctx, db); err == nil {
		t.Fatal("expected migration conflict")
	}
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != "27" {
		t.Fatalf("version=%q", version)
	}
}

func TestTimelineRetentionReceiptsStayBounded(t *testing.T) {
	ctx := context.Background()
	state := openTimelineRetentionStore(t, time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC))
	for index := 0; index < timelineReceiptLimit+3; index++ {
		tx, err := state.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = state.enforceTimelineRetentionTx(ctx, tx, state.Now().Add(time.Duration(index)*time.Second)); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err = tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_retention_receipts`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != timelineReceiptLimit {
		t.Fatalf("receipt count=%d", count)
	}
}

func TestTimelineRetentionOldestEligibleFirstAndSoftProtection(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	young, oldest, boundary := now.Add(-time.Hour), now.AddDate(0, 0, -29), now.AddDate(0, 0, -14)
	for i := 0; i < timelineMaxItems; i++ {
		insertTimelineRetentionItem(t, state, fmt.Sprintf("young-%03d", i), &young, "visible", 1)
	}
	insertTimelineRetentionItem(t, state, "oldest", &oldest, "visible", 1)
	insertTimelineRetentionItem(t, state, "boundary", &boundary, "visible", 1)
	settings, _ := state.GetSettings(context.Background())
	result, err := state.EnforceRetention(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.RemovedItems != 2 || result.Timeline.TotalItems != timelineMaxItems || result.Timeline.NeedsAttention {
		t.Fatalf("%+v", result.Timeline)
	}
	insertTimelineRetentionItem(t, state, "protected-overflow", &young, "visible", 1)
	result, err = state.EnforceRetention(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.RemovedItems != 0 || !result.Timeline.NeedsAttention {
		t.Fatalf("%+v", result.Timeline)
	}
}

func TestTimelineRetentionByteBudgetAndStableOldestSelection(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	oldest, newer := now.AddDate(0, 0, -20), now.AddDate(0, 0, -15)
	insertTimelineRetentionItem(t, state, "old", &oldest, "visible", 6*1024*1024)
	insertTimelineRetentionItem(t, state, "new", &newer, "visible", 5*1024*1024)
	settings, _ := state.GetSettings(context.Background())
	result, err := state.EnforceRetention(context.Background(), settings)
	if err != nil {
		t.Fatal(err)
	}
	var id string
	if err = state.db.QueryRow(`SELECT id FROM timeline_items`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if id != "new" || result.Timeline.RemovedItems != 1 || result.Timeline.LogicalBytes > timelineMaxLogicalBytes {
		t.Fatalf("id=%s status=%+v", id, result.Timeline)
	}
}

func TestTimelineRetentionFailsClosedForMissingInvalidAndHiddenPresentation(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -40)
	insertTimelineRetentionItem(t, state, "missing", nil, "visible", 1)
	insertTimelineRetentionItem(t, state, "invalid", &old, "visible", 1)
	insertTimelineRetentionItem(t, state, "hidden", &old, "visible", 1)
	if _, err := state.db.Exec(`UPDATE timeline_items SET presented_at='invalid' WHERE id='invalid'; INSERT INTO auto_update_batches(session_id,state,created_at) VALUES('session-hidden','expired','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(context.Background())
	settings.KnowledgeRetentionDays = 1
	for i := 0; i < 2; i++ {
		result, err := state.EnforceRetention(context.Background(), settings)
		if err != nil {
			t.Fatal(err)
		}
		if result.Timeline.RemovedItems != 0 || result.Timeline.TotalItems != 3 || result.Timeline.HiddenItems != 1 || result.Timeline.MissingPresentationItems != 2 {
			t.Fatalf("%+v", result.Timeline)
		}
	}
}

func TestTimelineRetentionPreservesFeedbackLearningAndSemanticHistory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -31)
	insertTimelineRetentionItem(t, state, "feedback", &old, "visible", 1)
	insertTimelineRetentionItem(t, state, "semantic", &old, "visible", 1)
	for _, direction := range []string{"more", "less"} {
		if _, err := state.db.Exec(`INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,created_at) VALUES(?, 'feedback','session-feedback','run-feedback','evidence-feedback',?,'2026-01-01')`, direction, direction); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.db.Exec(`INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('event','claim','2026-01-01','2026-01-01'); INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,created_at) VALUES('report','event','semantic','session-semantic','run-semantic','evidence-semantic','x','new_event',1,'2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.RemovedItems != 2 || result.Timeline.ProcessingProtectedItems != 0 {
		t.Fatalf("%+v", result.Timeline)
	}
	for _, q := range []string{`SELECT count(*) FROM feedback_events WHERE timeline_id IS NULL`, `SELECT count(*) FROM preference_learning_ledger WHERE active=1`} {
		var count int
		if err := state.db.QueryRow(q).Scan(&count); err != nil || count != 2 {
			t.Fatalf("count=%d err=%v", count, err)
		}
	}
	var reports int
	if err := state.db.QueryRow(`SELECT count(*) FROM semantic_event_reports`).Scan(&reports); err != nil || reports != 1 {
		t.Fatalf("reports=%d err=%v", reports, err)
	}
}

func TestTimelineRetentionReceiptFailureRollsBackEviction(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -31)
	insertTimelineRetentionItem(t, state, "retained", &old, "visible", 1)
	if _, err := state.db.Exec(`CREATE TRIGGER reject_retention_receipt BEFORE INSERT ON timeline_retention_receipts BEGIN SELECT RAISE(ABORT,'receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(context.Background())
	if _, err := state.EnforceRetention(context.Background(), settings); err == nil {
		t.Fatal("expected error")
	}
	var count int
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_items WHERE id='retained'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestTimelineRetentionActiveMigrationPreservesObservationReceipts(t *testing.T) {
	ctx := context.Background()
	db := lifecycleLegacyDB(t)
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema27To28(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO timeline_retention_receipts VALUES('receipt','observe','2026-01-01',14,30,500,10485760,7,123,1,6,2,0,0,0,0,6,100,2,30,0,'2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema28To29(ctx, db); err != nil {
		t.Fatal(err)
	}
	var mode string
	var removed, remaining int
	if err := db.QueryRow(`SELECT mode,removed_items,remaining_items FROM timeline_retention_receipts WHERE id='receipt'`).Scan(&mode, &removed, &remaining); err != nil {
		t.Fatal(err)
	}
	if mode != "observe" || removed != 0 || remaining != 7 {
		t.Fatalf("%s %d %d", mode, removed, remaining)
	}
}

func TestTimelineRetentionPreservesPersonalMemoryCopy(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:12345")
	memory, _, err := state.KeepTimelineFullCopy(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := state.CreateLivingTopic(ctx, "Retained knowledge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.AddLivingTopicMember(ctx, topic.ID, memory.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{TopicID: topic.ID, Status: "ready", Overview: "Retained knowledge.", Claims: []domain.LivingTopicClaim{{Text: "Retained claim.", Assessment: "supported", EvidenceIDs: []string{memory.ID}}}, EvidenceIDs: []string{memory.ID}, Provider: "fixture", Model: "fixture", Effort: "none", InputDigest: "retention-test"}); err != nil {
		t.Fatal(err)
	}
	if _, err = state.db.Exec(`UPDATE timeline_items SET presented_at=?,batch_state='visible' WHERE id=?`, now.AddDate(0, 0, -31).Format(time.RFC3339Nano), fixture.Item.ID); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.Timeline.RemovedItems != 1 {
		t.Fatalf("%+v", result.Timeline)
	}
	var count int
	if err := state.db.QueryRow(`SELECT count(*) FROM memory_items WHERE id=?`, memory.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("memory count=%d err=%v", count, err)
	}
	var claims int
	if err := state.db.QueryRow(`SELECT count(*) FROM memory_retention_claims WHERE memory_item_id=?`, memory.ID).Scan(&claims); err != nil || claims == 0 {
		t.Fatalf("claims=%d err=%v", claims, err)
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Members) != 1 || len(detail.Snapshots) != 1 || detail.Members[0].ID != memory.ID {
		t.Fatalf("knowledge=%+v err=%v", detail, err)
	}
}

func TestTimelineRetentionActiveMigrationRollsBackOnConflict(t *testing.T) {
	ctx := context.Background()
	db := lifecycleLegacyDB(t)
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema27To28(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE timeline_retention_receipts_v28(id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema28To29(ctx, db); err == nil {
		t.Fatal("expected migration conflict")
	}
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "28" {
		t.Fatalf("version=%s err=%v", version, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='timeline_retention_receipts_created'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("index count=%d err=%v", count, err)
	}
}
