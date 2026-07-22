//go:build darwin

package codexruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlatformCandidatesIncludeCodexAndChatGPTAppBundles(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	candidates := platformCandidates()
	want := []string{
		filepath.Join("/Applications", "Codex.app", "Contents", "Resources", "codex"),
		filepath.Join("/Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
		filepath.Join(home, "Applications", "Codex.app", "Contents", "Resources", "codex"),
		filepath.Join(home, "Applications", "ChatGPT.app", "Contents", "Resources", "codex"),
	}

	for _, path := range want {
		found := false
		for _, candidate := range candidates {
			if candidate.Path == path && candidate.Source == "codex-app-bundle" && candidate.SelectionGroup == "codex-app" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("candidate list does not include %q", path)
		}
	}
}
