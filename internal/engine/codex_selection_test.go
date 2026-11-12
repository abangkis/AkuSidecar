package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Only copied test executables opt into this synthetic, non-generative server.
// No installed Codex runtime, credentials, browser, or user Store is consulted.
func init() {
	if os.Getenv("AKU_CODEX_SELECTION_FIXTURE") != "1" || len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case "--version":
		fmt.Println("codex-cli 1.2.3")
		os.Exit(0)
	case "app-server":
		if len(os.Args) > 2 && os.Args[2] == "--help" {
			fmt.Println("Usage: codex app-server --listen stdio://")
			os.Exit(0)
		}
		scanner := bufio.NewScanner(os.Stdin)
		encoder := json.NewEncoder(os.Stdout)
		for scanner.Scan() {
			var message struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(scanner.Bytes(), &message) != nil {
				os.Exit(2)
			}
			if message.Method == "initialized" {
				continue
			}
			if message.Method != "initialize" {
				os.Exit(3)
			}
			if encoder.Encode(map[string]any{"id": message.ID, "result": map[string]any{"userAgent": "codex-selection-fixture"}}) != nil {
				os.Exit(4)
			}
		}
		os.Exit(0)
	}
}

func codexSelectionFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("AKU_CODEX_SELECTION_FIXTURE", "1")
	t.Setenv("AKU_CODEX_PATH", "")
	t.Setenv("LOCALAPPDATA", root)
	t.Setenv("APPDATA", filepath.Join(root, "empty-roaming"))
	t.Setenv("ProgramFiles", filepath.Join(root, "empty-programs"))
	path := filepath.Join(root, "OpenAI", "Codex", "bin", "current", "codex.exe")
	if runtime.GOOS != "windows" {
		path = filepath.Join(root, "bin", "codex")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(path))
	return path, filepath.Join(root, "OpenAI", "Codex", "bin", "d375f7df50d3b421", "codex.exe")
}

func TestCodexAutomaticReadinessAndProviderSwitchIgnoreStaleCache(t *testing.T) {
	for _, mode := range []string{"automatic", ""} {
		t.Run("mode="+mode, func(t *testing.T) {
			if mode == "" && runtime.GOOS != "windows" {
				t.Skip("Windows legacy path")
			}
			found, stale := codexSelectionFixture(t)
			engine, state := readinessEngine(t, map[string]config.ProviderConfig{
				"deterministic": {}, "codex-app-server": readinessProvider("", "codex-luna"),
			}, "deterministic")
			copySwapSchemas(t, engine.config.Root)
			before, _ := state.GetSettings(context.Background())
			before.ReasoningExecutablePath, before.ReasoningExecutableMode = stale, mode
			if err := state.SaveSettings(context.Background(), before); err != nil {
				t.Fatal(err)
			}
			providers, err := engine.ReasoningProviderReadiness(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var ready bool
			for _, provider := range providers {
				if provider.Name == "codex-app-server" {
					ready = provider.Available
					if !ready {
						t.Fatalf("readiness=%+v", provider)
					}
				}
			}
			if !ready {
				t.Fatal("Codex was not listed")
			}
			after, _ := state.GetSettings(context.Background())
			if !reflect.DeepEqual(before, after) || engine.ProviderName() != "deterministic" {
				t.Fatal("readiness mutated selection")
			}
			target := before
			target.ReasoningProvider = "codex-app-server"
			saved, err := engine.SaveSettings(context.Background(), target)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = engine.provider.(interface{ Close() error }).Close() })
			if engine.ProviderName() != "codex-app-server" || saved.ReasoningExecutableMode != "automatic" || saved.ReasoningExecutablePath != found {
				t.Fatalf("saved=%+v", saved)
			}
			stored, _ := state.GetSettings(context.Background())
			if stored.ReasoningExecutableMode != "automatic" || stored.ReasoningExecutablePath != found {
				t.Fatal("automatic discovery became an explicit pin")
			}
		})
	}
}

func TestCodexExplicitMissingPathsNeverFallBack(t *testing.T) {
	for _, source := range []string{"settings", "legacy_custom", "configuration", "environment"} {
		t.Run(source, func(t *testing.T) {
			_, stale := codexSelectionFixture(t)
			provider := readinessProvider("", "codex-luna")
			before := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
			before.ReasoningProvider = "deterministic"
			before.ReasoningExecutableMode, before.ReasoningExecutablePath = "automatic", stale
			switch source {
			case "settings":
				before.ReasoningExecutableMode = "explicit"
			case "legacy_custom":
				before.ReasoningExecutableMode, before.ReasoningExecutablePath = "", filepath.Join(t.TempDir(), "custom.exe")
			case "configuration":
				provider.Executable = stale
			case "environment":
				t.Setenv("AKU_CODEX_PATH", stale)
			}
			engine, state := readinessEngine(t, map[string]config.ProviderConfig{"deterministic": {}, "codex-app-server": provider}, "deterministic")
			copySwapSchemas(t, engine.config.Root)
			if err := state.SaveSettings(context.Background(), before); err != nil {
				t.Fatal(err)
			}
			target := before
			target.ReasoningProvider = "codex-app-server"
			if _, err := engine.SaveSettings(context.Background(), target); err == nil || !strings.Contains(err.Error(), "not available") {
				t.Fatalf("explicit override accepted: %v", err)
			}
			after, _ := state.GetSettings(context.Background())
			if !reflect.DeepEqual(before, after) || engine.ProviderName() != "deterministic" {
				t.Fatal("failed switch changed state")
			}
		})
	}
}

func TestCodexSelectionSavesExplicitAndAutomaticModes(t *testing.T) {
	found, stale := codexSelectionFixture(t)
	engine, state := readinessEngine(t, map[string]config.ProviderConfig{
		"deterministic": {}, "codex-app-server": readinessProvider("", "codex-luna"),
	}, "deterministic")
	copySwapSchemas(t, engine.config.Root)
	before, _ := state.GetSettings(context.Background())
	before.ReasoningExecutableMode, before.ReasoningExecutablePath = "automatic", stale
	if err := state.SaveSettings(context.Background(), before); err != nil {
		t.Fatal(err)
	}
	target := before
	target.ReasoningProvider = "codex-app-server"
	target.ReasoningExecutableMode, target.ReasoningExecutablePath = "explicit", found
	saved, err := engine.SaveSettings(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.provider.(interface{ Close() error }).Close() })
	if saved.ReasoningExecutableMode != "explicit" || saved.ReasoningExecutablePath != found {
		t.Fatalf("explicit switch=%+v", saved)
	}
	// Detect can choose automatic even if it resolves to the same executable.
	saved.ReasoningExecutableMode = "automatic"
	saved, err = engine.SaveSettings(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ReasoningExecutableMode != "automatic" || saved.ReasoningExecutablePath != found {
		t.Fatalf("automatic selection=%+v", saved)
	}
	saved.MaxScrolls++
	saved, err = engine.SaveSettings(context.Background(), saved)
	if err != nil || saved.ReasoningExecutableMode != "automatic" {
		t.Fatalf("unrelated save pinned discovery: mode=%q err=%v", saved.ReasoningExecutableMode, err)
	}
	// A new pin in a managed directory is still explicit and must fail closed.
	target = saved
	target.ReasoningExecutableMode, target.ReasoningExecutablePath = "explicit", stale
	if _, err := engine.SaveSettings(context.Background(), target); err == nil {
		t.Fatal("missing explicit managed path silently fell back")
	}
	stored, _ := state.GetSettings(context.Background())
	if !reflect.DeepEqual(saved, stored) {
		t.Fatal("rejected pin changed stored settings")
	}
}

func TestCodexSelectionDiscoveryPreservesExplicitPinUntilSave(t *testing.T) {
	for _, mode := range []string{"automatic", "explicit"} {
		t.Run(mode, func(t *testing.T) {
			engine, state, provider, before := executableChangeFixture(t)
			provider.name = "codex-app-server"
			before.ReasoningProvider, before.ReasoningExecutableMode = provider.name, mode
			if err := state.SaveSettings(context.Background(), before); err != nil {
				t.Fatal(err)
			}
			profile, err := engine.DiscoverReasoningExecutable(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if profile.SelectionMode != "automatic" {
				t.Fatalf("Detect preview mode=%s", profile.SelectionMode)
			}
			stored, _ := state.GetSettings(context.Background())
			if mode == "explicit" {
				if profile.AutoRepaired || len(provider.uses) != 0 || !reflect.DeepEqual(before, stored) {
					t.Fatal("Detect silently replaced an explicit pin")
				}
			} else if !profile.AutoRepaired || stored.ReasoningExecutableMode != "automatic" || stored.ReasoningExecutablePath != provider.resolved || len(provider.uses) != 1 {
				t.Fatal("automatic repair did not retain automatic mode")
			}
		})
	}
}
