//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"io"
)

func dispatchDevelopmentSession(options config.Options, input io.Reader, output io.Writer) (bool, error) {
	if options.DevelopmentSessionReuse == "" {
		return false, nil
	}
	if !options.Dev || (options.DevelopmentSessionReuse != "synthetic-fixture" && options.DevelopmentSessionReuse != "synthetic-broker-fixture") {
		return false, errors.New("development session requires --dev and supported synthetic-fixture provider")
	}
	if options.DevelopmentSessionReuse == "synthetic-broker-fixture" {
		return true, engine.RunDevelopmentSessionBrokerFixture(input, output, runDevelopmentBroker)
	}
	// Dispatch precedes config.Load, saved Store, real provider and app-shell launch.
	return true, engine.RunDevelopmentSessionFixture(input, output)
}
