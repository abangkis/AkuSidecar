package httpapi

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// RequireHostOnlyRetirement is set before bootstrap by a browser owner using
// hidden CDP targets. It survives transport rotation; negotiation does not.
func (s *Server) RequireHostOnlyRetirement() error {
	t := s.splitCapture
	if t == nil {
		return errors.New("capture host transport unavailable")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return errors.New("capture host transport stopped")
	}
	for _, action := range t.actions {
		if action.action.Type == "close_capture_host" {
			return errors.New("capture host retirement is already admitted")
		}
	}
	t.hostOnlyRequired = true
	return nil
}

// CloseSplitCaptureHost retires the authenticated host alone when required,
// preserving all ordinary targets. Legacy owners may also retire Bridge-owned
// background tabs. The caller must still verify natural process exit
// and complete owned-tree release. An acknowledgement never permits killing
// another window or reusing a profile while the old owner remains alive.
func (s *Server) CloseSplitCaptureHost(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t := s.splitCapture
	if t == nil {
		return errors.New("capture host transport unavailable")
	}
	t.mu.Lock()
	if t.closed || !t.hostCloseSupported || t.runtime == nil {
		t.mu.Unlock()
		return errors.New("capture host retirement is not negotiated")
	}
	if t.hostOnlyRequired && !t.hostOnlySupported {
		t.mu.Unlock()
		return errors.New("Bridge host-only retirement is not negotiated; update or reload AkuBridge before switching collection mode")
	}
	snapshot := t.runtime.Snapshot()
	if snapshot.State != captureruntime.Replacing || snapshot.Driver != "browser" || snapshot.ActiveLeases != 0 {
		t.mu.Unlock()
		return captureruntime.ErrBusy
	}
	var entry *pendingSplitAction
	for _, candidate := range t.actions {
		if candidate.action.Type == "close_capture_host" {
			entry = candidate // A claimed retirement is never replayed on timeout.
		} else if !candidate.completed {
			t.mu.Unlock()
			return captureruntime.ErrBusy
		}
	}
	if entry != nil && entry.completed {
		result := entry.completionResult
		t.mu.Unlock()
		if result != nil && result.OK {
			return nil
		}
		return errors.New("capture host retirement was rejected or unverified")
	}
	if entry == nil {
		entry = &pendingSplitAction{action: splitCaptureAction{ID: domain.NewID("handoff"), Type: "close_capture_host", HostOnly: t.hostOnlyRequired}, result: make(chan splitActionResult, 1)}
		t.actions = append(t.actions, entry)
	}
	t.mu.Unlock()
	t.notifyCapture()
	select {
	case <-ctx.Done():
		return ctx.Err() // Retain the action and owner for a late acknowledgement.
	case <-t.done:
		return errors.New("capture host transport stopped")
	case result := <-entry.result:
		if !result.OK {
			return errors.New("capture host retirement rejected: " + result.Message)
		}
		return nil
	}
}
