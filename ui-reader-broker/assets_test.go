package uireaderbroker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedBrokerKeepsIdentityAndNarrowsOrigin(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "broker")
	origin := "http://127.0.0.1:28173"
	if err := Materialize(dir, origin); err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	var original, generated map[string]any
	raw, _ := assets.ReadFile("manifest.json")
	json.Unmarshal(raw, &original)
	json.Unmarshal(read("manifest.json"), &generated)
	if original["key"] != generated["key"] {
		t.Fatal("extension identity changed")
	}
	permissions := generated["host_permissions"].([]any)
	if len(permissions) != 1 || permissions[0] != origin+"/*" {
		t.Fatal("origin widened")
	}
	worker := string(read("service-worker.js"))
	if strings.Contains(worker, "11122") || !strings.Contains(worker, origin) || !strings.Contains(worker, `"com.akubrowser.reader_activation"`) || !strings.Contains(worker, "!tab.active || !window.focused") {
		t.Fatal("origin or foreground/native contract changed")
	}
	if err := Materialize(dir, origin); err == nil {
		t.Fatal("existing assets overwritten")
	}
}

func TestIsolatedBrokerRejectsNoncanonicalOrigins(t *testing.T) {
	for _, origin := range []string{"https://127.0.0.1:28173", "http://localhost:28173", "http://example.com:28173", "http://127.0.0.1:1", "http://127.0.0.1:028173", "http://127.0.0.1:28173/", "http://127.0.0.1:28173?", "http://a@127.0.0.1:28173"} {
		dir := filepath.Join(t.TempDir(), "broker")
		if Materialize(dir, origin) == nil {
			t.Fatal("unsafe origin accepted", origin)
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("rejected origin wrote assets")
		}
	}
}
