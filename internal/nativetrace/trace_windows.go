//go:build windows

package nativetrace

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var setHook = user32.NewProc("SetWinEventHook")
var unhook = user32.NewProc("UnhookWinEvent")
var foreground = user32.NewProc("GetForegroundWindow")
var topWindow = user32.NewProc("GetTopWindow")
var nextWindow = user32.NewProc("GetWindow")
var threadProcess = user32.NewProc("GetWindowThreadProcessId")
var ancestor = user32.NewProc("GetAncestor")
var visible = user32.NewProc("IsWindowVisible")
var iconic = user32.NewProc("IsIconic")
var windowLong = user32.NewProc("GetWindowLongW")
var clearLastError = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError")
var windowRect = user32.NewProc("GetWindowRect")
var guiThread = user32.NewProc("GetGUIThreadInfo")
var peek = user32.NewProc("PeekMessageW")
var translate = user32.NewProc("TranslateMessage")
var dispatch = user32.NewProc("DispatchMessageW")
var callbacks sync.Map

// NewCallback slots are never freed by Go. Allocate exactly one for all traces.
var foregroundCallback = syscall.NewCallback(func(hook uintptr, event uint32, hwnd uintptr, object int32, child int32, thread uint32, tick uint32) uintptr {
	if fn, ok := callbacks.Load(hook); ok {
		fn.(func(uintptr, uint32, int32, int32))(hwnd, tick, object, child)
	}
	return 0
})

type rect struct{ Left, Top, Right, Bottom int32 }
type guiInfo struct {
	Size, Flags                                        uint32
	Active, Focus, Capture, MenuOwner, MoveSize, Caret uintptr
	CaretRect                                          rect
}
type message struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}

func handle(hwnd uintptr) string { return fmt.Sprintf("0x%x", hwnd) }

// readTopmost reads only GWL_EXSTYLE. A zero style is valid, so clear the
// thread-local last error before distinguishing it from a failed read. Keep
// this pair on one OS thread even if called outside the observer loop.
func readTopmost(hwnd uintptr) (topmost, available bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	clearLastError.Call(0)
	style, _, err := windowLong.Call(hwnd, ^uintptr(19)) // GWL_EXSTYLE = -20
	if style == 0 && err != syscall.Errno(0) {
		return false, false
	}
	return style&0x00000008 != 0, true // WS_EX_TOPMOST
}

func processClass(pid, browser uint32) string {
	if pid == 0 {
		return "unknown"
	}
	if browser != 0 && pid == browser {
		return "akubrowser_root"
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "unavailable"
	}
	defer windows.CloseHandle(p)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if windows.QueryFullProcessImageName(p, 0, &buf[0], &n) != nil {
		return "unavailable"
	}
	// The path is used only for classification and never retained or emitted.
	if strings.EqualFold(filepath.Base(windows.UTF16ToString(buf[:n])), "chrome.exe") {
		return "chrome_other"
	}
	return "other"
}

func snapshot(browser uint32, classes map[uint32]string) Sample {
	s := Sample{At: time.Now().UTC().Format(time.RFC3339Nano), Status: "available", Windows: []Window{}}
	fg, _, _ := foreground.Call()
	s.Foreground = handle(fg)
	thread, _, _ := threadProcess.Call(fg, 0)
	info := guiInfo{Size: uint32(unsafe.Sizeof(guiInfo{}))}
	if thread != 0 {
		ok, _, _ := guiThread.Call(thread, uintptr(unsafe.Pointer(&info)))
		s.FocusAvailable = ok != 0
		if ok != 0 {
			s.Focus = handle(info.Focus)
		}
	}
	var fgRect rect
	fgRectOK, _, _ := windowRect.Call(fg, uintptr(unsafe.Pointer(&fgRect)))
	w, _, _ := topWindow.Call(0)
	fgZ := -1
	for z := 0; w != 0 && z < 512; z++ {
		if w == fg {
			fgZ = z
		}
		var pid uint32
		threadProcess.Call(w, uintptr(unsafe.Pointer(&pid)))
		class, ok := classes[pid]
		if !ok {
			class = processClass(pid, browser)
			if len(classes) < 128 {
				classes[pid] = class
			}
		}
		if w == fg || class == "akubrowser_root" || class == "chrome_other" {
			if len(s.Windows) < 16 || w == fg {
				root, _, _ := ancestor.Call(w, 2) // GA_ROOT
				v, _, _ := visible.Call(w)
				m, _, _ := iconic.Call(w)
				var r rect
				rOK, _, _ := windowRect.Call(w, uintptr(unsafe.Pointer(&r)))
				overlaps := fgRectOK != 0 && rOK != 0 && r.Left < fgRect.Right && r.Right > fgRect.Left && r.Top < fgRect.Bottom && r.Bottom > fgRect.Top
				entry := Window{HWND: handle(w), Root: handle(root), PID: pid, Class: class, Visible: v != 0, Minimized: m != 0, Z: z, OverlapsForeground: overlaps, OverlapAvailable: fgRectOK != 0 && rOK != 0}
				entry.Topmost, entry.TopmostAvailable = readTopmost(w)
				if len(s.Windows) == 16 {
					s.Windows[15] = entry
					s.Truncated = true
				} else {
					s.Windows = append(s.Windows, entry)
				}
			} else {
				s.Truncated = true
			}
		}
		w, _, _ = nextWindow.Call(w, 2) // GW_HWNDNEXT, observation only
	}
	if w != 0 {
		s.Truncated = true
	}
	for i := range s.Windows {
		s.Windows[i].AboveForeground = fgZ >= 0 && s.Windows[i].Z < fgZ
	}
	s.ZOrderAvailable = fgZ >= 0
	if fg == 0 {
		s.Status = "foreground_unavailable"
	}
	return s
}

func observe(ctx context.Context, browser uint32, ready chan struct{}, emit func(Sample)) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	classes := map[uint32]string{}
	lastForeground, lastState := "", ""
	var previous Sample
	status := "available"
	record := func(trigger string, hwnd uintptr, tick uint32) {
		s := snapshot(browser, classes)
		if status != "available" {
			s.Status = status
		}
		// Compare native state only; timestamps and event provenance must not
		// force periodic writes when nothing changed.
		key, _ := json.Marshal(struct {
			Foreground, Focus, Status string
			FocusAvailable, Truncated bool
			Windows                   []Window
		}{s.Foreground, s.Focus, s.Status, s.FocusAvailable, s.Truncated, s.Windows})
		if (trigger == "state_change" || trigger == "reorder_event") && string(key) == lastState {
			return
		}
		s.Trigger, s.EventTick = trigger, tick
		if hwnd != 0 {
			s.EventHWND = handle(hwnd)
			threadProcess.Call(hwnd, uintptr(unsafe.Pointer(&s.EventPID)))
			s.EventClass = processClass(s.EventPID, browser)
			root, _, _ := ancestor.Call(hwnd, 2)
			s.EventRoot = handle(root)
		}
		s.PreviousForeground = lastForeground
		s.Classification = classifyCovering(s, previous)
		previous = s
		lastForeground, lastState = s.Foreground, string(key)
		emit(s)
	}
	hook, _, _ := setHook.Call(3, 3, 0, foregroundCallback, 0, 0, 0) // EVENT_SYSTEM_FOREGROUND, OUTOFCONTEXT
	if hook == 0 {
		status = "foreground_hook_unavailable_polling"
	} else {
		callbacks.Store(hook, func(hwnd uintptr, tick uint32, _, _ int32) { record("foreground_event", hwnd, tick) })
		defer callbacks.Delete(hook)
		defer unhook.Call(hook)
	}
	if browser != 0 {
		// EVENT_OBJECT_REORDER, restricted by the OS to the known browser-root
		// process. Filter to top-level window events before taking a snapshot.
		reorderHook, _, _ := setHook.Call(0x8004, 0x8004, 0, foregroundCallback, uintptr(browser), 0, 0)
		if reorderHook == 0 {
			if status == "available" {
				status = "reorder_hook_unavailable_polling"
			} else {
				status = "foreground_and_reorder_hooks_unavailable_polling"
			}
		} else {
			callbacks.Store(reorderHook, func(hwnd uintptr, tick uint32, object, child int32) {
				if hwnd == 0 || object != 0 || child != 0 { // OBJID_WINDOW, CHILDID_SELF
					return
				}
				var pid uint32
				threadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
				root, _, _ := ancestor.Call(hwnd, 2) // GA_ROOT
				if pid == browser && root == hwnd {
					record("reorder_event", hwnd, tick)
				}
			})
			defer callbacks.Delete(reorderHook)
			defer unhook.Call(reorderHook)
		}
	}
	record("trace_start", 0, 0)
	close(ready)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	nextSnapshot := time.Now().Add(100 * time.Millisecond)
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				status = "duration_limit_reached"
			}
			record("trace_end", 0, 0)
			return
		case <-ticker.C:
			var msg message
			for count := 0; count < 64; count++ {
				ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
				if ok == 0 {
					break
				}
				translate.Call(uintptr(unsafe.Pointer(&msg)))
				dispatch.Call(uintptr(unsafe.Pointer(&msg)))
			}
			if !time.Now().Before(nextSnapshot) {
				record("state_change", 0, 0)
				nextSnapshot = time.Now().Add(100 * time.Millisecond)
			}
		}
	}
}
