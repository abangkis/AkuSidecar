package store

// This file is also copied unchanged into the pinned pre-policy source tree.
// It uses only pre-policy APIs to prove the historical guard closes admission.
import (
	"context"
	"os"
	"reflect"
	"testing"
)

func TestAccountSessionPolicyDowngradeProbe(t *testing.T) {
	if os.Getenv("AKU_POLICY_STAGE") != "closed" {
		t.Skip("explicit synthetic policy compatibility stage required")
	}
	root := os.Getenv(accountCheckpointQARootEnv)
	if root == "" {
		t.Fatal("missing QA root")
	}
	s := openAccountCheckpointQAStore(t, root)
	before := map[string]capturePauseCommandSnapshot{}
	for _, item := range accountCheckpointQACommands {
		before[item.commandID] = loadCapturePauseCommandSnapshot(t, s, item.commandID)
	}
	var raw string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='synthetic_account_checkpoint_policy'`).Scan(&raw); err != nil {
		t.Fatal("policy absent", err)
	}
	for _, item := range accountCheckpointQACommands {
		claim, err := s.ClaimCommandForCollector(context.Background(), item.runID, "downgrade-probe", "headless")
		if claim != nil {
			t.Fatal("downgrade admitted command", err)
		}
		if after := loadCapturePauseCommandSnapshot(t, s, item.commandID); !reflect.DeepEqual(before[item.commandID], after) {
			t.Fatal("downgrade mutated queue")
		}
	}
	var after string
	if err := s.db.QueryRow(`SELECT value FROM meta WHERE key='synthetic_account_checkpoint_policy'`).Scan(&after); err != nil || after != raw {
		t.Fatal("downgrade changed policy", err)
	}
	t.Log("sameDatabase=true policyRetained=true allQueuedCommandsDenied=true payloadAndClaimFieldsUnchanged=true")
}
