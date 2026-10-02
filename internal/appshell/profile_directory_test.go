package appshell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProfileDirectory(t *testing.T) {
	for _, tc := range []struct{ name, state, directory, want string }{
		{"new", "", "", "Default"},
		{"authenticated secondary", `{"profile":{"last_used":"Profile 2"}}`, "Profile 2", "Profile 2"},
		{"implicit default", `{}`, "Default", "Default"},
		{"missing selected", `{"profile":{"last_used":"Profile 2"}}`, "Default", ""},
		{"invalid state", `{`, "Default", ""},
		{"escape", `{"profile":{"last_used":"../outside"}}`, "Default", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.directory != "" {
				if err := os.Mkdir(filepath.Join(root, tc.directory), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.state != "" {
				if err := os.WriteFile(filepath.Join(root, "Local State"), []byte(tc.state), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ResolveProfileDirectory(root)
			if tc.want == "" {
				if err == nil {
					t.Fatal("invalid selection accepted")
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
			if tc.state == "" {
				if _, err := os.Stat(filepath.Join(root, "Default")); !os.IsNotExist(err) {
					t.Fatal("resolver created profile data")
				}
			}
		})
	}
}
