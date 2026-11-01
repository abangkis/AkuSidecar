//go:build auth_session_dev

package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

type syntheticDailyCodeError string

func (e syntheticDailyCodeError) Error() string            { return "private synthetic RPC exception" }
func (e syntheticDailyCodeError) dailyFailureCode() string { return string(e) }

func TestXDailyNativeStartupPreservesWrappedRPCCode(t *testing.T) {
	code := "manual_initial_reader_settings_source_unavailable_primary"
	private := syntheticDailyCodeError(code)
	err := xDailyNativeStartupFail(fmt.Errorf("private context: %w", private), "seed_rpc", "rpc", nil, nativePilotDiagnostic{})
	detail, ok := nativePilotInitialCode(code)
	if !ok || xDailyStartupDetail(err).Origin != detail.String() || !errors.Is(err, private) {
		t.Fatal("wrapped RPC lost its closed domain cause")
	}
}

func TestXDailyNativeStartupPreservesCDPCause(t *testing.T) {
	for _, cause := range []string{"cdp_command_rejected", "cdp_request_rejected", "cdp_reply_invalid", "cdp_transport_closed", "cdp_timeout"} {
		t.Run(cause, func(t *testing.T) {
			detail := nativePilotInitialDetail{
				role: "pair", phase: "joint", reason: "unavailable", peer: true,
				trace: &nativePilotInitialTrace{firstCause: cause},
			}
			failure := xDailyNativeStartupFail(&nativePilotInitialError{detail}, "broker_start", "eligibility", nil, nativePilotDiagnostic{Reason: "prepare-failed"})
			projected := xDailyStartupDetail(failure)
			if projected == nil || projected.BrokerReason != "prepare-failed" || projected.Origin != detail.String() ||
				!strings.Contains(projected.Origin, "first="+cause) || !strings.Contains(projected.Origin, "domain=transport") {
				t.Fatal("X startup status lost the closed CDP cause")
			}
		})
	}
}

func TestXDailyNativeStartupRetainsOriginalDomainReason(t *testing.T) {
	original := &nativePilotInitialError{detail: nativePilotInitialDetail{role: "reader", phase: "inspect_before", reason: "rejected"}}
	broker := &nativePilotBroker{
		initial:         nativePilotInitialDetail{role: "collector", phase: "cleanup", reason: "cleanup_unverified"},
		firstStopReason: "source-unavailable", reason: "cleanup-uncertain",
	}
	err := xDailyNativeStartupFail(original, "broker_start", "eligibility", broker, nativePilotDiagnostic{Reason: "cleanup-uncertain"})
	d := xDailyStartupDetail(err)
	if d.Origin != original.detail.String() || d.BrokerReason != "source-unavailable" || !errors.Is(err, original) {
		t.Fatal("cleanup replaced original native failure")
	}
	if xDailyNativeStartupFail(err, "runtime", "runtime", nil, nativePilotDiagnostic{}) != err {
		t.Fatal("later wrapper replaced first failure")
	}
	broker.firstStopReason = "auth_token=secret"
	broker.initial = nativePilotInitialDetail{role: "account-secret", phase: "path-secret", reason: "cookie-secret"}
	raw, marshalErr := json.Marshal(xDailyStartupDetail(xDailyNativeStartupFail(errors.New("raw-secret"), "broker_start", "eligibility", broker, nativePilotDiagnostic{})))
	if marshalErr != nil || strings.Contains(string(raw), "secret") {
		t.Fatal("untrusted native metadata escaped closed projection")
	}
}

func TestXDailyNativeStartupSeedOriginIsClosed(t *testing.T) {
	for _, sample := range []struct {
		code, want string
	}{
		{"manual_seed_seed_write_unavailable_unknown", "seed_phase=seed_write seed_reason=unavailable seed_write=unknown"},
		{"manual_seed_enrollment_project_bootstrap_missing_not_attempted", "seed_phase=enrollment_project seed_reason=bootstrap_missing seed_write=not_attempted"},
		{"manual_seed_owner_after_ownership_changed_acknowledged", "seed_phase=owner_after seed_reason=ownership_changed seed_write=acknowledged"},
		{"manual_seed_private_secret", ""},
		{"manual_seed_seed_write_PRIVATE_unknown", ""},
		{"manual_seed_v2_09_3_0_v6_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_4", ""},
		{"manual_seed_v2_9_3_0_v6_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_4_extra", ""},
		{strings.Repeat("private", nativePilotSeedCodeMax), ""},
	} {
		private := syntheticDailyCodeError(sample.code)
		failure := xDailyNativeStartupFail(fmt.Errorf("PRIVATE_CONTEXT: %w", private), "seed_rpc", "rpc", nil, nativePilotDiagnostic{})
		detail := xDailyStartupDetail(failure)
		if detail == nil || detail.Origin != sample.want || failure.Error() != "daily_startup_seed_rpc_rpc_unavailable" ||
			!errors.Is(failure, private) || dailyTransientClass(failure) != "" {
			t.Fatal("seed projection changed operational failure or admitted unsafe metadata")
		}
		raw, err := json.Marshal(detail)
		if err != nil || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "PRIVATE") {
			t.Fatal("untrusted seed data escaped public status")
		}
	}
	spoof := errors.New("manual_seed_seed_write_unavailable_unknown")
	if xDailyStartupDetail(xDailyNativeStartupFail(spoof, "seed_rpc", "rpc", nil, nativePilotDiagnostic{})).Origin != "" {
		t.Fatal("exception text became trusted seed metadata")
	}
}

func TestXDailyNativeStartupSeedStatusPreservesFirstCauseAndCleanup(t *testing.T) {
	private := syntheticDailyCodeError("manual_seed_seed_write_unavailable_unknown")
	broker := &nativePilotBroker{initial: nativePilotInitialDetail{role: "collector", phase: "cleanup", reason: "cleanup_unverified"}}
	first := xDailyNativeStartupFail(private, "seed_rpc", "rpc", broker, nativePilotDiagnostic{})
	e := &Engine{}
	e.config.Capture.XMode = config.XCaptureConcurrent
	e.rememberXDailyFailure(first, "startup")
	e.rememberXDailyFailure(errors.New("PRIVATE_LATER_FAILURE"), "cleanup")
	e.rememberXDailyCleanup(xDailyCleanupFail(errors.New("PRIVATE_CLEANUP"), "batch_end", "unverified"))
	status := e.XCaptureStatus()
	if status.StartupDiagnostic == nil || status.StartupDiagnostic.Origin != "seed_phase=seed_write seed_reason=unavailable seed_write=unknown" ||
		status.FailureCode != "daily_startup_seed_rpc_rpc_unavailable" || status.FailurePhase != "startup" ||
		status.CleanupDiagnostic == nil || status.CleanupDiagnostic.Stage != "batch_end" || dailyTransientClass(first) != "" {
		t.Fatal("seed status lost its first cause or mixed cleanup and retry authority")
	}
	if xDailyNativeStartupFail(fmt.Errorf("PRIVATE_CLEANUP: %w", first), "runtime", "runtime", nil, nativePilotDiagnostic{}) != first {
		t.Fatal("later wrapper replaced seed failure")
	}
	raw, err := json.Marshal(status)
	if err != nil || strings.Contains(string(raw), "PRIVATE") {
		t.Fatal("private cause escaped seed status")
	}
}
