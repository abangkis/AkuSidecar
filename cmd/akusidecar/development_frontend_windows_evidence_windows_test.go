//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"github.com/abangkis/AkuSidecar/internal/engine"
	"reflect"
	"testing"
)

func TestDevelopmentFrontendNativeWindowPredicatesReportExactFailure(t *testing.T) {
	before := engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 2, BootstrapFlags: []bool{true, false}}
	if failed := developmentFrontendWindowFailures("before_bootstrap_close", before); len(failed) != 0 {
		t.Fatal(failed)
	}
	after := engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 1, BootstrapFlags: []bool{false}, CoordinatorPaused: true}
	if failed := developmentFrontendWindowFailures("after_bootstrap_close", after); len(failed) != 0 {
		t.Fatal(failed)
	}
	for _, test := range []struct {
		name, phase string
		value       engine.DevelopmentReaderWindowEvidence
		want        []string
	}{
		{"unknown_inventory", "after_bootstrap_close", engine.DevelopmentReaderWindowEvidence{WindowCount: -1, InventoryError: "inventory failed"}, []string{"inventory_unknown"}},
		{"before_count", "before_bootstrap_close", engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 1, BootstrapFlags: []bool{true}}, []string{"expected_two_windows"}},
		{"missing_bootstrap", "before_bootstrap_close", engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 2, BootstrapFlags: []bool{false, false}}, []string{"bootstrap_missing"}},
		{"after_count", "after_bootstrap_close", engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 2, BootstrapFlags: []bool{false, false}, CoordinatorPaused: true}, []string{"expected_one_remaining_window"}},
		{"after_bootstrap", "after_bootstrap_close", engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 1, BootstrapFlags: []bool{true}, CoordinatorPaused: true}, []string{"remaining_window_is_bootstrap"}},
		{"after_pause", "after_bootstrap_close", engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 1, BootstrapFlags: []bool{false}, Pause: engine.DevelopmentPauseEvidence{SQLitePaused: true}}, []string{"coordinator_not_paused"}},
		{"all_known_failures", "after_bootstrap_close", engine.DevelopmentReaderWindowEvidence{InventoryKnown: true, WindowCount: 2, BootstrapFlags: []bool{true, false}}, []string{"expected_one_remaining_window", "remaining_window_is_bootstrap", "coordinator_not_paused"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := developmentFrontendWindowFailures(test.phase, test.value); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}
