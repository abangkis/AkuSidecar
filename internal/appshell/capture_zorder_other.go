//go:build !windows

package appshell

import "log"

func (*Window) StartCaptureContainment(*log.Logger) (CaptureContainment, error) { return nil, nil }
