package appshell

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
)

// OwnedCommand assigns a worker to the same owned-tree cleanup boundary used
// by capture Chrome. The worker must wait for stdin initialization before it
// spawns children, so assignment finishes before Chrome can inherit the job.
type OwnedCommand struct {
	command     *exec.Cmd
	owner       processOwnership
	done        chan error
	closed      chan struct{}
	once        sync.Once
	ownershipMu sync.Mutex
	cleanupErr  error
}

func StartOwnedCommand(command *exec.Cmd) (*OwnedCommand, error) {
	owner, err := newProcessOwnership()
	if err != nil {
		return nil, err
	}
	prepareWorkerCommand(command)
	if err := command.Start(); err != nil {
		owner.close()
		return nil, err
	}
	if err := owner.attach(command); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		owner.close()
		return nil, fmt.Errorf("worker ownership: %w", err)
	}
	p := &OwnedCommand{command: command, owner: owner, done: make(chan error, 1), closed: make(chan struct{})}
	go func() {
		err := command.Wait()
		p.ownershipMu.Lock()
		p.cleanupErr = p.owner.drain()
		p.owner.close()
		p.owner = processOwnership{}
		p.ownershipMu.Unlock()
		close(p.closed)
		if p.cleanupErr != nil {
			err = p.cleanupErr
		}
		p.done <- err
	}()
	return p, nil
}
func (p *OwnedCommand) PID() int                { return p.command.Process.Pid }
func (p *OwnedCommand) Done() <-chan error      { return p.done }
func (p *OwnedCommand) Closed() <-chan struct{} { return p.closed }
func (p *OwnedCommand) Terminate() {
	p.once.Do(func() {
		p.ownershipMu.Lock()
		defer p.ownershipMu.Unlock()
		p.owner.terminate(p.command.Process)
	})
}
func (p *OwnedCommand) CloseForRetry(ctx context.Context) error {
	p.Terminate()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closed:
		return p.cleanupErr
	}
}
