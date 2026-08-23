//go:build windows

package codexruntime

import (
	"path/filepath"
	"testing"
)

func TestIsManagedPathRecognizesCodexAppLocationsOnly(t *testing.T) {
	localAppData := t.TempDir()
	t.Setenv("LOCALAPPDATA", localAppData)
	managedRoot := filepath.Join(localAppData, "OpenAI", "Codex", "bin", "codex.exe")
	if !IsManagedPath(managedRoot) {
		t.Fatalf("managed root path was not recognized: %q", managedRoot)
	}
	arbitrary := filepath.Join(localAppData, "custom", "codex.exe")
	if IsManagedPath(arbitrary) {
		t.Fatalf("custom path was treated as managed: %q", arbitrary)
	}
}
