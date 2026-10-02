package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Optional read-only contract check for an operator-owned private QA receipt.
// It never ingests observations or prints source identities/content.
func TestAuthenticatedParityReportObservationContract(t *testing.T) {
	path := os.Getenv("AKU_AUTHENTICATED_PARITY_REPORT")
	if path == "" {
		t.Skip("operator receipt not provided")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "build"))
	if err != nil {
		t.Fatal("build root unavailable")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal("receipt unavailable")
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		t.Fatal("receipt must stay inside project build")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 16*1024*1024 {
		t.Fatal("receipt shape unavailable")
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatal("receipt read failed")
	}
	var report struct {
		Captures []struct {
			Source string             `json:"source"`
			Kind   string             `json:"kind"`
			OK     bool               `json:"ok"`
			Result domain.Observation `json:"result"`
		} `json:"captures"`
		Execution struct {
			WorkerMode     string `json:"workerMode"`
			RuntimeControl struct {
				Restored        bool `json:"restored"`
				ProfileReleased bool `json:"profileReleasedBeforeRestore"`
			} `json:"runtimeControl"`
		} `json:"execution"`
	}
	if json.Unmarshal(data, &report) != nil {
		t.Fatal("receipt JSON contract mismatch")
	}
	if report.Execution.WorkerMode != "packaged_worker" || !report.Execution.RuntimeControl.Restored || !report.Execution.RuntimeControl.ProfileReleased {
		t.Fatal("official packaged-worker lifecycle proof missing")
	}
	seen := map[string]bool{}
	blocks := 0
	for _, capture := range report.Captures {
		if !capture.OK {
			t.Fatal("receipt contains a failed capture")
		}
		if string(capture.Result.Source) != capture.Source {
			t.Fatal("capture source mismatch")
		}
		if err := validateObservation(capture.Result); err != nil {
			t.Fatalf("%s %s observation contract rejected: %v", capture.Source, capture.Kind, err)
		}
		seen[capture.Source+":"+capture.Kind] = true
		for _, snapshot := range capture.Result.Snapshots {
			blocks += len(snapshot.Blocks)
		}
	}
	for _, source := range []string{"x", "facebook"} {
		for _, kind := range []string{"feed", "followup", "target"} {
			if !seen[source+":"+kind] {
				t.Fatal("bounded source journey missing")
			}
		}
	}
	t.Logf("Validated %d private observations / %d blocks without ingestion", len(report.Captures), blocks)
}
