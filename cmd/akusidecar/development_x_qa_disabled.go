//go:build !windows || !auth_session_dev

package main

import (
	"errors"
	"io"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func dispatchXApplicationQA(options config.Options, _ io.Reader, _ io.Writer) (bool, error) {
	if options.DevelopmentQAPilotPrepare != "" || options.DevelopmentQAPilotPlan != "" || options.DevelopmentSessionReuse == "native-x-manual-qa" || options.DevelopmentSessionReuse == "synthetic-x-manual-qa" {
		return true, errors.New("isolated X QA requires the Windows development build")
	}
	return false, nil
}
