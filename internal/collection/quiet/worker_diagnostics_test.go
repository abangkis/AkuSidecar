package quiet

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Holding the Sidecar sink lets the test exercise real RPC and retirement under log backpressure.
type delayedWorkerLog struct {
	rows    chan string
	release chan struct{}
}

func (l *delayedWorkerLog) Write(data []byte) (int, error) {
	l.rows <- string(data)
	<-l.release
	return len(data), nil
}

func TestWorkerDiagnosticsThroughLaunchers(t *testing.T) {
	root := os.Getenv("AKU_WORKER_DIAGNOSTICS_SMOKE_RUNTIME")
	if root == "" {
		t.Skip("set AKU_WORKER_DIAGNOSTICS_SMOKE_RUNTIME to the packaged Windows Node runtime")
	}
	fixture, err := filepath.Abs("testdata/worker-diagnostics.mjs")
	if err != nil {
		t.Fatal(err)
	}
	for _, launcher := range []string{"headless", "quiet"} {
		t.Run(launcher, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			logSink := &delayedWorkerLog{rows: make(chan string, 32), release: make(chan struct{})}
			release := sync.OnceFunc(func() { close(logSink.release) })
			defer release()
			node := filepath.Join(root, "node.exe")
			options := headless.Options{Node: node, Pin: filepath.Join(root, "node.pin.json"), Worker: fixture, Chrome: node,
				Profile: t.TempDir(), ProfileDirectory: "Default", BridgePath: filepath.Dir(fixture),
				Diagnostics: headless.NewWorkerDiagnostics(ctx, log.New(logSink, "", 0))}
			var capture func(context.Context, domain.Source, map[string]any) (domain.Observation, error)
			var retire func(context.Context) error
			if launcher == "headless" {
				process, err := headless.Launch(ctx, options)
				if process != nil {
					t.Cleanup(process.Terminate)
				}
				if err != nil {
					t.Fatal(err)
				}
				capture, retire = process.Capture, process.CloseForRetry
			} else {
				// The capture fixture never creates or borrows a browser target.
				w := NewWorker(&Targets{version: json.RawMessage(`{"product":"Fixture"}`), closed: true}, options)
				t.Cleanup(func() {
					if w.owner != nil {
						w.owner.Terminate()
					}
				})
				capture, retire = w.Capture, w.Retire
			}
			_, err := capture(ctx, domain.SourceX, map[string]any{})
			var failure *headless.CaptureError
			if !errors.As(err, &failure) || failure.Code != "login_required" || failure.Message != "private fixture detail" {
				t.Fatalf("unexpected RPC error: %v", err)
			}
			var line string
			select {
			case line = <-logSink.rows:
			case <-ctx.Done():
				t.Fatal("missing Sidecar worker diagnostic")
			}
			raw, found := bytes.CutPrefix([]byte(line), []byte("sidecar_activity "))
			var event struct {
				Operation, Phase, Outcome string
				Detail                    struct {
					Launcher, Reason string
					Sequence         int
				}
			}
			if !found || json.Unmarshal(raw, &event) != nil || bytes.Contains(raw, []byte("private")) ||
				event.Operation != "headless_worker" || event.Phase != "request_failed" || event.Outcome != "failed" ||
				event.Detail.Launcher != launcher || event.Detail.Reason != "login_required" || event.Detail.Sequence != 1 {
				t.Fatalf("unexpected diagnostic: %s", line)
			}
			observation, err := capture(ctx, domain.SourceX, map[string]any{"burst": true})
			if err != nil || observation.Source != domain.SourceX {
				t.Fatalf("RPC stalled or corrupted during stderr burst: %v", err)
			}
			if err := retire(ctx); err != nil {
				t.Fatalf("worker retirement: %v", err)
			}
			// The stalled sink queued the burst behind the controlled error. Its
			// first forwarded row must be the burst, not a duplicate request error.
			release()
			select {
			case next := <-logSink.rows:
				if strings.Contains(next, "private") || !strings.Contains(next, `"phase":"fatal"`) {
					t.Fatalf("unexpected next event: %s", next)
				}
			case <-ctx.Done():
				t.Fatal("no diagnostic after releasing the sink")
			}
		})
	}
}
