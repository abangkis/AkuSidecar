package store

import "context"

// A challenge may pause X only for an active, exact pilot claim. Stale results
// cannot relatch a recovered source. The normal pause transaction owns fan-out.
func (s *Store) FailManualPilotChallenge(ctx context.Context, h *ManualPilotClaim, category string) error {
	if h == nil || h.permit == nil || h.permit.store != s {
		return errManualPilot
	}
	if category != "cloudflare" && category != "x_human_verification" {
		category = ""
	}
	s.manualPilotMu.Lock()
	defer s.manualPilotMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.liveManualPilot(ctx, tx, h.permit); err != nil {
		return err
	}
	var active int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands WHERE id=? AND run_id=? AND status='claimed' AND claimed_by=?`, h.commandID, h.permit.state.RunID, h.binding).Scan(&active); err != nil {
		return err
	}
	if active != 1 {
		return errManualPilot
	}
	if err = s.pauseXCaptureTx(ctx, tx, "", category); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	h.permit.revoked.Store(true)
	return nil
}
