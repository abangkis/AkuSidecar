package headless

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

// Explicit fixture-only test of Go admission -> Node JSONL acknowledgement ->
// real Chrome. The profile is new, and no social-source navigation is performed.
func TestOwnedHeadlessIdleLeaseRPCSmoke(t *testing.T) {
	runtimePath, chrome, bridge := os.Getenv("AKU_IDLE_SMOKE_RUNTIME"), os.Getenv("AKU_IDLE_SMOKE_CHROME"), os.Getenv("AKU_IDLE_SMOKE_BRIDGE")
	if runtimePath == "" || chrome == "" || bridge == "" {
		t.Skip("set explicit pinned Node runtime, Chrome and Bridge paths for the fixture-only RPC smoke")
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve source worker")
	}
	options := Options{
		Node: filepath.Join(runtimePath, "node.exe"), Pin: filepath.Join(runtimePath, "node.pin.json"),
		Worker: filepath.Join(filepath.Dir(sourceFile), "worker", "worker.mjs"),
		Chrome: chrome, BridgePath: bridge, Profile: filepath.Join(t.TempDir(), "idle-fixture-profile"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	process, err := Launch(ctx, options)
	if process != nil {
		defer process.Terminate()
	}
	if err != nil {
		t.Fatal(err)
	}
	manager, err := captureruntime.New(process)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Terminate()
	first, err := manager.Acquire()
	if err != nil {
		t.Fatal("real worker did not acknowledge hold", err)
	}
	second, err := manager.Acquire()
	if err != nil {
		first.Release()
		t.Fatal(err)
	}
	first.Release()
	if manager.Snapshot().ActiveLeases != 1 {
		t.Fatal("overlapping lease lost")
	}
	second.Release()
	if manager.Snapshot().State != captureruntime.Ready {
		t.Fatal("real release was not acknowledged", manager.Snapshot())
	}
	// Exercise both acknowledgement values directly as well as through admission.
	if err := process.SetIdleHold(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := process.SetIdleHold(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := process.CloseForRetry(ctx); err != nil {
		t.Fatal("fixture tree cleanup was not verified", err)
	}
}
