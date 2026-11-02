package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// These tests exercise the HTTP boundary with the real Engine and disposable Store.
// Fixtures omit captureLeaseId so claiming never starts an OS native observer.
func bridgeCommandsFixture(t *testing.T, payload map[string]any) (*Server, string, domain.Run, domain.BridgeCommand) {
	t.Helper()
	s, token := splitTestServer(t)
	t.Cleanup(func() {
		s.engine.Shutdown()
		if !s.engine.WaitForIdle(3 * time.Second) {
			t.Error("fixture workers did not stop")
		}
	})
	ctx := context.Background()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	session, err := s.store.CreateUpdateSession(ctx, "HTTP command characterization", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	if payload == nil {
		payload = map[string]any{"source": string(run.Source)}
	}
	command, err := s.store.StartRun(ctx, run.ID, payload)
	if err != nil {
		t.Fatal(err)
	}
	return s, token, run, command
}

func bridgeCommandsRequest(s *Server, token, method, target, body string) *http.Request {
	r := httptest.NewRequest(method, "http://127.0.0.1:11122"+target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", token)
	r.Header.Set("X-Aku-Capture-Instance", s.splitCapture.key)
	r.Header.Set("X-Aku-Bridge-Id", "http-command-characterization")
	return r
}

func bridgeCommandsServe(s *Server, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	s.api().ServeHTTP(w, r)
	return w
}

func bridgeCommandsJSON(t *testing.T, w *httptest.ResponseRecorder, status int) map[string]json.RawMessage {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status=%d, want %d: %s", w.Code, status, w.Body)
	}
	if w.Header().Get("Content-Type") != "application/json; charset=utf-8" || w.Header().Get("Cache-Control") != "no-store, max-age=0" || w.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("JSON/cache headers changed: %v", w.Header())
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func bridgeCommandsError(t *testing.T, w *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	value := bridgeCommandsJSON(t, w, status)
	var gotCode, gotMessage string
	_ = json.Unmarshal(value["error"], &gotCode)
	_ = json.Unmarshal(value["message"], &gotMessage)
	if len(value) != 3 || gotCode != code || !strings.Contains(gotMessage, message) || string(value["details"]) != "null" {
		t.Fatalf("error envelope changed: %s", w.Body)
	}
}

func bridgeCommandsUnchanged(t *testing.T, s *Server, before domain.Run) {
	t.Helper()
	after, err := s.store.GetRun(context.Background(), before.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected request mutated run: before=%+v after=%+v err=%v", before, after, err)
	}
	observations, err := s.store.Observations(context.Background(), before.ID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("rejected request persisted observations: %v %v", observations, err)
	}
}

func TestBridgeCommandsCharacterizationAuthentication(t *testing.T) {
	for _, endpoint := range []string{"next", "pending", "observation", "failure"} {
		for _, guard := range []string{"contract", "token", "capture"} {
			t.Run(endpoint+"/"+guard, func(t *testing.T) {
				s, token, run, command := bridgeCommandsFixture(t, nil)
				method, target := http.MethodGet, "/api/bridge/commands/"+endpoint
				if endpoint == "observation" || endpoint == "failure" {
					method, target = http.MethodPost, "/api/bridge/commands/"+command.ID+"/"+endpoint
				}
				before, _ := s.store.GetRun(context.Background(), run.ID)
				r := bridgeCommandsRequest(s, token, method, target, "{")
				// Every request also violates lower-priority guards or input parsing.
				r.Header.Set("X-Aku-Capture-Instance", "stale")
				switch guard {
				case "contract":
					r.Header.Set("X-Aku-Bridge-Contract", "wrong")
					r.Header.Set("X-Aku-Bridge-Token", "wrong")
					bridgeCommandsError(t, bridgeCommandsServe(s, r), 401, "invalid_bridge_contract", "unsupported Bridge contract")
				case "token":
					r.Header.Set("X-Aku-Bridge-Token", "wrong")
					bridgeCommandsError(t, bridgeCommandsServe(s, r), 401, "invalid_bridge_token", "invalid Bridge token")
				case "capture":
					bridgeCommandsError(t, bridgeCommandsServe(s, r), 409, "capture_instance_mismatch", "active Windows capture process")
				}
				bridgeCommandsUnchanged(t, s, before)
			})
		}
	}
}

func TestBridgeCommandsCharacterizationPolling(t *testing.T) {
	s, token := splitTestServer(t)
	for _, target := range []string{"/api/bridge/commands/pending", "/api/bridge/commands/pending/"} {
		w := bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "GET", target, ""))
		if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
			t.Fatalf("empty poll: %d %s %v", w.Code, w.Body, w.Header())
		}
	}
	bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "GET", "/api/bridge/commands/next", "")), 400, "invalid_request", "runId is required")
	for _, id := range []string{"missing", "%20"} {
		bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "GET", "/api/bridge/commands/next?runId="+id, "")), 500, "internal_error", "internal server error")
	}

	s, token, run, command := bridgeCommandsFixture(t, nil)
	value := bridgeCommandsJSON(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "GET", "/api/bridge/commands/pending", "")), 200)
	if len(value) != 1 || string(value["runId"]) != `"`+run.ID+`"` {
		t.Fatalf("pending envelope: %s", value)
	}
	value = bridgeCommandsJSON(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "GET", "/api/bridge/commands/next/?runId="+run.ID, "")), 200)
	var claimed domain.BridgeCommand
	if err := json.Unmarshal(value["command"], &claimed); err != nil || len(value) != 1 || claimed.ID != command.ID || claimed.RunID != run.ID || claimed.Status != "claimed" || claimed.ClaimedAt == nil {
		t.Fatalf("claim envelope: %s, %v", value, err)
	}
	w := bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "GET", "/api/bridge/commands/next?runId="+run.ID, ""))
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Fatalf("repeat claim: %d %s", w.Code, w.Body)
	}
}

func TestBridgeCommandsCharacterizationParsing(t *testing.T) {
	for _, endpoint := range []string{"observation", "failure"} {
		for _, tc := range []struct {
			name, contentType, body, message string
			status                           int
		}{
			{"media", "text/plain", "{}", "Content-Type must be application/json", 415},
			{"malformed", "application/json", "{", "request body must be valid JSON:", 400},
			{"unknown", "application/json", `{"unexpected":true}`, `unknown field "unexpected"`, 400},
			{"oversize", "application/json", `{"runId":"` + strings.Repeat("a", 1_000_002) + `"}`, "unexpected EOF", 400},
			// The existing decoder reads one value; this is not an EOF-validation test.
			{"trailing", "application/json; charset=utf-8", `{"runId":"missing"} {}`, "sql: no rows", 400},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				s, token := splitTestServer(t)
				r := bridgeCommandsRequest(s, token, "POST", "/api/bridge/commands/missing/"+endpoint, tc.body)
				r.Header.Set("Content-Type", tc.contentType)
				code := "invalid_request"
				if tc.status == 415 {
					code = "unsupported_media_type"
				}
				bridgeCommandsError(t, bridgeCommandsServe(s, r), tc.status, code, tc.message)
			})
		}
	}
}

func bridgeCommandsObservation(run domain.Run) domain.Observation {
	return domain.Observation{Source: run.Source, PageURL: "https://www.linkedin.com/feed/", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: "1234567890123456789", Text: "Offline HTTP characterization evidence", Author: "fixture", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456789", FeedPosition: 1}}}}, Coverage: map[string]any{"quality": "complete"}}
}

func TestBridgeCommandsCharacterizationResults(t *testing.T) {
	for _, endpoint := range []string{"observation", "failure"} {
		for _, claim := range []bool{false, true} {
			name := endpoint + "/queued"
			if claim {
				name = endpoint + "/claimed"
			}
			t.Run(name, func(t *testing.T) {
				s, token, run, command := bridgeCommandsFixture(t, nil)
				if claim {
					claimed, err := s.store.ClaimCommand(context.Background(), run.ID, "fixture")
					if err != nil || claimed == nil {
						t.Fatalf("claim: %v %v", claimed, err)
					}
				}
				body := map[string]any{"runId": run.ID}
				status := 200
				if endpoint == "observation" {
					body["observation"], status = bridgeCommandsObservation(run), 202
				} else {
					body["error"] = domain.Failure{Message: "fixture failure"}
				}
				raw, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				// The extra path component and trailing slash lock existing path.Base routing.
				target := "/api/bridge/commands/extra/" + command.ID + "/" + endpoint + "/"
				before, _ := s.store.GetRun(context.Background(), run.ID)
				w := bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "POST", target, string(raw)))
				if !claim {
					bridgeCommandsError(t, w, 400, "invalid_request", "bridge command is not")
					bridgeCommandsUnchanged(t, s, before)
					return
				}
				value := bridgeCommandsJSON(t, w, status)
				var result domain.Run
				if err := json.Unmarshal(value["run"], &result); err != nil || len(value) != 1 || result.ID != run.ID {
					t.Fatalf("run envelope: %s %v", value, err)
				}
				if endpoint == "failure" {
					if result.Status != "failed" || result.Error == nil || result.Error.Code != "bridge_failure" || result.Error.Stage != "capture" || result.Error.Message != "fixture failure" {
						t.Fatalf("failure defaults: %+v", result)
					}
				} else {
					observations, err := s.store.Observations(context.Background(), run.ID)
					if err != nil || len(observations) != 1 || observations[0].Source != run.Source {
						t.Fatalf("durable observation: %v %v", observations, err)
					}
					waitHTTPTestSession(t, s.engine, run.SessionID, "completed")
				}
				bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "POST", target, string(raw))), 400, "invalid_request", "bridge command is not")
			})
		}
	}
}

type bridgeCommandsCancelBody struct {
	io.Reader
	cancel context.CancelFunc
}

func (b bridgeCommandsCancelBody) Close() error { b.cancel(); return nil }

func TestBridgeCommandsCharacterizationContext(t *testing.T) {
	for _, endpoint := range []string{"next", "pending", "observation", "failure"} {
		t.Run(endpoint+"/canceled-auth", func(t *testing.T) {
			s, token := splitTestServer(t)
			method, target := "GET", "/api/bridge/commands/"+endpoint
			if endpoint == "observation" || endpoint == "failure" {
				method, target = "POST", "/api/bridge/commands/missing/"+endpoint
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			r := bridgeCommandsRequest(s, token, method, target, "{}")
			bridgeCommandsError(t, bridgeCommandsServe(s, r.WithContext(ctx)), 401, "invalid_bridge_token", "invalid Bridge token")
		})
	}
	for _, endpoint := range []string{"observation", "failure"} {
		t.Run(endpoint+"/canceled-after-decode", func(t *testing.T) {
			s, token, run, command := bridgeCommandsFixture(t, nil)
			before, _ := s.store.GetRun(context.Background(), run.ID)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := bridgeCommandsRequest(s, token, "POST", "/api/bridge/commands/"+command.ID+"/"+endpoint, "")
			r.Body = bridgeCommandsCancelBody{strings.NewReader(`{"runId":"` + run.ID + `"}`), cancel}
			bridgeCommandsError(t, bridgeCommandsServe(s, r.WithContext(ctx)), 400, "invalid_request", "context canceled")
			bridgeCommandsUnchanged(t, s, before)
		})
	}
}

func TestBridgeCommandsCharacterizationRouting(t *testing.T) {
	s, token := splitTestServer(t)
	for _, tc := range []struct{ method, target string }{
		{"POST", "/api/bridge/commands/next"}, {"POST", "/api/bridge/commands/pending"},
		{"GET", "/api/bridge/commands/id/observation"}, {"GET", "/api/bridge/commands/id/failure"},
		{"PUT", "/api/bridge/commands/id/observation"}, {"DELETE", "/api/bridge/commands/id/failure"},
		{"GET", "/api/bridge/commands/next//"}, {"POST", "/api/bridge/commands/id/failure//"},
	} {
		t.Run(tc.method+tc.target, func(t *testing.T) {
			bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, tc.method, tc.target, "{}")), 404, "not_found", "route not found")
		})
	}
}

func TestBridgeCommandsCharacterizationStaleOwner(t *testing.T) {
	s, token, run, command := bridgeCommandsFixture(t, map[string]any{"captureRuntime": map[string]any{"driver": "browser", "generation": 999, "epoch": "stale"}})
	if claimed, err := s.store.ClaimCommand(context.Background(), run.ID, "fixture"); err != nil || claimed == nil {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	before, _ := s.store.GetRun(context.Background(), run.ID)
	raw, err := json.Marshal(map[string]any{"runId": run.ID, "observation": bridgeCommandsObservation(run)})
	if err != nil {
		t.Fatal(err)
	}
	bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "POST", "/api/bridge/commands/"+command.ID+"/observation", string(raw))), 400, "invalid_request", "capture command belongs to another runtime owner")
	bridgeCommandsUnchanged(t, s, before)
}

func TestBridgeCommandsCharacterizationRejectedResults(t *testing.T) {
	for _, endpoint := range []string{"observation", "failure"} {
		for _, missing := range []string{"run", "command"} {
			t.Run(endpoint+"/missing-"+missing, func(t *testing.T) {
				s, token, run, command := bridgeCommandsFixture(t, nil)
				before, err := s.store.GetRun(context.Background(), run.ID)
				if err != nil {
					t.Fatal(err)
				}
				runID, commandID := run.ID, command.ID
				if missing == "run" {
					runID = "other-run"
				} else {
					commandID = "other-command"
				}
				raw, err := json.Marshal(map[string]any{"runId": runID})
				if err != nil {
					t.Fatal(err)
				}
				bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "POST", "/api/bridge/commands/"+commandID+"/"+endpoint, string(raw))), 400, "invalid_request", "sql: no rows")
				bridgeCommandsUnchanged(t, s, before)
			})
		}
	}
	t.Run("observation/source-mismatch", func(t *testing.T) {
		s, token, run, command := bridgeCommandsFixture(t, nil)
		claimed, err := s.store.ClaimCommand(context.Background(), run.ID, "fixture")
		if err != nil || claimed == nil {
			t.Fatalf("claim: %v %v", claimed, err)
		}
		before, err := s.store.GetRun(context.Background(), run.ID)
		if err != nil {
			t.Fatal(err)
		}
		observation := bridgeCommandsObservation(run)
		observation.Source = domain.SourceX
		raw, err := json.Marshal(map[string]any{"runId": run.ID, "observation": observation})
		if err != nil {
			t.Fatal(err)
		}
		bridgeCommandsError(t, bridgeCommandsServe(s, bridgeCommandsRequest(s, token, "POST", "/api/bridge/commands/"+command.ID+"/observation", string(raw))), 400, "invalid_request", "observation source does not match run")
		bridgeCommandsUnchanged(t, s, before)
	})
}
