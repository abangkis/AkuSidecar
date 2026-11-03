package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type httpTimelineClock struct{ nano atomic.Int64 }

func (c *httpTimelineClock) Now() time.Time { return time.Unix(0, c.nano.Load()).UTC() }

type httpTimelineFixture struct {
	sessionUpdatesFixture
	clock *httpTimelineClock
	now   time.Time
}

func newHTTPTimelineFixture(t *testing.T) httpTimelineFixture {
	t.Helper()
	now := time.Date(2026, 11, 3, 2, 0, 0, 123456789, time.UTC)
	clock := &httpTimelineClock{}
	clock.nano.Store(now.UnixNano())
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	settings.SemanticEventMode = "show_all"
	settings.AutoUpdateEnabled = false
	settings.AIDetectionEnabled = false
	settings.CalibrationEnabled = false
	settings.LoadProfile = "custom"
	settings.TimelineCapacity = 1
	root := t.TempDir()
	state, err := store.OpenWithClock(filepath.Join(root, "timeline.db"), settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Root: root, Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	logger := log.New(io.Discard, "", 0)
	e := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	s, err := New(cfg, state, e, logger)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", state.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t.Cleanup(func() {
		e.Shutdown()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("Timeline fixture workers did not drain")
		}
	})
	return httpTimelineFixture{sessionUpdatesFixture{s, db}, clock, now}
}

func (f httpTimelineFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f httpTimelineFixture) mode(t *testing.T, mode string) {
	t.Helper()
	settings, err := f.s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.SemanticEventMode = mode
	if err := f.s.store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
}

func (f httpTimelineFixture) visible(t *testing.T, count int) {
	t.Helper()
	f.seed(t, "visible", "completed", true)
	f.exec(t, `DELETE FROM timeline_items WHERE run_id='visible-run'`)
	at := f.now.Add(-24 * time.Hour).Format(time.RFC3339Nano)
	f.exec(t, `UPDATE sessions SET completed_at=? WHERE id='visible'`, at)
	f.exec(t, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id='visible-run'`, at)
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("linkedin:http-timeline-%02d", i)
		item := domain.ReasonedItem{EvidenceKey: key, Source: domain.SourceLinkedIn, Author: fmt.Sprintf("Author %d", i), WhatChanged: fmt.Sprintf("Visible update %d", i), SourceURL: fmt.Sprintf("https://www.linkedin.com/feed/update/urn:li:activity:%d/", 1234567890000000000+int64(i)), SourceURLKind: "native_post", Confidence: .7, EvidenceState: "primary"}
		raw, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,'visible','visible-run','linkedin',?,?,?,'{}','{"fixture":"retained"}',?)`, fmt.Sprintf("item-%02d", i), key, i, string(raw), at)
	}
}

func (f httpTimelineFixture) batch(t *testing.T, id, state string, expires time.Time) {
	t.Helper()
	f.seed(t, id, "completed", true)
	at := f.now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
	f.exec(t, `UPDATE sessions SET completed_at=? WHERE id=?`, at, id)
	f.exec(t, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id=?`, at, id+"-run")
	f.exec(t, `INSERT INTO auto_update_batches(session_id,state,created_at,prepared_at,expires_at) VALUES(?,?,?,?,?)`, id, state, at, at, expires.Format(time.RFC3339Nano))
}

func (f httpTimelineFixture) bookkeeping(t *testing.T) []string {
	t.Helper()
	result := []string{}
	for _, query := range []string{`SELECT json_array(id,status,stage) FROM runs ORDER BY id`, `SELECT json_array(key,value_json) FROM settings ORDER BY key`, `SELECT json_array(key,value) FROM meta WHERE key<>'auto_update_queue_vacancy_at' ORDER BY key`} {
		rows, err := f.db.Query(query)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			result = append(result, raw)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"sessions", "runs", "bridge_commands", "observations", "calibration_sessions"} {
		result = append(result, fmt.Sprintf("%s=%d", table, f.count(t, "SELECT COUNT(*) FROM "+table)))
	}
	return result
}

type httpTimelineResponse struct {
	Items   []domain.TimelineItem         `json:"items"`
	Batches []domain.TimelineBatchSummary `json:"timelineBatches"`
	Latest  *domain.TimelineCheckSummary  `json:"latestCheck"`
	Auto    domain.AutoUpdateStatus       `json:"autoUpdate"`
}

func (f httpTimelineFixture) response(t *testing.T, w *httptest.ResponseRecorder) httpTimelineResponse {
	t.Helper()
	payload := f.json(t, w, 200)
	var result httpTimelineResponse
	if len(payload) != 4 || payload["items"] == nil || payload["timelineBatches"] == nil || payload["latestCheck"] == nil || payload["autoUpdate"] == nil || json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Batches == nil {
		t.Fatal("Timeline envelope changed", w.Body)
	}
	return result
}

func (f httpTimelineFixture) effects(t *testing.T, id, state, vacancy string) {
	t.Helper()
	var got string
	if err := f.db.QueryRow(`SELECT state FROM auto_update_batches WHERE session_id=?`, id).Scan(&got); err != nil || got != state {
		t.Fatalf("batch=%s want=%s err=%v", got, state, err)
	}
	var at string
	err := f.db.QueryRow(`SELECT value FROM meta WHERE key='auto_update_queue_vacancy_at'`).Scan(&at)
	if vacancy == "" {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatal("unexpected queue vacancy", at, err)
		}
	} else if err != nil || at != vacancy {
		t.Fatal("queue vacancy changed", at, vacancy, err)
	}
}

func TestHTTPTimelineCharacterizationEmptyAndModes(t *testing.T) {
	for _, mode := range []string{"show_all", "collapse", "hide"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			f.mode(t, mode)
			before := f.bookkeeping(t)
			w := f.serve(sessionUpdatesRequest("GET", "/api/timeline", ""))
			response := f.response(t, w)
			payload := f.json(t, w, 200)
			want := "[]"
			if mode == "show_all" {
				want = "null"
			}
			if string(payload["items"]) != want || string(payload["timelineBatches"]) != "[]" || string(payload["latestCheck"]) != "null" || len(response.Items) != 0 || response.Latest != nil || response.Auto.Enabled || response.Auto.State != "disabled" {
				t.Fatal("empty mode projection changed", mode, w.Body)
			}
			if !reflect.DeepEqual(before, f.bookkeeping(t)) {
				t.Fatal("empty Timeline read changed bookkeeping")
			}
		})
	}
}

func TestHTTPTimelineCharacterizationPaginationAndScope(t *testing.T) {
	f := newHTTPTimelineFixture(t)
	f.visible(t, 60)
	for _, tc := range []struct {
		name, query   string
		limit, offset int
	}{
		{"defaults", "", 24, 0}, {"page", "?limit=3&offset=5", 3, 5}, {"lower", "?limit=0&offset=-1", 1, 0}, {"upper_page", "?limit=999", 50, 0},
		{"offset_clamp", "?limit=3&offset=999999", 3, 100000}, {"invalid", "?limit=bad&offset=bad", 24, 0}, {"overflow", "?limit=999999999999999999999999&offset=999999999999999999999999", 24, 0},
		{"whitespace", "?limit=%203%20&offset=%202%20", 24, 0}, {"signed", "?limit=%2B3&offset=%2B2", 3, 2}, {"duplicate_first", "?limit=2&limit=8&offset=1&offset=6", 2, 1}, {"past_end", "?limit=4&offset=60", 4, 60}, {"unknown_ignored", "?extra=ignored&limit=2", 2, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := f.bookkeeping(t)
			response := f.response(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline"+tc.query, "")))
			wantCount := min(tc.limit, max(60-tc.offset, 0))
			if len(response.Items) != wantCount {
				t.Fatalf("page len=%d want=%d", len(response.Items), wantCount)
			}
			for i, item := range response.Items {
				if item.ID != fmt.Sprintf("item-%02d", i+tc.offset) || !item.OriginRunAvailable || !item.OriginSessionAvailable || item.Coverage["fixture"] != "retained" {
					t.Fatal("item projection/order changed", item)
				}
			}
			if response.Latest == nil || response.Latest.SessionID != "visible" || response.Latest.AddedItems != 60 || len(response.Batches) != 1 || response.Batches[0].SessionID != "visible" || response.Batches[0].ItemCount != 60 {
				t.Fatalf("summary scope was bounded by page: %+v", response)
			}
			if !reflect.DeepEqual(before, f.bookkeeping(t)) {
				t.Fatal("pagination changed bookkeeping")
			}
		})
	}
	// A newer terminal check with no items is eligible for latestCheck, not batches.
	f.seed(t, "empty-check", "failed", false)
	f.exec(t, `UPDATE sessions SET completed_at=? WHERE id='empty-check'`, f.now.Format(time.RFC3339Nano))
	response := f.response(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline?limit=1", "")))
	if response.Latest == nil || response.Latest.SessionID != "empty-check" || response.Latest.AddedItems != 0 || len(response.Batches) != 1 || response.Batches[0].SessionID != "visible" {
		t.Fatal("terminal empty check/batch eligibility changed", response)
	}
}

func TestHTTPTimelineCharacterizationSemanticAndMemoryProjection(t *testing.T) {
	t.Run("semantic_modes", func(t *testing.T) {
		f := newHTTPTimelineFixture(t)
		f.visible(t, 4)
		f.exec(t, `INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('timeline-event','Fixture event',?,?)`, domain.Now(), domain.Now())
		for i := 0; i < 4; i++ {
			relation := "new_event"
			if i%2 == 1 {
				relation = "duplicate_report"
			}
			f.exec(t, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES(?,'timeline-event',?,'visible','visible-run',?,'linkedin',?,0.9,'Fixture relation',?)`, fmt.Sprintf("timeline-report-%d", i), fmt.Sprintf("item-%02d", i), fmt.Sprintf("linkedin:http-timeline-%02d", i), relation, domain.Now())
		}
		for _, tc := range []struct {
			mode, query       string
			ids               []string
			added, duplicates int
		}{
			{"show_all", "?limit=1", []string{"item-00"}, 4, 0}, {"collapse", "?limit=1", []string{"item-00", "item-01"}, 2, 2}, {"collapse", "?limit=1&offset=1", []string{"item-01", "item-02", "item-03"}, 2, 2}, {"hide", "?limit=1&offset=1", []string{"item-02"}, 2, 2},
		} {
			t.Run(tc.mode+tc.query, func(t *testing.T) {
				f.mode(t, tc.mode)
				response := f.response(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline"+tc.query, "")))
				ids := []string{}
				for _, item := range response.Items {
					ids = append(ids, item.ID)
				}
				if !reflect.DeepEqual(ids, tc.ids) || response.Latest == nil || response.Latest.AddedItems != tc.added || response.Latest.DuplicateReports != tc.duplicates || len(response.Batches) != 1 || response.Batches[0].ItemCount != 4 {
					t.Fatal("semantic page/summary contract changed", ids, response)
				}
			})
		}
	})
	t.Run("personal_memory", func(t *testing.T) {
		f := newHTTPTimelineFixture(t)
		fixture := createTimelineMemoryHTTPFixture(t, f.s.store, "completed", "Persisted source evidence for the Timeline memory projection.", "1234567890123456789")
		if _, _, err := f.s.store.KeepTimelineFullCopy(context.Background(), fixture.TimelineID); err != nil {
			t.Fatal(err)
		}
		before := f.bookkeeping(t)
		w := f.serve(sessionUpdatesRequest("GET", "/api/timeline", ""))
		response := f.response(t, w)
		var payload struct {
			Items []map[string]json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if len(response.Items) != 1 || response.Items[0].PersonalMemory == nil {
			t.Fatal("missing personal memory projection", w.Body)
		}
		var memory map[string]json.RawMessage
		if json.Unmarshal(payload.Items[0]["personalMemory"], &memory) != nil || len(memory) != 3 || memory["retentionTier"] == nil || memory["saved"] == nil || memory["permanentKeep"] == nil {
			t.Fatal("personal memory projection exposed internal data", w.Body)
		}
		if !reflect.DeepEqual(before, f.bookkeeping(t)) {
			t.Fatal("memory projection mutated state")
		}
	})
}

func TestHTTPTimelineCharacterizationExpiryAndVisibility(t *testing.T) {
	f := newHTTPTimelineFixture(t)
	f.visible(t, 2)
	f.batch(t, "expired", "prepared", f.now.Add(-time.Nanosecond))
	f.batch(t, "at-boundary", "prepared", f.now)
	f.batch(t, "future", "prepared", f.now.Add(2*time.Hour))
	before := f.bookkeeping(t)
	response := f.response(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline", "")))
	if len(response.Items) != 2 || response.Latest == nil || response.Latest.SessionID != "visible" || len(response.Batches) != 1 || response.Batches[0].SessionID != "visible" || len(response.Auto.PreparedBatches) != 1 || response.Auto.PreparedBatches[0].SessionID != "future" || response.Auto.Enabled || response.Auto.State != "disabled" {
		t.Fatal("hidden batch/expiry/off status changed", response)
	}
	at := f.now.Format(time.RFC3339Nano)
	f.effects(t, "expired", "expired", at)
	f.effects(t, "at-boundary", "expired", at)
	f.effects(t, "future", "prepared", at)
	f.clock.nano.Add(int64(time.Minute))
	f.response(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline", "")))
	f.effects(t, "expired", "expired", at)
	if !reflect.DeepEqual(before, f.bookkeeping(t)) {
		t.Fatal("expiry changed unrelated bookkeeping")
	}
}

func TestHTTPTimelineCharacterizationFailureOrder(t *testing.T) {
	for _, mode := range []string{"timeline", "latest_check", "before_expiry", "expiry_update", "queue_vacancy", "after_expiry_budget"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			f.visible(t, 1)
			f.batch(t, "expired", "prepared", f.now.Add(-time.Hour))
			message := ""
			wantState, wantVacancy := "prepared", ""
			switch mode {
			case "timeline":
				f.exec(t, `DROP TABLE timeline_items`)
				message = "timeline_items"
			case "latest_check":
				f.seed(t, "bad-latest", "failed", true)
				f.exec(t, `DELETE FROM timeline_items WHERE run_id='bad-latest-run'`)
				f.exec(t, `UPDATE sessions SET completed_at=? WHERE id='bad-latest'`, f.now.Format(time.RFC3339Nano))
				f.exec(t, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('timeline-bad-command','bad-latest-run','collect_visible','completed','{}',?)`, domain.Now())
				f.exec(t, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('timeline-bad-observation','bad-latest-run','timeline-bad-command','linkedin','{',?,?)`, domain.Now(), domain.Now())
				message = "unexpected end"
			case "before_expiry":
				f.exec(t, `INSERT INTO meta(key,value) VALUES('source_capture_pause:x','{')`)
				message = "unexpected end"
			case "expiry_update":
				f.exec(t, `CREATE TRIGGER timeline_fault BEFORE UPDATE OF state ON auto_update_batches WHEN NEW.state='expired' BEGIN SELECT RAISE(ABORT,'offline-expiry-fault'); END`)
				message = "offline-expiry-fault"
			case "queue_vacancy":
				f.exec(t, `CREATE TRIGGER timeline_fault BEFORE INSERT ON meta WHEN NEW.key='auto_update_queue_vacancy_at' BEGIN SELECT RAISE(ABORT,'offline-vacancy-fault'); END`)
				message = "offline-vacancy-fault"
				wantState = "expired"
			case "after_expiry_budget":
				f.exec(t, `DROP TABLE reasoning_invocations`)
				message = "reasoning_invocations"
				wantState, wantVacancy = "expired", f.now.Format(time.RFC3339Nano)
			}
			before := f.bookkeeping(t)
			w := httptest.NewRecorder()
			err := f.s.route(w, sessionUpdatesRequest("GET", "/api/timeline", ""))
			if err == nil || !strings.Contains(err.Error(), message) || w.Body.Len() != 0 {
				t.Fatal("first original error or response order changed", err, w.Body)
			}
			f.effects(t, "expired", wantState, wantVacancy)
			if mode == "queue_vacancy" {
				// Expiry already committed. The next read has no new expiry,
				// so it does not retry the failed vacancy write.
				f.response(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline", "")))
			} else {
				f.error(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline", "")), 500, "internal_error", "internal server error")
			}
			f.effects(t, "expired", wantState, wantVacancy)
			if !reflect.DeepEqual(before, f.bookkeeping(t)) {
				t.Fatal("failed Timeline request changed unrelated bookkeeping")
			}
		})
	}
}

type httpTimelineBody struct {
	io.Reader
	reads, closes int
}

func (b *httpTimelineBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *httpTimelineBody) Close() error               { b.closes++; return nil }

func TestHTTPTimelineCharacterizationRoutingAndContext(t *testing.T) {
	for _, mode := range []string{"canonical", "slash", "encoded_slash", "double_slash", "wrong_method", "foreign_host", "foreign_origin", "preflight", "ignored_body", "cancelled", "deadline", "closed_store", "write_failure"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPTimelineFixture(t)
			f.visible(t, 1)
			f.batch(t, "expired", "prepared", f.now.Add(-time.Hour))
			target, status := "/api/timeline", 200
			switch mode {
			case "slash":
				target += "/"
			case "encoded_slash":
				target += "%2F"
			case "double_slash":
				target += "//"
			}
			r := sessionUpdatesRequest("GET", target, "")
			if mode == "double_slash" {
				reference := httptest.NewRecorder()
				mux := http.NewServeMux()
				mux.HandleFunc("/api/", func(http.ResponseWriter, *http.Request) { t.Fatal("unclean path reached reference handler") })
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
			case "cancelled":
				ctx, cancel := context.WithCancel(r.Context())
				cancel()
				r = r.WithContext(ctx)
				status = 500
			case "deadline":
				ctx, cancel := context.WithDeadline(r.Context(), time.Now().Add(-time.Second))
				defer cancel()
				r = r.WithContext(ctx)
				status = 500
			case "closed_store":
				if err := f.s.store.Close(); err != nil {
					t.Fatal(err)
				}
				status = 500
			}
			before := f.bookkeeping(t)
			if mode == "write_failure" {
				failure := errors.New("offline-timeline-response-failed")
				w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
				if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != 200 {
					t.Fatal("response write error changed", w.status, err)
				}
			} else {
				var body *httpTimelineBody
				if mode == "ignored_body" {
					body = &httpTimelineBody{Reader: strings.NewReader("{invalid")}
					r.Body = body
					r.Header.Set("Content-Type", "text/plain")
				}
				w := f.serve(r)
				if w.Code != status {
					t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body)
				}
				if status == 200 {
					f.response(t, w)
				}
				if status == 404 {
					f.error(t, w, 404, "not_found", "")
				}
				if status == 500 {
					f.error(t, w, 500, "internal_error", "internal server error")
				}
				if (mode == "double_slash" || status == 403 || status == 204) && w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
					t.Fatal("rejection/redirect/preflight reached API")
				}
				if mode == "double_slash" && w.Header().Get("Location") != strings.TrimSuffix(target, "/") {
					t.Fatal("redirect target changed", w.Header())
				}
				if status == 204 && w.Body.Len() != 0 {
					t.Fatal("preflight body changed")
				}
				if body != nil && (body.reads != 0 || body.closes != 0) {
					t.Fatal("GET body was consumed")
				}
			}
			wantState, wantVacancy := "prepared", ""
			if status == 200 {
				wantState, wantVacancy = "expired", f.now.Format(time.RFC3339Nano)
			}
			f.effects(t, "expired", wantState, wantVacancy)
			if !reflect.DeepEqual(before, f.bookkeeping(t)) {
				t.Fatal("request changed unrelated bookkeeping")
			}
		})
	}
}
