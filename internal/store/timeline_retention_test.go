package store

import (
	"context"
	"database/sql"
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

func TestTimelineRetentionObservationProtectsYoungAndHiddenCardsWithoutDeleting(t *testing.T) {
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
	if status.Mode != "observe" || status.TotalItems != 4 || status.ProtectedItems != 2 || status.EligibleItems != 2 || status.RoutineExpiredItems != 1 || status.HiddenItems != 1 || status.MissingPresentationItems != 0 || status.WouldRemoveItems != 1 {
		t.Fatalf("status=%+v", status)
	}
	var items, receipts int
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_retention_receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if items != 4 || receipts == 0 || status.ReceiptID == "" {
		t.Fatalf("items=%d receipts=%d receipt=%q", items, receipts, status.ReceiptID)
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
		if _, err = state.recordTimelineRetentionObservationTx(ctx, tx, state.Now().Add(time.Duration(index)*time.Second)); err != nil {
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
