package store

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func insertAccountCheckpointCommand(t *testing.T, s *Store, sessionID, source, runID, commandID, collector, epoch string, generation int) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"captureCollector": map[string]any{"version": 1, "backend": collector},
		"captureRuntime":   map[string]any{"driver": "headless", "epoch": epoch, "generation": generation},
		"continuation":     map[string]any{"cursor": commandID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at) VALUES(?,'update','running',2,8,'now') ON CONFLICT(id) DO NOTHING`, sessionID); err != nil {
		t.Fatal(err)
	}
	var ordinal int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(ordinal),-1)+1 FROM runs WHERE session_id=?`, sessionID).Scan(&ordinal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES(?,?,?,?,'waiting_for_bridge','capture','now')`, runID, sessionID, source, ordinal); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO bridge_commands(id,run_id,type,status,payload_json,created_at) VALUES(?,?,'collect_visible','queued',?,'now')`, commandID, runID, string(payload)); err != nil {
		t.Fatal(err)
	}
}

func readAccountCheckpointMediaRow(t *testing.T, s *Store, id string) capturePauseCommandSnapshot {
	t.Helper()
	var row capturePauseCommandSnapshot
	if err := s.db.QueryRow(`SELECT status,payload_json,claimed_at,claimed_by FROM media_recaptures WHERE id=?`, id).
		Scan(&row.status, &row.payload, &row.claimedAt, &row.claimedBy); err != nil {
		t.Fatal(err)
	}
	return row
}

func accountCheckpointMetadataFixture() (SyntheticAccountCheckpoint, string, string, string) {
	checkpoint := SyntheticAccountCheckpoint{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: "checkpoint-test", Generation: 1,
		ExpectedAccounts: map[domain.Source]SyntheticAccountIdentity{
			domain.SourceX: {AccountID: "account-x", StoreScopeID: "scope-x", ProfileID: "profile-x"},
		},
		AcceptedRevisions: map[domain.Source]string{},
	}
	checkpointRaw, _ := json.Marshal(checkpoint)
	requiredRaw, _ := json.Marshal(accountCheckpointRequired{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
	})
	fenceRaw, _ := json.Marshal(accountCheckpointFenceRecord{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
		CheckpointSHA256: checkpointSHA256(string(checkpointRaw)),
		Scope:            syntheticCheckpointScope(checkpoint.ExpectedAccounts), HoldID: "hold-test", State: "pending",
	})
	return checkpoint, string(checkpointRaw), string(requiredRaw), string(fenceRaw)
}

func installAccountCheckpointMetadata(t *testing.T, s *Store, checkpointRaw, requiredRaw, fenceRaw string) {
	t.Helper()
	for key, value := range map[string]string{
		syntheticAccountCheckpointKey:         checkpointRaw,
		syntheticAccountCheckpointRequiredKey: requiredRaw,
		syntheticAccountCheckpointFenceKey:    fenceRaw,
	} {
		if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)`, key, value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAccountCheckpointGuardBlocksClaimsWithoutHandlePermit(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	insertAccountCheckpointCommand(t, state, "checkpoint-session", "x", "checkpoint-run", "checkpoint-command", "headless", "synthetic-owner", 17)
	_, checkpointRaw, requiredRaw, fenceRaw := accountCheckpointMetadataFixture()
	installAccountCheckpointMetadata(t, state, checkpointRaw, requiredRaw, fenceRaw)
	want := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command")
	for _, collector := range []string{"", "headless", "bridge", "browser_quiet_hidden"} {
		var command *domain.BridgeCommand
		var err error
		if collector == "" {
			command, err = state.ClaimCommand(ctx, "checkpoint-run", "blocked-consumer")
		} else {
			command, err = state.ClaimCommandForCollector(ctx, "checkpoint-run", "blocked-consumer", collector)
		}
		if err != nil || command != nil {
			t.Fatalf("collector %q claim=%+v err=%v", collector, command, err)
		}
	}
	if got := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command"); !reflect.DeepEqual(got, want) {
		t.Fatalf("blocked claim changed command: got=%+v want=%+v", got, want)
	}
}

func TestAccountCheckpointGuardRejectsIncompleteUnknownAndCorruptMetadata(t *testing.T) {
	tests := []struct {
		name string
		seed func(*testing.T, *Store)
	}{
		{
			name: "missing requisite fence",
			seed: func(t *testing.T, s *Store) {
				_, cp, required, _ := accountCheckpointMetadataFixture()
				for key, value := range map[string]string{syntheticAccountCheckpointKey: cp, syntheticAccountCheckpointRequiredKey: required} {
					if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)`, key, value); err != nil {
						t.Fatal(err)
					}
				}
			},
		},
		{
			name: "missing expected identity",
			seed: func(t *testing.T, s *Store) {
				checkpoint, _, _, _ := accountCheckpointMetadataFixture()
				checkpoint.ExpectedAccounts[domain.SourceX] = SyntheticAccountIdentity{StoreScopeID: "scope-x", ProfileID: "profile-x"}
				cp, _ := json.Marshal(checkpoint)
				required, _ := json.Marshal(accountCheckpointRequired{Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion, CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation})
				fence, _ := json.Marshal(accountCheckpointFenceRecord{
					Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
					CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation, CheckpointSHA256: checkpointSHA256(string(cp)),
					Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts), HoldID: "hold-test", State: "pending",
				})
				installAccountCheckpointMetadata(t, s, string(cp), string(required), string(fence))
			},
		},
		{
			name: "expected-only checkpoint is not enrollment generation",
			seed: func(t *testing.T, s *Store) {
				checkpoint, _, _, _ := accountCheckpointMetadataFixture()
				checkpoint.Generation = 2
				cp, _ := json.Marshal(checkpoint)
				required, _ := json.Marshal(accountCheckpointRequired{Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion, CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation})
				fence, _ := json.Marshal(accountCheckpointFenceRecord{
					Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
					CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation, CheckpointSHA256: checkpointSHA256(string(cp)),
					Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts), HoldID: "hold-test", State: "pending",
				})
				installAccountCheckpointMetadata(t, s, string(cp), string(required), string(fence))
			},
		},
		{
			name: "corrupt value",
			seed: func(t *testing.T, s *Store) {
				_, _, required, fence := accountCheckpointMetadataFixture()
				installAccountCheckpointMetadata(t, s, "{broken", required, fence)
			},
		},
		{
			name: "future version",
			seed: func(t *testing.T, s *Store) {
				checkpoint, _, _, _ := accountCheckpointMetadataFixture()
				checkpoint.Version++
				cp, _ := json.Marshal(checkpoint)
				required, _ := json.Marshal(accountCheckpointRequired{Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion, CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation})
				fence, _ := json.Marshal(accountCheckpointFenceRecord{Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion, CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation, CheckpointSHA256: checkpointSHA256(string(cp)), Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts), HoldID: "hold-test", State: "pending"})
				installAccountCheckpointMetadata(t, s, string(cp), string(required), string(fence))
			},
		},
		{
			name: "unsafe owner generation",
			seed: func(t *testing.T, s *Store) {
				checkpoint, _, _, _ := accountCheckpointMetadataFixture()
				checkpoint.AcceptedRevisions = map[domain.Source]string{domain.SourceX: "revision-x"}
				checkpoint.VerificationTime = "2026-10-14T03:00:00Z"
				checkpoint.OwnerEpoch = "owner-epoch"
				checkpoint.HeadlessGeneration = accountCheckpointMaxGeneration + 1
				checkpoint.OwnerPID = 1248
				cp, _ := json.Marshal(checkpoint)
				required, _ := json.Marshal(accountCheckpointRequired{Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion, CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation})
				fence, _ := json.Marshal(accountCheckpointFenceRecord{
					Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
					CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation, CheckpointSHA256: checkpointSHA256(string(cp)),
					Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts), HoldID: "hold-test", State: "authorized",
					OwnerEpoch: checkpoint.OwnerEpoch, HeadlessGeneration: checkpoint.HeadlessGeneration, OwnerPID: checkpoint.OwnerPID,
				})
				installAccountCheckpointMetadata(t, s, string(cp), string(required), string(fence))
			},
		},
		{
			name: "unknown protected key",
			seed: func(t *testing.T, s *Store) {
				_, cp, required, fence := accountCheckpointMetadataFixture()
				installAccountCheckpointMetadata(t, s, cp, required, fence)
				if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES('synthetic_account_checkpoint_future','future-v2')`); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			insertAccountCheckpointCommand(t, state, "checkpoint-session", "x", "checkpoint-run", "checkpoint-command", "headless", "synthetic-owner", 17)
			test.seed(t, state)
			want := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command")
			command, err := state.ClaimCommandForCollector(ctx, "checkpoint-run", "blocked-consumer", "headless")
			if err == nil || command != nil {
				t.Fatalf("invalid metadata admitted claim=%+v err=%v", command, err)
			}
			if got := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command"); !reflect.DeepEqual(got, want) {
				t.Fatalf("invalid metadata changed command: got=%+v want=%+v", got, want)
			}
		})
	}
}

func TestAccountCheckpointGuardLookupErrorsFailClosed(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	insertAccountCheckpointCommand(t, state, "checkpoint-session", "x", "checkpoint-run", "checkpoint-command", "headless", "synthetic-owner", 17)
	want := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command")
	if _, err := state.db.Exec(`DROP TABLE meta`); err != nil {
		t.Fatal(err)
	}
	command, err := state.ClaimCommand(ctx, "checkpoint-run", "blocked-consumer")
	if err == nil || err.Error() != "capture pause lookup failed" || command != nil {
		t.Fatalf("failed metadata lookup admitted claim=%+v err=%v", command, err)
	}
	if got := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command"); !reflect.DeepEqual(got, want) {
		t.Fatalf("failed metadata lookup changed command: got=%+v want=%+v", got, want)
	}
}

func TestAccountCheckpointGuardBlocksExistingQueuedMediaRecapture(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID, _ := insertUnavailableMediaFixture(t, state)
	job, err := state.CreateOwnedMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, map[string]any{"driver": "headless"}, "headless")
	if err != nil {
		t.Fatal(err)
	}
	_, checkpointRaw, requiredRaw, fenceRaw := accountCheckpointMetadataFixture()
	installAccountCheckpointMetadata(t, state, checkpointRaw, requiredRaw, fenceRaw)
	before, err := state.MediaRecapture(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	readRow := func() capturePauseCommandSnapshot {
		t.Helper()
		var row capturePauseCommandSnapshot
		if err := state.db.QueryRow(`SELECT status,payload_json,claimed_at,claimed_by FROM media_recaptures WHERE id=?`, job.ID).
			Scan(&row.status, &row.payload, &row.claimedAt, &row.claimedBy); err != nil {
			t.Fatal(err)
		}
		return row
	}
	beforeRow := readRow()
	claimed, err := state.ClaimMediaRecaptureForCollector(ctx, job.ID, "blocked-consumer", "headless")
	if err != nil || claimed.ID != "" {
		t.Fatalf("protected media claim=%+v err=%v", claimed, err)
	}
	after, err := state.MediaRecapture(ctx, job.ID)
	if err != nil || after.Status != "queued" || !reflect.DeepEqual(after.Payload, before.Payload) || after.ClaimedAt != nil {
		t.Fatalf("protected media recapture changed: before=%+v after=%+v err=%v", before, after, err)
	}
	if afterRow := readRow(); afterRow != beforeRow {
		t.Fatalf("protected media raw row changed: before=%+v after=%+v", beforeRow, afterRow)
	}
}
