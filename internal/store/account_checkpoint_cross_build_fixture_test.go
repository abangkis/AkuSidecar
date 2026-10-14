//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func runAccountCheckpointCrossBuildStage(t *testing.T, root, stage string) {
	t.Helper()
	state := openAccountCheckpointQAStore(t, root)
	ctx := context.Background()
	switch stage {
	case "fixture-enroll":
		seedAccountCheckpointQADatabase(t, state)
		if err := state.CreateSyntheticAccountCheckpoint(ctx, accountCheckpointQACheckpoint()); err != nil {
			t.Fatal(err)
		}
		if generation := accountCheckpointQAStoredGeneration(t, state); generation != 1 {
			t.Fatalf("enrolled generation=%d want 1", generation)
		}
		assertAccountCheckpointQACommandsQueued(t, state, accountCheckpointQACommands)
	case "dev-authorize":
		checkpoint := accountCheckpointQACheckpoint()
		fence, err := state.BeginSyntheticCheckpointFence(ctx, accountCheckpointQAID, 1)
		if err != nil {
			t.Fatal(err)
		}
		receipt := accountCheckpointQAReceipt(state, fence, checkpoint, accountCheckpointQAEpoch, accountCheckpointQAGen)
		if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err != nil {
			t.Fatal(err)
		}
		if generation := accountCheckpointQAStoredGeneration(t, state); generation != 2 {
			t.Fatalf("authorized generation=%d want 2", generation)
		}
		claim, err := state.ClaimCommandForCollector(ctx, "checkpoint-run-x-one", "dev-authorized-owner", "headless")
		if err != nil || claim == nil || claim.ID != "checkpoint-command-x-one" {
			t.Fatalf("dev handle permit did not authorize one matching headless command: claim=%+v err=%v", claim, err)
		}
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-one", "claimed")
		assertAccountCheckpointQACommandsQueued(t, state, accountCheckpointQACommands[1:])
	case "fixture-cold-closed":
		if generation := accountCheckpointQAStoredGeneration(t, state); generation != 2 {
			t.Fatalf("authorized generation=%d want 2", generation)
		}
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-one", "claimed")
		assertAccountCheckpointQAClaimDenied(t, state, accountCheckpointQACommands[1:])
	case "fixture-reauthorize":
		blockedFence, err := state.BeginSyntheticCheckpointFence(ctx, accountCheckpointQAID, 2)
		if err != nil {
			t.Fatal(err)
		}
		blockedMetadata := readAccountCheckpointMetadataSnapshot(t, state)
		blockedCommand := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command-x-one")
		blockedReceipt := accountCheckpointQAReceipt(state, blockedFence, accountCheckpointQACheckpoint(), accountCheckpointQAEpoch, accountCheckpointQAGen)
		if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, blockedFence, blockedReceipt); err == nil {
			t.Fatal("reauthorization proceeded while x1 remained claimed")
		}
		if got := readAccountCheckpointMetadataSnapshot(t, state); got != blockedMetadata {
			t.Fatalf("active-claim rejection changed checkpoint/fence metadata: got=%+v want=%+v", got, blockedMetadata)
		}
		if got := loadCapturePauseCommandSnapshot(t, state, "checkpoint-command-x-one"); got != blockedCommand {
			t.Fatalf("active-claim rejection changed the claimed row: got=%+v want=%+v", got, blockedCommand)
		}
		if _, err := state.FailCommand(ctx, "checkpoint-command-x-one", "checkpoint-run-x-one", domain.Failure{
			Code: "synthetic-stage-drain", Stage: "capture", Message: "synthetic cross-build checkpoint drained the earlier claim",
		}); err != nil {
			t.Fatalf("could not drain prior synthetic claim: %v", err)
		}
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-one", "failed")
		checkpoint := accountCheckpointQACheckpoint()
		fence, err := state.BeginSyntheticCheckpointFence(ctx, accountCheckpointQAID, 2)
		if err != nil {
			t.Fatal(err)
		}
		receipt := accountCheckpointQAReceipt(state, fence, checkpoint, accountCheckpointQAEpoch, accountCheckpointQAGen)
		if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err != nil {
			t.Fatal(err)
		}
		if generation := accountCheckpointQAStoredGeneration(t, state); generation != 3 {
			t.Fatalf("reauthorized generation=%d want 3", generation)
		}
		claim, err := state.ClaimCommandForCollector(ctx, "checkpoint-run-x-two", "fixture-reauthorized-owner", "headless")
		if err != nil || claim == nil || claim.ID != "checkpoint-command-x-two" {
			t.Fatalf("fixture reauthorization did not grant the current handle: claim=%+v err=%v", claim, err)
		}
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-two", "claimed")
		assertAccountCheckpointQACommandsQueued(t, state, accountCheckpointQACommands[1:3])
	default:
		t.Fatalf("stage %q is not supported by a fixture-enabled build", stage)
	}
}

func assertAccountCheckpointQACommandsQueued(t *testing.T, state *Store, commands []struct {
	sessionID string
	source    string
	runID     string
	commandID string
}) {
	t.Helper()
	for _, command := range commands {
		assertAccountCheckpointQACommandStatus(t, state, command.commandID, "queued")
	}
}
