package appshell

import (
	"context"
	"errors"
)

// ErrCaptureHandoffUnsafe means the headed owner has no authenticated transport
// that can retire only its static host tab and background capture tabs.
var ErrCaptureHandoffUnsafe = errors.New("headed capture handoff unavailable: scoped host retirement is unsupported")

// ErrCaptureHandoffPending retains the headed owner while an independent
// interactive window or an unverified process tree prevents profile reuse.
var ErrCaptureHandoffPending = errors.New("capture host retirement is pending natural process-tree exit")

// SetCaptureHandoff binds the authenticated transport that closes only known
// background capture tabs and the static host tab. It must never close source,
// login or reader windows, nor promise process/profile release from an ACK.
func (w *Window) SetCaptureHandoff(closeHost func(context.Context) error) error {
	if w == nil || closeHost == nil {
		return ErrCaptureHandoffUnsafe
	}
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	if !w.captureHost || w.captureHandoff != nil || w.retiring {
		return ErrCaptureHandoffUnsafe
	}
	w.captureHandoff = closeHost
	return nil
}

func (w *Window) Retiring() bool {
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	return w.retiring
}

// ReplacementReadiness checks the full native reader lifetime separately from
// the short-lived foreground exemption. A manager must not infer reader closure
// from an action result or the five-second activation capability expiry.
func (w *Window) ReplacementReadiness(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w == nil {
		return errors.New("capture owner unavailable")
	}
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	select {
	case <-w.closed:
		return w.cleanupErr
	default:
	}
	if w.captureHandoff == nil {
		return ErrCaptureHandoffUnsafe
	}
	if w.retiring {
		// The host may have disappeared while a late independent popup keeps
		// Chrome alive. Further cleanup still only waits for natural exit.
		return nil
	}
	guard, ok := w.containment.(interface{ ReplacementReadiness(context.Context) error })
	if !ok {
		return errors.New("capture reader lifetime is unavailable")
	}
	if err := guard.ReplacementReadiness(ctx); err != nil {
		return err
	}
	return nil
}

// WaitForNaturalClose waits for the owned process tree's normal exit and
// verified cleanup. It never requests termination.
func (w *Window) WaitForNaturalClose(ctx context.Context) error {
	if w == nil || w.closed == nil {
		return errors.New("capture owner close state unavailable")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.closed:
		w.ownershipMu.Lock()
		defer w.ownershipMu.Unlock()
		return w.cleanupErr
	}
}
