//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"reflect"
	"testing"
)

func TestFixtureLiveReaderCommandEvidencePauseReopen(t *testing.T) {
	s := fixturePauseStore(t)
	ctx := context.Background()
	if _, err := s.db.Exec(`DELETE FROM bridge_commands WHERE id='done'`); err != nil {
		t.Fatal(err)
	}
	before, err := s.FixtureRunCommandEvidence(ctx, "run")
	if err != nil || len(before) != 1 || before[0].Status != "queued" || before[0].Collector != "headless" || before[0].Driver != "headless" || before[0].ClaimedAt.Valid || before[0].ClaimedBy.Valid || len(before[0].PayloadSHA256) != 64 {
		t.Fatal("prepared unclaimed command missing", err)
	}
	if err = s.PauseFixtureCaptureSession(ctx, "fixture"); err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimCommandForCollector(ctx, "run", "must-not-claim", "headless")
	if err != nil || claim != nil {
		t.Fatal("paused prepared command claimed", err)
	}
	after, err := s.FixtureRunCommandEvidence(ctx, "run")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("blocked claim changed row", err)
	}
	path := s.path
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	paused, err := reopened.FixtureCaptureSessionPaused(ctx, "fixture")
	if err != nil || !paused {
		t.Fatal("pause lost on reopen", err)
	}
	claim, err = reopened.ClaimCommandForCollector(ctx, "run", "after-reopen", "headless")
	if err != nil || claim != nil {
		t.Fatal("reopened prepared command claimed", err)
	}
	after, err = reopened.FixtureRunCommandEvidence(ctx, "run")
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("original row/payload lost on reopen", err)
	}
	if _, err = reopened.db.Exec(`UPDATE bridge_commands SET claimed_by='' WHERE id='queued'`); err != nil {
		t.Fatal(err)
	}
	after, err = reopened.FixtureRunCommandEvidence(ctx, "run")
	if err != nil || !after[0].ClaimedBy.Valid || reflect.DeepEqual(before, after) {
		t.Fatal("null versus empty claim metadata collapsed", err)
	}
	if _, err = reopened.db.Exec(`UPDATE bridge_commands SET claimed_by=NULL,payload_json='{"captureCollector":{"version":1,"backend":"headless"},"changed":true}' WHERE id='queued'`); err != nil {
		t.Fatal(err)
	}
	after, err = reopened.FixtureRunCommandEvidence(ctx, "run")
	if err != nil || before[0].PayloadSHA256 == after[0].PayloadSHA256 {
		t.Fatal("payload change was invisible", err)
	}
}
