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
	validateAuthenticatedReceipt(t, true)
}

// Targeted receipts can validate admission without pretending to contain the
// full six-capture parity journey (for example when no continuation is offered).
func TestAuthenticatedCaptureReceiptObservationContract(t *testing.T) {
	validateAuthenticatedReceipt(t, false)
}

func validateAuthenticatedReceipt(t *testing.T, requireFullJourney bool) {
	t.Helper()
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
				WorkerExited    bool `json:"workerExitConfirmed"`
			} `json:"runtimeControl"`
		} `json:"execution"`
	}
	if json.Unmarshal(data, &report) != nil {
		t.Fatal("receipt JSON contract mismatch")
	}
	if len(report.Captures) == 0 {
		t.Fatal("receipt contains no observations")
	}
	if (report.Execution.WorkerMode != "packaged_worker" && report.Execution.WorkerMode != "production_quiet_driver_packaged_worker") || !report.Execution.RuntimeControl.Restored || !report.Execution.RuntimeControl.ProfileReleased || !report.Execution.RuntimeControl.WorkerExited {
		t.Fatal("official packaged-worker lifecycle proof missing")
	}
	seen := map[string]bool{}
	blocks := 0
	for _, capture := range report.Captures {
		if (capture.Source != "x" && capture.Source != "facebook") ||
			(capture.Kind != "feed" && capture.Kind != "followup" && capture.Kind != "target") {
			t.Fatal("receipt capture labels are outside the bounded QA contract")
		}
		if !capture.OK {
			t.Fatal("receipt contains a failed capture")
		}
		if string(capture.Result.Source) != capture.Source {
			t.Fatal("capture source mismatch")
		}
		expectedMode := "headless_worker"
		if report.Execution.WorkerMode == "production_quiet_driver_packaged_worker" {
			expectedMode = "browser_quiet_hidden"
		}
		if capture.Result.Coverage["captureMode"] != expectedMode {
			t.Fatal("capture driver mode mismatch")
		}
		if err := validateObservation(capture.Result); err != nil {
			t.Fatalf("%s %s observation contract rejected: %v", capture.Source, capture.Kind, err)
		}
		key := capture.Source + ":" + capture.Kind
		if seen[key] {
			t.Fatal("duplicate capture label in bounded receipt")
		}
		seen[key] = true
		for _, snapshot := range capture.Result.Snapshots {
			blocks += len(snapshot.Blocks)
		}
	}
	if requireFullJourney {
		for _, source := range []string{"x", "facebook"} {
			for _, kind := range []string{"feed", "followup", "target"} {
				if !seen[source+":"+kind] {
					t.Fatal("bounded source journey missing")
				}
			}
		}
	}
	t.Logf("Validated %d private observations / %d blocks without ingestion", len(report.Captures), blocks)
}
