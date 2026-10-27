//go:build !windows || !auth_session_dev

package engine

import (
	"context"
	"errors"
)

const xDailyBuildSupported = false

func ConfigureXDailyNativeRuntime(_ context.Context, e *Engine, _, _ string) error {
	if e.XDailyConfigured() {
		return errors.New("daily X native owner unavailable in this build")
	}
	return nil
}
