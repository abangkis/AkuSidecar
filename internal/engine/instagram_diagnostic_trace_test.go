package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

func igTraceEnvelope(t *testing.T, raw []byte) igdiagnostic.Envelope {
	t.Helper()
	if _, ok := igdiagnostic.ValidateJSON(raw); !ok {
		t.Fatalf("invalid safe snapshot: %s", raw)
	}
	var result igdiagnostic.Envelope
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInstagramDiagnosticTraceKeepsPrimaryCleanupAndPrivateCause(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	e := instagramDiagnosticEngine(t)
	holdInstagramDiagnostic(t, e, `["x","instagram"]`)
	private := errors.New("PRIVATE_ACCOUNT_TOKEN")
	_, err := e.RunInstagramBootstrapCorroborationDiagnostic(context.Background(), func(ctx context.Context) (json.RawMessage, error) {
		igdiagnostic.Note(ctx, "body_read")
		igdiagnostic.Fail(ctx, "body_read", "body_bounds")
		igdiagnostic.SetCleanup(ctx, "uncertain", "stream_close_failed")
		return nil, private
	})
	var rejected *InstagramReaderDiagnosticError
	if !errors.As(err, &rejected) || !errors.Is(err, private) || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatal("operational cause or safe formatting changed")
	}
	snapshot := igTraceEnvelope(t, rejected.DiagnosticJSON())
	if snapshot.Cause == nil || snapshot.Cause.Stage != "body_read" || snapshot.Cause.Condition != "body_bounds" || snapshot.Cleanup.State != "uncertain" || snapshot.Cleanup.Condition != "stream_close_failed" {
		t.Fatal("first cause or cleanup lost")
	}
	if snapshot.Evidence["cookieMatched"] != "not_observed" {
		t.Fatal("unexecuted cookie comparison became negative evidence")
	}
	if strings.Contains(string(e.InstagramDiagnosticSnapshot()), "PRIVATE") {
		t.Fatal("private error retained in public snapshot")
	}
	retained := igTraceEnvelope(t, e.InstagramDiagnosticSnapshot())
	if retained.Attempt != snapshot.Attempt {
		t.Fatal("attempt correlation lost")
	}
	if _, err = e.RunInstagramBootstrapCorroborationDiagnostic(context.Background(), func(context.Context) (json.RawMessage, error) { t.Fatal("one-shot reset"); return nil, nil }); err == nil {
		t.Fatal("failed attempt was replayed")
	}
	if igTraceEnvelope(t, e.InstagramDiagnosticSnapshot()).Attempt != retained.Attempt {
		t.Fatal("rejected replay overwrote admitted attempt")
	}
}

type privateIGError string

func (e privateIGError) Error() string { return string(e) }

func TestInstagramDiagnosticErrorFormattingCannotExposePrivateCause(t *testing.T) {
	err := &InstagramReaderDiagnosticError{Reason: "diagnostic_rejected", cause: privateIGError("PRIVATE_TOKEN")}
	if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), "PRIVATE") {
		t.Fatal("private cause exposed by diagnostic formatting")
	}
	if !errors.Is(err, privateIGError("PRIVATE_TOKEN")) {
		t.Fatal("private causal chain lost")
	}
}

func TestInstagramDiagnosticTraceNamesEarlyAdmissionWithoutDispatch(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	for _, mode := range []string{"operation_busy", "maintenance_required", "runtime_missing", "already_used", "context_canceled"} {
		t.Run(mode, func(t *testing.T) {
			e := instagramDiagnosticEngine(t)
			if mode != "maintenance_required" {
				holdInstagramDiagnostic(t, e, `["x","instagram"]`)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "operation_busy" {
				e.operation.Lock()
				defer e.operation.Unlock()
			}
			if mode == "runtime_missing" {
				e.collectionRuntime = nil
			}
			if mode == "already_used" {
				e.instagramReaderDiagnosticUsed = true
			}
			if mode == "context_canceled" {
				cancel()
			}
			_, err := e.RunInstagramReaderDiagnostic(ctx, func(context.Context) (json.RawMessage, error) {
				t.Fatal("rejected admission dispatched")
				return nil, nil
			})
			var rejected *InstagramReaderDiagnosticError
			if !errors.As(err, &rejected) {
				t.Fatal("missing admission rejection")
			}
			s := igTraceEnvelope(t, rejected.DiagnosticJSON())
			if s.Cause == nil || s.Cause.Stage != "admission" || s.Cause.Condition != mode || s.Disposition != "not_started" || s.Cleanup.State != "not_started" {
				t.Fatalf("wrong admission observation: %+v", s)
			}
		})
	}
}

func TestInstagramDiagnosticSnapshotReconcilesWithoutAnotherDispatch(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	e := instagramDiagnosticEngine(t)
	holdInstagramDiagnostic(t, e, `["x","instagram"]`)
	var observed context.Context
	_, err := e.RunInstagramReaderDiagnostic(context.Background(), func(ctx context.Context) (json.RawMessage, error) {
		observed = ctx
		igdiagnostic.SetCleanup(ctx, "uncertain", "inventory_failed")
		return json.RawMessage(diagnosticSafeResult), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if igTraceEnvelope(t, e.InstagramDiagnosticSnapshot()).Cleanup.State != "uncertain" {
		t.Fatal("missing cleanup uncertainty")
	}
	// The retained observation can settle; it cannot itself release any resource.
	igdiagnostic.SetCleanup(observed, "verified", "none")
	if igTraceEnvelope(t, e.InstagramDiagnosticSnapshot()).Cleanup.State != "verified" || !e.instagramReaderDiagnosticUsed {
		t.Fatal("safe reconciliation changed one-shot state")
	}
}
