package appshell

import (
	"context"
	"errors"
)

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
	guard, ok := w.containment.(interface{ ReplacementReadiness(context.Context) error })
	if !ok {
		return errors.New("capture reader lifetime is unavailable")
	}
	return guard.ReplacementReadiness(ctx)
}
