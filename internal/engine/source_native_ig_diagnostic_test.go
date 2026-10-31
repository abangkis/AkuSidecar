//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

type syntheticIGWorkerError struct {
	cause error
	raw   json.RawMessage
}

func (e *syntheticIGWorkerError) Error() string                { return "synthetic worker failure" }
func (e *syntheticIGWorkerError) Unwrap() error                { return e.cause }
func (e *syntheticIGWorkerError) WorkerDiagnosticJSON() []byte { return append([]byte(nil), e.raw...) }

func TestInstagramWorkerDiagnosticIsIndependentAndSourceBound(t *testing.T) {
	const safe = `{"schema":"aku.main-transport-worker.v1","reason":"command_timeout","domain":"cdp","method":"IO.read","role":"ig_reader","stage":"command","reported":true,"controlRead":"eof","exited":true,"exitCode":0,"intentional":false,"cleanup":"verified","complete":true}`
	carrier := &syntheticIGWorkerError{cause: &appshell.MainBrowserError{Code: "native_source_wire_unavailable"}, raw: json.RawMessage(safe)}
	err := sourceNativeIGWireFailure(context.Background(), fmt.Errorf("wrapped: %w", carrier))
	e, _ := testEngine(t)
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	e.config.Capture.XMode = config.XCaptureConcurrent
	d := e.dailyController(domain.SourceInstagram)
	d.rememberFailure(err, "startup")
	status := d.status()
	if status.FailureCode != "native_source_wire_unavailable" || !appshell.ValidateMainBrowserWorkerDiagnostic(status.WorkerDiagnostic) || len(status.IGDiagnostic) != 0 {
		t.Fatal("worker trace changed operation or IG trace")
	}
	x := e.dailyController(domain.SourceX)
	x.rememberFailure(err, "startup")
	if len(x.status().WorkerDiagnostic) != 0 {
		t.Fatal("IG diagnostic changed X status")
	}
	carrier.raw = json.RawMessage(`{"token":"PRIVATE"}`)
	if len(d.status().WorkerDiagnostic) != 0 {
		t.Fatal("unsafe worker trace escaped")
	}
}

func TestInstagramVerifierDiagnosticRetainedWithoutChangingOperationalFailure(t *testing.T) {
	code := "native_source_evidence_reader_identity_http_status_400"
	w := igdiagnostic.Wire{Schema: igdiagnostic.Schema, Stage: "verifier", Completed: "none", Disposition: "started", Sequence: 1, Cause: &igdiagnostic.Cause{Stage: "verifier", Condition: "http_400"}, Cleanup: igdiagnostic.Settlement{State: "not_started", Condition: "none"}, Complete: true}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	failure := sourceNativeIGWireFailure(context.Background(), &appshell.MainBrowserError{Code: code, Diagnostic: raw})
	e, _ := testEngine(t)
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	d := e.dailyController(domain.SourceInstagram)
	d.rememberFailure(failure, "startup")
	status := d.status()
	if status.FailureCode != code || dailyTransientClass(failure) != "" {
		t.Fatal("telemetry changed operational code or retry permission")
	}
	s := igTraceEnvelope(t, status.IGDiagnostic)
	if s.Operation != "verifier" || s.Cause == nil || s.Cause.Condition != "http_400" {
		t.Fatal("production verifier sideband lost")
	}
	if len(e.XCaptureStatus().IGDiagnostic) != 0 {
		t.Fatal("IG diagnostic migrated X output")
	}
	status.IGDiagnostic[0] = '!'
	if !json.Valid(d.status().IGDiagnostic) {
		t.Fatal("caller mutated retained diagnostic")
	}
}

func TestInstagramVerifierInvalidMetadataCannotReplaceCauseOrLeak(t *testing.T) {
	code := "native_source_evidence_reader_identity_body_json"
	err := sourceNativeIGWireFailure(context.Background(), &appshell.MainBrowserError{Code: code, Diagnostic: json.RawMessage(`{"token":"PRIVATE_TOKEN"}`)})
	e, _ := testEngine(t)
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	d := e.dailyController(domain.SourceInstagram)
	d.rememberFailure(err, "startup")
	s := d.status()
	if s.FailureCode != code || dailyTransientClass(err) != "" || strings.Contains(string(s.IGDiagnostic), "PRIVATE") {
		t.Fatal("invalid metadata changed cause, retry or privacy")
	}
	trace := igTraceEnvelope(t, s.IGDiagnostic)
	if trace.Complete || trace.Cause == nil || trace.Cause.Condition != "invalid_sideband" {
		t.Fatal("incomplete diagnostic not identified")
	}
}
