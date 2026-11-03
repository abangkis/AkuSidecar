package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) shouldAdoptInstagramDefault(ctx context.Context, active []domain.Source) (bool, error) {
	var marker string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='source_default_instagram_v1'`).Scan(&marker)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read Instagram default migration: %w", err)
	}
	return len(active) == 3 &&
		active[0] == domain.SourceX &&
		active[1] == domain.SourceLinkedIn &&
		active[2] == domain.SourceFacebook, nil
}

func (s *Store) syncSourceDefinitions(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source registry sync: %w", err)
	}
	defer tx.Rollback()
	for ordinal, descriptor := range domain.Sources() {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_definitions(id,display_name,ordinal,enabled)
			VALUES(?,?,?,1)
			ON CONFLICT(id) DO UPDATE SET
				display_name=excluded.display_name,
				ordinal=excluded.ordinal,
				enabled=excluded.enabled`, descriptor.ID, descriptor.DisplayName, ordinal); err != nil {
			return fmt.Errorf("sync source definition %s: %w", descriptor.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source registry sync: %w", err)
	}
	return nil
}
