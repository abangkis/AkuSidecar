//go:build windows && auth_session_dev && auth_session_fixture

package engine

import "context"

// Sequential diagnostic reads, not an atomic ownership decision. Unknown
// inventory has count -1 and no flags; it must never look like zero windows.
type DevelopmentReaderWindowEvidence struct {
	InventoryKnown    bool
	InventoryError    string
	WindowCount       int
	BootstrapFlags    []bool
	CoordinatorPaused bool
	SQLiteKnown       bool
	Pause             DevelopmentPauseEvidence
}

func captureDevelopmentReaderWindows(inventory func() ([]bool, error), pause func() DevelopmentPauseEvidence, coordinatorPaused func() bool) DevelopmentReaderWindowEvidence {
	v := DevelopmentReaderWindowEvidence{WindowCount: -1}
	flags, err := inventory()
	if err != nil {
		v.InventoryError = err.Error()
	} else {
		v.InventoryKnown = true
		v.WindowCount = len(flags)
		v.BootstrapFlags = append([]bool{}, flags...)
	}
	v.Pause = pause()
	v.SQLiteKnown = v.Pause.SQLiteError == ""
	v.CoordinatorPaused = coordinatorPaused()
	return v
}

func (r *DevelopmentFixtureRuntime) ReaderWindowEvidence(ctx context.Context, sessionID string) DevelopmentReaderWindowEvidence {
	return captureDevelopmentReaderWindows(func() ([]bool, error) { return r.ReaderWindows(ctx) }, func() DevelopmentPauseEvidence { return r.PauseEvidence(ctx, sessionID) }, func() bool { return r.Coordinator.Status().BatchPaused })
}
