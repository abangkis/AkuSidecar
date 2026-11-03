package store

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

func (s *Store) BridgeToken(ctx context.Context) (string, error) { return s.bridgeToken(ctx) }

func (s *Store) bridgeToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='bridge_token'`).Scan(&token)
	if err == nil {
		return token, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("read bridge token: %w", err)
	}
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate bridge token: %w", err)
	}
	token = hex.EncodeToString(value[:])
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('bridge_token',?)`, token); err != nil {
		return "", fmt.Errorf("save bridge token: %w", err)
	}
	return token, nil
}

func (s *Store) MatchesBridgeToken(ctx context.Context, candidate string) bool {
	expected, err := s.bridgeToken(ctx)
	if err != nil || len(candidate) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(expected)) == 1
}
