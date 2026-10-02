//go:build windows

package main

import (
	"context"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var rootUser32 = windows.NewLazySystemDLL("user32.dll")
var rootTop = rootUser32.NewProc("GetTopWindow")
var rootNext = rootUser32.NewProc("GetWindow")
var rootThread = rootUser32.NewProc("GetWindowThreadProcessId")
var rootVisible = rootUser32.NewProc("IsWindowVisible")
var rootIconic = rootUser32.NewProc("IsIconic")
var rootForeground = rootUser32.NewProc("GetForegroundWindow")

// Aggregate only the exact bound root. Unlike the shared diagnostic inventory,
// other Chrome windows cannot consume a sixteen-window output allowance.
type rootSample struct {
	At           string `json:"at"`
	Trigger      string `json:"trigger"`
	PID          uint32 `json:"pid"`
	Status       string `json:"status"`
	ScanComplete bool   `json:"scanComplete"`
	Visible      int    `json:"visibleRootCount"`
	Exposed      int    `json:"visibleNonMinimizedRootCount"`
	Foreground   bool   `json:"rootForeground"`
}

func readRoot(pid uint32) rootSample {
	s := rootSample{At: time.Now().UTC().Format(time.RFC3339Nano), PID: pid, Status: "available"}
	fg, _, _ := rootForeground.Call()
	var foregroundPID uint32
	if fg == 0 {
		s.Status = "foreground_unavailable"
	} else {
		thread, _, _ := rootThread.Call(fg, uintptr(unsafe.Pointer(&foregroundPID)))
		if thread == 0 || foregroundPID == 0 {
			s.Status = "foreground_identity_unavailable"
		} else {
			s.Foreground = foregroundPID == pid
		}
	}
	w, _, _ := rootTop.Call(0)
	identityComplete := w != 0
	for count := 0; w != 0 && count < 512; count++ {
		var windowPID uint32
		thread, _, _ := rootThread.Call(w, uintptr(unsafe.Pointer(&windowPID)))
		if thread == 0 {
			identityComplete = false
		}
		if thread != 0 && windowPID == pid {
			visible, _, _ := rootVisible.Call(w)
			minimized, _, _ := rootIconic.Call(w)
			if visible != 0 {
				s.Visible++
				if minimized == 0 {
					s.Exposed++
				}
			}
		}
		w, _, _ = rootNext.Call(w, 2)
	}
	s.ScanComplete = w == 0 && identityComplete
	return s
}

func observeRoot(ctx context.Context, pid uint32, emit func(any), ready chan struct{}, done chan struct{}) {
	defer close(done)
	emitSample := func(trigger string) {
		s := readRoot(pid)
		s.Trigger = trigger
		if ctx.Err() == context.DeadlineExceeded {
			s.Status = "duration_limit_reached"
		}
		emit(map[string]any{"type": "root_sample", "sample": s})
	}
	last := readRoot(pid)
	last.Trigger = "root_start"
	emit(map[string]any{"type": "root_sample", "sample": last})
	close(ready)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	count := 1
	for {
		select {
		case <-ctx.Done():
			emitSample("root_end")
			return
		case <-ticker.C:
			s := readRoot(pid)
			if s.Status != last.Status || s.ScanComplete != last.ScanComplete || s.Visible != last.Visible || s.Exposed != last.Exposed || s.Foreground != last.Foreground {
				s.Trigger = "root_state_change"
				emit(map[string]any{"type": "root_sample", "sample": s})
				last = s
				count++
			}
			if count >= 254 {
				s.Trigger = "root_end"
				s.Status = "record_limit_reached"
				emit(map[string]any{"type": "root_sample", "sample": s})
				return
			}
		}
	}
}
