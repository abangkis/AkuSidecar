package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// providerSwapPlan is the fully constructed replacement runtime for a
// reasoning provider switch. Construction happens before persistence so a
// bad target fails the settings save; apply happens after persistence under
// the caller's operation lock.
type providerSwapPlan struct {
	target        string
	candidate     reasoning.Provider
	cfgCopy       config.Config
	eventResolver semanticengine.Resolver
	aiResolver    aidetector.Resolver
}

// closeCandidate releases a replacement that was constructed successfully but
// could not be committed. This matters for providers that own client pools or
// subprocess lifecycle even before their first invocation.
func (plan *providerSwapPlan) closeCandidate() {
	if plan == nil || plan.candidate == nil {
		return
	}
	if closer, ok := plan.candidate.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

// planProviderSwap constructs the replacement provider for target without
// touching the active runtime. It mirrors the startup sequence in
// cmd/akusidecar/main.go: executable projection for Codex, provider
// construction, remembered-profile activation, and per-slot profile migration.
// Settings are migrated in place so the caller persists the migrated values.
func (e *Engine) planProviderSwap(ctx context.Context, target string, settings *domain.Settings) (*providerSwapPlan, error) {
	target = strings.TrimSpace(target)
	if target == "" || target == e.ProviderName() {
		return nil, nil
	}
	cfgCopy := e.config
	if err := cfgCopy.Reasoning.Select(target); err != nil {
		return nil, fmt.Errorf("select reasoning provider %q: %w", target, err)
	}
	if target == "codex-app-server" && strings.TrimSpace(cfgCopy.Reasoning.Executable) == "" {
		if path := strings.TrimSpace(settings.ReasoningExecutablePath); path != "" {
			cfgCopy.Reasoning.Executable = path
		}
	}
	candidate, err := reasoning.NewProvider(cfgCopy)
	if err != nil {
		return nil, fmt.Errorf("construct reasoning provider %q: %w", target, err)
	}
	if profileProvider, ok := candidate.(reasoning.ProfileProvider); ok {
		if _, err := reasoning.ActivateProviderProfileSet(settings, target, profileProvider); err != nil {
			return nil, err
		}
		migrated := false
		for _, current := range []*string{
			&settings.ReasoningAcquisitionProfile,
			&settings.ReasoningEvaluationProfile,
			&settings.ReasoningSemanticProfile,
			&settings.ReasoningAIDeepProfile,
		} {
			if replacement := reasoning.EnsureResolvableProfile(candidate, *current); replacement != *current {
				*current = replacement
				migrated = true
			}
		}
		settings.RememberReasoningProfileSet(target)
		if migrated {
			e.logger.Printf("migrated reasoning profiles for provider %s", candidate.Name())
		}
	}
	plan := &providerSwapPlan{target: target, candidate: candidate, cfgCopy: cfgCopy}
	if structured, ok := candidate.(reasoning.StructuredInvoker); ok {
		plan.eventResolver, err = semanticengine.NewStructuredResolver(cfgCopy.Root, structured, cfgCopy.Reasoning.SemanticEvent)
		if err != nil {
			return nil, fmt.Errorf("construct semantic resolver for %q: %w", target, err)
		}
		plan.aiResolver, err = aidetector.NewStructuredResolver(cfgCopy.Root, structured, cfgCopy.Reasoning.AIDetection)
		if err != nil {
			return nil, fmt.Errorf("construct AI Deep resolver for %q: %w", target, err)
		}
	}
	return plan, nil
}

// apply swaps the active runtime under a held operation lock. The caller has
// already persisted settings; this only mutates process state.
func (plan *providerSwapPlan) apply(e *Engine) {
	previous := e.ProviderName()
	if closer, ok := e.provider.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			e.logger.Printf("previous reasoning provider %s shutdown failed: %v", previous, err)
		}
	}
	e.provider = plan.candidate
	e.config.Reasoning.Select(plan.target)
	if executableRuntime, ok := plan.candidate.(reasoning.ExecutableRuntime); ok {
		if settings, err := e.store.GetSettings(context.Background()); err == nil {
			resolved := executableRuntime.ExecutablePath()
			if resolved != "" && settings.ReasoningExecutablePath != resolved {
				settings.ReasoningExecutablePath = resolved
				if err := e.store.SaveSettings(context.Background(), settings); err != nil {
					e.logger.Printf("could not persist swapped provider executable path: %v", err)
				}
			}
		}
	}
	if e.events != nil {
		e.events.SetResolver(plan.eventResolver)
	}
	e.aiDeep = plan.aiResolver
	e.logger.Printf("reasoning provider switched %s -> %s", previous, plan.candidate.Name())
	select {
	case e.autoWake <- struct{}{}:
	default:
	}
}

// swapProviderPlan applies a prepared plan under the operation lock while
// keeping the engine idle-check contract in one place.
func (e *Engine) ensureIdleForProviderSwap(ctx context.Context) error {
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return err
	}
	if active != nil {
		return fmt.Errorf("finish the active update before switching the reasoning provider")
	}
	e.mu.RLock()
	activeWork := len(e.active) > 0
	pendingDeep := len(e.pending) > 0
	e.mu.RUnlock()
	if activeWork || pendingDeep {
		return fmt.Errorf("finish active reasoning work before switching the reasoning provider")
	}
	if e.shuttingDown {
		return fmt.Errorf("reasoning provider cannot switch while shutting down")
	}
	return nil
}
