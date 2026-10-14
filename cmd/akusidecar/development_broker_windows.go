//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync/atomic"
	"time"
	"unsafe"
)

type developmentUICommand struct {
	Op                                         string               `json:"op"`
	Present                                    bool                 `json:"present"`
	Development                                bool                 `json:"development"`
	Request                                    readerbroker.Request `json:"request"`
	APIAddress, Token, CaptureKey, EngineEpoch string
}
type developmentUIState struct {
	APIElapsedMS                                                                                                                                     int64
	Ready, Armed, Clicked, Focused, Visible, Minimized, ButtonVisible, ButtonEnabled, RectKnown, ButtonRectKnown, Topmost, Presented, StartupApplied bool
	PID                                                                                                                                              uint32
	HWND                                                                                                                                             uint64
	Rect, ButtonRect, WorkArea                                                                                                                       developmentUIRect
	Op, Intent, Error                                                                                                                                string
	Reply                                                                                                                                            readerbroker.Reply
	APIStatus                                                                                                                                        int
	APIBody                                                                                                                                          json.RawMessage
	HelperPID                                                                                                                                        uint32
	HelperStartupBlocked                                                                                                                             bool
	OK                                                                                                                                               bool
	Message                                                                                                                                          string
}
type developmentUIRect struct{ Left, Top, Right, Bottom int32 }

func (r developmentUIRect) inside(o developmentUIRect) bool {
	return r.Right > r.Left && r.Bottom > r.Top && r.Left >= o.Left && r.Top >= o.Top && r.Right <= o.Right && r.Bottom <= o.Bottom
}

type developmentActivation struct {
	target readerbroker.Target
	reply  readerbroker.Reply
}

func developmentWait(ctx context.Context, check func() bool) error {
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for !check() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
	return nil
}

func runDevelopmentBroker(ctx context.Context, r *engine.DevelopmentFixtureRuntime) (returnErr error) {
	if os.Getenv("AKU_DEV_BROKER_ENABLED") != "1" {
		return errors.New("development broker requires explicit staged validation")
	}
	pipe := os.Getenv("AKU_BROKER_FIXTURE_PIPE")
	if err := readerbroker.ValidateFixturePipe(pipe); err != nil {
		return err
	}
	cfg := config.Config{Dev: true, Root: r.Root, WindowsCaptureSplit: true, Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	server, err := httpapi.New(cfg, r.State, r.Engine, log.New(io.Discard, "", 0))
	if err != nil {
		return err
	}
	if err := server.SetSplitCaptureRuntime(r.Owner); err != nil {
		return err
	}
	server.SetSplitDirectNativeReader(r.PrepareNativePost, func(context.Context) error { return nil })
	address, err := server.Start()
	if err != nil {
		return err
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Stop(stopCtx); err != nil {
			returnErr = err
		}
	}()
	origin := "http://" + address.String()
	launchURL, err := server.SplitCaptureLaunchURL(origin)
	if err != nil {
		return err
	}
	parsed, err := url.Parse(launchURL)
	if err != nil {
		return err
	}
	token, err := r.State.BridgeToken(ctx)
	if err != nil {
		return err
	}
	ui, err := appshell.LaunchReaderBrokerFixtureUI(ctx, pipe)
	if err != nil {
		return err
	}
	defer func() {
		if err := ui.Close(); err != nil {
			returnErr = err
		}
	}()
	if err := readerbroker.Write(ui.Input, developmentUICommand{Op: "start", Present: true, Development: true}); err != nil {
		return err
	}
	var ready developmentUIState
	if err := readerbroker.Read(ui.Output, &ready); err != nil {
		return err
	}
	if !ready.Ready || ready.PID != uint32(ui.Session.PID()) || !ready.Presented || !ready.Visible || ready.Minimized || ready.StartupApplied || !ready.Topmost || !ready.RectKnown || !ready.Rect.inside(ready.WorkArea) {
		return errors.New("development owned UI presentation proof failed")
	}
	brokerCtx, cancelBroker := context.WithCancel(ctx)
	defer cancelBroker()
	brokerDone := make(chan error, 1)
	activation := make(chan developmentActivation, 2)
	var brokerCalls atomic.Int64
	go func() {
		brokerDone <- ui.Session.ServeReaderBrokerFixture(brokerCtx, pipe, func(c context.Context, request readerbroker.Request, exchange func(readerbroker.Target) (readerbroker.Reply, error)) error {
			brokerCalls.Add(1)
			return server.HandleReaderBroker(c, request, func(target readerbroker.Target) (readerbroker.Reply, error) {
				reply, err := exchange(target)
				activation <- developmentActivation{target, reply}
				return reply, err
			})
		})
	}()
	defer func() {
		cancelBroker()
		select {
		case err := <-brokerDone:
			if err != nil {
				returnErr = err
			}
		case <-time.After(3 * time.Second):
			returnErr = errors.New("development broker shutdown incomplete")
		}
	}()
	session, err := r.StartBatch(ctx)
	if err != nil {
		return err
	}
	defer r.StopCollection()
	collectorPID := r.CurrentPID()
	request := readerbroker.Request{RequestID: "broker_" + readerbroker.Secret()[:32], Source: "x", URL: "https://x.com/aku_fixture_only/status/1"}
	command := developmentUICommand{Op: "arm_open", Development: true, Request: request, APIAddress: origin, Token: token, CaptureKey: parsed.Fragment, EngineEpoch: r.Engine.Epoch()}
	if err := readerbroker.Write(ui.Input, command); err != nil {
		return err
	}
	var armed developmentUIState
	if err := readerbroker.Read(ui.Output, &armed); err != nil {
		return err
	}
	if !armed.Armed || !armed.Visible || armed.Minimized || !armed.Topmost || !armed.ButtonVisible || !armed.ButtonEnabled || !armed.ButtonRectKnown || !armed.RectKnown || !armed.ButtonRect.inside(armed.Rect) {
		return errors.New("development Open button readback failed")
	}
	if err := r.Emit(map[string]any{"Stage": "dev_broker_ready", "PID": os.Getpid(), "UIPID": armed.PID, "UIHWND": armed.HWND, "Root": r.Root, "Address": origin, "ButtonVisible": true, "ButtonEnabled": true, "ButtonRect": armed.ButtonRect, "UIRect": armed.Rect, "PresentedWithoutFocus": !ready.StartupApplied, "CaptureActive": r.CaptureCount() == 1, "CollectorPID": collectorPID, "SessionID": session.ID}); err != nil {
		return err
	}
	events := make(chan developmentUIState, 4)
	uiError := make(chan error, 1)
	go func() {
		for {
			var event developmentUIState
			if err := readerbroker.Read(ui.Output, &event); err != nil {
				uiError <- err
				return
			}
			events <- event
		}
	}()
	controls := make(chan string, 2)
	go func() {
		for {
			control, err := r.ReadControl()
			if err != nil {
				return
			}
			controls <- control
		}
	}()
	var operatorDeadline <-chan time.Time
	var clicked developmentUIState
	waiting := true
	for waiting {
		select {
		case control := <-controls:
			if control != "operator_cue" || operatorDeadline != nil {
				return errors.New("development operator cue invalid")
			}
			operatorDeadline = time.After(120 * time.Second)
		case event := <-events:
			clicked = event
			waiting = false
		case err := <-uiError:
			return err
		case <-operatorDeadline:
			return errors.New("physical Open click not observed after operator cue")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if !clicked.Clicked || !clicked.Focused || clicked.Intent != "open" || clicked.PID != armed.PID || clicked.HWND != armed.HWND || clicked.Topmost || clicked.Error != "" {
		return errors.New("development physical Open intent proof failed")
	}
	if err := r.Emit(map[string]any{"Stage": "dev_broker_click", "PID": os.Getpid(), "UIPID": clicked.PID, "UIHWND": clicked.HWND, "PhysicalOpen": true}); err != nil {
		return err
	}
	// A real click starts native clocks immediately, even before coordinator cue.
	if err := developmentWait(ctx, func() bool { return r.Coordinator.Status().BatchPaused }); err != nil {
		return err
	}
	if r.CurrentPID() != collectorPID || r.Owner.Snapshot().Driver != "headless" {
		return errors.New("collector retired before physical capture drained")
	}
	r.ReleaseCapture()
	reasoningCtx, cancelReasoning := context.WithTimeout(ctx, 3*time.Second)
	err = r.WaitReasoning(reasoningCtx)
	cancelReasoning()
	if err != nil {
		return err
	}
	if r.CurrentPID() != collectorPID || r.Owner.Snapshot().Driver != "headless" {
		return errors.New("reader replaced collector before reasoning drain")
	}
	select {
	case <-events:
		return errors.New("native request finished before reasoning drain")
	case <-time.After(40 * time.Millisecond):
	}
	r.ReleaseReasoning()
	var result developmentUIState
	select {
	case result = <-events:
	case err := <-uiError:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
	if result.Op != "open_result" || result.Error != "" || !result.Reply.OK || result.APIStatus != 200 {
		return fmt.Errorf("cold physical broker failed: helper=%s api=%d error=%s apiElapsedMS=%d", result.Reply.Message, result.APIStatus, result.Error, result.APIElapsedMS)
	}
	var api struct {
		OK     bool
		Result struct {
			State      string
			Foreground string
		}
	}
	if json.Unmarshal(result.APIBody, &api) != nil || !api.OK || api.Result.State != "native_post_opened" || api.Result.Foreground != "" {
		return errors.New("actual server did not complete exact foreground post")
	}
	var accepted developmentActivation
	select {
	case accepted = <-activation:
	default:
		return errors.New("actual server activation evidence missing")
	}
	if !accepted.reply.OK || !accepted.reply.Readback || accepted.reply.FocusCategory != "exact_reader" {
		return errors.New("first native helper did not verify exact reader")
	}
	native := windows.NewLazySystemDLL("user32.dll")
	foreground, _, _ := native.NewProc("GetForegroundWindow").Call()
	var pid uint32
	native.NewProc("GetWindowThreadProcessId").Call(foreground, uintptr(unsafe.Pointer(&pid)))
	visible, _, _ := native.NewProc("IsWindowVisible").Call(foreground)
	minimized, _, _ := native.NewProc("IsIconic").Call(foreground)
	if uint64(foreground) != accepted.target.HWND || pid != accepted.target.PID || visible == 0 || minimized != 0 {
		return errors.New("OS exact reader readback disagreement")
	}
	flags, err := r.ReaderWindows(ctx)
	if err != nil || len(flags) != 1 || !flags[0] {
		return errors.New("actual startup bootstrap not reused")
	}
	audit, err := r.State.ReadSplitActionAudit(ctx, "", 64)
	if err != nil {
		return err
	}
	foregroundAudit, resultAudit := false, false
	for _, row := range audit {
		if row.Phase == "reader_foreground" && row.Outcome == "accepted" {
			foregroundAudit = true
		}
		if row.Phase == "result" && row.Outcome == "accepted" {
			resultAudit = true
		}
	}
	if !foregroundAudit || !resultAudit {
		return errors.New("actual server durable reader audit missing")
	}
	if err := r.Emit(map[string]any{"Stage": "dev_broker_accepted", "PID": os.Getpid(), "ReaderPID": pid, "ReaderHWND": accepted.target.HWND, "Applied": accepted.reply.Applied, "Readback": true, "ExactForeground": true, "APICompleted": true, "DurableForegroundAudit": true, "CaptureAndReasoningDrained": true, "BootstrapReused": true}); err != nil {
		return err
	}
	// Programmatic follow-ups are denials only; no fabricated focus or input.
	command.Op = "read"
	command.Development = false
	if err := readerbroker.Write(ui.Input, command); err != nil {
		return err
	}
	var denied developmentUIState
	select {
	case denied = <-events:
	case <-ctx.Done():
		return ctx.Err()
	}
	// Legacy read emits the terminal Reply directly.
	if denied.OK || denied.Message != "AkuBrowser UI must remain foreground" || brokerCalls.Load() != 1 || readerbroker.ForegroundPID() != pid {
		return errors.New("changed foreground replay denial not verified")
	}
	for _, negative := range []string{"duplicate", "stale_epoch", "cancelled"} {
		deniedRequest := request
		epoch := r.Engine.Epoch()
		if negative == "stale_epoch" {
			epoch = "stale"
		}
		if negative == "cancelled" {
			deniedRequest.RequestID = "broker_" + readerbroker.Secret()[:32]
			if err := server.CancelReaderBroker(deniedRequest); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(map[string]string{"type": "open_native_post", "source": deniedRequest.Source, "url": deniedRequest.URL, "requestId": deniedRequest.RequestID})
		req, err := http.NewRequestWithContext(ctx, "POST", origin+"/api/split-capture/actions", bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		req.Header.Set("X-Aku-Bridge-Token", token)
		req.Header.Set("X-Aku-Capture-Instance", parsed.Fragment)
		req.Header.Set("X-Aku-Split-Epoch", epoch)
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 409 || r.CurrentPID() != int(pid) || !r.Coordinator.Status().BatchPaused {
			return errors.New("actual server negative admission changed reader ownership")
		}
	}
	if err := r.OpenIndependentWindow(ctx); err != nil {
		return err
	}
	flags, err = r.ReaderWindows(ctx)
	if err != nil || len(flags) != 2 {
		return errors.New("independent reader ownership missing")
	}
	readerPID := r.CurrentPID()
	bootstrapIndex := -1
	for index, bootstrap := range flags {
		if bootstrap {
			bootstrapIndex = index
		}
	}
	if bootstrapIndex < 0 {
		return errors.New("bootstrap identity missing")
	}
	if err := r.CloseWindow(ctx, bootstrapIndex); err != nil {
		return err
	}
	flags, err = r.ReaderWindows(ctx)
	if err != nil || len(flags) != 1 || flags[0] || !r.Coordinator.Status().BatchPaused {
		return errors.New("remaining window did not preserve paused owner")
	}
	if err := r.CloseWindow(ctx, 0); err != nil {
		return err
	}
	if err := developmentWait(ctx, func() bool { return r.OwnerReleased(readerPID) && r.CaptureCount() == 2 }); err != nil {
		return err
	}
	if err := developmentWait(ctx, func() bool {
		saved, err := r.Engine.Session(ctx, session.ID)
		return err == nil && (saved.Status == "completed" || saved.Status == "partial")
	}); err != nil {
		return err
	}
	// Actual API status is read back after final close, beyond Engine borrow.
	if _, err := readDevelopmentCollectionRuntime(ctx, origin); err != nil {
		return err
	}
	if err := r.Emit(map[string]any{"Stage": "dev_broker_resumed", "PID": os.Getpid(), "SessionID": session.ID, "CaptureCount": r.CaptureCount(), "RemainingWindowHeldPause": true, "FinalCloseVerified": true, "APIHeadlessReadback": true}); err != nil {
		return err
	}
	r.StopCollection()
	if !r.Engine.WaitForIdle(3 * time.Second) {
		return errors.New("dev collector stop not idle")
	}
	queued, err := r.Engine.StartVisibleUpdate(ctx, "development UI/helper owned lifetime queue")
	if err != nil {
		return err
	}
	if err := r.Engine.PauseDevelopmentSession(ctx); err != nil {
		return err
	}
	if err := readerbroker.Write(ui.Input, developmentUICommand{Op: "hold_helper", Development: true}); err != nil {
		return err
	}
	var held developmentUIState
	select {
	case held = <-events:
	case <-ctx.Done():
		return ctx.Err()
	}
	if held.Op != "helper_staged" || held.HelperPID == 0 || !held.HelperStartupBlocked || held.Error != "" {
		return errors.New("owned helper startup staging failed")
	}
	if err := r.Emit(map[string]any{"Stage": "dev_broker_lifetime_ready", "PID": os.Getpid(), "UIPID": ui.Session.PID(), "UIHWND": armed.HWND, "HelperPID": held.HelperPID, "HelperStartupBlocked": true, "NativeIntent": false, "ChromePID": r.CurrentPID(), "SessionID": queued.ID, "RunID": queued.Runs[0].ID, "DurablePause": true, "Root": r.Root}); err != nil {
		return err
	}
	select {
	case control := <-controls:
		if control != "stop" {
			return errors.New("development stop control invalid")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func readDevelopmentCollectionRuntime(ctx context.Context, origin string) (collection.RuntimeStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/api/collection/runtime", nil)
	if err != nil {
		return collection.RuntimeStatus{}, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return collection.RuntimeStatus{}, err
	}
	defer response.Body.Close()
	return decodeDevelopmentCollectionRuntime(response.StatusCode, response.Body)
}

func decodeDevelopmentCollectionRuntime(status int, body io.Reader) (collection.RuntimeStatus, error) {
	var result struct {
		CollectionRuntime *collection.RuntimeStatus `json:"collectionRuntime"`
	}
	if status != 200 || json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&result) != nil || result.CollectionRuntime == nil {
		return collection.RuntimeStatus{}, errors.New("collection runtime API response missing or invalid")
	}
	value := *result.CollectionRuntime
	if !value.Available || value.State != captureruntime.Ready || value.Requested != "headless" || value.Effective != "headless" || value.Pending || value.BatchPaused || !value.DevelopmentSession || value.Generation == 0 {
		return collection.RuntimeStatus{}, errors.New("final close API runtime readback not verified ready headless")
	}
	return value, nil
}
