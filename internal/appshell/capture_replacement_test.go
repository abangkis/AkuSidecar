package appshell

import "testing"

func TestReaderLifetimeOnlyReleasesAfterNativeWindowCloses(t *testing.T) {
	readers := map[uintptr]struct{}{11: {}, 22: {}}
	// A reader remains live even when its foreground token was consumed or it
	// is minimized. Neither condition participates in this existence boundary.
	if !retainLiveReaders(readers, func(hwnd uintptr) bool { return hwnd == 22 }) || len(readers) != 1 {
		t.Fatalf("live reader released: %+v", readers)
	}
	if retainLiveReaders(readers, func(uintptr) bool { return false }) || len(readers) != 0 {
		t.Fatalf("closed reader retained: %+v", readers)
	}
}
