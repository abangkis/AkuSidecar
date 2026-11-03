package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const inboxTracePath = "/api/inbox/runs/trace-run/trace"

type httpInboxFixture struct{ sessionUpdatesFixture }

func newHTTPInboxFixture(t *testing.T) httpInboxFixture {
	t.Helper()
	return httpInboxFixture{newSessionUpdatesFixture(t)}
}

func inboxReadJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f httpInboxFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f httpInboxFixture) snapshot(t *testing.T) []int {
	t.Helper()
	var result []int
	for _, table := range []string{"sessions", "runs", "bridge_commands", "observations", "timeline_items", "candidate_assessments", "selection_corrections", "knowledge_events", "reasoning_invocations"} {
		result = append(result, f.count(t, "SELECT COUNT(*) FROM "+table))
	}
	return result
}

type inboxListResponse struct {
	Sessions []domain.InboxSession `json:"sessions"`
	Total    int                   `json:"total"`
	Limit    int                   `json:"limit"`
	Offset   int                   `json:"offset"`
}

func (f httpInboxFixture) list(t *testing.T, w *httptest.ResponseRecorder) inboxListResponse {
	t.Helper()
	payload := f.json(t, w, 200)
	var result inboxListResponse
	if len(payload) != 4 || string(payload["sessions"]) == "null" || json.Unmarshal(w.Body.Bytes(), &result) != nil {
		t.Fatal("Inbox list envelope changed", w.Body)
	}
	return result
}

func (f httpInboxFixture) trace(t *testing.T, w *httptest.ResponseRecorder) domain.InboxFlowTrace {
	t.Helper()
	payload := f.json(t, w, 200)
	var result domain.InboxFlowTrace
	var fields map[string]json.RawMessage
	if len(payload) != 1 || json.Unmarshal(payload["trace"], &result) != nil || json.Unmarshal(payload["trace"], &fields) != nil || len(fields) != 8 || result.Items == nil {
		t.Fatal("Inbox trace envelope changed", w.Body)
	}
	if strings.Contains(w.Body.String(), "evidenceKey") || strings.Contains(w.Body.String(), "x:00000000000000000000070") || strings.Contains(w.Body.String(), "PRIVATE_COVERAGE") {
		t.Fatal("trace leaked internal identity or raw coverage", w.Body)
	}
	return result
}

func (f httpInboxFixture) seedTrace(t *testing.T) []string {
	t.Helper()
	f.seed(t, "trace", "completed", true)
	f.exec(t, `DELETE FROM timeline_items`)
	f.exec(t, `UPDATE runs SET source='x',status='completed',stage='completed',coverage_json='{"private":"PRIVATE_COVERAGE"}' WHERE id='trace-run'`)
	now := domain.Now()
	keys := []string{}
	blocks := []domain.Block{}
	for i := 1; i <= 5; i++ {
		key := fmt.Sprintf("x:00000000000000000000070%d", i)
		keys = append(keys, key)
		blocks = append(blocks, domain.Block{EvidenceKey: key, Author: fmt.Sprintf("Author %d", i), Text: fmt.Sprintf("Captured candidate %d", i), Permalink: fmt.Sprintf("https://x.com/example/status/70%d", i)})
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: now, Coverage: map[string]any{"private": "PRIVATE_COVERAGE"}, Snapshots: []domain.Snapshot{{Blocks: blocks}, {Blocks: []domain.Block{blocks[3]}}}}
	f.exec(t, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('trace-command','trace-run','collect_visible','completed','{}',?)`, now)
	f.exec(t, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('trace-observation','trace-run','trace-command','x',?,?,?)`, inboxReadJSON(t, observation), now, now)
	for i := 1; i <= 3; i++ {
		assessment := domain.CandidateAssessment{EvidenceKey: keys[i], Rationale: fmt.Sprintf("Assessment rationale %d", i+1)}
		item := domain.ReasonedItem{EvidenceKey: keys[i], Source: domain.SourceX, Author: blocks[i].Author, WhatChanged: blocks[i].Text, SourceURL: blocks[i].Permalink}
		selected := 0
		if i > 1 {
			selected = 1
		}
		f.exec(t, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at) VALUES('trace-run',?,'x',?,?,0.5,0,0.5,?,?)`, keys[i], inboxReadJSON(t, assessment), inboxReadJSON(t, item), selected, now)
		if selected == 1 {
			f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,'trace','trace-run','x',?,?,?,?,'{}',?)`, fmt.Sprintf("trace-item-%d", i), keys[i], i, inboxReadJSON(t, item), inboxReadJSON(t, assessment), now)
		}
	}
	f.exec(t, `INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('trace-event','A specific event',?,?)`, now, now)
	for i, relation := range []string{"new_event", "duplicate_report"} {
		f.exec(t, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES(?,'trace-event',?,'trace','trace-run',?,'x',?,0.95,'Specific event relationship',?)`, fmt.Sprintf("trace-report-%d", i), fmt.Sprintf("trace-item-%d", i+2), keys[i+2], relation, now)
	}
	f.exec(t, `INSERT INTO content_continuity_occurrences(run_id,evidence_key,status,action,observed_at,reason) VALUES('trace-run',?,'resurfaced_unchanged','fail_fast',?,'Unchanged retained post')`, keys[4], now)
	return keys
}

func TestHTTPInboxCharacterizationListPagination(t *testing.T) {
	f := newHTTPInboxFixture(t)
	empty := f.list(t, f.serve(sessionUpdatesRequest("GET", "/api/inbox", "")))
	if empty.Total != 0 || empty.Limit != 12 || empty.Offset != 0 || len(empty.Sessions) != 0 {
		t.Fatalf("empty Inbox=%+v", empty)
	}
	for i := 0; i < 30; i++ {
		id := fmt.Sprintf("session-%02d", i)
		f.seed(t, id, "completed", false)
		f.exec(t, `UPDATE sessions SET created_at=? WHERE id=?`, fmt.Sprintf("2026-10-%02dT00:00:00Z", i+1), id)
	}
	for _, tc := range []struct {
		name, query   string
		limit, offset int
	}{
		{"defaults", "", 12, 0}, {"explicit", "?limit=5&offset=7", 5, 7}, {"lower", "?limit=0&offset=-3", 1, 0},
		{"upper", "?limit=999&offset=999999", 25, 100000}, {"max_page", "?limit=25", 25, 0},
		{"invalid", "?limit=bad&offset=bad", 12, 0}, {"overflow", "?limit=999999999999999999999999&offset=999999999999999999999999", 12, 0},
		{"numeric_whitespace", "?limit=%205%20&offset=%202%20", 12, 0}, {"signed", "?limit=%2B3&offset=%2B2", 3, 2},
		{"duplicate_first", "?limit=2&limit=8&offset=1&offset=7", 2, 1}, {"past_end", "?limit=4&offset=30", 4, 30},
		{"unknown_ignored", "?extra=ignored&limit=3", 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := f.snapshot(t)
			got := f.list(t, f.serve(sessionUpdatesRequest("GET", "/api/inbox"+tc.query, "")))
			wantCount := min(tc.limit, max(30-tc.offset, 0))
			if got.Total != 30 || got.Limit != tc.limit || got.Offset != tc.offset || len(got.Sessions) != wantCount {
				t.Fatalf("pagination=%+v", got)
			}
			for i, session := range got.Sessions {
				if session.ID != fmt.Sprintf("session-%02d", 29-tc.offset-i) || session.Runs == nil || session.PreferenceDecisions == nil {
					t.Fatalf("session projection/order=%+v", session)
				}
			}
			if !reflect.DeepEqual(before, f.snapshot(t)) {
				t.Fatal("Inbox read changed durable state")
			}
		})
	}
}

func TestHTTPInboxCharacterizationTraceStagesAndProjection(t *testing.T) {
	f := newHTTPInboxFixture(t)
	f.seedTrace(t)
	wantCounts := domain.InboxFlowCounts{Captured: 5, Skipped: 1, Evaluated: 3, Selected: 2, Added: 1}
	for _, tc := range []struct {
		query, stage         string
		total, limit, offset int
		outcomes             []string
	}{
		{"", "captured", 5, 10, 0, []string{"captured_only", "not_selected", "added", "collapsed_duplicate", "resurfaced_unchanged"}},
		{"?stage=%20EvALuaTed%20", "evaluated", 3, 10, 0, []string{"not_selected", "added", "collapsed_duplicate"}},
		{"?stage=skipped", "skipped", 1, 10, 0, []string{"resurfaced_unchanged"}},
		{"?stage=selected", "selected", 2, 10, 0, []string{"added", "collapsed_duplicate"}},
		{"?stage=added", "added", 1, 10, 0, []string{"added"}},
		{"?stage=%20%20", "captured", 5, 10, 0, []string{"captured_only", "not_selected", "added", "collapsed_duplicate", "resurfaced_unchanged"}},
		{"?stage=selected&limit=1&offset=1", "selected", 2, 1, 1, []string{"collapsed_duplicate"}},
		{"?stage=captured&limit=0&offset=-1", "captured", 5, 1, 0, []string{"captured_only"}},
		{"?limit=999&offset=999999", "captured", 5, 20, 100000, []string{}},
		{"?limit=bad&offset=bad", "captured", 5, 10, 0, []string{"captured_only", "not_selected", "added", "collapsed_duplicate", "resurfaced_unchanged"}},
		{"?limit=999999999999999999999999&offset=999999999999999999999999", "captured", 5, 10, 0, []string{"captured_only", "not_selected", "added", "collapsed_duplicate", "resurfaced_unchanged"}},
		{"?stage=added&stage=raw&limit=1&limit=2", "added", 1, 1, 0, []string{"added"}},
	} {
		t.Run(tc.query, func(t *testing.T) {
			before := f.snapshot(t)
			trace := f.trace(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath+tc.query, "")))
			outcomes := []string{}
			for _, item := range trace.Items {
				outcomes = append(outcomes, item.Outcome)
			}
			if trace.RunID != "trace-run" || trace.Source != domain.SourceX || trace.Stage != tc.stage || trace.Counts != wantCounts || trace.Total != tc.total || trace.Limit != tc.limit || trace.Offset != tc.offset || !reflect.DeepEqual(outcomes, tc.outcomes) {
				t.Fatalf("trace=%+v outcomes=%v", trace, outcomes)
			}
			if !reflect.DeepEqual(before, f.snapshot(t)) {
				t.Fatal("trace read changed durable state")
			}
		})
	}
	list := f.list(t, f.serve(sessionUpdatesRequest("GET", "/api/inbox", "")))
	if list.Total != 1 || len(list.Sessions) != 1 || len(list.Sessions[0].Runs) != 1 {
		t.Fatalf("list=%+v", list)
	}
	session := list.Sessions[0]
	run := session.Runs[0]
	if session.CapturedCandidates != 5 || session.EvaluatedCandidates != 3 || session.SelectedCandidates != 2 || session.AddedItems != 1 || run.AddedItems != 1 || run.AcquisitionRounds != 1 {
		t.Fatalf("Inbox counts/projection=%+v", session)
	}
	full := f.serve(sessionUpdatesRequest("GET", "/api/inbox", ""))
	if strings.Contains(full.Body.String(), "PRIVATE_COVERAGE") {
		t.Fatal("Inbox exposed raw coverage", full.Body)
	}
	f.exec(t, `UPDATE sessions SET status='running' WHERE id='trace'`)
	f.list(t, f.serve(sessionUpdatesRequest("GET", "/api/inbox", "")))
	f.trace(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath, "")))
}

func TestHTTPInboxCharacterizationCorrectionProjection(t *testing.T) {
	f := newHTTPInboxFixture(t)
	f.seedTrace(t)
	trace := f.trace(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath+"?stage=evaluated", "")))
	ref := trace.Items[0].CandidateRef
	if !strings.HasPrefix(ref, "candidate_") || trace.Items[0].Correction != nil {
		t.Fatal("unselected candidate projection changed", trace)
	}
	correction, _, err := f.s.store.CreateSelectionCorrection(context.Background(), "trace-run", ref)
	if err != nil {
		t.Fatal(err)
	}
	before := f.snapshot(t)
	selected := f.trace(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath+"?stage=selected", "")))
	if selected.Total != 3 || selected.Counts.Selected != 3 || selected.Counts.Added != 2 || selected.Items[0].Outcome != "user_selected" || selected.Items[0].CandidateRef != ref || selected.Items[0].Correction == nil || selected.Items[0].Correction.ID != correction.ID {
		t.Fatalf("corrected trace=%+v", selected)
	}
	if !reflect.DeepEqual(before, f.snapshot(t)) {
		t.Fatal("correction projection mutated state")
	}
	if _, err := f.s.store.UndoSelectionCorrection(context.Background(), correction.ID); err != nil {
		t.Fatal(err)
	}
	trace = f.trace(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath+"?stage=evaluated", "")))
	if trace.Items[0].CandidateRef != ref || trace.Items[0].Correction != nil || trace.Items[0].Selected {
		t.Fatal("undone correction projection changed", trace)
	}
}

func TestHTTPInboxCharacterizationRouting(t *testing.T) {
	for _, action := range []string{"list", "trace"} {
		for _, mode := range []string{"canonical", "slash", "encoded_slash", "double_slash", "nested_id", "wrong_method", "foreign_host", "foreign_origin", "preflight"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newHTTPInboxFixture(t)
				f.seedTrace(t)
				target := "/api/inbox"
				if action == "trace" {
					target = inboxTracePath
				}
				status := 200
				switch mode {
				case "slash":
					target += "/"
				case "encoded_slash":
					target += "%2F"
				case "double_slash":
					target += "//"
				case "nested_id":
					if action == "trace" {
						target = "/api/inbox/runs/nested/trace-run/trace"
					} else {
						target = "/api/inbox/nested"
					}
					status = 404
				}
				r := sessionUpdatesRequest("GET", target, "")
				if mode == "double_slash" {
					// Path-cleaning redirect codes belong to the Go toolchain.
					// Keep the product router behind the standard mux behavior.
					reference := httptest.NewRecorder()
					mux := http.NewServeMux()
					mux.HandleFunc("/api/", func(http.ResponseWriter, *http.Request) {
						t.Fatal("unclean path reached reference handler")
					})
					mux.ServeHTTP(reference, r.Clone(r.Context()))
					status = reference.Code
				}
				switch mode {
				case "wrong_method":
					r.Method, status = "POST", 404
				case "foreign_host":
					r.Host, status = "example.com", 403
				case "foreign_origin":
					r.Header.Set("Origin", "https://example.com")
					status = 403
				case "preflight":
					r.Method, status = "OPTIONS", 204
				}
				before := f.snapshot(t)
				w := f.serve(r)
				if w.Code != status {
					t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body)
				}
				if status == 200 {
					if action == "list" {
						f.list(t, w)
					} else {
						f.trace(t, w)
					}
				}
				if status == 404 {
					f.error(t, w, 404, "not_found", "")
				}
				if (status == 403 || mode == "double_slash" || status == 204) && w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
					t.Fatal("rejection/redirect/preflight reached API")
				}
				if mode == "double_slash" && w.Header().Get("Location") != strings.TrimSuffix(target, "/") {
					t.Fatal("path-cleaning redirect target changed", w.Header())
				}
				if status == 204 && w.Body.Len() != 0 {
					t.Fatal("preflight body changed")
				}
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("HTTP read or rejection mutated state")
				}
			})
		}
	}
}

func TestHTTPInboxCharacterizationErrorPrecedence(t *testing.T) {
	for _, tc := range []struct {
		target  string
		status  int
		message string
	}{
		{inboxTracePath + "?stage=raw", 400, "stage must be"},
		{inboxTracePath + "?stage=all", 400, "stage must be"},
		{"/api/inbox/runs/missing/trace?stage=raw", 400, "stage must be"},
		{"/api/inbox/runs/missing/trace", 404, "run not found"},
		{"/api/inbox/runs/nested/missing/trace?stage=raw", 404, "run not found"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			f := newHTTPInboxFixture(t)
			f.seedTrace(t)
			before := f.snapshot(t)
			code := "not_found"
			if tc.status == 400 {
				code = "invalid_request"
			}
			f.error(t, f.serve(sessionUpdatesRequest("GET", tc.target, "")), tc.status, code, tc.message)
			if !reflect.DeepEqual(before, f.snapshot(t)) {
				t.Fatal("invalid trace request mutated state")
			}
		})
	}
	t.Run("empty_run_before_stage", func(t *testing.T) {
		f := newHTTPInboxFixture(t)
		w := httptest.NewRecorder()
		err := f.s.route(w, sessionUpdatesRequest("GET", "/api/inbox/runs//trace?stage=raw", ""))
		value, ok := err.(apiError)
		if !ok || value.Status != 404 || value.Message != "run not found" {
			t.Fatal("empty ID precedence changed", err)
		}
	})
	t.Run("empty_trace", func(t *testing.T) {
		f := newHTTPInboxFixture(t)
		f.seed(t, "trace", "completed", true)
		trace := f.trace(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath, "")))
		if trace.Total != 0 || len(trace.Items) != 0 || trace.Counts != (domain.InboxFlowCounts{}) {
			t.Fatal("empty trace changed", trace)
		}
	})
}

type inboxReadBody struct {
	io.Reader
	reads, closes int
}

func (b *inboxReadBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *inboxReadBody) Close() error               { b.closes++; return nil }

func TestHTTPInboxCharacterizationContextAndFailures(t *testing.T) {
	for _, target := range []string{"/api/inbox", inboxTracePath} {
		for _, mode := range []string{"ignored_body", "cancelled", "deadline", "closed_store", "corrupt_observation", "late_query_failure", "write_failure"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				f := newHTTPInboxFixture(t)
				f.seedTrace(t)
				r := sessionUpdatesRequest("GET", target, "")
				switch mode {
				case "cancelled":
					ctx, cancel := context.WithCancel(r.Context())
					cancel()
					r = r.WithContext(ctx)
				case "deadline":
					ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(-time.Second))
					defer cancel()
					r = r.WithContext(ctx)
				case "closed_store":
					if err := f.s.store.Close(); err != nil {
						t.Fatal(err)
					}
				case "corrupt_observation":
					f.exec(t, `UPDATE observations SET observation_json='{'`)
				case "late_query_failure":
					f.exec(t, `ALTER TABLE content_continuity_occurrences RENAME TO offline_continuity`)
				}
				before := f.snapshot(t)
				if mode == "ignored_body" {
					body := &inboxReadBody{Reader: strings.NewReader("{invalid")}
					r.Body = body
					r.Header.Set("Content-Type", "text/plain")
					w := f.serve(r)
					if target == "/api/inbox" {
						f.list(t, w)
					} else {
						f.trace(t, w)
					}
					if body.reads != 0 || body.closes != 0 {
						t.Fatal("GET body was consumed")
					}
				} else if mode == "write_failure" {
					failure := errors.New("offline-inbox-response-failed")
					w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
					if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != 200 {
						t.Fatal("response error changed", w.status, err)
					}
				} else {
					f.error(t, f.serve(r), 500, "internal_error", "internal server error")
				}
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("failed/ignored-body read mutated state")
				}
			})
		}
	}
	t.Run("validation_before_cancelled_lookup", func(t *testing.T) {
		f := newHTTPInboxFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		f.error(t, f.serve(sessionUpdatesRequest("GET", inboxTracePath+"?stage=raw", "").WithContext(ctx)), 400, "invalid_request", "stage must be")
	})
}
