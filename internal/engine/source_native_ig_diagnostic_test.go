//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

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
