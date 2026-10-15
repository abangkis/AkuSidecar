package store

import (
	"encoding/json"
	"errors"
)

// Keep this key under the historical prefix. Guard-aware older binaries reject
// the fourth record, including their tagged authorization APIs. Never delete it
// on opt-out or rename the legacy keys during a downgrade.
const accountSessionPolicyKey = "synthetic_account_checkpoint_policy"

type accountSessionPolicy struct {
	Domain         string `json:"domain"`
	Version        int    `json:"version"`
	Provider       string `json:"provider"`
	CheckpointID   string `json:"checkpointId"`
	ExpectedSHA256 string `json:"expectedSha256"`
	Generation     uint64 `json:"generation"`
	Enabled        bool   `json:"enabled"`
}

func expectedAccountDigest(checkpoint SyntheticAccountCheckpoint) string {
	// Account IDs and scope only: authentication observations never enroll users.
	raw, _ := json.Marshal(checkpoint.ExpectedAccounts)
	return checkpointSHA256(string(raw))
}

func readAccountSessionPolicy(values map[string]string, checkpoint SyntheticAccountCheckpoint) (*accountSessionPolicy, error) {
	raw, exists := values[accountSessionPolicyKey]
	if !exists {
		return nil, nil
	}
	var policy accountSessionPolicy
	if strictCheckpointJSON(raw, &policy) != nil || policy.Domain != "aku.account-session-policy" || policy.Version != 1 ||
		policy.Provider != "synthetic-fixture" || policy.CheckpointID != checkpoint.CheckpointID ||
		policy.ExpectedSHA256 != expectedAccountDigest(checkpoint) || policy.Generation == 0 || policy.Generation > accountCheckpointMaxGeneration {
		return nil, errors.New("account session policy invalid or unsupported")
	}
	return &policy, nil
}

func accountCheckpointRecordCountValid(values map[string]string) bool {
	_, policy := values[accountSessionPolicyKey]
	return len(values) == 3 && !policy || len(values) == 4 && policy
}
