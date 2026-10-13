//go:build windows && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"
)

var brokerFixtureUICount atomic.Int64
var brokerFixtureCleanupFailed atomic.Bool

type brokerUICommand struct {
	Op         string               `json:"op"`
	WrongImage bool                 `json:"wrongImage"`
	Indirect   bool                 `json:"indirect"`
	Request    readerbroker.Request `json:"request"`
}
type brokerUIState struct {
	Ready, Focused, Applied, Armed, Clicked bool
	PID                                     uint32
	HWND                                    uint64
}

func startBrokerFixtureUI(t *testing.T, ctx context.Context) (*appshell.ReaderBrokerFixtureUI, string, brokerUIState) {
	t.Helper()
	if os.Getenv("AKU_BROKER_FIXTURE_ENABLED") != "1" {
		t.Skip("explicit VerifyBroker helper required")
	}
	pipe := os.Getenv("AKU_BROKER_FIXTURE_PIPE")
	ui, err := appshell.LaunchReaderBrokerFixtureUI(ctx, pipe)
	if err != nil {
		t.Fatal(err)
	}
	brokerFixtureUICount.Add(1)
	t.Cleanup(func() {
		if err := ui.Close(); err != nil {
			brokerFixtureCleanupFailed.Store(true)
			t.Error(err)
		}
	})
	if err = readerbroker.Write(ui.Input, brokerUICommand{Op: "start"}); err != nil {
		t.Fatal(err)
	}
	var state brokerUIState
	if err = readerbroker.Read(ui.Output, &state); err != nil {
		t.Fatal(err)
	}
	if !state.Ready || state.PID != uint32(ui.Session.PID()) {
		t.Fatal("owned fixture UI identity invalid")
	}
	if readerbroker.ForegroundPID() != state.PID {
		t.Log("Click the owned window titled 'AkuBrowser Synthetic Reader Broker Fixture' once; waiting at most 60 seconds for OS foreground readback.")
		deadline := time.Now().Add(60 * time.Second)
		for readerbroker.ForegroundPID() != state.PID && time.Now().Before(deadline) && ctx.Err() == nil {
			time.Sleep(50 * time.Millisecond)
		}
	}
	if readerbroker.ForegroundPID() != state.PID {
		t.Fatal("Windows did not foreground the owned fixture UI; the required physical fixture UI click was not observed")
	}
	return ui, pipe, state
}
func focusBrokerFixtureUI(t *testing.T, ui *appshell.ReaderBrokerFixtureUI) brokerUIState {
	t.Helper()
	if err := readerbroker.Write(ui.Input, brokerUICommand{Op: "focus"}); err != nil {
		t.Fatal(err)
	}
	var state brokerUIState
	if err := readerbroker.Read(ui.Output, &state); err != nil {
		t.Fatal(err)
	}
	if !state.Focused || readerbroker.ForegroundPID() != uint32(ui.Session.PID()) {
		t.Fatal("Windows rejected the authorized owned UI focus action")
	}
	return state
}
func brokerFixtureRequest() readerbroker.Request {
	return readerbroker.Request{RequestID: "broker_" + readerbroker.Secret()[:32], Source: "x", URL: "https://x.com/aku_fixture_only/status/1"}
}
func runBrokerFixtureClient(t *testing.T, ui *appshell.ReaderBrokerFixtureUI, wrongImage bool) readerbroker.Reply {
	t.Helper()
	if err := readerbroker.Write(ui.Input, brokerUICommand{Op: "read", WrongImage: wrongImage, Request: brokerFixtureRequest()}); err != nil {
		t.Fatal(err)
	}
	var result readerbroker.Reply
	if err := readerbroker.Read(ui.Output, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func waitBrokerFixturePipe(t *testing.T, pipe string) {
	t.Helper()
	name, _ := windows.UTF16PtrFromString(pipe)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ok, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("WaitNamedPipeW").Call(uintptr(unsafe.Pointer(name)), 25)
		if ok != 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("isolated broker pipe was not ready")
}

func TestBrokerFixtureForegroundPrerequisite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	ui, _, state := startBrokerFixtureUI(t, ctx)
	focusBrokerFixtureUI(t, ui)
	t.Logf("owned native UI foreground readback: pid=%d hwnd=%d", state.PID, state.HWND)
}

func TestBrokerFixtureOSReaderHandoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ui, pipe, uiState := startBrokerFixtureUI(t, ctx)
	fixture := startChromeFixtureHTTP(t)
	defer fixture.server.Close()
	config, bridge := startChromeFixtureForTest(t, fixture)
	backend := newChromeFixtureCaptureBackend(bridge, true)
	h := newChromeFixtureEngineHarness(t, config, bridge, fixture, backend, nil, "")
	session, err := h.engine.StartVisibleUpdate(ctx, "OS broker fixture handoff")
	if err != nil {
		t.Fatal(err)
	}
	h.engine.StartHeadlessCollection(h.ctx)
	select {
	case <-backend.entered:
	case <-ctx.Done():
		t.Fatal("collector did not enter")
	}
	collector := backend.current.Load()
	var exactTarget readerbroker.Target
	var activationResult readerbroker.Reply
	type acceptedActivation struct {
		target readerbroker.Target
		reply  readerbroker.Reply
	}
	accepted := make(chan acceptedActivation, 1)
	var targetBinding *appshell.ReaderBrokerFixtureBinding
	var fixtureMode atomic.Int64
	var handlerCalls atomic.Int64
	brokerCtx, stopBroker := context.WithCancel(ctx)
	brokerDone := make(chan error, 1)
	go func() {
		brokerDone <- ui.Session.ServeReaderBrokerFixture(brokerCtx, pipe, func(handleCtx context.Context, req readerbroker.Request, exchange func(readerbroker.Target) (readerbroker.Reply, error)) error {
			handlerCalls.Add(1)
			if req.Source != "x" || req.URL != "https://x.com/aku_fixture_only/status/1" {
				return errors.New("fixture metadata mapping rejected")
			}
			lease, release, err := h.engine.BorrowNativeReader(handleCtx)
			if err != nil {
				return err
			}
			defer release()
			defer lease.Release()
			owner := backend.current.Load()
			if owner == nil {
				return errors.New("fixture reader absent")
			}
			marker := "AkuBrowser reader split_fixture_" + readerbroker.Secret()[:32]
			if err = bridge.rpc(handleCtx, "reader_marker", map[string]any{"owner": owner.owner, "marker": marker}, nil); err != nil {
				return err
			}
			if targetBinding == nil {
				targetBinding, err = appshell.NewReaderBrokerFixtureBinding(bridge.job, owner.pid)
				if err != nil {
					return err
				}
			}
			target, verify, err := targetBinding.Prepare(handleCtx, marker)
			if err != nil {
				return err
			}
			switch fixtureMode.Load() {
			case 1:
				target.HWND = uiState.HWND
			case 2:
				target.PID = uiState.PID
			case 3:
				target.Property = "AkuBrowser.ExplicitReader.fixture-missing"
			case 4:
				target.Value++
			case 5:
				target.Expires = time.Now().Add(-time.Second)
			}
			result, err := exchange(target)
			if err != nil {
				_ = verify(handleCtx)
				return err
			}
			diagnosticBool := func(value *bool) string {
				if value == nil {
					return "unknown"
				}
				return strconv.FormatBool(*value)
			}
			t.Logf("native activation result: reason=%s Applied=%t Readback=%t focus=%s visible=%s minimized=%s", readerbroker.ActivationReason(result, nil), result.Applied, result.Readback, readerbroker.DiagnosticFocusCategory(result), diagnosticBool(result.ReaderVisible), diagnosticBool(result.ReaderMinimized))
			if fixtureMode.Load() != 0 {
				_ = verify(handleCtx)
				if result.OK || result.Applied || result.Readback {
					return errors.New("fixture invalid binding unexpectedly activated")
				}
				return errors.New("fixture denied invalid binding: " + result.Message)
			}
			if !result.OK || !result.Readback {
				accepted <- acceptedActivation{target: target, reply: result}
				_ = verify(handleCtx)
				return errors.New("fixture actual broker activation rejected: " + result.Message)
			}
			if err = verify(handleCtx); err != nil {
				return err
			}
			if verify(handleCtx) == nil {
				return errors.New("fixture native capability replay accepted")
			}
			accepted <- acceptedActivation{target: target, reply: result}
			return nil
		})
	}()
	t.Cleanup(func() {
		stopBroker()
		select {
		case err := <-brokerDone:
			if err != nil {
				brokerFixtureCleanupFailed.Store(true)
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			brokerFixtureCleanupFailed.Store(true)
			t.Error("isolated broker did not stop")
		}
	})
	waitBrokerFixturePipe(t, pipe)
	focusBrokerFixtureUI(t, ui)
	if err = readerbroker.Write(ui.Input, brokerUICommand{Op: "read", Indirect: true, Request: brokerFixtureRequest()}); err != nil {
		t.Fatal(err)
	}
	var indirect readerbroker.Reply
	if err = readerbroker.Read(ui.Output, &indirect); err != nil {
		t.Fatal(err)
	}
	if indirect.OK || indirect.Message != "AkuBrowser UI must remain foreground" || handlerCalls.Load() != 0 {
		t.Fatalf("indirect helper parent gate failed: %+v", indirect)
	}
	denied := runBrokerFixtureClient(t, ui, true)
	if denied.OK || !strings.Contains(denied.Message, "helper image mismatch") || handlerCalls.Load() != 0 {
		t.Fatalf("wrong helper image reached handoff: %+v", denied)
	}
	focusBrokerFixtureUI(t, ui)
	if err = readerbroker.Write(ui.Input, brokerUICommand{Op: "read", Request: brokerFixtureRequest()}); err != nil {
		t.Fatal(err)
	}
	waitChromeFixture(t, func() bool { return h.coordinator.Status().BatchPaused }, "OS reader fences batch while capture drains")
	if collector.exited.Load() || backend.current.Load() != collector {
		t.Fatal("broker retired collector before drain")
	}
	close(backend.release)
	var result readerbroker.Reply
	if err = readerbroker.Read(ui.Output, &result); err != nil {
		t.Fatal(err)
	}
	var outcome acceptedActivation
	select {
	case outcome = <-accepted:
	default:
		t.Fatalf("actual native helper/client/server failed without activation evidence: %+v", result)
	}
	path := "cold_actual_success"
	if !result.OK {
		reply := outcome.reply
		known := reply.Message == "Reader intent expired or UI foreground changed" && (reply.FocusCategory == "other" || reply.FocusCategory == "none") && !reply.OK && !reply.Readback && !reply.Applied && reply.ReaderVisible != nil && reply.ReaderMinimized != nil
		known = known || readerbroker.CanNavigatePostWithoutForeground(reply)
		if !known || !strings.HasPrefix(result.Message, "fixture actual broker activation rejected: ") {
			t.Fatalf("cold failure was not an exact supported activation denial: final=%+v activation=%+v", result, reply)
		}
		owner := backend.current.Load()
		if owner == collector || owner == nil || owner.exited.Load() || !h.coordinator.Status().BatchPaused || bridge.launchCount.Load() != 2 {
			t.Fatal("cold rejection did not retain reader/windows/paused queue")
		}
		freshRequest := brokerFixtureRequest()
		if err = readerbroker.Write(ui.Input, brokerUICommand{Op: "arm_retry", Request: freshRequest}); err != nil {
			t.Fatal(err)
		}
		var armed brokerUIState
		if err = readerbroker.Read(ui.Output, &armed); err != nil || !armed.Armed {
			t.Fatal("physical retry button did not arm")
		}
		t.Logf("Cold actual activation denied: reason=%s. Chrome is ready. Click 'Retry explicit reader' inside 'AkuBrowser Synthetic Reader Broker Fixture' once now to authorize a fresh request/helper/ticket; no intent is replayed automatically.", readerbroker.ActivationReason(reply, nil))
		clicked := make(chan brokerUIState, 1)
		clickError := make(chan error, 1)
		go func() {
			var state brokerUIState
			if err := readerbroker.Read(ui.Output, &state); err != nil {
				clickError <- err
				return
			}
			clicked <- state
		}()
		select {
		case state := <-clicked:
			if !state.Clicked || !state.Focused || state.PID != uint32(ui.Session.PID()) || state.HWND != uiState.HWND {
				t.Fatal("fresh physical retry intent was not observed from owned UI")
			}
		case err := <-clickError:
			t.Fatal(err)
		case <-time.After(60 * time.Second):
			t.Fatal("physical retry button click was not observed within 60 seconds")
		}
		if err = readerbroker.Read(ui.Output, &result); err != nil {
			t.Fatal(err)
		}
		if !result.OK {
			t.Fatalf("fresh explicit helper actual activation failed: %+v", result)
		}
		select {
		case outcome = <-accepted:
		default:
			t.Fatal("fresh actual success had no native activation evidence")
		}
		if backend.current.Load() != owner || bridge.launchCount.Load() != 2 {
			t.Fatal("fresh retry replaced retained reader owner")
		}
		path = "warm_retained_owner_fresh_physical_intent"
	}
	t.Logf("broker accepted path=%s", path)
	exactTarget = outcome.target
	activationResult = outcome.reply
	if !activationResult.OK || !activationResult.Readback || activationResult.FocusCategory != "exact_reader" {
		t.Fatal("actual helper did not return successful exact-reader readback")
	}
	// The terminal server reply is minimal; the binding verifier above must have
	// consumed the actual client OK+Readback result before sending it.
	var pid uint32
	user := windows.NewLazySystemDLL("user32.dll")
	foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
	user.NewProc("GetWindowThreadProcessId").Call(foreground, uintptr(unsafe.Pointer(&pid)))
	visible, _, _ := user.NewProc("IsWindowVisible").Call(foreground)
	minimized, _, _ := user.NewProc("IsIconic").Call(foreground)
	if foreground != uintptr(exactTarget.HWND) || pid != exactTarget.PID || visible == 0 || minimized != 0 || pid == uiState.PID {
		t.Fatal("OS exact reader foreground/visibility/PID readback failed")
	}
	owner := backend.current.Load()
	evidence, err := owner.Evidence(ctx)
	if err != nil || evidence.AccountID != "alpha" || !evidence.Authenticated || fixture.logins.Load() != 1 {
		t.Fatal("same synthetic session did not reach real reader")
	}
	if !collector.exited.Load() || bridge.launchCount.Load() != 2 || !h.coordinator.Status().BatchPaused {
		t.Fatal("reader/batch ownership was not preserved")
	}
	flags, err := owner.FixtureBootstrapWindows(ctx)
	if err != nil || len(flags) != 2 || countBootstrapWindows(flags) != 1 {
		t.Fatal("reader/independent bootstrap windows not retained")
	}
	before := handlerCalls.Load()
	changed := runBrokerFixtureClient(t, ui, false)
	if changed.OK || changed.Message != "AkuBrowser UI must remain foreground" || handlerCalls.Load() != before || readerbroker.ForegroundPID() != exactTarget.PID {
		t.Fatalf("changed UI foreground allowed a new activation: %+v", changed)
	}
	t.Logf("actual RunClient activation accepted: Applied=%t Readback=%t exact reader HWND=%d PID=%d", activationResult.Applied, activationResult.Readback, exactTarget.HWND, exactTarget.PID)
	for index, name := range []string{"wrong_hwnd", "wrong_pid", "wrong_property", "wrong_value", "expired"} {
		t.Run(name, func(t *testing.T) {
			fixtureMode.Store(int64(index + 1))
			focusBrokerFixtureUI(t, ui)
			denied := runBrokerFixtureClient(t, ui, false)
			if denied.OK || (!strings.Contains(denied.Message, "fixture denied invalid binding") && !(name == "expired" && strings.Contains(denied.Message, "reader binding expired or invalid"))) {
				t.Fatalf("actual helper invalid binding denial missing: %+v", denied)
			}
			if readerbroker.ForegroundPID() != uint32(ui.Session.PID()) || !h.coordinator.Status().BatchPaused || backend.current.Load() != owner {
				t.Fatal("invalid binding changed foreground or released queue/owner")
			}
		})
	}
	fixtureMode.Store(0)
	closeChromeFixtureReaderWindow(t, ctx, owner)
	if owner.exited.Load() || !h.coordinator.Status().BatchPaused {
		t.Fatal("independent remaining window did not block batch")
	}
	if err = owner.CloseFixtureWindow(ctx, 0); err != nil {
		t.Fatal(err)
	}
	waitChromeFixture(t, func() bool {
		return h.coordinator.Status().Effective == "headless" && !h.coordinator.Status().BatchPaused
	}, "verified whole-tree release and collector resume")
	waitChromeFixture(t, func() bool { return backend.calls.Load() == 2 }, "durable continuation after actual broker reader closes")
	resumed := backend.current.Load()
	if !owner.rootExited.Load() || !owner.exited.Load() || bridge.launchCount.Load() != 3 || bridge.releaseProofs.Load() < 3 {
		t.Fatal("collector resumed without natural root exit and whole-Job Chrome release proof")
	}
	evidence, err = resumed.Evidence(ctx)
	if err != nil || evidence.AccountID != "alpha" || fixture.logins.Load() != 1 || resumed.PID() == owner.PID() {
		t.Fatal("same authenticated profile resume failed")
	}
	persisted, err := h.state.GetSession(ctx, session.ID)
	if err != nil || len(persisted.Runs) != 2 {
		t.Fatal("durable batch plan missing")
	}
	t.Logf("actual broker success: path=%s helper direct UI child, authenticated unique pipe, exact HWND=%d PID=%d visible=true minimized=false; same synthetic login; batch resumed; launches=%d whole-Job release proofs=%d", path, exactTarget.HWND, exactTarget.PID, bridge.launchCount.Load(), bridge.releaseProofs.Load())
}
