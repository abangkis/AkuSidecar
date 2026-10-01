//go:build windows

package appshell

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

const cdpPipeSmokeChromeVersion = "Chrome/152.0.7977.54"

func TestCDPPipeWindowsSmoke(t *testing.T) {
	chrome := os.Getenv("AKU_CDP_PIPE_SMOKE_CHROME")
	if chrome == "" {
		t.Skip("explicit staged Chrome path required")
	}
	chrome, err := filepath.Abs(chrome)
	if err != nil {
		t.Fatal("resolve staged Chrome path")
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve project-local smoke directory")
	}
	artifactRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "build", "cdp-pipe-smoke"))
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("create project-local smoke directory")
	}
	profile, err := os.MkdirTemp(artifactRoot, "profile-")
	if err != nil {
		t.Fatal("create disposable Chrome profile")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	hostServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pipe-host" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<!doctype html><title>CDP pipe smoke</title><p>fixture</p>"))
	}))
	defer hostServer.Close()
	hostURL := hostServer.URL + "/pipe-host"

	window, err := Launch(ctx, LaunchOptions{
		Executable:     chrome,
		UserDataDir:    profile,
		URL:            hostURL,
		StartMinimized: true,
		PrivateCDP:     true,
	})
	if err != nil {
		t.Fatal("launch managed Chrome with private CDP")
	}
	complete, cleaned := false, false
	t.Cleanup(func() {
		if !complete {
			window.Terminate() // Disposable test profile; production never uses this path for handoff.
			select {
			case <-window.Done():
				cleaned = true
			case <-time.After(10 * time.Second):
				t.Errorf("test Chrome did not finish owned cleanup")
			}
		} else {
			cleaned = true
		}
		if cleaned {
			_ = os.RemoveAll(profile)
		}
	})
	protocol := window.CaptureProtocol()
	if protocol == nil {
		t.Fatal("managed Window did not expose its opt-in capture protocol")
	}
	versionResult, err := protocol.Call(ctx, "Browser.getVersion", nil, "")
	if err != nil {
		t.Fatal("read Chrome version through Window capture protocol")
	}
	var version struct {
		Product string `json:"product"`
	}
	if err := json.Unmarshal(versionResult, &version); err != nil || version.Product != cdpPipeSmokeChromeVersion {
		t.Fatal("private pipe did not report the required Chrome version")
	}
	t.Logf("chrome_version=%s", version.Product)

	var targets struct {
		TargetInfos []struct {
			ID   string `json:"targetId"`
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"targetInfos"`
	}
	var hostTarget string
	var pageCount, emptyURLCount, expectedCount int
	probeFailed := false
	probeDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(probeDeadline) {
		probeCtx, stopProbe := context.WithDeadline(ctx, probeDeadline)
		targetResult, err := protocol.Call(probeCtx, "Target.getTargets", nil, "")
		stopProbe()
		if err != nil {
			probeFailed = true
			break
		}
		if err := json.Unmarshal(targetResult, &targets); err != nil {
			t.Fatal("decode test Chrome targets")
		}
		hostTarget, pageCount, emptyURLCount, expectedCount = "", 0, 0, 0
		for _, target := range targets.TargetInfos {
			if target.Type != "page" {
				continue
			}
			pageCount++
			if target.URL == "" {
				emptyURLCount++
			}
			if target.URL == hostURL {
				expectedCount++
				hostTarget = target.ID
			}
		}
		if expectedCount > 1 {
			t.Fatal("multiple exact fixture page targets; refusing ambiguous close")
		}
		if expectedCount == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("Chrome target probe exceeded test deadline")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if expectedCount != 1 || hostTarget == "" {
		t.Fatalf("exact fixture page target unavailable within bounded probe: pages=%d empty_urls=%d exact_matches=%d probe_failed=%t", pageCount, emptyURLCount, expectedCount, probeFailed)
	}
	closeResult, err := protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": hostTarget}, "")
	if err != nil {
		t.Fatal("close exact fixture app target through private pipe")
	}
	var closed struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(closeResult, &closed); err != nil || !closed.Success {
		t.Fatal("Chrome rejected exact app target close")
	}

	select {
	case waitErr := <-window.Done():
		if waitErr != nil {
			t.Fatal("Chrome root did not exit cleanly after closing its only app target")
		}
	case <-ctx.Done():
		t.Fatal("Chrome root and owned Job did not drain after closing its only app target")
	}
	if err := window.CloseForRetry(ctx); err != nil {
		t.Fatal("managed Window cleanup did not verify private pipe closure")
	}
	complete = true
	t.Log("pipe_roundtrip=true exact_target_closed=true root_exited=true job_empty=true parent_pipes_retained_until_drain=true")
}
