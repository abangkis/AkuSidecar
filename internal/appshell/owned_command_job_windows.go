//go:build windows

package appshell

import (
	"errors"

	"golang.org/x/sys/windows"
)

// WithOwnedJob permits trusted in-process ownership inspection or termination
// while the exact Job handle is held. The callback must not retain it or reenter.
func (p *OwnedCommand) WithOwnedJob(check func(windows.Handle) error) error {
	if p == nil || check == nil {
		return errors.New("owned worker Job unavailable")
	}
	p.ownershipMu.Lock()
	defer p.ownershipMu.Unlock()
	if p.owner.job == 0 {
		return errors.New("owned worker Job closed")
	}
	return check(p.owner.job)
}
