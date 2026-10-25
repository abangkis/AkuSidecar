//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

// Borrowed role adapter. The existing Job/lease owns cleanup; replacement is
// forbidden. Each manager observes its actual Chrome handle independently.
type retainedCaptureProcess struct {
	role xManualPilotWindowsRole
	done chan error
}

func (p *retainedCaptureProcess) Driver() string {
	if p.role.role == "reader" {
		return "browser"
	}
	return "headless"
}
func (p *retainedCaptureProcess) PID() int           { return int(p.role.chrome.PID) }
func (p *retainedCaptureProcess) Done() <-chan error { return p.done }
func (p *retainedCaptureProcess) Terminate()         { p.role.bridge.terminateOwnedTree() }
func (*retainedCaptureProcess) CloseForRetry(context.Context) error {
	return errors.New("pilot replacement forbidden")
}
func (*retainedCaptureProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("pilot extension navigation forbidden")
}
func (*retainedCaptureProcess) ReplacementReadiness(context.Context) error {
	return errors.New("pilot replacement forbidden")
}
func retainedCaptureManager(t *testing.T, role xManualPilotWindowsRole) *captureruntime.Manager {
	t.Helper()
	p := &retainedCaptureProcess{role: role, done: make(chan error, 1)}
	stop := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		defer close(p.done)
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				state, err := windows.WaitForSingleObject(role.chromeHandle, 0)
				if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
					return
				}
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-ended })
	manager, err := captureruntime.New(p)
	if err != nil {
		t.Fatal("capture QA manager unavailable")
	}
	return manager
}

// Pure transport-code regression; no Chrome, profile, lease or transfer setup.
func TestXManualCaptureDiagnosticCode(t *testing.T) {
	for _, stage := range nativePilotCaptureStages {
		for _, reason := range nativePilotCaptureReasons {
			for _, cleanup := range nativePilotCaptureCleanup {
				code := "manual_capture_detail_" + stage + "__" + reason + "__" + cleanup
				if safeFixtureCode(code) != code {
					t.Fatal("capture detail lost in bridge sanitizer")
				}
				if d, ok := nativePilotCaptureRPCDetail(safeFixtureCode(code)); !ok || d != (nativePilotCaptureDiagnostic{stage, reason, cleanup}) {
					t.Fatal("capture detail lost in private transport decoder")
				}
			}
		}
	}
}

// Actual Node encoder -> correlated stdio reader -> Windows RPC adapter. Only
// transport is exercised: no OS ownership assertion, Chrome, profile or seed.
func TestXManualCaptureDiagnosticPipe(t *testing.T) {
	project, node := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT"), os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if project == "" || node == "" {
		t.Skip("canonical offline helper required")
	}
	const script = `
import {pathToFileURL} from 'node:url'; import {createInterface} from 'node:readline';
const {executeManualReply}=await import(pathToFileURL(process.argv[1]));
const {syntheticCapturePort,syntheticCaptureTransport}=await import(pathToFileURL(process.argv[2]));
const transport=syntheticCaptureTransport(), command=transport.command.bind(transport);
transport.command=async(method,...args)=>{if(method==='Page.navigate')throw Object.assign(Error('PRIVATE'),{code:'challenge_required',challengeCategory:'cloudflare'});const value=await command(method,...args);if(method==='Target.closeTarget')throw Error('PRIVATE');return value;};
const pins={ownerId:'4102',profileId:'synthetic-collector',contextId:'proof-context',targetId:'proof-target',frameId:'proof-frame',documentId:'proof-document'};
const port=await syntheticCapturePort(pins,transport,process.argv[3]);
console.log(JSON.stringify({ready:true}));
for await(const line of createInterface({input:process.stdin})){const q=JSON.parse(line);await executeManualReply(q,()=>port.execute(q),(id,result,code,_limit,diagnostic)=>console.log(JSON.stringify({id,ok:!code,...(code?{code,diagnostic}:{result})})));}
`
	cmd := exec.Command(node, "--input-type=module", "--eval", script, filepath.Join(project, "scripts", "auth-session-fixture", "x-native-metadata-probe-driver.mjs"), filepath.Join(project, "scripts", "auth-session-fixture", "x-native-manual-capture-fixture.mjs"), filepath.Join(project, "..", "AkuBridge"))
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal("offline diagnostic pipe unavailable")
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("offline diagnostic input unavailable")
	}
	if cmd.Start() != nil {
		t.Fatal("offline diagnostic Node unavailable")
	}
	b := &chromeFixtureBridge{cmd: cmd, stdin: stdin, pipe: ownedrpc.New(stdin), owners: map[int]*chromeFixtureProcess{}, closed: make(chan struct{}), nodeWaited: make(chan struct{})}
	ready := make(chan error, 1)
	go b.readOutput(stdout, ready)
	t.Cleanup(func() {
		_ = stdin.Close()
		select {
		case <-b.nodeWaited:
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-b.nodeWaited
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case err = <-ready:
		if err != nil {
			t.Fatal("offline diagnostic handshake failed")
		}
	case <-ctx.Done():
		t.Fatal("offline diagnostic handshake timeout")
	}
	pins := map[string]string{"ownerId": "4102", "profileId": "synthetic-collector", "contextId": "proof-context", "targetId": "proof-target", "frameId": "proof-frame", "documentId": "proof-document"}
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"collector": {role: "collector", bridge: b, protocolOwner: 1}}}
	err = backend.rpc(ctx, "collector", "manual_capture", map[string]any{"handle": "collector-handle", "request": map[string]any{"correlation": "synthetic-capture", "pins": pins, "deadlineMS": time.Now().Add(4 * time.Second).UnixMilli(), "payload": map[string]any{"scrolls": 0}}}, nil)
	var challenge *headless.CaptureError
	if !errors.As(err, &challenge) || challenge.ChallengeCategory != "cloudflare" {
		t.Fatal("operational challenge lost in actual pipe")
	}
	if d := nativePilotCaptureDetail(err, "unknown", "unknown"); d.stage != "worker" || d.reason != "challenge_cloudflare" || d.cleanup != "failed" {
		t.Fatalf("actual pipe lost independent diagnostic: %s", d)
	}
}

func TestXManualCaptureRetainedQA(t *testing.T) {
	if os.Getenv("AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED") != "1" {
		t.Skip("capture QA requires explicit prepared capture authorization")
	}
	runXManualRetainedQA(t, func(ctx context.Context, ready xManualRetainedReady) {
		path := filepath.Join(ready.root, "capture-qa.db")
		// Never reopen an existing or user database, even if the helper is misused.
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("fresh capture QA database unavailable")
		}
		file.Close()
		settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false)
		settings.CollectionMode = "headless"
		settings.ActiveSources = []domain.Source{domain.SourceX}
		s, err := store.Open(path, settings)
		if err != nil {
			t.Fatal("capture QA database unavailable")
		}
		defer s.Close()
		session, err := s.CreateUpdateSession(ctx, "Retained QA native capture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
		if err != nil {
			t.Fatal("capture QA session rejected")
		}
		command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}, "scrolls": 1, "maxBlocksPerSnapshot": 5, "scrollFraction": .5, "scrollSettleMs": 1400})
		if err != nil {
			t.Fatal("capture QA command rejected")
		}
		e := New(s, reasoning.Deterministic{}, config.Config{Dev: true, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1, Behavior: config.CaptureBehaviorConfig{Enabled: true}}}, log.New(io.Discard, "", 0))
		defer func() { e.Shutdown(); e.WaitForIdle(3 * time.Second) }()
		reader, collector := retainedCaptureManager(t, ready.reader), retainedCaptureManager(t, ready.collector)
		runtime, err := e.bindXNativePilotRuntime(ctx, ready.broker, ready.ports, reader, collector, func(call context.Context, _ readerbroker.Request) error {
			var raw json.RawMessage
			if err := ready.reader.bridge.rpcRoundTripObserved(call, "manual_open_reader", map[string]any{"owner": ready.reader.protocolOwner}, &raw, nil); err != nil {
				return errors.New("capture QA reader failed")
			}
			defer clear(raw)
			var reply struct {
				Opened         bool `json:"opened"`
				WindowCount    int  `json:"windowCount"`
				BootstrapCount int  `json:"bootstrapCount"`
			}
			if xManualPilotDecode(raw, &reply) != nil || !reply.Opened || reply.WindowCount < 2 || reply.BootstrapCount != 1 {
				return errors.New("capture QA reader inventory failed")
			}
			return nil
		}, session.ID, command.RunID)
		if err != nil {
			t.Fatal("capture QA startup binding rejected")
		}
		request := readerbroker.Request{RequestID: "broker_0123456789abcdef0123456789abcdef", Source: "x", URL: "https://x.com/home"}
		if err = runtime.openReader(ctx, request); err != nil {
			t.Fatal("capture QA reader open rejected")
		}
		result := e.captureNativePilot(ctx, runtime, command.ID)
		if backend, ok := runtime.backend.(*xNativePilotCapture); ok {
			t.Log("manual-retained-capture-traffic " + backend.trafficDiagnostic())
		}
		if result.Outcome != store.Committed || result.PersistenceError != nil || result.DeliveryError != nil {
			t.Fatalf("capture QA rejected phase=%s outcome=%s detail=(%s); no retry", result.Phase, result.Outcome, result.CaptureDiagnostic)
		}
		observations, err := s.Observations(ctx, command.RunID)
		if err != nil || len(observations) != 1 {
			t.Fatal("capture QA observation count rejected")
		}
		blocks := 0
		for _, snapshot := range observations[0].Snapshots {
			blocks += len(snapshot.Blocks)
		}
		if blocks == 0 {
			t.Fatal("capture QA returned no posts")
		}
		if len(processFixtureWindows(ready.reader.chrome.PID)) < 2 || collector.PID() != int(ready.collector.chrome.PID) {
			t.Fatal("capture QA concurrent ownership lost")
		}
		if again := e.captureNativePilot(ctx, runtime, command.ID); again.Outcome != store.AlreadyCommitted {
			t.Fatal("capture QA replay fence failed")
		}
		if !e.WaitForIdle(3 * time.Second) {
			t.Fatal("capture QA reasoning did not settle")
		}
		t.Log("native X capture persisted once in disposable QA Store; reader windows survived; replay did not recapture")
	})
}
