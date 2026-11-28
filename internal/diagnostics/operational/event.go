// Package operational projects one bounded observation at an existing Sidecar log site.
package operational

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"strings"
	"time"

	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

type Logger interface{ Printf(string, ...any) }

// Fields are assembled by the event owner from measured values and closed catalogs.
// Never pass a raw request, error, configuration, URL or filesystem path.
type Fields map[string]any

type event struct {
	Operation d.Operation `json:"operation"`
	Phase     d.Phase     `json:"phase"`
	Outcome   string      `json:"outcome"`
	Reason    string      `json:"reason,omitempty"`
	ID        string      `json:"id,omitempty"`
	Detail    any         `json:"detail,omitempty"`
}

func (e event) write(logger Logger) {
	if sink, ok := logger.(*log.Logger); logger == nil || (ok && sink == nil) {
		return
	}
	if raw, err := json.Marshal(e); err == nil {
		logger.Printf("sidecar_activity %s", raw)
	}
}

func Report(logger Logger, operation d.Operation, phase d.Phase, outcome string, detail any) {
	event{Operation: operation, Phase: phase, Outcome: outcome, Detail: detail}.write(logger)
}

// Problem records the disposition chosen by the owner and a stable error category.
func Problem(logger Logger, operation d.Operation, phase d.Phase, outcome string, err error, id string) {
	entry := event{Operation: operation, Phase: phase, Outcome: outcome, Reason: category(err), ID: OpaqueID(id)}
	var guard StartupGuard
	if errors.As(err, &guard) && guard.guidance() != "" {
		entry.Reason = string(guard)
		entry.Detail = Fields{"hint": guard.guidance()}
	}
	entry.write(logger)
}

func category(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	case errors.Is(err, fs.ErrPermission):
		return "permission_denied"
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, sql.ErrNoRows):
		return "not_found"
	case errors.Is(err, fs.ErrExist):
		return "already_exists"
	case errors.Is(err, fs.ErrInvalid):
		return "invalid"
	default:
		return "unavailable"
	}
}

// OpaqueID permits only issued correlation IDs. The prefix is application-owned.
func OpaqueID(id string) string {
	split := strings.LastIndexByte(id, '_')
	if split < 1 {
		return ""
	}
	prefix, digits := id[:split], id[split+1:]
	if len(digits) != 32 || strings.Trim(digits, "0123456789abcdef") != "" {
		return ""
	}
	switch prefix {
	case "run", "session", "feedback", "item", "topic", "timeline", "job", "recapture", "lease", "assessment", "correction", "ai_feedback", "selection_correction", "topic_routing", "topic_understanding", "topic_activation", "media_provenance":
		return id
	}
	return ""
}

func Provider(name string) string {
	if strings.HasPrefix(name, "gemini-") {
		return "gemini"
	}
	if strings.HasPrefix(name, "ollama-") {
		return "ollama"
	}
	switch name {
	case "codex-app-server", "deterministic", "gemini", "groq", "ollama", "openrouter":
		return name
	default:
		return "other"
	}
}

func Milliseconds(value time.Duration) int64 {
	return d.Milliseconds(d.Observed[time.Duration]{Known: true, Value: value}, d.DurationPolicy{}).Value
}

func DiscoverySource(value string) string {
	switch value {
	case "explicit", "environment", "path", "discovery", "known", "registry", "bundled", "sidecar-relative", "workspace", "installed", "split-ui-pinned-cft":
		return value
	default:
		return "other"
	}
}
