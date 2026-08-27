//go:build !windows

package appshell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func executableName() string {
	return "chrome"
}

func platformCandidates() []Candidate {
	candidates := make([]Candidate, 0, 3)
	if root, err := os.Executable(); err == nil {
		candidates = append(candidates,
			Candidate{Path: filepath.Join(filepath.Dir(root), "chromium", "bin", "chrome"), Source: "sidecar-relative"},
		)
	}
	if root, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			Candidate{Path: filepath.Join(root, "runtime", "chromium", "bin", "chrome"), Source: "workspace"},
		)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, Candidate{
			Path:   filepath.Join(home, ".local", "share", "AkuBrowser", "chromium", "bin", "chrome"),
			Source: "installed",
		})
	}
	return candidates
}

type processOwnership struct {
	pid int
}

func newProcessOwnership() (processOwnership, error) {
	return processOwnership{}, nil
}

func prepareCommand(command *exec.Cmd) {
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.Setpgid = true
}

func (o *processOwnership) attach(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return errors.New("app shell process is unavailable for process group assignment")
	}
	o.pid = command.Process.Pid
	return nil
}

// terminate mirrors the Windows contract: request a normal exit first and
// keep the hard kill as the bounded fallback so owned trees never leak.
func (o *processOwnership) terminate(root *os.Process) {
	if o.pid <= 0 {
		return
	}
	_ = syscall.Kill(-o.pid, syscall.SIGTERM)
	if rootExited(root, gracefulTerminateTimeout) {
		return
	}
	_ = syscall.Kill(-o.pid, syscall.SIGKILL)
}

func rootExited(root *os.Process, timeout time.Duration) bool {
	if root == nil {
		return true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(root.Pid, syscall.Signal(0)); err != nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func (o processOwnership) close() {}

type windowIcon struct{}

func applyWindowIcon(int, string, string, ApplicationIdentity) (windowIcon, error) {
	return windowIcon{}, nil
}

func (windowIcon) close() {}
