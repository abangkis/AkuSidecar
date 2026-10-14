//go:build auth_session_dev && (!auth_session_fixture || !windows)

package main

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/config"
	"io"
)

func dispatchDevelopmentSession(options config.Options, _ io.Reader, _ io.Writer) (bool, error) {
	if options.DevelopmentSessionReuse != "" {
		return false, errors.New("development session provider unavailable in this build")
	}
	return false, nil
}
