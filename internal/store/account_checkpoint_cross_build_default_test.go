//go:build !auth_session_fixture && !auth_session_dev

package store

import "testing"

func runAccountCheckpointCrossBuildStage(t *testing.T, root, stage string) {
	t.Helper()
	state := openAccountCheckpointQAStore(t, root)
	switch stage {
	case "ordinary-closed":
		if got := accountCheckpointQAStoredGeneration(t, state); got != 1 {
			t.Fatalf("enrolled generation=%d want 1", got)
		}
		assertAccountCheckpointQAClaimDenied(t, state, accountCheckpointQACommands)
	case "ordinary-cold-closed":
		if got := accountCheckpointQAStoredGeneration(t, state); got != 2 {
			t.Fatalf("authorized generation=%d want 2", got)
		}
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-one", "claimed")
		assertAccountCheckpointQAClaimDenied(t, state, accountCheckpointQACommands[1:])
	case "ordinary-final-closed":
		if got := accountCheckpointQAStoredGeneration(t, state); got != 3 {
			t.Fatalf("reauthorized generation=%d want 3", got)
		}
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-one", "failed")
		assertAccountCheckpointQACommandStatus(t, state, "checkpoint-command-x-two", "claimed")
		assertAccountCheckpointQAClaimDenied(t, state, accountCheckpointQACommands[1:3])
	default:
		t.Fatalf("stage %q is not supported by the ordinary build", stage)
	}
}
