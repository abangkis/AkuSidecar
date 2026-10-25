//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// This compiled synthetic backend is selected before startup in the fixture.
// No offline vector is promoted and no Node response supplies native authority.
type xCaptureSyntheticBackend struct {
	fixture  *manualRuntimeFixture
	calls    atomic.Int32
	mode     string
	mu       sync.Mutex
	acquired map[string]bool
}

func (*xCaptureSyntheticBackend) provenance() nativePilotProvenance { return nativePilotTrustedStartup }
func (b *xCaptureSyntheticBackend) now() time.Time {
	b.fixture.mu.Lock()
	defer b.fixture.mu.Unlock()
	return b.fixture.now
}
func (b *xCaptureSyntheticBackend) snapshot(ctx context.Context, role string) (nativePilotOwner, error) {
	b.fixture.mu.Lock()
	defer b.fixture.mu.Unlock()
	o := b.fixture.actual[role]
	o.context, o.target, o.frame, o.document = "", "", "", ""
	return o, ctx.Err()
}
func (*xCaptureSyntheticBackend) cleanup(context.Context, string, string, bool) error { return nil }
func (b *xCaptureSyntheticBackend) rpc(ctx context.Context, role, op string, fields map[string]any, out any) error {
	if op == "manual_capture_cancel" {
		return nil
	}
	var raw []byte
	if op == "manual_inventory" {
		raw = []byte(`{"windowCount":1,"bootstrapCount":1}`)
	} else if op == "manual_acquire" {
		var request map[string]any
		encoded, _ := json.Marshal(fields["request"])
		_ = json.Unmarshal(encoded, &request)
		request["schema"] = nativePilotStartupWireSchema
		request["accountId"] = "1908123456789012345"
		request["identityRequestId"] = "identity-" + role
		request["settingsRequestId"] = "settings-" + role
		request["identityObservedAtMS"] = b.now().UnixMilli()
		request["settingsObservedAtMS"] = b.now().UnixMilli()
		request["elapsedMS"] = 1
		request["qualification"] = nativePilotSettingsQualification
		b.mu.Lock()
		b.acquired[role] = true
		b.mu.Unlock()
		raw, _ = json.Marshal(request)
	} else if op == "manual_inspect" || op == "manual_prepare" {
		b.fixture.mu.Lock()
		o := b.fixture.actual[role]
		b.fixture.mu.Unlock()
		pins := xNativeCapturePins{OwnerID: o.owner, ProfileID: o.profile, ContextID: o.context, TargetID: o.target, FrameID: o.frame, DocumentID: o.document}
		if op == "manual_prepare" {
			raw, _ = json.Marshal(map[string]any{"handle": role + "-handle", "pins": pins})
		} else {
			count := 0
			if role == "reader" {
				count = 1
			}
			b.mu.Lock()
			paused := !b.acquired[role]
			b.mu.Unlock()
			raw, _ = json.Marshal(map[string]any{"pins": pins, "windowCount": count, "bootstrapCount": count, "paused": paused, "invalid": false, "headless": role == "collector"})
		}
	} else if op == "manual_capture" {
		b.calls.Add(1)
		if b.mode == "cloudflare" || b.mode == "x_human_verification" {
			return errors.New("manual_capture_challenge_" + b.mode)
		}
		project, node := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_NODE")
		if os.Getenv("AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE") != "1" || !filepath.IsAbs(project) || !filepath.IsAbs(node) {
			return errors.New("offline Node fixture unavailable")
		}
		message := map[string]any{"id": 9, "op": op, "owner": 1, "role": role, "handle": "collector-handle", "request": fields["request"]}
		input, _ := json.Marshal(message)
		cmd := exec.CommandContext(ctx, node, filepath.Join(project, "scripts", "auth-session-fixture", "x-native-manual-capture-fixture.mjs"))
		cmd.Stdin = bytes.NewReader(append(input, '\n'))
		stdout, err := cmd.Output()
		if err != nil {
			return errors.New("synthetic Node capture failed")
		}
		var reply struct {
			ID     int             `json:"id"`
			OK     bool            `json:"ok"`
			Result json.RawMessage `json:"result"`
		}
		if json.Unmarshal(stdout, &reply) != nil || !reply.OK {
			return errors.New("synthetic Node result rejected")
		}
		raw = reply.Result
		if b.mode != "" {
			var wire map[string]any
			_ = json.Unmarshal(raw, &wire)
			switch b.mode {
			case "digest":
				wire["digest"] = "bad"
			case "target":
				wire["captureTarget"] = b.fixture.actual["collector"].target
			case "owner":
				wire["pins"].(map[string]any)["ownerId"] = "other-owner"
			case "source":
				var observation map[string]any
				_ = json.Unmarshal([]byte(wire["observationJSON"].(string)), &observation)
				observation["source"] = "facebook"
				encoded, _ := json.Marshal(observation)
				wire["observationJSON"] = string(encoded)
				digest := sha256.Sum256(encoded)
				wire["digest"] = hex.EncodeToString(digest[:])
			case "expiry":
				b.fixture.mu.Lock()
				b.fixture.now = b.fixture.now.Add(time.Minute)
				b.fixture.mu.Unlock()
			}
			raw, _ = json.Marshal(wire)
		}
	} else {
		return errors.New("unsupported synthetic operation")
	}
	return json.Unmarshal(raw, out)
}

func TestNativePilotXCaptureWorkerToStore(t *testing.T) {
	if os.Getenv("AKU_X_NATIVE_MANUAL_CAPTURE_OFFLINE") != "1" {
		t.Skip("explicit offline worker helper required")
	}
	for _, mode := range []string{"", "digest", "target", "owner", "source", "expiry", "cloudflare", "x_human_verification"} {
		t.Run("result-"+mode, func(t *testing.T) {
			x := newNativeStoreFixture(t, true, func(f *manualRuntimeFixture) {
				f.now = time.Now().Truncate(time.Second)
				f.broker.cfg.pilot.attemptStarted = f.now
				for _, role := range []string{"reader", "collector"} {
					o := f.actual[role]
					o.owner = strconv.Itoa(o.pid)
					f.actual[role] = o
				}
				f.backend.owner = f.actual["collector"]
			})
			x.e = New(x.s, reasoning.Deterministic{}, config.Config{Dev: true, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
			t.Cleanup(func() { x.e.Shutdown(); x.e.WaitForIdle(3 * time.Second) })
			backend := &xCaptureSyntheticBackend{fixture: x.f, mode: mode, acquired: map[string]bool{}}
			cfg := xManualPilotOwnerConfig{enabled: true, pilot: x.f.broker.cfg.pilot, policy: func(context.Context) (nativePilotBrokerState, error) {
				x.f.mu.Lock()
				defer x.f.mu.Unlock()
				return x.f.state, nil
			}}
			broker, ports, diagnostic := newXManualPilotOwnerBrokerPorts(context.Background(), cfg, backend)
			if broker == nil {
				t.Fatalf("owner factory %+v", diagnostic)
			}
			x.f.broker = broker
			if d := broker.start(context.Background()); !d.ManualEligible || d.CanAuthorize || d.NativeQualified || d.StorePermit {
				t.Fatalf("startup %+v", d)
			}
			runtime, err := x.e.bindXNativePilotRuntime(context.Background(), x.f.broker, ports, x.f.reader, x.f.collector, func(context.Context, readerbroker.Request) error { return nil }, x.session.ID, x.command.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "" {
				for _, payload := range []map[string]any{{"acquisitionRound": 2}, {"continuation": map[string]any{}}, {"script": "arbitrary"}} {
					if _, err := runtime.backend.Capture(context.Background(), "x", payload); err == nil {
						t.Fatal("unsupported payload silently changed")
					}
				}
				if backend.calls.Load() != 0 {
					t.Fatal("invalid payload reached Node capture")
				}
			}
			result := x.e.captureNativePilot(context.Background(), runtime, x.command.ID)
			if mode != "" {
				if result.Outcome == store.Committed {
					t.Fatal("invalid worker result committed")
				}
				x.count(t, 0)
				x.e.captureNativePilot(context.Background(), runtime, x.command.ID)
				if backend.calls.Load() != 1 {
					t.Fatal("invalid result recaptured")
				}
				if mode == "cloudflare" || mode == "x_human_verification" {
					pause, err := x.s.SourceCapturePause(context.Background())
					if err != nil || pause == nil || pause.Category != mode {
						t.Fatal("native challenge did not reach persistent pause")
					}
				}
				return
			}
			if result.Outcome != store.Committed || result.PersistenceError != nil || result.DeliveryError != nil {
				t.Fatalf("worker result %+v", result)
			}
			if !x.e.WaitForIdle(3 * time.Second) {
				t.Fatal("reasoning did not finish")
			}
			obs, err := x.s.Observations(context.Background(), x.command.RunID)
			if err != nil || len(obs) != 1 {
				t.Fatal("observation missing")
			}
			block := obs[0].Snapshots[0].Blocks[0]
			if block.Text != "Synthetic native worker parser evidence." || block.PlatformID != "x:status:1908123456789012346" || obs[0].Snapshots[0].AdapterVersion != "x-dom-v23" {
				t.Fatal("production parser evidence mismatch")
			}
			run, err := x.s.GetRun(context.Background(), x.command.RunID)
			if err != nil || run.Status != "completed" {
				t.Fatal("actual Engine reasoning not completed")
			}
			if replay := x.e.captureNativePilot(context.Background(), runtime, x.command.ID); replay.Outcome != store.AlreadyCommitted || backend.calls.Load() != 1 {
				t.Fatal("worker recaptured committed work")
			}
		})
	}
}
