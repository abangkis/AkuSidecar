package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	capturePauseQARootEnv  = "AKUSIDECAPTUREPAUSE_QA_ROOT"
	capturePauseQAStageEnv = "AKUSIDECAPTUREPAUSE_QA_STAGE"
	capturePauseQADBName   = "capture-pause-cross-build.db"
	capturePauseMain       = "cross-build-paused"
	capturePauseCommandOne = `{"captureCollector":{"version":1,"backend":"headless"},"captureRuntime":{"driver":"headless","epoch":"old-one","generation":1},"continuation":{"cursor":"keep-one"}}`
	capturePauseCommandTwo = `{"captureCollector":{"version":1,"backend":"headless"},"captureRuntime":{"driver":"headless","epoch":"old-two","generation":2},"continuation":{"cursor":"keep-two"}}`
)

// TestCapturePauseGuardCrossBuildStage is an opt-in, process-separated test
// protocol. The orchestrator supplies a project-local root and a stage name;
// separate ordinary and fixture-tagged binaries then reuse one SQLite file.
func TestCapturePauseGuardCrossBuildStage(t *testing.T) {
	root := strings.TrimSpace(os.Getenv(capturePauseQARootEnv))
	stage := strings.TrimSpace(os.Getenv(capturePauseQAStageEnv))
	if stage == "" {
		t.Skip("cross-build pause stages require an explicit opt-in QA stage")
	}
	if root == "" {
		t.Fatalf("both %s and %s are required", capturePauseQARootEnv, capturePauseQAStageEnv)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil || !info.IsDir() {
		t.Fatalf("QA root must already exist as a directory: %s (%v)", absoluteRoot, err)
	}
	runCapturePauseGuardStage(t, absoluteRoot, stage)
}

func openCapturePauseQAStore(t *testing.T, root string) *Store {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	s, err := Open(filepath.Join(root, capturePauseQADBName), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func seedCapturePauseQADatabase(t *testing.T, s *Store) {
	t.Helper()
	insertCapturePauseGuardSession(t, s, capturePauseMain, "cross-build-run-one", "cross-build-command-one", capturePauseCommandOne)
	insertCapturePauseGuardSession(t, s, capturePauseMain, "cross-build-run-two", "cross-build-command-two", capturePauseCommandTwo)
	insertCapturePauseGuardSession(t, s, "cross-build-open", "cross-build-open-run", "cross-build-open-command", `{"captureCollector":{"version":1,"backend":"bridge"},"continuation":{"cursor":"eligible"}}`)
}

func capturePauseQAMarker(t *testing.T, s *Store, sessionID string) string {
	t.Helper()
	var marker string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID).Scan(&marker); err != nil {
		t.Fatal(err)
	}
	return marker
}

func capturePauseQAMarkerExists(t *testing.T, s *Store, sessionID string) bool {
	t.Helper()
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count != 0
}

func assertCapturePauseQAQueuedUnchanged(t *testing.T, s *Store) {
	t.Helper()
	for id, want := range map[string]string{
		"cross-build-command-one": capturePauseCommandOne,
		"cross-build-command-two": capturePauseCommandTwo,
	} {
		got := loadCapturePauseCommandSnapshot(t, s, id)
		if got.status != "queued" || got.payload != want || got.claimedAt.Valid || got.claimedBy.Valid {
			t.Fatalf("command %s changed: %+v", id, got)
		}
	}
}

func assertCapturePauseQAResumed(t *testing.T, s *Store, id, wantCursor string, generation float64) {
	t.Helper()
	got := loadCapturePauseCommandSnapshot(t, s, id)
	if got.status != "queued" || got.claimedAt.Valid || got.claimedBy.Valid {
		t.Fatalf("resumed command metadata changed: %+v", got)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(got.payload), &payload); err != nil {
		t.Fatal(err)
	}
	stamp, ok := payload["captureRuntime"].(map[string]any)
	if !ok || stamp["driver"] != "headless" || stamp["epoch"] != "fixture-owner-next" || stamp["generation"] != generation {
		t.Fatalf("runtime stamp was not safely restamped: %s", got.payload)
	}
	if payload["continuation"].(map[string]any)["cursor"] != wantCursor {
		t.Fatalf("continuation changed on resume: %s", got.payload)
	}
	if payload["captureCollector"].(map[string]any)["backend"] != "headless" {
		t.Fatalf("collector route changed on resume: %s", got.payload)
	}
}

func assertCapturePauseQASnapshotsEqual(t *testing.T, s *Store, want map[string]capturePauseCommandSnapshot) {
	t.Helper()
	for id, expected := range want {
		if got := loadCapturePauseCommandSnapshot(t, s, id); !reflect.DeepEqual(got, expected) {
			t.Fatalf("command %s changed: got=%+v want=%+v", id, got, expected)
		}
	}
}

func capturePauseQASnapshots(t *testing.T, s *Store, ids ...string) map[string]capturePauseCommandSnapshot {
	t.Helper()
	out := make(map[string]capturePauseCommandSnapshot, len(ids))
	for _, id := range ids {
		out[id] = loadCapturePauseCommandSnapshot(t, s, id)
	}
	return out
}

func capturePauseQAUnknownMarkerSession(t *testing.T, s *Store) {
	t.Helper()
	insertCapturePauseGuardSession(t, s, "future-marker-session", "future-marker-run", "future-marker-command", capturePauseCommandOne)
	if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)`, fixtureCapturePausePrefix+"future-marker-session", "future-v2"); err != nil {
		t.Fatal(err)
	}
}

func capturePauseQAEnsureMarkerValue(t *testing.T, s *Store, sessionID, want string) {
	t.Helper()
	if got := capturePauseQAMarker(t, s, sessionID); got != want {
		t.Fatalf("marker value=%q want %q", got, want)
	}
}
