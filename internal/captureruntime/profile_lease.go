package captureruntime

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// idleHolder is an optional headless capability. Calls are bounded and made
// under the admission mutex: implementations must not call back into Manager.
// A lease is not admitted until the worker acknowledges the hold.
type idleHolder interface {
	SetIdleHold(context.Context, bool) error
}

func (m *Manager) setIdleHoldLocked(held bool) error {
	if m.current == nil || m.current.driver != "headless" {
		return nil
	}
	process, ok := m.current.process.(idleHolder)
	if !ok {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := process.SetIdleHold(ctx, held); err != nil {
		// A failed acknowledgement has an uncertain idle boundary. Recovery must
		// verify the owned tree before another generation can use the profile.
		m.state, m.launchBlocked = Blocked, true
		return fmt.Errorf("headless idle hold acknowledgement failed: %w", err)
	}
	return nil
}

// Lease pins the manager's owner while a collection or interactive operation is
// active. It is process-local admission, not a filesystem/Chrome profile lock.
// Callers must hold it through the entire operation, including follow-up work.
type Lease struct {
	manager    *Manager
	generation uint64
	driver     string
	once       sync.Once
}

func (m *Manager) Acquire() (*Lease, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return nil, ErrStopped
	}
	if m.state != Ready || m.current == nil {
		return nil, ErrUnavailable
	}
	if m.leases == 0 {
		if err := m.setIdleHoldLocked(true); err != nil {
			return nil, err
		}
	}
	m.leases++
	return &Lease{manager: m, generation: m.generation, driver: m.current.driver}, nil
}

func (l *Lease) Generation() uint64 { return l.generation }
func (l *Lease) Driver() string     { return l.driver }

func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		l.manager.mu.Lock()
		l.manager.leases--
		if l.manager.leases == 0 && !l.manager.stopped && l.manager.state == Ready {
			_ = l.manager.setIdleHoldLocked(false)
		}
		l.manager.mu.Unlock()
	})
}
