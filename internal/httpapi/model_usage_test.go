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

const httpSessionUsagePath = "/api/inbox/sessions/usage/model-usage"
const httpAggregateUsagePath = "/api/model-usage"

type httpModelUsageFixture struct{ sessionUpdatesFixture }

func newHTTPModelUsageFixture(t *testing.T) httpModelUsageFixture {
	t.Helper()
	return httpModelUsageFixture{newSessionUpdatesFixture(t)}
}

func httpUsageToken(value int64) *int64 { return &value }

func (f httpModelUsageFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.http.Handler.ServeHTTP(w, r)
	return w
}

func (f httpModelUsageFixture) snapshot(t *testing.T) []int {
	t.Helper()
	var counts []int
	for _, table := range []string{"sessions", "runs", "bridge_commands", "observations", "timeline_items", "reasoning_invocations", "event_resolution_invocations", "ai_detection_jobs", "living_topic_model_invocations"} {
		counts = append(counts, f.count(t, "SELECT COUNT(*) FROM "+table))
	}
	return counts
}

func (f httpModelUsageFixture) telemetry(t *testing.T, id, runID, phase string, tokens *int64, createdAt string) {
	t.Helper()
	if err := f.s.store.SaveTelemetry(context.Background(), domain.ReasoningTelemetry{ID: id, RunID: runID, Phase: phase, Provider: "offline", Model: "model-a", Effort: "high", Status: "completed", DurationMS: 100, InputTokens: tokens, CreatedAt: createdAt}); err != nil {
		t.Fatal(err)
	}
}

func (f httpModelUsageFixture) data(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	f.seed(t, "usage", "completed", true)
	createdAt := time.Now().UTC().AddDate(0, 0, -2).Format(time.RFC3339Nano)
	f.exec(t, `UPDATE sessions SET created_at=? WHERE id='usage'`, createdAt)
	f.exec(t, `UPDATE runs SET status='completed',stage='completed' WHERE id='usage-run'`)
	for i, duration := range []int64{1000, 1500} {
		if err := f.s.store.SaveTelemetry(ctx, domain.ReasoningTelemetry{ID: fmt.Sprintf("usage-plan-%d", i), RunID: "usage-run", Phase: "acquisition_planning", Provider: "offline", Model: "model-a", Effort: "high", ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental", DurationMS: duration, CallerLatencyMS: duration + 100, QueueWaitMS: 10, ProviderExecutionMS: duration - 100, ResponseTotalMS: duration, Status: "completed", InputTokens: httpUsageToken(100), CachedInputTokens: httpUsageToken(60), OutputTokens: httpUsageToken(20), ReasoningOutputTokens: httpUsageToken(5), CreatedAt: time.Now().UTC().Add(time.Duration(i-2) * time.Hour).Format(time.RFC3339Nano)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.store.SaveTelemetry(ctx, domain.ReasoningTelemetry{ID: "usage-evaluation", RunID: "usage-run", Phase: "candidate_evaluation", Provider: "offline", Model: "model-b", Effort: "low", ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental", DurationMS: 2000, Status: "failed", CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.store.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{SessionID: "usage", Status: "completed", Provider: "offline", Model: "model-a", Effort: "high", DurationMS: 3000, CallerLatencyMS: 3000, QueueWaitMS: 20, ProviderExecutionMS: 2500, ResponseTotalMS: 2600, Usage: domain.ModelUsage{Input: httpUsageToken(200), CachedInput: httpUsageToken(150), Output: httpUsageToken(40), ReasoningOutput: httpUsageToken(10), ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental"}, CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	job, err := f.s.store.CreateAIDetectionJob(ctx, domain.AIDetectionJob{SessionID: "usage", Provider: "offline", Model: "model-a", Effort: "high", CandidateCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.s.store.StartAIDetectionJob(ctx, job.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.store.FinishAIDetectionJob(ctx, job.ID, "completed", 4000, domain.ModelUsage{Input: httpUsageToken(300), CachedInput: httpUsageToken(200), Output: httpUsageToken(60), ReasoningOutput: httpUsageToken(15), ModelDescriptorVersion: "descriptor-1", ModelMaturity: "experimental", CallerLatencyMS: 4000, QueueWaitMS: 30, ProviderExecutionMS: 3500, ResponseTotalMS: 3600}, nil); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		category                        string
		input, cached, output, duration int64
	}{{"living_topic_routing", 80, 20, 10, 250}, {"living_topic_understanding", 120, 40, 30, 500}} {
		if err := f.s.store.RecordLivingTopicModelInvocation(ctx, item.category, "usage-owner", "completed", "offline", "topic-model", "low", time.Duration(item.duration)*time.Millisecond, domain.ModelUsage{Input: httpUsageToken(item.input), CachedInput: httpUsageToken(item.cached), Output: httpUsageToken(item.output), ReasoningOutput: httpUsageToken(0)}); err != nil {
			t.Fatal(err)
		}
	}
	return createdAt
}

func (f httpModelUsageFixture) report(t *testing.T, w *httptest.ResponseRecorder, scope string) domain.ModelUsageReport {
	t.Helper()
	payload := f.json(t, w, 200)
	var report domain.ModelUsageReport
	var fields map[string]json.RawMessage
	if len(payload) != 1 || json.Unmarshal(payload["usage"], &report) != nil || json.Unmarshal(payload["usage"], &fields) != nil || len(fields) != 11 || report.Scope != scope || report.Categories == nil || report.To != report.GeneratedAt {
		t.Fatal("model usage envelope changed", w.Body)
	}
	want := []string{"acquisition_planning", "candidate_evaluation", "semantic_event_resolution", "ai_deep_detection"}
	if scope == "aggregate" {
		want = append(want, "living_topic_routing", "living_topic_understanding")
		if fields["sessionId"] != nil || report.SessionID != "" || fields["windowDays"] == nil {
			t.Fatal("aggregate field projection changed", w.Body)
		}
	} else if report.SessionID != "usage" || fields["windowDays"] != nil {
		t.Fatal("session field projection changed", w.Body)
	}
	if len(report.Categories) != len(want) {
		t.Fatal("category count changed", report)
	}
	for i, category := range report.Categories {
		if category.ID != want[i] || category.Entries == nil {
			t.Fatal("category order/array changed", report)
		}
	}
	return report
}

func (f httpModelUsageFixture) get(t *testing.T, target, scope string) domain.ModelUsageReport {
	t.Helper()
	before := f.snapshot(t)
	started := time.Now()
	report := f.report(t, f.serve(sessionUpdatesRequest("GET", target, "")), scope)
	finished := time.Now()
	generated, err := time.Parse(time.RFC3339Nano, report.GeneratedAt)
	if err != nil || generated.Before(started) || generated.After(finished) {
		t.Fatal("generatedAt outside request", report.GeneratedAt, err)
	}
	from, err := time.Parse(time.RFC3339Nano, report.From)
	if err != nil {
		t.Fatal(err)
	}
	if scope == "aggregate" && (from.Before(started.UTC().AddDate(0, 0, -report.WindowDays)) || from.After(finished.UTC().AddDate(0, 0, -report.WindowDays))) {
		t.Fatal("aggregate time window changed", report)
	}
	if !reflect.DeepEqual(before, f.snapshot(t)) {
		t.Fatal("usage read changed durable state")
	}
	return report
}

func TestHTTPModelUsageCharacterizationProjection(t *testing.T) {
	f := newHTTPModelUsageFixture(t)
	createdAt := f.data(t)
	session := f.get(t, httpSessionUsagePath, "session")
	if session.From != createdAt || session.SessionCount != 1 || session.WindowDays != 0 || session.Status != "completed" || session.DurationMS != 11500 || session.UsageCoverage != "partial" {
		t.Fatalf("session report=%+v", session)
	}
	for _, counter := range []struct {
		got  *int64
		want int64
	}{{session.Usage.Input, 700}, {session.Usage.CachedInput, 470}, {session.Usage.Output, 140}, {session.Usage.ReasoningOutput, 35}} {
		if counter.got == nil || *counter.got != counter.want {
			t.Fatalf("usage counters=%+v", session.Usage)
		}
	}
	if session.Usage.CallerLatencyMS != 11700 || session.Usage.QueueWaitMS != 70 || session.Usage.ProviderExecutionMS != 8300 || session.Usage.ResponseTotalMS != 8700 || session.Usage.ModelDescriptorVersion != "descriptor-1" || session.Usage.ModelMaturity != "experimental" {
		t.Fatal("receipt metadata/timing changed", session.Usage)
	}
	plan := session.Categories[0]
	if len(plan.Entries) != 2 || plan.InvocationCount != 2 || plan.Entries[0].ID != "usage-plan-0" || plan.Entries[1].ID != "usage-plan-1" || plan.UsageCoverage != "complete" || session.Categories[1].UsageCoverage != "unavailable" || session.Categories[1].Status != "failed" {
		t.Fatal("session entries/coverage changed", session.Categories)
	}
	aggregate := f.get(t, httpAggregateUsagePath+"?windowDays=30", "aggregate")
	if aggregate.WindowDays != 30 || aggregate.SessionCount != 1 || aggregate.DurationMS != 12250 || aggregate.UsageCoverage != "partial" {
		t.Fatal("aggregate report changed", aggregate)
	}
	for _, counter := range []struct {
		got  *int64
		want int64
	}{{aggregate.Usage.Input, 900}, {aggregate.Usage.CachedInput, 530}, {aggregate.Usage.Output, 180}, {aggregate.Usage.ReasoningOutput, 35}} {
		if counter.got == nil || *counter.got != counter.want {
			t.Fatal("aggregate double counted breakouts", aggregate.Usage)
		}
	}
	group := aggregate.Categories[0].Entries
	if len(group) != 1 || group[0].InvocationCount != 2 || group[0].CreatedAt != "" || !strings.HasPrefix(group[0].ID, "aggregate:acquisition_planning:") || group[0].DurationMS != 2500 {
		t.Fatal("aggregate grouping changed", group)
	}
	for _, category := range aggregate.Categories[4:] {
		if category.Execution != "async" || category.InvocationCount != 1 || category.UsageCoverage != "complete" {
			t.Fatal("async receipt missing", category)
		}
	}
}

func TestHTTPModelUsageCharacterizationEmptyAndCoverage(t *testing.T) {
	for _, scope := range []string{"session", "aggregate"} {
		for _, mode := range []string{"empty", "zero", "partial", "unavailable", "running"} {
			t.Run(scope+"/"+mode, func(t *testing.T) {
				f := newHTTPModelUsageFixture(t)
				f.seed(t, "usage", "completed", true)
				if mode == "running" {
					f.exec(t, `UPDATE sessions SET status='running'`)
				}
				if mode != "empty" && mode != "running" {
					value := domain.ReasoningTelemetry{ID: "coverage-receipt", RunID: "usage-run", Phase: "candidate_evaluation", Provider: "offline", Model: "model-a", Status: "completed", DurationMS: 10, CreatedAt: domain.Now()}
					if mode == "zero" {
						value.InputTokens, value.CachedInputTokens, value.OutputTokens, value.ReasoningOutputTokens = httpUsageToken(0), httpUsageToken(0), httpUsageToken(0), httpUsageToken(0)
					}
					if mode == "partial" {
						value.InputTokens = httpUsageToken(10)
					}
					if err := f.s.store.SaveTelemetry(context.Background(), value); err != nil {
						t.Fatal(err)
					}
				}
				target := httpSessionUsagePath
				if scope == "aggregate" {
					target = httpAggregateUsagePath
				}
				w := f.serve(sessionUpdatesRequest("GET", target, ""))
				report := f.report(t, w, scope)
				want := map[string]string{"empty": "not_applicable", "running": "not_applicable", "zero": "complete", "partial": "partial", "unavailable": "unavailable"}[mode]
				if report.UsageCoverage != want || report.SessionCount != 1 {
					t.Fatal("coverage/count changed", report)
				}
				var payload struct {
					Usage struct {
						Usage map[string]json.RawMessage `json:"usage"`
					} `json:"usage"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				for _, key := range []string{"inputTokens", "cachedInputTokens", "outputTokens", "reasoningOutputTokens"} {
					want := "null"
					if mode == "zero" {
						want = "0"
					}
					if mode == "partial" && key == "inputTokens" {
						want = "10"
					}
					if string(payload.Usage.Usage[key]) != want {
						t.Fatal("nullable token projection changed", w.Body)
					}
				}
				if mode == "empty" || mode == "running" {
					for _, category := range report.Categories {
						want := "not_invoked"
						if mode == "running" {
							want = "pending"
						}
						if category.Status != want || category.InvocationCount != 0 || category.Note == "" || len(category.Entries) != 0 {
							t.Fatal("non-invocation explanation changed", category)
						}
					}
				}
			})
		}
	}
}

func TestHTTPModelUsageCharacterizationWindowParsing(t *testing.T) {
	for _, tc := range []struct {
		query        string
		days, status int
	}{
		{"", 30, 200}, {"?windowDays=7", 7, 200}, {"?windowDays=30", 30, 200}, {"?windowDays=90", 90, 200},
		{"?windowDays=0", 7, 200}, {"?windowDays=-9", 7, 200}, {"?windowDays=91", 90, 200}, {"?windowDays=999", 90, 200},
		{"?windowDays=bad", 30, 200}, {"?windowDays=", 30, 200}, {"?windowDays=%2030%20", 30, 200}, {"?windowDays=999999999999999999999999", 30, 200},
		{"?windowDays=8", 0, 400}, {"?windowDays=31", 0, 400}, {"?windowDays=8&windowDays=30", 0, 400},
		{"?windowDays=7&windowDays=8", 7, 200}, {"?windowDays=%2B7", 7, 200}, {"?WindowDays=7&extra=ignored", 30, 200},
	} {
		t.Run(tc.query, func(t *testing.T) {
			f := newHTTPModelUsageFixture(t)
			if tc.status == 400 {
				before := f.snapshot(t)
				f.error(t, f.serve(sessionUpdatesRequest("GET", httpAggregateUsagePath+tc.query, "")), 400, "invalid_request", "windowDays must be 7, 30, or 90")
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("invalid window mutated state")
				}
				return
			}
			report := f.get(t, httpAggregateUsagePath+tc.query, "aggregate")
			if report.WindowDays != tc.days || report.SessionCount != 0 || report.UsageCoverage != "not_applicable" {
				t.Fatal("window/default/empty aggregate changed", report)
			}
		})
	}
}

func TestHTTPModelUsageCharacterizationWindowMembership(t *testing.T) {
	f := newHTTPModelUsageFixture(t)
	f.data(t)
	now := time.Now().UTC()
	for _, item := range []struct {
		id     string
		days   int
		tokens int64
	}{{"old", 120, 5000}, {"middle", 15, 50}} {
		f.seed(t, item.id, "completed", true)
		f.exec(t, `UPDATE sessions SET created_at=? WHERE id=?`, now.AddDate(0, 0, -item.days).Format(time.RFC3339Nano), item.id)
		// Session-backed usage is filtered by session creation, not receipt time.
		receiptAt := now.Format(time.RFC3339Nano)
		if item.id == "middle" {
			receiptAt = now.AddDate(0, 0, -400).Format(time.RFC3339Nano)
		}
		f.telemetry(t, item.id+"-receipt", item.id+"-run", "acquisition_planning", httpUsageToken(item.tokens), receiptAt)
	}
	if err := f.s.store.RecordLivingTopicModelInvocation(context.Background(), "living_topic_routing", "old-owner", "completed", "offline", "old-topic-model", "low", time.Millisecond, domain.ModelUsage{Input: httpUsageToken(9000)}); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE living_topic_model_invocations SET created_at=? WHERE model='old-topic-model'`, now.AddDate(0, 0, -120).Format(time.RFC3339Nano))
	for _, days := range []int{7, 30, 90} {
		t.Run(fmt.Sprint(days), func(t *testing.T) {
			report := f.get(t, fmt.Sprintf("%s?windowDays=%d", httpAggregateUsagePath, days), "aggregate")
			want, count := int64(900), 1
			if days > 7 {
				want, count = 950, 2
			}
			if report.SessionCount != count || report.Usage.Input == nil || *report.Usage.Input != want {
				t.Fatal("window membership changed", report)
			}
		})
	}
}

func TestHTTPModelUsageCharacterizationRouting(t *testing.T) {
	for _, scope := range []string{"session", "aggregate"} {
		for _, mode := range []string{"canonical", "slash", "encoded_slash", "double_slash", "nested_id", "wrong_method", "foreign_host", "foreign_origin", "preflight"} {
			t.Run(scope+"/"+mode, func(t *testing.T) {
				f := newHTTPModelUsageFixture(t)
				f.seed(t, "usage", "completed", false)
				target := httpSessionUsagePath
				if scope == "aggregate" {
					target = httpAggregateUsagePath
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
					if scope == "session" {
						target = "/api/inbox/sessions/nested/usage/model-usage"
					} else {
						target += "/nested"
					}
					status = 404
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
				}
				before := f.snapshot(t)
				w := f.serve(r)
				if w.Code != status {
					t.Fatalf("status=%d want=%d body=%s", w.Code, status, w.Body)
				}
				if status == 200 {
					f.report(t, w, scope)
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
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("usage route changed state")
				}
			})
		}
	}
}

type httpModelUsageBody struct {
	io.Reader
	reads, closes int
}

func (b *httpModelUsageBody) Read(p []byte) (int, error) { b.reads++; return b.Reader.Read(p) }
func (b *httpModelUsageBody) Close() error               { b.closes++; return nil }

func TestHTTPModelUsageCharacterizationContextAndFailures(t *testing.T) {
	for _, scope := range []string{"session", "aggregate"} {
		for _, mode := range []string{"ignored_body", "cancelled", "deadline", "closed_store", "late_query_failure", "write_failure"} {
			t.Run(scope+"/"+mode, func(t *testing.T) {
				f := newHTTPModelUsageFixture(t)
				f.data(t)
				target := httpSessionUsagePath
				if scope == "aggregate" {
					target = httpAggregateUsagePath
				}
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
				case "late_query_failure":
					f.exec(t, `UPDATE reasoning_invocations SET duration_ms='invalid-duration' WHERE id='usage-plan-0'`)
				}
				before := f.snapshot(t)
				if mode == "ignored_body" {
					body := &httpModelUsageBody{Reader: strings.NewReader("{invalid")}
					r.Body = body
					r.Header.Set("Content-Type", "text/plain")
					f.report(t, f.serve(r), scope)
					if body.reads != 0 || body.closes != 0 {
						t.Fatal("GET body was consumed")
					}
				} else if mode == "write_failure" {
					failure := errors.New("offline-model-usage-response-failed")
					w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
					if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != 200 {
						t.Fatal("response error changed", w.status, err)
					}
				} else {
					f.error(t, f.serve(r), 500, "internal_error", "internal server error")
				}
				if !reflect.DeepEqual(before, f.snapshot(t)) {
					t.Fatal("failed or ignored-body read changed state")
				}
			})
		}
	}
}

func TestHTTPModelUsageCharacterizationErrorPrecedence(t *testing.T) {
	for _, mode := range []string{"missing_session", "nested_id", "empty_id", "invalid_window_cancelled", "invalid_window_closed_store", "invalid_id_cancelled"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPModelUsageFixture(t)
			target := "/api/inbox/sessions/missing/model-usage"
			status, code, message := 404, "not_found", "session not found"
			switch mode {
			case "nested_id", "invalid_id_cancelled":
				target = "/api/inbox/sessions/nested/missing/model-usage"
			case "empty_id":
				target = "/api/inbox/sessions//model-usage"
			case "invalid_window_cancelled", "invalid_window_closed_store":
				target = httpAggregateUsagePath + "?windowDays=8"
				status, code, message = 400, "invalid_request", "windowDays must be 7, 30, or 90"
			}
			r := sessionUpdatesRequest("GET", target, "")
			if strings.HasSuffix(mode, "cancelled") {
				ctx, cancel := context.WithCancel(r.Context())
				cancel()
				r = r.WithContext(ctx)
			}
			if mode == "invalid_window_closed_store" {
				if err := f.s.store.Close(); err != nil {
					t.Fatal(err)
				}
			}
			before := f.snapshot(t)
			if mode == "empty_id" {
				err := f.s.route(httptest.NewRecorder(), r)
				value, ok := err.(apiError)
				if !ok || value.Status != 404 || value.Message != message {
					t.Fatal("empty ID validation changed", err)
				}
			} else {
				f.error(t, f.serve(r), status, code, message)
			}
			if !reflect.DeepEqual(before, f.snapshot(t)) {
				t.Fatal("invalid request changed state")
			}
		})
	}
}
