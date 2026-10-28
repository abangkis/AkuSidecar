//go:build auth_session_dev

package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
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
