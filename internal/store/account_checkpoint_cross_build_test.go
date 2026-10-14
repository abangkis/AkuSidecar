package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	accountCheckpointQARootEnv  = "AKUSIDECA_ACCOUNT_CHECKPOINT_QA_ROOT"
	accountCheckpointQAStageEnv = "AKUSIDECA_ACCOUNT_CHECKPOINT_QA_STAGE"
	accountCheckpointQADBName   = "account-checkpoint-cross-build.db"
	accountCheckpointQAID       = "checkpoint-cross-build"
	accountCheckpointQAEpoch    = "fixture-owner-epoch"
	accountCheckpointQAGen      = uint64(41)
)

var accountCheckpointQACommands = []struct {
	sessionID string
	source    string
	runID     string
	commandID string
}{
	{"checkpoint-session-main", "x", "checkpoint-run-x-one", "checkpoint-command-x-one"},
	{"checkpoint-session-main", "instagram", "checkpoint-run-instagram", "checkpoint-command-instagram"},
	{"checkpoint-session-main", "linkedin", "checkpoint-run-linkedin", "checkpoint-command-linkedin"},
	{"checkpoint-session-extra", "x", "checkpoint-run-x-two", "checkpoint-command-x-two"},
}

func TestAccountCheckpointCrossBuildStage(t *testing.T) {
	root := strings.TrimSpace(os.Getenv(accountCheckpointQARootEnv))
	stage := strings.TrimSpace(os.Getenv(accountCheckpointQAStageEnv))
	if stage == "" {
		t.Skip("account checkpoint cross-build stages require an explicit opt-in QA stage")
	}
	if root == "" {
		t.Fatalf("both %s and %s are required", accountCheckpointQARootEnv, accountCheckpointQAStageEnv)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(absoluteRoot)
	if err != nil || !info.IsDir() {
		t.Fatalf("QA root must already exist as a directory: %s (%v)", absoluteRoot, err)
	}
	runAccountCheckpointCrossBuildStage(t, absoluteRoot, stage)
}

func openAccountCheckpointQAStore(t *testing.T, root string) *Store {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := Open(filepath.Join(root, accountCheckpointQADBName), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	return state
}

func accountCheckpointQACheckpoint() SyntheticAccountCheckpoint {
	return SyntheticAccountCheckpoint{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: accountCheckpointQAID, Generation: 1,
		ExpectedAccounts: map[domain.Source]SyntheticAccountIdentity{
			domain.SourceX:         {AccountID: "x-account-fixture", StoreScopeID: "shared-store-scope", ProfileID: "headless-profile"},
			domain.SourceInstagram: {AccountID: "instagram-account-fixture", StoreScopeID: "shared-store-scope", ProfileID: "headless-profile"},
			domain.SourceLinkedIn:  {AccountID: "linkedin-account-fixture", StoreScopeID: "shared-store-scope", ProfileID: "headless-profile"},
		},
	}
}

func accountCheckpointQAReceipt(state *Store, fence SyntheticCheckpointFence, checkpoint SyntheticAccountCheckpoint, ownerEpoch string, ownerGeneration uint64) SyntheticVerifiedReceipt {
	receipt := SyntheticVerifiedReceipt{
		Domain: syntheticAccountCheckpointDomain, CheckpointID: fence.CheckpointID, Generation: fence.Generation, HoldID: fence.HoldID,
		Accounts:      make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		StoreScopeIDs: make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		ProfileIDs:    make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		Revisions:     make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		OwnerEpoch:    ownerEpoch, HeadlessGeneration: ownerGeneration, OwnerPID: os.Getpid(), CapturedAt: state.Now(),
	}
	for source, identity := range checkpoint.ExpectedAccounts {
		receipt.Accounts[source] = identity.AccountID
		receipt.StoreScopeIDs[source] = identity.StoreScopeID
		receipt.ProfileIDs[source] = identity.ProfileID
		receipt.Revisions[source] = string(source) + "-revision-fixture-1"
	}
	return receipt
}

func seedAccountCheckpointQADatabase(t *testing.T, state *Store) {
	t.Helper()
	for _, command := range accountCheckpointQACommands {
		insertAccountCheckpointCommand(t, state, command.sessionID, command.source, command.runID, command.commandID, "headless", accountCheckpointQAEpoch, int(accountCheckpointQAGen))
	}
}

func assertAccountCheckpointQACommandStatus(t *testing.T, state *Store, id, wantStatus string) {
	t.Helper()
	snapshot := loadCapturePauseCommandSnapshot(t, state, id)
	if snapshot.status != wantStatus {
		t.Fatalf("command %s status=%q want %q", id, snapshot.status, wantStatus)
	}
	if wantStatus == "queued" && (snapshot.claimedAt.Valid || snapshot.claimedBy.Valid) {
		t.Fatalf("queued command %s has claim metadata: %+v", id, snapshot)
	}
}

func assertAccountCheckpointQAClaimDenied(t *testing.T, state *Store, commands []struct {
	sessionID string
	source    string
	runID     string
	commandID string
}) {
	t.Helper()
	ctx := context.Background()
	for _, item := range commands {
		claim, err := state.ClaimCommandForCollector(ctx, item.runID, "checkpoint-cross-build-consumer", "headless")
		if err != nil || claim != nil {
			t.Fatalf("command %s was claimable from a cold build: claim=%+v err=%v", item.commandID, claim, err)
		}
		assertAccountCheckpointQACommandStatus(t, state, item.commandID, "queued")
	}
}

func accountCheckpointQAStoredGeneration(t *testing.T, state *Store) uint64 {
	t.Helper()
	var raw string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key=?`, syntheticAccountCheckpointKey).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var checkpoint SyntheticAccountCheckpoint
	if err := json.Unmarshal([]byte(raw), &checkpoint); err != nil {
		t.Fatal(err)
	}
	return checkpoint.Generation
}
