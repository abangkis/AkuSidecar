package appshell

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitProfilesKeepAuthenticatedDataInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-profile")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "original-auth-marker")
	if err := os.WriteFile(marker, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	capture, ui, err := SplitProfilePaths(path + string(os.PathSeparator))
	if err != nil {
		t.Fatal(err)
	}
	if capture != path || ui != path+"-ui-split-cft" {
		t.Fatalf("profiles capture=%s ui=%s", capture, ui)
	}
	if _, err := os.Stat(ui); !os.IsNotExist(err) {
		t.Fatal("profile selection must not copy/create UI data")
	}
	if data, _ := os.ReadFile(marker); string(data) != "original" {
		t.Fatal("capture data modified")
	}
	if err := os.WriteFile(ui, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SplitProfilePaths(path); err == nil {
		t.Fatal("non-directory UI profile accepted")
	}
}

func TestSplitProfileRejectsAlias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app-profile")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(path, path+"-ui-split-cft"); err != nil {
		t.Skip("symlink creation unavailable")
	}
	if _, _, err := SplitProfilePaths(path); err == nil {
		t.Fatal("aliased authenticated profile accepted")
	}
}
