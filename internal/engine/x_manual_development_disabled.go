//go:build !windows || !auth_session_dev

package engine

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/config"
)

func PrepareXManualDevelopment(options config.Options) (XManualDevelopmentSelection, error) {
	if options.DevelopmentXManualPlan != "" || options.DevelopmentSessionReuse == "native-x-manual" {
		return nil, errors.New("manual X application requires the Windows development build")
	}
	return nil, nil
}
