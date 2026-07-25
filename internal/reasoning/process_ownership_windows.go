//go:build windows

package reasoning

import (
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// processOwnership gives every App Server instance its own Windows Job
// Object. This closes the forced-shutdown gap where Process.Kill only targets
// codex.exe while node/git descendants can otherwise outlive the invocation.
type processOwnership struct {
	job windows.Handle
}

func newProcessOwnership() (processOwnership, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return processOwnership{}, fmt.Errorf("create App Server Job Object: %w", err)
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
		return processOwnership{}, fmt.Errorf("configure App Server Job Object: %w", err)
	}
	return processOwnership{job: job}, nil
}

func (o processOwnership) attach(command *exec.Cmd) error {
	if o.job == 0 || command == nil || command.Process == nil {
		return fmt.Errorf("App Server process is unavailable for Job Object assignment")
	}
	process, err := windows.OpenProcess(
		windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(command.Process.Pid),
	)
	if err != nil {
		return fmt.Errorf("open App Server process for Job Object assignment: %w", err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(o.job, process); err != nil {
		return fmt.Errorf("assign App Server process to Job Object: %w", err)
	}
	return nil
}

func (o processOwnership) terminate() {
	if o.job != 0 {
		_ = windows.TerminateJobObject(o.job, 1)
	}
}

func (o processOwnership) close() {
	if o.job != 0 {
		_ = windows.CloseHandle(o.job)
	}
}
