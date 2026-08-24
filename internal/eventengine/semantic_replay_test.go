package eventengine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type replayFixtureReader struct {
	sessions []store.SemanticReplaySession
	events   []store.SemanticReplayEvent
}

func (f replayFixtureReader) ReadSemanticReplay(context.Context, int) ([]store.SemanticReplaySession, []store.SemanticReplayEvent, error) {
	return f.sessions, f.events, nil
}

func TestAnalyzeSemanticReplayClassifiesReviewCandidatesConservatively(t *testing.T) {
	fixture := replayFixtureReader{
		sessions: []store.SemanticReplaySession{
			{Status: "completed", InvocationStatus: "bypassed", DiagnosticsAvailable: true, StrongestOverlap: 0, SignalReceipt: receiptWithTrigger(1, 0, 0), Reports: []store.SemanticReplayReport{{Relation: "new_event"}}},
			{Status: "completed", InvocationStatus: "completed", ResolverInvoked: true, DiagnosticsAvailable: true, StrongestOverlap: 3, SignalReceipt: receiptWithTrigger(0, 1, 0), Reports: []store.SemanticReplayReport{{Relation: "new_event"}}},
			{Status: "completed", InvocationStatus: "completed", ResolverInvoked: true, SignalReceipt: receiptWithTrigger(0, 0, 1), Reports: []store.SemanticReplayReport{{Relation: "duplicate_report"}}},
			{Status: "completed", InvocationStatus: "completed", ResolverInvoked: true, Reports: []store.SemanticReplayReport{{Relation: "material_update"}}},
			{Status: "completed", InvocationStatus: "completed", ResolverInvoked: true, ActiveCorrections: 1, Reports: []store.SemanticReplayReport{{Relation: "new_event"}}},
			{Status: "completed", InvocationStatus: "completed", ResolverInvoked: true, ActiveCorrections: 0, UndoneCorrections: 1, Reports: []store.SemanticReplayReport{{Relation: "new_event"}}},
			{Status: "cancelled", InvocationStatus: "not_recorded"},
		},
		events: []store.SemanticReplayEvent{
			{CanonicalClaim: "alpha event one"},
			{CanonicalClaim: "common event two"},
			{CanonicalClaim: "common event three"},
			{CanonicalClaim: "common event four"},
			{CanonicalClaim: "common event five"},
			{CanonicalClaim: "https://secret.invalid private-evidence"},
		},
	}
	report, err := AnalyzeSemanticReplay(context.Background(), fixture, 20)
	if err != nil {
		t.Fatal(err)
	}
	if report.Classification.ObservedLocalBypassSessions != 1 {
		t.Fatalf("observed bypass=%d", report.Classification.ObservedLocalBypassSessions)
	}
	if report.Classification.CounterfactualReviewCandidateSessions != 1 {
		t.Fatalf("review candidates=%d", report.Classification.CounterfactualReviewCandidateSessions)
	}
	if report.Classification.ReceiptBackedReviewCandidateSessions != 1 || report.Classification.LegacyAllNewReviewCandidateSessions != 0 || report.SignalReceipt.SessionsWithReceipt != 3 {
		t.Fatalf("receipt classification=%+v aggregate=%+v", report.Classification, report.SignalReceipt)
	}
	if report.Classification.RequiresModelSessions != 5 || report.Classification.InsufficientDataSessions != 1 {
		t.Fatalf("classification=%+v", report.Classification)
	}
	if report.Corrections.ActiveUserCorrections != 1 || report.Corrections.UndoneUserCorrections != 1 {
		t.Fatalf("corrections=%+v", report.Corrections)
	}
	if report.HistoricalCompatibility.Actor != 6 || report.HistoricalCompatibility.Object != 6 || report.HistoricalCompatibility.Time != 6 || report.HistoricalCompatibility.ExactEventKey != 6 {
		t.Fatalf("historical compatibility=%+v", report.HistoricalCompatibility)
	}
	if report.TriggerRarityBuckets["rare"] != 1 || report.TriggerRarityBuckets["common"] != 1 || report.TriggerRarityBuckets["unknown"] != 1 {
		t.Fatalf("trigger rarity=%v", report.TriggerRarityBuckets)
	}
	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("aggregate JSON is not deterministic")
	}
	if strings.Contains(string(first), "TOP SECRET CLAIM") || strings.Contains(string(first), "https://secret.invalid") || strings.Contains(string(first), "Private Author") || strings.Contains(string(first), "private-evidence") {
		t.Error("aggregate output leaked raw source evidence")
	}
}

func TestAnalyzeSemanticReplayMarksTriggerSignalsUnavailable(t *testing.T) {
	report, err := AnalyzeSemanticReplay(context.Background(), replayFixtureReader{
		sessions: []store.SemanticReplaySession{{Status: "completed", InvocationStatus: "completed", ResolverInvoked: true, Reports: []store.SemanticReplayReport{{Relation: "new_event"}}}},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.TriggerRarityBuckets["unavailable"] != 1 || report.TriggerOverlapBuckets["unavailable"] != 1 {
		t.Fatalf("unavailable trigger signals=%+v %+v", report.TriggerRarityBuckets, report.TriggerOverlapBuckets)
	}
	if report.Classification.CounterfactualReviewCandidateSessions != 1 {
		t.Fatal("completed all-new session should be review candidate even with unavailable diagnostics")
	}
	if report.Classification.LegacyAllNewReviewCandidateSessions != 1 || report.Classification.ReceiptBackedReviewCandidateSessions != 0 {
		t.Fatalf("legacy classification=%+v", report.Classification)
	}
}

func TestAnalyzeSemanticReplayAgainstLegacyV9ReadOnlyDatabase(t *testing.T) {
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
		INSERT INTO semantic_event_reports VALUES('legacy-report','legacy-session','new_event','2026-08-03T00:00:00Z');
		INSERT INTO semantic_events VALUES('legacy-event','legacy claim','actor','action','object','[]','2026-08-03T00:00:00Z');
	`)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	sessions, _, err := state.ReadSemanticReplay(context.Background(), 1)
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SignalReceipt != nil {
		state.Close()
		t.Fatalf("legacy receipt should be unavailable: %+v", sessions)
	}
	report, err := AnalyzeSemanticReplay(context.Background(), state, 1)
	if err != nil {
		state.Close()
		t.Fatal(err)
	}
	if report.SessionsAnalyzed != 1 || report.SignalReceipt.SessionsWithReceipt != 0 || report.UnavailableSignals["signal_receipt"] != 1 {
		state.Close()
		t.Fatalf("legacy replay report=%+v", report)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("legacy read-only replay changed database bytes")
	}
}

func receiptWithTrigger(rare, common, unknown int) *domain.SemanticSignalReceipt {
	return &domain.SemanticSignalReceipt{Version: semanticSignalReceiptVersion, TriggerTokenTotal: 1, TriggerRareTokenCount: rare, TriggerCommonTokenCount: common, TriggerUnknownTokenCount: unknown}
}
