//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"github.com/abangkis/AkuSidecar/internal/engine"
	"testing"
)

func TestDevelopmentFrontendNativePauseEvidence(t *testing.T) {
	v := engine.DevelopmentPauseEvidence{AdmissionPaused: true, CollectorPID: 17, ManagerPID: 17, Driver: "headless", Generation: 1, ActiveLeases: 1, CaptureSteps: 1, ActiveWork: 1}
	if err := validateDevelopmentFrontendPause("capture_blocked", v, 17); err != nil {
		t.Fatal(err)
	}
	v.SQLitePaused = true
	if validateDevelopmentFrontendPause("capture_blocked", v, 17) == nil {
		t.Fatal("premature marker accepted")
	}
	v.SQLitePaused = false
	if err := validateDevelopmentFrontendPause("reasoning_blocked", v, 17); err != nil {
		t.Fatal(err)
	}
	v.SQLitePaused, v.BeforeLeaseRelease, v.CaptureSteps, v.ActiveWork = true, true, 0, 0
	if err := validateDevelopmentFrontendPause("persisted_before_lease_release", v, 17); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*engine.DevelopmentPauseEvidence){
		func(v *engine.DevelopmentPauseEvidence) { v.SQLiteError = "read failed" },
		func(v *engine.DevelopmentPauseEvidence) { v.SQLitePaused = false },
		func(v *engine.DevelopmentPauseEvidence) { v.AdmissionPaused = false },
		func(v *engine.DevelopmentPauseEvidence) { v.CollectorPID = 18 },
		func(v *engine.DevelopmentPauseEvidence) { v.ManagerPID = 18 },
		func(v *engine.DevelopmentPauseEvidence) { v.Driver = "browser" },
		func(v *engine.DevelopmentPauseEvidence) { v.ActiveLeases = 0 },
		func(v *engine.DevelopmentPauseEvidence) { v.CaptureSteps = 1 },
		func(v *engine.DevelopmentPauseEvidence) { v.ActiveWork = 1 },
		func(v *engine.DevelopmentPauseEvidence) { v.BeforeLeaseRelease = false },
	} {
		rejected := v
		mutate(&rejected)
		if validateDevelopmentFrontendPause("persisted_before_lease_release", rejected, 17) == nil {
			t.Fatalf("invalid evidence accepted: %+v", rejected)
		}
	}
}
