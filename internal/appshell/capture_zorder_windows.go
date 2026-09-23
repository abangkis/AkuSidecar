//go:build windows

package appshell

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

var (
	captureForeground  = user32.NewProc("GetForegroundWindow")
	captureSetPosition = user32.NewProc("SetWindowPos")
	captureSetHook     = user32.NewProc("SetWinEventHook")
	captureUnhook      = user32.NewProc("UnhookWinEvent")
	captureGetMessage  = user32.NewProc("GetMessageW")
	capturePeekMessage = user32.NewProc("PeekMessageW")
	capturePostThread  = user32.NewProc("PostThreadMessageW")
	captureWindowText  = user32.NewProc("GetWindowTextW")
	captureSetProp     = user32.NewProc("SetPropW")
	captureGetProp     = user32.NewProc("GetPropW")
	captureRemoveProp  = user32.NewProc("RemovePropW")
)

type captureZOrder struct {
	mu                                   sync.Mutex
	job                                  windows.Handle
	root                                 uint32
	logger                               *log.Logger
	stop, done                           chan struct{}
	once                                 sync.Once
	readers                              map[uintptr]captureReaderBinding
	nextReader                           uintptr
	property                             *uint16
	attempted, applied, readback, failed uint64
	foregroundCycles                     uint64
	lastFailure                          string
	pending                              map[uintptr]time.Time
	stopped                              bool
}

type captureReaderBinding struct {
	id      uintptr
	expires time.Time
}

// The caller gates this to Windows + experimental split. An ordinary window
// cannot opt in accidentally: Launch must have marked it as a capture host.
func (w *Window) StartCaptureContainment(logger *log.Logger) (CaptureContainment, error) {
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	if !w.captureHost || w.owner.job == 0 || w.PID() <= 0 {
		return nil, errors.New("capture containment requires an owned split capture host")
	}
	select {
	case <-w.closed:
		return nil, errors.New("capture process already exited")
	default:
	}
	if w.containment != nil {
		return w.containment, nil
	}
	prop, _ := windows.UTF16PtrFromString("AkuBrowser.ExplicitReader." + fmt.Sprint(w.PID()))
	c := &captureZOrder{job: w.owner.job, root: uint32(w.PID()), logger: logger, stop: make(chan struct{}), done: make(chan struct{}), readers: make(map[uintptr]captureReaderBinding), property: prop, pending: make(map[uintptr]time.Time)}
	w.containment = c
	go c.run()
	return c, nil
}

func (c *captureZOrder) owns(hwnd uintptr) bool {
	if hwnd == 0 || c.job == 0 {
		return false
	}
	var pid uint32
	procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return false
	}
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(p)
	return processBelongsToJob(p, c.job)
}

func (c *captureZOrder) isReader(hwnd uintptr) bool {
	binding, ok := c.readers[hwnd]
	if !ok {
		return false
	}
	if time.Now().After(binding.expires) {
		captureRemoveProp.Call(hwnd, uintptr(unsafe.Pointer(c.property)))
		delete(c.readers, hwnd)
		return false
	}
	value, _, _ := captureGetProp.Call(hwnd, uintptr(unsafe.Pointer(c.property)))
	if value != binding.id {
		delete(c.readers, hwnd)
		return false
	}
	return true
}

func (c *captureZOrder) snapshot() []captureZWindow {
	var result []captureZWindow
	owners := make(map[uint32]bool)
	// Reuse one callback for this monitor's lifetime: Windows callbacks cannot
	// be freed, so allocating one per polling pass would exhaust the runtime.
	captureEnumerationMu.Lock()
	defer captureEnumerationMu.Unlock()
	captureEnumerationVisit = func(hwnd uintptr) {
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		var pid uint32
		procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		owned, known := owners[pid]
		if !known {
			owned = c.owns(hwnd)
			owners[pid] = owned
		}
		result = append(result, captureZWindow{hwnd: hwnd, owned: owned, reader: owned && c.isReader(hwnd), visible: visible != 0})
	}
	defer func() { captureEnumerationVisit = nil }()
	ok, _, _ := procEnumWindows.Call(captureEnumerationCallback, 0)
	if ok == 0 {
		return nil
	}
	return result
}

var captureEnumerationMu sync.Mutex
var captureEnumerationVisit func(uintptr)
var captureEnumerationCallback = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
	if captureEnumerationVisit != nil {
		captureEnumerationVisit(hwnd)
	}
	return 1
})

func (c *captureZOrder) cycle() {
	c.mu.Lock()
	defer c.mu.Unlock()
	fg, _, _ := captureForeground.Call()
	ordered := c.snapshot()
	if ordered == nil {
		c.failed++
		c.lastFailure = "window_enumeration_unavailable"
		return
	}
	if captureOwnedForeground(fg, ordered) {
		// Chrome may asynchronously promote a capture window beyond the Bridge's
		// sampled focus checks. The containment path never activates an
		// external HWND, so retain explicit evidence instead of silently treating
		// this foreground state as an empty target set.
		c.foregroundCycles++
	}
	targets := captureWindowsToLower(fg, ordered)
	readbackForeground, _, _ := captureForeground.Call()
	for hwnd, at := range c.pending {
		// Readback counts only a still-owned, visible window below the same
		// currently sampled anchor, never disappearance or a missing anchor.
		found := readbackForeground == fg && captureReadbackVerified(hwnd, fg, ordered)
		if found {
			c.readback++
			delete(c.pending, hwnd)
		} else if time.Since(at) > time.Second {
			c.failed++
			c.lastFailure = "readback_unverified"
			delete(c.pending, hwnd)
		}
	}
	for _, hwnd := range targets {
		// Revalidate immediately before every write. HWND reuse and foreground
		// changes fail closed; the external HWND is never a mutation target.
		if at, pending := c.pending[hwnd]; pending && time.Since(at) < 100*time.Millisecond {
			continue
		}
		// HWND_BOTTOM also removes topmost status. ASYNCWINDOWPOS avoids
		// waiting on Chromium's UI thread; a subsequent pass proves readback.
		attempted, applied := lowerCaptureWindow(fg,
			func() uintptr { value, _, _ := captureForeground.Call(); return value },
			func() bool { return c.owns(hwnd) }, func() bool { return c.isReader(hwnd) },
			func() bool {
				ok, _, _ := captureSetPosition.Call(hwnd, 1, 0, 0, 0, 0, 0x0001|0x0002|0x0010|0x0200|0x0400|0x4000)
				return ok != 0
			})
		if !attempted {
			continue
		}
		c.attempted++
		if !applied {
			c.failed++
			c.lastFailure = "set_window_pos_rejected"
			continue
		}
		c.applied++
		if _, exists := c.pending[hwnd]; !exists && len(c.pending) < 128 {
			c.pending[hwnd] = time.Now()
		}
	}
}

func (c *captureZOrder) logStats() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.logger != nil && (c.attempted > 0 || c.readback > 0 || c.failed > 0 || c.foregroundCycles > 0) {
		c.logger.Printf("capture_zorder foreground_cycles=%d attempted=%d applied=%d readback=%d failed=%d last_failure=%s", c.foregroundCycles, c.attempted, c.applied, c.readback, c.failed, c.lastFailure)
	}
	c.foregroundCycles, c.attempted, c.applied, c.readback, c.failed = 0, 0, 0, 0, 0
	c.lastFailure = ""
}

func (c *captureZOrder) run() {
	defer close(c.done)
	wake := make(chan struct{}, 1)
	stopHook := startCaptureZOrderHook(c.root, wake, c.logger)
	defer stopHook()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	logs := time.NewTicker(5 * time.Second)
	defer logs.Stop()
	c.cycle()
	for {
		select {
		case <-c.stop:
			c.logStats()
			return
		case <-wake:
			c.cycle()
		case <-ticker.C:
			c.cycle()
		case <-logs.C:
			c.logStats()
		}
	}
}

// One message-pump hook observes capture window show/reorder/location events.
// Native callbacks only enqueue; all ownership checks/writes run in the worker.
func startCaptureZOrderHook(pid uint32, wake chan struct{}, logger *log.Logger) func() {
	ready := make(chan uint32, 1)
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)
		var msg struct {
			hwnd           uintptr
			message        uint32
			wParam, lParam uintptr
			time           uint32
			x, y           int32
			private        uint32
		}
		capturePeekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
		callback := syscall.NewCallback(func(_ uintptr, event uint32, hwnd uintptr, object, child int32, _ uint32, _ uint32) uintptr {
			if hwnd != 0 && object == 0 && child == 0 && (event == 0x8002 || event == 0x8004 || event == 0x800B) {
				select {
				case wake <- struct{}{}:
				default:
				}
			}
			return 0
		})
		hook, _, _ := captureSetHook.Call(0x8002, 0x800B, 0, callback, uintptr(pid), 0, 0)
		if hook == 0 && logger != nil {
			logger.Printf("capture_zorder hook_failed=true fallback_poll_ms=250")
		}
		ready <- windows.GetCurrentThreadId()
		for {
			result, _, _ := captureGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(result) <= 0 {
				break
			}
		}
		if hook != 0 {
			captureUnhook.Call(hook)
		}
	}()
	thread := <-ready
	return func() { capturePostThread.Call(uintptr(thread), 0x0012, 0, 0); <-done }
}

func (c *captureZOrder) Stop() {
	c.once.Do(func() {
		close(c.stop)
		<-c.done
		c.mu.Lock()
		defer c.mu.Unlock()
		c.stopped = true
		for hwnd := range c.readers {
			if c.owns(hwnd) && c.isReader(hwnd) {
				captureRemoveProp.Call(hwnd, uintptr(unsafe.Pointer(c.property)))
			}
		}
	})
}

func (c *captureZOrder) PrepareReader(ctx context.Context, marker string) (func(context.Context) error, error) {
	_, verify, err := c.PrepareBrokerReader(ctx, marker)
	return verify, err
}

func (c *captureZOrder) PrepareBrokerReader(ctx context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
	if !strings.HasPrefix(marker, "AkuBrowser reader split_") || len(marker) > 300 {
		return readerbroker.Target{}, nil, errors.New("invalid reader marker")
	}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		hwnd, id, expires, err := c.bindReader(marker)
		if err != nil {
			return readerbroker.Target{}, nil, err
		}
		if hwnd != 0 {
			var once sync.Once
			var pid uint32
			procGetWindowThreadProcess.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			target := readerbroker.Target{HWND: uint64(hwnd), PID: pid, Property: "AkuBrowser.ExplicitReader." + fmt.Sprint(c.root), Value: uint64(id), Expires: expires, Action: strings.TrimPrefix(marker, "AkuBrowser reader ")}
			return target, func(ctx context.Context) error {
				err := errors.New("reader foreground intent already consumed")
				once.Do(func() { err = c.foregroundReader(ctx, hwnd, id, expires) })
				return err
			}, nil
		}
		select {
		case <-ctx.Done():
			return readerbroker.Target{}, nil, ctx.Err()
		case <-deadline.C:
			return readerbroker.Target{}, nil, errors.New("reader HWND marker not found")
		case <-tick.C:
		}
	}
}

func (c *captureZOrder) bindReader(marker string) (uintptr, uintptr, time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return 0, 0, time.Time{}, errors.New("capture containment stopped")
	}
	var match uintptr
	for _, w := range c.snapshot() {
		if !w.owned {
			continue
		}
		var text [512]uint16
		captureWindowText.Call(w.hwnd, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
		title := windows.UTF16ToString(text[:])
		if title != marker && !strings.HasPrefix(title, marker+" - ") {
			continue
		}
		if match != 0 {
			return 0, 0, time.Time{}, errors.New("ambiguous reader HWND marker")
		}
		match = w.hwnd
	}
	for hwnd := range c.readers {
		if !c.owns(hwnd) || !c.isReader(hwnd) {
			delete(c.readers, hwnd)
		}
	}
	if match == 0 {
		return 0, 0, time.Time{}, nil
	}
	if !c.owns(match) {
		return 0, 0, time.Time{}, errors.New("reader HWND ownership changed")
	}
	if c.isReader(match) {
		binding := c.readers[match]
		return match, binding.id, binding.expires, nil
	}
	if len(c.readers) >= 32 {
		return 0, 0, time.Time{}, errors.New("reader HWND limit reached")
	}
	c.nextReader++
	ok, _, _ := captureSetProp.Call(match, uintptr(unsafe.Pointer(c.property)), c.nextReader)
	if ok == 0 {
		return 0, 0, time.Time{}, errors.New("reader HWND property rejected")
	}
	binding := captureReaderBinding{id: c.nextReader, expires: time.Now().Add(5 * time.Second)}
	c.readers[match] = binding
	return match, binding.id, binding.expires, nil
}

func (c *captureZOrder) foregroundReader(ctx context.Context, hwnd, id uintptr, expires time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		if binding, ok := c.readers[hwnd]; ok && binding.id == id {
			captureRemoveProp.Call(hwnd, uintptr(unsafe.Pointer(c.property)))
			delete(c.readers, hwnd)
		}
	}()
	binding, bound := c.readers[hwnd]
	if c.stopped || ctx.Err() != nil || time.Now().After(expires) || !c.owns(hwnd) || !c.isReader(hwnd) || !bound || binding.id != id {
		return errors.New("reader foreground binding expired or changed")
	}
	// The exemption is a single explicit-action capability, not a permanent
	// reader privilege. Once this attempt completes, future background batches
	// contain this HWND like every other capture-owned window. If it remains the
	// actual foreground, the containment path already performs no write.
	// Sidecar performs readback only. Actual activation belongs exclusively to
	// the short-lived helper directly launched by foreground UI Chromium.
	ok := false
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil || !c.owns(hwnd) || !c.isReader(hwnd) {
			break
		}
		fg, _, _ := captureForeground.Call()
		iconic, _, _ := isCaptureIconic.Call(hwnd)
		visible, _, _ := procIsWindowVisible.Call(hwnd)
		// A maximized reader is still a valid visible foreground result. Only a
		// minimized/hidden reader fails this explicit-action readback.
		if fg == hwnd && iconic == 0 && visible != 0 {
			ok = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if c.logger != nil {
		c.logger.Printf("reader_foreground actor=broker verified=%t", ok)
	}
	if !ok {
		return errors.New("Windows did not foreground the native reader; select its window and retry")
	}
	return nil
}
