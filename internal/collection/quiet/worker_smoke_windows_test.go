//go:build windows

package quiet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Explicit disposable-profile smoke. Never borrows signed-in profiles or stops
// the running Sidecar. New fixture windows are immediately minimized via CDP.
func TestQuietWorkerIsolationWindowsSmoke(t *testing.T) {
	runtimeRoot, chrome, bridge := os.Getenv("AKU_QUIET_SMOKE_RUNTIME"), os.Getenv("AKU_QUIET_SMOKE_CHROME"), os.Getenv("AKU_QUIET_SMOKE_BRIDGE")
	if runtimeRoot == "" || chrome == "" || bridge == "" {
		t.Skip("explicit pinned Node/Chrome/Bridge paths required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "aku_quiet_fixture", Value: "shared", Path: "/"})
		_, _ = fmt.Fprintf(w, "<!doctype html><title>Quiet fixture</title><main data-fixture=%q>fixture</main>", r.URL.Path)
	}))
	defer fixture.Close()
	profile := filepath.Join(t.TempDir(), "profile")
	hostURL := fixture.URL + "/host"
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{Executable: chrome, ExtensionPath: bridge, UserDataDir: profile, URL: hostURL, StartMinimized: true, PrivateCDP: true, ExtraArgs: []string{"--start-minimized"}})
	if err != nil {
		t.Fatal("launch disposable private capture", err)
	}
	defer window.Terminate()
	protocol := window.CaptureProtocol()
	if err := appshell.NavigateCaptureHost(ctx, protocol, hostURL); err != nil {
		t.Fatal(err)
	}
	list := func() map[string]string {
		raw, err := protocol.Call(ctx, "Target.getTargets", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			TargetInfos []struct {
				TargetID string `json:"targetId"`
				URL      string `json:"url"`
			} `json:"targetInfos"`
		}
		if json.Unmarshal(raw, &v) != nil {
			t.Fatal("invalid target listing")
		}
		out := map[string]string{}
		for _, target := range v.TargetInfos {
			out[target.TargetID] = target.URL
		}
		return out
	}
	hostID := ""
	for id, url := range list() {
		if url == hostURL {
			if hostID != "" {
				t.Fatal("ambiguous host")
			}
			hostID = id
		}
	}
	if hostID == "" {
		t.Fatal("host missing")
	}
	decodeID := func(raw json.RawMessage, key string) string {
		var value map[string]any
		if json.Unmarshal(raw, &value) != nil {
			t.Fatal("invalid ID response")
		}
		id, _ := value[key].(string)
		if id == "" {
			t.Fatal("missing fixture target ID")
		}
		return id
	}
	raw, err := protocol.Call(ctx, "Target.createTarget", map[string]any{"url": fixture.URL + "/interactive", "newWindow": true, "background": true}, "")
	if err != nil {
		t.Fatal("create independent interactive fixture", err)
	}
	interactiveID := decodeID(raw, "targetId")
	raw, err = protocol.Call(ctx, "Browser.getWindowForTarget", map[string]string{"targetId": interactiveID}, "")
	if err != nil {
		t.Fatal("interactive native window absent", err)
	}
	var native struct {
		WindowID int `json:"windowId"`
	}
	if json.Unmarshal(raw, &native) != nil || native.WindowID == 0 {
		t.Fatal("invalid native fixture window")
	}
	if _, err := protocol.Call(ctx, "Browser.setWindowBounds", map[string]any{"windowId": native.WindowID, "bounds": map[string]string{"windowState": "minimized"}}, ""); err != nil {
		t.Fatal("minimize fixture window", err)
	}
	targets, err := NewTargets(ctx, protocol)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = targets.Close(cleanup)
	}()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceFacebook} {
		if _, err := targets.Call(ctx, source, "Page.navigate", map[string]string{"url": fixture.URL + "/hidden"}); err != nil {
			t.Fatal(err)
		}
	}
	xID, fbID := targets.owned[domain.SourceX].targetID, targets.owned[domain.SourceFacebook].targetID
	if xID == fbID {
		t.Fatal("sources shared a target")
	}
	for _, id := range []string{xID, fbID} {
		if _, err := protocol.Call(ctx, "Browser.getWindowForTarget", map[string]string{"targetId": id}, ""); err == nil {
			t.Fatal("hidden collector acquired native window")
		}
	}
	shared := false
	for until := time.Now().Add(3 * time.Second); time.Now().Before(until); {
		raw, err := targets.Call(ctx, domain.SourceX, "Runtime.evaluate", map[string]any{"expression": "document.cookie.includes('aku_quiet_fixture=shared')", "returnByValue": true})
		var result struct {
			Result struct {
				Value bool `json:"value"`
			} `json:"result"`
		}
		if err == nil && json.Unmarshal(raw, &result) == nil && result.Result.Value {
			shared = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !shared {
		t.Fatal("hidden default-context cookie continuity failed")
	}
	workerPath, err := filepath.Abs(filepath.Join("..", "headless", "worker", "worker.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(targets, headless.Options{Node: filepath.Join(runtimeRoot, "node.exe"), Pin: filepath.Join(runtimeRoot, "node.pin.json"), Worker: workerPath, Chrome: chrome, Profile: profile, BridgePath: bridge})
	if err := worker.start(ctx); err != nil {
		t.Fatal(err)
	}
	worker.owner.Terminate() // Simulate extraction worker failure, never capture Chrome.
	select {
	case <-worker.owner.Closed():
	case <-ctx.Done():
		t.Fatal("Node owner did not drain")
	}
	if _, err := worker.Capture(ctx, domain.SourceX, map[string]any{}); err == nil {
		t.Fatal("dead worker accepted capture")
	}
	remaining := list()
	if _, ok := remaining[xID]; ok {
		t.Fatal("X hidden target survived worker cleanup")
	}
	if _, ok := remaining[fbID]; ok {
		t.Fatal("Facebook hidden target survived worker cleanup")
	}
	if remaining[interactiveID] != fixture.URL+"/interactive" || remaining[hostID] != hostURL {
		t.Fatal("worker failure affected ordinary fixture targets")
	}
	if _, err := protocol.Call(ctx, "Browser.getVersion", nil, ""); err != nil {
		t.Fatal("worker failure closed root pipe")
	}
	select {
	case <-window.Done():
		t.Fatal("worker failure exited capture Chrome")
	default:
	}
	// Exact host closure must leave the independent fixture window and owner alive.
	if _, err := protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": hostID}, ""); err != nil {
		t.Fatal(err)
	}
	if list()[interactiveID] != fixture.URL+"/interactive" {
		t.Fatal("static host close lost interactive fixture")
	}
	if err := window.SetCaptureHandoff(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": interactiveID}, ""); err != nil {
		t.Fatal(err)
	}
	if err := window.CloseForRetry(ctx); err != nil {
		t.Fatal("natural owner drain after final window close", err)
	}
	t.Log("hidden_targets=2 cookie_shared=true node_failure_contained=true interactive_window_retained=true root_pipe_retained=true natural_owner_drain=true")
}
