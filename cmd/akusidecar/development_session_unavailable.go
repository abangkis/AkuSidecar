//go:build auth_session_dev && (!auth_session_fixture || !windows)

package main

import (
	"io"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/diagnostics/operational"
)

func dispatchDevelopmentSession(options config.Options, _ io.Reader, _ io.Writer) (bool, error) {
	if options.DevelopmentSessionReuse != "" {
		return false, operational.DevProviderUnavailable
	}
	return false, nil
}
