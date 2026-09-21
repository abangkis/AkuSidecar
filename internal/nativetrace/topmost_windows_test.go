//go:build windows

package nativetrace

import "testing"

func TestReadTopmostInvalidHandleIsUnavailable(t *testing.T) {
	if topmost, available := readTopmost(0); topmost || available {
		t.Fatalf("invalid HWND must not become known non-topmost: %v %v", topmost, available)
	}
}

func TestReadTopmostDesktopIsAvailable(t *testing.T) {
	// Read an existing system-owned window; create/activate/reorder nothing.
	desktop, _, _ := user32.NewProc("GetDesktopWindow").Call()
	if desktop == 0 {
		t.Skip("desktop HWND unavailable in this environment")
	}
	if _, available := readTopmost(desktop); !available {
		t.Fatal("desktop extended style could not be read")
	}
}
