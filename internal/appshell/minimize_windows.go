//go:build windows

package appshell

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var showMinimizedWithoutActivation = user32.NewProc("ShowWindowAsync")
var isCaptureIconic = user32.NewProc("IsIconic")

// This one-time startup action is exclusive to the opted-in capture root.
// Chromium may show its first window before it is observed here; no claim of
// zero startup promotion is made. Never restore/focus/reorder the user's HWND.
func (o *processOwnership) minimizeInitialWindow(ctx context.Context, pid int) error {
	if o.job == 0 || pid <= 0 {
		return fmt.Errorf("capture startup has no owned process/job")
	}
	deadline := time.Now().Add(10 * time.Second)
	var hwnd uintptr
	for hwnd == 0 && time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		hwnd = visibleTopLevelWindow(uint32(pid))
		if hwnd == 0 {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if hwnd == 0 {
		return fmt.Errorf("capture host did not expose an owned startup window")
	}
	var actualPID uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&actualPID)))
	if actualPID != uint32(pid) {
		return fmt.Errorf("capture startup HWND ownership changed")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, actualPID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	if !processBelongsToJob(process, o.job) {
		return fmt.Errorf("capture startup HWND is outside its Job Object")
	}
	result, _, _ := showMinimizedWithoutActivation.Call(hwnd, 7) // SW_SHOWMINNOACTIVE
	if result == 0 {
		return fmt.Errorf("capture startup minimization was rejected")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&actualPID)))
		if actualPID != uint32(pid) {
			return fmt.Errorf("capture startup HWND changed before readback")
		}
		iconic, _, _ := isCaptureIconic.Call(hwnd)
		if iconic != 0 {
			return nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("capture startup minimization could not be verified")
}
