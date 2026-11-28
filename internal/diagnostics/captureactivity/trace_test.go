package captureactivity

import (
	"bytes"
	"encoding/json"
	"log"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLaunchAndMeasuredTransitionProjection(t *testing.T) {
	var output bytes.Buffer
	logger := log.New(&output, "", 0)
	trace := NewLaunch(logger)
	exe := filepath.Join("private-profile", "chrome.exe")
	trace.Process("capture_started", 11, exe, "capture-version")
	trace.Process("ui_started", 22, exe, "ui-version")
	ok := false
	Transition(logger, "owner_release", 35*time.Millisecond, &ok)
	Transition(logger, "coordinator_wait", 0, nil)
	var records []record
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		payload, ok := strings.CutPrefix(line, "capture_activity ")
		var r record
		if !ok || json.Unmarshal([]byte(payload), &r) != nil {
			t.Fatal(line)
		}
		records = append(records, r)
	}
	if len(records) != 4 {
		t.Fatal(records)
	}
	capture, ui, release, wait := records[0], records[1], records[2], records[3]
	if capture.Operation != "browser_launch" || capture.Sequence != 1 || capture.PID != 11 || capture.Executable != "chrome.exe" ||
		ui.Sequence != 2 || ui.Phase != "ui_started" || ui.PID != 22 || ui.Version != "ui-version" || ui.ElapsedMS == nil {
		t.Fatal(output.String())
	}
	if release.Operation != "capture_transition" || release.OK == nil || *release.OK || release.ElapsedMS == nil || *release.ElapsedMS != 35 ||
		wait.OK != nil || wait.ElapsedMS == nil || *wait.ElapsedMS != 0 {
		t.Fatal(output.String())
	}
}
