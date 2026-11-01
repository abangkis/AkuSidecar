package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func diagnosticsExportFixture(t *testing.T) (*Engine, *store.Store, string) {
	t.Helper()
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	settings.ReasoningProvider = "deterministic"
	settings.ReasoningExecutablePath = `C:\fixture\diagnostics.exe`
	database := filepath.Join(t.TempDir(), "sidecar.db")
	state, err := store.Open(database, settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	runtime := New(state, reasoning.Deterministic{}, config.Config{Root: "private-root-sentinel"}, log.New(io.Discard, "", 0))
	return runtime, state, database
}

// SQL mutations are restricted to this disposable fixture database. They seed
// ordering metadata and make individual read failures observable without new
// production injection points or a second Store implementation.
func diagnosticsExportSQL(t *testing.T, database, statement string, args ...any) {
	t.Helper()
	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statement, args...); err != nil {
		t.Fatal(err)
	}
}

func diagnosticsExportSession(t *testing.T, state *store.Store) domain.Session {
	t.Helper()
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateUpdateSession(context.Background(), "diagnostics fixture intent", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func TestDiagnosticsExportEmptyJSONContract(t *testing.T) {
	runtime, _, _ := diagnosticsExportFixture(t)
	value, err := runtime.DiagnosticsExport(context.Background(), "", "2026-11-01T12:00:00+07:00", "")
	if err != nil {
		t.Fatal(err)
	}
	if value.Metadata.Version != domain.ApplicationVersion || value.Metadata.InstanceEpoch != runtime.Epoch() || !value.Metadata.PIIRedacted || value.Metadata.TimeRange != (domain.TimeRange{Since: "2026-10-31T12:00:00+07:00", Until: "2026-11-01T12:00:00+07:00"}) {
		t.Fatalf("metadata contract: %+v", value.Metadata)
	}
	if value.Config.ReasoningProvider != "deterministic" || value.Config.ReasoningExecutablePath != `C:\fixture\diagnostics.exe` || value.Config.DatabaseSchemaVersion != store.SchemaVersion || value.Config.BridgeContractVersion != domain.BridgeContractVersion || len(value.Config.ReasoningProcesses) == 0 {
		t.Fatalf("config projection: %+v", value.Config)
	}
	if value.ModelUsage.WindowDays != 0 || value.ModelUsage.Status != "" || value.ModelUsage.AggregateTokens != (domain.TokenUsage{}) {
		t.Fatalf("the existing ignored unsupported usage window changed: %+v", value.ModelUsage)
	}
	if value.Bridge.State != "reconnecting" || value.Bridge.Compatible || value.Bridge.Actual != "<nil>" || value.Bridge.NegotiatedProtocol != "<nil>" {
		t.Fatalf("Bridge projection: %+v", value.Bridge)
	}
	if _, err := time.Parse(time.RFC3339Nano, value.GeneratedAt); err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(wire, &fields); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	if !reflect.DeepEqual(keys, []string{"bridge", "config", "errors", "generatedAt", "metadata", "modelUsage", "reasoning", "sessions"}) || string(fields["sessions"]) != "[]" || string(fields["reasoning"]) != "null" || string(fields["errors"]) != "[]" {
		t.Fatalf("JSON shape: %s", wire)
	}
	var metadata map[string]json.RawMessage
	var usage map[string]json.RawMessage
	_ = json.Unmarshal(fields["metadata"], &metadata)
	_ = json.Unmarshal(fields["modelUsage"], &usage)
	if _, exists := metadata["sessionId"]; exists || string(usage["categories"]) != "[]" || strings.Contains(string(wire), "private-root-sentinel") {
		t.Fatalf("omission/empty projection contract: %s", wire)
	}
}

func TestDiagnosticsExportTimeRangeContract(t *testing.T) {
	runtime, _, _ := diagnosticsExportFixture(t)
	until := time.Now().UTC().Truncate(time.Second)
	for _, tc := range []struct {
		name string
		days int
		want int
	}{{"unsupported-one-day", 1, 0}, {"seven-day-window", 6, 7}, {"thirty-day-window", 29, 30}, {"ninety-day-window", 89, 90}, {"reversed-range", -1, 0}} {
		t.Run(tc.name, func(t *testing.T) {
			since := until.Add(-time.Duration(tc.days) * 24 * time.Hour).Format(time.RFC3339)
			value, err := runtime.DiagnosticsExport(context.Background(), since, until.Format(time.RFC3339), "")
			if err != nil || value.Metadata.TimeRange.Since != since || value.Metadata.TimeRange.Until != until.Format(time.RFC3339) || value.ModelUsage.WindowDays != tc.want {
				t.Fatalf("range/window contract: metadata=%+v usage=%+v err=%v", value.Metadata, value.ModelUsage, err)
			}
		})
	}
	before := time.Now().UTC().Add(-time.Second)
	value, err := runtime.DiagnosticsExport(context.Background(), "", "", "")
	after := time.Now().UTC()
	if err != nil {
		t.Fatal(err)
	}
	since, err := time.Parse(time.RFC3339, value.Metadata.TimeRange.Since)
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse(time.RFC3339, value.Metadata.TimeRange.Until)
	if err != nil || end.Before(before) || end.After(after) || end.Sub(since) != 24*time.Hour {
		t.Fatalf("default range: %+v err=%v", value.Metadata.TimeRange, err)
	}
}

func TestDiagnosticsExportExplicitSessionPrecedesActiveSession(t *testing.T) {
	runtime, state, database := diagnosticsExportFixture(t)
	old := diagnosticsExportSession(t, state)
	if err := state.CancelSession(context.Background(), old.ID); err != nil {
		t.Fatal(err)
	}
	active := diagnosticsExportSession(t, state)
	diagnosticsExportSQL(t, database, `UPDATE sessions SET coverage_json=? WHERE id=?`, `{"pipelineStage":"fixture-stage","custom":"retained"}`, old.ID)
	diagnosticsExportSQL(t, database, `UPDATE runs SET stage='fixture-run',error_json=? WHERE id=?`, `{"code":"fixture-code","message":"fixture-message"}`, old.Runs[0].ID)
	for _, tc := range []struct{ requested, expected string }{{"", active.ID}, {old.ID, old.ID}} {
		value, err := runtime.DiagnosticsExport(context.Background(), "", "", tc.requested)
		if err != nil || len(value.Sessions) != 1 || value.Sessions[0].ID != tc.expected || value.Metadata.SessionID != tc.requested {
			t.Fatalf("selection contract: sessions=%+v metadata=%+v err=%v", value.Sessions, value.Metadata, err)
		}
		if tc.requested != "" {
			session := value.Sessions[0]
			if session.PipelineStage != "fixture-stage" || session.Coverage["custom"] != "retained" || session.CompletedAt == nil || session.Runs[0].Error == nil || *session.Runs[0].Error != (domain.RunError{Code: "fixture-code", Message: "fixture-message", Stage: "fixture-run"}) {
				t.Fatalf("explicit session/run projection: %+v", session)
			}
		}
	}
	_, err := runtime.DiagnosticsExport(context.Background(), "", "", "missing-explicit-session")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing explicit session fell back to active: %v", err)
	}
}

func TestDiagnosticsExportInboxLimitAndProjection(t *testing.T) {
	runtime, state, database := diagnosticsExportFixture(t)
	var sessions []domain.Session
	for i := 0; i < 12; i++ {
		session := diagnosticsExportSession(t, state)
		if err := state.CancelSession(context.Background(), session.ID); err != nil {
			t.Fatal(err)
		}
		diagnosticsExportSQL(t, database, `UPDATE sessions SET created_at=?,coverage_json=? WHERE id=?`, fmt.Sprintf("2020-01-%02dT00:00:00Z", i+1), `{"pipelineStage":"not-projected-in-inbox"}`, session.ID)
		sessions = append(sessions, session)
	}
	value, err := runtime.DiagnosticsExport(context.Background(), "2026-10-26T00:00:00Z", "2026-11-01T00:00:00Z", "")
	if err != nil || len(value.Sessions) != 10 {
		t.Fatalf("inbox limit: len=%d err=%v", len(value.Sessions), err)
	}
	for i, session := range value.Sessions {
		if session.ID != sessions[11-i].ID || session.Coverage != nil || session.PipelineStage != "" || len(session.Runs) != 2 || session.Runs[0].Error != nil {
			t.Fatalf("inbox order/projection at %d: %+v", i, session)
		}
	}
}

func TestDiagnosticsExportTelemetryGroupingAndUsage(t *testing.T) {
	runtime, state, _ := diagnosticsExportFixture(t)
	session := diagnosticsExportSession(t, state)
	input, cached, output, reasoningTokens := int64(11), int64(3), int64(5), int64(2)
	now := time.Now().UTC().Add(-time.Minute)
	invocations := []domain.ReasoningTelemetry{
		{ID: "first-run-first", RunID: session.Runs[0].ID, Phase: "acquisition_planning", Provider: "first-provider", Model: "first-model", Effort: "high", DurationMS: 13, CallerLatencyMS: 15, QueueWaitMS: 2, ProviderExecutionMS: 7, ResponseTotalMS: 11, InputTokens: &input, CachedInputTokens: &cached, OutputTokens: &output, ReasoningOutputTokens: &reasoningTokens, Status: "completed", CreatedAt: now.Format(time.RFC3339Nano)},
		{ID: "second-run-first", RunID: session.Runs[1].ID, Phase: "candidate_evaluation", Provider: "other-provider", Model: "other-model", Effort: "low", DurationMS: 7, Status: "completed", CreatedAt: now.Add(time.Second).Format(time.RFC3339Nano)},
		{ID: "first-run-last", RunID: session.Runs[0].ID, Phase: "candidate_evaluation", Provider: "last-provider", Model: "last-model", Effort: "medium", DurationMS: 17, Status: "failed", CreatedAt: now.Add(2 * time.Second).Format(time.RFC3339Nano)},
	}
	for i := len(invocations) - 1; i >= 0; i-- {
		if err := state.SaveTelemetry(context.Background(), invocations[i]); err != nil {
			t.Fatal(err)
		}
	}
	end := time.Now().UTC()
	value, err := runtime.DiagnosticsExport(context.Background(), end.Add(-6*24*time.Hour).Format(time.RFC3339), end.Format(time.RFC3339), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(value.Reasoning))
	for _, invocation := range value.Reasoning {
		ids = append(ids, invocation.ID)
	}
	if !reflect.DeepEqual(ids, []string{"first-run-first", "first-run-last", "second-run-first"}) {
		t.Fatalf("telemetry must remain grouped by exported run order: %v", ids)
	}
	run := value.Sessions[0].Runs[0]
	if run.ElapsedMillis != 30 || run.Provider != "first-provider" || run.Model != "first-model" || run.Effort != "high" || run.Telemetry == nil || run.Telemetry.ID != "first-run-last" || run.Telemetry.InputTokens != nil {
		t.Fatalf("first/last invocation projection: %+v", run)
	}
	if value.Reasoning[0].CallerLatencyMS != 15 || value.Reasoning[0].QueueWaitMS != 2 || value.Reasoning[0].ProviderExecutionMS != 7 || value.Reasoning[0].ResponseTotalMS != 11 || value.Reasoning[0].InputTokens == nil || *value.Reasoning[0].InputTokens != input {
		t.Fatalf("invocation detail lost: %+v", value.Reasoning[0])
	}
	if value.ModelUsage.WindowDays != 7 || value.ModelUsage.SessionCount != 1 || value.ModelUsage.AggregateTokens != (domain.TokenUsage{InputTokens: 11, CachedInputTokens: 3, OutputTokens: 5, ReasoningOutputTokens: 2}) || len(value.ModelUsage.Categories) == 0 || len(value.Errors) != 0 {
		t.Fatalf("usage projection: %+v errors=%+v", value.ModelUsage, value.Errors)
	}
}

func TestDiagnosticsExportFailurePriority(t *testing.T) {
	for _, tc := range []struct{ name, since, until, wantParse string }{
		{"since-before-until-and-store", "bad-since", "bad-until", "bad-since"},
		{"until-before-store", "2026-10-26T00:00:00Z", "bad-until", "bad-until"},
		{"settings-before-session", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, state, _ := diagnosticsExportFixture(t)
			if err := state.Close(); err != nil {
				t.Fatal(err)
			}
			value, err := runtime.DiagnosticsExport(context.Background(), tc.since, tc.until, "missing-session")
			if err == nil || !reflect.DeepEqual(value, domain.DiagnosticsExport{}) {
				t.Fatalf("failure result: %+v err=%v", value, err)
			}
			if tc.wantParse != "" {
				var parseError *time.ParseError
				if !errors.As(err, &parseError) || parseError.Value != tc.wantParse {
					t.Fatalf("parse priority: %v", err)
				}
			} else if !strings.HasPrefix(err.Error(), "read settings: ") {
				t.Fatalf("settings error was changed: %v", err)
			}
		})
	}
	runtime, _, _ := diagnosticsExportFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runtime.DiagnosticsExport(ctx, "", "", "missing-session")
	if !errors.Is(err, context.Canceled) || !strings.HasPrefix(err.Error(), "read settings: ") {
		t.Fatalf("context cause changed: %v", err)
	}
}

func TestDiagnosticsExportIgnoredTelemetryFailure(t *testing.T) {
	runtime, state, database := diagnosticsExportFixture(t)
	session := diagnosticsExportSession(t, state)
	diagnosticsExportSQL(t, database, `ALTER TABLE reasoning_invocations RENAME TO unavailable_reasoning_invocations`)
	_, unavailable := state.ReasoningInvocationsBySession(context.Background(), session.ID)
	if unavailable == nil {
		t.Fatal("telemetry failure fixture did not fail")
	}
	value, err := runtime.DiagnosticsExport(context.Background(), "2026-10-26T00:00:00Z", "2026-11-01T00:00:00Z", session.ID)
	if err != nil || len(value.Sessions) != 1 || value.Reasoning != nil || value.Sessions[0].Runs[0].Telemetry != nil || value.ModelUsage.WindowDays != 0 || value.ModelUsage.Status != "" || len(value.Errors) != 0 {
		t.Fatalf("best-effort failure changed: %+v err=%v", value, err)
	}
}

func TestDiagnosticsExportInboxReadFailure(t *testing.T) {
	runtime, state, database := diagnosticsExportFixture(t)
	diagnosticsExportSQL(t, database, `ALTER TABLE sessions RENAME TO unavailable_sessions`)
	_, _, expected := state.ListInboxSessions(context.Background(), 10, 0)
	if expected == nil {
		t.Fatal("inbox failure fixture did not fail")
	}
	value, err := runtime.DiagnosticsExport(context.Background(), "", "", "")
	if err == nil || err.Error() != expected.Error() || !reflect.DeepEqual(value, domain.DiagnosticsExport{}) {
		t.Fatalf("inbox failure was swallowed or wrapped: %+v err=%v want=%v", value, err, expected)
	}
}

func TestDiagnosticsExportBridgeStringContract(t *testing.T) {
	value := convertBridgeStatus(BridgeStatus{State: "incompatible", Reasons: []string{"fixture-reason"}, Warnings: []string{"fixture-warning"}, Expected: map[string]any{"b": 2, "a": 1}})
	expected := domain.BridgeStatus{State: "incompatible", Reasons: []string{"fixture-reason"}, Warnings: []string{"fixture-warning"}, Expected: "map[a:1 b:2]", Actual: "<nil>", NegotiatedProtocol: "<nil>"}
	if !reflect.DeepEqual(value, expected) {
		t.Fatalf("Bridge diagnostic string contract: %+v", value)
	}
}
