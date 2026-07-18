//go:build darwin

package codexruntime

import (
	"os"
	"path/filepath"
)

func platformCandidates() []Candidate {
	home, _ := os.UserHomeDir()
	result := []Candidate{
		{Path: "/Applications/Codex.app/Contents/Resources/codex", Source: "codex-app-bundle"},
		{Path: "/opt/homebrew/bin/codex", Source: "codex-cli-homebrew"},
		{Path: "/usr/local/bin/codex", Source: "codex-cli-common"},
		{Path: "/usr/bin/codex", Source: "codex-cli-common"},
	}
	if home != "" {
		result = append(result,
			Candidate{Path: filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"), Source: "codex-app-bundle"},
			Candidate{Path: filepath.Join(home, ".local", "bin", "codex"), Source: "codex-cli-user"},
			Candidate{Path: filepath.Join(home, ".npm-global", "bin", "codex"), Source: "codex-cli-npm"},
		)
		managed, _ := filepath.Glob(filepath.Join(home, "Library", "Application Support", "OpenAI", "Codex", "bin", "*", "codex"))
		for _, path := range newestFirst(managed) {
			result = append(result, Candidate{Path: path, Source: "codex-app-managed"})
		}
	}
	return result
}
