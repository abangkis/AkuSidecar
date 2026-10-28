package engine

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
)

// XUsesOrdinaryBrowserActions selects the ordinary main-profile UI action path,
// independently of batch consent/readiness. Dedicated readers remain isolated.
func (e *Engine) XUsesOrdinaryBrowserActions() bool {
	return xDailyBuildSupported && e.config.Dev && e.config.WindowsCaptureSplit && e.config.Capture.XUsesMainProfile()
}

func (e *Engine) xMainBrowserActionOwner() (*captureruntime.Manager, *collection.Coordinator, error) {
	if !e.XUsesOrdinaryBrowserActions() {
		return nil, nil, errors.New("ordinary X Browser actions are not selected")
	}
	e.captureMu.Lock()
	owner := e.captureOwner
	e.captureMu.Unlock()
	runtime := e.collectionRuntime
	if runtime == nil {
		return nil, nil, errors.New("ordinary X Browser action runtime is unavailable")
	}
	if err := runtime.ValidatePersistentBrowser(owner); err != nil {
		return nil, nil, err
	}
	return owner, runtime, nil
}

func (e *Engine) BorrowXMainBrowserAction(ctx context.Context, nativeReader bool) (*captureruntime.Lease, func(), error) {
	owner, runtime, err := e.xMainBrowserActionOwner()
	if err != nil {
		return nil, nil, err
	}
	return runtime.BorrowPersistentBrowser(ctx, owner, nativeReader)
}
