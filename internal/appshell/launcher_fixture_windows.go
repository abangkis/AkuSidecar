//go:build windows && auth_session_fixture

package appshell

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// LaunchUnassignedFixture exercises the actual launcher, only from the private
// dedicated worker after it has authenticated its fresh controller grant.
// There is no ordinary launch option, environment switch, or runtime endpoint.
func LaunchUnassignedFixture(ctx context.Context, options LaunchOptions, root string, barrier func(uint32, windows.Handle, windows.Handle) error) (*Window, error) {
	executable, err := os.Executable()
	project, projectErr := filepath.Abs(filepath.Join("..", ".."))
	parent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	rel, relErr := filepath.Rel(parent, root)
	resolved, resolvedErr := filepath.EvalSymlinks(root)
	if err != nil || projectErr != nil || relErr != nil || resolvedErr != nil || !strings.EqualFold(root, resolved) || strings.Contains(rel, "..") || filepath.IsAbs(rel) || !strings.HasPrefix(filepath.Base(root), "run-") || !strings.EqualFold(filepath.Base(executable), "auth-session-process-fixture.exe") || !strings.EqualFold(options.Executable, executable) || !strings.EqualFold(filepath.Dir(executable), filepath.Dir(root)) || !strings.EqualFold(options.UserDataDir, filepath.Join(root, "shared")) {
		return nil, errors.New("private launcher fixture scope invalid")
	}
	if _, err = os.Lstat(options.UserDataDir); !os.IsNotExist(err) {
		return nil, errors.New("private launcher fixture profile is not fresh")
	}
	if barrier == nil {
		return Launch(ctx, options)
	}
	return launch(ctx, options, func(command *exec.Cmd, owner *processOwnership) error {
		return barrier(uint32(command.Process.Pid), owner.job, launcherLifetime.job)
	})
}

// FixtureOwnsLauncherPeers reads actual per-tree and lifetime membership. It
// does not duplicate the lifetime handle into the controller or the children.
func FixtureOwnsLauncherPeers(window *Window, worker *OwnedCommand, pids []uint32) bool {
	job := windows.Handle(0)
	if window != nil {
		job = window.owner.job
	} else if worker != nil {
		job = worker.owner.job
	}
	if job == 0 || launcherLifetime.job == 0 {
		return false
	}
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			return false
		}
		member := processBelongsToJob(h, job) && processBelongsToJob(h, launcherLifetime.job)
		windows.CloseHandle(h)
		if !member {
			return false
		}
	}
	return true
}

func FixtureStartupCleanupVerified(err error) bool {
	return err != nil && !errors.Is(err, errCleanupUnverified)
}
