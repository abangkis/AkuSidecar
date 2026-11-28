//go:build !windows || !auth_session_dev

package main

import (
	"io"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/diagnostics/operational"
)

func dispatchXApplicationQA(options config.Options, _ io.Reader, _ io.Writer) (bool, error) {
	if options.DevelopmentQAPilotPrepare != "" || options.DevelopmentQAPilotPlan != "" || options.DevelopmentSessionReuse == "native-x-manual-qa" || options.DevelopmentSessionReuse == "synthetic-x-manual-qa" {
		return true, operational.QABuildRequired
	}
	return false, nil
}
