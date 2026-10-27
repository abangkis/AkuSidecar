//go:build !windows

package appshell

import (
	"context"
	"errors"
)

func newMainBrowserProtocolLaunch(context.Context, MainBrowserTransportOptions) (captureProtocolLaunch, error) {
	return nil, errors.New("main browser transport is Windows-only")
}

func newCaptureProtocolLaunch() (captureProtocolLaunch, error) {
	return nil, errors.New("private capture CDP pipes are only available on Windows")
}
