package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

// Capture the running owner's identity at composition, without a launcher,
// coordinator borrow, profile lookup, session transfer or foreground action.
func instagramReaderDiagnosticAction(protocol appshell.CaptureProtocol, manager *captureruntime.Manager, pid int) func(context.Context, string) (json.RawMessage, error) {
	diagnostic, ok := protocol.(appshell.MainBrowserInstagramDiagnosticProtocol)
	if !ok {
		return nil
	}
	return bindInstagramDiagnosticAction(diagnostic.DiagnoseInstagramReader, manager, pid)
}

func instagramBootstrapDiagnosticAction(protocol appshell.CaptureProtocol, manager *captureruntime.Manager, pid int) func(context.Context, string) (json.RawMessage, error) {
	diagnostic, ok := protocol.(appshell.MainBrowserInstagramBootstrapDiagnosticProtocol)
	if !ok {
		return nil
	}
	return bindInstagramDiagnosticAction(diagnostic.DiagnoseInstagramBootstrap, manager, pid)
}

func instagramBootstrapCorroborationDiagnosticAction(protocol appshell.CaptureProtocol, manager *captureruntime.Manager, pid int) func(context.Context, string) (json.RawMessage, error) {
	diagnostic, ok := protocol.(appshell.MainBrowserInstagramBootstrapCorroborationDiagnosticProtocol)
	if !ok {
		return nil
	}
	return bindInstagramDiagnosticAction(diagnostic.DiagnoseInstagramBootstrapCorroboration, manager, pid)
}

func bindInstagramDiagnosticAction(action func(context.Context, string) (json.RawMessage, error), manager *captureruntime.Manager, pid int) func(context.Context, string) (json.RawMessage, error) {
	if manager == nil || pid < 1 || manager.PID() != pid || !manager.Persistent() {
		return nil
	}
	generation := manager.Snapshot().Generation
	return func(ctx context.Context, nonce string) (json.RawMessage, error) {
		reject := func(cause error, condition string) (json.RawMessage, error) {
			igdiagnostic.Fail(ctx, "owner", condition)
			if cause == nil {
				cause = errors.New("existing Instagram diagnostic owner is unavailable")
			}
			return nil, igdiagnostic.Wrap(ctx, cause)
		}
		igdiagnostic.Note(ctx, "owner")
		if ctx == nil || ctx.Err() != nil {
			return reject(nil, "context_canceled")
		}
		snapshot := manager.Snapshot()
		if snapshot.State != captureruntime.Ready {
			return reject(nil, "owner_not_ready")
		}
		if snapshot.Driver != "browser" {
			return reject(nil, "owner_driver")
		}
		if snapshot.Generation != generation {
			return reject(nil, "owner_generation")
		}
		if snapshot.ActiveLeases != 0 {
			return reject(nil, "lease_present")
		}
		if manager.PID() != pid {
			return reject(nil, "owner_process")
		}
		if !manager.Persistent() {
			return reject(nil, "owner_persistent")
		}
		lease, err := manager.Acquire()
		if err != nil {
			return reject(err, "lease_acquire")
		}
		defer lease.Release()
		if lease.Driver() != "browser" {
			return reject(nil, "lease_driver")
		}
		if lease.Generation() != generation {
			return reject(nil, "lease_generation")
		}
		if !manager.Accepts(generation) {
			return reject(nil, "lease_invalid")
		}
		if manager.PID() != pid {
			return reject(nil, "owner_process")
		}
		if manager.Snapshot().ActiveLeases != 1 {
			return reject(nil, "lease_count")
		}
		return action(ctx, nonce)
	}
}
