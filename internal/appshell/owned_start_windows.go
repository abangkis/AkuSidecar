//go:build windows

package appshell

import (
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This handle belongs to the Go process lifetime, never to an individual
// window. All children created after initialization inherit its membership.
// It is unnamed and noninheritable, with no breakaway, UI or resource limits.
// Closing it would kill Go itself, so only process exit may release it.
var launcherLifetime struct {
	once sync.Once
	job  windows.Handle
	err  error
}

func ensureLauncherLifetime() error {
	launcherLifetime.once.Do(func() {
		owner, err := createProcessOwnership()
		if err != nil {
			launcherLifetime.err = err
			return
		}
		if err = windows.AssignProcessToJobObject(owner.job, windows.CurrentProcess()); err != nil {
			owner.close() // Go was not assigned, so this cannot terminate Go.
			launcherLifetime.err = fmt.Errorf("assign launcher lifetime Job: %w", err)
			return
		}
		launcherLifetime.job = owner.job
	})
	return launcherLifetime.err
}

// start preserves exec.Cmd's argument, handle-inheritance and Wait behavior.
// The lifetime Job covers even the suspended pre-assignment root if Go dies.
// The per-tree Job is assigned before any child code can spawn descendants.
func (o *processOwnership) start(command *exec.Cmd, beforeAttach func(*exec.Cmd, *processOwnership) error) error {
	if err := ensureLauncherLifetime(); err != nil {
		return err
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	attributes := *command.SysProcAttr
	attributes.CreationFlags |= windows.CREATE_SUSPENDED
	command.SysProcAttr = &attributes
	if err := command.Start(); err != nil {
		return err
	}
	startErr := func() error {
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(command.Process.Pid))
		if err != nil {
			return err
		}
		defer windows.CloseHandle(process)
		if !processBelongsToJob(process, launcherLifetime.job) {
			return errors.New("startup root outside launcher lifetime Job")
		}
		thread, err := suspendedStartupThread(uint32(command.Process.Pid))
		if err != nil {
			return err
		}
		defer windows.CloseHandle(thread)
		if beforeAttach != nil {
			if err := beforeAttach(command, o); err != nil {
				return err
			}
		}
		if err := o.attach(command); err != nil {
			return err
		}
		if !processBelongsToJob(process, o.job) {
			return errors.New("startup root outside per-tree Job")
		}
		status, err := windows.WaitForSingleObject(process, 0)
		if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
			return fmt.Errorf("startup root not live before resume: wait=%d: %v", status, err)
		}
		previous, err := windows.ResumeThread(thread)
		if err != nil || previous != 1 {
			return fmt.Errorf("resume startup thread: previous suspend count=%d: %v", previous, err)
		}
		return nil
	}()
	if startErr == nil {
		return nil
	}
	// Wait is the actual exec.Cmd wait, even on failure. No resume or retry
	// proceeds after failed ownership; a cleanup failure remains explicit.
	killErr := command.Process.Kill()
	drainErr := o.drain()
	type terminalWait struct {
		verified bool
		err      error
	}
	waited := make(chan terminalWait, 1)
	go func() {
		err := command.Wait()
		waited <- terminalWait{command.ProcessState != nil && command.ProcessState.Exited(), err}
	}()
	var terminalErr error
	select {
	case result := <-waited:
		if !result.verified {
			terminalErr = fmt.Errorf("startup root terminal wait not verified: %v", result.err)
		}
	case <-time.After(5 * time.Second):
		// The actual wait continues to reap this root. The caller must retain
		// private endpoints when terminal ownership cannot be verified.
		terminalErr = errors.New("startup root terminal wait timed out")
	}
	if killErr != nil || drainErr != nil || terminalErr != nil {
		return errors.Join(errCleanupUnverified, startErr, killErr, drainErr, terminalErr)
	}
	return fmt.Errorf("owned startup: %w", startErr)
}

func suspendedStartupThread(pid uint32) (windows.Handle, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return 0, fmt.Errorf("snapshot startup threads: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	var id uint32
	count := 0
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.Size < uint32(unsafe.Offsetof(entry.OwnerProcessID)+unsafe.Sizeof(entry.OwnerProcessID)) {
			return 0, errors.New("startup thread entry incomplete")
		}
		if entry.OwnerProcessID == pid {
			id = entry.ThreadID
			count++
		}
		entry.Size = uint32(unsafe.Sizeof(entry))
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return 0, fmt.Errorf("enumerate startup threads: %w", err)
	}
	if count != 1 {
		return 0, fmt.Errorf("suspended startup root has %d threads, want one", count)
	}
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME|windows.THREAD_QUERY_LIMITED_INFORMATION, false, id)
	if err != nil {
		return 0, err
	}
	// Pin before checking the owner: a thread ID from the snapshot alone can
	// be recycled if the suspended process was killed by an external actor.
	owner, _, queryErr := kernel32.NewProc("GetProcessIdOfThread").Call(uintptr(thread))
	if owner != uintptr(pid) {
		windows.CloseHandle(thread)
		return 0, fmt.Errorf("startup thread owner changed: %v", queryErr)
	}
	return thread, nil
}
