package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) SaveSettings(ctx context.Context, value domain.Settings) (domain.Settings, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	value.Normalize()
	if err := e.validateDevelopmentSelection(value); err != nil {
		return domain.Settings{}, err
	}
	if e.collectionRuntime != nil {
		if err := e.collectionRuntime.ValidateSelection(value.CollectionMode, value.ActiveSources); err != nil {
			return domain.Settings{}, err
		}
	} else if value.CollectionMode == "headless" {
		return domain.Settings{}, errors.New("headless requires the managed Windows split capture runtime")
	}
	current, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	if target := strings.TrimSpace(value.ReasoningProvider); target != "" && target != current.ReasoningProvider && len(e.config.Reasoning.Providers) > 0 {
		provider, selectable := e.reasoningProviderConfiguration(target)
		if !selectable {
			return domain.Settings{}, fmt.Errorf("reasoning provider %q is not selectable", target)
		}
		if !provider.Configured {
			return domain.Settings{}, fmt.Errorf("%s is not configured: set %s before selecting it", provider.Label, provider.CredentialName)
		}
		if err := e.ensureReasoningProviderReady(ctx, target); err != nil {
			return domain.Settings{}, err
		}
	}
	executableRuntime, resolvedExecutable, err := e.prepareReasoningExecutableChange(ctx, current.ReasoningExecutablePath, value.ReasoningExecutablePath)
	if err != nil {
		return domain.Settings{}, err
	}
	if executableRuntime != nil {
		value.ReasoningExecutablePath = resolvedExecutable
	}
	value.ReasoningProfilesByProvider = mergeReasoningProfilesByProvider(current, value)
	value.Normalize()
	var swap *providerSwapPlan
	if strings.TrimSpace(value.ReasoningProvider) != current.ReasoningProvider {
		if err := e.ensureIdleForProviderSwap(ctx); err != nil {
			return domain.Settings{}, err
		}
		swap, err = e.planProviderSwap(ctx, value.ReasoningProvider, &value)
		if err != nil {
			return domain.Settings{}, err
		}
	}
	defer func() {
		if swap != nil {
			swap.closeCandidate()
		}
	}()
	if swap != nil {
		if err := validateReasoningProfilesAgainst(swap.candidate, value); err != nil {
			return domain.Settings{}, err
		}
	} else if err := e.validateReasoningProfiles(value); err != nil {
		return domain.Settings{}, err
	}
	if err := e.store.SaveSettings(ctx, value); err != nil {
		return domain.Settings{}, err
	}
	if e.collectionRuntime != nil {
		e.collectionRuntime.Request(value.CollectionMode)
	}
	if current.AIDetectionEnabled && !value.AIDetectionEnabled {
		e.cancelDeepDetections()
	}
	if executableRuntime != nil {
		executableRuntime.UseExecutable(resolvedExecutable)
	}
	if swap != nil {
		swap.apply(e)
		swap = nil
	}
	saved, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	if _, err := e.store.EnforceRetention(ctx, saved); err != nil {
		return domain.Settings{}, fmt.Errorf("apply retention settings: %w", err)
	}
	e.scheduler.notify()
	return saved, nil
}
