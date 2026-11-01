package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// prepareReasoningExecutableChange validates a change against the active runtime.
// The caller holds operation and owns persistence and subsequent activation.
func (e *Engine) prepareReasoningExecutableChange(ctx context.Context, currentPath, requestedPath string) (reasoning.ExecutableRuntime, string, error) {
	if strings.TrimSpace(requestedPath) == strings.TrimSpace(currentPath) {
		return nil, "", nil
	}
	executableRuntime, ok := e.provider.(reasoning.ExecutableRuntime)
	if !ok {
		return nil, "", fmt.Errorf("%s does not expose an editable executable", e.ProviderName())
	}
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return nil, "", err
	}
	e.mu.RLock()
	activeWork := len(e.work.active) > 0
	e.mu.RUnlock()
	if active != nil || activeWork {
		return nil, "", errors.New("finish the active update or AI Deep Detection before changing the reasoning executable")
	}
	resolvedExecutable, err := executableRuntime.DiscoverExecutable(ctx, strings.TrimSpace(requestedPath))
	if err != nil {
		return nil, "", fmt.Errorf("validate reasoning executable: %w", err)
	}
	return executableRuntime, resolvedExecutable, nil
}
