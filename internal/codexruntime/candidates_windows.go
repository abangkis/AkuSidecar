//go:build windows

package codexruntime

import (
	"os"
	"path/filepath"
)

func platformCandidates() []Candidate {
	result := make([]Candidate, 0, 12)
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		codexBin := filepath.Join(localAppData, "OpenAI", "Codex", "bin")
		managed, _ := filepath.Glob(filepath.Join(codexBin, "*", "codex.exe"))
		for _, path := range newestFirst(managed) {
			result = append(result, Candidate{Path: path, Source: "codex-app-managed", SelectionGroup: "codex-app"})
		}
		result = append(result, Candidate{Path: filepath.Join(codexBin, "codex.exe"), Source: "codex-app-root", SelectionGroup: "codex-app"})
	}
	if programFiles := os.Getenv("ProgramFiles"); programFiles != "" {
		packages, _ := filepath.Glob(filepath.Join(programFiles, "WindowsApps", "OpenAI.Codex_*", "app", "resources", "codex.exe"))
		for _, path := range newestFirst(packages) {
			result = append(result, Candidate{Path: path, Source: "codex-app-package", SelectionGroup: "codex-app"})
		}
	}
	if localAppData != "" {
		result = append(result, Candidate{Path: filepath.Join(localAppData, "Microsoft", "WindowsApps", "codex.exe"), Source: "windows-app-alias"})
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		npmRoot := filepath.Join(appData, "npm")
		for _, pattern := range []string{
			filepath.Join(npmRoot, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-*", "vendor", "*", "bin", "codex.exe"),
			filepath.Join(npmRoot, "node_modules", "@openai", "codex", "node_modules", "@openai", "codex-win32-*", "vendor", "*", "codex", "codex.exe"),
		} {
			native, _ := filepath.Glob(pattern)
			for _, path := range newestFirst(native) {
				result = append(result, Candidate{Path: path, Source: "codex-cli-npm-native"})
			}
		}
		result = append(result,
			Candidate{Path: filepath.Join(npmRoot, "codex.exe"), Source: "codex-cli-npm"},
			Candidate{Path: filepath.Join(npmRoot, "codex.cmd"), Source: "codex-cli-npm-shim"},
		)
	}
	return result
}
