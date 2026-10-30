//go:build auth_session_dev

package engine

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

// Diagnostic metadata cannot change the independently decoded operational cause
// or its retry classification. This adapter is deliberately IG-only.
func sourceNativeIGWireFailure(ctx context.Context, cause error) error {
	operational := sourceNativeWireFailure(cause)
	var wire *appshell.MainBrowserError
	if !errors.As(cause, &wire) || len(wire.Diagnostic) == 0 {
		return operational
	}
	ctx, attempt := igdiagnostic.Ensure(ctx, "verifier")
	if attempt == nil {
		return operational
	}
	igdiagnostic.Note(ctx, "rpc")
	if !attempt.Observe(wire.Diagnostic) {
		igdiagnostic.MarkIncomplete(ctx)
		igdiagnostic.Fail(ctx, "rpc", "invalid_sideband")
	}
	return igdiagnostic.Wrap(ctx, operational)
}
