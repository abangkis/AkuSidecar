package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestStoreRecordsTheRuntimeVersionThatLastOpenedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "aku-browser.db")
	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(filepath.Dir(path), RuntimeVersionMarkerName))
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != domain.ApplicationVersion+"\n" {
		t.Fatalf("runtime marker=%q", marker)
	}
}

func TestStoreRejectsDataWrittenByANewerRuntime(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "aku-browser.db")
	if err := os.WriteFile(filepath.Join(directory, RuntimeVersionMarkerName), []byte("99.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	var newer *NewerRuntimeDataError
	if !errors.As(err, &newer) {
		t.Fatalf("expected NewerRuntimeDataError, got %v", err)
	}
	if newer.DataVersion != "99.0.0" || newer.RuntimeVersion != domain.ApplicationVersion {
		t.Fatalf("unexpected incompatibility: %+v", newer)
	}
	if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("incompatible database was opened or created: %v", statErr)
	}
}

func TestRuntimeVersionComparisonRecognizesDowngradesAndPrereleases(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"0.8.0", "0.7.9", 1},
		{"0.8.0-preview.1", "0.8.0", -1},
		{"0.8.0", "0.8.0-preview.1", 1},
		{"0.8.0", "0.8.0", 0},
	}
	for _, test := range cases {
		got := compareRuntimeVersions(test.left, test.right)
		if got < 0 {
			got = -1
		} else if got > 0 {
			got = 1
		}
		if got != test.want {
			t.Fatalf("compareRuntimeVersions(%q,%q)=%d want %d", test.left, test.right, got, test.want)
		}
	}
}
