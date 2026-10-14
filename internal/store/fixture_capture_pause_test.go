//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"encoding/json"
	"testing"
)

func fixturePauseStore(t *testing.T) *Store {
	t.Helper()
	s := openTestStore(t)
	for _, query := range []string{
		`INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES('fixture','update','running',1,1,'now')`,
		`INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('run','fixture','x',0,'waiting_for_bridge','capture','now')`,
		`INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('queued','run','collect_visible','queued','{"captureCollector":{"version":1,"backend":"headless"},"captureRuntime":{"driver":"headless","epoch":"old","generation":1},"continuation":{"cursor":"fixture-cursor"}}','now')`,
		`INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES('done','run','collect_visible','completed','{"keep":"unchanged"}','before')`,
	} {
		if _, err := s.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestFixtureCapturePauseResumePreservesQueue(t *testing.T) {
	ctx := context.Background()
	s := fixturePauseStore(t)
	if err := s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	ids, err := s.ListFixturePausedCaptureSessions(ctx)
	if err != nil || len(ids) != 1 || ids[0] != "fixture" {
		t.Fatalf("markers=%v err=%v", ids, err)
	}
	if err := s.ResumeFixtureCaptureSession(ctx, "fixture", "replacement", 2); err != nil {
		t.Fatal(err)
	}
	var raw, status, done string
	if err := s.db.QueryRow(`SELECT payload_json,status FROM bridge_commands WHERE id='queued'`).Scan(&raw, &status); err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatal(err)
	}
	stamp := payload["captureRuntime"].(map[string]any)
	if stamp["epoch"] != "replacement" || stamp["generation"] != float64(2) || status != "queued" || payload["continuation"].(map[string]any)["cursor"] != "fixture-cursor" {
		t.Fatalf("queue changed unexpectedly: %s %s", status, raw)
	}
	if err := s.db.QueryRow(`SELECT payload_json FROM bridge_commands WHERE id='done'`).Scan(&done); err != nil || done != `{"keep":"unchanged"}` {
		t.Fatalf("completed changed: %s %v", done, err)
	}
	paused, err := s.FixtureCaptureSessionPaused(ctx, "fixture")
	if err != nil || paused {
		t.Fatalf("marker retained: %t %v", paused, err)
	}
	if err := s.ResumeFixtureCaptureSession(ctx, "fixture", "replacement", 3); err == nil {
		t.Fatal("duplicate resume accepted")
	}
}

func TestFixtureCapturePauseRejectsUnsafeCommands(t *testing.T) {
	for _, query := range []string{
		`UPDATE bridge_commands SET status='claimed' WHERE id='queued'`,
		`UPDATE bridge_commands SET claimed_at='before' WHERE id='queued'`,
		`UPDATE bridge_commands SET payload_json='{"captureCollector":{"version":1,"backend":"bridge"},"captureRuntime":{"driver":"headless","epoch":"old","generation":1}}' WHERE id='queued'`,
		`UPDATE bridge_commands SET payload_json='{"captureCollector":{"version":1,"backend":"headless"},"captureRuntime":{"driver":"headless","epoch":"old","generation":0}}' WHERE id='queued'`,
	} {
		t.Run(query, func(t *testing.T) {
			s := fixturePauseStore(t)
			if _, err := s.db.Exec(query); err != nil {
				t.Fatal(err)
			}
			if err := s.PauseFixtureCaptureSession(context.Background(), "fixture"); err == nil {
				t.Fatal("unsafe pause accepted")
			}
			ids, err := s.ListFixturePausedCaptureSessions(context.Background())
			if err != nil || len(ids) != 0 {
				t.Fatalf("partial marker persisted: %v %v", ids, err)
			}
		})
	}
}

func TestFixtureCaptureResumeRollbackAndCancellation(t *testing.T) {
	ctx := context.Background()
	s := fixturePauseStore(t)
	if err := s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fixture_resume_failure BEFORE UPDATE OF payload_json ON bridge_commands BEGIN SELECT RAISE(ABORT,'fixture rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ResumeFixtureCaptureSession(ctx, "fixture", "next", 2); err == nil {
		t.Fatal("failed update accepted")
	}
	paused, err := s.FixtureCaptureSessionPaused(ctx, "fixture")
	if err != nil || !paused {
		t.Fatalf("rollback lost marker: %t %v", paused, err)
	}
	if err := s.CancelSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.ResumeFixtureCaptureSession(ctx, "fixture", "next", 2); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := s.db.QueryRow(`SELECT status FROM bridge_commands WHERE id='queued'`).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("cancel resurrected: %s %v", status, err)
	}
}

func TestFixtureCaptureResumeRejectsRouteMutation(t *testing.T) {
	ctx := context.Background()
	s := fixturePauseStore(t)
	if err := s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE bridge_commands SET payload_json='{"captureCollector":{"version":1,"backend":"headless"}}' WHERE id='queued'`); err != nil {
		t.Fatal(err)
	}
	if err := s.ResumeFixtureCaptureSession(ctx, "fixture", "next", 2); err == nil {
		t.Fatal("malformed stamp resumed")
	}
	paused, err := s.FixtureCaptureSessionPaused(ctx, "fixture")
	if err != nil || !paused {
		t.Fatalf("rejected resume lost marker: %t %v", paused, err)
	}
	var marker string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='fixture_capture_pause:fixture'`).Scan(&marker); err != nil || marker != "1" {
		t.Fatalf("marker stores more than flag: %q %v", marker, err)
	}
}

func TestFixtureCaptureClaimGateAndTerminalPause(t *testing.T) {
	ctx := context.Background()
	s := fixturePauseStore(t)
	if err := s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := fixtureCaptureClaimPaused(ctx, tx, "run")
	if err != nil || !paused {
		t.Fatalf("claim gate=%t %v", paused, err)
	}
	missing, err := fixtureCaptureClaimPaused(ctx, tx, "foreign-run")
	if err != nil || missing {
		t.Fatalf("foreign gate=%t %v", missing, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	paused, err = s.FixtureCaptureSessionPaused(ctx, "fixture")
	if err != nil || paused {
		t.Fatalf("terminal marker=%t %v", paused, err)
	}
}
