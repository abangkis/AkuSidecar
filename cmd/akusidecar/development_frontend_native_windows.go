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
	"reflect"
	"time"
	"unsafe"
)

// This full journey uses the existing DEV3 Engine/SQLite/native preparation and
// OS broker contracts. It creates no reader through DOM/CDP input and never
// navigates the canonical X permalink; PrepareNativePost maps it to loopback.
func validateDevelopmentFrontendPause(phase string, v engine.DevelopmentPauseEvidence, collectorPID int) error {
	if v.SQLiteError != "" || !v.AdmissionPaused || v.CollectorPID != collectorPID || v.ManagerPID != collectorPID || v.Driver != "headless" || v.Generation == 0 || v.ActiveLeases < 1 {
		return fmt.Errorf("native frontend pause evidence invalid phase=%s expectedCollectorPID=%d evidence=%+v", phase, collectorPID, v)
	}
	if phase == "persisted_before_lease_release" {
		if !v.SQLitePaused || !v.BeforeLeaseRelease || v.CaptureSteps != 0 || v.ActiveWork != 0 {
			return fmt.Errorf("native frontend durable pause before lease release invalid evidence=%+v", v)
		}
	} else if v.SQLitePaused || v.BeforeLeaseRelease {
		return fmt.Errorf("native frontend premature durable pause phase=%s evidence=%+v", phase, v)
	}
	return nil
}

// Classify only the existing window predicates; this does not loosen or add
// lifecycle admission. SQLite/PID/driver remain separately observed metadata.
func developmentFrontendWindowFailures(phase string, v engine.DevelopmentReaderWindowEvidence) []string {
	failed := []string{}
	if !v.InventoryKnown || v.InventoryError != "" {
		failed = append(failed, "inventory_unknown")
		return failed
	}
	if phase == "before_bootstrap_close" {
		if v.WindowCount != 2 {
			failed = append(failed, "expected_two_windows")
		}
		bootstrap := false
		for _, flag := range v.BootstrapFlags {
			bootstrap = bootstrap || flag
		}
		if !bootstrap {
			failed = append(failed, "bootstrap_missing")
		}
	} else {
		if v.WindowCount != 1 {
			failed = append(failed, "expected_one_remaining_window")
		}
		if len(v.BootstrapFlags) > 0 && v.BootstrapFlags[0] {
			failed = append(failed, "remaining_window_is_bootstrap")
		}
		if !v.CoordinatorPaused {
			failed = append(failed, "coordinator_not_paused")
		}
	}
	return failed
}

func developmentFrontendQueuedContinuation(batch domain.Session) (domain.Run, error) {
	// Store ordinals are zero-based: the second original run has ordinal 1.
	if len(batch.Runs) != 2 || batch.Runs[1].Ordinal != 1 || batch.Runs[1].ID == "" || batch.Runs[1].SessionID != batch.ID || batch.Runs[1].Status != "queued" {
		return domain.Run{}, errors.New("native live-reader original queued continuation missing")
	}
	return batch.Runs[1], nil
}

func developmentFrontendLiveReaderFailures(v engine.DevelopmentReaderWindowEvidence, readerPID uint32) []string {
	failed := []string{}
	if !v.InventoryKnown || v.InventoryError != "" {
		return append(failed, "inventory_unknown")
	}
	if v.WindowCount < 1 || len(v.BootstrapFlags) != v.WindowCount {
		failed = append(failed, "reader_window_missing")
	}
	bootstrap := false
	for _, flag := range v.BootstrapFlags {
		bootstrap = bootstrap || flag
	}
	if !bootstrap {
		failed = append(failed, "bootstrap_missing")
	}
	if !v.CoordinatorPaused {
		failed = append(failed, "coordinator_not_paused")
	}
	if !v.SQLiteKnown || v.Pause.SQLiteError != "" || !v.Pause.SQLitePaused || !v.Pause.AdmissionPaused || v.Pause.CollectorPID != int(readerPID) || v.Pause.ManagerPID != int(readerPID) || v.Pause.Driver != "browser" || v.Pause.Generation == 0 || v.Pause.CaptureSteps != 0 || v.Pause.ActiveWork != 0 {
		failed = append(failed, "reader_pause_owner_invalid")
	}
	return failed
}

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
	persisted, err := r.ObservePauseBeforeLeaseRelease(batch.ID)
	if err != nil {
		return err
	}
	admission := r.PauseEvidence(ctx, batch.ID)
	if err = validateDevelopmentFrontendPause("capture_blocked", admission, collectorPID); err != nil {
		return err
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_admission_paused", "PID": os.Getpid(), "Frontend": map[string]any{"Pause": admission, "ExpectedCollectorPID": collectorPID}}); err != nil {
		return err
	}
	select {
	case <-activations:
		return errors.New("native activation preceded capture drain")
	default:
	}
	r.ReleaseCapture()
	reasoningCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = r.WaitReasoning(reasoningCtx)
	cancel()
	if err != nil {
		return err
	}
	reasoning := r.PauseEvidence(ctx, batch.ID)
	if err = validateDevelopmentFrontendPause("reasoning_blocked", reasoning, collectorPID); err != nil {
		return err
	}
	select {
	case <-activations:
		return errors.New("native activation preceded reasoning drain")
	default:
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_draining", "PID": os.Getpid(), "SessionID": batch.ID, "CaptureAndReasoningDrained": false, "DurablePause": false, "Frontend": map[string]any{"CaptureReleased": true, "ReasoningBlocked": true, "CollectorRetained": true, "Pause": reasoning, "ExpectedCollectorPID": collectorPID}}); err != nil {
		return err
	}
	r.ReleaseReasoning()
	var durable engine.DevelopmentPauseEvidence
	select {
	case durable = <-persisted:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err = validateDevelopmentFrontendPause("persisted_before_lease_release", durable, collectorPID); err != nil {
		return err
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_pause_persisted", "PID": os.Getpid(), "DurablePause": true, "CaptureAndReasoningDrained": true, "Frontend": map[string]any{"Pause": durable, "ExpectedCollectorPID": collectorPID}}); err != nil {
		return err
	}
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
	scenario, err := engine.DevelopmentFrontendNativeScenario()
	if err != nil {
		return err
	}
	if scenario == engine.DevelopmentFrontendNativeScenarioForegroundOnly {
		if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_foreground_complete", "PID": os.Getpid(), "ReaderPID": readerPID, "ReaderHWND": activation.target.HWND, "CaptureCount": r.CaptureCount(), "BootstrapReused": true, "ExactForeground": true}); err != nil {
			return err
		}
		select {
		case control := <-controls:
			if control != "stop" {
				return errors.New("foreground-only graceful stop control invalid")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if scenario == engine.DevelopmentFrontendNativeScenarioReaderOpenParentDeath {
		// Keep the actual source reader open and preserve its original second run.
		// The already-completed helper and accepted action above remain the only
		// native click/login in this scenario.
		r.StopCollection()
		if !r.Engine.WaitForIdle(3*time.Second) || r.CaptureCount() != 1 || r.CurrentPID() != int(readerPID) || r.Owner.PID() != int(readerPID) || r.Owner.Snapshot().Driver != "browser" {
			return errors.New("native live-reader collection/owner state invalid")
		}
		originalRun, e := developmentFrontendQueuedContinuation(batch)
		if e != nil {
			return e
		}
		queuedRun, e := r.Engine.Run(ctx, originalRun.ID)
		if e != nil || queuedRun.SessionID != batch.ID {
			return errors.New("native live-reader original second run missing")
		}
		commands, e := r.State.FixtureRunCommandEvidence(ctx, queuedRun.ID)
		if e != nil {
			return e
		}
		if e = engine.ValidateDevelopmentLiveReaderQueue(queuedRun, commands); e != nil {
			return e
		}
		paused, e := r.State.ClaimCommandForCollector(ctx, queuedRun.ID, "native-live-reader-probe", "headless")
		if e != nil || paused != nil {
			return fmt.Errorf("native live-reader paused second-run claim was not blocked: command=%+v err=%v", paused, e)
		}
		afterProbe, e := r.Engine.Run(ctx, queuedRun.ID)
		if e != nil || !reflect.DeepEqual(afterProbe, queuedRun) {
			return fmt.Errorf("native live-reader queue metadata changed during blocked claim: before=%+v after=%+v err=%v", queuedRun, afterProbe, e)
		}
		afterCommands, e := r.State.FixtureRunCommandEvidence(ctx, queuedRun.ID)
		if e != nil || !reflect.DeepEqual(afterCommands, commands) {
			return errors.New("blocked live-reader claim changed command payload/metadata")
		}
		windowsEvidence := r.ReaderWindowEvidence(ctx, batch.ID)
		windowFailures := developmentFrontendLiveReaderFailures(windowsEvidence, readerPID)
		if r.Coordinator.Status().BatchPaused != windowsEvidence.CoordinatorPaused || len(windowFailures) != 0 {
			return fmt.Errorf("native live-reader evidence invalid failures=%v evidence=%+v", windowFailures, windowsEvidence)
		}
		window := ui.DevelopmentFrontendWindowReadback()
		if window.HWND == 0 || window.PID != uint32(ui.PID()) || !window.Visible || window.Minimized {
			return errors.New("native live-reader UI window readback invalid")
		}
		if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_live_reader_lifetime_ready", "PID": os.Getpid(), "UIPID": ui.PID(), "UIHWND": window.HWND, "ReaderPID": readerPID, "ReaderHWND": activation.target.HWND, "SessionID": batch.ID, "RunID": queuedRun.ID, "CaptureCount": r.CaptureCount(), "DurablePause": true, "Root": r.Root, "Frontend": map[string]any{"Scenario": scenario, "Windows": windowsEvidence, "QueuedRun": queuedRun, "Commands": commands, "HelperExitedNormally": true, "NoKeeper": true}}); err != nil {
			return err
		}
		select {
		case control := <-controls:
			if control != "stop" {
				return errors.New("native live-reader parent-death control invalid")
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if scenario != engine.DevelopmentFrontendNativeScenarioNormal {
		return errors.New("native frontend scenario rejected")
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
	beforeClose := r.ReaderWindowEvidence(ctx, batch.ID)
	beforeFailures := developmentFrontendWindowFailures("before_bootstrap_close", beforeClose)
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_windows_before_close", "PID": os.Getpid(), "Frontend": map[string]any{"Windows": beforeClose, "WindowFailures": beforeFailures, "ExpectedReaderPID": readerPID}}); err != nil {
		return err
	}
	if len(beforeFailures) > 0 {
		return fmt.Errorf("native frontend pre-close window predicates failed=%v evidence=%+v", beforeFailures, beforeClose)
	}
	flags = beforeClose.BootstrapFlags
	bootstrap := -1
	for index, value := range flags {
		if value {
			bootstrap = index
		}
	}
	if bootstrap < 0 {
		return errors.New("native frontend bootstrap identity missing")
	}
	closeErr := r.CloseWindow(ctx, bootstrap)
	closeError := ""
	if closeErr != nil {
		closeError = closeErr.Error()
	}
	afterClose := r.ReaderWindowEvidence(ctx, batch.ID)
	afterFailures := developmentFrontendWindowFailures("after_bootstrap_close", afterClose)
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_native_windows_after_close", "PID": os.Getpid(), "Frontend": map[string]any{"Windows": afterClose, "WindowFailures": afterFailures, "ExpectedReaderPID": readerPID, "ClosedBootstrapIndex": bootstrap, "BootstrapCloseError": closeError}}); err != nil {
		return err
	}
	if closeErr != nil {
		return fmt.Errorf("native frontend bootstrap close failed=%s evidence=%+v", closeError, afterClose)
	}
	if len(afterFailures) > 0 {
		return fmt.Errorf("native frontend remaining window predicates failed=%v evidence=%+v", afterFailures, afterClose)
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
