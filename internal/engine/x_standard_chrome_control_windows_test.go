//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func decodeXStandardOutcome(raw []byte) (string, error) {
	var result struct {
		Outcome string `json:"outcome"`
	}
	invalid := errors.New("invalid manual control outcome")
	if len(raw) < 1 || len(raw) > 128 {
		return "", invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF {
		return "", invalid
	}
	switch result.Outcome {
	case "home_visible", "google_rejected", "x_rejected", "other_error", "not_attempted", "cancelled":
		return result.Outcome, nil
	default:
		return "", invalid
	}
}

func TestXStandardChromeControlOffline(t *testing.T) {
	for _, value := range []string{"home_visible", "google_rejected", "x_rejected", "other_error", "not_attempted", "cancelled"} {
		if result, err := decodeXStandardOutcome([]byte(`{"outcome":"` + value + `"}`)); err != nil || result != value {
			t.Fatal("fixed manual outcome rejected")
		}
	}
	for _, value := range []string{`{"outcome":"account-value"}`, `{"outcome":"home_visible","account":"private"}`, `{} {}`, `[]`} {
		if _, err := decodeXStandardOutcome([]byte(value)); err == nil {
			t.Fatal("unknown or private data crossed manual outcome boundary")
		}
	}
}

func TestXStandardChromeControl(t *testing.T) {
	if os.Getenv("AKU_X_STANDARD_CONTROL_ENABLED") != "1" {
		t.Skip("standard Chrome control requires explicit manual-login helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("control source root unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	qaParent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	outputPath := func(variable, suffix string) string {
		path := os.Getenv(variable)
		if !samePath(filepath.Dir(path), qaParent) ||
			!regexp.MustCompile(`^x-standard-chrome-[a-f0-9]{32}\.`+suffix+`$`).MatchString(filepath.Base(path)) {
			t.Fatal("control output scope invalid")
		}
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("control output must be new")
		}
		return path
	}
	progressPath := outputPath("AKU_X_STANDARD_PROGRESS_PATH", `progress\.jsonl`)
	resultPath := outputPath("AKU_X_STANDARD_RESULT_PATH", `result\.json`)
	reportPath := outputPath("AKU_X_STANDARD_REPORT_PATH", `report\.json`)
	progress, err := os.OpenFile(progressPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("control progress unavailable")
	}
	t.Cleanup(func() { _ = progress.Close() })
	stage := func(value string) {
		if json.NewEncoder(progress).Encode(map[string]string{"stage": value}) != nil {
			t.Fatal("control progress write failed")
		}
		_ = progress.Sync()
		t.Log("Standard Chrome control stage:", value)
	}
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal("control scope unavailable")
	}
	caseRoot := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "run-x-standard-chrome-"+hex.EncodeToString(nonce[:]))
	if err := os.Mkdir(caseRoot, 0700); err != nil {
		t.Fatal("new control root unavailable")
	}
	config := chromeFixtureConfig{NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"),
		Root: caseRoot, Origin: server.URL,
		DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-standard-chrome-control.mjs")}
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("control bridge startup failed:", err)
	}
	registerChromeFixtureBridge(bridge)
	var rootHandle windows.Handle
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
			t.Error("control Job cleanup unverified")
		}
		if rootHandle != 0 {
			state, waitErr := windows.WaitForSingleObject(rootHandle, 5000)
			if waitErr != nil || state != windows.WAIT_OBJECT_0 {
				t.Error("control Chrome exit unverified")
			}
			_ = windows.CloseHandle(rootHandle)
		}
		if !t.Failed() {
			stage("cleanup_verified")
		}
	})
	if err := bridge.assertBridgeOnly(ctx); err != nil {
		t.Fatal("Chrome started before control Job assignment")
	}
	stage("bridge_ready")
	owner, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("owned standard Chrome launch failed:", err)
	}
	rootHandle, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(owner.PID()))
	if err != nil {
		t.Fatal("control Chrome root handle unavailable")
	}
	verifyXChromeFixtureImage(t, bridge.pid, config.NodePath)
	verifyXChromeFixtureImage(t, uint32(owner.PID()), config.ChromePath)
	stage("manual_login")
	deadline := time.Now().Add(5 * time.Minute)
	outcome := "timeout"
	reportSource := "harness_deadline"
	for time.Now().Before(deadline) {
		info, err := os.Lstat(resultPath)
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 128 {
				t.Fatal("manual outcome file invalid")
			}
			file, openErr := os.Open(resultPath)
			if openErr != nil {
				t.Fatal("manual outcome unavailable")
			}
			raw, readErr := io.ReadAll(io.LimitReader(file, 129))
			_ = file.Close()
			if readErr != nil {
				t.Fatal("manual outcome read failed")
			}
			outcome, err = decodeXStandardOutcome(raw)
			if err != nil {
				t.Fatal("manual outcome projection rejected")
			}
			reportSource = "manual_user_report"
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal("manual outcome scope unavailable")
		}
		select {
		case <-ctx.Done():
			t.Fatal("control lifetime expired")
		case <-time.After(time.Second):
		}
	}
	report := struct {
		Schema             string `json:"schema"`
		Outcome            string `json:"outcome"`
		Source             string `json:"source"`
		DebuggingEnabled   bool   `json:"debuggingEnabled"`
		ObservationEnabled bool   `json:"observationEnabled"`
		LoginQualified     bool   `json:"loginQualified"`
		CanAuthorize       bool   `json:"canAuthorize"`
	}{Schema: "aku.x.standard-chrome-control.v1", Outcome: outcome, Source: reportSource}
	output, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("control report unavailable")
	}
	if err := json.NewEncoder(output).Encode(report); err != nil {
		_ = output.Close()
		t.Fatal("control report write failed")
	}
	_ = output.Close()
	t.Log("Reported control outcome:", outcome, "source:", reportSource, "; no browser observation or authorization")
	stage("complete")
}
