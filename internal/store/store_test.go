package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	value, err := Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { value.Close() })
	return value
}

func TestFreshSchemaContainsOnlyNewTables(t *testing.T) {
	state := openTestStore(t)
	rows, err := state.db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	want := []string{"bridge_commands", "candidate_assessments", "feedback_events", "knowledge_events", "meta", "observations", "preference_model", "reasoning_invocations", "runs", "sessions", "settings", "timeline_items"}
	if len(names) != len(want) {
		t.Fatalf("tables=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tables=%v", names)
		}
	}
}

func TestSessionCommandAndObservationLifecycle(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "What changed?", settings)
	if err != nil {
		t.Fatal(err)
	}
	run, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || run == nil {
		t.Fatalf("next run: %+v %v", run, err)
	}
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := state.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || claimed == nil || claimed.ID != command.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	observation := domain.Observation{Source: run.Source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Text: "Material update"}}}}, Coverage: map[string]any{"status": "complete"}}
	if err := state.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	stored, err := state.Observations(ctx, run.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("observations: %d %v", len(stored), err)
	}
	updated, err := state.GetRun(ctx, run.ID)
	if err != nil || updated.Status != "reasoning" {
		t.Fatalf("run: %+v %v", updated, err)
	}
}

func TestBridgeTokenComparison(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	token, err := state.BridgeToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.MatchesBridgeToken(ctx, token) || state.MatchesBridgeToken(ctx, token+"x") {
		t.Fatal("constant-time token boundary failed")
	}
}

func TestSchemaMismatchFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sidecar.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','99')`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(path, domain.DefaultSettings("expanded", "quiet", "rank_only", true))
	if err == nil {
		t.Fatal("schema mismatch must fail")
	}
}
