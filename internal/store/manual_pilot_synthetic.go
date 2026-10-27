//go:build auth_session_dev

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// ManualPilotSyntheticRequest opts into synthetic Store-contract exercises.
// It carries no native account-verification authority.
type ManualPilotSyntheticRequest struct {
	FactoryOptIn                                          bool
	SessionID, RunID, SyntheticOwnerRef, SyntheticPairRef string
	StartedAt                                             time.Time
	// ValidUntil may shorten, never extend, the original synthetic attempt.
	ValidUntil time.Time
}

// IssueSyntheticManualPilot exists only in development builds. It cannot
// enroll an existing checkpoint or issue authority for real owner identities.
func (s *Store) IssueSyntheticManualPilot(ctx context.Context, r ManualPilotSyntheticRequest) (*ManualPilotPermit, error) {
	if !r.FactoryOptIn || !strings.HasPrefix(r.SyntheticOwnerRef, "synthetic:") || !strings.HasPrefix(r.SyntheticPairRef, "synthetic:") || len(r.SyntheticOwnerRef) <= 10 || len(r.SyntheticPairRef) <= 10 || r.RunID == "" || r.SessionID == "" {
		return nil, errManualPilot
	}
	if r.StartedAt.IsZero() || r.StartedAt.After(s.Now()) || !s.Now().Before(r.StartedAt.Add(60*time.Second)) {
		return nil, errManualPilot
	}
	deadline := r.StartedAt.Add(60 * time.Second)
	if !r.ValidUntil.IsZero() {
		if !r.ValidUntil.After(r.StartedAt) || r.ValidUntil.After(deadline) {
			return nil, errManualPilot
		}
		deadline = r.ValidUntil
	}
	return s.issueDevelopmentManualPilot(ctx, r.SessionID, r.RunID, r.SyntheticOwnerRef, r.SyntheticPairRef, domain.NewID("manual-attempt"), 1, deadline)
}

func (s *Store) issueDevelopmentManualPilot(ctx context.Context, sessionID, runID, owner, pair, attempt string, generation uint64, deadline time.Time) (*ManualPilotPermit, error) {
	return s.issueDevelopmentPilot(ctx, sessionID, runID, owner, pair, attempt, generation, deadline, false)
}
func (s *Store) issueDevelopmentPilot(ctx context.Context, sessionID, runID, owner, pair, attempt string, generation uint64, deadline time.Time, runScoped bool) (*ManualPilotPermit, error) {
	s.manualPilotMu.Lock()
	defer s.manualPilotMu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var n int
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key LIKE 'synthetic_account_checkpoint%'`).Scan(&n); e != nil {
		return nil, e
	}
	if n != 0 {
		return nil, errors.New("existing account checkpoint is incompatible with manual pilot; no migration or relabeling performed")
	}
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key=?`, manualPilotSessionPrefix+sessionID).Scan(&n); e != nil {
		return nil, e
	}
	if n != 0 {
		return nil, errManualPilot
	}
	if e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.session_id=? AND c.status='claimed'`, sessionID).Scan(&n); e != nil {
		return nil, e
	}
	if n != 0 {
		return nil, errManualPilot
	}
	incarnation := ""
	e = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='manual_pilot_incarnation_v1'`).Scan(&incarnation)
	if errors.Is(e, sql.ErrNoRows) {
		incarnation = domain.NewID("manual-db")
		_, e = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('manual_pilot_incarnation_v1',?)`, incarnation)
	}
	if e != nil {
		return nil, e
	}
	v := manualPilotState{SessionID: sessionID, RunID: runID, Source: domain.SourceX, Incarnation: incarnation, Epoch: domain.NewID("manual-epoch"), Attempt: attempt, OwnerRef: owner, PairRef: pair, OwnerGeneration: generation, Revision: 1, RunScoped: runScoped}
	raw, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, manualPilotSessionPrefix+sessionID, string(raw)); e != nil {
		return nil, e
	}
	p := &ManualPilotPermit{store: s, state: v, deadline: deadline}
	if s.manualPilots == nil {
		s.manualPilots = make(map[string]*ManualPilotPermit)
	}
	s.manualPilots[sessionID] = p
	s.manualPilotClosures.Store(sessionID, p)
	if e = s.liveManualPilot(ctx, tx, p); e != nil {
		delete(s.manualPilots, sessionID)
		s.manualPilotClosures.Delete(sessionID)
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		p.revoked.Store(true)
		delete(s.manualPilots, sessionID)
		s.manualPilotClosures.Delete(sessionID)
		return nil, e
	}
	return p, nil
}
