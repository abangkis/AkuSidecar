//go:build darwin

package codexruntime

import (
	"os"
	"path/filepath"
)

func platformCandidates() []Candidate {
	home, _ := os.UserHomeDir()
	result := make([]Candidate, 0, 16)
	for _, appName := range []string{"Codex.app", "ChatGPT.app"} {
		result = append(result, Candidate{
			Path:           filepath.Join("/Applications", appName, "Contents", "Resources", "codex"),
			Source:         "codex-app-bundle",
			SelectionGroup: "codex-app",
		})
	}
	if home != "" {
		for _, appName := range []string{"Codex.app", "ChatGPT.app"} {
			result = append(result, Candidate{
				Path:           filepath.Join(home, "Applications", appName, "Contents", "Resources", "codex"),
				Source:         "codex-app-bundle",
				SelectionGroup: "codex-app",
			})
		}
		managed, _ := filepath.Glob(filepath.Join(home, "Library", "Application Support", "OpenAI", "Codex", "bin", "*", "codex"))
		for _, path := range newestFirst(managed) {
			result = append(result, Candidate{Path: path, Source: "codex-app-managed", SelectionGroup: "codex-app"})
		}
	}
	result = append(result,
		Candidate{Path: "/opt/homebrew/bin/codex", Source: "codex-cli-homebrew"},
		Candidate{Path: "/usr/local/bin/codex", Source: "codex-cli-common"},
		Candidate{Path: "/usr/bin/codex", Source: "codex-cli-common"},
	)
	if home != "" {
		result = append(result,
			Candidate{Path: filepath.Join(home, ".local", "bin", "codex"), Source: "codex-cli-user"},
			Candidate{Path: filepath.Join(home, ".npm-global", "bin", "codex"), Source: "codex-cli-npm"},
		)
	}
	return result
}
