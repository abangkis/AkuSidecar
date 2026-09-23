package store

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSplitActionAuditIsWhitelistedBoundedAndQueryable(t *testing.T) {
	ctx := context.Background()
	state, err := Open(filepath.Join(t.TempDir(), "split-action-audit.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })

	columns := []string{}
	rows, err := state.db.QueryContext(ctx, `SELECT name FROM pragma_table_info('split_action_audit') ORDER BY cid`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		columns = append(columns, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	wantColumns := []string{"action_id", "action_type", "phase", "outcome", "occurred_at"}
	if len(columns) != len(wantColumns) {
		t.Fatalf("audit columns=%v", columns)
	}
	for i := range columns {
		if columns[i] != wantColumns[i] {
			t.Fatalf("audit columns=%v", columns)
		}
	}

	base := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	for i := 0; i < splitActionAuditLimit+8; i++ {
		value := SplitActionAudit{
			ActionID: "split_action_" + strconv.Itoa(i), ActionType: "open_native_post",
			Phase: "claimed", Outcome: "accepted", OccurredAt: base.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
		}
		if err := state.RecordSplitActionAudit(ctx, value); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	var count int
	if err := state.db.QueryRowContext(ctx, `SELECT count(*) FROM split_action_audit`).Scan(&count); err != nil || count != splitActionAuditLimit {
		t.Fatalf("retained count=%d err=%v", count, err)
	}
	events, err := state.ReadSplitActionAudit(ctx, base.Add(time.Duration(splitActionAuditLimit+3)*time.Second).Format(time.RFC3339Nano), 8)
	if err != nil || len(events) != 5 {
		t.Fatalf("filtered events=%d err=%v", len(events), err)
	}
	newestAt, parseErr := time.Parse(time.RFC3339Nano, events[0].OccurredAt)
	if parseErr != nil || !newestAt.Equal(base.Add(time.Duration(splitActionAuditLimit+7)*time.Second)) {
		t.Fatalf("most recent event=%+v", events[0])
	}
	if _, err := state.ReadSplitActionAudit(ctx, "", splitActionAuditLimit+1); err == nil {
		t.Fatal("unbounded query limit accepted")
	}

	for _, invalid := range []SplitActionAudit{
		{ActionID: "split_action_secret/url", ActionType: "open_source", Phase: "queued", Outcome: "accepted"},
		{ActionID: "split_action_safe", ActionType: "unsupported", Phase: "queued", Outcome: "accepted"},
		{ActionID: "split_action_safe", ActionType: "open_source", Phase: "payload", Outcome: "accepted"},
		{ActionID: "split_action_safe", ActionType: "open_source", Phase: "queued", Outcome: "https://secret"},
		{ActionID: "split_action_safe", ActionType: "open_source", Phase: "queued", Outcome: "accepted", OccurredAt: "not-a-timestamp"},
	} {
		if err := state.RecordSplitActionAudit(ctx, invalid); err == nil {
			t.Fatalf("invalid audit row accepted: %+v", invalid)
		}
	}
}

func TestSchema28To29CreatesSplitActionAuditTable(t *testing.T) {
	ctx := context.Background()
	db := lifecycleLegacyDB(t)
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema27To28(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema28To29(ctx, db); err != nil {
		t.Fatal(err)
	}
	var version, table string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "29" {
		t.Fatalf("schema version=%s err=%v", version, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name='split_action_audit'`).Scan(&table); err != nil || table != "split_action_audit" {
		t.Fatalf("audit table=%s err=%v", table, err)
	}
}
