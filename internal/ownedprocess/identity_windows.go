//go:build windows

// Package ownedprocess provides local OS facts, never account or session authority.
package ownedprocess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Identity struct {
	PID     uint32
	Created string
}

func ProcessIdentity(pid uint32) (Identity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return Identity{}, err
	}
	defer windows.CloseHandle(h)
	return IdentityFromHandle(h)
}

func IdentityFromHandle(h windows.Handle) (Identity, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return Identity{}, err
	}
	pid, err := windows.GetProcessId(h)
	if err != nil {
		return Identity{}, err
	}
	return Identity{pid, fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime)}, nil
}

func DirectoryID(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("owned directory identity unavailable")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || !strings.EqualFold(filepath.Clean(path), filepath.Clean(resolved)) {
		return "", errors.New("owned directory resolves elsewhere")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(p, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	var data windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &data); err != nil {
		return "", err
	}
	if data.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 || data.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		return "", errors.New("owned directory is not an ordinary directory")
	}
	return fmt.Sprintf("%08x:%08x%08x:%08x%08x", data.VolumeSerialNumber, data.FileIndexHigh, data.FileIndexLow, data.CreationTime.HighDateTime, data.CreationTime.LowDateTime), nil
}

// NewJob creates a noninheritable lifetime Job. The owner must close its handle.
func NewJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, errors.New("owned Job configuration failed")
	}
	return job, nil
}

func Assign(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
}

func Membership(process, job windows.Handle) (bool, error) {
	if process == 0 || job == 0 {
		return false, errors.New("owned Job membership handles unavailable")
	}
	var inJob int32
	result, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
	if result == 0 {
		return false, errors.New("owned Job membership query unavailable")
	}
	return inJob != 0, nil
}

// JobPIDs requires complete Job accounting; truncated/unknown membership fails.
func JobPIDs(job windows.Handle) ([]uint32, error) {
	if job == 0 {
		return nil, errors.New("owned Job unavailable")
	}
	const maxProcesses, pidOffset, basicProcessIDList = 4096, 8, 3
	size := pidOffset + maxProcesses*int(unsafe.Sizeof(uintptr(0)))
	buffer := make([]byte, size)
	var returned uint32
	if err := windows.QueryInformationJobObject(job, basicProcessIDList, uintptr(unsafe.Pointer(&buffer[0])), uint32(len(buffer)), &returned); err != nil {
		return nil, err
	}
	assigned := *(*uint32)(unsafe.Pointer(&buffer[0]))
	listed := *(*uint32)(unsafe.Pointer(&buffer[4]))
	if listed > maxProcesses || listed != assigned || returned < uint32(pidOffset+int(listed)*int(unsafe.Sizeof(uintptr(0)))) {
		return nil, errors.New("owned Job membership incomplete")
	}
	pointers := unsafe.Slice((*uintptr)(unsafe.Pointer(&buffer[pidOffset])), int(listed))
	pids := make([]uint32, 0, len(pointers))
	for _, pid := range pointers {
		if pid == 0 || pid > uintptr(^uint32(0)) {
			return nil, errors.New("owned Job process identity invalid")
		}
		pids = append(pids, uint32(pid))
	}
	sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	return pids, nil
}

var user32 = windows.NewLazySystemDLL("user32.dll")

func WindowPID(hwnd uintptr) uint32 {
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// VisibleWindows enumerates only the selected process; it never changes focus.
func VisibleWindows(pid uint32) []uintptr {
	var result []uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
		if visible != 0 && WindowPID(hwnd) == pid {
			result = append(result, hwnd)
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return result
}

func WindowsGone(handles []uintptr) bool {
	for _, hwnd := range handles {
		present, _, _ := user32.NewProc("IsWindow").Call(hwnd)
		if present != 0 {
			return false
		}
	}
	return true
}
