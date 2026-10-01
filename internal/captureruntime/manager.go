// Package captureruntime owns the managed capture process separately from the
// long-lived UI. It does not select collection modes or dispatch source commands.
package captureruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrBusy        = errors.New("capture runtime has active leases")
	ErrUnavailable = errors.New("capture runtime is unavailable")
	ErrStopped     = errors.New("capture runtime is stopped")
)

// Process must release its entire owned process tree before CloseForRetry
// succeeds. Root-process exit alone is insufficient proof of profile release.
type Process interface {
	Done() <-chan error
	Terminate()
	CloseForRetry(context.Context) error
	PID() int
	OpenExtensionsPage(context.Context) error
	ReplacementReadiness(context.Context) error
}

// Launch starts only the managed capture process, never the UI. It must use the
// same authenticated profile. A failed launch is treated as an unverified owner
// boundary and blocks further launches until application shutdown/recovery.
type Launch func(context.Context, uint64) (Process, error)

type State string

const (
	Ready     State = "ready"
	Replacing State = "replacing"
	Blocked   State = "blocked"
	Failed    State = "failed"
	Stopped   State = "stopped"
)

type Snapshot struct {
	State        State
	Generation   uint64
	ActiveLeases int
	Driver       string
}

type owner struct {
	process     Process
	intentional bool
	driver      string
}

type Manager struct {
	operation           sync.Mutex
	mu                  sync.Mutex
	current             *owner
	state               State
	generation          uint64
	leases              int
	stopped             bool
	launchBlocked       bool
	done                chan error
	finishOnce          sync.Once
	transitionReadiness func(context.Context) error
}

// SetTransitionReadiness binds transport capability checks before handoff is
// exposed. The check is immutable; a replacement must rebind the transport that
// it reads rather than silently dropping the guard.
func (m *Manager) SetTransitionReadiness(check func(context.Context) error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if check == nil || m.transitionReadiness != nil || m.state != Ready || m.stopped {
		return ErrUnavailable
	}
	m.transitionReadiness = check
	return nil
}

// New adopts an already launched capture process. Ordinary unexpected exit
// retains the application's existing shutdown policy; intentional replacement
// never signals Done. OS-level exclusivity remains the process owner's job.
func New(process Process) (*Manager, error) {
	if process == nil || process.Done() == nil {
		return nil, ErrUnavailable
	}
	m := &Manager{current: &owner{process: process, driver: processDriver(process)}, state: Ready, generation: 1, done: make(chan error, 1)}
	go m.watch(m.current)
	return m, nil
}

func (m *Manager) watch(value *owner) {
	err := <-value.process.Done()
	m.mu.Lock()
	unexpected := m.current == value && !value.intentional && !m.stopped
	if unexpected {
		m.state = Failed
		if value.driver == "headless" || m.generation > 1 {
			m.launchBlocked = true
		} else {
			m.stopped = true
		}
	}
	shutdown := unexpected && m.stopped
	m.mu.Unlock()
	if shutdown {
		m.finish(err)
	}
}

func (m *Manager) finish(err error) {
	m.finishOnce.Do(func() { m.done <- err; close(m.done) })
}

func (m *Manager) Done() <-chan error {
	if m == nil {
		return nil
	}
	return m.done
}

func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	driver := ""
	if m.current != nil {
		driver = m.current.driver
	}
	return Snapshot{State: m.state, Generation: m.generation, ActiveLeases: m.leases, Driver: driver}
}

// Retiring identifies a retained headed owner whose scoped host-close request
// may have completed while independent windows still keep its profile alive.
func (m *Manager) Retiring() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return false
	}
	process, ok := m.current.process.(interface{ Retiring() bool })
	return ok && process.Retiring()
}

// Accepts is a local generation fence, not a replacement for durable command
// authorization. Dispatch/result ingestion must wire their persisted fence too.
func (m *Manager) Accepts(generation uint64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state == Ready && !m.stopped && generation == m.generation
}

// Replace refuses active work. The coordinator must stop admission and drain
// its runs/readers before calling this; it must never release leases early.
// The caller supplies a bounded/cancellable transition context.
func (m *Manager) Replace(ctx context.Context, launch Launch) error {
	if launch == nil {
		return errors.New("capture launcher is required")
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return ErrStopped
	}
	if m.launchBlocked {
		m.mu.Unlock()
		return ErrUnavailable
	}
	if m.leases != 0 {
		m.mu.Unlock()
		return ErrBusy
	}
	previous := m.current
	previousState := m.state
	check := m.transitionReadiness
	m.state = Replacing
	m.mu.Unlock()
	if check != nil {
		if err := check(ctx); err != nil {
			m.mu.Lock()
			if !m.stopped {
				m.state = previousState
			}
			m.mu.Unlock()
			return fmt.Errorf("capture transport not ready: %w", err)
		}
	}
	if previous != nil {
		if err := previous.process.ReplacementReadiness(ctx); err != nil {
			m.mu.Lock()
			if !m.stopped {
				m.state = previousState
			}
			m.mu.Unlock()
			return fmt.Errorf("capture replacement not ready: %w", err)
		}
		m.mu.Lock()
		if m.stopped {
			m.mu.Unlock()
			return ErrStopped
		}
		previous.intentional = true
		m.mu.Unlock()
		if err := previous.process.CloseForRetry(ctx); err != nil {
			m.mu.Lock()
			m.state = Blocked // Retain the owner until cleanup is actually verified.
			m.mu.Unlock()
			return fmt.Errorf("release capture ownership: %w", err)
		}
	}
	m.mu.Lock()
	m.current = nil
	m.generation++
	generation := m.generation
	m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		m.mu.Lock()
		m.state = Blocked
		m.mu.Unlock()
		return err
	}
	next, err := launch(ctx, generation)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && (next == nil || next.Done() == nil) {
		err = ErrUnavailable
	}
	m.mu.Lock()
	if err != nil {
		if next != nil {
			m.current = &owner{process: next, intentional: true, driver: processDriver(next)}
		}
		m.state = Blocked
		m.launchBlocked = true // A failed factory may have left an owned tree.
		m.mu.Unlock()
		return fmt.Errorf("launch replacement capture: %w", err)
	}
	value := &owner{process: next, driver: processDriver(next)}
	m.current = value
	m.state = Ready
	m.mu.Unlock()
	go m.watch(value)
	return nil
}

func processDriver(process Process) string {
	if driver, ok := process.(interface{ Driver() string }); ok {
		return driver.Driver()
	}
	return "browser"
}

// Recover may retry only an owner whose complete process-tree cleanup is
// positively verified. A nil failed factory is unknown, never proof of release.
func (m *Manager) Recover(ctx context.Context, launch Launch) error {
	if launch == nil {
		return errors.New("capture launcher is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	if m.stopped || m.leases != 0 || m.current == nil || (m.state != Blocked && m.state != Failed) {
		m.mu.Unlock()
		return ErrUnavailable
	}
	previous := m.current
	previousState := m.state
	m.state = Replacing
	m.mu.Unlock()
	// Recovery is not permission to close a live interactive owner. An exited
	// process may report readiness after its verified cleanup; a live headed
	// owner must satisfy the same native-lifetime boundary as ordinary handoff.
	if err := previous.process.ReplacementReadiness(ctx); err != nil {
		m.mu.Lock()
		if !m.stopped {
			m.state = previousState
		}
		m.mu.Unlock()
		return fmt.Errorf("capture recovery not ready: %w", err)
	}
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return ErrStopped
	}
	previous.intentional = true
	m.mu.Unlock()
	if err := previous.process.CloseForRetry(ctx); err != nil {
		m.mu.Lock()
		m.state = Blocked
		m.mu.Unlock()
		return err
	}
	m.mu.Lock()
	m.current = nil
	m.generation++
	generation := m.generation
	m.mu.Unlock()
	next, err := launch(ctx, generation)
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && (next == nil || next.Done() == nil) {
		err = ErrUnavailable
	}
	m.mu.Lock()
	if err != nil {
		if next != nil {
			m.current = &owner{process: next, intentional: true, driver: processDriver(next)}
		}
		m.state = Blocked
		m.launchBlocked = true
		m.mu.Unlock()
		return err
	}
	value := &owner{process: next, driver: processDriver(next)}
	m.current = value
	m.state = Ready
	m.launchBlocked = false
	m.mu.Unlock()
	go m.watch(value)
	return nil
}

func (m *Manager) PID() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.current == nil {
		return 0
	}
	return m.current.process.PID()
}

func (m *Manager) OpenExtensionsPage(ctx context.Context) error {
	// Serialize recovery action dispatch with replacement. Full interactive
	// reader lifetimes must acquire a Lease at the coordinator boundary.
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	if m.state != Ready || m.stopped || m.current == nil {
		m.mu.Unlock()
		return ErrUnavailable
	}
	process := m.current.process
	m.mu.Unlock()
	return process.OpenExtensionsPage(ctx)
}

func (m *Manager) Terminate() {
	if m == nil {
		return
	}
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	m.stopped = true
	m.state = Stopped
	value := m.current
	if value != nil {
		value.intentional = true
	}
	m.mu.Unlock()
	if value != nil {
		value.process.Terminate()
	}
	m.finish(nil)
}
