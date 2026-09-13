//go:build !windows

package store

import (
	"errors"
	"os"
)

func lockDatabaseFile(path string) (*os.File, error) {
	return nil, errors.New("offline database preparation currently requires Windows exclusive file locking")
}
