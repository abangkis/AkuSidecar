package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type bridgeMediaRecaptureFixture struct {
	s           *Server
	token       string
	db          *sql.DB
	job         domain.MediaRecapture
	observation domain.Observation
}

// Seed only disposable evidence; job creation and all HTTP operations use the
// real Store/Engine. No collector, account, provenance inspector or OS observer
// is attached, and no browser dispatch is requested.
func newBridgeMediaRecaptureFixture(t *testing.T, route string) bridgeMediaRecaptureFixture {
	t.Helper()
	s, token := splitTestServer(t)
	db, err := sql.Open("sqlite", s.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Cleanup(func() {
		s.engine.Shutdown()
		if !s.engine.WaitForIdle(3 * time.Second) {
			t.Error("recapture fixture workers did not stop")
		}
	})
	source := domain.SourceLinkedIn
	native := "https://www.linkedin.com/feed/update/urn:li:activity:1234567890123456789"
	if route == "waiting" {
		source, native = domain.SourceFacebook, "https://www.facebook.com/photo/?fbid=123456789012345"
	}
	if route == "x" {
		source, native = domain.SourceX, "https://x.com/example/status/1234567890123456789"
	}
	key := string(source) + ":http-recapture"
	block := domain.Block{EvidenceKey: key, PlatformID: "1234567890123456789", Permalink: native, Author: "Fixture", Text: "Offline recapture evidence", MediaRecovery: map[string]any{"outcome": "unavailable"}}
	observation := domain.Observation{Source: source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}, Coverage: map[string]any{"quality": "complete"}}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	now := domain.Now()
	if _, err := db.Exec(`INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('recapture-observation','recapture-run','recapture-command',?,?,?,?)`, source, string(raw), now, now); err != nil {
		t.Fatal(err)
	}
	item, err := json.Marshal(domain.ReasonedItem{Source: source, SourceURL: native, EvidenceKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('recapture-item','recapture-session','recapture-run',?,?,0,?,'{}',?)`, source, key, string(item), now); err != nil {
		t.Fatal(err)
	}
	var stamp map[string]any
	collector := "bridge"
	if route == "stale" {
		stamp = map[string]any{"driver": "browser", "epoch": "stale", "generation": 999}
	}
	if route == "quiet" {
		collector = "browser_quiet_hidden"
	}
	var job domain.MediaRecapture
	if route == "waiting" {
		job, err = s.store.CreateOwnedMediaRecaptureWithAdmission(context.Background(), "recapture-item", domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, collector, domain.MediaRecaptureCaptureAdmission{Policy: domain.MediaRecaptureAdmissionHybridHeadlessV1, Driver: "browser", Phase: "waiting"})
	} else {
		job, err = s.store.CreateOwnedMediaRecapture(context.Background(), "recapture-item", domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, stamp, collector)
	}
	if err != nil {
		t.Fatal(err)
	}
	observation.Snapshots[0].Blocks[0].Media = []map[string]any{{"kind": "image", "url": "https://media.licdn.com/dms/image/fixture.jpg"}}
	return bridgeMediaRecaptureFixture{s, token, db, job, observation}
}

func (f bridgeMediaRecaptureFixture) request(method, suffix, body string) *http.Request {
	r := bridgeCommandsRequest(f.s, f.token, method, "/api/bridge/media-recaptures/"+f.job.ID+"/"+suffix, body)
	r.Header.Set("X-Aku-Bridge-Id", "recapture-characterization")
	return r
}

func (f bridgeMediaRecaptureFixture) body(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f bridgeMediaRecaptureFixture) state(t *testing.T) string {
	t.Helper()
	var row, override string
	if err := f.db.QueryRow(`SELECT json_object('status',status,'payload',payload_json,'claimedBy',claimed_by,'claimedAt',claimed_at,'completedAt',completed_at,'result',result_json,'error',error_json) FROM media_recaptures WHERE id=?`, f.job.ID).Scan(&row); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT evidence_json FROM timeline_evidence_overrides WHERE timeline_id=?`, f.job.TimelineID).Scan(&override); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	return row + "\n" + override
}

func TestBridgeMediaRecaptureCharacterizationAuthentication(t *testing.T) {
	for _, endpoint := range []string{"claim", "observation", "failure"} {
		for _, guard := range []string{"contract", "token", "capture"} {
			t.Run(endpoint+"/"+guard, func(t *testing.T) {
				f := newBridgeMediaRecaptureFixture(t, "bridge")
				before := f.state(t)
				method := "POST"
				if endpoint == "claim" {
					method = "GET"
				}
				r := f.request(method, endpoint, "{")
				r.Header.Set("X-Aku-Capture-Instance", "stale")
				switch guard {
				case "contract":
					r.Header.Set("X-Aku-Bridge-Contract", "wrong")
					r.Header.Set("X-Aku-Bridge-Token", "wrong")
					bridgeCommandsError(t, bridgeCommandsServe(f.s, r), 401, "invalid_bridge_contract", "unsupported Bridge contract")
				case "token":
					r.Header.Set("X-Aku-Bridge-Token", "wrong")
					bridgeCommandsError(t, bridgeCommandsServe(f.s, r), 401, "invalid_bridge_token", "invalid Bridge token")
				case "capture":
					bridgeCommandsError(t, bridgeCommandsServe(f.s, r), 409, "capture_instance_mismatch", "active Windows capture process")
				}
				if after := f.state(t); after != before {
					t.Fatalf("authentication rejection changed durable state: %s", after)
				}
			})
		}
	}
}

func TestBridgeMediaRecaptureCharacterizationClaim(t *testing.T) {
	for _, bridgeID := range []string{"recapture-characterization", "", " bridge with spaces "} {
		t.Run("available/"+bridgeID, func(t *testing.T) {
			f := newBridgeMediaRecaptureFixture(t, "bridge")
			r := f.request("GET", "claim/", "")
			r.Header.Set("X-Aku-Bridge-Id", bridgeID)
			value := bridgeCommandsJSON(t, bridgeCommandsServe(f.s, r), 200)
			var claimed domain.MediaRecapture
			if err := json.Unmarshal(value["recapture"], &claimed); err != nil || len(value) != 1 || claimed.ID != f.job.ID || claimed.Status != "claimed" || claimed.ClaimedAt == nil {
				t.Fatalf("claim envelope=%s err=%v", value, err)
			}
			var owner string
			if err := f.db.QueryRow(`SELECT claimed_by FROM media_recaptures WHERE id=?`, f.job.ID).Scan(&owner); err != nil || owner != bridgeID {
				t.Fatalf("Bridge identity changed: %q %v", owner, err)
			}
			before := f.state(t)
			bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("GET", "claim", "")), 409, "conflict", "claimed, not queued")
			if f.state(t) != before {
				t.Fatal("repeat claim changed durable state")
			}
		})
	}
	for _, route := range []string{"quiet", "waiting"} {
		t.Run("empty/"+route, func(t *testing.T) {
			f := newBridgeMediaRecaptureFixture(t, route)
			before := f.state(t)
			w := bridgeCommandsServe(f.s, f.request("GET", "claim", ""))
			if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" || f.state(t) != before {
				t.Fatalf("empty claim changed: %d %s %v", w.Code, w.Body, w.Header())
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		f := newBridgeMediaRecaptureFixture(t, "bridge")
		f.job.ID = "missing"
		bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("GET", "claim", "")), 404, "not_found", "media recapture not found")
	})
}

func TestBridgeMediaRecaptureCharacterizationParsing(t *testing.T) {
	for _, endpoint := range []string{"observation", "failure"} {
		for _, tc := range []struct {
			name, contentType, body, message string
			status                           int
		}{
			{"media", "text/plain", "{}", "Content-Type must be application/json", 415},
			{"malformed", "application/json", "{", "request body must be valid JSON:", 400},
			{"unknown", "application/json", `{"runId":"unsupported"}`, `unknown field "runId"`, 400},
			{"oversize", "application/json", `{"` + map[string]string{"observation": "observation", "failure": "error"}[endpoint] + `":{"` + map[string]string{"observation": "pageTitle", "failure": "message"}[endpoint] + `":"` + strings.Repeat("a", 1_000_002) + `"}}`, "unexpected EOF", 400},
		} {
			t.Run(endpoint+"/"+tc.name, func(t *testing.T) {
				f := newBridgeMediaRecaptureFixture(t, "bridge")
				before := f.state(t)
				r := f.request("POST", endpoint, tc.body)
				r.Header.Set("Content-Type", tc.contentType)
				code := "invalid_request"
				if tc.status == 415 {
					code = "unsupported_media_type"
				}
				bridgeCommandsError(t, bridgeCommandsServe(f.s, r), tc.status, code, tc.message)
				if f.state(t) != before {
					t.Fatal("invalid input mutated durable state")
				}
			})
		}
	}
}

type bridgeRecaptureCommitWriter struct {
	*httptest.ResponseRecorder
	beforeSuccess func()
}

func (w bridgeRecaptureCommitWriter) WriteHeader(status int) {
	if status == 200 {
		w.beforeSuccess()
	}
	w.ResponseRecorder.WriteHeader(status)
}

func TestBridgeMediaRecaptureCharacterizationResults(t *testing.T) {
	for _, endpoint := range []string{"observation", "failure"} {
		for _, claim := range []bool{false, true} {
			name := endpoint + "/queued"
			if claim {
				name = endpoint + "/claimed"
			}
			t.Run(name, func(t *testing.T) {
				f := newBridgeMediaRecaptureFixture(t, "bridge")
				if claim {
					job, err := f.s.store.ClaimMediaRecapture(context.Background(), f.job.ID, "fixture")
					if err != nil || job.ID != f.job.ID {
						t.Fatalf("claim: %+v %v", job, err)
					}
				}
				body := map[string]any{"error": domain.Failure{Message: "fixture failure"}}
				status := "failed"
				if endpoint == "observation" {
					body, status = map[string]any{"observation": f.observation}, "completed"
				}
				raw := f.body(t, body)
				before := f.state(t)
				recorder := httptest.NewRecorder()
				w := bridgeRecaptureCommitWriter{recorder, func() {
					job, err := f.s.store.MediaRecapture(context.Background(), f.job.ID)
					if err != nil || job.Status != status {
						t.Fatalf("success preceded durable result: %+v %v", job, err)
					}
				}}
				// Extra path components, one trailing slash, and trailing JSON retain
				// the existing path.Base and single-Decode contracts.
				r := bridgeCommandsRequest(f.s, f.token, "POST", "/api/bridge/media-recaptures/extra/"+f.job.ID+"/"+endpoint+"/", raw+" {}")
				f.s.api().ServeHTTP(w, r)
				if endpoint == "observation" && !claim {
					bridgeCommandsError(t, recorder, 400, "invalid_request", "queued, not claimed")
					if f.state(t) != before {
						t.Fatal("queued observation changed durable state")
					}
					return
				}
				value := bridgeCommandsJSON(t, recorder, 200)
				var result domain.MediaRecapture
				if err := json.Unmarshal(value["recapture"], &result); err != nil || len(value) != 1 || result.ID != f.job.ID || result.Status != status || result.CompletedAt == nil {
					t.Fatalf("result envelope=%s err=%v", value, err)
				}
				if endpoint == "failure" {
					if result.Error == nil || result.Error.Stage != "media_recapture" || result.Error.Code != "" || result.Error.Message != "fixture failure" {
						t.Fatalf("failure defaults changed: %+v", result.Error)
					}
				} else {
					if result.Outcome != "recovered" {
						t.Fatalf("outcome=%q", result.Outcome)
					}
					var evidence string
					if err := f.db.QueryRow(`SELECT evidence_json FROM timeline_evidence_overrides WHERE timeline_id=?`, f.job.TimelineID).Scan(&evidence); err != nil || !strings.Contains(evidence, "fixture.jpg") {
						t.Fatalf("evidence not persisted: %s %v", evidence, err)
					}
					var count int
					if err := f.db.QueryRow(`SELECT COUNT(*) FROM timeline_items`).Scan(&count); err != nil || count != 1 {
						t.Fatalf("timeline count=%d %v", count, err)
					}
				}
				before = f.state(t)
				bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("POST", endpoint, raw)), 400, "invalid_request", "not ")
				if f.state(t) != before {
					t.Fatal("replay changed durable state")
				}
			})
		}
	}
}

func TestBridgeMediaRecaptureCharacterizationRejectedObservations(t *testing.T) {
	for _, invalid := range []string{"source", "target", "stale-owner"} {
		t.Run(invalid, func(t *testing.T) {
			route := "bridge"
			if invalid == "stale-owner" {
				route = "stale"
			}
			f := newBridgeMediaRecaptureFixture(t, route)
			if _, err := f.s.store.ClaimMediaRecapture(context.Background(), f.job.ID, "fixture"); err != nil {
				t.Fatal(err)
			}
			message := "capture command belongs to another runtime owner"
			if invalid == "source" {
				f.observation.Source, message = domain.SourceX, "recapture observation source does not match the item"
				f.observation.Snapshots[0].Blocks[0].Permalink = "https://x.com/fixture/status/1234567890123456789"
			}
			if invalid == "target" {
				block := &f.observation.Snapshots[0].Blocks[0]
				block.EvidenceKey, block.PlatformID, block.Permalink = "other", "9999999999999999999", "https://www.linkedin.com/feed/update/urn:li:activity:9999999999999999999"
				message = "recapture did not return the requested native post"
			}
			before := f.state(t)
			bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("POST", "observation", f.body(t, map[string]any{"observation": f.observation}))), 400, "invalid_request", message)
			if f.state(t) != before {
				t.Fatal("rejected observation changed durable state")
			}
		})
	}
}

func TestBridgeXRecaptureSelectionCoverageContract(t *testing.T) {
	t.Run("observation", func(t *testing.T) {
		f := newBridgeMediaRecaptureFixture(t, "x")
		bridgeCommandsJSON(t, bridgeCommandsServe(f.s, f.request("GET", "claim", "")), 200)
		f.observation.PageURL = f.job.TargetURL
		f.observation.Coverage["xRecaptureSelection"] = map[string]any{
			"outcome": "target_captured", "targetMatchCount": 1,
		}
		f.observation.Coverage["xRecaptureRoute"] = map[string]any{"kind": "target_status", "source": "tab"}
		const imageURL = "https://pbs.twimg.com/media/fixture.jpg"
		block := &f.observation.Snapshots[0].Blocks[0]
		block.PlatformID = "x:status:1234567890123456789"
		block.Media = []map[string]any{{"kind": "image", "url": imageURL}}
		body := f.body(t, map[string]any{"observation": f.observation})
		value := bridgeCommandsJSON(t, bridgeCommandsServe(f.s, f.request("POST", "observation", body)), 200)
		var job domain.MediaRecapture
		if err := json.Unmarshal(value["recapture"], &job); err != nil || job.Status != "completed" || job.Outcome != "recovered" {
			t.Fatalf("recapture result: %+v, %v", job, err)
		}
		var stored domain.Observation
		var raw string
		if err := f.db.QueryRow("SELECT result_json FROM media_recaptures WHERE id = ?", f.job.ID).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(raw), &stored); err != nil {
			t.Fatal(err)
		}
		selection, _ := stored.Coverage["xRecaptureSelection"].(map[string]any)
		if selection["outcome"] != "target_captured" || selection["targetMatchCount"] != float64(1) {
			t.Fatalf("stored selection = %v", selection)
		}
		item, err := f.s.store.TimelineItem(context.Background(), f.job.TimelineID)
		if err != nil || item.Evidence == nil || len(item.Evidence.Media) != 1 || item.Evidence.Media[0]["url"] != imageURL {
			t.Fatalf("Timeline recapture evidence missing: %+v, %v", item, err)
		}
	})

	t.Run("failure", func(t *testing.T) {
		f := newBridgeMediaRecaptureFixture(t, "x")
		bridgeCommandsJSON(t, bridgeCommandsServe(f.s, f.request("GET", "claim", "")), 200)
		failure := domain.Failure{
			Code: "recapture_target_not_observed", Stage: "capture",
			Details: map[string]any{"xRecaptureSelection": map[string]any{
				"outcome": "target_not_identified", "targetMatchCount": 0,
			}},
		}
		body := f.body(t, map[string]any{"error": failure})
		bridgeCommandsJSON(t, bridgeCommandsServe(f.s, f.request("POST", "failure", body)), 200)
		job, err := f.s.store.MediaRecapture(context.Background(), f.job.ID)
		if err != nil || job.Status != "failed" || job.Error == nil || job.Error.Code != failure.Code {
			t.Fatalf("stored failure: %+v, %v", job, err)
		}
		selection, _ := job.Error.Details["xRecaptureSelection"].(map[string]any)
		if selection["outcome"] != "target_not_identified" || selection["targetMatchCount"] != float64(0) {
			t.Fatalf("stored failure selection = %v", selection)
		}
	})
}

func TestBridgeXRecapturePersistsRejectedRouteAndCandidateMatching(t *testing.T) {
	for _, tc := range []struct{ name, pageURL, kind, source, wantKind, wantSource, wantMetadata string }{
		{"prefer final tab", "https://x.com/home", "target_status", "tab", "target_status", "tab", "reported"},
		{"Bridge observation", "https://x.com/private", "feed", "observation", "feed", "observation", "reported"},
		{"missing metadata with target", "https://x.com/i/web/status/1234567890123456789?token=secret-marker", "", "", "target_status", "observation", "missing"},
		{"missing metadata with another post", "https://x.com/other/status/9999999999999999999#secret-marker", "", "", "other_status", "observation", "missing"},
		{"unavailable tab with feed", "https://x.com/home?token=secret-marker", "unavailable", "unavailable", "feed", "observation", "unavailable"},
		{"invalid metadata with login", "https://x.com/i/flow/login?token=secret-marker", "https://x.com/private?token=secret-marker", "unknown", "login", "observation", "invalid"},
		{"challenge route", "https://x.com/account/access", "", "", "challenge", "observation", "missing"},
		{"outside X", "https://x.com.example/home", "", "", "outside_x", "observation", "missing"},
		{"other X route", "https://x.com/private", "", "", "other_x", "observation", "missing"},
		{"invalid observation route", "/home", "", "", "unavailable", "unavailable", "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBridgeMediaRecaptureFixture(t, "x")
			if _, err := f.s.store.ClaimMediaRecapture(context.Background(), f.job.ID, "fixture"); err != nil {
				t.Fatal(err)
			}
			f.observation.PageURL = tc.pageURL
			if tc.kind != "" || tc.source != "" {
				f.observation.Coverage["xRecaptureRoute"] = map[string]any{"kind": tc.kind, "source": tc.source, "url": tc.pageURL}
			}
			f.observation.Snapshots[0].Blocks = []domain.Block{
				{EvidenceKey: "secret-marker", PlatformID: "x:status:1234567890123456789",
					Permalink: "https://x.com/renamed/status/1234567890123456789", Text: "secret-marker"},
				{EvidenceKey: "missing", PlatformID: "1234567890123456789", Text: "secret-marker"},
				{EvidenceKey: "different", Permalink: "https://x.com/other/status/9999999999999999999", Text: "secret-marker"},
			}
			body := f.body(t, map[string]any{"observation": f.observation})
			bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("POST", "observation", body)), 400, "invalid_request",
				"recapture did not return the requested native post")

			status := bridgeCommandsServe(f.s, httptest.NewRequest(http.MethodGet, "/api/media-recaptures/"+f.job.ID, nil))
			var response struct {
				Recapture struct {
					Status string         `json:"status"`
					Error  domain.Failure `json:"error"`
				} `json:"recapture"`
			}
			if status.Code != 200 || json.Unmarshal(status.Body.Bytes(), &response) != nil {
				t.Fatalf("status response: %d %s", status.Code, status.Body.String())
			}
			job := response.Recapture
			if job.Status != "failed" || job.Error.Code != "recapture_target_mismatch" || job.Error.Stage != "capture" {
				t.Fatalf("job=%+v", job)
			}
			route, _ := job.Error.Details["xRecaptureRoute"].(map[string]any)
			if route["kind"] != tc.wantKind || route["source"] != tc.wantSource ||
				route["bridgeMetadata"] != tc.wantMetadata || len(route) != 3 {
				t.Fatalf("route=%v", route)
			}
			counts, _ := job.Error.Details["candidateMatching"].(map[string]any)
			for key, want := range map[string]float64{
				"candidateCount": 3, "missingPermalink": 1, "evidenceKeyMatches": 0, "permalinkMatches": 0,
				"platformStatusMatches": 2, "permalinkStatusMatches": 1,
			} {
				if counts[key] != want {
					t.Errorf("%s=%v; want %v", key, counts[key], want)
				}
			}
			state := f.state(t)
			if strings.Contains(state, "secret-marker") || !strings.Contains(state, `"result":null`) {
				t.Fatalf("saved raw rejected evidence: %s", state)
			}
			item, err := f.s.store.TimelineItem(context.Background(), f.job.TimelineID)
			if err != nil || item.Evidence == nil || len(item.Evidence.Media) != 0 {
				t.Fatalf("Timeline item changed after rejected observation: %+v %v", item, err)
			}
			// Older Bridge code sends a generic failure after the HTTP rejection.
			fallback := f.body(t, map[string]any{"error": domain.Failure{Code: "bridge_failure"}})
			bridgeCommandsServe(f.s, f.request("POST", "failure", fallback))
			if f.state(t) != state {
				t.Fatal("fallback failure replaced the stored mismatch diagnostic")
			}
		})
	}
}

func TestBridgeMediaRecaptureCharacterizationStaleOwner(t *testing.T) {
	t.Run("claim-fails-job", func(t *testing.T) {
		f := newBridgeMediaRecaptureFixture(t, "stale")
		bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("GET", "claim", "")), 409, "conflict", "capture command belongs to another runtime owner")
		job, err := f.s.store.MediaRecapture(context.Background(), f.job.ID)
		if err != nil || job.Status != "failed" || job.Error == nil || job.Error.Code != "capture_runtime_changed" {
			t.Fatalf("stale claim side effect changed: %+v %v", job, err)
		}
	})
	t.Run("failure-rejected", func(t *testing.T) {
		f := newBridgeMediaRecaptureFixture(t, "stale")
		before := f.state(t)
		bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request("POST", "failure", `{"error":{}}`)), 400, "invalid_request", "capture command belongs to another runtime owner")
		if f.state(t) != before {
			t.Fatal("stale failure changed durable state")
		}
	})
}

func TestBridgeMediaRecaptureCharacterizationContext(t *testing.T) {
	for _, endpoint := range []string{"claim", "observation", "failure"} {
		t.Run(endpoint+"/canceled-auth", func(t *testing.T) {
			f := newBridgeMediaRecaptureFixture(t, "bridge")
			before := f.state(t)
			method := "POST"
			if endpoint == "claim" {
				method = "GET"
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request(method, endpoint, "{}").WithContext(ctx)), 401, "invalid_bridge_token", "invalid Bridge token")
			if f.state(t) != before {
				t.Fatal("canceled authentication changed durable state")
			}
		})
	}
	for _, endpoint := range []string{"observation", "failure"} {
		t.Run(endpoint+"/canceled-after-decode", func(t *testing.T) {
			f := newBridgeMediaRecaptureFixture(t, "bridge")
			before := f.state(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := f.request("POST", endpoint, "")
			r.Body = bridgeCommandsCancelBody{strings.NewReader("{}"), cancel}
			bridgeCommandsError(t, bridgeCommandsServe(f.s, r.WithContext(ctx)), 400, "invalid_request", "context canceled")
			if f.state(t) != before {
				t.Fatal("canceled result changed durable state")
			}
		})
	}
}

func TestBridgeMediaRecaptureCharacterizationRouting(t *testing.T) {
	f := newBridgeMediaRecaptureFixture(t, "bridge")
	for _, tc := range []struct{ method, suffix string }{
		{"POST", "claim"}, {"GET", "observation"}, {"GET", "failure"}, {"PUT", "observation"}, {"DELETE", "failure"}, {"GET", "claim//"}, {"POST", "failure//"},
	} {
		t.Run(tc.method+"/"+tc.suffix, func(t *testing.T) {
			before := f.state(t)
			bridgeCommandsError(t, bridgeCommandsServe(f.s, f.request(tc.method, tc.suffix, "{}")), 404, "not_found", "route not found")
			if f.state(t) != before {
				t.Fatal("unmatched route changed durable state")
			}
		})
	}
	for _, endpoint := range []string{"observation", "failure"} {
		t.Run("missing/"+endpoint, func(t *testing.T) {
			r := bridgeCommandsRequest(f.s, f.token, "POST", "/api/bridge/media-recaptures/missing/"+endpoint, "{}")
			bridgeCommandsError(t, bridgeCommandsServe(f.s, r), 400, "invalid_request", "sql: no rows")
		})
	}
}
