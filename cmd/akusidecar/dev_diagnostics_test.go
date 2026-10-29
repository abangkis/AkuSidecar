package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Actual executables exercise the CLI ordering that component-only fixtures
// missed. All paths belong to a synthetic root; no normal startup is permitted.
func TestDevDiagnosticsIndependentOfOptionalCaptureComposition(t *testing.T) {
	images := os.Getenv("AKU_DEV_RUNTIME_TEST_IMAGES")
	if images == "" {
		t.Skip("requires the offline development-runtime helper")
	}
	for _, image := range []string{"default-diagnostic.exe", "dev-diagnostic.exe"} {
		for _, mode := range []string{"exclusive", "concurrent"} {
			t.Run(image+"/"+mode, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "config", "sidecar.json")
				capture := map[string]any{"xMode": mode, "profile": "standard", "visibility": "quiet", "maxAcquisitionRounds": 1}
				if mode == "concurrent" {
					capture["xReaderSource"] = "main"
				}
				cfg := map[string]any{
					"version": 1, "server": map[string]any{"host": "127.0.0.1", "port": 11122},
					"database": map[string]any{"path": "runtime/synthetic.db"},
					"capture":  capture, "preference": map[string]any{"mode": "guarded_live"},
					"reasoning": map[string]any{"provider": "deterministic", "timeoutMs": 5000,
						"planning": map[string]any{"model": "synthetic"}, "evaluation": map[string]any{"model": "synthetic"},
						"semanticEvent": map[string]any{"model": "synthetic"}, "aiDetection": map[string]any{"model": "synthetic"}},
				}
				raw, err := json.Marshal(cfg)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				db := filepath.Join(root, "override.db")
				base := []string{"--config", path, "--database", db, "--bridge-extension-origin", "chrome-extension://synthetic/"}
				for _, ordinary := range []bool{false, true} {
					for _, diagnostic := range []string{"--database-inspect", "--runtime-candidate-probe", "--native-derivation-hold-probe"} {
						args := append([]string(nil), base...)
						if ordinary {
							args = append(args, "--dev", "--app-shell", "--windows-capture-split", "--ui-chromium-path", filepath.Join(root, "missing-chrome.exe"))
						}
						args = append(args, diagnostic)
						out, err := exec.Command(filepath.Join(images, image), args...).CombinedOutput()
						if err != nil {
							t.Fatalf("diagnostic %v failed: %v %s", args, err, out)
						}
						var reply map[string]any
						if err = json.Unmarshal(out, &reply); err != nil {
							t.Fatalf("invalid diagnostic JSON: %s: %v", out, err)
						}
						want := "ok"
						if diagnostic == "--database-inspect" {
							want = "absent"
						}
						if reply["status"] != want {
							t.Fatalf("diagnostic status: %+v", reply)
						}
					}
				}
				if mode == "concurrent" {
					out, err := exec.Command(filepath.Join(images, image), base...).CombinedOutput()
					if err == nil || !strings.Contains(string(out), "concurrent X capture requires") {
						t.Fatalf("ordinary unsupported startup lost its guard: %v %s", err, out)
					}
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(raw) {
					t.Fatal("diagnostic changed configuration")
				}
				for _, absent := range []string{db, filepath.Join(root, "runtime"), filepath.Join(root, "missing-chrome.exe")} {
					if _, err := os.Stat(absent); !os.IsNotExist(err) {
						t.Fatalf("diagnostic created application state %s: %v", absent, err)
					}
				}
			})
		}
	}
}
