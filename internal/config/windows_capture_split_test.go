package config

import "testing"

func TestWindowsCaptureSplitRequiresExplicitWindowsAppShell(t *testing.T) {
	for _, platform := range []string{"windows", "darwin", "linux"} {
		for _, shell := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				got := windowsCaptureSplitEnabled(platform, Options{AppShell: shell, ExperimentalWindowsCaptureSplit: enabled})
				if got != (platform == "windows" && shell && enabled) {
					t.Fatalf("unexpected platform gate: %s shell=%t flag=%t got=%t", platform, shell, enabled, got)
				}
			}
		}
	}
}
