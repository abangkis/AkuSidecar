//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"io"
	"net/http"
	"os"
	"time"
	"unsafe"
)

// This full journey uses the existing DEV3 Engine/SQLite/native preparation and
// OS broker contracts. It creates no reader through DOM/CDP input and never
// navigates the canonical X permalink; PrepareNativePost maps it to loopback.
func verifyDevelopmentFrontendNativeJourney(ctx context.Context, r *engine.DevelopmentFixtureRuntime, server *httpapi.Server, origin string, ui *appshell.Session, protocol appshell.CaptureProtocol, uiSession string, request readerbroker.Request, helper appshell.DevelopmentReaderHelperProof, activations <-chan developmentActivation, observation *httpapi.DevelopmentFrontendObservation, batch domain.Session, collectorPID int, controls <-chan string) error {
	if request.Validate() != nil || request.Source != "x" || request.URL != "https://x.com/aku_fixture_only/status/1" || helper.PID == 0 || helper.Parent != uint32(ui.PID()) || helper.Image != r.NativeHost.HelperPath || !helper.JobMember || !helper.TopmostReleased || helper.Created == "" || helper.AgeMS < 0 || helper.AgeMS >= readerbroker.PreparationLifetime.Milliseconds() {
		return errors.New("actual native frontend helper identity disagreement")
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, helper.PID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	var created, exited, kernel, user windows.Filetime
	if err = windows.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil || fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime) != helper.Created {
		return errors.New("native helper retained handle creation mismatch")
	}
	image, err := readerbroker.ProcessImage(helper.PID)
	if err != nil || image != helper.Image {
		return errors.New("native helper retained image mismatch")
	}
	var dom developmentFrontendDOM
	if err = developmentWait(ctx, func() bool {
		var e error
		dom, e = developmentFrontendSnapshot(ctx, protocol, uiSession)
		return e == nil && dom.ClickRequestID == request.RequestID
	}); err != nil {
		return err
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_click", "PID": os.Getpid(), "UIPID": ui.PID(), "HelperPID": helper.PID, "PhysicalOpen": true, "Frontend": map[string]any{"Helper": helper, "RequestID": request.RequestID, "ActualFrontendClick": true, "HelperHandlePinned": true}}); err != nil {
		return err
	}
	if err = developmentWait(ctx, func() bool { return r.Coordinator.Status().BatchPaused }); err != nil {
		return err
	}
	paused, err := r.State.FixtureCaptureSessionPaused(ctx, batch.ID)
	if err != nil || !paused || r.CurrentPID() != collectorPID || r.Owner.Snapshot().Driver != "headless" {
		return errors.New("native frontend did not persist pause before capture drain")
	}
	r.ReleaseCapture()
	reasoningCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = r.WaitReasoning(reasoningCtx)
	cancel()
	if err != nil {
		return err
	}
	if r.CurrentPID() != collectorPID || r.Owner.Snapshot().Driver != "headless" {
		return errors.New("native frontend owner replaced before reasoning drain")
	}
	select {
	case <-activations:
		return errors.New("native activation preceded reasoning drain")
	default:
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_draining", "PID": os.Getpid(), "SessionID": batch.ID, "CaptureAndReasoningDrained": false, "DurablePause": true, "Frontend": map[string]any{"CaptureReleased": true, "ReasoningBlocked": true, "CollectorRetained": true}}); err != nil {
		return err
	}
	r.ReleaseReasoning()
	var activation developmentActivation
	select {
	case activation = <-activations:
	case <-ctx.Done():
		return ctx.Err()
	}
	if !activation.reply.OK || !activation.reply.Readback || activation.reply.FocusCategory != "exact_reader" {
		return errors.New("native frontend exact-reader activation rejected")
	}
	native := windows.NewLazySystemDLL("user32.dll")
	foreground, _, _ := native.NewProc("GetForegroundWindow").Call()
	var readerPID uint32
	native.NewProc("GetWindowThreadProcessId").Call(foreground, uintptr(unsafe.Pointer(&readerPID)))
	visible, _, _ := native.NewProc("IsWindowVisible").Call(foreground)
	minimized, _, _ := native.NewProc("IsIconic").Call(foreground)
	if uint64(foreground) != activation.target.HWND || readerPID != activation.target.PID || visible == 0 || minimized != 0 {
		return errors.New("native frontend OS exact foreground readback disagreement")
	}
	completionCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err = developmentWait(completionCtx, func() bool {
		var e error
		dom, e = developmentFrontendSnapshot(completionCtx, protocol, uiSession)
		stats := observation.Snapshot()
		return e == nil && dom.NativeOpened && !dom.Rejected && dom.RequestID == request.RequestID && stats.RequestID == request.RequestID && stats.NativeAccepted == 1 && stats.NativeRequests == 1
	}); err != nil {
		return errors.New("actual frontend/relay/API did not complete exact native click")
	}
	if err = r.VerifyFrontend(ctx); err != nil {
		return err
	}
	if state, e := windows.WaitForSingleObject(handle, 3000); e != nil || state != windows.WAIT_OBJECT_0 {
		return errors.New("Chrome-launched native helper did not exit normally after reply")
	}
	flags, err := r.ReaderWindows(ctx)
	if err != nil || len(flags) != 1 || !flags[0] {
		return errors.New("native frontend startup bootstrap not reused")
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
		return errors.New("actual frontend native durable audit missing")
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_accepted", "PID": os.Getpid(), "ReaderPID": readerPID, "ReaderHWND": activation.target.HWND, "Readback": true, "ExactForeground": true, "APICompleted": true, "DurableForegroundAudit": true, "CaptureAndReasoningDrained": true, "BootstrapReused": true, "Frontend": map[string]any{"DOM": dom, "Stats": observation.Snapshot(), "HelperExitedNormally": true, "Applied": activation.reply.Applied}}); err != nil {
		return err
	}
	// Post-positive actions below are API denials or owned synthetic window
	// lifetime operations. They never manufacture another UI/native click.
	token, err := r.State.BridgeToken(ctx)
	if err != nil {
		return err
	}
	for _, negative := range []string{"duplicate", "stale_epoch", "cancelled"} {
		denied := request
		epoch := r.Engine.Epoch()
		if negative == "stale_epoch" {
			epoch = "stale"
		}
		if negative == "cancelled" {
			denied.RequestID = "broker_" + readerbroker.Secret()[:32]
			if err = server.CancelReaderBroker(denied); err != nil {
				return err
			}
		}
		body, _ := json.Marshal(map[string]string{"type": "open_native_post", "requestId": denied.RequestID, "source": denied.Source, "url": denied.URL})
		req, e := http.NewRequestWithContext(ctx, "POST", origin+"/api/split-capture/actions", bytes.NewReader(body))
		if e != nil {
			return e
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Aku-Bridge-Token", token)
		req.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		req.Header.Set("X-Aku-Split-Epoch", epoch)
		reply, e := http.DefaultClient.Do(req)
		if e != nil {
			return e
		}
		_, _ = io.Copy(io.Discard, reply.Body)
		reply.Body.Close()
		if reply.StatusCode != 409 || r.CurrentPID() != int(readerPID) || !r.Coordinator.Status().BatchPaused {
			return errors.New("native frontend negative changed paused reader owner")
		}
	}
	if err = r.OpenIndependentWindow(ctx); err != nil {
		return err
	}
	flags, err = r.ReaderWindows(ctx)
	if err != nil || len(flags) != 2 {
		return errors.New("native frontend independent window missing")
	}
	bootstrap := -1
	for index, value := range flags {
		if value {
			bootstrap = index
		}
	}
	if bootstrap < 0 {
		return errors.New("native frontend bootstrap identity missing")
	}
	if err = r.CloseWindow(ctx, bootstrap); err != nil {
		return err
	}
	flags, err = r.ReaderWindows(ctx)
	if err != nil || len(flags) != 1 || flags[0] || !r.Coordinator.Status().BatchPaused {
		return errors.New("native frontend remaining window lost pause")
	}
	if err = r.CloseWindow(ctx, 0); err != nil {
		return err
	}
	if err = developmentWait(ctx, func() bool { return r.OwnerReleased(int(readerPID)) && r.CaptureCount() == 2 }); err != nil {
		return err
	}
	if err = developmentWait(ctx, func() bool {
		saved, e := r.Engine.Session(ctx, batch.ID)
		return e == nil && (saved.Status == "completed" || saved.Status == "partial")
	}); err != nil {
		return err
	}
	if _, err = readDevelopmentCollectionRuntime(ctx, origin); err != nil {
		return err
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_resumed", "PID": os.Getpid(), "SessionID": batch.ID, "CaptureCount": r.CaptureCount(), "RemainingWindowHeldPause": true, "FinalCloseVerified": true, "APIHeadlessReadback": true, "Frontend": map[string]any{"DuplicateStaleCancelledDenied": true, "HelperExitedNormally": true}}); err != nil {
		return err
	}
	r.StopCollection()
	if !r.Engine.WaitForIdle(3 * time.Second) {
		return errors.New("native frontend collector not idle")
	}
	queued, err := r.Engine.StartVisibleUpdate(ctx, "native frontend parent lifetime queue")
	if err != nil {
		return err
	}
	if err = r.Engine.PauseDevelopmentSession(ctx); err != nil {
		return err
	}
	window := ui.DevelopmentFrontendWindowReadback()
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_lifetime_ready", "PID": os.Getpid(), "UIPID": ui.PID(), "UIHWND": window.HWND, "ChromePID": r.CurrentPID(), "SessionID": queued.ID, "RunID": queued.Runs[0].ID, "DurablePause": true, "Root": r.Root, "Frontend": map[string]any{"HelperExitedNormally": true, "NoKeeper": true}}); err != nil {
		return err
	}
	select {
	case control := <-controls:
		if control != "stop" {
			return errors.New("native frontend stop control invalid")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
