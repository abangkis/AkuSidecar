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
	if !options.Dev || (options.DevelopmentSessionReuse != "synthetic-x-manual-fixture" && options.DevelopmentSessionReuse != "synthetic-concurrent-chrome-fixture" && options.DevelopmentSessionReuse != "synthetic-concurrent-fixture" && options.DevelopmentSessionReuse != "synthetic-fixture" && options.DevelopmentSessionReuse != "synthetic-policy-fixture" && options.DevelopmentSessionReuse != "synthetic-broker-fixture" && options.DevelopmentSessionReuse != "synthetic-frontend-fixture" && options.DevelopmentSessionReuse != "synthetic-frontend-native-fixture") {
		return false, errors.New("development session requires --dev and supported synthetic-fixture provider")
	}
	if options.DevelopmentSessionReuse == "synthetic-x-manual-fixture" {
		// The manual startup connector is accountless here and exits before
		// config.Load, Store.Open, provider construction or app-shell launch.
		return true, engine.RunDevelopmentXManualFixture(input, output)
	}
	if options.DevelopmentSessionReuse == "synthetic-concurrent-chrome-fixture" {
		return true, engine.RunDevelopmentConcurrentChromeFixture(input, output)
	}
	if options.DevelopmentSessionReuse == "synthetic-concurrent-fixture" {
		return true, engine.RunDevelopmentConcurrentFixture(input, output)
	}
	if options.DevelopmentSessionReuse == "synthetic-policy-fixture" {
		return true, engine.RunDevelopmentPolicyFixture(input, output)
	}
	if options.DevelopmentSessionReuse == "synthetic-frontend-native-fixture" {
		return true, engine.RunDevelopmentSessionFrontendNativeFixture(input, output, runDevelopmentFrontendNative)
	}
	if options.DevelopmentSessionReuse == "synthetic-frontend-fixture" {
		return true, engine.RunDevelopmentSessionFrontendFixture(input, output, runDevelopmentFrontend)
	}
	if options.DevelopmentSessionReuse == "synthetic-broker-fixture" {
		return true, engine.RunDevelopmentSessionBrokerFixture(input, output, runDevelopmentBroker)
	}
	// Dispatch precedes config.Load, saved Store, real provider and app-shell launch.
	return true, engine.RunDevelopmentSessionFixture(input, output)
}
