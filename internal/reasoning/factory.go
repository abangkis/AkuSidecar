package reasoning

import (
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func NewProvider(cfg config.Config) (Provider, error) {
	switch cfg.Reasoning.Provider {
	case "deterministic":
		return Deterministic{}, nil
	case "codex-app-server":
		return NewCodexAppServer(cfg)
	default:
		return nil, fmt.Errorf("unsupported reasoning provider %q", cfg.Reasoning.Provider)
	}
}
