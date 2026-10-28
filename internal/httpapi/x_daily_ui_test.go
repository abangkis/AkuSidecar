package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestXDailyUIOfflineChrome(t *testing.T) {
	if os.Getenv("AKU_X_DAILY_UI_CHROME") == "" {
		t.Skip("opt-in accountless UI fixture requires AKU_X_DAILY_UI_CHROME")
	}
	_, source, _, _ := runtime.Caller(0)
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	node, err := screenshotNode(project, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	chrome, err := appshell.Discover(ctx, os.Getenv("AKU_X_DAILY_UI_CHROME"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts := os.Getenv("AKU_X_DAILY_UI_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	command := exec.Command(node, filepath.Join(project, "scripts", "x-daily-ui-fixture.mjs"), chrome.Executable, t.TempDir(), artifacts)
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	owner, err := appshell.StartOwnedCommand(command)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-owner.Done():
	case <-ctx.Done():
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if cleanupErr := owner.CloseForRetry(cleanup); cleanupErr != nil {
			t.Fatalf("synthetic UI cleanup: %v", cleanupErr)
		}
		err = ctx.Err()
	}
	if err != nil {
		t.Fatalf("synthetic UI fixture: %v\n%s", err, output.String())
	}
	t.Log(output.String())
}

func TestXDailyUIBootstrapStatusDoesNotGrantConsentOrChangeSettings(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(map[bool]string{false: "exclusive", true: "concurrent"}[concurrent], func(t *testing.T) {
			s, _ := splitTestServer(t)
			cfg := s.config
			cfg.Dev = true
			if concurrent {
				cfg.Capture.XMode = config.XCaptureConcurrent
			}
			s.engine = engine.New(s.store, reasoning.Deterministic{}, cfg, s.logger)
			before, err := s.store.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var bootstrap struct {
				InstanceEpoch string                `json:"instanceEpoch"`
				XCapture      engine.XCaptureStatus `json:"xCapture"`
			}
			w := httptest.NewRecorder()
			s.security(s.api()).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11122/api/bootstrap", nil))
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &bootstrap) != nil {
				t.Fatalf("bootstrap status=%d", w.Code)
			}
			if bootstrap.InstanceEpoch != s.engine.Epoch() || !reflect.DeepEqual(bootstrap.XCapture, s.engine.XCaptureStatus()) || bootstrap.XCapture.DerivationAuthorized {
				t.Fatal("bootstrap status changed process authority or lost its current binding")
			}
			if concurrent && (bootstrap.XCapture.AuthorizationPolicy != "profile" || bootstrap.XCapture.MaxBatchMS != 60000) {
				t.Fatal("bootstrap did not expose the profile permission and exact batch limit")
			}
			after, err := s.store.GetSettings(context.Background())
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("reading process status changed persisted settings")
			}
		})
	}
}

func TestXDailyUIActionsKeepDevelopmentAndEpochGates(t *testing.T) {
	for _, dev := range []bool{false, true} {
		for _, action := range []string{"start", "stop"} {
			s, _ := splitTestServer(t)
			s.config.Dev = dev
			for _, epoch := range []string{"", "stale-ui-epoch"} {
				r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/development/x-daily/"+action, strings.NewReader(`{}`))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", "http://127.0.0.1:11122")
				r.Header.Set("X-Aku-Split-Epoch", epoch)
				w := httptest.NewRecorder()
				s.security(s.api()).ServeHTTP(w, r)
				want := http.StatusNotFound
				if dev {
					want = http.StatusConflict
				}
				if w.Code != want || s.engine.XCaptureStatus().DerivationAuthorized {
					t.Fatalf("development=%t action=%s status=%d want=%d", dev, action, w.Code, want)
				}
			}
		}
	}
}

func TestXDailyUIControllerIsServedFromEmbeddedAssets(t *testing.T) {
	s, _ := splitTestServer(t)
	w := httptest.NewRecorder()
	s.http.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11122/x-daily-controls.js", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "javascript") || !strings.Contains(w.Body.String(), "export function createXDailyControls") {
		t.Fatalf("embedded X controller unavailable: status=%d type=%s", w.Code, w.Header().Get("Content-Type"))
	}
}
