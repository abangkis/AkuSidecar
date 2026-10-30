package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
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
		rejected := errors.New("existing Instagram diagnostic owner is unavailable")
		if ctx == nil || ctx.Err() != nil {
			return nil, rejected
		}
		snapshot := manager.Snapshot()
		if snapshot.State != captureruntime.Ready || snapshot.Driver != "browser" || snapshot.Generation != generation || snapshot.ActiveLeases != 0 || manager.PID() != pid || !manager.Persistent() {
			return nil, rejected
		}
		lease, err := manager.Acquire()
		if err != nil {
			return nil, rejected
		}
		defer lease.Release()
		if lease.Driver() != "browser" || lease.Generation() != generation || !manager.Accepts(generation) || manager.PID() != pid || manager.Snapshot().ActiveLeases != 1 {
			return nil, rejected
		}
		return action(ctx, nonce)
	}
}
