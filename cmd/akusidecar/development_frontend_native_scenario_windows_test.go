//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func TestDevelopmentFrontendLiveReaderOriginalQueuedContinuation(t *testing.T) {
	batch := domain.Session{ID: "original-session", Runs: []domain.Run{
		{ID: "first", SessionID: "original-session", Ordinal: 0, Status: "waiting_for_bridge"},
		{ID: "second", SessionID: "original-session", Ordinal: 1, Status: "queued"},
	}}
	run, err := developmentFrontendQueuedContinuation(batch)
	if err != nil || run.ID != "second" || run.Ordinal != 1 {
		t.Fatalf("original zero-based continuation rejected: run=%+v err=%v", run, err)
	}
	for _, mutate := range []func(*domain.Session){
		func(v *domain.Session) { v.Runs[1].Ordinal = 2 },
		func(v *domain.Session) { v.Runs[1].SessionID = "replacement" },
		func(v *domain.Session) { v.Runs[1].Status = "completed" },
		func(v *domain.Session) { v.Runs = v.Runs[:1] },
	} {
		candidate := batch
		candidate.Runs = append([]domain.Run(nil), batch.Runs...)
		mutate(&candidate)
		if _, err := developmentFrontendQueuedContinuation(candidate); err == nil {
			t.Fatal("invalid original queued continuation accepted")
		}
	}
}

func TestDevelopmentFrontendLiveReaderEvidenceContract(t *testing.T) {
	valid := engine.DevelopmentReaderWindowEvidence{
		InventoryKnown:    true,
		WindowCount:       1,
		BootstrapFlags:    []bool{true},
		CoordinatorPaused: true,
		SQLiteKnown:       true,
		Pause: engine.DevelopmentPauseEvidence{
			AdmissionPaused: true,
			SQLitePaused:    true,
			Driver:          "browser",
			CollectorPID:    2314,
			ManagerPID:      2314,
			ActiveLeases:    0,
			Generation:      2,
		},
	}
	if failures := developmentFrontendLiveReaderFailures(valid, 2314); len(failures) != 0 {
		t.Fatalf("valid live-reader evidence rejected: %v", failures)
	}
	cases := []struct {
		name   string
		mutate func(*engine.DevelopmentReaderWindowEvidence)
	}{
		{"inventory_unknown", func(v *engine.DevelopmentReaderWindowEvidence) { v.InventoryKnown = false }},
		{"no_window", func(v *engine.DevelopmentReaderWindowEvidence) { v.WindowCount = 0; v.BootstrapFlags = nil }},
		{"no_bootstrap", func(v *engine.DevelopmentReaderWindowEvidence) { v.BootstrapFlags = []bool{false} }},
		{"coordinator_unpaused", func(v *engine.DevelopmentReaderWindowEvidence) { v.CoordinatorPaused = false }},
		{"sqlite_unpaused", func(v *engine.DevelopmentReaderWindowEvidence) { v.Pause.SQLitePaused = false }},
		{"headless_owner", func(v *engine.DevelopmentReaderWindowEvidence) { v.Pause.Driver = "headless" }},
		{"foreign_manager", func(v *engine.DevelopmentReaderWindowEvidence) { v.Pause.ManagerPID++ }},
		{"generation_unknown", func(v *engine.DevelopmentReaderWindowEvidence) { v.Pause.Generation = 0 }},
		{"capture_not_drained", func(v *engine.DevelopmentReaderWindowEvidence) { v.Pause.CaptureSteps = 1 }},
		{"reasoning_not_drained", func(v *engine.DevelopmentReaderWindowEvidence) { v.Pause.ActiveWork = 1 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			value.BootstrapFlags = append([]bool(nil), valid.BootstrapFlags...)
			test.mutate(&value)
			if failures := developmentFrontendLiveReaderFailures(value, 2314); len(failures) == 0 {
				t.Fatal("invalid live-reader evidence accepted")
			}
		})
	}
}
