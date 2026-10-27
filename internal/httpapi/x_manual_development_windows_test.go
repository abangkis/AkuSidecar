//go:build windows && auth_session_dev && auth_session_fixture

package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestXDevelopmentApplicationHTTPJourney(t *testing.T) {
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("canonical development entry helper required")
	}
	plan, err := engine.WriteXApplicationQASyntheticPlan(t.TempDir(), node, 28174)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(plan)
	if os.Mkdir(filepath.Join(root, "config"), 0700) != nil {
		t.Fatal("config directory unavailable")
	}
	opts := config.Options{Dev: true, DevelopmentSessionReuse: "native-x-manual", DevelopmentXManualPlan: plan, ConfigPath: filepath.Join(root, "config", "sidecar.json"), DatabasePath: filepath.Join(root, "state", "pilot.db"), Port: 28174}
	if os.WriteFile(opts.ConfigPath, []byte(`{}`), 0600) != nil {
		t.Fatal("config unavailable")
	}
	selection, err := engine.PrepareXManualDevelopmentSynthetic(opts)
	if err != nil {
		t.Fatal(err)
	}
	state, err := selection.OpenDatabase(domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Dev: true, Root: root, Server: config.ServerConfig{Host: "127.0.0.1", Port: opts.Port}, Database: config.DatabaseConfig{Path: opts.DatabasePath}, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}
	logger := log.New(io.Discard, "", 0)
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	if err = selection.Attach(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	s, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		s.splitCapture.close()
		runtime.Shutdown()
		if !runtime.WaitForIdle(3 * time.Second) {
			t.Error("synthetic reasoning did not drain before database close")
		}
		// Startup has completed and Engine is drained here. Close SQLite before
		// the final idempotent selection close so file-release checks are explicit.
		if err := state.Close(); err != nil {
			t.Error("database close failed", err)
		}
		if err := selection.Close(); err != nil {
			t.Error(err)
		}
		if err := os.Rename(state.Path(), state.Path()+".released"); err != nil {
			t.Error("closed synthetic database still locked", err)
		}
	})
	if s.splitCapture == nil {
		t.Fatal("ordinary dev server did not initialize broker transport")
	}
	token, err := state.BridgeToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request := func(route, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d%s", opts.Port, route), strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Aku-Split-Epoch", runtime.Epoch())
		r.Header.Set("X-Aku-Capture-Instance", s.splitCapture.key)
		r.Header.Set("X-Aku-Bridge-Token", token)
		r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		w := httptest.NewRecorder()
		s.security(s.api()).ServeHTTP(w, r)
		return w
	}
	consent := `{"acknowledgeLimitations":true,"allowOneSessionDerivation":true}`
	if w := request("/api/development/x-manual/start", consent); w.Code != 200 {
		t.Fatalf("start=%d %s", w.Code, w.Body)
	}
	session, err := state.ActiveSession(context.Background())
	if err != nil || session == nil {
		t.Fatal("started session missing", err)
	}
	if w := request("/api/development/x-manual/start", consent); w.Code != 409 {
		t.Fatal("start replay accepted", w.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := readerbroker.Request{RequestID: "broker_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Source: "x", URL: "https://x.com/example/status/123"}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- request("/api/split-capture/actions", `{"type":"open_native_post","source":"x","url":"`+req.URL+`","requestId":"`+req.RequestID+`"}`)
	}()
	if err = s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) {
		return readerbroker.Reply{OK: true, Readback: true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case w := <-done:
		var result splitActionResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || !result.OK {
			t.Fatalf("post=%d %s", w.Code, w.Body)
		}
	case <-ctx.Done():
		t.Fatal("reader action stalled")
	}
	// Wait for the completed capture/analysis journey. Canceled-query ownership
	// and file release are covered separately by the driver regression suite.
	for {
		completed, err := state.GetSession(context.Background(), session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if completed.Status == "completed" {
			break
		}
		if completed.Status == "failed" || completed.Status == "cancelled" {
			t.Fatal("synthetic reasoning failed", completed.Status)
		}
		select {
		case <-ctx.Done():
			t.Fatal("completed synthetic session did not settle")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !runtime.WaitForIdle(3 * time.Second) {
		t.Fatal("completed synthetic reasoning did not drain")
	}
	if w := request("/api/development/x-manual/stop", `{}`); w.Code != 200 {
		t.Fatalf("stop=%d %s", w.Code, w.Body)
	}
	if status := runtime.XManualStatus(); status.State != "stopped" || status.Cleanup != "verified" {
		t.Fatal(status)
	}
}
