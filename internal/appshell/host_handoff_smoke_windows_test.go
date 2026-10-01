//go:build windows

package appshell_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"golang.org/x/sys/windows"
)

// Explicit local opt-in only. Chrome, Node and Bridge are staged paths; the
// profile contains static loopback pages and is never an installed profile.
// Closing the popup is a separate simulated user action, not handoff cleanup.
func TestHostOnlyHandoffRetainsPopupWindowsSmoke(t *testing.T) {
	runtimeRoot := os.Getenv("AKU_HOST_HANDOFF_SMOKE_RUNTIME")
	chrome := os.Getenv("AKU_HOST_HANDOFF_SMOKE_CHROME")
	bridge := os.Getenv("AKU_HOST_HANDOFF_SMOKE_BRIDGE")
	if runtimeRoot == "" || chrome == "" || bridge == "" {
		t.Skip("explicit staged Node runtime, Chrome and Bridge paths required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	profile := filepath.Join(t.TempDir(), "capture-profile")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/popup" {
			_, _ = io.WriteString(w, "<!doctype html><title>Isolated source popup</title><p>Independent source window</p>")
			return
		}
		_, _ = io.WriteString(w, "<!doctype html><title>AkuBrowser capture host</title><p>Static isolated host</p>")
	}))
	defer server.Close()
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{
		Executable: chrome, UserDataDir: profile, URL: server.URL + "/host", StartMinimized: true,
		ExtraArgs: []string{"--start-minimized", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Explicit whole-test cleanup is separate from the handoff under test.
	defer window.Terminate()
	if _, err := window.StartCaptureContainment(log.New(io.Discard, "", 0)); err != nil {
		t.Fatal(err)
	}
	root, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(window.PID()))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(root)
	endpoint := smokeDevToolsEndpoint(t, ctx, profile)
	node := filepath.Join(runtimeRoot, "node.exe")
	cdp := func(method string, params any, target any) error {
		encoded, err := json.Marshal(params)
		if err != nil {
			return err
		}
		command := exec.CommandContext(ctx, node, filepath.Join("testdata", "host_handoff_cdp.mjs"), endpoint, method, string(encoded))
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		output, err := command.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w: %.600s", method, err, output)
		}
		return json.Unmarshal(output, target)
	}
	type targetInfo struct {
		ID  string `json:"targetId"`
		URL string `json:"url"`
	}
	getTargets := func() ([]targetInfo, error) {
		var result struct {
			Targets []targetInfo `json:"targetInfos"`
		}
		err := cdp("Target.getTargets", map[string]any{}, &result)
		return result.Targets, err
	}
	targets, err := getTargets()
	if err != nil {
		t.Fatal(err)
	}
	var host string
	for _, target := range targets {
		if target.URL == server.URL+"/host" {
			host = target.ID
		}
	}
	if host == "" {
		t.Fatal("isolated capture host target missing")
	}
	var popup struct {
		ID string `json:"targetId"`
	}
	createLatePopup := func() error {
		if err := cdp("Target.createTarget", map[string]any{
			"url": server.URL + "/popup", "newWindow": true, "background": true, "windowState": "minimized",
		}, &popup); err != nil {
			return err
		}
		var bounds struct {
			Bounds struct {
				State string `json:"windowState"`
			} `json:"bounds"`
		}
		if err := cdp("Browser.getWindowForTarget", map[string]any{"targetId": popup.ID}, &bounds); err != nil || bounds.Bounds.State != "minimized" {
			return fmt.Errorf("source popup not minimized: state=%q error=%v", bounds.Bounds.State, err)
		}
		return nil
	}
	var closeRequests atomic.Int32
	if err := window.SetCaptureHandoff(func(context.Context) error {
		values, err := getTargets()
		if err != nil {
			return err
		}
		for _, target := range values {
			if target.ID != host {
				continue
			}
			if target.URL != server.URL+"/host" {
				return errors.New("host navigation invalidated scoped close")
			}
			// Reproduce the consequential interval: Manager readiness has
			// already passed, but a new independent window appears before the
			// scoped host close. It must survive this and subsequent retries.
			if popup.ID == "" {
				if err := createLatePopup(); err != nil {
					return err
				}
			}
			var closed map[string]any
			closeRequests.Add(1)
			return cdp("Target.closeTarget", map[string]any{"targetId": host}, &closed)
		}
		return nil // The exact host was already retired; never close another tab.
	}); err != nil {
		t.Fatal(err)
	}
	manager, err := captureruntime.New(window)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Terminate()
	options := headless.Options{
		Node: node, Worker: filepath.Join(runtimeRoot, "worker.mjs"), Pin: filepath.Join(runtimeRoot, "node.pin.json"),
		Chrome: chrome, Profile: profile, BridgePath: bridge,
	}
	launch := func(nextCtx context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		if mode != "headless" {
			return nil, fmt.Errorf("unexpected smoke mode %q", mode)
		}
		return headless.Launch(nextCtx, options)
	}
	coordinator := collection.NewCoordinator(manager, launch, func() error { return headless.Validate(options) })
	coordinator.Request("headless")
	// Actual bounded handoff times out while the independent popup stays alive.
	transition, stopTransition := context.WithTimeout(ctx, 2*time.Second)
	err = manager.Replace(transition, func(nextCtx context.Context, generation uint64) (captureruntime.Process, error) {
		return launch(nextCtx, "headless", generation)
	})
	stopTransition()
	if !errors.Is(err, appshell.ErrCaptureHandoffPending) {
		t.Fatalf("expected live-popup pending retirement, got %v", err)
	}
	values, err := getTargets()
	if err != nil {
		t.Fatal(err)
	}
	popupAlive := false
	for _, target := range values {
		if target.ID == host {
			t.Fatal("host target survived scoped close")
		}
		popupAlive = popupAlive || target.ID == popup.ID
	}
	if !popupAlive || !smokeProcessAliveInJob(t, root) || manager.Snapshot().Generation != 1 || !coordinator.Status().Pending {
		t.Fatalf("late popup/owner/profile were not retained: popup=%t status=%+v", popupAlive, coordinator.Status())
	}
	t.Logf("popup created after readiness; host closed once; minimized popup, Chrome root and owned Job survived timeout; generation=%d", manager.Snapshot().Generation)
	coordinator.Start(ctx)
	// The Coordinator automatically resumes the retained retirement; no new
	// Request is issued. Its ordinary tick must run before the popup is closed.
	smokeWait(t, ctx, func() bool { return manager.Snapshot().State == captureruntime.Replacing })
	var closed map[string]any
	if err := cdp("Target.closeTarget", map[string]any{"targetId": popup.ID}, &closed); err != nil {
		t.Fatal(err)
	}
	smokeWait(t, ctx, func() bool { return coordinator.Status().Effective == "headless" })
	status := coordinator.Status()
	if status.Generation != 2 || status.Pending || closeRequests.Load() != 1 {
		t.Fatalf("automatic profile reuse failed: %+v hostCloses=%d", status, closeRequests.Load())
	}
	if event, err := windows.WaitForSingleObject(root, 0); err != nil || event != windows.WAIT_OBJECT_0 {
		t.Fatalf("old Chrome root still alive after headless reuse: event=%d error=%v", event, err)
	}
	t.Log("final popup closed separately; old root exited naturally and verified Job release allowed automatic headless generation 2 on the same profile")
}

func smokeDevToolsEndpoint(t *testing.T, ctx context.Context, profile string) string {
	t.Helper()
	var endpoint string
	smokeWait(t, ctx, func() bool {
		raw, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
		if err != nil {
			return false
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(lines) != 2 {
			return false
		}
		endpoint = "ws://127.0.0.1:" + strings.TrimSpace(lines[0]) + strings.TrimSpace(lines[1])
		return true
	})
	return endpoint
}

func smokeWait(t *testing.T, ctx context.Context, check func() bool) {
	t.Helper()
	for !check() {
		select {
		case <-ctx.Done():
			t.Fatal("isolated host handoff smoke timed out:", ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func smokeProcessAliveInJob(t *testing.T, process windows.Handle) bool {
	t.Helper()
	event, err := windows.WaitForSingleObject(process, 0)
	if err != nil || event != uint32(windows.WAIT_TIMEOUT) {
		return false
	}
	var inJob int32
	result, _, err := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(process), 0, uintptr(unsafe.Pointer(&inJob)))
	if result == 0 {
		t.Fatal("read Chrome Job membership:", err)
	}
	return inJob != 0
}
