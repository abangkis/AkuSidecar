package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func (s *Server) beginNativeReaderAction(ctx context.Context) (func(), error) {
	s.nativeReaderGateMu.Lock()
	if s.nativeReaderClosing {
		s.nativeReaderGateMu.Unlock()
		return nil, errors.New("native reader is closing; retry after the current close finishes")
	}
	epoch := s.nativeReaderEpoch
	s.nativeReaderWaiters++
	s.nativeReaderGateMu.Unlock()
	if s.nativeReaderGate == nil {
		s.nativeReaderGateMu.Lock()
		s.nativeReaderWaiters--
		s.nativeReaderGateMu.Unlock()
		return func() {}, nil
	}
	select {
	case s.nativeReaderGate <- struct{}{}:
	case <-ctx.Done():
		s.nativeReaderGateMu.Lock()
		s.nativeReaderWaiters--
		s.nativeReaderGateMu.Unlock()
		return nil, ctx.Err()
	}
	s.nativeReaderGateMu.Lock()
	s.nativeReaderWaiters--
	stale := s.nativeReaderClosing || epoch != s.nativeReaderEpoch
	s.nativeReaderGateMu.Unlock()
	if stale {
		<-s.nativeReaderGate
		return nil, errors.New("native reader close began while this request was waiting")
	}
	return func() { <-s.nativeReaderGate }, nil
}

func (s *Server) beginNativeReaderClose(ctx context.Context) (func(), error) {
	s.nativeReaderGateMu.Lock()
	if s.nativeReaderClosing {
		s.nativeReaderGateMu.Unlock()
		return nil, errors.New("a native reader close is already in progress")
	}
	s.nativeReaderClosing = true
	s.nativeReaderEpoch++
	s.nativeReaderGateMu.Unlock()
	finish := func() {
		s.nativeReaderGateMu.Lock()
		s.nativeReaderClosing = false
		s.nativeReaderGateMu.Unlock()
		if s.nativeReaderGate != nil {
			<-s.nativeReaderGate
		}
	}
	if s.nativeReaderGate == nil {
		return finish, nil
	}
	select {
	case s.nativeReaderGate <- struct{}{}:
		return finish, nil
	case <-ctx.Done():
		s.nativeReaderGateMu.Lock()
		s.nativeReaderClosing = false
		s.nativeReaderGateMu.Unlock()
		return nil, ctx.Err()
	}
}

func (s *Server) closeNativeReaderAndResume(ctx context.Context) (map[string]any, error) {
	finish, err := s.beginNativeReaderClose(ctx)
	if err != nil {
		return nil, err
	}
	defer finish()
	runtimeStatus := s.engine.CollectionRuntime()
	closed := runtimeStatus.NativeReaderOnly
	if !closed {
		autoUpdate, statusErr := s.engine.AutoUpdateStatus(ctx)
		if statusErr != nil {
			return nil, statusErr
		}
		return map[string]any{
			"closed": false, "resumeOutcome": "no_reader",
			"resumeReason":      "The native reader is already released; refresh status before retrying.",
			"collectionRuntime": runtimeStatus, "autoUpdate": autoUpdate,
		}, nil
	}
	if closed {
		if t := s.splitCapture; t != nil {
			t.mu.Lock()
			active := false
			for _, entry := range t.actions {
				if entry != nil && entry.action.Type == "open_native_post" && !entry.completed {
					active = true
					break
				}
			}
			t.mu.Unlock()
			if active {
				return nil, errors.New("a native reader action is still active; wait for it to finish before closing")
			}
		}
		closeAction := s.nativeReaderCloseAction()
		if closeAction == nil {
			return nil, errors.New("native reader close action is unavailable")
		}
		if err := closeAction(ctx); err != nil {
			return nil, err
		}
		s.engine.RetryCollectionRuntime()
	}
	// Only prepare immediately after verified headless readiness. The close is
	// still successful if another configured mode or safety guard prevents it.
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for runtimeStatus.Requested == "headless" && !(runtimeStatus.Effective == "headless" && runtimeStatus.State == "ready" && !runtimeStatus.Pending) {
		select {
		case <-ctx.Done():
			result := map[string]any{"closed": true, "resumeOutcome": "headless_not_ready", "resumeReason": "Reader window closed, but headless capture has not become ready.", "collectionRuntime": runtimeStatus}
			statusCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if autoUpdate, statusErr := s.engine.AutoUpdateStatus(statusCtx); statusErr == nil {
				result["autoUpdate"] = autoUpdate
			}
			cancel()
			return result, nil
		case <-tick.C:
			runtimeStatus = s.engine.CollectionRuntime()
		}
	}
	autoUpdate, err := s.engine.AutoUpdateStatus(ctx)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"closed": closed, "collectionRuntime": runtimeStatus, "autoUpdate": autoUpdate}
	if runtimeStatus.Requested != "headless" || runtimeStatus.Effective != "headless" || runtimeStatus.State != "ready" || runtimeStatus.Pending {
		result["resumeOutcome"] = "guarded"
		result["resumeReason"] = "Headless capture is not ready for an automatic retry."
		return result, nil
	}
	if !autoUpdate.Enabled {
		result["resumeOutcome"] = "guarded"
		result["resumeReason"] = "Auto Update is off."
		return result, nil
	}
	session, err := s.engine.RetryPreparedUpdateAfterReaderClose(ctx)
	if err != nil {
		latest, statusErr := s.engine.AutoUpdateStatus(ctx)
		if statusErr == nil {
			result["autoUpdate"] = latest
		}
		result["resumeOutcome"], result["resumeReason"] = "guarded", err.Error()
		return result, nil
	}
	if session.ID == "" {
		result["resumeOutcome"], result["resumeReason"] = "deferred", "No prepared batch was started."
		return result, nil
	}
	latest, statusErr := s.engine.AutoUpdateStatus(ctx)
	if statusErr == nil {
		result["autoUpdate"] = latest
	}
	result["resumeOutcome"], result["session"] = "started", session
	return result, nil
}

const nativeReaderManualForegroundMessage = "The post opened in your Native Reader tab, but Windows did not bring that window to the foreground. Switch to the Native Reader window manually."

func (s *Server) runDirectNativeReader(ctx context.Context, t *splitCaptureTransport, entry *pendingSplitAction, prepare func(context.Context, string, string, string) (readerbroker.NativePostPreparation, error)) {
	stageStarted := time.Now()
	timing := func(stage string, ok bool) {
		if s.logger != nil {
			s.logger.Printf("native_reader_timing action=%s stage=%s elapsed_ms=%d ok=%t", entry.action.ID, stage, time.Since(stageStarted).Milliseconds(), ok)
		}
		stageStarted = time.Now()
	}
	var outcome error
	defer func() {
		t.mu.Lock()
		manualForegroundRequired := entry.readerManualForegroundRequired
		result := splitActionResult{OK: outcome == nil}
		if outcome != nil {
			result.Message = "Native reader could not complete; close any opened reader window and retry."
		} else if manualForegroundRequired {
			result.Message = nativeReaderManualForegroundMessage
			result.Result, _ = json.Marshal(map[string]string{
				"source": entry.action.Source, "state": "native_post_opened", "url": entry.action.URL,
				"foreground": "manual_required", "message": nativeReaderManualForegroundMessage,
			})
		} else {
			result.Result, _ = json.Marshal(map[string]string{"source": entry.action.Source, "state": "native_post_opened", "url": entry.action.URL})
		}
		entry.completed, entry.completionResult = true, &result
		select {
		case <-entry.brokerReady:
		default:
			close(entry.brokerReady)
		}
		if entry.detached {
			t.removeAction(entry)
		}
		t.mu.Unlock()
		phase := "accepted"
		if outcome != nil {
			phase = "rejected"
		}
		auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		s.auditSplitAction(auditCtx, entry.action, "result", phase)
		cancel()
		select {
		case entry.result <- result:
		default:
		}
	}()
	// API admission alone grants no native foreground capability. Wait for the
	// OS-authenticated helper with the identical request ID, source and URL.
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		t.mu.Lock()
		attached, closed := entry.brokerAttached, t.closed
		if ctx.Err() != nil {
			t.mu.Unlock()
			outcome = ctx.Err()
			return
		}
		present := false
		for _, candidate := range t.actions {
			if candidate == entry {
				present = true
				break
			}
		}
		if !present {
			t.mu.Unlock()
			outcome = errors.New("reader action no longer active")
			return
		}
		if attached && !closed {
			entry.claimed = true
		}
		t.mu.Unlock()
		if closed {
			outcome = errors.New("reader transport closed")
			return
		}
		if attached {
			break
		}
		select {
		case <-ctx.Done():
			outcome = ctx.Err()
			return
		case <-tick.C:
		}
	}
	timing("broker_attach_wait", true)
	s.auditSplitAction(ctx, entry.action, "claimed", "accepted")
	s.auditSplitAction(ctx, entry.action, "reader_prepare", "pending")
	if prepare == nil {
		outcome = errors.New("reader unavailable")
		return
	}
	marker := "/split-reader-intent?id=" + entry.action.ID
	prepared, err := prepare(ctx, entry.action.ID, entry.action.URL, marker)
	timing("target_prepare", err == nil)
	if err != nil {
		outcome = err
		s.auditSplitAction(ctx, entry.action, "reader_prepare", "rejected")
		return
	}
	if prepared.VerifyForeground == nil || prepared.NavigatePost == nil {
		outcome = errors.New("native reader completion is unavailable")
		s.auditSplitAction(ctx, entry.action, "reader_prepare", "rejected")
		return
	}
	foreground := func(activeCtx context.Context) error {
		if err := prepared.VerifyForeground(activeCtx); err != nil {
			return err
		}
		return prepared.NavigatePost(activeCtx)
	}
	t.mu.Lock()
	entry.brokerTarget, entry.readerForeground = prepared.Target, foreground
	entry.readerNavigatePost = prepared.NavigatePost
	close(entry.brokerReady)
	t.mu.Unlock()
	s.auditSplitAction(ctx, entry.action, "reader_prepare", "accepted")
	select {
	case outcome = <-entry.brokerDone:
	case <-ctx.Done():
		outcome = ctx.Err()
	case <-t.done:
		outcome = errors.New("reader transport stopped")
	}
	timing("reader_completion_wait", outcome == nil)
}
