//go:build windows

package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicitly gated capability check, not production reader/auto-return parity.
// Uses no social URLs, logged-in profile, Bridge, or foreground activation call.
func TestHostlessReaderPrivateCDPCapability(t *testing.T) {
	if os.Getenv("AKU_HOSTLESS_READER_SMOKE_ALLOW_FOREGROUND") != "1" {
		t.Skip("requires explicit foreground approval and a Chrome path")
	}
	chrome := os.Getenv("AKU_HOSTLESS_READER_SMOKE_CHROME_PATH")
	if !filepath.IsAbs(chrome) {
		t.Fatal("an explicit absolute Chrome executable is required")
	}
	if info, err := os.Stat(chrome); err != nil || info.IsDir() {
		t.Fatal("Chrome executable unavailable")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "build", fmt.Sprintf("hostless-reader-smoke-%d-%d", os.Getpid(), time.Now().UnixNano())))
	if err != nil {
		t.Fatal(err)
	}
	profile := filepath.Join(root, "empty-profile")
	if err := os.MkdirAll(profile, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/marker" {
			_, _ = io.WriteString(w, "<!doctype html><title>AkuBrowser reader split_hostless_poc</title><p>Local reader marker</p>")
		} else {
			_, _ = io.WriteString(w, "<!doctype html><title>Local native post fixture</title><p>No social data</p>")
		}
	}))
	defer server.Close()
	started := time.Now()
	window, err := Launch(ctx, LaunchOptions{Executable: chrome, UserDataDir: profile,
		URL: server.URL + "/marker", PrivateCDP: true, NormalWindow: true,
		ExtraArgs: []string{"--disable-extensions"}})
	t.Logf("stage=chrome_launch elapsed_ms=%d", time.Since(started).Milliseconds())
	if err != nil {
		t.Fatal(err)
	}
	// Whole-process cleanup is scoped to this new empty-profile fixture only.
	defer window.Terminate()
	containment, err := window.StartCaptureContainment(log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	target, _, err := containment.PrepareBrokerReader(ctx, "AkuBrowser reader split_hostless_poc")
	t.Logf("stage=window_binding elapsed_ms=%d", time.Since(started).Milliseconds())
	if err != nil || target.HWND == 0 || target.PID == 0 {
		t.Fatalf("hostless marker could not bind an owned reader: %v", err)
	}
	iconic, _, _ := isCaptureIconic.Call(uintptr(target.HWND))
	if iconic != 0 {
		t.Fatal("normal reader window is minimized")
	}
	protocol := window.CaptureProtocol()
	raw, err := protocol.Call(ctx, "Target.getTargets", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var targets struct {
		Infos []struct {
			ID   string `json:"targetId"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"targetInfos"`
	}
	if err := json.Unmarshal(raw, &targets); err != nil {
		t.Fatal(err)
	}
	id := ""
	pages := 0
	for _, item := range targets.Infos {
		if item.Type == "page" {
			pages++
			if item.URL == server.URL+"/marker" {
				id = item.ID
			}
		}
	}
	if pages != 1 || id == "" {
		t.Fatal("expected exactly the owned local reader page and no capture host")
	}
	raw, err = protocol.Call(ctx, "Target.attachToTarget", map[string]any{"targetId": id, "flatten": true}, "")
	if err != nil {
		t.Fatal(err)
	}
	var attached struct {
		ID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &attached); err != nil || attached.ID == "" {
		t.Fatal("reader CDP session unavailable")
	}
	if _, err := protocol.Call(ctx, "Page.navigate", map[string]string{"url": server.URL + "/post"}, attached.ID); err != nil {
		t.Fatal(err)
	}
	if err := containment.(interface{ ReplacementReadiness(context.Context) error }).ReplacementReadiness(ctx); err == nil {
		t.Fatal("an open native reader must retain profile ownership")
	}
	raw, err = protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": id}, "")
	var closed struct {
		Success bool `json:"success"`
	}
	if err != nil || json.Unmarshal(raw, &closed) != nil || !closed.Success {
		t.Fatal("owned local reader close was not acknowledged")
	}
	// Do not mistake close ACK for process/profile release.
	select {
	case <-window.Done():
	case <-ctx.Done():
		t.Fatal("Chrome did not naturally exit after its only reader closed")
	}
	if err := window.CloseForRetry(ctx); err != nil {
		t.Fatal("reader process tree cleanup unverified")
	}
	_ = os.WriteFile(filepath.Join(root, "receipt.json"), []byte(`{"localReaderOnly":true,"ownedHWNDBound":true,"profileHeldWhileOpen":true,"naturalExitVerified":true}`), 0600)
}
