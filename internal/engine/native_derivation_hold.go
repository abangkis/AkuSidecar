package engine

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/activationguard"
)

// Bind before HTTP, owner restoration or scheduling can begin. Maintenance
// cannot erase saved permission and cannot be installed over a running owner.
func (e *Engine) BindNativeDerivationHold(guard *activationguard.Guard) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if guard == nil || e.nativeDerivationHold != nil || e.autoCancel != nil {
		return errors.New("native maintenance hold binding rejected")
	}
	for _, source := range dailySources {
		d := e.dailyController(source)
		d.state.mu.RLock()
		started := d.state.startUsed || d.state.owner != nil
		d.state.mu.RUnlock()
		if started {
			return errors.New("native maintenance hold binding rejected")
		}
	}
	e.nativeDerivationHold = guard
	return nil
}

func (d dailyController) maintenanceHeld() bool {
	return d.engine.nativeDerivationHold.Held(string(d.source))
}
