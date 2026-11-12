package codexselection

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestRequestPreservesOverridesAndAutomaticCache(t *testing.T) {
	t.Setenv("AKU_CODEX_PATH", "")
	root := t.TempDir()
	for _, tc := range []struct {
		name, mode, cached, configured, environment, want string
	}{
		{"automatic ignores missing cache", Automatic, "deleted.exe", "", "", ""},
		{"explicit stays pinned", Explicit, "chosen.exe", "", "", "chosen.exe"},
		{"legacy custom stays pinned", "", "custom.exe", "", "", "custom.exe"},
		{"configuration beats stored path", Explicit, "chosen.exe", "config.exe", "env.exe", "config.exe"},
		{"environment beats automatic cache", Automatic, "deleted.exe", "", "env.exe", "env.exe"},
		{"environment beats stored path", Explicit, "chosen.exe", "", "env.exe", "env.exe"},
		{"relative explicit uses project root", Explicit, filepath.Join("tools", "codex.exe"), "", "", filepath.Join(root, "tools", "codex.exe")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("AKU_CODEX_PATH", tc.environment)
			settings := domain.Settings{ReasoningExecutableMode: tc.mode, ReasoningExecutablePath: tc.cached}
			if got := Request(root, tc.configured, settings); got != tc.want {
				t.Fatalf("request=%q want=%q", got, tc.want)
			}
		})
	}
}

func TestLegacyManagedWindowsPathsDoNotNeedToExist(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows legacy settings")
	}
	local, program := t.TempDir(), t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("ProgramFiles", program)
	for _, tc := range []struct{ path, want string }{
		{filepath.Join(local, "OpenAI", "Codex", "bin", "d375f7df50d3b421", "codex.exe"), Automatic},
		{filepath.Join(program, "WindowsApps", "OpenAI.Codex_old", "app", "resources", "codex.exe"), Automatic},
		{filepath.Join(local, "OpenAI", "Codex", "bin-other", "old", "codex.exe"), Explicit},
		{filepath.Join(local, "OpenAI", "Codex", "bin", "..", "custom", "codex.exe"), Explicit},
		{filepath.Join(local, "custom", "codex.exe"), Explicit},
	} {
		settings := domain.Settings{ReasoningExecutablePath: tc.path}
		if got := Mode(settings); got != tc.want {
			t.Fatalf("mode(%q)=%q want=%q", tc.path, got, tc.want)
		}
		settings.ReasoningExecutableMode = Explicit
		if got := Mode(settings); got != Explicit {
			t.Fatalf("explicit managed path lost its pin: %s", got)
		}
	}
}
