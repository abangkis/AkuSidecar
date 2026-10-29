package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"io"
	"strings"
)

const xDailyPermissionMetaKey = "x_daily_permission_v1"

type xDailyPermissionRecord struct {
	Version      *int    `json:"version"`
	Enabled      *bool   `json:"enabled"`
	ProfileScope *string `json:"profileScope"`
}

// XDailyPermission returns whether daily X access was explicitly enabled for
// the supplied reader-profile scope. Missing or differently scoped records are
// treated as disabled.
func (s *Store) XDailyPermission(ctx context.Context, profileScope string) (bool, error) {
	return s.DailySourcePermission(ctx, domain.SourceX, profileScope)
}

func (s *Store) DailySourcePermission(ctx context.Context, source domain.Source, profileScope string) (bool, error) {
	key, err := dailyPermissionKey(source)
	if err != nil {
		return false, err
	}
	if err := validateXDailyPermissionProfileScope(profileScope); err != nil {
		return false, err
	}

	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read X daily permission: %w", err)
	}

	record, err := decodeXDailyPermissionRecord(raw)
	if err != nil {
		return false, fmt.Errorf("decode X daily permission: %w", err)
	}
	if *record.ProfileScope != profileScope {
		return false, nil
	}
	return *record.Enabled, nil
}

// SaveXDailyPermission atomically stores the explicit daily X permission for a
// reader-profile scope. Passing enabled=false persists a revocation.
func (s *Store) SaveXDailyPermission(ctx context.Context, profileScope string, enabled bool) error {
	return s.SaveDailySourcePermission(ctx, domain.SourceX, profileScope, enabled)
}

func (s *Store) SaveDailySourcePermission(ctx context.Context, source domain.Source, profileScope string, enabled bool) error {
	key, err := dailyPermissionKey(source)
	if err != nil {
		return err
	}
	if err := validateXDailyPermissionProfileScope(profileScope); err != nil {
		return err
	}
	version := 1
	record := xDailyPermissionRecord{Version: &version, Enabled: &enabled, ProfileScope: &profileScope}
	raw, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode X daily permission: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(raw)); err != nil {
		return fmt.Errorf("save X daily permission: %w", err)
	}
	return nil
}

func validateXDailyPermissionProfileScope(profileScope string) error {
	if len(profileScope) != 64 {
		return errors.New("X daily permission profile scope must be 64 lowercase hexadecimal characters")
	}
	for _, char := range profileScope {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return errors.New("X daily permission profile scope must be 64 lowercase hexadecimal characters")
		}
	}
	return nil
}

func decodeXDailyPermissionRecord(raw string) (xDailyPermissionRecord, error) {
	fields, err := decodeXDailyPermissionFields(raw)
	if err != nil {
		return xDailyPermissionRecord{}, err
	}
	if len(fields) != 3 {
		return xDailyPermissionRecord{}, errors.New("record must contain exactly version, enabled, and profileScope")
	}
	for _, field := range []string{"version", "enabled", "profileScope"} {
		if _, ok := fields[field]; !ok {
			return xDailyPermissionRecord{}, fmt.Errorf("record is missing %s", field)
		}
	}

	var record xDailyPermissionRecord
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return xDailyPermissionRecord{}, err
	}
	if record.Version == nil || record.Enabled == nil || record.ProfileScope == nil {
		return xDailyPermissionRecord{}, errors.New("record fields must not be null")
	}
	if *record.Version != 1 {
		return xDailyPermissionRecord{}, fmt.Errorf("unsupported record version %d", *record.Version)
	}
	if err := validateXDailyPermissionProfileScope(*record.ProfileScope); err != nil {
		return xDailyPermissionRecord{}, fmt.Errorf("invalid stored profile scope: %w", err)
	}
	return record, nil
}

func decodeXDailyPermissionFields(raw string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	start, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return nil, errors.New("record must be a JSON object")
	}

	fields := make(map[string]json.RawMessage, 3)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, errors.New("record field name must be a string")
		}
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("record contains duplicate field %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if closing, err := decoder.Token(); err != nil {
		return nil, err
	} else if delimiter, ok := closing.(json.Delim); !ok || delimiter != '}' {
		return nil, errors.New("record object is not closed")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("record contains trailing JSON data")
		}
		return nil, err
	}
	return fields, nil
}

func dailyPermissionKey(source domain.Source) (string, error) {
	switch source {
	case domain.SourceX:
		return xDailyPermissionMetaKey, nil
	case domain.SourceInstagram:
		return "instagram_daily_permission_v1", nil
	default:
		return "", errors.New("unsupported daily permission source")
	}
}
