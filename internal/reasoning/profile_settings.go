package reasoning

import (
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// ActivateProviderProfileSet restores a previously persisted profile projection
// for providerName. It deliberately leaves legacy/missing projections alone;
// callers continue to run EnsureResolvableProfile so that fallback semantics
// remain unchanged.
func ActivateProviderProfileSet(settings *domain.Settings, providerName string, provider ProfileProvider) (bool, error) {
	if settings == nil {
		return false, fmt.Errorf("settings are nil")
	}
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		return false, fmt.Errorf("reasoning provider is empty")
	}
	if provider == nil {
		return false, fmt.Errorf("profile provider %q is nil", providerName)
	}

	profiles, found := settings.ReasoningProfilesByProvider[providerName]
	if !found {
		return false, nil
	}

	changed := profiles != settings.ActiveReasoningProfileSet()
	if changed {
		settings.ApplyReasoningProfileSet(profiles)
	}
	if settings.ReasoningProfilesByProvider == nil {
		settings.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{}
	}
	if current, ok := settings.ReasoningProfilesByProvider[providerName]; !ok || current != profiles {
		settings.ReasoningProfilesByProvider[providerName] = profiles
		changed = true
	}
	return changed, nil
}
