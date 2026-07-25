//go:build !windows

package reasoning

import "os/exec"

// processOwnership is a no-op on platforms where the Windows Job Object is
// unavailable. The App Server is still terminated through exec.Cmd there.
type processOwnership struct{}

func newProcessOwnership() (processOwnership, error) { return processOwnership{}, nil }

func (processOwnership) attach(_ *exec.Cmd) error { return nil }

func (processOwnership) terminate() {}

func (processOwnership) close() {}
