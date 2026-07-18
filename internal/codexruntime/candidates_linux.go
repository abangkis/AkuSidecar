//go:build linux

package codexruntime

import (
	"os"
	"path/filepath"
)

func platformCandidates() []Candidate {
	result := []Candidate{
		{Path: "/usr/local/bin/codex", Source: "codex-cli-common"},
		{Path: "/usr/bin/codex", Source: "codex-cli-common"},
		{Path: "/snap/bin/codex", Source: "codex-cli-snap"},
	}
	if home, err := os.UserHomeDir(); err == nil {
		result = append(result,
			Candidate{Path: filepath.Join(home, ".local", "bin", "codex"), Source: "codex-cli-user"},
			Candidate{Path: filepath.Join(home, ".npm-global", "bin", "codex"), Source: "codex-cli-npm"},
		)
	}
	return result
}
