//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func TestXDevelopmentCLIRejectsBeforeOrdinaryState(t *testing.T) {
	root := os.Getenv("AKU_X_DEV_ENTRY_IMAGES")
	if root == "" {
		t.Skip("canonical development entry helper required")
	}
	for _, mode := range []string{"default", "auth_session_dev", "auth_session_fixture", "auth_session_dev-auth_session_fixture"} {
		t.Run(mode, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "must-not-create.db")
			cmd := exec.Command(filepath.Join(root, mode+".exe"), "--dev", "--dev-session-reuse=native-x-manual", "--database="+db, "--config="+filepath.Join(root, "missing-config.json"))
			output, err := cmd.CombinedOutput()
			if err == nil || strings.Contains(string(output), "read config") || strings.Contains(string(output), "missing-config.json") {
				t.Fatalf("did not reject before ordinary config: %s %v", output, err)
			}
			if _, err = os.Stat(db); !os.IsNotExist(err) {
				t.Fatal("early rejection touched database")
			}
		})
	}
}

func TestXDevelopmentCLIOrdinaryMainWaitsForConsent(t *testing.T) {
	images := os.Getenv("AKU_X_DEV_ENTRY_IMAGES")
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if images == "" || node == "" {
		t.Skip("canonical development entry helper required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	plan, err := engine.WriteXApplicationQASyntheticPlan(t.TempDir(), node, port)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(plan)
	if err = os.Mkdir(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, "config", "sidecar.json")
	db := filepath.Join(root, "state", "pilot.db")
	raw := []byte(fmt.Sprintf(`{"version":1,"server":{"host":"127.0.0.1","port":%d},"database":{"path":"state/pilot.db"},"reasoning":{"provider":"deterministic","timeoutMs":5000},"capture":{"profile":"expanded","visibility":"quiet","maxAcquisitionRounds":1},"preference":{"mode":"promote_unused_budget"},"bridge":{"trustedExtensionOrigins":["chrome-extension://dlibmmlopdahibfniinemhnghlifiple"]}}`, port))
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	// Validate the complete fixture before launching an actual main image.
	if _, err = config.Load(config.Options{Dev: true, ConfigPath: configPath, DatabasePath: db, Port: port}); err != nil {
		t.Fatal("invalid bootstrap fixture", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	control := strings.Repeat("a", 64)
	cmd := exec.CommandContext(ctx, filepath.Join(images, "auth_session_dev.exe"), "--dev", "--dev-session-reuse=native-x-manual", "--dev-x-manual-plan="+plan, "--config="+configPath, "--database="+db, fmt.Sprint("--port=", port), "--runtime-control-token="+control)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	var status struct {
		XManual struct {
			State, Reason string
			Available     bool
		}
	}
	lastStage := "not_polled"
	for {
		response, err := client.Get(base + "/api/collection/runtime")
		if err == nil {
			lastStage = fmt.Sprintf("http_%d", response.StatusCode)
			decodeErr := json.NewDecoder(response.Body).Decode(&status)
			response.Body.Close()
			if response.StatusCode == 200 && decodeErr == nil {
				break
			}
			if decodeErr != nil {
				lastStage += "_decode_rejected"
			}
		} else {
			lastStage = "transport_rejected"
			if e, ok := err.(net.Error); ok && e.Timeout() {
				lastStage = "transport_timeout"
			}
		}
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatalf("ordinary main did not become ready; last=%s: %s", lastStage, output.String())
		case <-time.After(20 * time.Millisecond):
		}
	}
	if status.XManual.State != "disabled" || !status.XManual.Available || status.XManual.Reason != "explicit-start-required" {
		t.Fatal("ordinary main inherited start authority", status)
	}
	request, _ := http.NewRequestWithContext(ctx, "POST", base+"/api/runtime/shutdown-if-idle", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Runtime-Control-Token", control)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatal("isolated main refused graceful idle shutdown", response.StatusCode)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatalf("ordinary main shutdown: %s", output.String())
	}
}

func TestXDevelopmentUIProfileUsesOnlyInstanceRoot(t *testing.T) {
	cfg := config.Config{Root: filepath.Join(t.TempDir(), "isolated")}
	opts := config.Options{DevelopmentSessionReuse: "native-x-manual"}
	if got := browserProfilePath(opts, cfg); got != filepath.Join(cfg.Root, "profiles", "ui", "profile") {
		t.Fatal("manual UI profile escaped instance", got)
	}
}

type xDevelopmentCloseFunc func() error

func (f xDevelopmentCloseFunc) Close() error { return f() }

func TestXDevelopmentDatabaseCloseFollowsManualCleanup(t *testing.T) {
	var events []string
	manual := xDevelopmentCloseFunc(func() error {
		events = append(events, "manual")
		return nil
	})
	database := xDevelopmentCloseFunc(func() error {
		events = append(events, "database")
		return nil
	})
	if err := closeDatabaseAfterManualSelection(manual, database); err != nil || strings.Join(events, ",") != "manual,database" {
		t.Fatal("database closed before manual cleanup", events, err)
	}
	events = nil
	if err := closeDatabaseAfterManualSelection(nil, database); err != nil || strings.Join(events, ",") != "database" {
		t.Fatal("ordinary database cleanup changed", events, err)
	}
}

func TestXDevelopmentUncertainManualCleanupRetainsDatabase(t *testing.T) {
	first := fmt.Errorf("synthetic startup cleanup timeout")
	closed := false
	manual := xDevelopmentCloseFunc(func() error { return first })
	database := xDevelopmentCloseFunc(func() error { closed = true; return nil })
	if err := closeDatabaseAfterManualSelection(manual, database); err != first || closed {
		t.Fatal("uncertain cleanup closed database or lost first failure", err)
	}
}
