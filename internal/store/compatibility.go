package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// DatabaseCompatibility is produced by the executing runtime, not bundle metadata.
type DatabaseCompatibility struct {
	SchemaVersion         int    `json:"schemaVersion"`
	Status                string `json:"status"`
	DatabaseSchemaVersion int    `json:"databaseSchemaVersion"`
	TargetSchemaVersion   int    `json:"targetSchemaVersion"`
	Reason                string `json:"reason"`
	BackupPath            string `json:"backupPath,omitempty"`
	Fingerprint           string `json:"fingerprint,omitempty"`
}

func InspectDatabase(path string) DatabaseCompatibility {
	r := inspectDatabase(path)
	if r.Status != "absent" {
		r.Fingerprint, _ = databaseFingerprint(path)
	}
	return r
}

func databaseFingerprint(path string) (string, error) {
	h := sha256.New()
	for _, p := range []string{path, path + "-wal", path + "-journal", filepath.Join(filepath.Dir(path), RuntimeVersionMarkerName)} {
		f, err := os.Open(p)
		if errors.Is(err, os.ErrNotExist) {
			if p == path {
				return "", err
			}
			fmt.Fprintln(h, filepath.Base(p), "absent")
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintln(h, filepath.Base(p), "present")
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func inspectDatabase(path string) DatabaseCompatibility {
	r := DatabaseCompatibility{SchemaVersion: 1, Status: "unknown", TargetSchemaVersion: SchemaVersion}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		r.Status = "absent"
		return r
	}
	if err != nil {
		r.Reason = err.Error()
		return r
	}
	if !info.Mode().IsRegular() {
		r.Reason = "database is not a regular file"
		return r
	}
	s, err := OpenReadOnly(path)
	if err != nil {
		r.Reason = err.Error()
		return r
	}
	defer s.Close()
	var integrity, version string
	if err = s.db.QueryRowContext(context.Background(), "PRAGMA quick_check").Scan(&integrity); err != nil || integrity != "ok" {
		r.Reason = fmt.Sprintf("database integrity could not be verified: %v %s", err, integrity)
		return r
	}
	if err = s.db.QueryRowContext(context.Background(), "SELECT value FROM meta WHERE key='schema_version'").Scan(&version); err != nil {
		r.Reason = err.Error()
		return r
	}
	r.DatabaseSchemaVersion, err = strconv.Atoi(version)
	if err != nil {
		r.Reason = "invalid schema version"
		return r
	}
	if err = validateDataRuntimeVersion(path, domain.ApplicationVersion); err != nil {
		var newer *NewerRuntimeDataError
		if errors.As(err, &newer) {
			r.Status = "newer"
		}
		r.Reason = err.Error()
		return r
	}
	switch {
	case r.DatabaseSchemaVersion == SchemaVersion:
		r.Status = "current"
	case canMigrateSchema(r.DatabaseSchemaVersion):
		r.Status = "migratable"
	case r.DatabaseSchemaVersion > SchemaVersion:
		r.Status = "newer"
	default:
		r.Status = "unsupported"
	}
	return r
}
