//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func xQAPlanTest(t *testing.T) string {
	t.Helper()
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("canonical QA entry helper required")
	}
	path, err := WriteXApplicationQASyntheticPlan(t.TempDir(), node, 28171)
	if err != nil {
		t.Fatal(err)
	}
	return path
}
func xQAEditPlan(t *testing.T, path string, change func(*XApplicationQAPlan)) XApplicationQAPlan {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var p XApplicationQAPlan
	if json.Unmarshal(raw, &p) != nil {
		t.Fatal("plan read failed")
	}
	change(&p)
	raw, _ = json.Marshal(p)
	if os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("plan edit failed")
	}
	return p
}
func TestXQAPlanRejectsBeforeDatabaseOpen(t *testing.T) {
	for _, scenario := range []string{"root", "state", "reader", "collector", "profile-outside", "db-outside", "asset", "port", "marker", "hardlink", "wal", "shm", "journal", "version-alias", "version-newer"} {
		t.Run(scenario, func(t *testing.T) {
			path := xQAPlanTest(t)
			p := xQAEditPlan(t, path, func(p *XApplicationQAPlan) {
				switch scenario {
				case "root":
					p.RootID = "wrong"
				case "state":
					p.StateID = "wrong"
				case "reader":
					p.Reader.DirectoryID = "wrong"
				case "collector":
					p.Collector.RootID = "wrong"
				case "profile-outside":
					p.Reader.Directory = p.Root
				case "db-outside":
					p.Database = filepath.Join(filepath.Dir(p.Root), "outside.db")
				case "asset":
					p.Driver.SHA256 = strings.Repeat("a", 64)
				case "port":
					p.Port = 11122
				}
			})
			if scenario == "marker" {
				_ = os.WriteFile(filepath.Join(p.Root, "instance.json"), []byte(`{"schema":"wrong"}`), 0600)
			}
			if scenario == "hardlink" {
				if err := os.Link(path, filepath.Join(p.Root, "alias.json")); err != nil {
					t.Skipf("hardlink unavailable: %v", err)
				}
			}
			if scenario == "wal" || scenario == "shm" || scenario == "journal" {
				_ = os.WriteFile(p.Database+"-"+scenario, []byte("untrusted sidecar"), 0600)
			}
			if scenario == "version-alias" {
				if err := os.Link(path, filepath.Join(p.Root, "state", store.RuntimeVersionMarkerName)); err != nil {
					t.Skipf("hardlink unavailable: %v", err)
				}
			}
			if scenario == "version-newer" {
				if err := os.WriteFile(filepath.Join(p.Root, "state", store.RuntimeVersionMarkerName), []byte("99.0.0\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if q, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true}); err == nil {
				q.Close()
				t.Fatal("invalid isolation accepted")
			}
			if _, err := os.Stat(filepath.Join(p.Root, "state", "pilot.db")); !os.IsNotExist(err) {
				t.Fatal("invalid plan reached SQLite creation")
			}
		})
	}
}

func TestXQARetainedPlanSourceIsDeferredAndFixed(t *testing.T) {
	trusted, err := xRetainedQAProjectRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"deferred", "wrong-project", "explicit-profile", "unknown-source", "local-project"} {
		t.Run(scenario, func(t *testing.T) {
			path := xQAPlanTest(t)
			p := xQAEditPlan(t, path, func(p *XApplicationQAPlan) {
				p.ReaderSource = "retained-qa"
				p.ProjectRoot = trusted
				if scenario != "explicit-profile" {
					p.Reader = XApplicationQAProfile{}
				}
				switch scenario {
				case "wrong-project":
					p.ProjectRoot = p.Root
				case "unknown-source":
					p.ReaderSource = "external"
				case "local-project":
					p.ReaderSource = ""
				}
			})
			s, err := openXQASelection(context.Background(), path)
			if scenario != "deferred" {
				if err == nil {
					s.close()
					t.Fatal("invalid source accepted")
				}
				if _, err = os.Stat(p.Database); !os.IsNotExist(err) {
					t.Fatal("invalid source touched database")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.close()
			// Reader remains unresolved: opening a plan cannot acquire its lease
			// or reconstruct profile authority from a serialized descriptor.
			if s.owner.retainedProject != trusted || s.owner.reader.Root != "" || s.owner.reader.DirectoryID != "" {
				t.Fatal("retained source resolved before explicit start")
			}
		})
	}
}
func TestXQAEntrySyntheticStartReaderCaptureStopAndReuse(t *testing.T) {
	path := xQAPlanTest(t)
	q, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	before, _ := q.state.GetSettings(context.Background())
	if v := q.Report(); v.Status.State != "disabled" || !v.Synthetic || v.NativeQualified {
		t.Fatal("ready state grants authority")
	}
	if other, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true}); err == nil {
		other.Close()
		t.Fatal("duplicate instance opened")
	}
	if err = q.Start(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	originalDeadline := q.runtime.deadline
	request := readerbroker.Request{RequestID: "broker_0123456789abcdef0123456789abcdef", Source: "x", URL: "https://x.com/aku_fixture/status/1"}
	if err = q.OpenPost(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err = q.Settle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := q.Report(); v.Observations != 1 || v.Blocks < 1 || v.NativeQualified {
		t.Fatalf("capture %+v", v)
	}
	if q.Report().Evidence.Overlap != "observed" {
		t.Fatal("held synthetic capture and foreground did not prove overlap")
	}
	if !q.runtime.deadline.Equal(originalDeadline) {
		t.Fatal("reader/capture reset original attempt deadline")
	}
	if err = q.Start(context.Background(), true, false); err == nil {
		t.Fatal("second start accepted")
	}
	if err = q.OpenPost(context.Background(), request); err == nil {
		t.Fatal("reader request replay accepted")
	}
	if err = q.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := q.state.GetSettings(context.Background())
	if before.AutoUpdateEnabled != after.AutoUpdateEnabled {
		t.Fatal("Auto Update changed")
	}
	if err = q.Close(); err != nil {
		t.Fatal(err)
	}
	file, id, err := xQAPinFile(q.selection.plan.Database)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	xQAEditPlan(t, path, func(p *XApplicationQAPlan) { p.DatabaseID = id })
	reopened, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Report().Status.State != "disabled" {
		t.Fatal("restart inherited start consent")
	}
}
func TestXQAEntryDefaultOffAndStrictControls(t *testing.T) {
	if _, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{}); err == nil {
		t.Fatal("default opt-in enabled")
	}
	for _, raw := range []string{`{"op":"start","op":"stop"}`, `{"op":"status","nativeQualified":true}`, `{"op":"status","allowOneSessionDerivation":true}`, `{"op":"open_post"}`, `{"op":"unknown"}`} {
		if _, err := DecodeXApplicationQACommand([]byte(raw)); err == nil {
			t.Fatal("invalid command accepted")
		}
	}
}

func TestXQAEntryFailedStartupHasNoCaptureToSettle(t *testing.T) {
	q := &XApplicationQAPilot{runtime: &xManualApplication{captureDone: make(chan struct{})}}
	if err := q.Settle(context.Background()); err == nil {
		t.Fatal("failed startup reported a completed capture")
	}
}

func TestXQARetainedSyntheticJourneyKeepsLeaseThroughStop(t *testing.T) {
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("canonical QA helper required")
	}
	path, err := WriteXApplicationQASyntheticRetainedPlan(t.TempDir(), node, 28175)
	if err != nil {
		t.Fatal(err)
	}
	q, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	parent := filepath.Join(q.selection.plan.Root, "retained-fixture")
	if err = q.Start(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	request := readerbroker.Request{RequestID: "broker_0123456789abcdef0123456789abcdef", Source: "x", URL: "https://x.com/aku_fixture/status/1"}
	if err = q.OpenPost(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err = q.Settle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = q.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifest, err := xRetainedReadManifest(filepath.Join(parent, xRetainedQAProfileManifestName))
	if err != nil || manifest.State != "active" {
		t.Fatal("reader-preserving stop released retained profile")
	}
	if competing, err := acquireXRetainedProfileAtParent(parent, false); err == nil {
		competing.Release(false)
		t.Fatal("legacy caller acquired still-owned reader")
	}
	if report := q.Report(); report.Observations != 1 || report.Evidence.Overlap != "observed" || report.NativeQualified {
		t.Fatal("retained journey lost offline provenance, capture or overlap")
	}
	if err = q.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := acquireXRetainedProfileAtParent(parent, false)
	if err != nil {
		t.Fatal("verified parent cleanup did not permit legacy reacquisition")
	}
	if err = next.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestXQAAsyncCaptureFailurePreservesCauseWithoutAnotherCommand(t *testing.T) {
	q, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: xQAPlanTest(t), Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	f := q.kit.clock.(xQAClock).fixture
	f.backend.probe = func() error { return errors.New("PRIVATE backend failure cookie=secret") }
	if err = q.Start(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-q.Failed():
	case <-time.After(3 * time.Second):
		t.Fatal("capture failure waited for another input command")
	}
	first := q.Report().Evidence.FirstFailure
	if first == nil || first.Stage != "capture" || strings.Contains(first.Detail, "PRIVATE") || strings.Contains(first.Detail, "secret") {
		t.Fatal("capture cause missing or leaked")
	}
	q.RecordControlFailure("output")
	if err = q.Close(); err != nil {
		t.Fatal(err)
	}
	final := q.Report().Evidence
	if final.FirstFailure == nil || *final.FirstFailure != *first || final.Cleanup != "verified" {
		t.Fatal("later output/cleanup replaced primary cause")
	}
}
