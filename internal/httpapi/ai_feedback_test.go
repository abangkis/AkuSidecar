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
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const httpAIFeedbackPath = "/api/timeline/ai-item/ai-feedback"
const httpAIFeedbackAI = `{"verdict":"ai","targetType":"post","signalScope":"social_post"}`
const httpAIFeedbackUnsure = `{"verdict":"unsure","targetType":"post","signalScope":"social_post"}`

type httpAIFeedbackFixture struct{ sessionUpdatesFixture }

type httpAIFeedbackBody struct {
	io.Reader
	reads, closes int
}

func (b *httpAIFeedbackBody) Read(p []byte) (int, error) {
	b.reads++
	return b.Reader.Read(p)
}
func (b *httpAIFeedbackBody) Close() error { b.closes++; return nil }

func newHTTPAIFeedbackFixture(t *testing.T) httpAIFeedbackFixture {
	t.Helper()
	f := httpAIFeedbackFixture{newSessionUpdatesFixture(t)}
	f.seed(t, "ai", "completed", true)
	f.exec(t, `UPDATE runs SET status='completed',stage='completed' WHERE id='ai-run'`)
	f.exec(t, `UPDATE timeline_items SET evidence_key='feedback-evidence',item_json='{"source":"linkedin","evidenceKey":"feedback-evidence","author":"Item Author","whatChanged":"Offline feedback fixture"}',evidence_snapshot_json='{"author":"  Captured Author  ","text":"PRIVATE_CAPTURE_TEXT"}',coverage_json='{"diagnostic":"PRIVATE_COVERAGE"}' WHERE id='ai-item'`)
	f.enabled(t, false)
	f.s.engine.SetAIDeepResolver(nil)
	return f
}

func (f httpAIFeedbackFixture) enabled(t *testing.T, enabled bool) {
	t.Helper()
	settings, err := f.s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled = enabled
	if err := f.s.store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
}

func (f httpAIFeedbackFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func httpAIFeedbackUndo(id string) string { return "/api/ai-feedback/" + id + "/undo" }

func (f httpAIFeedbackFixture) event(t *testing.T, w *httptest.ResponseRecorder, status int) domain.AIFeedbackEvent {
	t.Helper()
	payload := f.json(t, w, status)
	var event domain.AIFeedbackEvent
	if len(payload) != 1 || json.Unmarshal(payload["feedback"], &event) != nil {
		t.Fatal("feedback envelope changed", w.Body)
	}
	f.eventFields(t, payload["feedback"], event)
	return event
}

func (f httpAIFeedbackFixture) eventFields(t *testing.T, raw json.RawMessage, event domain.AIFeedbackEvent) {
	t.Helper()
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("invalid feedback event", string(raw))
	}
	count := 9
	if event.Reason != "" {
		count++
	}
	if event.SupersedesID != "" {
		count++
	}
	if len(fields) != count || event.ID == "" || event.TimelineID != "ai-item" || event.SessionID != "ai" || event.Source != domain.SourceLinkedIn {
		t.Fatal("feedback projection changed", string(raw))
	}
	for _, name := range strings.Fields("id timelineId sessionId source targetType targetKey verdict signalScope createdAt") {
		if _, ok := fields[name]; !ok {
			t.Fatal("feedback field missing", name)
		}
	}
	if _, ok := fields["reason"]; ok != (event.Reason != "") {
		t.Fatal("reason omission changed")
	}
	if _, ok := fields["supersedesId"]; ok != (event.SupersedesID != "") {
		t.Fatal("supersedes omission changed")
	}
	if _, err := time.Parse(time.RFC3339Nano, event.CreatedAt); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"PRIVATE_CAPTURE_TEXT", "PRIVATE_COVERAGE"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("raw evidence exposed", private)
		}
	}
}

func (f httpAIFeedbackFixture) history(t *testing.T, target string) []domain.AIFeedbackEvent {
	t.Helper()
	w := f.serve(sessionUpdatesRequest("GET", target, ""))
	payload := f.json(t, w, 200)
	var values []json.RawMessage
	if len(payload) != 1 || json.Unmarshal(payload["feedback"], &values) != nil {
		t.Fatal("history envelope changed", w.Body)
	}
	var result []domain.AIFeedbackEvent
	for _, raw := range values {
		var value domain.AIFeedbackEvent
		if json.Unmarshal(raw, &value) != nil {
			t.Fatal("history event changed")
		}
		f.eventFields(t, raw, value)
		result = append(result, value)
	}
	return result
}

func TestHTTPAIFeedbackCharacterizationInputs(t *testing.T) {
	for _, target := range []struct{ kind, scope, key string }{
		{"post", "social_post", "post|linkedin|feedback-evidence"},
		{"media", "attached_media", "media|linkedin|feedback-evidence"},
		{"quote", "quoted_post", "quote|linkedin|feedback-evidence"},
		{"account", "author_account", "account|linkedin|captured author"},
	} {
		for _, verdict := range []string{"ai", "not_ai", "unsure"} {
			t.Run(target.kind+"/"+verdict, func(t *testing.T) {
				f := newHTTPAIFeedbackFixture(t)
				body := `{"verdict":"` + verdict + `","targetType":"` + target.kind + `","signalScope":"` + target.scope + `","reason":"insufficient_evidence"}`
				value := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, body)), 201)
				if value.Verdict != verdict || value.TargetType != target.kind || value.SignalScope != target.scope || value.TargetKey != target.key || value.Reason != "insufficient_evidence" || value.SupersedesID != "" {
					t.Fatal("feedback input/target changed", value)
				}
				if f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs`) != 0 || !reflect.DeepEqual(f.history(t, httpAIFeedbackPath), []domain.AIFeedbackEvent{value}) {
					t.Fatal("disabled review or history changed")
				}
			})
		}
	}
	for _, body := range []string{httpAIFeedbackAI + `{}`, `{"verdict":"invalid","verdict":"ai","targetType":"post","signalScope":"social_post"}`} {
		t.Run("existing_JSON_behavior", func(t *testing.T) {
			f := newHTTPAIFeedbackFixture(t)
			if value := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, body)), 201); value.Verdict != "ai" {
				t.Fatal(value)
			}
		})
	}
}

func TestHTTPAIFeedbackCharacterizationHistoryAndClear(t *testing.T) {
	f := newHTTPAIFeedbackFixture(t)
	w := f.serve(sessionUpdatesRequest("GET", httpAIFeedbackPath, ""))
	if p := f.json(t, w, 200); len(p) != 1 || string(p["feedback"]) != "null" {
		t.Fatal("empty history changed", w.Body)
	}
	first := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, httpAIFeedbackAI)), 201)
	second := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, strings.Replace(httpAIFeedbackAI, `"ai"`, `"not_ai"`, 1))), 201)
	account := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, `{"verdict":"ai","targetType":"account","signalScope":"author_account"}`)), 201)
	if second.SupersedesID != first.ID || account.SupersedesID != "" {
		t.Fatal("supersedes changed")
	}
	f.error(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackUndo(first.ID), "")), 400, "invalid_request", "only the current AI feedback decision can be cleared")
	r := sessionUpdatesRequest("POST", httpAIFeedbackUndo(second.ID), "not JSON")
	r.Header.Del("Content-Type")
	body := &httpAIFeedbackBody{Reader: strings.NewReader("not JSON")}
	r.Body = body
	clear := f.event(t, f.serve(r), 200)
	if clear.ID == second.ID || clear.Verdict != "clear" || clear.Reason != "" || clear.SupersedesID != second.ID || clear.TargetKey != second.TargetKey || body.reads != 0 || body.closes != 0 {
		t.Fatal("clear event/body changed", clear)
	}
	f.error(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackUndo(second.ID), "")), 400, "invalid_request", "only the current AI feedback decision can be cleared")
	f.error(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackUndo(clear.ID), "")), 404, "not_found", "AI feedback not found")
	f.exec(t, `UPDATE ai_feedback_events SET created_at='2099-01-01T00:00:00Z' WHERE id=?`, first.ID)
	values := f.history(t, httpAIFeedbackPath+"?limit=1")
	var ids []string
	for _, v := range values {
		ids = append(ids, v.ID)
	}
	if !reflect.DeepEqual(ids, []string{clear.ID, account.ID, second.ID, first.ID}) || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 4 {
		t.Fatal("append-only insertion order changed", ids)
	}
	for _, tc := range []struct {
		id, author string
		count      int
	}{{"same", " CAPTURED   AUTHOR ", 1}, {"other", "Different Author", 0}} {
		f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,evidence_snapshot_json,created_at) SELECT ?,session_id,run_id,source,?,rank+1,item_json,assessment_json,coverage_json,json_set(evidence_snapshot_json,'$.author',?),created_at FROM timeline_items WHERE id='ai-item'`, tc.id, tc.id+"-evidence", tc.author)
		shared := f.history(t, "/api/timeline/"+tc.id+"/ai-feedback")
		if len(shared) != tc.count || tc.count == 1 && shared[0].ID != account.ID {
			t.Fatal("account identity isolation changed", shared)
		}
	}
}

func TestHTTPAIFeedbackCharacterizationValidation(t *testing.T) {
	f := newHTTPAIFeedbackFixture(t)
	for _, tc := range []struct {
		name, path, body, media string
		status                  int
		code                    string
	}{
		{"missing_fields", httpAIFeedbackPath, `{}`, "application/json", 400, "invalid_request"},
		{"null", httpAIFeedbackPath, `null`, "application/json", 400, "invalid_request"},
		{"clear_input", httpAIFeedbackPath, strings.Replace(httpAIFeedbackAI, `"ai"`, `"clear"`, 1), "application/json", 400, "invalid_request"},
		{"target", httpAIFeedbackPath, strings.Replace(httpAIFeedbackAI, `"post"`, `"other"`, 1), "application/json", 400, "invalid_request"},
		{"scope", httpAIFeedbackPath, strings.Replace(httpAIFeedbackAI, "social_post", "attached_media", 1), "application/json", 400, "invalid_request"},
		{"reason", httpAIFeedbackPath, strings.TrimSuffix(httpAIFeedbackAI, "}") + `,"reason":"free text"}`, "application/json", 400, "invalid_request"},
		{"unknown", httpAIFeedbackPath, strings.TrimSuffix(httpAIFeedbackAI, "}") + `,"extra":true}`, "application/json", 400, "invalid_request"},
		{"wrong_type", httpAIFeedbackPath, `{"verdict":2}`, "application/json", 400, "invalid_request"},
		{"malformed", httpAIFeedbackPath, `{`, "application/json", 400, "invalid_request"},
		{"media", httpAIFeedbackPath, httpAIFeedbackAI, "text/plain", 415, "unsupported_media_type"},
		{"missing_item", "/api/timeline/missing/ai-feedback", httpAIFeedbackAI, "application/json", 404, "not_found"},
		{"validation_before_missing", "/api/timeline/missing/ai-feedback", `{}`, "application/json", 400, "invalid_request"},
		{"missing_undo", httpAIFeedbackUndo("missing"), "", "", 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sessionUpdatesRequest("POST", tc.path, tc.body)
			r.Header.Set("Content-Type", tc.media)
			f.error(t, f.serve(r), tc.status, tc.code, "")
			if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 0 {
				t.Fatal("invalid input was persisted")
			}
		})
	}
	f.error(t, f.serve(sessionUpdatesRequest("GET", "/api/timeline/missing/ai-feedback", "")), 404, "not_found", "timeline item not found")
	f.exec(t, `UPDATE timeline_items SET item_json='{}',evidence_snapshot_json='{}' WHERE id='ai-item'`)
	f.error(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, `{"verdict":"ai","targetType":"account","signalScope":"author_account"}`)), 400, "invalid_request", "no stable captured account identity")
}

func TestHTTPAIFeedbackCharacterizationRouting(t *testing.T) {
	for _, action := range []string{"submit", "history", "undo"} {
		t.Run(action, func(t *testing.T) {
			for _, mode := range []string{"canonical", "slash", "encoded_slash", "nested_id", "wrong_method", "foreign_host", "foreign_origin", "preflight"} {
				t.Run(mode, func(t *testing.T) {
					f := newHTTPAIFeedbackFixture(t)
					method, target, body, status := "POST", httpAIFeedbackPath, httpAIFeedbackAI, 201
					if action == "history" {
						method, body, status = "GET", "", 200
					}
					if action == "undo" {
						prior := f.event(t, f.serve(sessionUpdatesRequest("POST", target, body)), 201)
						target, body, status = httpAIFeedbackUndo(prior.ID), "", 200
					}
					if mode == "slash" {
						target += "/"
					}
					if mode == "encoded_slash" {
						target += "%2F"
					}
					if mode == "nested_id" {
						prefix := "/api/timeline/"
						if action == "undo" {
							prefix = "/api/ai-feedback/"
						}
						target = strings.Replace(target, prefix, prefix+"nested/", 1)
					}
					r := sessionUpdatesRequest(method, target, body)
					switch mode {
					case "wrong_method":
						r.Method, status = "PUT", 404
					case "foreign_host":
						r.Host, status = "example.com", 403
					case "foreign_origin":
						r.Header.Set("Origin", "https://example.com")
						status = 403
					case "preflight":
						r.Method, status = "OPTIONS", 204
					}
					before := f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`)
					w := f.serve(r)
					if w.Code != status {
						t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body)
					}
					if status == 404 {
						f.error(t, w, 404, "not_found", "")
					}
					if status == 403 && w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
						t.Fatal("security rejection reached API")
					}
					if status == 204 && w.Body.Len() != 0 {
						t.Fatal("preflight body changed")
					}
					if status >= 400 || status == 204 {
						if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != before {
							t.Fatal("rejected route mutated feedback")
						}
					} else if action != "history" {
						f.event(t, w, status)
					} else {
						f.json(t, w, 200)
					}
				})
			}
		})
	}
}

type httpAIFeedbackResolver struct {
	onModel func()
	entered chan context.Context
	release chan struct{}
	once    sync.Once
}

func (*httpAIFeedbackResolver) Name() string { return "offline-http-ai-feedback" }
func (r *httpAIFeedbackResolver) Model() config.ModelConfig {
	if r.onModel != nil {
		r.onModel()
	}
	return config.ModelConfig{Model: "offline-feedback", Effort: "low"}
}
func (r *httpAIFeedbackResolver) unblock() { r.once.Do(func() { close(r.release) }) }
func (r *httpAIFeedbackResolver) Resolve(ctx context.Context, items []domain.TimelineItem) (domain.DeepAIResult, domain.ModelUsage, time.Duration, error) {
	r.entered <- ctx
	select {
	case <-ctx.Done():
		return domain.DeepAIResult{}, domain.ModelUsage{}, 0, ctx.Err()
	case <-r.release:
	}
	if err := ctx.Err(); err != nil {
		return domain.DeepAIResult{}, domain.ModelUsage{}, 0, err
	}
	values := make([]domain.DeepAIAssessment, len(items))
	for i := range values {
		values[i] = domain.DeepAIAssessment{Status: "no_signal_detected", ConfidenceBand: "medium", EvidenceCodes: []string{}, AssessedObject: "social_post", SignalScope: "social_post", Rationale: "Offline HTTP characterization"}
	}
	return domain.DeepAIResult{Assessments: values}, domain.ModelUsage{}, time.Millisecond, nil
}
func (f httpAIFeedbackFixture) resolver(t *testing.T) *httpAIFeedbackResolver {
	t.Helper()
	r := &httpAIFeedbackResolver{entered: make(chan context.Context, 2), release: make(chan struct{})}
	f.s.engine.SetAIDeepResolver(r)
	t.Cleanup(r.unblock)
	return r
}
func awaitHTTPAIFeedbackReview(t *testing.T, r *httpAIFeedbackResolver) context.Context {
	t.Helper()
	select {
	case ctx := <-r.entered:
		return ctx
	case <-time.After(3 * time.Second):
		t.Fatal("offline review did not start")
		return nil
	}
}

func TestHTTPAIFeedbackCharacterizationHandoffFailures(t *testing.T) {
	for _, mode := range []string{"disabled", "nil_resolver", "settings", "item", "queue"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPAIFeedbackFixture(t)
			f.enabled(t, mode != "disabled")
			if mode != "nil_resolver" {
				f.resolver(t)
			}
			switch mode {
			case "settings":
				f.exec(t, `CREATE TRIGGER http_ai_bad_settings AFTER INSERT ON ai_feedback_events BEGIN UPDATE settings SET value_json='{' WHERE key='runtime'; END`)
			case "item":
				f.exec(t, `CREATE TRIGGER http_ai_delete_item AFTER INSERT ON ai_feedback_events BEGIN DELETE FROM timeline_items WHERE id=NEW.timeline_id; END`)
			case "queue":
				f.exec(t, `CREATE TRIGGER http_ai_bad_queue BEFORE INSERT ON ai_detection_jobs BEGIN SELECT RAISE(ABORT,'offline-http-queue-failed'); END`)
			}
			value := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, httpAIFeedbackUnsure)), 201)
			if value.Verdict != "unsure" || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 1 || f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs`) != 0 {
				t.Fatal("handoff failure lost durable feedback")
			}
		})
	}
}

func TestHTTPAIFeedbackCharacterizationReviewCancellation(t *testing.T) {
	f := newHTTPAIFeedbackFixture(t)
	f.enabled(t, true)
	r := f.resolver(t)
	first := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, httpAIFeedbackUnsure)), 201)
	old := awaitHTTPAIFeedbackReview(t, r)
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.onModel = func() {
		if old.Err() != context.Canceled || f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != 2 {
			t.Error("feedback/old-review cancellation/queue order changed")
		}
		cancel()
	}
	second := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackPath, httpAIFeedbackUnsure).WithContext(requestCtx)), 201)
	current := awaitHTTPAIFeedbackReview(t, r)
	if second.SupersedesID != first.ID || requestCtx.Err() != context.Canceled || current.Err() != nil {
		t.Fatal("request cancellation replaced review lifetime")
	}
	clear := f.event(t, f.serve(sessionUpdatesRequest("POST", httpAIFeedbackUndo(second.ID), "")), 200)
	if clear.Verdict != "clear" || current.Err() != nil || f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs`) != 2 {
		t.Fatal("clear cancelled/requeued the running review")
	}
	r.unblock()
	if !f.s.engine.WaitForIdle(3 * time.Second) {
		t.Fatal("offline review did not retire")
	}
	if f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs WHERE status='completed'`) != 1 || f.count(t, `SELECT COUNT(*) FROM ai_detection_jobs WHERE status='cancelled'`) != 1 {
		t.Fatal("review outcomes changed")
	}
}

func TestHTTPAIFeedbackCharacterizationErrorsAndWriteFailures(t *testing.T) {
	for _, action := range []string{"submit", "history", "undo"} {
		for _, mode := range []string{"cancelled", "deadline", "closed_store", "insert_failure", "write_failure"} {
			if action == "history" && mode == "insert_failure" {
				continue
			}
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newHTTPAIFeedbackFixture(t)
				method, target, body, status := "POST", httpAIFeedbackPath, httpAIFeedbackAI, 201
				if action == "history" {
					method, body, status = "GET", "", 200
				}
				if action == "undo" {
					prior := f.event(t, f.serve(sessionUpdatesRequest("POST", target, body)), 201)
					target, body, status = httpAIFeedbackUndo(prior.ID), "", 200
				}
				before := f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`)
				r := sessionUpdatesRequest(method, target, body)
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
				case "insert_failure":
					f.exec(t, `CREATE TRIGGER http_ai_write_fault BEFORE INSERT ON ai_feedback_events BEGIN SELECT RAISE(ABORT,'offline-http-feedback-failed'); END`)
				}
				if mode == "write_failure" {
					failure := errors.New("http-ai-feedback-write-failed")
					w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
					if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != status {
						t.Fatal("response error changed", w.status, err)
					}
					want := before
					if action != "history" {
						want++
					}
					if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != want {
						t.Fatal("response failure undid committed feedback")
					}
				} else {
					status, code := 400, "invalid_request"
					if action == "history" {
						status, code = 500, "internal_error"
					}
					f.error(t, f.serve(r), status, code, "")
					if f.count(t, `SELECT COUNT(*) FROM ai_feedback_events`) != before {
						t.Fatal("failed mutation changed history")
					}
				}
			})
		}
	}
}
