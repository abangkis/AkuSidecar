//go:build windows

package appshell_test

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

// A diagnostic, not a release acceptance test: inspect the initial document,
// hidden-target capability, and one explicit host navigation in an owned,
// minimized Chrome with a disposable project-local profile. The debugging
// WebSocket is test-only; this does not choose a production transport.
func TestHiddenTargetCapabilityWindowsSmoke(t *testing.T) {
	chrome := os.Getenv("AKU_HIDDEN_TARGET_SMOKE_CHROME")
	node := os.Getenv("AKU_HIDDEN_TARGET_SMOKE_NODE")
	bridge := os.Getenv("AKU_HIDDEN_TARGET_SMOKE_BRIDGE")
	if chrome == "" || node == "" || bridge == "" {
		t.Skip("explicit packaged Chrome, Node and Bridge paths required")
	}
	for _, path := range []string{chrome, node, bridge} {
		if !filepath.IsAbs(path) {
			t.Fatal("smoke paths must be absolute")
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatal("packaged smoke path unavailable")
		}
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve project-local fixture directory")
	}
	artifactRoot := filepath.Clean(filepath.Join(filepath.Dir(source), "..", "..", "build", "hidden-target-capability-smoke"))
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("create project-local fixture directory")
	}
	profile, err := os.MkdirTemp(artifactRoot, "isolated-")
	if err != nil {
		t.Fatal("create disposable fixture profile")
	}
	defer os.RemoveAll(profile)
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	var startupHits, forcedHits, hiddenHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch r.URL.Path {
		case "/host":
			if r.URL.Query().Get("force") == "1" {
				forcedHits.Add(1)
			} else {
				startupHits.Add(1)
			}
			// Only the visible host seeds this harmless fixture cookie.
			http.SetCookie(w, &http.Cookie{Name: "aku_smoke_fixture", Value: "isolated", Path: "/", SameSite: http.SameSiteLaxMode})
			_, _ = io.WriteString(w, "<!doctype html><title>AkuBrowser capture host</title><main data-aku-smoke='host'>Static isolated host</main>")
		case "/hidden":
			hiddenHits.Add(1)
			_, _ = io.WriteString(w, "<!doctype html><title>Hidden collector fixture</title><main data-aku-smoke='hidden'>Static hidden collector</main>")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{
		Executable: chrome, ExtensionPath: bridge, UserDataDir: profile,
		URL: server.URL + "/host", StartMinimized: true,
		ExtraArgs: []string{"--start-minimized", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic whole-fixture cleanup is explicitly separate from retirement.
	defer window.Terminate()
	if _, err := window.StartCaptureContainment(log.New(io.Discard, "", 0)); err != nil {
		t.Fatal(err)
	}
	endpoint := smokeDevToolsEndpoint(t, ctx, profile)
	command := exec.CommandContext(ctx, node, filepath.Join("testdata", "hidden_target_capability_cdp.mjs"), endpoint, server.URL)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.CombinedOutput()
	if err != nil {
		// Never echo raw diagnostic output, endpoint, URLs, or document content.
		t.Fatalf("isolated capability helper failed: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatal("capability helper did not return its sanitized diagnostic")
	}
	result["startupHTTPHits"] = startupHits.Load()
	result["forcedHostHTTPHits"] = forcedHits.Load()
	result["hiddenHTTPHits"] = hiddenHits.Load()
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(encoded))
	if result["probeState"] != "complete" {
		t.Fatal("isolated capability probe incomplete; consult sanitized states above")
	}
}
