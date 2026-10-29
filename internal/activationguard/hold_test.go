package activationguard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, sources []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sidecar.exe"+FileSuffix)
	raw, _ := json.Marshal(state{Schema: Schema, ActivationID: strings.Repeat("a", 32), Sources: sources})
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestOrdinaryStartupDoesNotCreateHold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sidecar.exe"+FileSuffix)
	g, err := Open(path)
	if err != nil || g.Held("x") || g.Release("x") != nil {
		t.Fatal("ordinary startup changed")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("ordinary startup wrote maintenance state")
	}
}

func TestReleaseIsExplicitSourceScopedAndSurvivesReopen(t *testing.T) {
	path := fixture(t, []string{"x", "instagram"})
	g, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !g.Held("x") || !g.Held("instagram") {
		t.Fatal("hold not loaded")
	}
	if err = g.Release("x"); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil || reopened.Held("x") || !reopened.Held("instagram") {
		t.Fatal("source hold release lost across restart")
	}
	if err = reopened.Release("instagram"); err != nil {
		t.Fatal(err)
	}
	final, err := Open(path)
	if err != nil || final.Held("x") || final.Held("instagram") {
		t.Fatal("explicit release did not restore daily admission")
	}
}

func TestStaleOrMissingActivationCannotReleaseHold(t *testing.T) {
	for _, mode := range []string{"replaced", "missing"} {
		t.Run(mode, func(t *testing.T) {
			path := fixture(t, []string{"x"})
			g, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "missing" {
				err = os.Remove(path)
			} else {
				raw, _ := json.Marshal(state{Schema: Schema, ActivationID: strings.Repeat("b", 32), Sources: []string{"x", "instagram"}})
				err = os.WriteFile(path, raw, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if g.Release("x") == nil || !g.Held("x") {
				t.Fatal("stale activation released authority")
			}
		})
	}
}

func TestMalformedMaintenanceStateFailsClosed(t *testing.T) {
	for _, raw := range []string{
		`{}`, `{"schema":"aku.native-derivation-hold.v1","activationId":"bad","sources":["x"]}`,
		`{"schema":"aku.native-derivation-hold.v1","activationId":"` + strings.Repeat("a", 32) + `","sources":["facebook"]}`,
		`{"schema":"aku.native-derivation-hold.v1","activationId":"` + strings.Repeat("a", 32) + `","sources":["x","x"]}`,
		`{"schema":"aku.native-derivation-hold.v1","activationId":"` + strings.Repeat("a", 32) + `","sources":["x"],"sources":[]}`,
		`{"schema":"aku.native-derivation-hold.v1","activationId":"` + strings.Repeat("a", 32) + `","sources":null}`,
		strings.Repeat("x", 2049),
	} {
		path := filepath.Join(t.TempDir(), "hold.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(path); err == nil {
			t.Fatal("malformed hold accepted")
		}
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
}
