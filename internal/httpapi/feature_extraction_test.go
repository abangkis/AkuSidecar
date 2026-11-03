package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func featureExtractionRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.Host = "127.0.0.1"
	return r
}

// These requests freeze the HTTP boundary before the remaining adapters move.
// Existing feature tests retain successful workflows, projection and durable
// effects; this matrix adds dispatch, validation precedence and shared guards.
func TestHTTPFeatureExtractionCharacterizationValidation(t *testing.T) {
	for _, tc := range []struct {
		name, method, target, body string
		status                     int
		code                       string
	}{
		{"extensions production gate", "POST", "/api/app-shell/open-extensions", "{", 404, "not_found"},
		{"performance production gate", "POST", "/api/diagnostics/ui-performance", "{", 404, "not_found"},
		{"diagnostic calibration post gate", "POST", "/api/diagnostics/calibration", "{", 404, "not_found"},
		{"diagnostic calibration get gate", "GET", "/api/diagnostics/calibration", "", 404, "not_found"},
		{"shutdown authorization before readiness", "POST", "/api/runtime/shutdown-if-idle", "{", 403, "runtime_control_denied"},
		{"cleanup JSON", "POST", "/api/database/cleanup", "{", 400, "invalid_request"},
		{"calibration session JSON", "POST", "/api/calibration/sessions", "{", 400, "invalid_request"},
		{"calibration missing", "GET", "/api/calibration/sessions/missing", "", 404, "not_found"},
		{"calibration sample path", "PUT", "/api/calibration/sessions/missing", "{", 404, "not_found"},
		{"calibration sample ordinal", "PUT", "/api/calibration/sessions/missing/samples/10", "{", 400, "invalid_request"},
		{"calibration sample JSON", "PUT", "/api/calibration/sessions/missing/samples/0", "{", 400, "invalid_request"},
		{"onboarding JSON", "PUT", "/api/onboarding", "{", 400, "invalid_request"},
		{"credentials JSON", "PUT", "/api/reasoning/credentials", "{", 400, "invalid_request"},
		{"credentials unknown", "PUT", "/api/reasoning/credentials", `{"provider":"missing","secret":"fixture"}`, 400, "unknown_provider"},
		{"settings JSON", "PUT", "/api/settings", "{", 400, "invalid_request"},
		{"run missing", "GET", "/api/runs/missing", "", 404, "not_found"},
		{"context duplicate limit", "GET", "/api/timeline/missing/content-context?limit=1&limit=2", "", 400, "invalid_request"},
		{"context empty limit", "GET", "/api/timeline/missing/content-context?limit=", "", 400, "invalid_request"},
		{"context strict ID precedes limit", "GET", "/api/timeline/a/b/content-context?limit=0", "", 404, "not_found"},
		{"context feedback JSON", "POST", "/api/timeline/missing/content-context-feedback", "{", 400, "invalid_request"},
		{"context feedback missing", "POST", "/api/timeline/missing/content-context-feedback", `{"memoryItemId":"missing","verdict":"relevant"}`, 404, "not_found"},
		{"context undo missing", "POST", "/api/content-context-feedback/missing/undo", "{", 404, "not_found"},
		{"topic create JSON", "POST", "/api/living-topics", "{", 400, "invalid_request"},
		{"topic move undo body", "POST", "/api/living-topic-moves/missing/undo", "{}", 400, "invalid_request"},
		{"topic unknown nested route", "POST", "/api/living-topics/a/b", "{", 404, "not_found"},
		{"topic patch JSON", "PATCH", "/api/living-topics/missing", "{", 400, "invalid_request"},
		{"topic member JSON", "POST", "/api/living-topics/missing/members", "{", 400, "invalid_request"},
		{"topic move JSON", "POST", "/api/living-topics/missing/members/missing/move", "{", 400, "invalid_request"},
		{"topic snapshot body", "POST", "/api/living-topics/missing/snapshots", "{}", 400, "invalid_request"},
		{"topic activation body", "POST", "/api/living-topics/missing/activation", "{}", 400, "invalid_request"},
		{"topic candidate action", "POST", "/api/living-topics/missing/candidates/missing/invalid", "", 404, "not_found"},
		{"topic seen JSON", "POST", "/api/living-topics/missing/seen", "{", 400, "invalid_request"},
		{"library query limit", "GET", "/api/library/items?limit=0", "", 400, "invalid_request"},
		{"library saved limit", "GET", "/api/library/saved?limit=0", "", 400, "invalid_request"},
		{"library storage limit", "GET", "/api/library/storage?limit=0", "", 400, "invalid_request"},
		{"library release strict ID", "POST", "/api/library/items/a/b/release-full-copy", "", 404, "not_found"},
		{"library keep strict ID", "POST", "/api/library/items/a/b/keep-in-library", "", 404, "not_found"},
		{"library done strict ID", "POST", "/api/library/items/a/b/done", "", 404, "not_found"},
		{"library forget strict ID", "POST", "/api/library/items/a/b/forget-permanently", "", 404, "not_found"},
		{"library delete strict ID", "DELETE", "/api/library/items/a/b", "", 404, "not_found"},
		{"library detail missing", "GET", "/api/library/items/missing", "", 404, "not_found"},
		{"reveal JSON", "POST", "/api/auto-update/batches/missing/reveal", "{", 400, "invalid_request"},
		{"capture recovery confirmation", "POST", "/api/capture/x/confirm-recovery", "{}", 400, "invalid_request"},
		{"timeline keep strict ID", "POST", "/api/timeline/a/b/keep-full-copy", "", 404, "not_found"},
		{"timeline save strict ID", "POST", "/api/timeline/a/b/read-later", "", 404, "not_found"},
		{"timeline feedback JSON", "POST", "/api/timeline/missing/feedback", "{", 400, "invalid_request"},
		{"heartbeat auth before body", "POST", "/api/bridge/heartbeat", "{", 401, "invalid_bridge_contract"},
		{"telemetry auth before body", "POST", "/api/bridge/capture-surfaces/events", "{", 401, "invalid_bridge_contract"},
		{"learning reset confirmation", "POST", "/api/operations/reset-learning", `{"confirmation":"reset learning"}`, 400, "invalid_request"},
		{"full reset confirmation", "POST", "/api/operations/full-reset", `{"confirmation":"RESET LEARNING"}`, 400, "invalid_request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			before := f.bookkeeping(t)
			r := featureExtractionRequest(tc.method, tc.target, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			w := f.serve(r)
			var value struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil || w.Code != tc.status || value.Error != tc.code {
				t.Fatalf("status=%d body=%s err=%v; want %d/%s", w.Code, w.Body, err, tc.status, tc.code)
			}
			if !reflect.DeepEqual(before, f.bookkeeping(t)) {
				t.Fatal("rejected request changed durable work")
			}
			if w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != f.s.engine.Epoch() || w.Header().Get("X-Frame-Options") != "DENY" {
				t.Fatal("shared response headers changed")
			}
			select {
			case <-f.s.ShutdownRequested():
				t.Fatal("rejected request scheduled shutdown")
			default:
			}
		})
	}
}

func TestHTTPFeatureExtractionCharacterizationReadEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		target string
		keys   []string
	}{
		{"/api/health", []string{"status", "version", "instanceEpoch", "database", "deployment", "loadProfile"}},
		{"/api/collection/runtime", []string{"collectionRuntime", "xManual", "xCapture", "instagramCapture"}},
		{"/api/runtime/update-readiness", []string{"ready", "reason", "instanceEpoch", "controlAvailable"}},
		{"/api/database/health", []string{"databaseHealth"}},
		{"/api/timeline/storage", []string{"timelineStorage"}},
		{"/api/calibration/active", []string{"calibration"}},
		{"/api/onboarding", []string{"onboarding"}},
		{"/api/settings", []string{"provider", "settings", "collectionRuntime", "reasoningProviders", "reasoningRuntime", "reasoningProcesses", "mediaProvenanceRuntime"}},
		{"/api/reasoning/providers/readiness", []string{"reasoningProviders"}},
		{"/api/living-topics", []string{"topics"}},
		{"/api/living-topics/notifications", []string{"notifications"}},
		{"/api/library/items", []string{"items", "nextCursor"}},
		{"/api/library/saved", []string{"items", "nextCursor"}},
		{"/api/auto-update/status", []string{"autoUpdate"}},
		{"/api/bridge/health", []string{"bridge"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			before := f.bookkeeping(t)
			w := f.serve(featureExtractionRequest("GET", tc.target, strings.NewReader("ignored body")))
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || w.Code != 200 {
				t.Fatalf("status=%d body=%s err=%v", w.Code, w.Body, err)
			}
			for _, key := range tc.keys {
				if _, ok := payload[key]; !ok {
					t.Fatalf("missing envelope key %s", key)
				}
			}
			if !reflect.DeepEqual(before, f.bookkeeping(t)) {
				t.Fatal("empty read changed work")
			}
			for _, method := range []string{"OPTIONS", "DELETE"} {
				w = f.serve(featureExtractionRequest(method, tc.target, nil))
				want := 404
				if method == "OPTIONS" {
					want = 204
				}
				if w.Code != want {
					t.Fatalf("%s %s: status=%d body=%s", method, tc.target, w.Code, w.Body)
				}
			}
		})
	}
}

func TestHTTPFeatureExtractionCharacterizationControlAndContext(t *testing.T) {
	t.Run("extensions dev timeout context", func(t *testing.T) {
		f := newHTTPTimelineFixture(t)
		f.s.config.Dev = true
		f.s.config.Deployment.Mode = "development"
		key := struct{}{}
		parent := context.WithValue(context.Background(), key, "marker")
		called := false
		f.s.SetOpenExtensionsAction(func(ctx context.Context) error {
			called = true
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > 5*time.Second || ctx.Value(key) != "marker" {
				t.Fatal("action context changed")
			}
			return errors.New("fixture unavailable")
		})
		w := f.serve(featureExtractionRequest("POST", "/api/app-shell/open-extensions", nil).WithContext(parent))
		if !called || w.Code != 409 || !strings.Contains(w.Body.String(), "app_shell_action_unavailable") {
			t.Fatal(w.Code, w.Body)
		}
	})
	t.Run("dev ledger and malformed calibration", func(t *testing.T) {
		f := newHTTPTimelineFixture(t)
		f.s.config.Dev = true
		w := f.serve(featureExtractionRequest("GET", "/api/diagnostics/calibration", nil))
		if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"reports":[]}` {
			t.Fatal(w.Code, w.Body)
		}
		r := featureExtractionRequest("POST", "/api/diagnostics/calibration", strings.NewReader("{"))
		r.Header.Set("Content-Type", "application/json")
		w = f.serve(r)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	})
	t.Run("shutdown accepted only after successful response", func(t *testing.T) {
		f := newHTTPTimelineFixture(t)
		token := strings.Repeat("a", 32)
		f.s.config.RuntimeControlToken = token
		r := featureExtractionRequest("POST", "/api/runtime/shutdown-if-idle", nil)
		r.Header.Set("X-Aku-Runtime-Control-Token", token)
		writer := &sessionUpdatesFailWriter{header: make(http.Header), err: errors.New("fixture write failure")}
		if err := f.s.route(writer, r); err == nil {
			t.Fatal("write error lost")
		}
		select {
		case <-f.s.ShutdownRequested():
			t.Fatal("failed response scheduled shutdown")
		case <-time.After(40 * time.Millisecond):
		}
		w := f.serve(r)
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body)
		}
		select {
		case <-f.s.ShutdownRequested():
		case <-time.After(time.Second):
			t.Fatal("accepted shutdown not scheduled")
		}
	})
	for _, tc := range []struct {
		method, target string
		status         int
	}{
		{"POST", "/api/collection/interactive", 200},
		{"POST", "/api/reasoning/runtime/discover", 400},
		{"POST", "/api/auto-update/budget/reset", 409},
		{"POST", "/api/auto-update/usage-limit/restore", 409},
		{"POST", "/api/auto-update/prepare", 409},
		{"POST", "/api/collection/native-reader/close-and-resume", 409},
		{"POST", "/api/ui/activity", 500},
	} {
		t.Run(tc.target, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			w := f.serve(featureExtractionRequest(tc.method, tc.target, nil).WithContext(ctx))
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body)
			}
			if f.count(t, "SELECT COUNT(*) FROM sessions") != 0 {
				t.Fatal("canceled control created session")
			}
		})
	}
}

func TestHTTPFeatureExtractionCharacterizationFeedbackTrace(t *testing.T) {
	f := newHTTPTimelineFixture(t)
	trace := "01234567-89ab-cdef-0123-456789abcdef"
	r := featureExtractionRequest("POST", "/api/timeline/missing/feedback", strings.NewReader("{"))
	r.Header.Set("X-Aku-Feedback-Trace", trace)
	r.Header.Set("Content-Type", "application/json")
	w := f.serve(r)
	if w.Code != 400 || w.Header().Get("X-Aku-Feedback-Trace") != trace || w.Header().Get("Server-Timing") != "" {
		t.Fatal(w.Code, w.Header(), w.Body)
	}
	if f.count(t, "SELECT COUNT(*) FROM feedback_events") != 0 {
		t.Fatal("invalid feedback persisted")
	}
}

func TestHTTPFeatureExtractionCharacterizationSharedSecurity(t *testing.T) {
	for _, target := range []string{"/api/settings", "/api/library/items", "/api/living-topics", "/api/auto-update/status", "/api/operations/full-reset", "/api/bridge/capture-surfaces/events"} {
		t.Run(target, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			r := featureExtractionRequest(http.MethodPost, target, nil)
			r.Header.Set("Origin", "https://untrusted.invalid")
			w := f.serve(r)
			if w.Code != 403 {
				t.Fatal(w.Code, w.Body)
			}
		})
	}
}

func TestHTTPFeatureExtractionCharacterizationContextFeedbackCycle(t *testing.T) {
	f := newHTTPTimelineFixture(t)
	f.visible(t, 1)
	item := domain.ReasonedItem{EvidenceKey: "linkedin:http-timeline-00", Source: domain.SourceLinkedIn, WhatChanged: "Quantum systems research update", WhyItMatters: "Quantum systems research context", SourceURL: "https://www.linkedin.com/feed/update/urn:li:activity:1234567890000000000/"}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE timeline_items SET item_json=? WHERE id='item-00'`, string(raw))
	input := libraryHTTPInput("1234567890123456789")
	input.Title = "Quantum systems research context"
	input.Summary = "Quantum systems local research context"
	input.Tags = []string{"quantum", "research"}
	memory, err := f.s.store.CreateMemoryRecallStub(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.store.KeepMemoryFullCopy(context.Background(), memory.ID, domain.MemoryFullCopyInput{Content: "private context payload"}); err != nil {
		t.Fatal(err)
	}
	lookup := f.serve(featureExtractionRequest("GET", "/api/timeline/item-00/content-context", nil))
	if lookup.Code != 200 || !strings.Contains(lookup.Body.String(), memory.ID) || strings.Contains(lookup.Body.String(), "private context payload") {
		t.Fatal(lookup.Code, lookup.Body)
	}
	body, err := json.Marshal(domain.ContentContextFeedbackInput{MemoryItemID: memory.ID, Verdict: domain.ContentContextFeedbackRelevant})
	if err != nil {
		t.Fatal(err)
	}
	r := featureExtractionRequest("POST", "/api/timeline/item-00/content-context-feedback", strings.NewReader(string(body)))
	r.Header.Set("Content-Type", "application/json")
	writer := &sessionUpdatesFailWriter{header: make(http.Header), err: errors.New("fixture write failure")}
	if err := f.s.route(writer, r); err == nil || writer.status != 201 {
		t.Fatal("feedback write error lost", err)
	}
	if f.count(t, "SELECT COUNT(*) FROM content_context_feedback_events") != 1 {
		t.Fatal("response failure rolled back accepted feedback")
	}
	var id string
	if err := f.db.QueryRow(`SELECT id FROM content_context_feedback_events`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	undo := f.serve(featureExtractionRequest("POST", "/api/content-context-feedback/"+id+"/undo", strings.NewReader("ignored body")))
	if undo.Code != 200 || strings.Contains(undo.Body.String(), "contextKey") || !strings.Contains(undo.Body.String(), `"verdict":"clear"`) {
		t.Fatal(undo.Code, undo.Body)
	}
	if f.count(t, "SELECT COUNT(*) FROM content_context_feedback_events") != 2 {
		t.Fatal("undo must append a clear event")
	}
	undo = f.serve(featureExtractionRequest("POST", "/api/content-context-feedback/"+id+"/undo", nil))
	if undo.Code != 400 || f.count(t, "SELECT COUNT(*) FROM content_context_feedback_events") != 2 {
		t.Fatal("stale undo contract changed", undo.Code, undo.Body)
	}
}

func TestHTTPFeatureExtractionCharacterizationExport(t *testing.T) {
	f := newHTTPTimelineFixture(t)
	for _, target := range []string{"/api/diagnostics/export?since=invalid", "/api/diagnostics/export?sessionId=missing"} {
		w := f.serve(featureExtractionRequest("GET", target, nil))
		if w.Code != 500 || w.Header().Get("Content-Disposition") != "" {
			t.Fatal(w.Code, w.Header(), w.Body)
		}
	}
	w := f.serve(featureExtractionRequest("GET", "/api/diagnostics/export?since=2026-11-01T00:00:00Z&until=2026-11-03T00:00:00Z", nil))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment; filename=aku-diagnostics-") || !strings.Contains(w.Body.String(), `"piiRedacted":true`) {
		t.Fatal(w.Code, w.Header(), w.Body)
	}
}

func TestHTTPFeatureExtractionCharacterizationFullResetWriteFailure(t *testing.T) {
	f := newHTTPTimelineFixture(t)
	cfg := f.s.config
	cfg.Capture.Profile = "expanded"
	cfg.Capture.Visibility = "quiet"
	cfg.Preference.Mode = "promote_unused_budget"
	f.s.engine = engine.New(f.s.store, reasoning.Deterministic{}, cfg, f.s.logger)
	t.Cleanup(f.s.engine.Shutdown)
	f.visible(t, 1)
	r := featureExtractionRequest("POST", "/api/operations/full-reset", strings.NewReader(`{"confirmation":"RESET AKUBROWSER"}`))
	r.Header.Set("Content-Type", "application/json")
	w := &sessionUpdatesFailWriter{header: make(http.Header), err: errors.New("fixture response failure")}
	if err := f.s.route(w, r); err == nil || w.status != 200 {
		t.Fatal("reset/write contract changed", err, w.status)
	}
	if f.count(t, "SELECT COUNT(*) FROM sessions") != 0 || f.count(t, "SELECT COUNT(*) FROM timeline_items") != 0 {
		t.Fatal("response failure undid committed full reset")
	}
}
