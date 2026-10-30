package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
	"time"
)

// The observer is passive: it never retries, cancels cleanup or releases an owner.
func instagramTraceRequest(ctx context.Context, op, nonce string, call func(context.Context, string, map[string]any, func([]byte)) (json.RawMessage, error), validate func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	operation := "reader"
	if op == "main_instagram_bootstrap_diagnostic" {
		operation = "bootstrap"
	}
	if op == "main_instagram_bootstrap_corroboration" {
		operation = "corroboration"
	}
	ctx, a := igdiagnostic.Ensure(ctx, operation)
	if !validInstagramDiagnosticNonce(nonce) {
		igdiagnostic.Fail(ctx, "rpc", "nonce_invalid")
		return nil, igdiagnostic.Wrap(ctx, &MainBrowserError{Code: "instagram_diagnostic_invalid_request"})
	}
	budget := int64(30000)
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline).Milliseconds() < budget {
		budget = time.Until(deadline).Milliseconds()
	}
	if budget < 1 {
		igdiagnostic.Fail(ctx, "rpc", "deadline")
		return nil, igdiagnostic.Wrap(ctx, context.DeadlineExceeded)
	}
	igdiagnostic.Note(ctx, "rpc")
	igdiagnostic.SetDisposition(ctx, "start_unknown")
	raw, err := call(ctx, op, map[string]any{"nonce": nonce, "budgetMS": budget}, func(raw []byte) {
		if a != nil {
			a.Observe(raw)
		}
	})
	if err != nil {
		var transport *MainBrowserError
		if errors.As(err, &transport) && len(transport.Diagnostic) > 0 && a != nil {
			a.Observe(transport.Diagnostic)
		}
		if transport != nil {
			// Arbitrary peer codes/bytes cannot survive through errors.As.
			err = &MainBrowserError{Code: "instagram_diagnostic_rejected"}
		}
		condition := "terminal_missing"
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			condition = "deadline"
		case errors.Is(err, context.Canceled):
			condition = "canceled"
		case errors.Is(err, ownedrpc.ErrClosed):
			condition = "transport_closed"
		case errors.Is(err, ownedrpc.ErrWrite):
			condition = "write_failed"
		}
		igdiagnostic.Fail(ctx, "rpc", condition)
		igdiagnostic.SetDelivery(ctx, "unavailable", condition)
		if a != nil && a.Disposition() == "completed" {
			igdiagnostic.SetDisposition(ctx, "result_lost")
		}
		return nil, igdiagnostic.Wrap(ctx, err)
	}
	igdiagnostic.SetDelivery(ctx, "observed", "none")
	raw, err = validate(raw)
	if err != nil {
		igdiagnostic.Fail(ctx, "projection", "projection_invalid")
		igdiagnostic.SetDisposition(ctx, "result_lost")
		return nil, igdiagnostic.Wrap(ctx, err)
	}
	igdiagnostic.SetDisposition(ctx, "completed")
	// Legacy false fields can also mean unexecuted. Only true is evidence here;
	// exact failure categories establish the corresponding observed false value.
	var result map[string]json.RawMessage
	_ = json.Unmarshal(raw, &result)
	if a != nil {
		for _, key := range []string{"bodyBound", "documentBound", "serverFresh", "scanComplete", "idMatched", "cookieMatched"} {
			var passed bool
			if json.Unmarshal(result[key], &passed) == nil && passed {
				a.Evidence(key, "passed")
			}
		}
		var reason string
		_ = json.Unmarshal(result["reason"], &reason)
		if reason == "id_mismatch" {
			a.Evidence("idMatched", "failed")
		}
		if reason == "cookie_mismatch" {
			a.Evidence("cookieMatched", "failed")
		}
		if reason == "document_changed" {
			a.Evidence("documentBound", "invalidated")
		}
	}
	return raw, nil
}
