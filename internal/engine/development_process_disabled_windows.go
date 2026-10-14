//go:build windows && auth_session_fixture && !auth_session_dev

package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
)

func bindProcessSession(_ context.Context, e *Engine, _ *captureruntime.Manager, _ *collection.Coordinator, _ processFixtureStart, authority, browser func(context.Context) (SyntheticSessionEvidence, error), development bool) error {
	if development {
		return errors.New("development adapter unavailable")
	}
	return e.BindSyntheticExclusiveWithBrowserProbe("alpha", authority, browser)
}

func validateDevelopmentController(processFixtureStart) error {
	return errors.New("development adapter unavailable")
}
