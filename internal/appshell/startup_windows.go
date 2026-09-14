//go:build windows

package appshell

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	startupRegisterClass    = user32.NewProc("RegisterClassExW")
	startupUnregisterClass  = user32.NewProc("UnregisterClassW")
	startupCreateWindow     = user32.NewProc("CreateWindowExW")
	startupDefWindowProc    = user32.NewProc("DefWindowProcW")
	startupDestroyWindow    = user32.NewProc("DestroyWindow")
	startupShowWindow       = user32.NewProc("ShowWindow")
	startupSetWindowText    = user32.NewProc("SetWindowTextW")
	startupEnableWindow     = user32.NewProc("EnableWindow")
	startupSetTimer         = user32.NewProc("SetTimer")
	startupKillTimer        = user32.NewProc("KillTimer")
	startupGetMessage       = user32.NewProc("GetMessageW")
	startupTranslateMessage = user32.NewProc("TranslateMessage")
	startupDispatchMessage  = user32.NewProc("DispatchMessageW")
	startupIsDialogMessage  = user32.NewProc("IsDialogMessageW")
	startupPostQuitMessage  = user32.NewProc("PostQuitMessage")
	startupGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	startupLoadCursor       = user32.NewProc("LoadCursorW")
	startupOpenClipboard    = user32.NewProc("OpenClipboard")
	startupCloseClipboard   = user32.NewProc("CloseClipboard")
	startupEmptyClipboard   = user32.NewProc("EmptyClipboard")
	startupSetClipboardData = user32.NewProc("SetClipboardData")
	startupGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	startupGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	startupGlobalLock       = kernel32.NewProc("GlobalLock")
	startupGlobalUnlock     = kernel32.NewProc("GlobalUnlock")
	startupGlobalFree       = kernel32.NewProc("GlobalFree")
	startupGetStockObject   = windows.NewLazySystemDLL("gdi32.dll").NewProc("GetStockObject")
)

type startupWindowClass struct {
	Size, Style                        uint32
	Procedure                          uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}

type startupMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

const startupOpeningText = "Opening AkuBrowser…\r\n\r\nThe local service has started. Waiting for the interface to initialize."
const startupOverdueText = "The interface has not confirmed it is ready.\r\n\r\nIf AkuBrowser is still blank, Retry window closes and reopens its browser using the same profile. X closes only this status."
const startupReadyText = "The interface reports ready.\r\n\r\nIf it is still blank, use Retry window or copy diagnostics. When AkuBrowser is visible, X closes only this status."
const startupRetryText = "Retrying the window…\r\n\r\nWaiting for the previous browser to close before reopening it. The local service and your data stay in place."
const startupRetryFailedText = "The window could not be retried.\r\n\r\nCopy diagnostics or use Retry window again. X closes AkuBrowser if no browser window remains; otherwise it closes only this status."

// Start owns one ordinary native window on its own message-loop thread. It is
// never topmost and never requests foreground activation. An explicit user
// click may activate it normally so keyboard and accessibility still work.
func (s *Startup) Start(reportError func(error)) {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := s.runStartupWindow(); err != nil && reportError != nil {
			reportError(err)
		}
	}()
}

func startupWide(value string) *uint16 { return windows.StringToUTF16Ptr(value) }

func (s *Startup) runStartupWindow() error {
	instance, _, _ := startupGetModuleHandle.Call(0)
	className := startupWide(fmt.Sprintf("AkuBrowserStartup-%p", s))
	var textWindow, retryButton uintptr
	shown, destroyed := false, false
	lastText := startupOpeningText
	procedure := syscall.NewCallback(func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case 0x0113: // WM_TIMER: all native updates stay on this thread.
			show, overdue, finished, ready := s.status(time.Now())
			if finished {
				startupDestroyWindow.Call(hwnd)
				return 0
			}
			busy, failed := s.retryStatus()
			text := startupOpeningText
			if overdue {
				text = startupOverdueText
			}
			if ready {
				text = startupReadyText
			}
			if failed {
				text = startupRetryFailedText
			}
			if busy {
				text = startupRetryText
			}
			if text != lastText {
				startupSetWindowText.Call(textWindow, uintptr(unsafe.Pointer(startupWide(text))))
				lastText = text
			}
			enabled := uintptr(1)
			if busy {
				enabled = 0
			}
			startupEnableWindow.Call(retryButton, enabled)
			if show && !shown {
				startupShowWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE; never SetForegroundWindow.
				shown = true
			}
			return 0
		case 0x0111: // WM_COMMAND
			switch wParam & 0xffff {
			case 101:
				s.keepWaiting(time.Now())
			case 102:
				if err := copyStartupDiagnostics(hwnd, s.diagnostics(time.Now())); err != nil {
					startupSetWindowText.Call(textWindow, uintptr(unsafe.Pointer(startupWide("Diagnostics could not be copied. Try Copy diagnostics again.\r\n\r\nX closes only this status window."))))
				}
			case 103:
				s.requestRetry()
			}
			return 0
		case 0x0010: // WM_CLOSE: do not terminate Chromium or Sidecar.
			s.Stop()
			startupDestroyWindow.Call(hwnd)
			return 0
		case 0x0002: // WM_DESTROY
			destroyed = true
			startupKillTimer.Call(hwnd, 1)
			startupPostQuitMessage.Call(0)
			return 0
		}
		result, _, _ := startupDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
		return result
	})
	cursor, _, _ := startupLoadCursor.Call(0, 32512) // IDC_ARROW, owned by Windows.
	class := startupWindowClass{Size: uint32(unsafe.Sizeof(startupWindowClass{})), Procedure: procedure, Instance: instance, Cursor: cursor, Background: 6, ClassName: className}
	if result, _, err := startupRegisterClass.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("register startup window: %w", err)
	}
	defer startupUnregisterClass.Call(uintptr(unsafe.Pointer(className)), instance)
	width, height := uintptr(560), uintptr(245)
	screenWidth, _, _ := startupGetSystemMetrics.Call(0)
	screenHeight, _, _ := startupGetSystemMetrics.Call(1)
	x, y := uintptr(0), uintptr(0)
	if screenWidth > width {
		x = (screenWidth - width) / 2
	}
	if screenHeight > height {
		y = (screenHeight - height) / 2
	}
	// WS_EX_CONTROLPARENT; ordinary caption/system menu/minimize, no WS_VISIBLE
	// until the delay expires, no WS_EX_TOPMOST or forced foreground behavior.
	hwnd, _, err := startupCreateWindow.Call(0x00010000, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(startupWide("AkuBrowser startup"))), 0x00CA0000, x, y, width, height, 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("create startup window: %w", err)
	}
	defer func() {
		if !destroyed {
			startupDestroyWindow.Call(hwnd)
		}
	}()
	font, _, _ := startupGetStockObject.Call(17) // DEFAULT_GUI_FONT, owned by Windows.
	createControl := func(kind, label string, id, left, top, w, h uintptr, tab bool) (uintptr, error) {
		style := uintptr(0x50000000) // WS_CHILD | WS_VISIBLE
		if tab {
			style |= 0x00010000
		} // WS_TABSTOP
		control, _, err := startupCreateWindow.Call(0, uintptr(unsafe.Pointer(startupWide(kind))), uintptr(unsafe.Pointer(startupWide(label))), style, left, top, w, h, hwnd, id, instance, 0)
		if control == 0 {
			return 0, fmt.Errorf("create startup control: %w", err)
		}
		procSendMessageW.Call(control, 0x0030, font, 1) // WM_SETFONT
		return control, nil
	}
	textWindow, err = createControl("STATIC", startupOpeningText, 0, 20, 18, 510, 120, false)
	if err != nil {
		return err
	}
	for _, control := range []struct {
		label           string
		id, left, width uintptr
	}{
		{"Keep waiting", 101, 20, 135}, {"Copy diagnostics", 102, 165, 180}, {"Retry window", 103, 355, 160},
	} {
		handle, err := createControl("BUTTON", control.label, control.id, control.left, 155, control.width, 30, true)
		if err != nil {
			return err
		}
		if control.id == 103 {
			retryButton = handle
		}
	}
	if result, _, err := startupSetTimer.Call(hwnd, 1, 250, 0); result == 0 {
		return fmt.Errorf("start status timer: %w", err)
	}
	var message startupMessage
	for {
		result, _, err := startupGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("read startup messages: %w", err)
		}
		if result == 0 {
			return nil
		}
		if handled, _, _ := startupIsDialogMessage.Call(hwnd, uintptr(unsafe.Pointer(&message))); handled == 0 {
			startupTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			startupDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
		}
	}
}

func copyStartupDiagnostics(hwnd uintptr, text string) error {
	wide, err := windows.UTF16FromString(text)
	if err != nil {
		return err
	}
	memory, _, err := startupGlobalAlloc.Call(0x0042, uintptr(len(wide)*2)) // GMEM_MOVEABLE | GMEM_ZEROINIT
	if memory == 0 {
		return err
	}
	transferred := false
	defer func() {
		if !transferred {
			startupGlobalFree.Call(memory)
		}
	}()
	pointer, _, err := startupGlobalLock.Call(memory)
	if pointer == 0 {
		return err
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(pointer)), len(wide)), wide)
	startupGlobalUnlock.Call(memory)
	if result, _, err := startupOpenClipboard.Call(hwnd); result == 0 {
		return err
	}
	defer startupCloseClipboard.Call()
	if result, _, err := startupEmptyClipboard.Call(); result == 0 {
		return err
	}
	if result, _, err := startupSetClipboardData.Call(13, memory); result == 0 {
		return err
	} // CF_UNICODETEXT
	transferred = true // Windows now owns the allocation.
	return nil
}
