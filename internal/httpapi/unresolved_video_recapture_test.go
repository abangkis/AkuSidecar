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
	native := "https://x.com/ESPNAsia/status/2108207027298435417"
	key := "x:status:2108207027298435417"
	now := domain.Now()
	block := domain.Block{EvidenceKey: key, PlatformID: key, Permalink: native, ContentKind: "video", Media: []map[string]any{{"kind": "video_poster", "url": "https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg"}}, MediaRecovery: map[string]any{"expected": []string{"video"}, "outcome": "unresolved"}}
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
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/timeline/item/recapture", strings.NewReader(`{"captureMode":"background","reason":"unresolved_video"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.http.Handler.ServeHTTP(response, request)
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
	playback := "https://video.twimg.com/ext_tw_video/12345/pu/vid/clip.mp4"
	block.Media = []map[string]any{{"kind": "video", "posterUrl": "https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg", "playbackMode": "inline", "playbackUrl": playback}}
	job, err := state.CompleteMediaRecapture(ctx, body.Recapture.ID, domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}}})
	if err != nil {
		t.Fatal(err)
	}
	if job.Outcome != "recovered" {
		t.Fatal(job)
	}
	items, err := state.ListTimeline(ctx, 24, 0)
	if err != nil || len(items) != 1 || items[0].Evidence.Media[0]["playbackUrl"] != playback {
		t.Fatalf("updated timeline=%+v err=%v", items, err)
	}
}
