//go:build windows

package appshell

import (
	"errors"
	"golang.org/x/sys/windows"
	"strings"
)

// Borrowers inspect an independently owned main Job without adopting its lifetime.
func (w *Window) WithMainBrowserJob(profile, subprofile string, check func(windows.Handle) error) error {
	w.ownershipMu.Lock()
	defer w.ownershipMu.Unlock()
	if check == nil || w.owner.job == 0 || w.PID() <= 0 || !strings.EqualFold(profile, w.userDataDir) || subprofile == "" || subprofile != w.mainProfileDirectory {
		return errors.New("main browser scope unavailable")
	}
	if _, ok := w.captureProtocol.(MainBrowserProtocol); !ok {
		return errors.New("main browser transport unavailable")
	}
	select {
	case <-w.closed:
		return errors.New("main browser owner exited")
	default:
	}
	return check(w.owner.job)
}
