//go:build !windows

package appshell

import (
	"errors"
)

func newCaptureProtocolLaunch() (captureProtocolLaunch, error) {
	return nil, errors.New("private capture CDP pipes are only available on Windows")
}
