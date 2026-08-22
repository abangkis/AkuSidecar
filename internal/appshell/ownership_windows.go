//go:build windows

package appshell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func executableName() string {
	return "chrome.exe"
}

func platformCandidates() []Candidate {
	candidates := make([]Candidate, 0, 3)
	if root, err := os.Executable(); err == nil {
		candidates = append(candidates,
			Candidate{Path: filepath.Join(filepath.Dir(root), "chromium", "bin", "chrome.exe"), Source: "sidecar-relative"},
		)
	}
	if root, err := os.Getwd(); err == nil {
		candidates = append(candidates,
			Candidate{Path: filepath.Join(root, "runtime", "chromium", "bin", "chrome.exe"), Source: "workspace"},
		)
	}
	local := os.Getenv("LOCALAPPDATA")
	if strings.TrimSpace(local) != "" {
		candidates = append(candidates, Candidate{
			Path:   filepath.Join(local, "AkuBrowser", "chromium", "bin", "chrome.exe"),
			Source: "installed",
		})
	}
	return candidates
}

type processOwnership struct {
	job windows.Handle
}

func newProcessOwnership() (processOwnership, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processOwnership{}, fmt.Errorf("create app shell Job Object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return processOwnership{}, fmt.Errorf("configure app shell Job Object: %w", err)
	}
	return processOwnership{job: job}, nil
}

func (o *processOwnership) attach(command *exec.Cmd) error {
	if o.job == 0 || command == nil || command.Process == nil {
		return fmt.Errorf("app shell process is unavailable for Job Object assignment")
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		return fmt.Errorf("open app shell process for Job Object assignment: %w", err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(o.job, process); err != nil {
		return fmt.Errorf("assign app shell process to Job Object: %w", err)
	}
	return nil
}

func (o *processOwnership) terminate() {
	if o.job != 0 {
		_ = windows.TerminateJobObject(o.job, 1)
	}
}

func (o *processOwnership) close() {
	if o.job != 0 {
		_ = windows.CloseHandle(o.job)
	}
}

func prepareCommand(command *exec.Cmd) {}
