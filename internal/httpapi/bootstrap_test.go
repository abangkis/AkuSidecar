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
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestHTTPBootstrapCharacterizationEnvelope(t *testing.T) {
	f := newSessionUpdatesFixture(t)
	for _, target := range []string{"/api/bootstrap", "/api/bootstrap/"} {
		payload := f.json(t, f.serve(sessionUpdatesRequest("GET", target, ""), true), 200)
		keys := strings.Fields("version runtime deployment provider reasoningProviders reasoningRuntime reasoningProcesses mediaProvenanceRuntime instanceEpoch bridgeContractVersion softwareUpdate bridgeToken bridge database databaseHealth sources settings collectionRuntime xCapture instagramCapture onboarding calibration activeSession timeline timelineBatches latestCheck autoUpdate")
		if len(payload) != len(keys) {
			t.Fatalf("bootstrap field count=%d want=%d", len(payload), len(keys))
		}
		for _, key := range keys {
			if _, ok := payload[key]; !ok {
				t.Fatal("missing bootstrap field", key)
			}
		}
		for key, want := range map[string]string{
			"version": `"` + domain.ApplicationVersion + `"`, "runtime": `"go"`,
			"provider": `"deterministic"`, "instanceEpoch": `"` + f.s.engine.Epoch() + `"`,
			"bridgeContractVersion": `"` + domain.BridgeContractVersion + `"`,
			"activeSession":         "null", "latestCheck": "null", "timeline": "[]", "timelineBatches": "[]", "reasoningProviders": "[]",
		} {
			if string(payload[key]) != want {
				t.Fatalf("bootstrap %s=%s want=%s", key, payload[key], want)
			}
		}
		var token string
		if err := json.Unmarshal(payload["bridgeToken"], &token); err != nil || !f.s.store.MatchesBridgeToken(context.Background(), token) {
			t.Fatal("bootstrap lost the UI Bridge token")
		}
		var database struct {
			Status        string `json:"status"`
			SchemaVersion int    `json:"schemaVersion"`
		}
		if err := json.Unmarshal(payload["database"], &database); err != nil || database.Status != "healthy" || database.SchemaVersion != store.SchemaVersion {
			t.Fatal("database summary changed", database, err)
		}
		metadata, err := json.Marshal(domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion))
		if err != nil || string(payload["softwareUpdate"]) != string(metadata) {
			t.Fatal("software update metadata changed", err)
		}
	}
}

func TestHTTPBootstrapCharacterizationProjectionAndCapacity(t *testing.T) {
	f := newSessionUpdatesFixture(t)
	settings, err := f.s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.LoadProfile = "custom"
	settings.TimelineCapacity = 1
	settings.SemanticEventMode = "show_all"
	if err := f.s.store.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	f.seed(t, "active", "queued", true)
	f.seed(t, "older", "completed", true)
	f.exec(t, `UPDATE sessions SET completed_at='2026-01-01T00:00:00Z' WHERE id='older'`)
	f.exec(t, `UPDATE timeline_items SET rank=1 WHERE id='older-item'`)
	before, err := f.s.store.GetSession(context.Background(), "active")
	if err != nil {
		t.Fatal(err)
	}
	payload := f.json(t, f.serve(sessionUpdatesRequest("GET", "/api/bootstrap", ""), true), 200)
	var active domain.Session
	if err := json.Unmarshal(payload["activeSession"], &active); err != nil {
		t.Fatal(err)
	}
	if active.ID != "active" || active.ItemCount != 1 || active.Items != nil || len(active.Runs) != 1 || active.Runs[0].Coverage != nil || len(active.Coverage) != 2 || active.Coverage["pipelineStage"] != "capture" || active.Coverage["pipelineStageUpdatedAt"] != "fixture" {
		t.Fatal("bootstrap lost its bounded session projection")
	}
	var timeline []domain.TimelineItem
	if err := json.Unmarshal(payload["timeline"], &timeline); err != nil || len(timeline) != 1 {
		t.Fatal("bootstrap ignored TimelineCapacity", len(timeline), err)
	}
	var batches []domain.TimelineBatchSummary
	var latest domain.TimelineCheckSummary
	if err := json.Unmarshal(payload["timelineBatches"], &batches); err != nil || len(batches) != 1 || batches[0].SessionID != "older" {
		t.Fatal("bootstrap batch summaries changed", err)
	}
	if err := json.Unmarshal(payload["latestCheck"], &latest); err != nil || latest.SessionID != "older" {
		t.Fatal("bootstrap latest check changed", err)
	}
	after, err := f.s.store.GetSession(context.Background(), "active")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("bootstrap projection mutated durable session state", err)
	}
}

func TestHTTPBootstrapCharacterizationSecurityAndRouting(t *testing.T) {
	for _, tc := range []struct {
		name, method, target, host, origin string
		status                             int
	}{
		{"same_origin_no_bridge_headers", "GET", "/api/bootstrap", "127.0.0.1:11122", "http://127.0.0.1:11122", 200},
		{"untrusted_host", "GET", "/api/bootstrap", "attacker.test", "", 403},
		{"cross_origin", "GET", "/api/bootstrap", "127.0.0.1:11122", "https://attacker.test", 403},
		{"trusted_extension_is_not_ui_origin", "GET", "/api/bootstrap", "127.0.0.1:11122", "chrome-extension://abcdefghijklmnop", 403},
		{"wrong_method", "POST", "/api/bootstrap", "127.0.0.1:11122", "", 404},
		{"nested_path", "GET", "/api/bootstrap/extra", "127.0.0.1:11122", "", 404},
		{"double_slash", "GET", "/api/bootstrap//", "127.0.0.1:11122", "", 404},
		{"preflight", "OPTIONS", "/api/bootstrap", "127.0.0.1:11122", "", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSessionUpdatesFixture(t)
			f.s.config.Bridge.TrustedExtensionOrigins = []string{"chrome-extension://abcdefghijklmnop"}
			f.exec(t, `DELETE FROM meta WHERE key='bridge_token'`)
			r := sessionUpdatesRequest(tc.method, tc.target, "")
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			w := f.serve(r, true)
			if w.Code != tc.status || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("X-Frame-Options") != "DENY" || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatal("bootstrap security/routing changed", w.Code, w.Header())
			}
			if tc.status == 200 {
				f.json(t, w, 200)
			} else if tc.status == 404 {
				f.error(t, w, 404, "not_found", "route not found")
			} else if w.Header().Get("X-Aku-Sidecar-Instance-Epoch") != "" {
				t.Fatal("rejected or preflight request entered the route")
			}
			wantTokens := 0
			if tc.status == 200 {
				wantTokens = 1
			}
			if f.count(t, `SELECT COUNT(*) FROM meta WHERE key='bridge_token'`) != wantTokens {
				t.Fatal("rejected request ran bootstrap side effects")
			}
		})
	}
}

// Seed only local evidence; no browser, provider call, or scheduler is started.
func bootstrapPendingFixture(t *testing.T) sessionUpdatesFixture {
	t.Helper()
	f := newSessionUpdatesFixture(t)
	f.ready(t)
	f.seed(t, "eligible", "completed", true)
	f.exec(t, `UPDATE sessions SET completed_at='2026-01-01T00:00:00Z' WHERE id='eligible'`)
	f.exec(t, `UPDATE runs SET status='completed',completed_at='2026-01-01T00:00:00Z' WHERE id='eligible-run'`)
	f.exec(t, `INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('bootstrap-command','eligible-run','collect_visible','completed','{}',?)`, domain.Now())
	observation := domain.Observation{Source: domain.SourceLinkedIn, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "http-session-evidence", Text: "Offline bootstrap candidate", Permalink: "https://www.linkedin.com/feed/update/urn:li:activity:1234567890"}}}}}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('bootstrap-observation','eligible-run','bootstrap-command','linkedin',?,?,?)`, string(raw), domain.Now(), domain.Now())
	f.exec(t, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES('eligible-run','http-session-evidence','linkedin','{}',0,0,0,1,?)`, domain.Now())
	f.seed(t, "expired", "completed", false)
	f.exec(t, `INSERT INTO auto_update_batches(session_id,state,created_at,prepared_at,expires_at) VALUES('expired','prepared','2000-01-01T00:00:00Z','2000-01-01T00:00:00Z','2000-01-02T00:00:00Z')`)
	f.exec(t, `DELETE FROM meta WHERE key='bridge_token'`)
	return f
}

func assertBootstrapEffects(t *testing.T, f sessionUpdatesFixture, token, calibration, expired int) {
	t.Helper()
	for _, tc := range []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM meta WHERE key='bridge_token'`, token},
		{`SELECT COUNT(*) FROM calibration_sessions WHERE trigger_kind='first_run'`, calibration},
		{`SELECT COUNT(*) FROM auto_update_batches WHERE session_id='expired' AND state='expired'`, expired},
		{`SELECT COUNT(*) FROM meta WHERE key='auto_update_queue_vacancy_at'`, expired},
	} {
		if got := f.count(t, tc.query); got != tc.want {
			t.Fatalf("bootstrap effect count=%d want=%d query=%s", got, tc.want, tc.query)
		}
	}
}

func TestHTTPBootstrapCharacterizationOrderedEffects(t *testing.T) {
	f := bootstrapPendingFixture(t)
	before, err := f.s.store.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		payload := f.json(t, f.serve(sessionUpdatesRequest("GET", "/api/bootstrap", ""), true), 200)
		assertBootstrapEffects(t, f, 1, 1, 1)
		var calibration domain.CalibrationOverview
		var autoUpdate domain.AutoUpdateStatus
		if err := json.Unmarshal(payload["calibration"], &calibration); err != nil || calibration.Active == nil || calibration.Active.SampleCount != 1 || calibration.Active.Status != "reviewing" || calibration.FirstRunStatus != "pending" {
			t.Fatal("bootstrap did not expose pending calibration", err)
		}
		if err := json.Unmarshal(payload["autoUpdate"], &autoUpdate); err != nil || len(autoUpdate.PreparedBatches) != 0 {
			t.Fatal("bootstrap exposed an expired prepared batch", err)
		}
	}
	after, err := f.s.store.GetSettings(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) || f.s.engine.XCaptureStatus().DerivationAuthorized {
		t.Fatal("bootstrap changed settings or granted X authority", err)
	}
}

func TestHTTPBootstrapCharacterizationFailureOrder(t *testing.T) {
	for _, tc := range []struct {
		name, fault, message        string
		token, calibration, expired int
	}{
		{"settings", `UPDATE settings SET value_json='invalid-json' WHERE key='runtime'`, "decode settings", 0, 0, 0},
		{"token", `CREATE TRIGGER bootstrap_fault BEFORE INSERT ON meta WHEN NEW.key='bridge_token' BEGIN SELECT RAISE(ABORT,'bootstrap-token-fault'); END`, "bootstrap-token-fault", 0, 0, 0},
		{"active_session", `DROP TABLE sessions`, "sessions", 1, 0, 0},
		{"timeline", `DROP TABLE timeline_items`, "timeline_items", 1, 0, 0},
		{"onboarding", `UPDATE meta SET value='invalid-status' WHERE key='onboarding_status'`, "invalid onboarding status", 1, 0, 0},
		{"calibration", `CREATE TRIGGER bootstrap_fault BEFORE INSERT ON calibration_sessions BEGIN SELECT RAISE(ABORT,'bootstrap-calibration-fault'); END`, "bootstrap-calibration-fault", 1, 0, 0},
		{"auto_update", `CREATE TRIGGER bootstrap_fault BEFORE UPDATE OF state ON auto_update_batches WHEN NEW.state='expired' BEGIN SELECT RAISE(ABORT,'bootstrap-expiry-fault'); END`, "bootstrap-expiry-fault", 1, 1, 0},
		{"database_health", `DROP TABLE memory_retention_claims`, "memory_retention_claims", 1, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := bootstrapPendingFixture(t)
			f.exec(t, tc.fault)
			w := httptest.NewRecorder()
			err := f.s.route(w, sessionUpdatesRequest("GET", "/api/bootstrap", ""))
			if err == nil || !strings.Contains(err.Error(), tc.message) || w.Body.Len() != 0 {
				t.Fatal("bootstrap did not return the first original error", err)
			}
			assertBootstrapEffects(t, f, tc.token, tc.calibration, tc.expired)
			f.error(t, f.serve(sessionUpdatesRequest("GET", "/api/bootstrap", ""), true), 500, "internal_error", "internal server error")
			assertBootstrapEffects(t, f, tc.token, tc.calibration, tc.expired)
		})
	}
}

func TestHTTPBootstrapCharacterizationCancellationAndWriteFailure(t *testing.T) {
	for _, kind := range []string{"cancelled", "deadline", "writer"} {
		t.Run(kind, func(t *testing.T) {
			f := bootstrapPendingFixture(t)
			r := sessionUpdatesRequest("GET", "/api/bootstrap", "")
			if kind == "writer" {
				failure := errors.New("bootstrap-response-write-failed")
				w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
				if err := f.s.route(w, r); !errors.Is(err, failure) || w.status != 200 {
					t.Fatal("bootstrap lost the response write error", err)
				}
				assertBootstrapEffects(t, f, 1, 1, 1)
				return
			}
			ctx, cancel := context.WithCancel(r.Context())
			want := context.Canceled
			if kind == "deadline" {
				cancel()
				ctx, cancel = context.WithDeadline(r.Context(), time.Unix(1, 0))
				want = context.DeadlineExceeded
			}
			cancel()
			if err := f.s.route(httptest.NewRecorder(), r.WithContext(ctx)); !errors.Is(err, want) {
				t.Fatal("bootstrap lost the request context error", err)
			}
			f.error(t, f.serve(r.WithContext(ctx), true), 500, "internal_error", "internal server error")
			assertBootstrapEffects(t, f, 0, 0, 0)
		})
	}
}
