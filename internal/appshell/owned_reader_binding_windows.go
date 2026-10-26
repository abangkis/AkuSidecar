//go:build windows && auth_session_dev

package appshell

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

// OwnedReaderBinding borrows the caller's held Job and exact reader PID. It
// starts no monitor, adopts no process and never closes a reader window.
type OwnedReaderBinding struct{ containment *captureZOrder }

var ownedReaderMarker = regexp.MustCompile(`^AkuBrowser reader split_[a-f0-9]{32}$`)

func NewOwnedReaderBinding(job windows.Handle, pid uint32) (*OwnedReaderBinding, error) {
	if job == 0 || pid == 0 {
		return nil, errors.New("owned reader binding unavailable")
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return nil, errors.New("owned reader process unavailable")
	}
	defer windows.CloseHandle(process)
	if !processBelongsToJob(process, job) {
		return nil, errors.New("owned reader outside Job")
	}
	property, err := windows.UTF16PtrFromString("AkuBrowser.ExplicitReader." + fmt.Sprint(pid))
	if err != nil {
		return nil, errors.New("owned reader property unavailable")
	}
	return &OwnedReaderBinding{containment: &captureZOrder{job: job, root: pid, readerPID: pid, readers: map[uintptr]captureReaderBinding{}, property: property}}, nil
}

func (b *OwnedReaderBinding) Prepare(ctx context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
	if b == nil || b.containment == nil || ctx == nil || ctx.Err() != nil || !ownedReaderMarker.MatchString(marker) {
		return readerbroker.Target{}, nil, errors.New("owned reader marker rejected")
	}
	return b.containment.PrepareBrokerReader(ctx, marker)
}

// WaitForForeground observes one unique Job-owned marker without issuing a
// property/capability or writing foreground state. Prepare must revalidate it.
func (b *OwnedReaderBinding) WaitForForeground(ctx context.Context, marker string, ready func()) error {
	if b == nil || b.containment == nil || !ownedReaderMarker.MatchString(marker) {
		return errors.New("owned reader marker rejected")
	}
	c := b.containment
	return waitOwnedReaderForeground(ctx, func() (uintptr, bool, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		hwnd, err := c.findOwnedMarker(marker)
		if err != nil {
			return 0, false, err
		}
		foreground, _, _ := captureForeground.Call()
		return hwnd, hwnd != 0 && foreground == hwnd, nil
	}, ready)
}

func waitOwnedReaderForeground(ctx context.Context, probe func() (uintptr, bool, error), ready func()) error {
	if ctx == nil || probe == nil {
		return errors.New("owned reader wait unavailable")
	}
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	var selected uintptr
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		hwnd, foreground, err := probe()
		if err != nil {
			return errors.New("owned reader marker unavailable")
		}
		if selected != 0 && hwnd != selected {
			return errors.New("owned reader marker changed")
		}
		if hwnd != 0 && selected == 0 {
			selected = hwnd
			if ready != nil {
				ready()
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if selected != 0 && foreground {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// Release retires only this exact property's capability. User windows survive
// every failure, cancellation, expiration and completed activation.
func (b *OwnedReaderBinding) Release(target readerbroker.Target) {
	if b == nil || b.containment == nil {
		return
	}
	c := b.containment
	c.mu.Lock()
	defer c.mu.Unlock()
	hwnd := uintptr(target.HWND)
	binding, ok := c.readers[hwnd]
	if !ok || uint64(binding.id) != target.Value {
		return
	}
	if c.owns(hwnd) {
		value, _, _ := captureGetProp.Call(hwnd, uintptr(unsafe.Pointer(c.property)))
		if value == binding.id {
			captureRemoveProp.Call(hwnd, uintptr(unsafe.Pointer(c.property)))
		}
	}
	delete(c.readers, hwnd)
}
