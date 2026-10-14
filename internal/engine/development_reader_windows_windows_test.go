//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"errors"
	"testing"
)

func TestDevelopmentFrontendNativeWindowSnapshotPreservesUnknownAndIndependentPause(t *testing.T) {
	flags := []bool{true, false}
	pause := DevelopmentPauseEvidence{CollectorPID: 17, ManagerPID: 17, Driver: "browser", SQLitePaused: true}
	v := captureDevelopmentReaderWindows(func() ([]bool, error) { return flags, nil }, func() DevelopmentPauseEvidence { return pause }, func() bool { return false })
	flags[0] = false
	if !v.InventoryKnown || v.WindowCount != 2 || len(v.BootstrapFlags) != 2 || !v.BootstrapFlags[0] || v.CoordinatorPaused || !v.SQLiteKnown || !v.Pause.SQLitePaused || v.Pause.CollectorPID != 17 || v.Pause.Driver != "browser" {
		t.Fatalf("snapshot lost independent values or retained mutable flags: %+v", v)
	}
	pause.SQLiteError = "database read failed"
	v = captureDevelopmentReaderWindows(func() ([]bool, error) { return []bool{true}, errors.New("inventory failed") }, func() DevelopmentPauseEvidence { return pause }, func() bool { return true })
	if v.InventoryKnown || v.WindowCount != -1 || v.BootstrapFlags != nil || v.InventoryError != "inventory failed" || v.SQLiteKnown || v.Pause.SQLiteError != "database read failed" || !v.CoordinatorPaused {
		t.Fatalf("unknown was converted to empty inventory or a known SQLite pause: %+v", v)
	}
	v = captureDevelopmentReaderWindows(func() ([]bool, error) { return nil, nil }, func() DevelopmentPauseEvidence { return DevelopmentPauseEvidence{} }, func() bool { return false })
	if !v.InventoryKnown || v.WindowCount != 0 || v.InventoryError != "" || v.BootstrapFlags == nil {
		t.Fatalf("known empty inventory lost distinction: %+v", v)
	}
}
