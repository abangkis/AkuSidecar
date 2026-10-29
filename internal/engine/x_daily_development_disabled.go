//go:build !windows || !auth_session_dev

package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
)

const xDailyBuildSupported = false

func MainBrowserTransportOptions(cfg config.Config) (*appshell.MainBrowserTransportOptions, error) {
	if cfg.Capture.XUsesMainProfile() {
		return nil, errors.New("main profile unavailable in this build")
	}
	return nil, nil
}
func ConfigureXMainDailyRuntime(_ context.Context, e *Engine, _ *appshell.Window, _ *captureruntime.Manager, _, _, _ string) error {
	if e.config.Capture.XUsesMainProfile() {
		return errors.New("main profile unavailable in this build")
	}
	return nil
}

func ConfigureInstagramMainDailyRuntime(_ context.Context, e *Engine, _ *appshell.Window, _ *captureruntime.Manager, _, _, _ string) error {
	if e != nil && e.config.Capture.InstagramCaptureMode() == config.XCaptureConcurrent {
		e.instagramDaily.mu.Lock()
		e.instagramDaily.reason = "native-adapter-build-unavailable"
		e.instagramDaily.mu.Unlock()
	}
	return nil
}

func ConfigureXDailyNativeRuntime(_ context.Context, e *Engine, _, _ string) error {
	if e.XDailyConfigured() {
		return errors.New("daily X native owner unavailable in this build")
	}
	return nil
}
