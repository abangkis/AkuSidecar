// Package codexselection owns AkuSidecar's executable selection policy.
// Candidate discovery and validation belong to LibraryGo.
package codexselection

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	Automatic = "automatic"
	Explicit  = "explicit"
)

// Mode upgrades legacy settings without requiring the old executable to exist.
// Only known app-managed layouts are treated as cached automatic discoveries;
// arbitrary legacy paths remain pinned. An explicit stored mode always wins.
func Mode(settings domain.Settings) string {
	if settings.ReasoningExecutableMode != "" {
		return settings.ReasoningExecutableMode
	}
	path := strings.TrimSpace(settings.ReasoningExecutablePath)
	if path == "" || legacyManagedPath(path) {
		return Automatic
	}
	return Explicit
}

// Request gives configured overrides precedence over the persisted selection.
// An automatic path is display-only cache, never an executable override.
func Request(root, configured string, settings domain.Settings) string {
	requested := strings.TrimSpace(configured)
	if requested == "" {
		requested = strings.TrimSpace(os.Getenv("AKU_CODEX_PATH"))
	}
	if requested == "" && Mode(settings) == Explicit {
		requested = strings.TrimSpace(settings.ReasoningExecutablePath)
	}
	if requested != "" && strings.ContainsAny(requested, `\/`) && !filepath.IsAbs(requested) {
		requested = filepath.Join(root, requested)
	}
	return requested
}

// legacyManagedPath is a one-time compatibility rule for paths written before
// selection mode existed. It does not infer user intent for new explicit paths.
func legacyManagedPath(path string) bool {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		if base := os.Getenv("LOCALAPPDATA"); base != "" {
			if matchesLayout(path, filepath.Join(base, "OpenAI", "Codex", "bin"), "codex.exe", true) {
				return true
			}
		}
		if base := os.Getenv("ProgramFiles"); base != "" {
			rel, err := filepath.Rel(filepath.Join(base, "WindowsApps"), path)
			parts := strings.Split(strings.ToLower(rel), string(filepath.Separator))
			if err == nil && len(parts) == 4 && strings.HasPrefix(parts[0], "openai.codex_") && parts[1] == "app" && parts[2] == "resources" && parts[3] == "codex.exe" {
				return true
			}
		}
	}
	if runtime.GOOS == "darwin" {
		for _, base := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
			for _, app := range []string{"Codex.app", "ChatGPT.app"} {
				if path == filepath.Join(base, app, "Contents", "Resources", "codex") {
					return true
				}
			}
		}
		if home, err := os.UserHomeDir(); err == nil {
			return matchesLayout(path, filepath.Join(home, "Library", "Application Support", "OpenAI", "Codex", "bin"), "codex", true)
		}
	}
	return false
}

func matchesLayout(path, base, executable string, versioned bool) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.ToLower(rel), string(filepath.Separator))
	return len(parts) == 1 && parts[0] == executable || versioned && len(parts) == 2 && parts[0] != ".." && parts[1] == executable
}
