package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type capturePauseCommandSnapshot struct {
	status    string
	payload   string
	claimedAt sql.NullString
	claimedBy sql.NullString
}

func insertCapturePauseGuardSession(t *testing.T, s *Store, sessionID, runID, commandID, payload string) {
	t.Helper()
	var existing string
	err := s.db.QueryRow(`SELECT id FROM sessions WHERE id=?`, sessionID).Scan(&existing)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.db.Exec(`INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES(?,'update','running',1,1,'now')`, sessionID); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
	var ordinal int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(ordinal),-1)+1 FROM runs WHERE session_id=?`, sessionID).Scan(&ordinal); err != nil {
		t.Fatal(err)
	}
	sources := []string{"x", "instagram", "linkedin", "facebook"}
	if ordinal >= len(sources) {
		t.Fatal("synthetic test session exceeds its source list")
	}
	if _, err := s.db.Exec(`INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES(?,?,?,?,'waiting_for_bridge','capture','now')`, runID, sessionID, sources[ordinal], ordinal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES(?,?,'collect_visible','queued',?,'now')`, commandID, runID, payload); err != nil {
		t.Fatal(err)
	}
}

func loadCapturePauseCommandSnapshot(t *testing.T, s *Store, commandID string) capturePauseCommandSnapshot {
	t.Helper()
	var snapshot capturePauseCommandSnapshot
	err := s.db.QueryRow(`SELECT status,payload_json,claimed_at,claimed_by FROM bridge_commands WHERE id=?`, commandID).Scan(
		&snapshot.status, &snapshot.payload, &snapshot.claimedAt, &snapshot.claimedBy,
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestCapturePauseGuardBlocksUnknownMarkerAcrossSessionRuns(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	insertCapturePauseGuardSession(t, s, "paused-session", "paused-run-one", "paused-command-one", `{"captureCollector":{"version":1,"backend":"headless"},"captureRuntime":{"driver":"headless","epoch":"epoch-one","generation":1},"continuation":{"cursor":"keep-one"}}`)
	insertCapturePauseGuardSession(t, s, "paused-session", "paused-run-two", "paused-command-two", `{"captureCollector":{"version":1,"backend":"bridge"},"continuation":{"cursor":"keep-two"}}`)
	insertCapturePauseGuardSession(t, s, "paused-session", "paused-run-three", "paused-command-three", `{"captureCollector":{"version":1,"backend":"browser_quiet_hidden"},"continuation":{"cursor":"keep-three"}}`)
	insertCapturePauseGuardSession(t, s, "open-session", "open-run", "open-command", `{"captureCollector":{"version":1,"backend":"bridge"},"continuation":{"cursor":"eligible"}}`)
	if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)`, fixtureCapturePausePrefix+"paused-session", "future-v2"); err != nil {
		t.Fatal(err)
	}
	wantOne := loadCapturePauseCommandSnapshot(t, s, "paused-command-one")
	wantTwo := loadCapturePauseCommandSnapshot(t, s, "paused-command-two")
	wantThree := loadCapturePauseCommandSnapshot(t, s, "paused-command-three")
	for _, claim := range []struct {
		runID     string
		collector string
	}{
		{runID: "paused-run-one"},
		{runID: "paused-run-two", collector: "bridge"},
		{runID: "paused-run-three", collector: "browser_quiet_hidden"},
	} {
		var command *domain.BridgeCommand
		var err error
		if claim.collector == "" {
			command, err = s.ClaimCommand(ctx, claim.runID, "must-stay-queued")
		} else {
			command, err = s.ClaimCommandForCollector(ctx, claim.runID, "must-stay-queued", claim.collector)
		}
		if err != nil || command != nil {
			t.Fatalf("paused run %s claim=%+v err=%v", claim.runID, command, err)
		}
	}
	if got := loadCapturePauseCommandSnapshot(t, s, "paused-command-one"); !reflect.DeepEqual(got, wantOne) {
		t.Fatalf("first paused command changed: got=%+v want=%+v", got, wantOne)
	}
	if got := loadCapturePauseCommandSnapshot(t, s, "paused-command-two"); !reflect.DeepEqual(got, wantTwo) {
		t.Fatalf("second paused command changed: got=%+v want=%+v", got, wantTwo)
	}
	if got := loadCapturePauseCommandSnapshot(t, s, "paused-command-three"); !reflect.DeepEqual(got, wantThree) {
		t.Fatalf("third paused command changed: got=%+v want=%+v", got, wantThree)
	}
	command, err := s.ClaimCommandForCollector(ctx, "open-run", "eligible-bridge", "bridge")
	if err != nil || command == nil || command.ID != "open-command" || command.Status != "claimed" {
		t.Fatalf("unmarked session claim=%+v err=%v", command, err)
	}
}

func TestCapturePauseGuardDatabaseQueryErrorBlocksClaim(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	insertCapturePauseGuardSession(t, s, "error-session", "error-run", "error-command", `{"captureCollector":{"version":1,"backend":"bridge"}}`)
	want := loadCapturePauseCommandSnapshot(t, s, "error-command")
	if _, err := s.db.Exec(`DROP TABLE meta`); err != nil {
		t.Fatal(err)
	}
	command, err := s.ClaimCommand(ctx, "error-run", "must-stay-queued")
	if err == nil || command != nil {
		t.Fatalf("claim with failed pause lookup=%+v err=%v", command, err)
	}
	if got := loadCapturePauseCommandSnapshot(t, s, "error-command"); !reflect.DeepEqual(got, want) {
		t.Fatalf("command changed after pause lookup error: got=%+v want=%+v", got, want)
	}
}
