//go:build windows

package appshell_test

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

// Explicit local diagnostic for minimized Chrome app-mode navigation. It uses
// random loopback ports and disposable project-local profiles only.
func TestCaptureNavigationVisibilityWindowsSmoke(t *testing.T) {
	chrome := os.Getenv("AKU_BRIDGE_HANDOFF_SMOKE_CHROME")
	bridge := os.Getenv("AKU_BRIDGE_HANDOFF_SMOKE_BRIDGE")
	if chrome == "" || bridge == "" {
		t.Skip("explicit staged Chrome and packaged Bridge paths required")
	}
	var err error
	chrome, err = filepath.Abs(chrome)
	if err != nil {
		t.Fatal("resolve staged Chrome path")
	}
	bridge, err = filepath.Abs(bridge)
	if err != nil {
		t.Fatal("resolve packaged Bridge path")
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve project-local smoke directory")
	}
	artifactRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "build", "capture-navigation-smoke"))
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("create project-local smoke directory")
	}
	variants := []struct {
		name string
		args []string
	}{
		{name: "baseline", args: []string{"--start-minimized"}},
		{name: "disable-calculate-native-win-occlusion", args: []string{"--start-minimized", "--disable-features=CalculateNativeWinOcclusion"}},
	}
	for _, variant := range variants {
		loaded, hits, err := runCaptureNavigationVariant(artifactRoot, chrome, bridge, variant.args)
		if err != nil {
			t.Fatalf("variant=%s navigation diagnostic failed: %v", variant.name, err)
		}
		t.Logf("variant=%s hits=%d loaded=%t", variant.name, hits, loaded)
	}
}

func runCaptureNavigationVariant(artifactRoot, chrome, bridge string, extraArgs []string) (bool, int32, error) {
	profile, err := os.MkdirTemp(artifactRoot, "capture-navigation-")
	if err != nil {
		return false, 0, err
	}
	defer os.RemoveAll(profile)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/" {
			hits.Add(1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, "<!doctype html><title>Capture navigation probe</title><main>Fixed local page</main>")
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{
		Executable: chrome, ExtensionPath: bridge, UserDataDir: profile,
		URL: server.URL + "/", StartMinimized: true, ExtraArgs: extraArgs,
	})
	if err != nil {
		if ctx.Err() != nil {
			return false, hits.Load(), nil
		}
		return false, hits.Load(), err
	}
	defer window.Terminate()
	if _, err := window.StartCaptureContainment(log.New(io.Discard, "", 0)); err != nil {
		return false, hits.Load(), err
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for hits.Load() == 0 {
		select {
		case <-ctx.Done():
			return false, hits.Load(), nil
		case <-ticker.C:
		}
	}
	return true, hits.Load(), nil
}
