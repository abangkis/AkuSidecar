package captureruntime

import "sync"

// Lease pins the manager's owner while a collection or interactive operation is
// active. It is process-local admission, not a filesystem/Chrome profile lock.
// Callers must hold it through the entire operation, including follow-up work.
type Lease struct {
	manager    *Manager
	generation uint64
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
	m.leases++
	return &Lease{manager: m, generation: m.generation}, nil
}

func (l *Lease) Generation() uint64 { return l.generation }

func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		l.manager.mu.Lock()
		l.manager.leases--
		l.manager.mu.Unlock()
	})
}
