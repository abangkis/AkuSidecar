package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestUnresolvedVideoHTTPRecaptureReplacesPosterWithPlayback(t *testing.T) {
	for _, poster := range []string{"https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg", "https://pbs.twimg.com/media/GlKfj-9W4AAMzDO.jpg"} {
		t.Run(poster, func(t *testing.T) {
			testUnresolvedVideoHTTPRecaptureWithPlayback(t, "https://x.com/ESPNAsia/status/2108207027298435417", poster, "https://video.twimg.com/ext_tw_video/12345/pu/vid/clip.mp4", "recovered")
		})
	}
}

func TestUnresolvedXHLSRecaptureProjectsOwnedPlaylistInTimeline(t *testing.T) {
	const native = "https://x.com/Ananth7e/status/2119482362614677566"
	const poster = "https://pbs.twimg.com/amplify_video_thumb/2119481355943948288/img/fixture.jpg"
	const asset = "https://video.twimg.com/amplify_video/2119481355943948288/"
	for _, tc := range []struct{ name, playlist, outcome string }{
		{"owned master", asset + "pl/fixture.m3u8?tag=12", "recovered"},
		{"foreign asset", "https://video.twimg.com/amplify_video/2119471570100113408/pl/fixture.m3u8", "unavailable"},
		{"variant only", asset + "pl/avc1/640x360/variant.m3u8", "unavailable"},
		{"untrusted host", "https://video.twimg.com.example.com/amplify_video/2119481355943948288/pl/fixture.m3u8", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testUnresolvedVideoHTTPRecaptureWithPlayback(t, native, poster, tc.playlist, tc.outcome)
		})
	}
}

func testUnresolvedVideoHTTPRecaptureWithPlayback(t *testing.T, native, poster, playback, outcome string) {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "video.db")
	state, err := store.Open(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := "x:status:" + native[strings.LastIndex(native, "/")+1:]
	now := domain.Now()
	block := domain.Block{EvidenceKey: key, PlatformID: key, Permalink: native, ContentKind: "video", Media: []map[string]any{{"kind": "video_poster", "url": poster}}, MediaRecovery: map[string]any{"expected": []string{"video"}, "outcome": "unresolved"}}
	raw, _ := json.Marshal(domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
	if _, err = db.Exec(`INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES('obs','run','cmd','x',?,?,?)`, string(raw), now, now); err != nil {
		t.Fatal(err)
	}
	item, _ := json.Marshal(domain.ReasonedItem{Source: domain.SourceX, SourceURL: native, EvidenceKey: key})
	if _, err = db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('item','session','run','x',?,0,?,'{}',?)`, key, string(item), now); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	defer runtime.Shutdown()
	runtime.RecordHeartbeat(engine.ExpectedHeartbeat())
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	requestUnresolved := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/timeline/item/recapture", strings.NewReader(`{"captureMode":"background","reason":"unresolved_video"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(response, request)
		return response
	}
	response := requestUnresolved()
	if response.Code != http.StatusAccepted {
		t.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Recapture domain.MediaRecapture `json:"recapture"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Recapture.Payload["reason"] != "unresolved_video" {
		t.Fatal(body.Recapture.Payload)
	}
	if _, err = state.ClaimMediaRecapture(ctx, body.Recapture.ID, "fixture"); err != nil {
		t.Fatal(err)
	}
	block.Media = []map[string]any{{"kind": "video", "posterUrl": poster, "playbackMode": "inline", "playbackUrl": playback}}
	token, err := state.BridgeToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"observation": domain.Observation{
		Source: domain.SourceX, CapturedAt: now, Coverage: map[string]any{"quality": "complete"},
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	bridge := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/bridge/media-recaptures/"+body.Recapture.ID+"/observation", strings.NewReader(string(payload)))
	bridge.Header.Set("Content-Type", "application/json")
	bridge.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	bridge.Header.Set("X-Aku-Bridge-Token", token)
	result := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(result, bridge)
	err = json.Unmarshal(result.Body.Bytes(), &body)
	if result.Code != http.StatusOK || err != nil || body.Recapture.Outcome != outcome {
		t.Fatalf("Bridge submission status=%d body=%s err=%v", result.Code, result.Body.String(), err)
	}
	timeline := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(timeline, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/timeline", nil))
	var timelineBody struct {
		Items []domain.TimelineItem `json:"items"`
	}
	err = json.Unmarshal(timeline.Body.Bytes(), &timelineBody)
	if timeline.Code != http.StatusOK || err != nil || len(timelineBody.Items) != 1 {
		t.Fatalf("updated timeline status=%d body=%s err=%v", timeline.Code, timeline.Body.String(), err)
	}
	media := timelineBody.Items[0].Evidence.Media[0]
	if outcome == "recovered" && (media["playbackUrl"] != playback || media["playbackMode"] != "inline") {
		t.Fatalf("Timeline lost the recovered playback: %v", media)
	}
	if outcome == "unavailable" && (media["playbackUrl"] != nil || media["playbackMode"] != "native") {
		t.Fatalf("Timeline published unowned playback: %v", media)
	}
	if media["posterUrl"] != poster {
		t.Fatal("recapture changed the owned video poster")
	}
	if outcome == "recovered" {
		retry := requestUnresolved()
		if retry.Code != http.StatusConflict || !strings.Contains(retry.Body.String(), "does not have unresolved captured video") {
			t.Fatalf("resolved video remained eligible for unresolved recapture: %d %s", retry.Code, retry.Body.String())
		}
	}
}
