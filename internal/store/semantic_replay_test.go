package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestReadSemanticReplayLegacyV9ReceiptFallsBackToUnavailable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v9.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE sessions (id TEXT PRIMARY KEY,status TEXT,created_at TEXT);
		CREATE TABLE event_resolution_invocations (session_id TEXT PRIMARY KEY,status TEXT,candidate_count INTEGER,shortlist_count INTEGER);
		CREATE TABLE event_resolution_diagnostics (session_id TEXT PRIMARY KEY,resolver_invoked INTEGER,strongest_overlap INTEGER,trigger_reason TEXT,trigger_tokens_json TEXT);
		CREATE TABLE semantic_event_reports (id TEXT PRIMARY KEY,session_id TEXT,relation TEXT,created_at TEXT);
		CREATE TABLE semantic_event_corrections (report_id TEXT,undone_at TEXT);
		CREATE TABLE semantic_events (id TEXT PRIMARY KEY,canonical_claim TEXT,actor TEXT,action TEXT,object TEXT,aliases_json TEXT,last_seen_at TEXT);
		INSERT INTO sessions VALUES('legacy-session','completed','2026-08-03T00:00:00Z');
		INSERT INTO event_resolution_invocations VALUES('legacy-session','completed',1,1);
		INSERT INTO event_resolution_diagnostics VALUES('legacy-session',1,3,'historical_shortlist','["legacy"]');
		INSERT INTO semantic_events VALUES('legacy-event','legacy claim','actor','action','object','[]','2026-08-03T00:00:00Z');
	`)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	state, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	sessions, _, err := state.ReadSemanticReplay(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SignalReceipt != nil {
		t.Fatalf("legacy receipt should be unavailable: %+v", sessions)
	}
}

func TestReadSemanticReplayUsesReadOnlyProjection(t *testing.T) {
	ctx := context.Background()
	database := filepath.Join(t.TempDir(), "semantic-replay.db")
	settings := domain.DefaultSettings("standard", "quiet", "rank_only", false)
	writable, err := Open(database, settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedSemanticReplayFixture(ctx, writable, "session-bypassed", "bypassed", "new_event", false); err != nil {
		writable.Close()
		t.Fatal(err)
	}
	if err := seedSemanticReplayFixture(ctx, writable, "session-model", "completed", "duplicate_report", true); err != nil {
		writable.Close()
		t.Fatal(err)
	}
	if err := seedSemanticReplayFixture(ctx, writable, "session-undone", "completed", "new_event", true); err != nil {
		writable.Close()
		t.Fatal(err)
	}
	if _, err := writable.db.ExecContext(ctx, `UPDATE semantic_event_corrections SET undone_at=? WHERE id=?`, domain.Now(), "correction-session-undone"); err != nil {
		writable.Close()
		t.Fatal(err)
	}
	for sessionID, stamp := range map[string]string{
		"session-bypassed": "2026-08-01T00:00:00Z",
		"session-model":    "2026-08-02T00:00:00Z",
		"session-undone":   "2026-08-03T00:00:00Z",
	} {
		if _, err := writable.db.ExecContext(ctx, `UPDATE sessions SET created_at=? WHERE id=?`, stamp, sessionID); err != nil {
			writable.Close()
			t.Fatal(err)
		}
		if _, err := writable.db.ExecContext(ctx, `UPDATE semantic_events SET last_seen_at=? WHERE id=?`, stamp, "event-"+sessionID); err != nil {
			writable.Close()
			t.Fatal(err)
		}
	}
	if _, err := writable.db.ExecContext(ctx, `UPDATE semantic_events SET canonical_claim=? WHERE id=?`, "newest bounded event", "event-session-undone"); err != nil {
		writable.Close()
		t.Fatal(err)
	}
	if err := writable.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}

	readonly, err := OpenReadOnly(database)
	if err != nil {
		t.Fatal(err)
	}
	sessions, events, err := readonly.ReadSemanticReplay(ctx, 10)
	if err != nil {
		readonly.Close()
		t.Fatal(err)
	}
	if len(sessions) != 3 || len(events) != 3 {
		readonly.Close()
		t.Fatalf("unexpected projection sizes sessions=%d events=%d", len(sessions), len(events))
	}
	var bypassed, active, undone int
	for _, session := range sessions {
		if session.InvocationStatus == "bypassed" {
			bypassed++
		}
		active += session.ActiveCorrections
		undone += session.UndoneCorrections
	}
	if bypassed != 1 {
		t.Errorf("expected one bypassed session, got %d", bypassed)
	}
	if active != 1 || undone != 1 {
		t.Errorf("correction aggregation active=%d undone=%d", active, undone)
	}
	newestSessions, newestEvents, err := readonly.ReadSemanticReplay(ctx, 1)
	if err != nil {
		readonly.Close()
		t.Fatal(err)
	}
	if len(newestSessions) != 1 || newestSessions[0].UndoneCorrections != 1 {
		t.Fatalf("bounded session query did not select newest session: %+v", newestSessions)
	}
	if len(newestEvents) == 0 || newestEvents[0].CanonicalClaim != "newest bounded event" {
		t.Fatalf("corpus query did not select newest event first: %+v", newestEvents)
	}
	if err := readonly.SaveSettings(ctx, settings); err == nil {
		t.Error("expected writes through read-only handle to fail")
	}
	if err := readonly.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("read-only replay changed database bytes")
	}
}

func seedSemanticReplayFixture(ctx context.Context, state *Store, sessionID, invocationStatus, relation string, correction bool) error {
	now := domain.Now()
	if _, err := state.db.ExecContext(ctx, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at,completed_at) VALUES(?,?,?,?,?,?,?)`, sessionID, "manual", "completed", 5, 10, now, now); err != nil {
		return err
	}
	runID := "run-" + sessionID
	if _, err := state.db.ExecContext(ctx, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at,completed_at) VALUES(?,?,?,?,?,?,?,?)`, runID, sessionID, domain.SourceX, 0, "completed", "completed", now, now); err != nil {
		return err
	}
	timelineID := "timeline-" + sessionID
	itemRaw := `{"id":"item-opaque","whatChanged":"private candidate claim","whyItMatters":"private context","source":"x","sourceUrl":"https://private.example/secret","evidenceKey":"private-evidence","eventKey":"private-event","author":"Private Author","publishedAt":"2026-08-01T00:00:00Z"}`
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, timelineID, sessionID, runID, domain.SourceX, "private-evidence", 0, itemRaw, `{}`, `{}`, now); err != nil {
		return err
	}
	eventID := "event-" + sessionID
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_events(id,canonical_claim,actor,action,object,event_kind,aliases_json,event_start,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, eventID, "Private canonical claim", "Private Actor", "changed", "Private Object", "other", `["private-event"]`, "2026-08-01T00:00:00Z", now, now); err != nil {
		return err
	}
	reportID := "report-" + sessionID
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,corrected,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, reportID, eventID, timelineID, sessionID, runID, "private-evidence", domain.SourceX, relation, .99, "private reason", correction, now); err != nil {
		return err
	}
	if correction {
		if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_event_corrections(id,report_id,timeline_id,action,from_event_id,from_relation,to_event_id,to_relation,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, "correction-"+sessionID, reportID, timelineID, "not_same_event", eventID, relation, eventID, "new_event", now); err != nil {
			return err
		}
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO event_resolution_invocations(session_id,status,provider,model,effort,candidate_count,shortlist_count,unique_items,duplicate_reports,duration_ms,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, sessionID, invocationStatus, "test", "test", "none", 1, 1, 1, 0, 0, now); err != nil {
		return err
	}
	_, err := state.db.ExecContext(ctx, `INSERT INTO event_resolution_diagnostics(session_id,historical_event_count,resolver_invoked,trigger_reason,strongest_overlap) VALUES(?,?,?,?,?)`, sessionID, 1, invocationStatus == "completed", "fixture", 3)
	return err
}
