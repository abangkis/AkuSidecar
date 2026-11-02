package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

type bridgeActionsFixture struct {
	s     *Server
	token string
}

func newBridgeActionsFixture(t *testing.T) bridgeActionsFixture {
	t.Helper()
	s, token := splitTestServer(t)
	return bridgeActionsFixture{s, token}
}

func (f bridgeActionsFixture) request(method, target, body string) *http.Request {
	r := sessionUpdatesRequest(method, target, body)
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", f.token)
	if f.s.splitCapture != nil {
		r.Header.Set("X-Aku-Capture-Instance", f.s.splitCapture.key)
	}
	return r
}

func (f bridgeActionsFixture) serve(r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.s.security(f.s.api()).ServeHTTP(w, r)
	return w
}

func (f bridgeActionsFixture) action(t *testing.T, w *httptest.ResponseRecorder, status int) engine.ReloadAction {
	t.Helper()
	payload := (sessionUpdatesFixture{s: f.s}).json(t, w, status)
	var fields map[string]json.RawMessage
	if len(payload) != 1 || json.Unmarshal(payload["action"], &fields) != nil {
		t.Fatal("action envelope changed", w.Body)
	}
	keys := strings.Fields("id requestId type actor reason status createdAt expiresAt deliveredAt acceptedAt heartbeatObservedAt completedAt previousBuildId expectedBuildId observedBuildId expectedSurfacePolicy observedSurfacePolicy errorCategory message")
	if len(fields) != len(keys) {
		t.Fatal("action field count changed", len(fields))
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			t.Fatal("missing action field", key)
		}
	}
	var action engine.ReloadAction
	if err := json.Unmarshal(payload["action"], &action); err != nil {
		t.Fatal(err)
	}
	return action
}

func (f bridgeActionsFixture) seed(t *testing.T) engine.ReloadAction {
	t.Helper()
	action, err := f.s.engine.RequestBridgeReload("characterization", nil, "Offline reload characterization")
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func TestBridgeActionsCharacterizationLifecycle(t *testing.T) {
	for _, split := range []bool{true, false} {
		t.Run(map[bool]string{true: "split", false: "non_split"}[split], func(t *testing.T) {
			f := newBridgeActionsFixture(t)
			if !split {
				f.s.splitCapture = nil
			}
			heartbeat := engine.ExpectedHeartbeat()
			heartbeat.BuildID = "previous-build"
			f.s.engine.RecordHeartbeat(heartbeat)
			body := `{"requestId":"  lifecycle  ","actor":{"kind":"ui"},"reason":"  reload fixture  "}`
			created := f.action(t, f.serve(f.request("POST", "/api/operations/bridge/actions/reload-self/", body)), 202)
			if created.Status != "pending" || created.RequestID != "lifecycle" || created.Reason != "reload fixture" || created.PreviousBuildID != "previous-build" || created.ExpectedBuildID != engine.ExpectedBridgeBuildID || created.ExpectedSurfacePolicy != engine.ExpectedBridgeSurfacePolicyFingerprint || created.DeliveredAt != nil || created.AcceptedAt != nil || created.CompletedAt != nil || created.CreatedAt == "" || created.ExpiresAt == "" || created.Type != "reload_self" {
				t.Fatal("reload request contract changed", created)
			}
			for _, conflict := range []string{
				`{"requestId":"lifecycle","actor":{"kind":"other"},"reason":"reload fixture"}`,
				`{"requestId":"lifecycle","actor":{"kind":"ui"},"reason":"different"}`,
				`{"requestId":"different","reason":"reload fixture"}`,
			} {
				bridgeCommandsError(t, f.serve(f.request("POST", "/api/operations/bridge/actions/reload-self", conflict)), 409, "conflict", "bridge action conflict")
			}
			replay := f.action(t, f.serve(f.request("POST", "/api/operations/bridge/actions/reload-self", body)), 202)
			if !reflect.DeepEqual(created, replay) {
				t.Fatal("idempotent replay changed the action")
			}
			accept := "/api/operations/bridge/actions/nested/" + created.ID + "/accept/"
			bridgeCommandsError(t, f.serve(f.request("POST", accept, "")), 409, "conflict", "bridge action conflict")
			delivered := f.action(t, f.serve(f.request("GET", "/api/operations/bridge/actions/next/?waitMs=999999", "")), 200)
			if delivered.ID != created.ID || delivered.Status != "delivered" || delivered.DeliveredAt == nil {
				t.Fatal("delivery contract changed", delivered)
			}
			detail := "/api/operations/bridge/actions/nested/" + created.ID + "/"
			if got := f.action(t, f.serve(f.request("GET", detail, "")), 200); !reflect.DeepEqual(delivered, got) {
				t.Fatal("detail projection changed")
			}
			accepted := f.action(t, f.serve(f.request("POST", accept, "ignored non-JSON body")), 202)
			if accepted.Status != "accepted" || accepted.AcceptedAt == nil || accepted.CompletedAt != nil {
				t.Fatal("acceptance contract changed", accepted)
			}
			if again := f.action(t, f.serve(f.request("POST", accept, "")), 202); !reflect.DeepEqual(accepted, again) {
				t.Fatal("repeated acceptance changed state")
			}
			for _, hb := range []domain.BridgeHeartbeat{{BuildID: "wrong"}, {BuildID: engine.ExpectedBridgeBuildID}} {
				f.s.engine.RecordHeartbeat(hb)
				if got := f.action(t, f.serve(f.request("GET", detail, "")), 200); got.Status != "accepted" || got.CompletedAt != nil {
					t.Fatal("reload completed without the expected executing policy")
				}
			}
			f.s.engine.RecordHeartbeat(engine.ExpectedHeartbeat())
			completed := f.action(t, f.serve(f.request("GET", detail, "")), 200)
			if completed.Status != "completed" || completed.CompletedAt == nil || completed.HeartbeatObservedAt == nil {
				t.Fatal("matching heartbeat did not complete reload")
			}
			if got := f.action(t, f.serve(f.request("POST", accept, "")), 202); !reflect.DeepEqual(completed, got) {
				t.Fatal("completed acceptance was not idempotent")
			}
			f.action(t, f.serve(f.request("POST", "/api/operations/bridge/actions/reload-self", `{"requestId":"second","reason":"second fixture"}`)), 202)
			if got := f.action(t, f.serve(f.request("GET", detail, "")), 200); !reflect.DeepEqual(completed, got) {
				t.Fatal("retained action status was lost")
			}
			bridgeCommandsError(t, f.serve(f.request("POST", accept, "")), 404, "not_found", "bridge action not found")
		})
	}
}

func TestBridgeActionsCharacterizationRequestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, contentType, message string
		status                           int
	}{
		{"malformed", `{`, "application/json", "request body must be valid JSON", 400},
		{"unknown", `{"requestId":"id","reason":"reload","unknown":1}`, "application/json", "unknown field", 400},
		{"type", `{"requestId":1,"reason":"reload"}`, "application/json", "cannot unmarshal", 400},
		{"missing_id", `{"reason":"reload"}`, "application/json", "requestId is required and bounded", 400},
		{"missing_reason", `{"requestId":"id"}`, "application/json", "reason is required and bounded", 400},
		{"long_id", `{"requestId":"` + strings.Repeat("x", 129) + `","reason":"reload"}`, "application/json", "requestId is required and bounded", 400},
		{"long_reason", `{"requestId":"id","reason":"` + strings.Repeat("x", 501) + `"}`, "application/json", "reason is required and bounded", 400},
		{"mime", `{}`, "text/plain", "Content-Type must be application/json", 415},
		{"trailing_json", `{"requestId":"id","reason":"reload"} {"ignored":true}`, "application/json", "", 202},
		{"scalar_actor", `{"requestId":"id","reason":"reload","actor":7}`, "application/json; charset=utf-8", "", 202},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBridgeActionsFixture(t)
			r := f.request("POST", "/api/operations/bridge/actions/reload-self", tc.body)
			r.Header.Set("Content-Type", tc.contentType)
			w := f.serve(r)
			if tc.status == 202 {
				f.action(t, w, 202)
				return
			}
			code := "invalid_request"
			if tc.status == 415 {
				code = "unsupported_media_type"
			}
			bridgeCommandsError(t, w, tc.status, code, tc.message)
			if action, err := f.s.engine.NextBridgeAction(0, nil); err != nil || action != nil {
				t.Fatal("rejected request created an action", err)
			}
		})
	}
}

func TestBridgeActionsCharacterizationAuthentication(t *testing.T) {
	for _, endpoint := range []string{"request", "next", "accept", "detail"} {
		for _, credential := range []string{"contract", "token", "no_owner", "stale_owner"} {
			t.Run(endpoint+"/"+credential, func(t *testing.T) {
				f := newBridgeActionsFixture(t)
				action := f.seed(t)
				method, target, body, status := "GET", "/api/operations/bridge/actions/"+action.ID+"/", "", 200
				switch endpoint {
				case "request":
					method, target, body, status = "POST", "/api/operations/bridge/actions/reload-self/", `{"requestId":"characterization","reason":"Offline reload characterization"}`, 202
				case "next":
					target = "/api/operations/bridge/actions/next/"
				case "accept":
					method, target, status = "POST", "/api/operations/bridge/actions/"+action.ID+"/accept%2F", 409
					if _, err := f.s.engine.NextBridgeAction(0, nil); err != nil {
						t.Fatal(err)
					}
				}
				before, _ := f.s.engine.BridgeAction(action.ID)
				r := f.request(method, target, body)
				code := "capture_instance_mismatch"
				if credential == "contract" {
					r.Header.Set("X-Aku-Bridge-Contract", "bad")
					status, code = 401, "invalid_bridge_contract"
				} else if credential == "token" {
					r.Header.Set("X-Aku-Bridge-Token", "bad")
					status, code = 401, "invalid_bridge_token"
				} else {
					r.Header.Set("X-Aku-Capture-Instance", map[string]string{"no_owner": "", "stale_owner": "stale"}[credential])
				}
				w := f.serve(r)
				if status >= 400 {
					bridgeCommandsError(t, w, status, code, "")
					after, err := f.s.engine.BridgeAction(action.ID)
					if err != nil || !reflect.DeepEqual(before, after) {
						t.Fatal("authentication failure mutated action", err)
					}
				} else {
					f.action(t, w, status)
				}
			})
		}
	}
}

func TestBridgeActionsCharacterizationRoutingAndEmptyPoll(t *testing.T) {
	f := newBridgeActionsFixture(t)
	for _, wait := range []string{"", "bad", "-1", "0", "999999999999999999999999"} {
		w := f.serve(f.request("GET", "/api/operations/bridge/actions/next/?waitMs="+wait, ""))
		if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" || w.Header().Get("Cache-Control") != "" || w.Header().Get("Pragma") != "" || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != f.s.engine.Epoch() {
			t.Fatal("empty polling contract changed", wait, w.Code, w.Header())
		}
	}
	for _, tc := range []struct{ method, target, message string }{
		{"POST", "/api/operations/bridge/actions/next", "route not found"},
		{"GET", "/api/operations/bridge/actions/reload-self", "bridge action not found"},
		{"GET", "/api/operations/bridge/actions/missing", "bridge action not found"},
		{"POST", "/api/operations/bridge/actions/missing/accept", "bridge action not found"},
		{"POST", "/api/operations/bridge/actions/missing/accept//", "route not found"},
	} {
		bridgeCommandsError(t, f.serve(f.request(tc.method, tc.target, "")), 404, "not_found", tc.message)
	}
	w := f.serve(f.request("OPTIONS", "/api/operations/bridge/actions/reload-self", ""))
	if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
		t.Fatal("preflight entered the handler")
	}
	for _, boundary := range []string{"host", "origin"} {
		r := f.request("POST", "/api/operations/bridge/actions/reload-self", `{}`)
		if boundary == "host" {
			r.Host = "attacker.test"
		} else {
			r.Header.Set("Origin", "https://attacker.test")
		}
		if w := f.serve(r); w.Code != 403 {
			t.Fatal("foreign host/origin reached reload", boundary, w.Code)
		}
	}
}

func TestBridgeActionsCharacterizationCancellation(t *testing.T) {
	for _, target := range []string{"reload-self", "next?waitMs=30000", "missing/accept", "missing"} {
		t.Run("before-auth/"+target, func(t *testing.T) {
			f := newBridgeActionsFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			method := "GET"
			if target == "reload-self" || target == "missing/accept" {
				method = "POST"
			}
			bridgeCommandsError(t, f.serve(f.request(method, "/api/operations/bridge/actions/"+target, `{}`).WithContext(ctx)), 401, "invalid_bridge_token", "invalid Bridge token")
		})
	}
	t.Run("after-decode", func(t *testing.T) {
		f := newBridgeActionsFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := f.request("POST", "/api/operations/bridge/actions/reload-self", "")
		r.Body = bridgeCommandsCancelBody{strings.NewReader(`{"requestId":"cancelled-after-auth","reason":"offline fixture"}`), cancel}
		action := f.action(t, f.serve(r.WithContext(ctx)), 202)
		if ctx.Err() != context.Canceled || action.Status != "pending" {
			t.Fatal("post-auth in-memory request gained a cancellation check")
		}
	})
	t.Run("long-poll-terminates", func(t *testing.T) {
		f := newBridgeActionsFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			done <- f.serve(f.request("GET", "/api/operations/bridge/actions/next?waitMs=999999", "").WithContext(ctx))
		}()
		select {
		case w := <-done:
			// Cancellation can precede token authentication on a busy machine.
			if w.Code == 401 {
				bridgeCommandsError(t, w, 401, "invalid_bridge_token", "invalid Bridge token")
			} else if w.Code != 204 || w.Body.Len() != 0 {
				t.Fatal("cancelled empty poll changed response", w.Code)
			}
		case <-time.After(3 * time.Second):
			cancel()
			f.seed(t) // Drain a broken waiter without leaving a test goroutine.
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Error("poll waiter did not drain")
			}
			t.Fatal("poll ignored request cancellation")
		}
	})
	t.Run("engine-delivery-priority-control", func(t *testing.T) {
		f := newBridgeActionsFixture(t)
		action := f.seed(t)
		done := make(chan struct{})
		close(done)
		delivered, err := f.s.engine.NextBridgeAction(30*time.Second, done)
		if err != nil || delivered == nil || delivered.ID != action.ID || delivered.Status != "delivered" {
			t.Fatal("pending delivery lost its priority over cancellation", err)
		}
		if empty, err := f.s.engine.NextBridgeAction(30*time.Second, done); err != nil || empty != nil {
			t.Fatal("cancelled empty waiter returned an action", err)
		}
	})
}

func TestBridgeActionsCharacterizationResponseWriteFailure(t *testing.T) {
	for _, stage := range []string{"request", "next", "accept", "detail"} {
		t.Run(stage, func(t *testing.T) {
			f := newBridgeActionsFixture(t)
			method, target, body, wantStatus, wantState := "POST", "/api/operations/bridge/actions/reload-self", `{"requestId":"writer","reason":"offline fixture"}`, 202, "pending"
			var seeded engine.ReloadAction
			if stage != "request" {
				seeded = f.seed(t)
				method, target, body, wantStatus = "GET", "/api/operations/bridge/actions/"+seeded.ID, "", 200
				if stage == "next" {
					target, wantState = "/api/operations/bridge/actions/next", "delivered"
				} else if stage == "accept" {
					if _, err := f.s.engine.NextBridgeAction(0, nil); err != nil {
						t.Fatal(err)
					}
					method, target, wantStatus, wantState = "POST", target+"/accept", 202, "accepted"
				}
			}
			failure := errors.New("reload-action-response-write-failed")
			w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
			if err := f.s.route(w, f.request(method, target, body)); !errors.Is(err, failure) || w.status != wantStatus {
				t.Fatal("response write error contract changed", w.status, err)
			}
			if stage == "request" {
				if delivered, err := f.s.engine.NextBridgeAction(0, nil); err != nil || delivered == nil || delivered.RequestID != "writer" {
					t.Fatal("write failure lost the created action", err)
				}
			} else if after, err := f.s.engine.BridgeAction(seeded.ID); err != nil || after.Status != wantState {
				t.Fatal("write failure rolled back in-memory action state", err)
			}
		})
	}
}
