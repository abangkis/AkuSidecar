package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
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
	var executableRuntime reasoning.ExecutableRuntime
	var resolvedExecutable string
	if strings.TrimSpace(value.ReasoningExecutablePath) != strings.TrimSpace(current.ReasoningExecutablePath) {
		var ok bool
		executableRuntime, ok = e.provider.(reasoning.ExecutableRuntime)
		if !ok {
			return domain.Settings{}, fmt.Errorf("%s does not expose an editable executable", e.ProviderName())
		}
		active, activeErr := e.store.ActiveSession(ctx)
		if activeErr != nil {
			return domain.Settings{}, activeErr
		}
		e.mu.RLock()
		activeWork := len(e.work.active) > 0
		e.mu.RUnlock()
		if active != nil || activeWork {
			return domain.Settings{}, errors.New("finish the active update or AI Deep Detection before changing the reasoning executable")
		}
		resolvedExecutable, err = executableRuntime.DiscoverExecutable(ctx, strings.TrimSpace(value.ReasoningExecutablePath))
		if err != nil {
			return domain.Settings{}, fmt.Errorf("validate reasoning executable: %w", err)
		}
		value.ReasoningExecutablePath = resolvedExecutable
	}
	// Keep the currently running provider's projection before accepting a
	// provider switch. The next runtime can restore the target provider's
	// projection without reusing (or overwriting) this one.
	profilesByProvider := make(map[string]domain.ReasoningProfileSet, len(current.ReasoningProfilesByProvider)+len(value.ReasoningProfilesByProvider)+1)
	for provider, profiles := range current.ReasoningProfilesByProvider {
		profilesByProvider[provider] = profiles
	}
	for provider, profiles := range value.ReasoningProfilesByProvider {
		profilesByProvider[provider] = profiles
	}
	if current.ReasoningProvider != "" {
		profilesByProvider[current.ReasoningProvider] = current.ActiveReasoningProfileSet()
	}
	if value.ReasoningProvider == current.ReasoningProvider && value.ReasoningProvider != "" {
		profilesByProvider[value.ReasoningProvider] = value.ActiveReasoningProfileSet()
	}
	value.ReasoningProfilesByProvider = profilesByProvider
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
