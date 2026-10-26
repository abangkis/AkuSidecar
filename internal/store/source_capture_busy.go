package store

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// SourceCaptureBusy is a preliminary read only. Durable claim transactions
// remain authoritative if another process races application reservation.
func (s *Store) SourceCaptureBusy(ctx context.Context, source domain.Source) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	n, err := activeSourceCaptureCount(ctx, tx, source)
	return n != 0, err
}
