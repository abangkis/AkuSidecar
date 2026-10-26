//go:build windows && auth_session_dev

package appshell

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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
