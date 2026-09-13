package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// PrepareDatabase stages migration away from the original database. Original
// files are archived only after preparation succeeds. Browser profiles are never
// included. Exclusive OS handles prevent a live SQLite owner from being moved.
func PrepareDatabase(path, action string, confirmed bool, defaults domain.Settings) (DatabaseCompatibility, error) {
	return PrepareDatabaseExpected(path, action, confirmed, InspectDatabase(path).Fingerprint, defaults)
}

func PrepareDatabaseExpected(path, action string, confirmed bool, expected string, defaults domain.Settings) (DatabaseCompatibility, error) {
	r := InspectDatabase(path)
	if expected == "" || expected != r.Fingerprint {
		return r, errors.New("database changed since inspection or cannot be fingerprinted; inspect and confirm again")
	}
	if !confirmed {
		return r, errors.New("database action requires explicit confirmation")
	}
	if action != "fresh" && action != "migrate" {
		return r, errors.New("invalid database action")
	}
	if action == "migrate" && r.Status != "migratable" {
		return r, fmt.Errorf("migration unavailable for %s database", r.Status)
	}
	if r.Status == "absent" {
		return r, errors.New("no existing database to prepare")
	}
	paths := []string{path, path + "-wal", path + "-shm", path + "-journal", filepath.Join(filepath.Dir(path), RuntimeVersionMarkerName)}
	var locked []*os.File
	defer func() {
		for _, f := range locked {
			f.Close()
		}
	}()
	for _, p := range paths {
		f, err := lockDatabaseFile(p)
		if errors.Is(err, os.ErrNotExist) {
			if p == path {
				return r, errors.New("database disappeared during preparation")
			}
			continue
		}
		if err != nil {
			return r, fmt.Errorf("database must be closed before preparation: %w", err)
		}
		locked = append(locked, f)
	}
	if len(locked) == 0 {
		return r, errors.New("database changed during preparation")
	}
	stage, err := os.MkdirTemp(filepath.Dir(path), "database-preparation-")
	if err != nil {
		return r, err
	}
	// Retain failed staging directories for diagnosis; originals remain untouched.
	for _, f := range locked {
		if err = copyDatabaseFile(f, filepath.Join(stage, filepath.Base(f.Name()))); err != nil {
			return r, err
		}
	}
	stagedPath := filepath.Join(stage, filepath.Base(path))
	stagedFingerprint, err := databaseFingerprint(stagedPath)
	if err != nil || stagedFingerprint != expected {
		return r, errors.New("locked database differs from confirmed inspection; original preserved")
	}
	staged := InspectDatabase(stagedPath)
	if staged.Status != r.Status || staged.DatabaseSchemaVersion != r.DatabaseSchemaVersion {
		return r, errors.New("database compatibility changed since inspection; original preserved")
	}
	if action == "migrate" {
		if staged.Status != "migratable" || staged.DatabaseSchemaVersion != r.DatabaseSchemaVersion {
			return r, errors.New("database changed since inspection")
		}
		s, openErr := Open(stagedPath, defaults)
		if openErr != nil {
			return r, fmt.Errorf("staged migration failed; original preserved: %w", openErr)
		}
		if err = s.Close(); err != nil {
			return r, err
		}
		if check := InspectDatabase(stagedPath); check.Status != "current" {
			return r, errors.New("staged migration verification failed")
		}
	}
	archive, err := os.MkdirTemp(filepath.Dir(path), "database-backup-")
	if err != nil {
		return r, err
	}
	var moved []string
	rollback := func() error {
		var restoreErr error
		for i := len(moved) - 1; i >= 0; i-- {
			p := moved[i]
			restoreErr = errors.Join(restoreErr, os.Rename(filepath.Join(archive, filepath.Base(p)), p))
		}
		return restoreErr
	}
	for _, f := range locked {
		p := f.Name()
		if err = os.Rename(p, filepath.Join(archive, filepath.Base(p))); err != nil {
			return r, errors.Join(err, rollback())
		}
		moved = append(moved, p)
	}
	r.BackupPath = archive
	if action == "migrate" {
		// Close() checkpoints the isolated connection; refuse stray WAL rather than
		// activating a partial database image.
		if info, e := os.Stat(stagedPath + "-wal"); e == nil && info.Size() > 0 {
			return r, errors.Join(errors.New("staged WAL remains; activation cancelled"), rollback())
		}
		if err = os.Rename(stagedPath, path); err != nil {
			return r, errors.Join(err, rollback())
		}
		// The archived marker remains recoverable. The normal startup writes its
		// current marker after successfully opening the migrated database.
		r.Status = "current"
		r.DatabaseSchemaVersion = SchemaVersion
	} else {
		r.Status = "absent"
		r.DatabaseSchemaVersion = 0
	}
	return r, nil
}

func copyDatabaseFile(source *os.File, target string) error {
	if _, err := source.Seek(0, 0); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(out, hash), source)
	syncErr := out.Sync()
	closeErr := out.Close()
	if err = errors.Join(err, syncErr, closeErr); err != nil {
		return err
	}
	check, err := os.Open(target)
	if err != nil {
		return err
	}
	defer check.Close()
	verified := sha256.New()
	if _, err := io.Copy(verified, check); err != nil {
		return err
	}
	if string(hash.Sum(nil)) != string(verified.Sum(nil)) {
		return errors.New("database backup copy verification failed")
	}
	return nil
}
