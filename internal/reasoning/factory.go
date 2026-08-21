package reasoning

import (
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func NewProvider(cfg config.Config) (Provider, error) {
	switch cfg.Reasoning.Provider {
	case "deterministic":
		return Deterministic{}, nil
	case "codex-app-server":
		return NewCodexAppServer(cfg)
	default:
		if config.IsOllamaProvider(cfg.Reasoning.Provider) {
			return NewOllama(cfg)
		}
		return nil, fmt.Errorf("unsupported reasoning provider %q", cfg.Reasoning.Provider)
	}
}

// effortRank orders the native effort vocabularies published by AkuSidecar
// providers so a catalog default can be chosen deterministically.
func effortRank(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "none", "off":
		return 0
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "xhigh":
		return 4
	case "max":
		return 5
	default:
		return -1
	}
}

// EnsureResolvableProfile returns the supplied profile ID when the provider
// catalog can resolve it. Unknown IDs fall back to the provider's high-effort
// profile, or its lowest-effort profile when high is unavailable. This keeps a
// provider switch from silently escalating automatic work to xhigh or max.
// Providers without a catalog return the input unchanged.
func EnsureResolvableProfile(provider Provider, id string) string {
	catalog, ok := provider.(ProfileProvider)
	if !ok {
		return id
	}
	if _, found := catalog.ResolveProfile(id); found {
		return id
	}
	options := catalog.ProfileOptions()
	for _, option := range options {
		if effortRank(option.Effort) == effortRank("high") {
			return option.ID
		}
	}
	lowest, lowestRank := "", int(^uint(0)>>1)
	for _, option := range options {
		if rank := effortRank(option.Effort); rank >= 0 && rank < lowestRank {
			lowest, lowestRank = option.ID, rank
		}
	}
	if lowest != "" {
		return lowest
	}
	if len(options) > 0 {
		return options[0].ID
	}
	return id
}
