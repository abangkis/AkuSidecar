//go:build windows

package appshell

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowIconMessage = 0x0080
	iconSmall         = 0
	iconBig           = 1
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procCreateIconFromResource = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon            = user32.NewProc("DestroyIcon")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")
	procSendMessageW           = user32.NewProc("SendMessageW")
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

type windowIcon struct {
	handles []windows.Handle
}

func applyWindowIcon(pid int, path string) (windowIcon, error) {
	if strings.TrimSpace(path) == "" {
		return windowIcon{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return windowIcon{}, fmt.Errorf("read AkuBrowser app-shell icon: %w", err)
	}
	large, err := createWindowIcon(data, 128)
	if err != nil {
		return windowIcon{}, err
	}
	small, err := createWindowIcon(data, 32)
	if err != nil {
		destroyWindowIcon(large)
		return windowIcon{}, err
	}
	icon := windowIcon{handles: []windows.Handle{large, small}}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if handle := visibleTopLevelWindow(uint32(pid)); handle != 0 {
			procSendMessageW.Call(handle, windowIconMessage, iconSmall, uintptr(small))
			procSendMessageW.Call(handle, windowIconMessage, iconBig, uintptr(large))
			return icon, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	icon.close()
	return windowIcon{}, fmt.Errorf("AkuBrowser app-shell window did not become available for icon assignment")
}

func createWindowIcon(data []byte, size uintptr) (windows.Handle, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("AkuBrowser app-shell icon is empty")
	}
	handle, _, callErr := procCreateIconFromResource.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		1,
		0x00030000,
		size,
		size,
		0,
	)
	if handle == 0 {
		return 0, fmt.Errorf("decode AkuBrowser app-shell icon: %v", callErr)
	}
	return windows.Handle(handle), nil
}

func visibleTopLevelWindow(pid uint32) uintptr {
	var found uintptr
	callback := syscall.NewCallback(func(handle uintptr, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadProcess.Call(handle, uintptr(unsafe.Pointer(&owner)))
		visible, _, _ := procIsWindowVisible.Call(handle)
		titleLength, _, _ := procGetWindowTextLengthW.Call(handle)
		if owner == pid && visible != 0 && titleLength > 0 {
			found = handle
			return 0
		}
		return 1
	})
	procEnumWindows.Call(callback, 0)
	return found
}

func destroyWindowIcon(handle windows.Handle) {
	if handle != 0 {
		procDestroyIcon.Call(uintptr(handle))
	}
}

func (icon windowIcon) close() {
	for _, handle := range icon.handles {
		destroyWindowIcon(handle)
	}
}

func prepareCommand(command *exec.Cmd) {}
