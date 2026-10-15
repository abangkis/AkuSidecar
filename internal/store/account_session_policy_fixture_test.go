//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func policyCheckpoint(t *testing.T, s *Store) SyntheticAccountCheckpoint {
	t.Helper()
	raw, _, _ := readAccountCheckpointMetadata(t, s)
	var cp SyntheticAccountCheckpoint
	if err := json.Unmarshal([]byte(raw), &cp); err != nil {
		t.Fatal(err)
	}
	return cp
}

func policyAuthorize(t *testing.T, s *Store, revision string) error {
	t.Helper()
	cp := policyCheckpoint(t, s)
	fence, err := s.BeginSyntheticCheckpointFence(context.Background(), cp.CheckpointID, cp.Generation)
	if err != nil {
		return err
	}
	receipt := accountCheckpointReceipt(s, fence, cp, "synthetic-owner", 17)
	for source := range receipt.Revisions {
		receipt.Revisions[source] = revision
	}
	return s.AuthorizeSyntheticAccountCheckpoint(context.Background(), fence, receipt)
}

func TestSyntheticAccountPolicyEnrollmentOptInSessionAndOptOut(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	insertAccountCheckpointCommand(t, s, "session", "x", "run", "command", "headless", "synthetic-owner", 17)
	before := loadCapturePauseCommandSnapshot(t, s, "command")
	cp := accountCheckpointTestValue(domain.SourceX)
	if err := s.CreateSyntheticAccountCheckpoint(ctx, cp); err != nil {
		t.Fatal(err)
	}
	cp = policyCheckpoint(t, s)
	wrong := cp
	wrong.ExpectedAccounts = copySyntheticAccounts(cp.ExpectedAccounts)
	identity := wrong.ExpectedAccounts[domain.SourceX]
	identity.AccountID = "other-account"
	wrong.ExpectedAccounts[domain.SourceX] = identity
	if err := s.ApplySyntheticAccountPolicy(ctx, wrong, 0, "migrate"); err == nil {
		t.Fatal("migrated unconfirmed different account")
	}
	if err := s.ApplySyntheticAccountPolicy(ctx, cp, 0, "migrate"); err != nil {
		t.Fatal(err)
	}
	if err := policyAuthorize(t, s, "revision-1"); err == nil {
		t.Fatal("migration implicitly enabled authorization")
	}
	if err := s.ApplySyntheticAccountPolicy(ctx, cp, 0, "enable"); err == nil {
		t.Fatal("stale policy confirmation accepted")
	}
	if err := s.ApplySyntheticAccountPolicy(ctx, cp, 1, "enable"); err != nil {
		t.Fatal(err)
	}
	if claim, err := s.ClaimCommandForCollector(ctx, "run", "test", "headless"); err != nil || claim != nil {
		t.Fatal("opt-in granted a permit", err)
	}
	if err := policyAuthorize(t, s, "revision-1"); err != nil {
		t.Fatal(err)
	}
	cp = policyCheckpoint(t, s)
	if err := policyAuthorize(t, s, "revision-2"); err == nil {
		t.Fatal("new session revision silently accepted")
	}
	if got := policyCheckpoint(t, s); !reflect.DeepEqual(got, cp) {
		t.Fatal("changed session overwrote enrollment or revision")
	}
	if claim, err := s.ClaimCommandForCollector(ctx, "run", "test", "headless"); err != nil || claim != nil {
		t.Fatal("failed session proof opened admission", err)
	}
	if err := policyAuthorize(t, s, "revision-1"); err != nil {
		t.Fatal(err)
	}
	cp = policyCheckpoint(t, s)
	if err := s.ApplySyntheticAccountPolicy(ctx, cp, 2, "disable"); err != nil {
		t.Fatal(err)
	}
	if err := policyAuthorize(t, s, "revision-1"); err == nil {
		t.Fatal("disabled policy authorized with valid proof")
	}
	if claim, err := s.ClaimCommandForCollector(ctx, "run", "test", "headless"); err != nil || claim != nil {
		t.Fatal("opt-out admitted claim", err)
	}
	if got := loadCapturePauseCommandSnapshot(t, s, "command"); got != before {
		t.Fatal("policy/session failure changed queued work")
	}
	if err := s.ApplySyntheticAccountPolicy(ctx, cp, 3, "enable"); err != nil {
		t.Fatal(err)
	}
	if err := policyAuthorize(t, s, "revision-1"); err != nil {
		t.Fatal(err)
	}
	if claim, err := s.ClaimCommandForCollector(ctx, "run", "test", "headless"); err != nil || claim == nil {
		t.Fatal("matching confirmed policy/proof cannot claim", err)
	}
	// Disabling admission does not cancel or rewrite an already claimed capture.
	claimed := loadCapturePauseCommandSnapshot(t, s, "command")
	if err := s.ApplySyntheticAccountPolicy(ctx, policyCheckpoint(t, s), 4, "disable"); err != nil {
		t.Fatal(err)
	}
	if got := loadCapturePauseCommandSnapshot(t, s, "command"); got != claimed {
		t.Fatal("opt-out mutated active claim")
	}
}

func TestSyntheticAccountPolicyMigrationInvalidatesOtherHandle(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "two-handles.sqlite")
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	s, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.CreateSyntheticAccountCheckpoint(ctx, accountCheckpointTestValue(domain.SourceX)); err != nil {
		t.Fatal(err)
	}
	if err := policyAuthorize(t, s, "revision-1"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	// The first handle keeps its actual old permit while a different handle
	// migrates and enables the database. Only durable fencing can invalidate it.
	cp := policyCheckpoint(t, s)
	if err := other.ApplySyntheticAccountPolicy(ctx, cp, 0, "migrate"); err != nil {
		t.Fatal(err)
	}
	if err := other.ApplySyntheticAccountPolicy(ctx, cp, 1, "enable"); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	gate, err := s.currentAccountCheckpointClaimGate(ctx, tx)
	if err != nil || gate.allowed || !gate.protected {
		t.Fatal("migration retained stale permit", err)
	}
}

func TestSyntheticAccountPolicyMigrationAtomicFailure(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	if err := s.CreateSyntheticAccountCheckpoint(ctx, accountCheckpointTestValue(domain.SourceX)); err != nil {
		t.Fatal(err)
	}
	before, err := readAccountCheckpointValuesTxForTest(t, s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_policy BEFORE INSERT ON meta WHEN NEW.key='synthetic_account_checkpoint_policy' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplySyntheticAccountPolicy(ctx, policyCheckpoint(t, s), 0, "migrate"); err == nil {
		t.Fatal("failed write reported success")
	}
	after, err := readAccountCheckpointValuesTxForTest(t, s)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("partial migration persisted", err)
	}
}
