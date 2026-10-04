package store

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestCaptureCollectorClaimsFenceRoutesBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload map[string]any
		route   string
	}{
		{"legacy_null", nil, "bridge"},
		{"legacy", map[string]any{}, "bridge"},
		{"legacy_headless", map[string]any{"captureRuntime": map[string]any{"driver": "headless"}}, "headless"},
		{"quiet", map[string]any{"captureCollector": map[string]any{"backend": "browser_quiet_hidden", "version": 1}}, "browser_quiet_hidden"},
		{"unknown", map[string]any{"captureCollector": map[string]any{"backend": "future", "version": 1}}, ""},
		{"unknown_version", map[string]any{"captureCollector": map[string]any{"backend": "bridge", "version": 2}}, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			settings, _ := state.GetSettings(ctx)
			session, err := createVisibleUpdateSession(state, ctx, "collector boundary", settings)
			if err != nil {
				t.Fatal(err)
			}
			run := session.Runs[0]
			if _, err := state.StartRun(ctx, run.ID, test.payload); err != nil {
				t.Fatal(err)
			}
			for _, route := range []string{"bridge", "browser_quiet_hidden", "headless"} {
				if route == test.route {
					continue
				}
				command, err := state.ClaimCommandForCollector(ctx, run.ID, route, route)
				if command != nil || (test.route != "" && err != nil) {
					t.Fatalf("wrong route claimed: %+v %v", command, err)
				}
				if test.route == "" && err == nil {
					t.Fatal("unknown route admitted")
				}
				stored, err := state.GetRun(ctx, run.ID)
				if err != nil || stored.BridgeCommandStatus != "queued" {
					t.Fatalf("wrong consumer mutated state: %+v %v", stored, err)
				}
			}
			if test.route != "" {
				command, err := state.ClaimCommandForCollector(ctx, run.ID, "owner", test.route)
				if err != nil || command == nil {
					t.Fatalf("right consumer: %+v %v", command, err)
				}
			}
		})
	}
}

func TestCaptureCollectorCorruptPayloadCannotDefaultToBridge(t *testing.T) {
	for _, raw := range []string{"{", "[]", ""} {
		t.Run(raw, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			settings, _ := state.GetSettings(ctx)
			session, err := createVisibleUpdateSession(state, ctx, "corrupt routing fence", settings)
			if err != nil {
				t.Fatal(err)
			}
			corrupt, valid := session.Runs[0], session.Runs[1]
			command, err := state.StartRun(ctx, corrupt.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.db.ExecContext(ctx, `UPDATE bridge_commands SET payload_json=? WHERE id=?`, raw, command.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := state.StartRun(ctx, valid.ID, map[string]any{}); err != nil {
				t.Fatal(err)
			}
			if claim, err := state.ClaimCommandForCollector(ctx, corrupt.ID, "bridge", "bridge"); err == nil || claim != nil {
				t.Fatalf("corrupt command admitted=%+v %v", claim, err)
			}
			if route, exists, err := state.FirstCommandCollector(ctx, corrupt.ID); err == nil || !exists || route != "" {
				t.Fatalf("corrupt command inferred legacy route=%s %t %v", route, exists, err)
			}
			if id, err := state.PendingRunIDForCollector(ctx, "bridge"); err != nil || id != valid.ID {
				t.Fatalf("corrupt command dispatched or blocked valid work=%s %v", id, err)
			}
			var status string
			if err := state.db.QueryRowContext(ctx, `SELECT status FROM bridge_commands WHERE id=?`, command.ID).Scan(&status); err != nil || status != "queued" {
				t.Fatalf("corrupt claim mutated state=%s %v", status, err)
			}
		})
	}
}

func TestMediaCollectorCorruptPayloadCannotBeClaimed(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _ := insertUnavailableMediaFixture(t, state)
	job, err := state.CreateOwnedMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, nil, "browser_quiet_hidden")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE media_recaptures SET payload_json='{' WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}
	if claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "bridge", "bridge"); err == nil || claim.ID != "" {
		t.Fatalf("corrupt recapture admitted=%+v %v", claim, err)
	}
	var status string
	if err := state.db.QueryRowContext(ctx, `SELECT status FROM media_recaptures WHERE id=?`, job.ID).Scan(&status); err != nil || status != "queued" {
		t.Fatalf("corrupt recapture mutated state=%s %v", status, err)
	}
}

func TestCaptureCollectorMixedPendingAndSessionSerialization(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "mixed consumers", settings)
	if err != nil {
		t.Fatal(err)
	}
	quiet, bridge := session.Runs[0], session.Runs[1]
	command, err := state.StartRun(ctx, quiet.ID, map[string]any{"captureCollector": map[string]any{"backend": "browser_quiet_hidden", "version": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.StartRun(ctx, bridge.ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	for route, expected := range map[string]string{"bridge": bridge.ID, "browser_quiet_hidden": quiet.ID, "headless": ""} {
		id, err := state.PendingRunIDForCollector(ctx, route)
		if err != nil || id != expected {
			t.Fatalf("pending %s=%s want %s: %v", route, id, expected, err)
		}
	}
	if _, err := state.ClaimCommandForCollector(ctx, quiet.ID, "quiet", "browser_quiet_hidden"); err != nil {
		t.Fatal(err)
	}
	if claim, err := state.ClaimCommandForCollector(ctx, bridge.ID, "bridge", "bridge"); err != nil || claim != nil {
		t.Fatalf("simultaneous mixed claim=%+v %v", claim, err)
	}
	if err := state.SaveObservation(ctx, command.ID, quiet.ID, domain.Observation{Source: quiet.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{}}}); err != nil {
		t.Fatal(err)
	}
	if claim, err := state.ClaimCommandForCollector(ctx, bridge.ID, "bridge", "bridge"); err != nil || claim == nil {
		t.Fatalf("bridge lane disappeared: %+v %v", claim, err)
	}
	if _, err := state.QueueFollowUp(ctx, quiet.ID, map[string]any{"captureCollector": map[string]any{"backend": "bridge", "version": 1}}); err != nil {
		t.Fatal(err)
	}
	if route, exists, err := state.FirstCommandCollector(ctx, quiet.ID); err != nil || !exists || route != "browser_quiet_hidden" {
		t.Fatalf("first route=%s %t %v", route, exists, err)
	}
}

func TestMediaCollectorAdmissionAndWrongClaimRemainAtomic(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _ := insertUnavailableMediaFixture(t, state)
	job, err := state.CreateOwnedMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, map[string]any{"driver": "browser"}, "browser_quiet_hidden")
	if err != nil {
		t.Fatal(err)
	}
	if claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "bridge", "bridge"); err != nil || claim.ID != "" {
		t.Fatalf("wrong recapture consumer=%+v %v", claim, err)
	}
	stored, err := state.MediaRecapture(ctx, job.ID)
	if err != nil || stored.Status != "queued" {
		t.Fatalf("wrong consumer mutation=%+v %v", stored, err)
	}
	if route, err := CaptureCollector(stored.Payload); err != nil || route != "browser_quiet_hidden" {
		t.Fatalf("persisted route=%s %v", route, err)
	}
	if claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "quiet", "browser_quiet_hidden"); err != nil || claim.ID != job.ID {
		t.Fatalf("right recapture consumer=%+v %v", claim, err)
	}
}

func TestHybridFacebookRecaptureWaitsForAtomicBrowserAdmission(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _, _ := insertFacebookPlaybackFixture(t, state)
	job, err := state.CreateOwnedMediaRecaptureWithAdmission(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecapturePlaybackError, "bridge", domain.MediaRecaptureCaptureAdmission{
		Policy: domain.MediaRecaptureAdmissionHybridHeadlessV1,
		Driver: "browser",
		Phase:  "waiting",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, consumer := range []string{"", "bridge", "headless"} {
		claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "early-consumer", consumer)
		if err != nil || claim.ID != "" {
			t.Fatalf("waiting recapture claim by %q = %+v, %v", consumer, claim, err)
		}
	}
	stored, err := state.MediaRecapture(ctx, job.ID)
	if err != nil || stored.Status != "queued" {
		t.Fatalf("early claim changed waiting job: %+v, %v", stored, err)
	}
	stamp := map[string]any{"driver": "browser", "epoch": "epoch-test", "generation": 7}
	if admitted, err := state.BindMediaRecaptureBrowserAdmission(ctx, job.ID, stamp); err != nil || !admitted {
		t.Fatalf("Browser admission bound=%t: %v", admitted, err)
	}
	stored, err = state.MediaRecapture(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	marker, exists, err := MediaRecaptureAdmission(stored.Payload)
	if err != nil || !exists || marker.Phase != "admitted" || stored.Payload["captureRuntime"] == nil {
		t.Fatalf("admitted marker/runtime=%+v, %t, %v, payload=%+v", marker, exists, err, stored.Payload)
	}
	if claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "wrong", "headless"); err != nil || claim.ID != "" {
		t.Fatalf("wrong collector claimed admitted Browser job: %+v, %v", claim, err)
	}
	if claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "right", "bridge"); err != nil || claim.ID != job.ID {
		t.Fatalf("Bridge could not claim admitted job: %+v, %v", claim, err)
	}
	if _, err := state.FailMediaRecapture(ctx, job.ID, domain.Failure{Code: "test", Stage: "test", Message: "terminal cleanup fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := state.SetMediaRecaptureCaptureCleanupState(ctx, job.ID, "pending"); err != nil {
		t.Fatal(err)
	}
	stored, err = state.MediaRecapture(ctx, job.ID)
	if err != nil || stored.Payload["captureCleanup"] != "pending" {
		t.Fatalf("pending cleanup state did not persist: %+v, %v", stored.Payload, err)
	}
	ids, err := state.BrowserAdmissionMediaRecaptureIDs(ctx)
	if err != nil || len(ids) != 1 || ids[0] != job.ID {
		t.Fatalf("pending terminal cleanup missing from recovery scan: %v, %v", ids, err)
	}
	if err := state.SetMediaRecaptureCaptureCleanupState(ctx, job.ID, "released"); err != nil {
		t.Fatal(err)
	}
	if err := state.SetMediaRecaptureCaptureCleanupState(ctx, job.ID, "pending"); err != nil {
		t.Fatalf("stale pending cleanup write should be idempotent after release: %v", err)
	}
	stored, err = state.MediaRecapture(ctx, job.ID)
	if err != nil || stored.Payload["captureCleanup"] != "released" {
		t.Fatalf("released cleanup state regressed: %+v, %v", stored.Payload, err)
	}
	ids, err = state.BrowserAdmissionMediaRecaptureIDs(ctx)
	if err != nil || len(ids) != 0 {
		t.Fatalf("released terminal admitted job still appears in recovery scan: %v, %v", ids, err)
	}
}

func TestMalformedHybridRecaptureAdmissionFailsClosed(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _, _ := insertFacebookPlaybackFixture(t, state)
	job, err := state.CreateOwnedMediaRecaptureWithAdmission(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecapturePlaybackError, "bridge", domain.MediaRecaptureCaptureAdmission{
		Policy: domain.MediaRecaptureAdmissionHybridHeadlessV1,
		Driver: "browser",
		Phase:  "waiting",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE media_recaptures SET payload_json=json_set(payload_json,'$.captureAdmission.phase','future') WHERE id=?`, job.ID); err != nil {
		t.Fatal(err)
	}
	if claim, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "bridge", "bridge"); err == nil || claim.ID != "" {
		t.Fatalf("malformed admission claim=%+v, %v", claim, err)
	}
	stored, err := state.MediaRecapture(ctx, job.ID)
	if err != nil || stored.Status != "queued" {
		t.Fatalf("malformed marker mutated job: %+v, %v", stored, err)
	}
}
