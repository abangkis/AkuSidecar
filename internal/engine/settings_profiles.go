package engine

import "github.com/abangkis/AkuSidecar/internal/domain"

// mergeReasoningProfilesByProvider returns an independent snapshot while preserving
// the active provider projection before a switch or same-provider update.
func mergeReasoningProfilesByProvider(current, value domain.Settings) map[string]domain.ReasoningProfileSet {
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
	return profilesByProvider
}
