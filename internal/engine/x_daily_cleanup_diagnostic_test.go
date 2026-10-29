package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func TestXDailyCleanupDiagnosticKeepsStartupAndCleanupSeparate(t *testing.T) {
	e := &Engine{}
	e.config.Capture.XMode = config.XCaptureConcurrent
	primary := xDailyStartupFail(errors.New("PRIVATE_STARTUP"), "broker_start", "eligibility")
	e.rememberXDailyFailure(primary, "startup")
	resources := &XDailyCleanupResources{"uncertain", "verified", "verified", "verified", "verified", "verified"}
	secondary := &xDailyCleanupError{detail: XDailyCleanupDiagnostic{Stage: "batch_end", Condition: "unverified", Broker: "uncertain", BatchEnd: "uncertain", CollectorExit: "verified", PinRelease: "verified", Resources: resources}, cause: errors.New("PRIVATE_CLEANUP")}
	e.rememberXDailyCleanup(secondary)
	e.rememberXDailyCleanup(xDailyCleanupFail(errors.New("LATE_PRIVATE_ERROR"), "owner_close", "unverified"))
	s := e.XCaptureStatus()
	if s.FailurePhase != "startup" || s.FailureCode != primary.(*xDailyStartupError).dailyFailureCode() || s.StartupDiagnostic == nil || s.CleanupDiagnostic == nil || s.CleanupDiagnostic.Stage != "batch_end" || s.CleanupDiagnostic.Resources.Bootstrap != "verified" || s.CleanupDiagnostic.Resources.ReaderControls != "uncertain" {
		t.Fatalf("diagnostic channels mixed: %+v", s)
	}
	s.CleanupDiagnostic.Resources.Bootstrap = "mutated"
	if e.XCaptureStatus().CleanupDiagnostic.Resources.Bootstrap != "verified" {
		t.Fatal("status exposed mutable cleanup state")
	}
	raw, err := json.Marshal(e.XCaptureStatus())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{string(raw), fmt.Sprintf("%#v", secondary), secondary.Error()} {
		if strings.Contains(value, "PRIVATE") {
			t.Fatal("private cleanup cause leaked")
		}
	}
}

func TestXDailyCleanupDiagnosticRejectsMissingOrUnknownResources(t *testing.T) {
	all := XDailyCleanupResources{"verified", "not_needed", "verified", "verified", "verified", "verified"}
	if !all.valid() || !all.complete() {
		t.Fatal("valid cleanup could not complete")
	}
	all.Bootstrap = "uncertain"
	if !all.valid() || all.complete() {
		t.Fatal("unverified target cleanup completed")
	}
	all.Bootstrap = "PRIVATE_TARGET"
	if all.valid() || all.complete() {
		t.Fatal("unknown cleanup became verified")
	}
	d := (XDailyCleanupDiagnostic{Stage: "PRIVATE_STAGE", Condition: "PRIVATE_REASON", Resources: &all}).closed()
	raw, _ := json.Marshal(d)
	if d.Stage != "unknown" || d.Resources.Bootstrap != "uncertain" || strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("cleanup projection was not closed")
	}
	if (XDailyCleanupResources{}).complete() {
		t.Fatal("missing resources manufactured cleanup proof")
	}
}

func TestXDailyBootstrapDiagnosticIsClosedAndIndependent(t *testing.T) {
	e := &Engine{}
	e.config.Capture.XMode = config.XCaptureConcurrent
	bootstrap := &XDailyBootstrapDiagnostic{Stage: "absence", Condition: "timeout"}
	cleanup := &xDailyCleanupError{detail: XDailyCleanupDiagnostic{Stage: "batch_end", Condition: "unverified", Bootstrap: bootstrap}, cause: errors.New("PRIVATE_TARGET_COOKIE")}
	e.rememberXDailyCleanup(cleanup)
	status := e.XCaptureStatus()
	if status.CleanupDiagnostic == nil || status.CleanupDiagnostic.Bootstrap == nil || *status.CleanupDiagnostic.Bootstrap != *bootstrap {
		t.Fatal("bootstrap diagnostic was not projected")
	}
	status.CleanupDiagnostic.Bootstrap.Condition = "mutated"
	if e.XCaptureStatus().CleanupDiagnostic.Bootstrap.Condition != "timeout" {
		t.Fatal("bootstrap status exposed mutable state")
	}
	if !strings.Contains(e.XCaptureStatus().CleanupDiagnostic.String(), "bootstrap_stage=absence bootstrap_condition=timeout") {
		t.Fatal("bootstrap diagnostic was not logged separately")
	}
	for _, invalid := range []XDailyBootstrapDiagnostic{{Stage: "PRIVATE_TARGET", Condition: "timeout"}, {Stage: "close", Condition: "PRIVATE_TOKEN"}, {}} {
		if invalid.valid() {
			t.Fatal("unbounded bootstrap diagnostic accepted")
		}
		d := (XDailyCleanupDiagnostic{Bootstrap: &invalid}).closed()
		raw, _ := json.Marshal(d)
		if strings.Contains(string(raw), "PRIVATE") || strings.Contains(d.String(), "PRIVATE") {
			t.Fatal("private bootstrap error leaked")
		}
	}
}
