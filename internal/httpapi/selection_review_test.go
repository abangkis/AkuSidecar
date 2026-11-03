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
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

const selectionReviewRunPath = "/api/inbox/runs/review-run/"

type selectionReviewInvocation struct {
	ctx         context.Context
	run         domain.Run
	observation domain.Observation
}

type selectionReviewProvider struct {
	reasoning.Deterministic
	entered chan selectionReviewInvocation
	release chan struct{}
	once    sync.Once
	plans   atomic.Int32
}

func (p *selectionReviewProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, previous []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	p.plans.Add(1)
	return p.Deterministic.Plan(ctx, run, observation, previous)
}

func (p *selectionReviewProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, previous []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.entered <- selectionReviewInvocation{ctx, run, observation}
	select {
	case <-ctx.Done():
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, ctx.Err()
	case <-p.release:
		return p.Deterministic.Analyze(ctx, run, observation, previous)
	}
}

func (p *selectionReviewProvider) unblock() { p.once.Do(func() { close(p.release) }) }

type selectionReviewFixture struct {
	sessionUpdatesFixture
	provider *selectionReviewProvider
	ref      string
}

func newSelectionReviewFixture(t *testing.T) selectionReviewFixture {
	t.Helper()
	f := selectionReviewFixture{sessionUpdatesFixture: newSessionUpdatesFixture(t)}
	f.provider = &selectionReviewProvider{entered: make(chan selectionReviewInvocation, 1), release: make(chan struct{})}
	e := engine.New(f.s.store, f.provider, f.s.config, f.s.logger)
	f.s.engine = e
	t.Cleanup(func() {
		e.Shutdown()
		f.provider.unblock()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("selection review workers did not drain")
		}
	})
	settings, err := f.s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.AIDetectionEnabled, settings.CalibrationEnabled, settings.SemanticEventMode = false, false, "show_all"
	if err := f.s.store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	f.seed(t, "review", "completed", true)
	f.exec(t, `DELETE FROM timeline_items`)
	f.exec(t, `UPDATE runs SET status='completed',stage='completed',completed_at=?,summary='original',coverage_json='{"reviewDiagnostic":"retained"}' WHERE id='review-run'`, domain.Now())
	now := domain.Now()
	observation := domain.Observation{Source: domain.SourceLinkedIn, CapturedAt: now, Coverage: map[string]any{"quality": "complete"}, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "linkedin:selection-review", Author: "Offline author", Text: "An independently captured update that was evaluated but not selected in the original bounded run.", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456789/"}}}}}
	f.exec(t, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('review-command','review-run','collect_visible','completed','{}',?)`, now)
	f.exec(t, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('review-observation','review-run','review-command','linkedin',?,?,?)`, selectionReviewJSON(t, observation), now, now)
	run, err := f.s.store.GetRun(context.Background(), "review-run")
	if err != nil {
		t.Fatal(err)
	}
	result, _, err := (reasoning.Deterministic{}).Analyze(context.Background(), run, observation, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at) VALUES('review-run',?,'linkedin',?,?,0.4,0,0.4,0,?)`, result.Items[0].EvidenceKey, selectionReviewJSON(t, result.CandidateAssessments[0]), selectionReviewJSON(t, result.Items[0]), now)
	trace, err := f.s.store.InboxRunTrace(context.Background(), "review-run", "evaluated", 20, 0)
	if err != nil || len(trace.Items) != 1 || trace.Items[0].CandidateRef == "" {
		t.Fatalf("candidate trace=%+v err=%v", trace, err)
	}
	f.ref = trace.Items[0].CandidateRef
	return f
}

func selectionReviewJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f selectionReviewFixture) failed(t *testing.T) {
	t.Helper()
	f.exec(t, `DELETE FROM candidate_assessments`)
	f.exec(t, `UPDATE runs SET status='failed',stage='failed',error_json='{"code":"fixture_failure"}' WHERE id='review-run'`)
	f.exec(t, `UPDATE sessions SET status='failed',completed_at=?,error_json='{"code":"fixture_failure"}' WHERE id='review'`, domain.Now())
}

func (f selectionReviewFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f selectionReviewFixture) correction(t *testing.T) domain.SelectionCorrection {
	t.Helper()
	w := f.serve(sessionUpdatesRequest("POST", selectionReviewRunPath+"selection-corrections", selectionReviewJSON(t, map[string]string{"candidateRef": f.ref})))
	return f.result(t, w, 201)
}

func (f selectionReviewFixture) result(t *testing.T, w *httptest.ResponseRecorder, status int) domain.SelectionCorrection {
	t.Helper()
	payload := f.json(t, w, status)
	var correction domain.SelectionCorrection
	if err := json.Unmarshal(payload["correction"], &correction); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(payload["correction"], &fields)
	wantFields := 6
	if status == 200 {
		wantFields++
	}
	if len(fields) != wantFields || fields["evidenceKey"] != nil || correction.ID == "" || correction.SessionID != "review" || correction.RunID != "review-run" || correction.TimelineID == "" || correction.Action != "should_select" || correction.CreatedAt == "" || (correction.UndoneAt != nil) != (status == 200) {
		t.Fatalf("correction contract changed: %s", w.Body)
	}
	if status == 201 {
		item, err := f.s.store.TimelineItem(context.Background(), correction.TimelineID)
		if err != nil {
			t.Fatal(err)
		}
		var want, got any
		_ = json.Unmarshal([]byte(selectionReviewJSON(t, item)), &want)
		_ = json.Unmarshal(payload["item"], &got)
		if len(payload) != 2 || !reflect.DeepEqual(got, want) || item.EvidenceKey != "linkedin:selection-review" || item.Coverage["reviewDiagnostic"] != "retained" {
			t.Fatalf("item projection changed: %s", w.Body)
		}
	} else if len(payload) != 1 {
		t.Fatal("undo envelope changed", w.Body)
	}
	return correction
}

func (f selectionReviewFixture) request(t *testing.T, action string) *http.Request {
	t.Helper()
	switch action {
	case "correction":
		return sessionUpdatesRequest("POST", selectionReviewRunPath+"selection-corrections", selectionReviewJSON(t, map[string]string{"candidateRef": f.ref}))
	case "retry":
		f.failed(t)
		return sessionUpdatesRequest("POST", selectionReviewRunPath+"re-evaluate", "")
	case "undo":
		value := f.correction(t)
		return sessionUpdatesRequest("POST", "/api/selection-corrections/"+value.ID+"/undo", "")
	default:
		t.Fatal("unknown action")
		return nil
	}
}

func (f selectionReviewFixture) snapshot(t *testing.T) any {
	t.Helper()
	counts := []int{}
	for _, table := range []string{"selection_corrections WHERE undone_at IS NULL", "timeline_items", "knowledge_events", "observations", "bridge_commands", "candidate_assessments"} {
		counts = append(counts, f.count(t, "SELECT COUNT(*) FROM "+table))
	}
	var run, session string
	if err := f.db.QueryRow(`SELECT r.status,s.status FROM runs r JOIN sessions s ON s.id=r.session_id WHERE r.id='review-run'`).Scan(&run, &session); err != nil {
		t.Fatal(err)
	}
	return struct {
		Counts       []int
		Run, Session string
	}{counts, run, session}
}

func TestHTTPSelectionReviewCharacterizationCorrectionAndUndo(t *testing.T) {
	f := newSelectionReviewFixture(t)
	value := f.correction(t)
	f.error(t, f.serve(sessionUpdatesRequest("POST", selectionReviewRunPath+"selection-corrections", selectionReviewJSON(t, map[string]string{"candidateRef": f.ref}))), 409, "conflict", "active selection correction")
	if f.count(t, `SELECT COUNT(*) FROM timeline_items`) != 1 || f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != 1 {
		t.Fatal("correction durable effects changed")
	}
	target := "/api/selection-corrections/" + value.ID + "/undo"
	undone := f.result(t, f.serve(sessionUpdatesRequest("POST", target, "")), 200)
	if undone.ID != value.ID || f.count(t, `SELECT COUNT(*) FROM timeline_items`) != 0 || f.count(t, `SELECT COUNT(*) FROM knowledge_events`) != 0 {
		t.Fatal("undo durable effects changed")
	}
	f.error(t, f.serve(sessionUpdatesRequest("POST", target, "")), 404, "not_found", "selection correction not found")
}

func TestHTTPSelectionReviewCharacterizationCorrectionInput(t *testing.T) {
	for _, mode := range []string{"missing_type", "malformed", "unknown", "empty", "null", "missing_ref", "whitespace", "wrong_type", "padded_ref", "missing_run", "missing_candidate", "nested_run", "missing_id_segment", "duplicate_field", "trailing_json", "case_folded"} {
		t.Run(mode, func(t *testing.T) {
			f := newSelectionReviewFixture(t)
			target, body, status, code, message := selectionReviewRunPath+"selection-corrections", selectionReviewJSON(t, map[string]string{"candidateRef": f.ref}), 400, "invalid_request", ""
			switch mode {
			case "missing_type":
				status, code = 415, "unsupported_media_type"
			case "malformed":
				body = "{"
			case "unknown":
				body = `{"candidateRef":"x","extra":true}`
			case "empty":
				body = ""
			case "null":
				body = "null"
				message = "candidateRef is required"
			case "missing_ref":
				body = "{}"
				message = "candidateRef is required"
			case "whitespace":
				body = `{"candidateRef":"  "}`
				message = "candidateRef is required"
			case "wrong_type":
				body = `{"candidateRef":42}`
			case "padded_ref":
				body = selectionReviewJSON(t, map[string]string{"candidateRef": " " + f.ref + " "})
				status, code, message = 404, "not_found", "candidate not found"
			case "missing_run":
				target = "/api/inbox/runs/missing/selection-corrections"
				status, code, message = 404, "not_found", "candidate not found"
			case "missing_candidate":
				body = `{"candidateRef":"missing"}`
				status, code, message = 404, "not_found", "candidate not found"
			case "nested_run":
				target = "/api/inbox/runs/nested/review-run/selection-corrections"
				body = "{"
				status, code, message = 404, "not_found", "run not found"
			case "missing_id_segment":
				target = "/api/inbox/runs/selection-corrections"
				body = "{"
				// The suffix includes a slash: this path reaches JSON decoding
				// with the literal run ID "selection-corrections".
			case "duplicate_field":
				body = `{"candidateRef":"missing","candidateRef":"` + f.ref + `"}`
				status = 201
			case "trailing_json":
				body += ` {"ignored":true}`
				status = 201
			case "case_folded":
				body = `{"CandidateRef":"` + f.ref + `"}`
				status = 201
			}
			r := sessionUpdatesRequest("POST", target, body)
			if mode == "missing_type" {
				r.Header.Del("Content-Type")
			}
			before := f.snapshot(t)
			w := f.serve(r)
			if status == 201 {
				f.result(t, w, status)
			} else {
				f.error(t, w, status, code, message)
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("invalid request mutated state")
				}
			}
		})
	}
}

func TestHTTPSelectionReviewCharacterizationRouting(t *testing.T) {
	for _, action := range []string{"correction", "retry", "undo"} {
		for _, mode := range []string{"canonical", "slash", "encoded_slash", "nested_id", "wrong_method", "foreign_host", "foreign_origin", "preflight"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newSelectionReviewFixture(t)
				r := f.request(t, action)
				status := map[string]int{"correction": 201, "retry": 202, "undo": 200}[action]
				switch mode {
				case "slash":
					r.URL.Path += "/"
				case "encoded_slash":
					r = sessionUpdatesRequest("POST", r.URL.Path+"%2F", selectionReviewJSON(t, map[string]string{"candidateRef": f.ref}))
				case "nested_id":
					prefix := "/api/inbox/runs/"
					if action == "undo" {
						prefix = "/api/selection-corrections/"
					} else {
						status = 404
					}
					r.URL.Path = strings.Replace(r.URL.Path, prefix, prefix+"nested/", 1)
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
				before := f.snapshot(t)
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
				if status == 204 && (w.Body.Len() != 0 || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "") {
					t.Fatal("preflight contract changed")
				}
				if status >= 400 || status == 204 {
					if !reflect.DeepEqual(before, f.snapshot(t)) {
						t.Fatal("rejected route changed state")
					}
				} else if action != "retry" {
					f.result(t, w, status)
				} else {
					f.json(t, w, 202)
				}
			})
		}
	}
}

type selectionReviewBody struct {
	io.Reader
	reads, closes int
	onClose       func()
}

func (b *selectionReviewBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *selectionReviewBody) Close() error {
	b.closes++
	if b.onClose != nil {
		b.onClose()
	}
	return nil
}

func TestHTTPSelectionReviewCharacterizationBodyAndAdmission(t *testing.T) {
	for _, action := range []string{"correction", "retry", "undo"} {
		for _, mode := range []string{"ignored_body", "active", "active_missing", "cancelled", "deadline", "closed_store", "sql_failure", "write_failure"} {
			if action == "correction" && mode == "ignored_body" {
				continue
			}
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newSelectionReviewFixture(t)
				r := f.request(t, action)
				if mode == "active" || mode == "active_missing" {
					f.seed(t, "other", "running", false)
				}
				if mode == "active_missing" {
					if action == "undo" {
						r.URL.Path = "/api/selection-corrections/missing/undo"
					} else {
						r.URL.Path = strings.Replace(r.URL.Path, "review-run", "missing", 1)
					}
				}
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
				case "sql_failure":
					query := map[string]string{"correction": `BEFORE INSERT ON selection_corrections`, "retry": `BEFORE UPDATE OF status ON runs`, "undo": `BEFORE UPDATE OF undone_at ON selection_corrections`}[action]
					f.exec(t, `CREATE TRIGGER http_review_fault `+query+` BEGIN SELECT RAISE(ABORT,'offline-review-fault'); END`)
				}
				before := f.snapshot(t)
				if mode == "ignored_body" {
					body := &selectionReviewBody{Reader: strings.NewReader("{invalid")}
					r.Body = body
					r.Header.Set("Content-Type", "text/plain")
					w := f.serve(r)
					f.json(t, w, map[string]int{"retry": 202, "undo": 200}[action])
					if body.reads != 0 || body.closes != 0 {
						t.Fatal("body was consumed")
					}
				} else if mode == "write_failure" {
					failure := errors.New("offline-review-response-failed")
					w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
					if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != map[string]int{"correction": 201, "retry": 202, "undo": 200}[action] {
						t.Fatal("response failure contract changed", w.status, err)
					}
					if reflect.DeepEqual(before, f.snapshot(t)) {
						t.Fatal("response failure rolled back mutation")
					}
				} else {
					message := ""
					if mode == "active" || mode == "active_missing" {
						message = "finish the active update before"
					}
					if mode == "sql_failure" {
						message = "offline-review-fault"
					}
					f.error(t, f.serve(r), 409, "conflict", message)
					if !reflect.DeepEqual(before, f.snapshot(t)) {
						t.Fatal("rejected operation mutated state")
					}
				}
			})
		}
	}
	for _, body := range []string{"{", `{"candidateRef":" "}`} {
		t.Run("validation_before_active/"+body, func(t *testing.T) {
			f := newSelectionReviewFixture(t)
			f.seed(t, "other", "running", false)
			f.error(t, f.serve(sessionUpdatesRequest("POST", selectionReviewRunPath+"selection-corrections", body)), 400, "invalid_request", "")
		})
	}
	t.Run("context_after_decode", func(t *testing.T) {
		f := newSelectionReviewFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := f.request(t, "correction").WithContext(ctx)
		body := &selectionReviewBody{Reader: r.Body, onClose: cancel}
		r.Body = body
		before := f.snapshot(t)
		f.error(t, f.serve(r), 409, "conflict", "")
		if body.reads == 0 || body.closes != 1 || !reflect.DeepEqual(before, f.snapshot(t)) {
			t.Fatal("decode/context ordering changed")
		}
	})
}

func TestHTTPSelectionReviewCharacterizationRetryGuards(t *testing.T) {
	for _, mode := range []string{"not_failed", "no_capture", "evaluated", "timeline", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f := newSelectionReviewFixture(t)
			f.failed(t)
			target := selectionReviewRunPath + "re-evaluate"
			status, code, message := 409, "conflict", ""
			switch mode {
			case "not_failed":
				f.exec(t, `UPDATE runs SET status='completed'`)
				message = "only a failed run can be re-evaluated"
			case "no_capture":
				f.exec(t, `DELETE FROM observations`)
				message = "no durable captured evidence"
			case "evaluated":
				f.exec(t, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at) VALUES('review-run','other','linkedin','{}','{}',0,0,0,0,?)`, domain.Now())
				message = "already contains evaluated state"
			case "timeline":
				f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('existing','review','review-run','linkedin','other',0,'{}','{}',?)`, domain.Now())
				message = "already contains evaluated state"
			case "missing":
				target = "/api/inbox/runs/missing/re-evaluate"
				status, code, message = 404, "not_found", "run not found"
			}
			before := f.snapshot(t)
			f.error(t, f.serve(sessionUpdatesRequest("POST", target, "")), status, code, message)
			if !reflect.DeepEqual(before, f.snapshot(t)) {
				t.Fatal("rejected retry changed state")
			}
			select {
			case <-f.provider.entered:
				t.Fatal("rejected retry started provider")
			default:
			}
		})
	}
}

func TestHTTPSelectionReviewCharacterizationPartialFailures(t *testing.T) {
	for _, mode := range []string{"settings", "item_readback", "knowledge", "undo_cleanup"} {
		t.Run(mode, func(t *testing.T) {
			f := newSelectionReviewFixture(t)
			r := f.request(t, "correction")
			status, code, message := 409, "conflict", ""
			switch mode {
			case "settings":
				f.exec(t, `CREATE TRIGGER review_settings AFTER INSERT ON selection_corrections BEGIN UPDATE settings SET value_json='{' WHERE key='runtime'; END`)
			case "item_readback":
				f.exec(t, `CREATE TRIGGER review_readback AFTER INSERT ON knowledge_events BEGIN DELETE FROM timeline_items WHERE evidence_key=NEW.evidence_key; END`)
				status, code, message = 404, "not_found", "candidate not found"
			case "knowledge":
				f.exec(t, `CREATE TRIGGER review_knowledge BEFORE INSERT ON knowledge_events BEGIN SELECT RAISE(ABORT,'offline-knowledge-fault'); END`)
				status = 201
			case "undo_cleanup":
				r = f.request(t, "undo")
				f.exec(t, `INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('review-orphan','Offline orphan',?,?)`, domain.Now(), domain.Now())
				f.exec(t, `CREATE TRIGGER review_cleanup BEFORE DELETE ON semantic_events BEGIN SELECT RAISE(ABORT,'offline-cleanup-fault'); END`)
				message = "cleanup semantic event after undo:"
			}
			w := f.serve(r)
			if status == 201 {
				f.result(t, w, 201)
			} else {
				f.error(t, w, status, code, message)
			}
			if f.count(t, `SELECT COUNT(*) FROM selection_corrections`) != 1 {
				t.Fatal("committed correction disappeared")
			}
			wantItems := 1
			if mode == "item_readback" || mode == "undo_cleanup" {
				wantItems = 0
			}
			if f.count(t, `SELECT COUNT(*) FROM timeline_items`) != wantItems {
				t.Fatal("partial effect order changed")
			}
			if mode == "undo_cleanup" && f.count(t, `SELECT COUNT(*) FROM selection_corrections WHERE undone_at IS NOT NULL`) != 1 {
				t.Fatal("undo commit was lost")
			}
		})
	}
}

func TestHTTPSelectionReviewCharacterizationRetryHandoff(t *testing.T) {
	f := newSelectionReviewFixture(t)
	r := f.request(t, "retry")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(ctx)
	payload := f.json(t, f.serve(r), 202)
	var run domain.Run
	if len(payload) != 1 || json.Unmarshal(payload["run"], &run) != nil || run.ID != "review-run" || run.Status != "reasoning" || run.Stage != "reasoning" || run.Summary != "" || run.Error != nil || run.CompletedAt != nil || run.Coverage["reviewDiagnostic"] != "retained" {
		t.Fatal("retry response changed", payload)
	}
	var call selectionReviewInvocation
	select {
	case call = <-f.provider.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("retry did not reach provider")
	}
	cancel()
	if call.ctx.Err() != nil || call.run.ID != run.ID || len(call.observation.Snapshots) != 1 || len(call.observation.Snapshots[0].Blocks) != 1 || call.observation.Snapshots[0].Blocks[0].EvidenceKey != "linkedin:selection-review" {
		t.Fatal("retry worker context or durable capture changed")
	}
	f.error(t, f.serve(sessionUpdatesRequest("POST", selectionReviewRunPath+"re-evaluate", "")), 409, "conflict", "finish the active update before")
	f.provider.unblock()
	if !f.s.engine.WaitForIdle(3 * time.Second) {
		t.Fatal("retry did not finish")
	}
	updated, err := f.s.store.GetRun(context.Background(), run.ID)
	if err != nil || updated.Status != "completed" || f.provider.plans.Load() != 0 || f.count(t, `SELECT COUNT(*) FROM observations`) != 1 || f.count(t, `SELECT COUNT(*) FROM bridge_commands WHERE type='collect_visible'`) != 1 || f.count(t, `SELECT COUNT(*) FROM candidate_assessments`) != 1 {
		t.Fatalf("retry recaptured or failed: run=%+v err=%v plans=%d", updated, err, f.provider.plans.Load())
	}
}
