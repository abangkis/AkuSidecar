package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
)

func TestHeadlessNavigationDiagnosticPersistsOnRun(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the offline worker fixture")
	}
	binary, err := os.ReadFile(node)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := filepath.Abs(filepath.Join("..", "collection", "headless", "worker", "worker.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pin := filepath.Join(dir, "node.pin.json")
	if err := os.WriteFile(pin, []byte(fmt.Sprintf(`{"protocol":1,"nodeSha256":"%x"}`, sha256.Sum256(binary))), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(dir, "worker-fixture.mjs")
	if err := os.WriteFile(fixture, []byte(fmt.Sprintf(`
import { pathToFileURL } from 'node:url';
const { runWorker } = await import(pathToFileURL(%q).href);
const replies = ['net::ERR_NAME_NOT_RESOLVED', 'net::ERR_ABORTED https://private.example/?token=private'];
await runWorker({
  sourceAssetsImpl: async () => [{}, {}, {}],
  launchChromeImpl: async () => ({
    backend: 'headless_worker',
    async navigate(url) {
      if (url !== 'https://x.com/home' || replies.length === 0) throw Error('unexpected fixture navigation');
      return { errorText: replies.shift() };
    },
    async close() {},
  }),
});
`, worker)), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	process, err := headless.Launch(ctx, headless.Options{
		Node: node, Pin: pin, Worker: fixture, Chrome: fixture,
		Profile: dir, ProfileDirectory: "Default", BridgePath: dir,
	})
	if process != nil {
		t.Cleanup(process.Terminate)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, category := range []string{"dns", "other"} {
		t.Run(category, func(t *testing.T) {
			e, state := singleSourceEngine(t, nil)
			manager := attachTestCapture(t, e)
			e.AttachCollectionCoordinator(collection.NewCoordinator(manager, nil, func() error { return nil }))
			settings, err := e.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.CollectionMode = "headless"
			if _, err := e.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			e.ResetCaptureHeartbeat()
			if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
				return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
			}); err != nil {
				t.Fatal(err)
			}
			session, err := e.StartVisibleUpdate(ctx, "offline navigation fixture")
			if err != nil {
				t.Fatal(err)
			}
			run := session.Runs[0]
			command, err := e.claimCommandForCollector(ctx, run.ID, "fixture", "headless", collection.BackendHeadless)
			if err != nil || command == nil {
				t.Fatalf("claim: %+v %v", command, err)
			}
			_, err = process.Capture(ctx, run.Source, command.Payload)
			if err == nil {
				t.Fatal("expected a navigation failure")
			}
			if _, err := e.FailCommand(ctx, command.ID, run.ID, internalCollectorFailure(err, collection.BackendHeadless)); err != nil {
				t.Fatal(err)
			}
			stored, err := state.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"cdpCommand": "Page.navigate", "chromeErrorCategory": category}
			if stored.BridgeCommandStatus != "failed" || stored.Error == nil || stored.Error.Code != "headless_navigation_failed" ||
				stored.Error.Stage != "capture" || !stored.Error.Retryable ||
				stored.Error.Message != "navigation_failed: Chrome could not navigate to the requested source page" ||
				!reflect.DeepEqual(stored.Error.Details, want) {
				t.Fatalf("unexpected persisted navigation failure: command=%s error=%+v", stored.BridgeCommandStatus, stored.Error)
			}
		})
	}
	if err := process.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
}
