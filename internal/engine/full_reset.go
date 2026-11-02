package engine

import (
	"context"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (e *Engine) FullReset(ctx context.Context) (store.FullResetResult, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	e.cancelDeepDetections()
	defaults := domain.DefaultSettings(e.config.Capture.Profile, e.config.Capture.Visibility, e.config.Preference.Mode, e.config.Capture.OpenMissingSource)
	target := strings.TrimSpace(e.config.Reasoning.ActiveProvider)
	if target == "" {
		target = e.ProviderName()
	}
	defaults.ReasoningProvider = target
	defaults.ReasoningExecutablePath = e.ReasoningRuntime().ExecutablePath
	var swap *providerSwapPlan
	var err error
	if target != e.ProviderName() && len(e.config.Reasoning.Providers) > 0 {
		if err := e.ensureIdleForProviderSwap(ctx); err != nil {
			return store.FullResetResult{}, err
		}
		swap, err = e.planProviderSwap(ctx, target, &defaults)
		if err != nil {
			return store.FullResetResult{}, err
		}
	}
	defer func() {
		if swap != nil {
			swap.closeCandidate()
		}
	}()
	result, err := e.store.FullReset(ctx, defaults)
	if err != nil {
		return store.FullResetResult{}, err
	}
	if e.collectionRuntime != nil {
		e.collectionRuntime.Request("browser")
	}
	e.mu.Lock()
	e.headlessAccess = nil
	e.mu.Unlock()
	if swap != nil {
		swap.apply(e)
		swap = nil
	}
	return result, nil
}
