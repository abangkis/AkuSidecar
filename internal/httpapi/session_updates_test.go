package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type sessionUpdatesFixture struct {
	s  *Server
	db *sql.DB
}

func newSessionUpdatesFixture(t *testing.T) sessionUpdatesFixture {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	root := t.TempDir()
	state, err := store.Open(filepath.Join(root, "sessions.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Root: root, Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Capture: config.CaptureConfig{Profile: "expanded", Visibility: "quiet", OpenMissingSource: true, MaxAcquisitionRounds: 1}, Preference: config.PreferenceConfig{Mode: "promote_unused_budget"}}
	logger := log.New(io.Discard, "", 0)
	e := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	e.RecordHeartbeat(engine.ExpectedHeartbeat())
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
			t.Error("session fixture workers did not drain")
		}
	})
	return sessionUpdatesFixture{s, db}
}

func (f sessionUpdatesFixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.db.Exec(query, args...); err != nil {
		t.Fatal(err)
	}
}
func (f sessionUpdatesFixture) count(t *testing.T, query string) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func (f sessionUpdatesFixture) ready(t *testing.T) {
	t.Helper()
	if _, err := f.s.store.CompleteOnboarding(context.Background(), []domain.Source{domain.SourceLinkedIn}); err != nil {
		t.Fatal(err)
	}
}
func (f sessionUpdatesFixture) seed(t *testing.T, id, status string, items bool) {
	t.Helper()
	f.exec(t, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at,coverage_json) VALUES(?,'fixture',?,2,2,?,'{"pipelineStage":"capture","pipelineStageUpdatedAt":"fixture","largeEvidence":["hidden"]}')`, id, status, domain.Now())
	if !items {
		return
	}
	f.exec(t, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at,coverage_json) VALUES(?,?,'linkedin',0,'waiting_for_bridge','capture',?,'{"largeCaptureTelemetry":["hidden"]}')`, id+"-run", id, domain.Now())
	f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES(?,?,?,'linkedin','http-session-evidence',0,'{}','{}',?)`, id+"-item", id, id+"-run", domain.Now())
}
func sessionUpdatesRequest(method, target, body string) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:11122"+target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}
func (f sessionUpdatesFixture) serve(r *http.Request, security bool) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var handler http.Handler = f.s.api()
	if security {
		handler = f.s.security(handler)
	}
	handler.ServeHTTP(w, r)
	return w
}
func (f sessionUpdatesFixture) json(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]json.RawMessage {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" || w.Header().Get("Cache-Control") != "no-store, max-age=0" || w.Header().Get("Pragma") != "no-cache" || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != f.s.engine.Epoch() {
		t.Fatalf("response headers changed: %v", w.Header())
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}
func (f sessionUpdatesFixture) error(t *testing.T, w *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	payload := f.json(t, w, status)
	var gotCode, gotMessage string
	_ = json.Unmarshal(payload["error"], &gotCode)
	_ = json.Unmarshal(payload["message"], &gotMessage)
	if len(payload) != 3 || gotCode != code || !strings.Contains(gotMessage, message) || string(payload["details"]) != "null" {
		t.Fatalf("error envelope changed: %s", w.Body)
	}
}

func TestSessionUpdatesCharacterizationProjectionAndRouting(t *testing.T) {
	f := newSessionUpdatesFixture(t)
	value := f.json(t, f.serve(sessionUpdatesRequest("GET", "/api/sessions/active", ""), false), 200)
	if len(value) != 1 || string(value["session"]) != "null" {
		t.Fatal("empty active-session envelope changed", value)
	}
	f.seed(t, "active", "completed", false)
	f.seed(t, "target", "queued", true)
	before, err := f.s.store.GetSession(context.Background(), "target")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/api/sessions/active", "/api/sessions/active/", "/api/sessions/target", "/api/sessions/nested/target/"} {
		t.Run(target, func(t *testing.T) {
			payload := f.json(t, f.serve(sessionUpdatesRequest("GET", target, ""), false), 200)
			var got domain.Session
			if err := json.Unmarshal(payload["session"], &got); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 1 || got.ID != "target" || got.ItemCount != 1 || got.Items != nil || len(got.Runs) != 1 || got.Runs[0].Coverage != nil || len(got.Coverage) != 2 || got.Coverage["pipelineStage"] != "capture" {
				t.Fatalf("progress projection changed: %+v", got)
			}
		})
	}
	after, err := f.s.store.GetSession(context.Background(), "target")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("polling mutated durable session", err)
	}
	for _, tc := range []struct{ method, target string }{
		{"GET", "/api/updates"}, {"PUT", "/api/updates"}, {"POST", "/api/updates//"},
		{"GET", "/api/sessions/target/cancel"}, {"PUT", "/api/sessions/target"}, {"POST", "/api/sessions/target"}, {"POST", "/api/sessions/target/cancel//"},
	} {
		t.Run(tc.method+tc.target, func(t *testing.T) {
			f.error(t, f.serve(sessionUpdatesRequest(tc.method, tc.target, "{}"), false), 404, "not_found", "route not found")
		})
	}
	f.error(t, f.serve(sessionUpdatesRequest("GET", "/api/sessions/missing", ""), false), 404, "not_found", "session not found")
	f.error(t, f.serve(sessionUpdatesRequest("POST", "/api/sessions/missing/cancel", ""), false), 500, "internal_error", "internal server error")
}

func TestSessionUpdatesCharacterizationStartJSON(t *testing.T) {
	for _, tc := range []struct {
		name, content, body string
		status              int
		code, message       string
	}{
		{"media", "text/plain", `{}`, 415, "unsupported_media_type", "Content-Type must be application/json"},
		{"missing-media", "", `{}`, 415, "unsupported_media_type", "Content-Type must be application/json"},
		{"empty", "application/json", "", 400, "invalid_request", "request body must be valid JSON"},
		{"malformed", "application/json", `{`, 400, "invalid_request", "request body must be valid JSON"},
		{"unknown", "application/json", `{"intent":"x","extra":true}`, 400, "invalid_request", "unknown field"},
		{"type", "application/json", `{"intent":1}`, 400, "invalid_request", "cannot unmarshal"},
		{"valid", "application/json", `{"intent":"x"}`, 409, "conflict", "complete onboarding"},
		{"charset", "application/json; charset=utf-8", `{}`, 409, "conflict", "complete onboarding"},
		{"null", "application/json", `null`, 409, "conflict", "complete onboarding"},
		// Preserve readJSON's existing single-decode contract; no new EOF validation.
		{"trailing-json", "application/json", `{"intent":"x"}{"extra":true}`, 409, "conflict", "complete onboarding"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			r := sessionUpdatesRequest("POST", "/api/updates", tc.body)
			r.Header.Set("Content-Type", tc.content)
			f.error(t, f.serve(r, false), tc.status, tc.code, tc.message)
			if f.count(t, `SELECT COUNT(*) FROM sessions`) != 0 {
				t.Fatal("rejected start persisted a session")
			}
		})
	}
}

func TestSessionUpdatesCharacterizationStartSuccess(t *testing.T) {
	f := newSessionUpdatesFixture(t)
	f.ready(t)
	// Stamp run coverage on the real Bridge-wait transition to distinguish raw start output.
	f.exec(t, `CREATE TRIGGER stamp_http_start AFTER UPDATE OF status ON runs WHEN NEW.status='waiting_for_bridge' BEGIN UPDATE runs SET coverage_json='{"rawStartCoverage":"retained"}' WHERE id=NEW.id; END`)
	payload := f.json(t, f.serve(sessionUpdatesRequest("POST", "/api/updates/", `{"intent":"  offline HTTP start  "}`), true), 201)
	var got domain.Session
	if err := json.Unmarshal(payload["session"], &got); err != nil {
		t.Fatal(err)
	}
	if len(payload) != 1 || got.ID == "" || got.Intent != "offline HTTP start" || len(got.Runs) != 1 || got.Runs[0].Coverage["rawStartCoverage"] != "retained" {
		t.Fatalf("raw start response changed: %+v", got)
	}
	stored, err := f.s.store.GetSession(context.Background(), got.ID)
	if err != nil || !reflect.DeepEqual(got, stored) {
		t.Fatal("start response differs from original Engine session", err)
	}
}

func TestSessionUpdatesCharacterizationCancel(t *testing.T) {
	for _, target := range []string{"/api/sessions/target/cancel", "/api/sessions/nested/target/cancel/"} {
		t.Run(target, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			f.seed(t, "target", "queued", true)
			for i := 0; i < 2; i++ {
				payload := f.json(t, f.serve(sessionUpdatesRequest("POST", target, "ignored body"), true), 200)
				var got domain.Session
				if err := json.Unmarshal(payload["session"], &got); err != nil {
					t.Fatal(err)
				}
				if got.Status != "cancelled" || got.CompletedAt == nil || got.ItemCount != 1 || got.Items != nil || got.Runs[0].Status != "cancelled" || got.Runs[0].Coverage != nil {
					t.Fatalf("cancel/reload/projection changed: %+v", got)
				}
			}
		})
	}
}

func TestSessionUpdatesCharacterizationSecurity(t *testing.T) {
	for _, endpoint := range []struct{ method, target string }{{"POST", "/api/updates"}, {"GET", "/api/sessions/active"}, {"GET", "/api/sessions/target"}, {"POST", "/api/sessions/target/cancel"}} {
		for _, guard := range []string{"host", "foreign-origin", "extension"} {
			t.Run(endpoint.method+endpoint.target+"/"+guard, func(t *testing.T) {
				f := newSessionUpdatesFixture(t)
				f.seed(t, "target", "queued", false)
				r := sessionUpdatesRequest(endpoint.method, endpoint.target, `{}`)
				switch guard {
				case "host":
					r.Host = "foreign.example"
				case "foreign-origin":
					r.Header.Set("Origin", "https://foreign.example")
				case "extension":
					r.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
				}
				if w := f.serve(r, true); w.Code != 403 || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
					t.Fatal("UI security boundary changed", w.Code, w.Body.String())
				}
				stored, err := f.s.store.GetSession(context.Background(), "target")
				if err != nil || stored.Status != "queued" || f.count(t, `SELECT COUNT(*) FROM sessions`) != 1 {
					t.Fatal("security rejection mutated session", err)
				}
			})
		}
	}
	f := newSessionUpdatesFixture(t)
	r := sessionUpdatesRequest("GET", "/api/sessions/active", "")
	r.Header.Set("Origin", "http://127.0.0.1:11122")
	f.json(t, f.serve(r, true), 200) // Local UI needs no Bridge token or contract header.
	w := f.serve(sessionUpdatesRequest("OPTIONS", "/api/updates", ""), true)
	if w.Code != 204 || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
		t.Fatal("OPTIONS early return changed", w.Code)
	}
}

type sessionUpdatesCancelBody struct {
	io.Reader
	cancel context.CancelFunc
}

func (b sessionUpdatesCancelBody) Close() error { b.cancel(); return nil }

func TestSessionUpdatesCharacterizationContextAndFailures(t *testing.T) {
	for _, endpoint := range []struct {
		method, target string
		status         int
	}{{"POST", "/api/updates", 409}, {"GET", "/api/sessions/active", 500}, {"GET", "/api/sessions/target", 500}, {"POST", "/api/sessions/target/cancel", 500}} {
		t.Run(endpoint.method+endpoint.target, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			f.seed(t, "target", "queued", false)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			code, message := "internal_error", "internal server error"
			if endpoint.status == 409 {
				code, message = "conflict", "context canceled"
			}
			f.error(t, f.serve(sessionUpdatesRequest(endpoint.method, endpoint.target, `{}`).WithContext(ctx), false), endpoint.status, code, message)
			stored, err := f.s.store.GetSession(context.Background(), "target")
			if err != nil || stored.Status != "queued" {
				t.Fatal("cancelled caller mutated session", err)
			}
		})
	}
	t.Run("cancel-after-decode", func(t *testing.T) {
		f := newSessionUpdatesFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := sessionUpdatesRequest("POST", "/api/updates", "").WithContext(ctx)
		r.Body = sessionUpdatesCancelBody{strings.NewReader(`{"intent":"fixture"}`), cancel}
		f.error(t, f.serve(r, false), 409, "conflict", "context canceled")
		if f.count(t, `SELECT COUNT(*) FROM sessions`) != 0 {
			t.Fatal("post-decode cancellation created session")
		}
	})
	for _, mode := range []string{"cancel-rollback", "cancel-read-after-commit", "start-error-after-commit", "closed-store"} {
		t.Run(mode, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			method, target, status, code, message := "POST", "/api/sessions/target/cancel", 500, "internal_error", "internal server error"
			if mode == "start-error-after-commit" {
				f.ready(t)
				target, status, code, message = "/api/updates", 409, "conflict", "unsupported update trigger"
			} else {
				f.seed(t, "target", "queued", false)
			}
			switch mode {
			case "cancel-rollback":
				f.exec(t, `CREATE TRIGGER cancel_fault BEFORE UPDATE OF status ON sessions WHEN NEW.status='cancelled' BEGIN SELECT RAISE(ABORT,'cancel-fault'); END`)
			case "cancel-read-after-commit":
				f.exec(t, `CREATE TRIGGER cancel_reload_fault AFTER UPDATE OF status ON sessions WHEN NEW.status='cancelled' BEGIN UPDATE sessions SET coverage_json='{"trigger":"read-after-cancel-fault"}' WHERE id=NEW.id; END`)
			case "start-error-after-commit":
				f.exec(t, `CREATE TRIGGER start_read_fault AFTER INSERT ON sessions BEGIN UPDATE sessions SET coverage_json='{"trigger":"read-after-start-fault"}' WHERE id=NEW.id; END`)
			case "closed-store":
				f.s.store.Close()
				method, target = "GET", "/api/sessions/active"
			}
			f.error(t, f.serve(sessionUpdatesRequest(method, target, `{"intent":"offline failure fixture"}`), false), status, code, message)
			if mode != "closed-store" {
				var storedStatus string
				if err := f.db.QueryRow(`SELECT status FROM sessions LIMIT 1`).Scan(&storedStatus); err != nil {
					t.Fatal(err)
				}
				if mode == "cancel-rollback" && storedStatus != "queued" {
					t.Fatal("failed cancellation committed")
				}
				if mode == "cancel-read-after-commit" && storedStatus != "cancelled" {
					t.Fatal("response error rolled back durable cancellation")
				}
				if f.count(t, `SELECT COUNT(*) FROM sessions`) != 1 {
					t.Fatal("post-commit error changed durable session count")
				}
			}
		})
	}
}

type sessionUpdatesFailWriter struct {
	header http.Header
	status int
	err    error
}

func (w *sessionUpdatesFailWriter) Header() http.Header       { return w.header }
func (w *sessionUpdatesFailWriter) WriteHeader(status int)    { w.status = status }
func (w *sessionUpdatesFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestSessionUpdatesCharacterizationResponseWriteFailure(t *testing.T) {
	for _, action := range []string{"start", "cancel"} {
		t.Run(action, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			target, wantStatus := "/api/updates", 201
			if action == "start" {
				f.ready(t)
			} else {
				f.seed(t, "target", "queued", false)
				target, wantStatus = "/api/sessions/target/cancel", 200
			}
			failure := errors.New("session-response-write-failed")
			w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
			if err := f.s.route(w, sessionUpdatesRequest("POST", target, `{"intent":"offline writer fixture"}`)); !errors.Is(err, failure) || w.status != wantStatus {
				t.Fatal("response write error contract changed", w.status, err)
			}
			if f.count(t, `SELECT COUNT(*) FROM sessions`) != 1 {
				t.Fatal("write failure lost durable session")
			}
			if action == "cancel" && f.count(t, `SELECT COUNT(*) FROM sessions WHERE status='cancelled'`) != 1 {
				t.Fatal("write failure undid cancellation")
			}
		})
	}
}
