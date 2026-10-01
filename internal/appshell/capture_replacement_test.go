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

func TestCapturePopupReadiness(t *testing.T) {
	host := captureZWindow{hwnd: 1, owned: true, chromeWindow: true, host: true}
	for _, tc := range []struct {
		name    string
		windows []captureZWindow
		ready   bool
	}{
		{"host only", []captureZWindow{host}, true},
		{"other user Chrome", []captureZWindow{host, {hwnd: 2, chromeWindow: true}}, true},
		{"native helper", []captureZWindow{host, {hwnd: 2, owned: true}}, true},
		{"visible popup", []captureZWindow{host, {hwnd: 2, owned: true, chromeWindow: true, visible: true}}, false},
		{"hidden or minimized popup after parent closed", []captureZWindow{host, {hwnd: 3, owned: true, chromeWindow: true}}, false},
		{"enumeration failed", nil, false},
		{"host missing", []captureZWindow{}, false},
		{"duplicate host marker", []captureZWindow{host, host}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := capturePopupReadiness(tc.windows) == nil; got != tc.ready {
				t.Fatalf("ready=%t want %t", got, tc.ready)
			}
		})
	}
}
