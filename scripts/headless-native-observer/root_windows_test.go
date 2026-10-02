//go:build windows

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

func TestExactRootSeesDisposableMinimizedChromeHost(t *testing.T) {
	chrome := os.Getenv("AKU_OBSERVER_FIXTURE_CHROME")
	if chrome == "" {
		t.Skip("explicit disposable native fixture requested only")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<!doctype html><title>Passive observer fixture</title><p>Fixture</p>"))
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{Executable: chrome, UserDataDir: filepath.Join(t.TempDir(), "profile"), URL: server.URL,
		StartMinimized: true, PrivateCDP: true, ExtraArgs: []string{"--start-minimized"}})
	if err != nil {
		t.Fatal(err)
	}
	defer window.Terminate() // This test owns its disposable profile and all fixture windows.
	sample := readRoot(uint32(window.PID()))
	if sample.Status != "available" || !sample.ScanComplete {
		t.Fatal("root scan unavailable")
	}
	if sample.Visible < 1 || sample.Exposed != 0 {
		t.Fatal("exact root should include its minimized host without an exposed window")
	}
	t.Log("Exact root scan observed the known minimized Chrome fixture despite unrelated Chrome inventory")
}
