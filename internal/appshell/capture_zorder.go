package appshell

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// CaptureContainment exists only for the opted-in Windows capture process.
// Reader preparation binds a single HWND; its returned foreground action must
// only be called by the authenticated, single-use explicit reader transport.
type CaptureContainment interface {
	PrepareReader(context.Context, string) (func(context.Context) error, error)
	PrepareBrokerReader(context.Context, string) (readerbroker.Target, func(context.Context) error, error)
	Stop()
}

type captureZWindow struct {
	hwnd                   uintptr
	owned, reader, visible bool
}

// EnumWindows order is front-to-back. Never operate without a known external
// foreground anchor, or on that anchor, a reader, or an unowned window.
func captureWindowsToLower(foreground uintptr, ordered []captureZWindow) []uintptr {
	if foreground == 0 {
		return nil
	}
	anchor := -1
	for i, w := range ordered {
		if w.hwnd == foreground {
			if w.owned && !w.reader {
				return nil
			}
			anchor = i
			break
		}
	}
	if anchor < 0 {
		return nil
	}
	var result []uintptr
	for _, w := range ordered[:anchor] {
		if w.owned && !w.reader && w.visible {
			result = append(result, w.hwnd)
		}
	}
	return result
}

func captureReadbackVerified(hwnd, foreground uintptr, ordered []captureZWindow) bool {
	if foreground == 0 {
		return false
	}
	anchorSeen := false
	for _, w := range ordered {
		if w.hwnd == foreground {
			anchorSeen = !w.owned || w.reader
		}
		if w.hwnd == hwnd {
			return anchorSeen && w.owned && w.visible && !w.reader
		}
	}
	return false
}

// Recheck current native identity immediately before the write. All operands
// are functions so tests also prove that rejection never reaches SetWindowPos.
func lowerCaptureWindow(expectedForeground uintptr, foreground func() uintptr, owned, reader func() bool, lower func() bool) (attempted, applied bool) {
	if expectedForeground == 0 || foreground() != expectedForeground || !owned() || reader() {
		return false, false
	}
	return true, lower()
}
