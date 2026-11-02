package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

type httpMediaRecaptureFixture struct {
	sessionUpdatesFixture
	token string
	block domain.Block
	input domain.PassiveXMediaEvidence
}

func newHTTPMediaRecaptureFixture(t *testing.T) httpMediaRecaptureFixture {
	t.Helper()
	s, token := splitTestServer(t)
	db, err := sql.Open("sqlite", s.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	t.Cleanup(func() {
		s.engine.Shutdown()
		if !s.engine.WaitForIdle(3 * time.Second) {
			t.Error("media HTTP fixture did not drain")
		}
	})
	s.engine.RecordHeartbeat(engine.ExpectedHeartbeat())
	s.config.Bridge.TrustedExtensionOrigins = []string{"chrome-extension://abcdefghijklmnop"}
	native := "https://x.com/fixture/status/1234567890123456789"
	block := domain.Block{EvidenceKey: "x:status:1234567890123456789", PlatformID: "1234567890123456789", Permalink: native, Author: "Fixture", Text: "Offline HTTP media evidence", MediaRecovery: map[string]any{"outcome": "unavailable"}}
	f := httpMediaRecaptureFixture{sessionUpdatesFixture: sessionUpdatesFixture{s, db}, token: token, block: block, input: domain.PassiveXMediaEvidence{CandidateID: "x:status:1234567890123456789", Provenance: "passive_x_cache", Media: []domain.PassiveXMediaCandidate{{Kind: "image", URL: "https://pbs.twimg.com/media/http-fixture.jpg#ignored", Provenance: "x_response_graphql"}}}}
	raw, err := json.Marshal(domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('http-media-observation','http-media-run','http-media-command','x',?,?,?)`, string(raw), domain.Now(), domain.Now())
	item, err := json.Marshal(domain.ReasonedItem{Source: domain.SourceX, SourceURL: native, EvidenceKey: block.EvidenceKey})
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('http-media-item','http-media-session','http-media-run','x',?,0,?,'{}',?)`, block.EvidenceKey, string(item), domain.Now())
	return f
}

func (f httpMediaRecaptureFixture) request(method, target, body string, bridge bool) *http.Request {
	r := sessionUpdatesRequest(method, target, body)
	if bridge {
		r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		r.Header.Set("X-Aku-Bridge-Token", f.token)
		r.Header.Set("X-Aku-Bridge-Id", "  http-passive-bridge  ")
	}
	return r
}
func (f httpMediaRecaptureFixture) body(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
func (f httpMediaRecaptureFixture) setBlock(t *testing.T, block domain.Block) {
	t.Helper()
	raw := f.body(t, domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
	f.exec(t, `UPDATE observations SET observation_json=? WHERE id='http-media-observation'`, raw)
}
func (f httpMediaRecaptureFixture) noMutation(t *testing.T) {
	t.Helper()
	if f.count(t, `SELECT COUNT(*) FROM media_recaptures`) != 0 || f.count(t, `SELECT COUNT(*) FROM timeline_evidence_overrides`) != 0 {
		t.Fatal("rejected request persisted media work")
	}
}

func TestHTTPMediaRecaptureCharacterizationStatus(t *testing.T) {
	f := newHTTPMediaRecaptureFixture(t)
	job, err := f.s.store.CreateOwnedMediaRecapture(context.Background(), "http-media-item", domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, nil, "bridge")
	if err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{job.ID, job.ID + "/"} {
		payload := f.json(t, f.serve(f.request("GET", "/api/media-recaptures/"+suffix, "", false), true), 200)
		var projected map[string]json.RawMessage
		if err := json.Unmarshal(payload["recapture"], &projected); err != nil {
			t.Fatal(err)
		}
		if len(payload) != 1 || len(projected) != 4 || string(projected["status"]) != `"queued"` || string(projected["error"]) != "null" {
			t.Fatalf("status projection changed: %s", payload["recapture"])
		}
		for _, key := range []string{"id", "status", "outcome", "error"} {
			if _, ok := projected[key]; !ok {
				t.Fatal("missing status field", key)
			}
		}
		for _, key := range []string{"payload", "result", "targetUrl", "evidenceKey"} {
			if _, ok := projected[key]; ok {
				t.Fatal("private recapture field exposed", key)
			}
		}
	}
	for _, target := range []string{"/api/media-recaptures/missing", "/api/media-recaptures/nested/" + job.ID, "/api/media-recaptures/" + job.ID + "//"} {
		f.error(t, f.serve(f.request("GET", target, "", false), false), 404, "not_found", "media recapture not found")
	}
	f.s.store.Close()
	f.error(t, f.serve(f.request("GET", "/api/media-recaptures/"+job.ID, "", false), false), 500, "internal_error", "internal server error")
	f.error(t, f.serve(f.request("GET", "/api/media-recaptures/nested/id", "", false), false), 404, "not_found", "media recapture not found")
}

func TestHTTPMediaRecaptureCharacterizationEnqueue(t *testing.T) {
	for _, mode := range []string{"default", "missing_media", "unresolved_video", "playback_error", "foreground"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPMediaRecaptureFixture(t)
			reason, captureMode := mode, "background"
			if mode == "default" || mode == "foreground" {
				reason = ""
			}
			if mode == "unresolved_video" {
				block := f.block
				block.ContentKind = "video"
				block.Media = []map[string]any{{"kind": "video_poster", "url": "https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg"}}
				block.MediaRecovery = map[string]any{"expected": []string{"video"}, "outcome": "unresolved"}
				f.setBlock(t, block)
			}
			if mode == "playback_error" {
				block := f.block
				block.ContentKind = "video"
				block.Media = []map[string]any{{"kind": "video", "playbackMode": "inline", "playbackUrl": "https://video.twimg.com/ext_tw_video/12345/pu/vid/fixture.mp4"}}
				f.setBlock(t, block)
			}
			if mode == "foreground" {
				prior, err := f.s.store.CreateOwnedMediaRecapture(context.Background(), "http-media-item", domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, nil, "bridge")
				if err != nil {
					t.Fatal(err)
				}
				f.exec(t, `UPDATE media_recaptures SET status='completed',outcome='unavailable',completed_at=? WHERE id=?`, domain.Now(), prior.ID)
				captureMode = "foreground"
			}
			body := f.body(t, map[string]string{"captureMode": captureMode, "reason": reason})
			payload := f.json(t, f.serve(f.request("POST", "/api/timeline/nested/http-media-item/recapture/", body, false), true), 202)
			var got domain.MediaRecapture
			if err := json.Unmarshal(payload["recapture"], &got); err != nil {
				t.Fatal(err)
			}
			wantReason := reason
			if wantReason == "" {
				wantReason = "missing_media"
			}
			if len(payload) != 1 || got.ID == "" || got.Status != "queued" || got.Payload["reason"] != wantReason || got.Payload["foregroundAuthorized"] != (captureMode == "foreground") {
				t.Fatalf("enqueue response/defaults changed: %+v", got)
			}
			stored, err := f.s.store.MediaRecapture(context.Background(), got.ID)
			if err != nil || stored.ID != got.ID {
				t.Fatal("response preceded durable admission", err)
			}
		})
	}
}

func TestHTTPMediaRecaptureCharacterizationValidationAndRoutes(t *testing.T) {
	for _, tc := range []struct{ name, body, message string }{
		{"mode", `{"captureMode":"other"}`, "captureMode must be background or foreground"},
		{"reason", `{"captureMode":"background","reason":"other"}`, "reason must be missing_media, playback_error, or unresolved_video"},
		{"reason-whitespace", `{"captureMode":"background","reason":" "}`, "reason must be"},
		{"unknown", `{"captureMode":"background","extra":true}`, "unknown field"},
		{"type", `{"captureMode":1}`, "cannot unmarshal"},
		{"malformed", `{`, "request body must be valid JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newHTTPMediaRecaptureFixture(t)
			f.error(t, f.serve(f.request("POST", "/api/timeline/http-media-item/recapture", tc.body, false), false), 400, "invalid_request", tc.message)
			f.noMutation(t)
		})
	}
	f := newHTTPMediaRecaptureFixture(t)
	f.error(t, f.serve(f.request("POST", "/api/timeline/missing/recapture", `{"captureMode":"background"}`, false), false), 404, "not_found", "timeline item not found")
	f.error(t, f.serve(f.request("POST", "/api/timeline/http-media-item/recapture", `{"captureMode":"foreground"}`, false), false), 409, "conflict", "foreground recapture requires")
	r := f.request("POST", "/api/timeline/http-media-item/recapture", `{}`, false)
	r.Header.Set("Content-Type", "text/plain")
	f.error(t, f.serve(r, false), 415, "unsupported_media_type", "Content-Type must be application/json")
	for _, tc := range []struct{ method, target string }{
		{"POST", "/api/media-recaptures/id"}, {"GET", "/api/timeline/id/recapture"}, {"POST", "/api/timeline/id/recapture//"}, {"GET", "/api/bridge/timeline/id/media-evidence"}, {"POST", "/api/bridge/timeline/id/media-evidence//"},
	} {
		f.error(t, f.serve(f.request(tc.method, tc.target, `{}`, true), false), 404, "not_found", "route not found")
	}
	f.noMutation(t)
}

func TestHTTPMediaRecaptureCharacterizationPassiveAuthenticationAndReplay(t *testing.T) {
	for _, guard := range []string{"contract", "token", "decode"} {
		t.Run(guard, func(t *testing.T) {
			f := newHTTPMediaRecaptureFixture(t)
			r := f.request("POST", "/api/bridge/timeline/http-media-item/media-evidence", `{`, true)
			status, code, message := 401, "invalid_bridge_contract", "unsupported Bridge contract"
			if guard == "contract" {
				r.Header.Del("X-Aku-Bridge-Contract")
			}
			if guard == "token" {
				r.Header.Set("X-Aku-Bridge-Token", "invalid")
				code, message = "invalid_bridge_token", "invalid Bridge token"
			}
			if guard == "decode" {
				status, code, message = 400, "invalid_request", "request body must be valid JSON"
			}
			f.error(t, f.serve(r, false), status, code, message)
			f.noMutation(t)
		})
	}
	f := newHTTPMediaRecaptureFixture(t)
	body := f.body(t, f.input)
	for _, replay := range []bool{false, true} {
		r := f.request("POST", "/api/bridge/timeline/nested/http-media-item/media-evidence/", body, true)
		r.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
		if replay {
			r.Header.Set("X-Aku-Capture-Instance", "wrong-instance")
		}
		payload := f.json(t, f.serve(r, true), 200)
		if replay {
			if len(payload) != 1 || string(payload["updated"]) != "false" {
				t.Fatal("no-op response changed", payload)
			}
		} else {
			if len(payload) != 2 || string(payload["updated"]) != "true" {
				t.Fatal("updated response changed", payload)
			}
			var job domain.MediaRecapture
			if err := json.Unmarshal(payload["recapture"], &job); err != nil || job.Status != "completed" || job.Payload["browserOperation"] != "none" {
				t.Fatal("passive result changed", job, err)
			}
			var claimed string
			if err := f.db.QueryRow(`SELECT claimed_by FROM media_recaptures WHERE id=?`, job.ID).Scan(&claimed); err != nil || claimed != "http-passive-bridge" {
				t.Fatal("Bridge id forwarding changed", claimed, err)
			}
		}
	}
	if f.count(t, `SELECT COUNT(*) FROM media_recaptures`) != 1 || f.count(t, `SELECT COUNT(*) FROM timeline_evidence_overrides`) != 1 {
		t.Fatal("passive replay duplicated durable work")
	}
}

func TestHTTPMediaRecaptureCharacterizationPassiveValidation(t *testing.T) {
	for _, mode := range []string{"text-field", "provenance", "identity", "untrusted", "missing"} {
		t.Run(mode, func(t *testing.T) {
			f := newHTTPMediaRecaptureFixture(t)
			input := f.input
			target := "/api/bridge/timeline/http-media-item/media-evidence"
			status, code, message := 400, "invalid_request", ""
			switch mode {
			case "provenance":
				input.Provenance = "other"
				message = "passive media provenance must be passive_x_cache"
			case "identity":
				input.CandidateID = "x:status:9876543210987654321"
				message = "candidateId does not match"
			case "untrusted":
				input.Media = []domain.PassiveXMediaCandidate{{Kind: "image", URL: "https://untrusted.example/image.jpg"}}
				message = "media URL is invalid"
			case "missing":
				target = "/api/bridge/timeline/missing/media-evidence"
				status, code, message = 404, "not_found", "timeline item not found"
			}
			body := f.body(t, input)
			if mode == "text-field" {
				body = `{"candidateId":"x:status:1234567890123456789","provenance":"passive_x_cache","media":[{"kind":"image","url":"https://pbs.twimg.com/media/x.jpg","alt":"private text"}]}`
				message = "unknown field"
			}
			f.error(t, f.serve(f.request("POST", target, body, true), false), status, code, message)
			f.noMutation(t)
		})
	}
}

func TestHTTPMediaRecaptureCharacterizationSecurity(t *testing.T) {
	for _, endpoint := range []struct {
		method, target string
		bridge         bool
	}{{"GET", "/api/media-recaptures/missing", false}, {"POST", "/api/timeline/http-media-item/recapture", false}, {"POST", "/api/bridge/timeline/http-media-item/media-evidence", true}} {
		for _, guard := range []string{"host", "origin"} {
			t.Run(endpoint.target+guard, func(t *testing.T) {
				f := newHTTPMediaRecaptureFixture(t)
				r := f.request(endpoint.method, endpoint.target, `{}`, endpoint.bridge)
				if guard == "host" {
					r.Host = "foreign.example"
				} else {
					r.Header.Set("Origin", "https://foreign.example")
				}
				w := f.serve(r, true)
				if w.Code != 403 {
					t.Fatal("security boundary changed", w.Code, w.Body.String())
				}
				f.noMutation(t)
			})
		}
		if !endpoint.bridge {
			f := newHTTPMediaRecaptureFixture(t)
			r := f.request(endpoint.method, endpoint.target, `{}`, false)
			r.Header.Set("Origin", "chrome-extension://abcdefghijklmnop")
			if w := f.serve(r, true); w.Code != 403 {
				t.Fatal("extension gained UI route access", w.Code)
			}
		}
	}
}

type httpRecaptureCancelBody struct {
	io.Reader
	cancel context.CancelFunc
}

func (b httpRecaptureCancelBody) Close() error { b.cancel(); return nil }

func TestHTTPMediaRecaptureCharacterizationContextAndStoreFailures(t *testing.T) {
	for _, phase := range []string{"status-cancelled", "enqueue-cancelled", "passive-auth-cancelled", "enqueue-after-decode", "passive-after-decode", "enqueue-write-fault", "passive-write-fault"} {
		t.Run(phase, func(t *testing.T) {
			f := newHTTPMediaRecaptureFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			method, target, body, bridge := "POST", "/api/timeline/http-media-item/recapture", `{"captureMode":"background"}`, false
			status, code, message := 409, "conflict", "context canceled"
			if strings.HasPrefix(phase, "passive") {
				target, body, bridge = "/api/bridge/timeline/http-media-item/media-evidence", f.body(t, f.input), true
				status, code = 400, "invalid_request"
			}
			if phase == "status-cancelled" {
				method, target, body = "GET", "/api/media-recaptures/missing", ""
				status, code, message = 500, "internal_error", "internal server error"
			}
			if phase == "passive-auth-cancelled" {
				status, code, message = 401, "invalid_bridge_token", "invalid Bridge token"
			}
			if strings.HasSuffix(phase, "cancelled") {
				cancel()
			}
			if phase == "enqueue-write-fault" {
				f.exec(t, `CREATE TRIGGER http_enqueue_fault BEFORE INSERT ON media_recaptures BEGIN SELECT RAISE(ABORT,'http-enqueue-fault'); END`)
				message = "http-enqueue-fault"
			}
			if phase == "passive-write-fault" {
				f.exec(t, `CREATE TRIGGER http_passive_fault BEFORE INSERT ON timeline_evidence_overrides BEGIN SELECT RAISE(ABORT,'http-passive-fault'); END`)
				message = "http-passive-fault"
			}
			r := f.request(method, target, body, bridge).WithContext(ctx)
			if strings.HasSuffix(phase, "after-decode") {
				r.Body = httpRecaptureCancelBody{strings.NewReader(body), cancel}
			}
			f.error(t, f.serve(r, false), status, code, message)
			f.noMutation(t)
		})
	}
}

func TestHTTPMediaRecaptureCharacterizationResponseWriteFailure(t *testing.T) {
	for _, passive := range []bool{false, true} {
		name := "enqueue"
		if passive {
			name = "passive"
		}
		t.Run(name, func(t *testing.T) {
			f := newHTTPMediaRecaptureFixture(t)
			target, body, status := "/api/timeline/http-media-item/recapture", `{"captureMode":"background"}`, 202
			if passive {
				target, body, status = "/api/bridge/timeline/http-media-item/media-evidence", f.body(t, f.input), 200
			}
			failure := errors.New("media-response-write-failed")
			w := &sessionUpdatesFailWriter{header: http.Header{}, err: failure}
			if err := f.s.route(w, f.request("POST", target, body, passive)); !errors.Is(err, failure) || w.status != status {
				t.Fatal("response write error changed", w.status, err)
			}
			if f.count(t, `SELECT COUNT(*) FROM media_recaptures`) != 1 {
				t.Fatal("response failure lost durable admission")
			}
			if passive && f.count(t, `SELECT COUNT(*) FROM timeline_evidence_overrides`) != 1 {
				t.Fatal("response failure lost durable evidence")
			}
		})
	}
}
