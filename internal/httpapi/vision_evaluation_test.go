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

const httpVisionListPath = "/api/inbox/runs/vision-run/vision-evaluations"
const httpVisionRetryPath = "/api/vision-evaluations/target/retry"

type httpVisionClock struct{ at time.Time }

func (c httpVisionClock) Now() time.Time { return c.at }

type httpVisionProvider struct {
	reasoning.Deterministic
	calls atomic.Int32
}

func (p *httpVisionProvider) Plan(ctx context.Context, run domain.Run, observation domain.Observation, previous []domain.ReasonedItem) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	p.calls.Add(1)
	return p.Deterministic.Plan(ctx, run, observation, previous)
}
func (p *httpVisionProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, previous []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.calls.Add(1)
	return p.Deterministic.Analyze(ctx, run, observation, previous)
}

type httpVisionFixture struct {
	sessionUpdatesFixture
	now      time.Time
	provider *httpVisionProvider
}

func newHTTPVisionFixture(t *testing.T) httpVisionFixture {
	t.Helper()
	now := time.Date(2026, 11, 3, 2, 0, 0, 123456789, time.UTC)
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ActiveSources = []domain.Source{domain.SourceInstagram}
	root := t.TempDir()
	state, err := store.OpenWithClock(filepath.Join(root, "vision.db"), settings, httpVisionClock{now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Root: root, Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	logger := log.New(io.Discard, "", 0)
	provider := &httpVisionProvider{}
	e := engine.New(state, provider, cfg, logger)
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
			t.Error("vision fixture workers did not drain")
		}
	})
	f := httpVisionFixture{sessionUpdatesFixture{s, db}, now, provider}
	f.seedRun(t, "vision", domain.SourceInstagram)
	return f
}

func (f httpVisionFixture) seedRun(t *testing.T, id string, source domain.Source) {
	t.Helper()
	f.seed(t, id, "completed", true)
	f.exec(t, `UPDATE runs SET source=?,status='completed',stage='completed' WHERE id=?`, source, id+"-run")
	f.exec(t, `DELETE FROM timeline_items WHERE run_id=?`, id+"-run")
}

func httpVisionTime(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z07:00")
}

func (f httpVisionFixture) job(t *testing.T, id, runID string, source domain.Source, status string, queuedAt time.Time) {
	t.Helper()
	var sessionID string
	if err := f.db.QueryRow(`SELECT session_id FROM runs WHERE id=?`, runID).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	candidate := domain.Block{EvidenceKey: "instagram:p:" + id, Author: "Fixture", Permalink: "https://www.instagram.com/p/" + id + "/", Media: []map[string]any{{"kind": "image", "url": "https://scontent.cdninstagram.com/" + id + ".jpg"}}}
	raw, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO vision_evaluation_jobs(id,run_id,session_id,source,evidence_key,canonical_identity,media_fingerprint,status,reason,attempt_count,queued_at,next_attempt_at,last_error,candidate_json,result_json,created_at,started_at,completed_at) VALUES(?,?,?,?,?,?,?,?,'original_reason',2,?,?,'previous failure',?,'{"retained":"result"}',?,?,?)`, id, runID, sessionID, source, candidate.EvidenceKey, candidate.EvidenceKey, "fixture-"+id, status, httpVisionTime(queuedAt), httpVisionTime(f.now.Add(time.Hour)), string(raw), httpVisionTime(f.now.Add(-48*time.Hour)), httpVisionTime(f.now.Add(-24*time.Hour)), httpVisionTime(f.now.Add(-time.Hour)))
}

type httpVisionStored struct {
	ID, RunID, SessionID, Source, EvidenceKey, Identity, Fingerprint, Status, Reason string
	Attempts                                                                         int
	QueuedAt                                                                         string
	Next                                                                             sql.NullString
	LastError, Candidate, Result, CreatedAt                                          string
	Started, Completed                                                               sql.NullString
}

func (f httpVisionFixture) row(t *testing.T, id string) httpVisionStored {
	t.Helper()
	var value httpVisionStored
	err := f.db.QueryRow(`SELECT id,run_id,session_id,source,evidence_key,canonical_identity,media_fingerprint,status,reason,attempt_count,queued_at,next_attempt_at,last_error,candidate_json,result_json,created_at,started_at,completed_at FROM vision_evaluation_jobs WHERE id=?`, id).Scan(&value.ID, &value.RunID, &value.SessionID, &value.Source, &value.EvidenceKey, &value.Identity, &value.Fingerprint, &value.Status, &value.Reason, &value.Attempts, &value.QueuedAt, &value.Next, &value.LastError, &value.Candidate, &value.Result, &value.CreatedAt, &value.Started, &value.Completed)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func (f httpVisionFixture) snapshot(t *testing.T) []string {
	t.Helper()
	result := []string{}
	for _, query := range []string{`SELECT json_array(id,run_id,session_id,source,evidence_key,canonical_identity,media_fingerprint,status,reason,attempt_count,queued_at,next_attempt_at,last_error,candidate_json,result_json,created_at,started_at,completed_at) FROM vision_evaluation_jobs ORDER BY id`, `SELECT json_array(id,status,stage) FROM runs ORDER BY id`} {
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
	return result
}

func (f httpVisionFixture) noDispatch(t *testing.T) {
	t.Helper()
	if f.provider.calls.Load() != 0 || f.count(t, `SELECT COUNT(*) FROM bridge_commands`) != 0 || f.count(t, `SELECT COUNT(*) FROM reasoning_invocations`) != 0 {
		t.Fatal("vision adapter launched capture or model work")
	}
}

func (f httpVisionFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f httpVisionFixture) jobs(t *testing.T, w *httptest.ResponseRecorder) []domain.VisionEvaluationJob {
	t.Helper()
	payload := f.json(t, w, 200)
	var jobs []domain.VisionEvaluationJob
	if len(payload) != 1 || json.Unmarshal(payload["jobs"], &jobs) != nil || jobs == nil {
		t.Fatal("vision jobs envelope changed", w.Body)
	}
	return jobs
}

func (f httpVisionFixture) accepted(t *testing.T, w *httptest.ResponseRecorder, id string) {
	t.Helper()
	payload := f.json(t, w, 202)
	var accepted bool
	var jobID string
	if len(payload) != 2 || json.Unmarshal(payload["accepted"], &accepted) != nil || json.Unmarshal(payload["jobId"], &jobID) != nil || !accepted || jobID != id {
		t.Fatal("vision retry envelope changed", w.Body)
	}
}

func TestHTTPVisionEvaluationCharacterizationList(t *testing.T) {
	f := newHTTPVisionFixture(t)
	if got := f.jobs(t, f.serve(sessionUpdatesRequest("GET", httpVisionListPath, ""))); len(got) != 0 {
		t.Fatal("empty jobs changed", got)
	}
	for i, status := range []string{"pending", "deferred", "evaluating", "retry_wait", "ready", "failed"} {
		id := fmt.Sprintf("job-%d", i)
		queued := f.now.Add(time.Duration(i/2) * time.Nanosecond)
		f.job(t, id, "vision-run", domain.SourceInstagram, status, queued)
	}
	f.seedRun(t, "peer", domain.SourceInstagram)
	f.job(t, "other-run", "peer-run", domain.SourceInstagram, "pending", f.now.Add(-time.Hour))
	f.job(t, "other-source", "vision-run", domain.SourceX, "pending", f.now.Add(-time.Hour))
	f.exec(t, `UPDATE vision_evaluation_jobs SET candidate_json=json_set(candidate_json,'$.unknownFixture','omitted') WHERE id='job-0'`)
	f.exec(t, `UPDATE vision_evaluation_jobs SET candidate_json='{' WHERE id='job-5'`)
	before := f.snapshot(t)
	w := f.serve(sessionUpdatesRequest("GET", httpVisionListPath, ""))
	jobs := f.jobs(t, w)
	if len(jobs) != 6 {
		t.Fatal("run/source filtering changed", jobs)
	}
	var payload struct {
		Jobs []map[string]json.RawMessage `json:"jobs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for i, job := range jobs {
		if job.ID != fmt.Sprintf("job-%d", i) || job.Source != domain.SourceInstagram || job.RunID != "vision-run" || job.EvidenceKey != "instagram:p:"+job.ID || job.QueuePosition != 0 || payload.Jobs[i]["queuePosition"] != nil || payload.Jobs[i]["result"] != nil || payload.Jobs[i]["resultJson"] != nil {
			t.Fatal("job ordering/field projection changed", job, w.Body)
		}
		if i < 5 && (job.ManualLabel != "Manual review · image post" || job.Candidate == nil) {
			t.Fatal("manual label/candidate changed", job)
		}
		if i == 5 && (job.ManualLabel != "" || job.Candidate != nil || payload.Jobs[i]["candidate"] != nil) {
			t.Fatal("malformed candidate handling changed", job)
		}
	}
	if strings.Contains(w.Body.String(), "unknownFixture") || !reflect.DeepEqual(before, f.snapshot(t)) {
		t.Fatal("list projected unknown fields or mutated queue", w.Body)
	}
	f.exec(t, `UPDATE sessions SET status='running' WHERE id='vision'`)
	f.jobs(t, f.serve(sessionUpdatesRequest("GET", httpVisionListPath, "")))
	f.noDispatch(t)
}

func TestHTTPVisionEvaluationCharacterizationRetryAdmission(t *testing.T) {
	for _, status := range []string{"pending", "deferred", "evaluating", "retry_wait", "ready", "failed"} {
		t.Run(status, func(t *testing.T) {
			f := newHTTPVisionFixture(t)
			f.job(t, "target", "vision-run", domain.SourceInstagram, status, f.now.Add(-time.Hour))
			before := f.snapshot(t)
			w := f.serve(sessionUpdatesRequest("POST", httpVisionRetryPath, ""))
			if status == "failed" {
				f.accepted(t, w, "target")
				if f.row(t, "target").Status != "pending" {
					t.Fatal("failed job not requeued")
				}
			} else {
				f.error(t, w, 409, "conflict", "vision evaluation target is "+status+", not failed")
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("non-failed retry mutated queue")
				}
			}
			f.noDispatch(t)
		})
	}
	t.Run("active_session", func(t *testing.T) {
		f := newHTTPVisionFixture(t)
		f.job(t, "target", "vision-run", domain.SourceInstagram, "failed", f.now.Add(-time.Hour))
		f.exec(t, `UPDATE sessions SET status='running'`)
		f.accepted(t, f.serve(sessionUpdatesRequest("POST", httpVisionRetryPath, "")), "target")
		f.noDispatch(t)
	})
}

func TestHTTPVisionEvaluationCharacterizationCapacityAndReset(t *testing.T) {
	for _, mode := range []string{"free", "full", "older_peer_first", "peer_id_tie", "other_source"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPVisionFixture(t)
			f.job(t, "target", "vision-run", domain.SourceInstagram, "failed", f.now.Add(-time.Hour))
			f.seedRun(t, "peer", domain.SourceInstagram)
			active := 0
			if mode == "full" || mode == "other_source" {
				active = 8
			}
			if mode == "older_peer_first" || mode == "peer_id_tie" {
				active = 7
			}
			for i := 0; i < active; i++ {
				source := domain.SourceInstagram
				if mode == "other_source" {
					source = domain.SourceX
				}
				status := []string{"pending", "evaluating", "retry_wait"}[i%3]
				f.job(t, fmt.Sprintf("active-%d", i), "peer-run", source, status, f.now.Add(-48*time.Hour))
			}
			if mode == "older_peer_first" || mode == "peer_id_tie" {
				f.job(t, "peer-z", "peer-run", domain.SourceInstagram, "deferred", f.now.Add(-72*time.Hour))
				queued := f.now.Add(-48 * time.Hour)
				if mode == "peer_id_tie" {
					queued = f.now.Add(-72 * time.Hour)
				}
				f.job(t, "peer-a", "peer-run", domain.SourceInstagram, "deferred", queued)
			}
			before := f.row(t, "target")
			f.accepted(t, f.serve(sessionUpdatesRequest("POST", httpVisionRetryPath, "")), "target")
			want := before
			want.Status = "pending"
			if mode == "full" || mode == "older_peer_first" || mode == "peer_id_tie" {
				want.Status = "deferred"
			}
			want.Reason = "manual_retry"
			want.Attempts = 0
			want.QueuedAt = httpVisionTime(f.now)
			want.Next = sql.NullString{}
			want.LastError = ""
			want.Completed = sql.NullString{}
			if got := f.row(t, "target"); !reflect.DeepEqual(got, want) {
				t.Fatalf("reset/retention changed: got=%+v want=%+v", got, want)
			}
			if mode == "older_peer_first" || mode == "peer_id_tie" {
				promoted, waiting := "peer-z", "peer-a"
				if mode == "peer_id_tie" {
					promoted, waiting = waiting, promoted
				}
				if f.row(t, promoted).Status != "pending" || f.row(t, promoted).Reason != "capacity_available" || f.row(t, waiting).Status != "deferred" {
					t.Fatal("peer FIFO promotion priority changed")
				}
			}
			f.noDispatch(t)
		})
	}
}

func TestHTTPVisionEvaluationCharacterizationPartialFailures(t *testing.T) {
	for _, mode := range []string{"promotion_failure", "target_failure", "target_no_rows"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPVisionFixture(t)
			f.job(t, "target", "vision-run", domain.SourceInstagram, "failed", f.now.Add(-time.Hour))
			f.job(t, "peer", "vision-run", domain.SourceInstagram, "deferred", f.now.Add(-24*time.Hour))
			beforeTarget, beforePeer := f.row(t, "target"), f.row(t, "peer")
			switch mode {
			case "promotion_failure":
				f.exec(t, `CREATE TRIGGER vision_fault BEFORE UPDATE OF status ON vision_evaluation_jobs WHEN OLD.id='peer' BEGIN SELECT RAISE(ABORT,'offline-promotion-fault'); END`)
			case "target_failure":
				f.exec(t, `CREATE TRIGGER vision_fault BEFORE UPDATE OF status ON vision_evaluation_jobs WHEN OLD.id='target' BEGIN SELECT RAISE(ABORT,'offline-target-fault'); END`)
			case "target_no_rows":
				f.exec(t, `CREATE TRIGGER vision_fault BEFORE UPDATE OF status ON vision_evaluation_jobs WHEN OLD.id='target' BEGIN SELECT RAISE(IGNORE); END`)
			}
			w := f.serve(sessionUpdatesRequest("POST", httpVisionRetryPath, ""))
			if mode == "target_no_rows" {
				f.error(t, w, 404, "not_found", "vision evaluation not found")
			} else {
				message := "offline-target-fault"
				if mode == "promotion_failure" {
					message = "offline-promotion-fault"
				}
				f.error(t, w, 409, "conflict", message)
			}
			if f.row(t, "target") != beforeTarget {
				t.Fatal("failed target update mutated target")
			}
			wantPeer := beforePeer
			if mode != "promotion_failure" {
				wantPeer.Status = "pending"
				wantPeer.Reason = "capacity_available"
			}
			if f.row(t, "peer") != wantPeer {
				t.Fatal("partial promotion commit contract changed")
			}
			f.noDispatch(t)
		})
	}
}

func TestHTTPVisionEvaluationCharacterizationRouting(t *testing.T) {
	for _, action := range []string{"list", "retry"} {
		for _, mode := range []string{"canonical", "slash", "encoded_slash", "double_slash", "nested_id", "wrong_method", "foreign_host", "foreign_origin", "preflight"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newHTTPVisionFixture(t)
				f.job(t, "target", "vision-run", domain.SourceInstagram, "failed", f.now.Add(-time.Hour))
				method, target, status := "GET", httpVisionListPath, 200
				if action == "retry" {
					method, target, status = "POST", httpVisionRetryPath, 202
				}
				switch mode {
				case "slash":
					target += "/"
				case "encoded_slash":
					target += "%2F"
				case "double_slash":
					target += "//"
				case "nested_id":
					if action == "list" {
						target = "/api/inbox/runs/nested/vision-run/vision-evaluations"
					} else {
						target = "/api/vision-evaluations/nested/target/retry"
					}
					status = 404
				}
				r := sessionUpdatesRequest(method, target, "")
				if mode == "double_slash" {
					reference := httptest.NewRecorder()
					mux := http.NewServeMux()
					mux.HandleFunc("/api/", func(http.ResponseWriter, *http.Request) { t.Fatal("unclean path reached reference handler") })
					mux.ServeHTTP(reference, r.Clone(r.Context()))
					status = reference.Code
				}
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
				before := f.snapshot(t)
				w := f.serve(r)
				if w.Code != status {
					t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body)
				}
				if status == 200 {
					f.jobs(t, w)
				}
				if status == 202 {
					f.accepted(t, w, "target")
				}
				if status == 404 {
					f.error(t, w, 404, "not_found", "")
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
				if status != 202 && !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("list or rejected request mutated queue")
				}
				f.noDispatch(t)
			})
		}
	}
}

type httpVisionBody struct {
	io.Reader
	reads, closes int
}

func (b *httpVisionBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *httpVisionBody) Close() error               { b.closes++; return nil }

func TestHTTPVisionEvaluationCharacterizationContextAndWriteFailures(t *testing.T) {
	for _, action := range []string{"list", "retry"} {
		for _, mode := range []string{"ignored_body", "cancelled", "deadline", "closed_store", "write_failure"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newHTTPVisionFixture(t)
				f.job(t, "target", "vision-run", domain.SourceInstagram, "failed", f.now.Add(-time.Hour))
				method, target, status := "GET", httpVisionListPath, 200
				if action == "retry" {
					method, target, status = "POST", httpVisionRetryPath, 202
				}
				r := sessionUpdatesRequest(method, target, "")
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
				}
				before := f.snapshot(t)
				if mode == "ignored_body" {
					body := &httpVisionBody{Reader: strings.NewReader("{invalid")}
					r.Body = body
					r.Header.Set("Content-Type", "text/plain")
					w := f.serve(r)
					if action == "list" {
						f.jobs(t, w)
					} else {
						f.accepted(t, w, "target")
					}
					if body.reads != 0 || body.closes != 0 {
						t.Fatal("body was consumed")
					}
				} else if mode == "write_failure" {
					failure := errors.New("offline-vision-response-failed")
					w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
					if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != status {
						t.Fatal("write error changed", w.status, err)
					}
				} else {
					code, message := "internal_error", "internal server error"
					want := 500
					if action == "retry" {
						want, code, message = 409, "conflict", ""
					}
					f.error(t, f.serve(r), want, code, message)
				}
				mutates := action == "retry" && (mode == "ignored_body" || mode == "write_failure")
				if mutates {
					if f.row(t, "target").Status != "pending" {
						t.Fatal("successful retry was not durable")
					}
				} else if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("failed request/list mutated queue")
				}
				f.noDispatch(t)
			})
		}
	}
}

func TestHTTPVisionEvaluationCharacterizationMissingAndIDPrecedence(t *testing.T) {
	for _, action := range []string{"list", "retry"} {
		for _, mode := range []string{"missing", "nested_cancelled", "empty_id"} {
			t.Run(action+"/"+mode, func(t *testing.T) {
				f := newHTTPVisionFixture(t)
				method, target, message := "GET", "/api/inbox/runs/missing/vision-evaluations", "run not found"
				if action == "retry" {
					method, target, message = "POST", "/api/vision-evaluations/missing/retry", "vision evaluation not found"
				}
				if mode == "nested_cancelled" {
					if action == "list" {
						target = "/api/inbox/runs/nested/missing/vision-evaluations"
					} else {
						target = "/api/vision-evaluations/nested/missing/retry"
					}
				}
				if mode == "empty_id" {
					if action == "list" {
						target = "/api/inbox/runs//vision-evaluations"
					} else {
						target = "/api/vision-evaluations//retry"
					}
				}
				r := sessionUpdatesRequest(method, target, "")
				if mode == "nested_cancelled" {
					ctx, cancel := context.WithCancel(r.Context())
					cancel()
					r = r.WithContext(ctx)
				}
				before := f.snapshot(t)
				if mode == "empty_id" {
					err := f.s.route(httptest.NewRecorder(), r)
					value, ok := err.(apiError)
					if !ok || value.Status != 404 || value.Message != message {
						t.Fatal("empty ID precedence changed", err)
					}
				} else {
					f.error(t, f.serve(r), 404, "not_found", message)
				}
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("invalid/missing request mutated state")
				}
				f.noDispatch(t)
			})
		}
	}
}
