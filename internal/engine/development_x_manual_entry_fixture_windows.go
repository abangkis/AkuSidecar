//go:build windows && auth_session_dev && auth_session_fixture

package engine

import "github.com/abangkis/AkuSidecar/internal/config"

// Only combined-tag accountless tests can construct this offline factory.
// No CLI, environment variable or HTTP request selects it in the native entry.
func PrepareXManualDevelopmentSynthetic(options config.Options) (XManualDevelopmentSelection, error) {
	selection, err := PrepareXManualDevelopment(options)
	if err != nil || selection == nil {
		return selection, err
	}
	v := selection.(*xManualDevelopmentEntry)
	kit, err := newXQASyntheticKit()
	if err != nil {
		v.Close()
		return nil, err
	}
	v.factory = kit.factory
	v.clock = kit.clock
	v.fixtureClose = kit.close
	return v, nil
}
