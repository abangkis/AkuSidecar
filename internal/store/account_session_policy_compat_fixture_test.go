//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"os"
	"testing"
)

func TestSyntheticAccountPolicyCompatibilityStage(t *testing.T) {
	stage := os.Getenv("AKU_POLICY_STAGE")
	if stage == "" || stage == "closed" {
		t.Skip("explicit policy stage required")
	}
	root := os.Getenv(accountCheckpointQARootEnv)
	if root == "" {
		t.Fatal("missing QA root")
	}
	s := openAccountCheckpointQAStore(t, root)
	ctx := context.Background()
	if stage == "enroll" {
		seedAccountCheckpointQADatabase(t, s)
		if err := s.CreateSyntheticAccountCheckpoint(ctx, accountCheckpointQACheckpoint()); err != nil {
			t.Fatal(err)
		}
		return
	}
	cp := policyCheckpoint(t, s)
	switch stage {
	case "migrate":
		if err := s.ApplySyntheticAccountPolicy(ctx, cp, 0, "migrate"); err != nil {
			t.Fatal(err)
		}
		if err := policyAuthorize(t, s, "revision-1"); err == nil {
			t.Fatal("migrated policy auto-enabled")
		}
	case "enable", "reenable":
		generation := uint64(1)
		if stage == "reenable" {
			generation = 3
		}
		if err := s.ApplySyntheticAccountPolicy(ctx, cp, generation, "enable"); err != nil {
			t.Fatal(err)
		}
		fence, err := s.BeginSyntheticCheckpointFence(ctx, cp.CheckpointID, cp.Generation)
		if err != nil {
			t.Fatal(err)
		}
		receipt := accountCheckpointQAReceipt(s, fence, cp, accountCheckpointQAEpoch, accountCheckpointQAGen)
		if err := s.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err != nil {
			t.Fatal(err)
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		gate, err := s.currentAccountCheckpointClaimGate(ctx, tx)
		_ = tx.Rollback()
		if err != nil || !gate.allowed {
			t.Fatal("verified policy permit unavailable", err)
		}
	case "session-change":
		before := readAccountCheckpointMetadataSnapshot(t, s)
		if err := policyAuthorize(t, s, "changed-revision"); err == nil {
			t.Fatal("session changed silently")
		}
		if after := policyCheckpoint(t, s); after.Generation != cp.Generation || expectedAccountDigest(after) != expectedAccountDigest(cp) {
			t.Fatal("session change rewrote enrollment")
		}
		// A new fence is intentional; checkpoint and required marker stay exact.
		after := readAccountCheckpointMetadataSnapshot(t, s)
		if before.checkpoint != after.checkpoint || before.required != after.required {
			t.Fatal("session rejection changed durable identity")
		}
	case "disable":
		if err := s.ApplySyntheticAccountPolicy(ctx, cp, 2, "disable"); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unknown stage", stage)
	}
	assertAccountCheckpointQACommandsQueued(t, s, accountCheckpointQACommands)
	t.Log("policyStage=" + stage + " sameDatabase=true queuedWorkPreserved=true")
}
