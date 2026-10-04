package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func (s *Server) runDirectNativeReader(ctx context.Context, t *splitCaptureTransport, entry *pendingSplitAction, prepare func(context.Context, string, string, string) (readerbroker.Target, func(context.Context) error, error)) {
	var outcome error
	defer func() {
		result := splitActionResult{OK: outcome == nil}
		if outcome != nil {
			result.Message = "Native reader could not complete; close any opened reader window and retry."
		} else {
			result.Result, _ = json.Marshal(map[string]string{"source": entry.action.Source, "state": "native_post_opened", "url": entry.action.URL})
		}
		t.mu.Lock()
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
	s.auditSplitAction(ctx, entry.action, "claimed", "accepted")
	s.auditSplitAction(ctx, entry.action, "reader_prepare", "pending")
	if prepare == nil {
		outcome = errors.New("reader unavailable")
		return
	}
	marker := "/split-reader-intent?id=" + entry.action.ID
	target, verify, err := prepare(ctx, entry.action.ID, entry.action.URL, marker)
	if err != nil {
		outcome = err
		s.auditSplitAction(ctx, entry.action, "reader_prepare", "rejected")
		return
	}
	t.mu.Lock()
	entry.brokerTarget, entry.readerForeground = target, verify
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
}
