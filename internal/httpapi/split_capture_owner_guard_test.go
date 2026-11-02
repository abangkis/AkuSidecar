package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func ownerGuardRequest(s *Server, token, key, method, target, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:11122"+target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", token)
	r.Header.Set("X-Aku-Capture-Instance", key)
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, r)
	return w
}

func TestSplitCaptureOwnerGuardReloadAcceptance(t *testing.T) {
	for _, variant := range []string{"canonical", "trailing_slash", "encoded_trailing_slash", "encoded_separator", "query"} {
		for _, owner := range []string{"missing", "stale", "current"} {
			t.Run(variant+"/"+owner, func(t *testing.T) {
				s, token := splitTestServer(t)
				action, err := s.engine.RequestBridgeReload("owner-guard", nil, "Offline owner guard regression")
				if err != nil {
					t.Fatal(err)
				}
				if delivered, err := s.engine.NextBridgeAction(0, nil); err != nil || delivered == nil || delivered.Status != "delivered" {
					t.Fatal("fixture did not deliver the action", err)
				}
				target := "/api/operations/bridge/actions/" + action.ID + "/accept"
				switch variant {
				case "trailing_slash":
					target += "/"
				case "encoded_trailing_slash":
					target += "%2F"
				case "encoded_separator":
					target = "/api/operations/bridge/actions/" + action.ID + "%2Faccept"
				case "query":
					target += "/?ignored=%2F"
				}
				key := map[string]string{"missing": "", "stale": "old-capture-owner", "current": s.splitCapture.key}[owner]
				w := ownerGuardRequest(s, token, key, http.MethodPost, target, "")
				after, err := s.engine.BridgeAction(action.ID)
				if err != nil {
					t.Fatal(err)
				}
				if owner == "current" {
					if w.Code != http.StatusAccepted || after.Status != "accepted" || after.AcceptedAt == nil {
						t.Fatalf("valid worker rejected: status=%d action=%s", w.Code, after.Status)
					}
				} else if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"error":"capture_instance_mismatch"`) || after.Status != "delivered" || after.AcceptedAt != nil {
					t.Fatalf("non-owner reached acceptance: status=%d action=%s accepted=%t", w.Code, after.Status, after.AcceptedAt != nil)
				}
			})
		}
	}
}

func TestSplitCaptureOwnerGuardSurfaceEvents(t *testing.T) {
	for _, suffix := range []string{"", "/", "%2f"} {
		for _, owner := range []string{"missing", "stale", "current"} {
			t.Run(map[string]string{"": "canonical", "/": "trailing_slash", "%2f": "encoded_trailing_slash"}[suffix]+"/"+owner, func(t *testing.T) {
				s, token := splitTestServer(t)
				ctx := context.Background()
				settings, err := s.store.GetSettings(ctx)
				if err != nil {
					t.Fatal(err)
				}
				settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
				session, err := s.store.CreateUpdateSession(ctx, "Offline surface owner guard", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
				if err != nil {
					t.Fatal(err)
				}
				run := session.Runs[0]
				raw, err := json.Marshal(map[string]any{"events": []domain.CaptureSurfaceEvent{{ID: "owner-guard-event", SessionID: session.ID, RunID: run.ID, Source: run.Source, Event: "created"}}})
				if err != nil {
					t.Fatal(err)
				}
				key := map[string]string{"missing": "", "stale": "old-capture-owner", "current": s.splitCapture.key}[owner]
				w := ownerGuardRequest(s, token, key, http.MethodPost, "/api/bridge/capture-surfaces/events"+suffix, string(raw))
				events, err := s.store.CaptureSurfaceEvents(ctx, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if owner == "current" {
					if w.Code != http.StatusAccepted || len(events) != 1 || events[0].ID != "owner-guard-event" {
						t.Fatalf("valid worker telemetry rejected: status=%d events=%d", w.Code, len(events))
					}
				} else if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), `"error":"capture_instance_mismatch"`) || len(events) != 0 {
					t.Fatalf("non-owner recorded telemetry: status=%d events=%d", w.Code, len(events))
				}
			})
		}
	}
}

func TestSplitCaptureOwnerGuardAuthenticationBeforeMutation(t *testing.T) {
	for _, target := range []string{"/api/operations/bridge/actions/missing/accept/", "/api/bridge/capture-surfaces/events/"} {
		for _, credential := range []string{"contract", "token", "owner"} {
			t.Run(target+credential, func(t *testing.T) {
				s, token := splitTestServer(t)
				r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122"+target, strings.NewReader("invalid-json"))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
				r.Header.Set("X-Aku-Bridge-Token", token)
				status, code := http.StatusConflict, "capture_instance_mismatch"
				if credential == "contract" {
					r.Header.Set("X-Aku-Bridge-Contract", "invalid")
					status, code = http.StatusUnauthorized, "invalid_bridge_contract"
				} else if credential == "token" {
					r.Header.Set("X-Aku-Bridge-Token", "invalid")
					status, code = http.StatusUnauthorized, "invalid_bridge_token"
				}
				w := httptest.NewRecorder()
				s.http.Handler.ServeHTTP(w, r)
				if w.Code != status || !strings.Contains(w.Body.String(), `"error":"`+code+`"`) {
					t.Fatalf("authentication order changed: status=%d want=%d body=%s", w.Code, status, w.Body)
				}
			})
		}
	}
}

func TestSplitCaptureOwnerGuardPreservesRelayAndNonSplitAcceptance(t *testing.T) {
	for _, split := range []bool{true, false} {
		t.Run(map[bool]string{true: "split", false: "non_split"}[split], func(t *testing.T) {
			s, token := splitTestServer(t)
			if !split {
				s.splitCapture = nil
			}
			w := ownerGuardRequest(s, token, "", http.MethodPost, "/api/operations/bridge/actions/reload-self/", `{"requestId":"ui-relay","actor":{"kind":"ui"},"reason":"Offline relay compatibility"}`)
			if w.Code != http.StatusAccepted {
				t.Fatal("UI could not request reload", w.Code)
			}
			var payload struct {
				Action struct {
					ID string `json:"id"`
				} `json:"action"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil || payload.Action.ID == "" {
				t.Fatal("missing relay action", err)
			}
			if w := ownerGuardRequest(s, token, "", http.MethodGet, "/api/operations/bridge/actions/next/?waitMs=0", ""); w.Code != http.StatusOK {
				t.Fatal("UI relay could not deliver the action", w.Code)
			}
			if w := ownerGuardRequest(s, token, "", http.MethodGet, "/api/operations/bridge/actions/"+payload.Action.ID+"/", ""); w.Code != http.StatusOK {
				t.Fatal("UI could not read action status", w.Code)
			}
			if !split {
				if w := ownerGuardRequest(s, token, "", http.MethodPost, "/api/operations/bridge/actions/"+payload.Action.ID+"/accept/", ""); w.Code != http.StatusAccepted {
					t.Fatal("non-split extension acceptance now requires a capture key", w.Code)
				}
			}
		})
	}
}

func TestSplitCaptureOwnerGuardClassifierMatchesDispatch(t *testing.T) {
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/api/operations/bridge/actions/id/accept/", true},
		{"/api/operations/bridge/actions/id/accept%2F", true},
		{"/api/bridge/capture-surfaces/events/", true},
		{"/api/bridge/capture-surfaces/events%2f", true},
		{"/api/bridge/commands/next/", true},
		{"/api/bridge/media-recaptures/id/claim/", true},
		{"/api/bridge/split-capture/next/", true},
		{"/api/operations/bridge/actions/id/accept//", false},
		{"/api/operations/bridge/actions/id/accept%252F", false},
		{"/api/operations/bridge/actions/next/", false},
		{"/api/operations/bridge/actions/reload-self/", false},
		{"/api/operations/bridge/actions/id/", false},
		{"/api/bridge/timeline/id/media-evidence/", false},
		{"/api/bridge/heartbeat/", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122"+tc.path, nil)
			if got := splitCaptureOwnedRoute(r); got != tc.want {
				t.Fatalf("classifier=%t want=%t decodedPath=%s", got, tc.want, r.URL.Path)
			}
		})
	}
}
