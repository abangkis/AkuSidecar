//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"os"
	"testing"
)

// Run only in the pinned old source snapshot, where the new policy is unknown.
func TestAccountSessionPolicyOldAuthorizationRejected(t *testing.T) {
	if os.Getenv("AKU_POLICY_OLD_PROBE") != "1" {
		t.Skip("old binary compatibility only")
	}
	s := openAccountCheckpointQAStore(t, os.Getenv(accountCheckpointQARootEnv))
	before := readAccountCheckpointMetadataSnapshot(t, s)
	generation := accountCheckpointQAStoredGeneration(t, s)
	if _, err := s.BeginSyntheticCheckpointFence(context.Background(), accountCheckpointQAID, generation); err == nil {
		t.Fatal("old binary accepted new policy fence")
	}
	if after := readAccountCheckpointMetadataSnapshot(t, s); after != before {
		t.Fatal("old authorization altered metadata")
	}
	for _, item := range accountCheckpointQACommands {
		claim, err := s.ClaimCommandForCollector(context.Background(), item.runID, "old-policy-probe", "headless")
		if claim != nil || err == nil {
			t.Fatal("old binary did not reject unknown policy", err)
		}
	}
}
