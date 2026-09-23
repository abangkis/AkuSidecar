package config

import "testing"

func TestWindowsCaptureSplitRequiresExplicitWindowsAppShell(t *testing.T) {
	for _, platform := range []string{"windows", "darwin", "linux"} {
		for _, shell := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				for _, legacy := range []bool{false, true} {
					got := windowsCaptureSplitEnabled(platform, Options{AppShell: shell, WindowsCaptureSplit: enabled, ExperimentalWindowsCaptureSplit: legacy})
					if got != (platform == "windows" && shell && (enabled || legacy)) {
						t.Fatalf("unexpected platform gate: %s shell=%t flag=%t legacy=%t got=%t", platform, shell, enabled, legacy, got)
					}
				}
			}
		}
	}
}
