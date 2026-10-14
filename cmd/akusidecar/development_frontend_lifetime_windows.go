//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"os"
	"reflect"
	"time"
)

// This automatic lane invokes no gesture, native activation or foreground callback.
func verifyDevelopmentFrontendAutomaticLifetime(ctx context.Context, r *engine.DevelopmentFixtureRuntime, ui *appshell.Session, protocol appshell.CaptureProtocol, uiSession string, observation *httpapi.DevelopmentFrontendObservation, batch domain.Session, collectorPID int) error {
	persisted, err := r.ObservePauseBeforeLeaseRelease(batch.ID)
	if err != nil {
		return err
	}
	type borrowResult struct {
		lease   *captureruntime.Lease
		release func()
		err     error
	}
	borrowed := make(chan borrowResult, 1)
	go func() { l, release, e := r.Engine.BorrowNativeReader(ctx); borrowed <- borrowResult{l, release, e} }()
	if err = developmentWait(ctx, func() bool { return r.Coordinator.Status().BatchPaused }); err != nil {
		return err
	}
	if err = validateDevelopmentFrontendPause("capture_blocked", r.PauseEvidence(ctx, batch.ID), collectorPID); err != nil {
		return err
	}
	r.ReleaseCapture()
	reasoningCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	err = r.WaitReasoning(reasoningCtx)
	cancel()
	if err != nil {
		return err
	}
	if err = validateDevelopmentFrontendPause("reasoning_blocked", r.PauseEvidence(ctx, batch.ID), collectorPID); err != nil {
		return err
	}
	r.ReleaseReasoning()
	select {
	case durable := <-persisted:
		if err = validateDevelopmentFrontendPause("persisted_before_lease_release", durable, collectorPID); err != nil {
			return err
		}
	case <-ctx.Done():
		return ctx.Err()
	}
	var borrow borrowResult
	select {
	case borrow = <-borrowed:
	case <-ctx.Done():
		return ctx.Err()
	}
	if borrow.err != nil {
		return borrow.err
	}
	if borrow.lease == nil || borrow.release == nil {
		return errors.New("automatic lifetime reader borrow missing")
	}
	release := func() { borrow.lease.Release(); borrow.release() }
	defer func() {
		if release != nil {
			release()
		}
	}()
	id := "automatic-lifetime"
	prepared, err := r.PrepareNativePost(ctx, id, "https://x.com/aku_fixture_only/status/1", "/split-reader-intent?id="+id)
	if err != nil {
		return err
	}
	// Read only its scoped target identity; never call activation or navigation.
	release()
	release = nil
	r.StopCollection()
	if !r.Engine.WaitForIdle(3*time.Second) || r.CaptureCount() != 1 {
		return errors.New("automatic lifetime collection did not drain once")
	}
	original, err := developmentFrontendQueuedContinuation(batch)
	if err != nil {
		return err
	}
	queued, err := r.Engine.Run(ctx, original.ID)
	if err != nil {
		return err
	}
	commands, err := r.State.FixtureRunCommandEvidence(ctx, queued.ID)
	if err != nil {
		return err
	}
	if err = engine.ValidateDevelopmentLiveReaderQueue(queued, commands); err != nil {
		return err
	}
	claimed, err := r.State.ClaimCommandForCollector(ctx, queued.ID, "automatic-lifetime-probe", "headless")
	if err != nil || claimed != nil {
		return errors.New("automatic lifetime original claim not blocked")
	}
	after, err := r.Engine.Run(ctx, queued.ID)
	if err != nil || !reflect.DeepEqual(after, queued) {
		return errors.New("automatic lifetime blocked claim changed run")
	}
	afterCommands, err := r.State.FixtureRunCommandEvidence(ctx, queued.ID)
	if err != nil || !reflect.DeepEqual(afterCommands, commands) {
		return errors.New("automatic lifetime blocked claim changed command")
	}
	windowsEvidence := r.ReaderWindowEvidence(ctx, batch.ID)
	if failures := developmentFrontendLiveReaderFailures(windowsEvidence, prepared.Target.PID); len(failures) != 0 {
		return errors.New("automatic lifetime owner/pause/window invalid")
	}
	window := ui.DevelopmentFrontendLifetimeWindowReadback()
	if window.HWND == 0 || window.PID != uint32(ui.PID()) || !window.JobMember || !window.TitleMatched || window.Class != "Chrome_WidgetWin_1" {
		return errors.New("automatic lifetime UI readback missing")
	}
	dom, err := developmentFrontendSnapshot(ctx, protocol, uiSession)
	if err != nil {
		return err
	}
	stats := observation.Snapshot()
	if dom.ClickRequestID != "" || dom.RequestID != "" || dom.NativeOpened || dom.Rejected || dom.RelayRejected || stats.NativeRequests != 0 || stats.NativeAccepted != 0 || stats.NativeDenied != 0 || stats.Cancelled != 0 {
		return errors.New("automatic lifetime observed unexpected frontend action")
	}
	if err = r.Emit(map[string]any{"Stage": "dev_frontend_automatic_lifetime_ready", "PID": os.Getpid(), "UIPID": ui.PID(), "UIHWND": window.HWND, "ReaderPID": prepared.Target.PID, "ReaderHWND": prepared.Target.HWND, "SessionID": batch.ID, "RunID": queued.ID, "Root": r.Root, "CaptureCount": r.CaptureCount(), "DurablePause": true, "Frontend": map[string]any{"DOM": dom, "Stats": stats, "Windows": windowsEvidence, "QueuedRun": queued, "Commands": commands, "NativeActivationClosed": true, "NativeHelpersStaged": 0, "ManualClicks": 0, "NoKeeper": true}}); err != nil {
		return err
	}
	control, err := r.ReadControl()
	if err != nil || control != "stop" {
		return errors.New("automatic lifetime controller closed")
	}
	return nil
}
