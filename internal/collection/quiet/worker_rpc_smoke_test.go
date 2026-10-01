package quiet

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// This opt-in smoke starts the pinned Node worker and exercises its real
// JSONL/CDP RPC path. Chrome is stat'ed by headless.Validate but never launched.
func TestQuietWorkerBorrowedRPCEndToEnd(t *testing.T) {
	runtimePath := os.Getenv("AKU_QUIET_RPC_SMOKE_RUNTIME")
	chromePath := os.Getenv("AKU_QUIET_RPC_SMOKE_CHROME")
	bridgePath := os.Getenv("AKU_QUIET_RPC_SMOKE_BRIDGE")
	if runtimePath == "" || chromePath == "" || bridgePath == "" {
		t.Skip("set the pinned worker runtime, packaged Chrome path and Bridge path to run the local RPC smoke")
	}
	if runtime.GOOS != "windows" {
		t.Skip("the configured smoke runtime is a packaged Windows Node executable")
	}
	for _, path := range []string{runtimePath, chromePath, bridgePath} {
		if !filepath.IsAbs(path) {
			t.Fatal("smoke paths must be absolute")
		}
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve the current source checkout")
	}
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	workerPath := filepath.Join(projectRoot, "internal", "collection", "headless", "worker", "worker.mjs")
	options := headless.Options{
		Node: filepath.Join(runtimePath, "node.exe"), Worker: workerPath,
		Pin: filepath.Join(runtimePath, "node.pin.json"), Chrome: chromePath,
		Profile: filepath.Join(projectRoot, "build", "quiet-rpc-smoke-profile"), BridgePath: bridgePath,
	}
	if err := headless.Validate(options); err != nil {
		t.Fatalf("configured pinned worker paths are invalid: %v", err)
	}

	const snapshot = `{"posts":[{"id":"1890000000000000000","permalink":"https://x.com/example/status/1890000000000000000","author":"Fixture Author","text":"A bounded fixture post from the real Quiet worker RPC path.","contentKind":"video","media":[],"mediaExpected":["video"],"mediaEvidence":{"status":"missing_expected_url","expectedWithoutUrl":["video"]},"limitations":["video_stream_not_resolved"],"textStatus":"visible_text_no_collapse_control"}],"candidateCount":1,"discoveryStrategy":"rpc_fixture","adapterVersion":"rpc_fixture_v1","scroll":{"y":0,"viewportHeight":900,"height":900},"documentReady":true}`
	createCount := 0
	pageRPCCount := 0
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		switch call.method {
		case "Target.attachToBrowserTarget":
			return raw(`{"sessionId":"rpc-browser-child"}`), nil
		case "Browser.getVersion":
			return raw(`{"product":"Chrome/152.0.0.0","protocolVersion":"1.3"}`), nil
		case "Target.createTarget":
			createCount++
			params := paramMap(t, call.params)
			if call.session != "rpc-browser-child" || params["url"] != "about:blank" || params["hidden"] != true || params["forTab"] != false {
				t.Fatalf("worker target was not created as an owned hidden target: %#v", call)
			}
			return raw(`{"targetId":"rpc-owned-x"}`), nil
		case "Target.attachToTarget":
			if call.session != "rpc-browser-child" || paramString(t, call.params, "targetId") != "rpc-owned-x" || paramMap(t, call.params)["flatten"] != true {
				t.Fatalf("worker target attachment escaped its browser session: %#v", call)
			}
			return raw(`{"sessionId":"rpc-page-x"}`), nil
		case "Page.enable", "Runtime.enable", "Network.enable", "Emulation.setDeviceMetricsOverride":
			if call.session != "rpc-page-x" {
				t.Fatalf("page setup used unexpected session %q", call.session)
			}
			return raw(`{}`), nil
		case "Page.navigate":
			pageRPCCount++
			if call.session != "rpc-page-x" {
				t.Fatalf("navigation used unexpected session %q", call.session)
			}
			return raw(`{"frameId":"rpc-frame"}`), nil
		case "Runtime.evaluate":
			pageRPCCount++
			if call.session != "rpc-page-x" {
				t.Fatalf("evaluation used unexpected session %q", call.session)
			}
			var params struct {
				Expression string `json:"expression"`
			}
			if err := json.Unmarshal(call.params.(json.RawMessage), &params); err != nil {
				t.Fatalf("decode Runtime.evaluate parameters: %v", err)
			}
			switch {
			case strings.Contains(params.Expression, "XHeadlessPoC.collect()"):
				result, err := json.Marshal(map[string]any{"result": map[string]any{"type": "string", "value": snapshot}})
				return result, err
			case strings.Contains(params.Expression, "prepareEvidenceTargets"):
				return raw(`{"result":{"type":"object","subtype":"array","value":[]}}`), nil
			default:
				return raw(`{"result":{"type":"undefined"}}`), nil
			}
		case "Input.dispatchMouseEvent":
			pageRPCCount++
			if call.session != "rpc-page-x" {
				t.Fatalf("mouse event used unexpected session %q", call.session)
			}
			return raw(`{}`), nil
		case "Target.closeTarget":
			if call.session != "rpc-browser-child" || paramString(t, call.params, "targetId") != "rpc-owned-x" {
				t.Fatalf("worker cleanup targeted an unowned target: %#v", call)
			}
			return raw(`{"success":true}`), nil
		case "Target.getTargets":
			if call.session != "rpc-browser-child" {
				t.Fatalf("owned target verification used session %q", call.session)
			}
			return raw(`{"targetInfos":[]}`), nil
		case "Target.detachFromTarget":
			if call.session != "" || paramString(t, call.params, "sessionId") != "rpc-browser-child" {
				t.Fatalf("worker cleanup detached an unexpected session: %#v", call)
			}
			return raw(`{}`), nil
		default:
			return nil, fmt.Errorf("unexpected CDP method %s", call.method)
		}
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(targets, options)
	retired := false
	t.Cleanup(func() {
		if retired {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := worker.Retire(cleanupCtx); err != nil {
			t.Errorf("cleanup after smoke failure: %v", err)
		}
	})

	captureCtx, cancelCapture := context.WithTimeout(context.Background(), 20*time.Second)
	observation, err := worker.Capture(captureCtx, domain.SourceX, map[string]any{
		"acquisitionRound": 1, "scrolls": 0, "sourceHydrationTimeoutMs": 1000,
		"captureTimeoutMs": 6000, "maxBlocksPerSnapshot": 20,
	})
	cancelCapture()
	if err != nil {
		t.Fatalf("real worker capture over fake CDP failed: %v", err)
	}
	if observation.Source != domain.SourceX || len(observation.Snapshots) != 1 || len(observation.Snapshots[0].Blocks) != 1 {
		t.Fatalf("worker returned an unexpected observation shape: source=%q snapshots=%d", observation.Source, len(observation.Snapshots))
	}
	block := observation.Snapshots[0].Blocks[0]
	if block.PlatformID != "x:status:1890000000000000000" || block.Text != "A bounded fixture post from the real Quiet worker RPC path." {
		t.Fatalf("worker did not preserve X identity and text: %#v", block)
	}
	if observation.Coverage["captureMode"] != "browser_quiet_hidden" || block.CaptureQuality["status"] != "unverified" || block.MediaRecovery["unknownVideo"] != "unresolved" {
		t.Fatalf("worker promoted fixture quality or lost Quiet provenance: coverage=%#v quality=%#v media=%#v", observation.Coverage, block.CaptureQuality, block.MediaRecovery)
	}
	if createCount != 1 || pageRPCCount < 3 {
		t.Fatalf("worker RPC coverage was too small: hidden target creates=%d page RPCs=%d", createCount, pageRPCCount)
	}

	retireCtx, cancelRetire := context.WithTimeout(context.Background(), 20*time.Second)
	err = worker.Retire(retireCtx)
	cancelRetire()
	if err != nil {
		t.Fatalf("retiring Node and owned hidden target failed: %v", err)
	}
	retired = true
	if len(callsFor(protocol.calls, "Browser.close")) != 0 {
		t.Fatal("retiring the Node worker attempted to close the browser")
	}
	if len(callsFor(protocol.calls, "Target.closeTarget")) != 1 || len(callsFor(protocol.calls, "Target.detachFromTarget")) != 1 {
		t.Fatal("worker retirement did not clean up the exact hidden target and child session")
	}
}
