//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// A prepared command is still unclaimed work. Require a real queued SQLite row
// so a blocked claim is not a vacuous test against a nonexistent command.
func ValidateDevelopmentLiveReaderQueue(run domain.Run, commands []store.FixtureCommandEvidence) error {
	if run.ID == "" || run.SessionID == "" || run.Ordinal != 1 || run.Status != "waiting_for_bridge" || run.Stage != "capture" || run.BridgeCommandStatus != "queued" || run.StartedAt == nil || run.CompletedAt != nil || run.Error != nil || len(commands) != 1 {
		return errors.New("live-reader original continuation is not prepared unclaimed work")
	}
	c := commands[0]
	if c.ID == "" || c.RunID != run.ID || c.Type != "collect_visible" || c.Status != "queued" || c.Collector != "headless" || c.Driver != "headless" || len(c.PayloadSHA256) != 64 || c.ClaimedBy.Valid || c.ClaimedAt.Valid || c.CompletedAt.Valid || c.ErrorJSON.Valid {
		return errors.New("live-reader queued command is not never-claimed headless work")
	}
	return nil
}
