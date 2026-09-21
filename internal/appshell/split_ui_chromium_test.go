package appshell

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSplitUIExecutableNeverFallsBackToCaptureBrowser(t *testing.T) {
	base := filepath.Join(t.TempDir(), "AkuSidecar.exe")
	expected := filepath.Join(filepath.Dir(base), "chromium", "bin", "chrome.exe")
	if got := splitUIExecutable(base, ""); got != expected {
		t.Fatalf("default=%s", got)
	}
	explicit := filepath.Join(t.TempDir(), "pinned", "bin", "chrome.exe")
	if got := splitUIExecutable(base, explicit); got != explicit {
		t.Fatal(got)
	}
	if runtime.GOOS != "windows" {
		if _, err := DiscoverSplitUI(context.Background(), base, explicit); err == nil {
			t.Fatal("UI override enabled outside Windows")
		}
	}
}

func TestSplitUIRequiresCfTIdentityPinSourceVersionAndChecksum(t *testing.T) {
	for _, mode := range []string{"valid", "branded", "version", "hash", "source", "missing_pin"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			os.Mkdir(filepath.Join(root, "bin"), 0700)
			exe := filepath.Join(root, "bin", "chrome.exe")
			data := []byte("fixture executable")
			os.WriteFile(exe, data, 0600)
			hash := sha256.Sum256(data)
			pin := map[string]any{"schemaVersion": 1, "component": "AkuBrowserAppShellChromium", "channel": "stable", "platform": "win64", "version": "152.0.7977.54", "sourceUrl": "https://storage.googleapis.com/chrome-for-testing-public/152.0.7977.54/win64/chrome-win64.zip", "executable": "bin/chrome.exe", "executableSha256": hex.EncodeToString(hash[:])}
			if mode == "hash" {
				pin["executableSha256"] = hex.EncodeToString(make([]byte, 32))
			}
			if mode == "source" {
				pin["sourceUrl"] = "https://example.test/chrome.zip"
			}
			if mode != "missing_pin" {
				b, _ := json.Marshal(pin)
				os.WriteFile(filepath.Join(root, "pin.json"), b, 0600)
			}
			result, err := validateSplitUI(context.Background(), exe, func(string) (string, string, error) {
				product, version := "Google Chrome for Testing", "152.0.7977.54"
				if mode == "branded" {
					product = "Google Chrome"
				}
				if mode == "version" {
					version = "153.0.0.0"
				}
				return version, product, nil
			})
			if mode == "valid" {
				if err != nil || result.Executable != exe {
					t.Fatal(result, err)
				}
			} else if err == nil {
				t.Fatalf("accepted %s", mode)
			}
		})
	}
}

func TestSplitUIRejectsUnpinnedExecutableLayout(t *testing.T) {
	called := false
	_, err := validateSplitUI(context.Background(), filepath.Join(t.TempDir(), "chrome.exe"), func(string) (string, string, error) { called = true; return "", "", nil })
	if err == nil || called {
		t.Fatal("unscoped executable reached probe")
	}
}
