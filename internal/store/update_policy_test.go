package store

import (
	"context"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func createVisibleUpdateSession(s *Store, ctx context.Context, intent string, settings domain.Settings) (domain.Session, error) {
	return s.CreateUpdateSession(ctx, intent, settings, visibleUpdatePolicy())
}

func createPreparedUpdateSession(s *Store, ctx context.Context, intent string, settings domain.Settings) (domain.Session, error) {
	return s.CreateUpdateSession(ctx, intent, settings, preparedUpdatePolicy())
}
