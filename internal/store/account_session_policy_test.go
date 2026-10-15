package store

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAccountSessionPolicyMalformedAndDisabledCloseAdmission(t *testing.T) {
	for _, mode := range []string{"disabled", "future-version", "foreign-provider", "wrong-identity", "extra-field", "missing-checkpoint"} {
		t.Run(mode, func(t *testing.T) {
			s := openTestStore(t)
			insertAccountCheckpointCommand(t, s, "session", "x", "run", "command", "headless", "synthetic-owner", 17)
			cp, raw, required, fence := accountCheckpointMetadataFixture()
			installAccountCheckpointMetadata(t, s, raw, required, fence)
			policy := accountSessionPolicy{Domain: "aku.account-session-policy", Version: 1, Provider: "synthetic-fixture", CheckpointID: cp.CheckpointID, ExpectedSHA256: expectedAccountDigest(cp), Generation: 1}
			switch mode {
			case "future-version":
				policy.Version = 2
			case "foreign-provider":
				policy.Provider = "real-source"
			case "wrong-identity":
				policy.ExpectedSHA256 = "wrong"
			}
			encoded, _ := json.Marshal(policy)
			if mode == "extra-field" {
				encoded = append(encoded[:len(encoded)-1], []byte(`,"override":true}`)...)
			}
			if _, err := s.db.Exec(`INSERT INTO meta(key,value) VALUES(?,?)`, accountSessionPolicyKey, string(encoded)); err != nil {
				t.Fatal(err)
			}
			if mode == "missing-checkpoint" {
				if _, err := s.db.Exec(`DELETE FROM meta WHERE key=?`, syntheticAccountCheckpointKey); err != nil {
					t.Fatal(err)
				}
			}
			before := loadCapturePauseCommandSnapshot(t, s, "command")
			claim, err := s.ClaimCommandForCollector(context.Background(), "run", "test", "headless")
			if claim != nil || mode == "disabled" && err != nil || mode != "disabled" && err == nil {
				t.Fatal("policy admitted claim", err)
			}
			if after := loadCapturePauseCommandSnapshot(t, s, "command"); after != before {
				t.Fatal("policy rejection mutated queue")
			}
		})
	}
}
