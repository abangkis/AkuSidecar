//go:build !auth_session_dev

package main

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/config"
	"io"
)

func dispatchDevelopmentSession(options config.Options, _ io.Reader, _ io.Writer) (bool, error) {
	if options.DevelopmentSessionReuse != "" {
		return false, errors.New("development session integration requires an auth_session_dev build")
	}
	return false, nil
}
