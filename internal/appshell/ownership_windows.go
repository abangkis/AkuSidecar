//go:build windows

package appshell

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowIconMessage  = 0x0080
	iconSmall          = 0
	iconBig            = 1
	windowCloseMessage = 0x0010
)

var (
	user32                     = windows.NewLazySystemDLL("user32.dll")
	shell32                    = windows.NewLazySystemDLL("shell32.dll")
	kernel32                   = windows.NewLazySystemDLL("kernel32.dll")
	procCreateIconFromResource = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon            = user32.NewProc("DestroyIcon")
	procEnumWindows            = user32.NewProc("EnumWindows")
	procGetWindowTextLengthW   = user32.NewProc("GetWindowTextLengthW")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible        = user32.NewProc("IsWindowVisible")
	procSendMessageW           = user32.NewProc("SendMessageW")
	procPostMessageW           = user32.NewProc("PostMessageW")
	procIsProcessInJob         = kernel32.NewProc("IsProcessInJob")
	procPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
)

func processBelongsToJob(process windows.Handle, job windows.Handle) bool {
	if job == 0 || process == 0 {
		return false
	}
	var inJob int32
	result, _, _ := procIsProcessInJob.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&inJob)))
	return result != 0 && inJob != 0
}

var (
	propertyStoreIID = windows.GUID{Data1: 0x886d8eeb, Data2: 0x8cf2, Data3: 0x4446, Data4: [8]byte{0x8d, 0x02, 0xcd, 0xba, 0x1d, 0xbd, 0xcf, 0x99}}
	appModelFormatID = windows.GUID{Data1: 0x9f4c2855, Data2: 0x9f79, Data3: 0x4b39, Data4: [8]byte{0xa8, 0xd0, 0xe1, 0xd4, 0x2d, 0xe1, 0xd5, 0xf3}}
)

const variantWideString = 31

type propertyKey struct {
	FormatID windows.GUID
	ID       uint32
}

type propertyVariant struct {
	Type      uint16
	Reserved1 uint16
	Reserved2 uint16
	Reserved3 uint16
	Pointer   uintptr
	Padding   uintptr
}

type propertyStore struct {
	VTable *propertyStoreVTable
}

type propertyStoreVTable struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	GetCount       uintptr
	GetAt          uintptr
	GetValue       uintptr
	SetValue       uintptr
	Commit         uintptr
}

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

// terminate first asks the owned Chromium tree to exit normally by closing
// its top-level windows, waits a bounded time for the root browser process,
// and only then falls back to the Job Object kill. The Job Object remains the
// ownership boundary; the graceful stage exists so a normal stop produces a
// normal Chromium shutdown instead of a crash-labeled profile.
func (o *processOwnership) terminate(root *os.Process) {
	o.closeOwnedWindows()
	if o.awaitRootExit(root, gracefulTerminateTimeout) {
		return
	}
	if o.job != 0 {
		_ = windows.TerminateJobObject(o.job, 1)
	}
}

func (o *processOwnership) closeOwnedWindows() {
	if o.job == 0 {
		return
	}
	callback := syscall.NewCallback(func(handle uintptr, _ uintptr) uintptr {
		var owner uint32
		procGetWindowThreadProcess.Call(handle, uintptr(unsafe.Pointer(&owner)))
		if owner == 0 {
			return 1
		}
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, owner)
		if err != nil {
			return 1
		}
		defer windows.CloseHandle(process)
		if !processBelongsToJob(process, o.job) {
			return 1
		}
		procPostMessageW.Call(handle, windowCloseMessage, 0, 0)
		return 1
	})
	procEnumWindows.Call(callback, 0)
}

func (o *processOwnership) awaitRootExit(root *os.Process, timeout time.Duration) bool {
	if root == nil {
		return true
	}
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE,
		false,
		uint32(root.Pid),
	)
	if err != nil {
		// The root process is already gone; ownership only needs cleanup.
		return true
	}
	defer windows.CloseHandle(handle)
	event, err := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	return err == nil && event == uint32(windows.WAIT_OBJECT_0)
}

func (o *processOwnership) close() {
	if o.job != 0 {
		_ = windows.CloseHandle(o.job)
	}
}

type windowIcon struct {
	handles []windows.Handle
}

func applyWindowIcon(pid int, path, userDataDir string, identity ApplicationIdentity) (windowIcon, error) {
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
			if strings.TrimSpace(identity.ID) != "" {
				iconFile, err := materializeRelaunchIcon(data, userDataDir)
				if err != nil {
					icon.close()
					return windowIcon{}, err
				}
				if err := setWindowApplicationIdentity(handle, identity, iconFile+",0"); err != nil {
					icon.close()
					return windowIcon{}, err
				}
			}
			procSendMessageW.Call(handle, windowIconMessage, iconSmall, uintptr(small))
			procSendMessageW.Call(handle, windowIconMessage, iconBig, uintptr(large))
			return icon, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	icon.close()
	return windowIcon{}, fmt.Errorf("AkuBrowser app-shell window did not become available for icon assignment")
}

func materializeRelaunchIcon(pngData []byte, userDataDir string) (string, error) {
	if len(pngData) == 0 {
		return "", fmt.Errorf("AkuBrowser relaunch icon is empty")
	}
	if strings.TrimSpace(userDataDir) == "" {
		return "", fmt.Errorf("AkuBrowser relaunch icon requires a stable user-data directory")
	}
	if len(pngData) > int(^uint32(0)) {
		return "", fmt.Errorf("AkuBrowser relaunch icon is too large")
	}
	ico := make([]byte, 22+len(pngData))
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], 1)
	ico[6] = 128
	ico[7] = 128
	binary.LittleEndian.PutUint16(ico[10:12], 1)
	binary.LittleEndian.PutUint16(ico[12:14], 32)
	binary.LittleEndian.PutUint32(ico[14:18], uint32(len(pngData)))
	binary.LittleEndian.PutUint32(ico[18:22], 22)
	copy(ico[22:], pngData)
	if err := os.MkdirAll(userDataDir, 0o700); err != nil {
		return "", fmt.Errorf("create app-shell identity directory: %w", err)
	}
	path := filepath.Join(userDataDir, "AkuBrowser.ico")
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, ico) {
		return path, nil
	}
	if err := os.WriteFile(path, ico, 0o600); err != nil {
		return "", fmt.Errorf("write AkuBrowser relaunch icon: %w", err)
	}
	return path, nil
}

func setWindowApplicationIdentity(window uintptr, identity ApplicationIdentity, iconResource string) error {
	var store *propertyStore
	result, _, callErr := procPropertyStoreForWindow.Call(
		window,
		uintptr(unsafe.Pointer(&propertyStoreIID)),
		uintptr(unsafe.Pointer(&store)),
	)
	if int32(result) < 0 || store == nil {
		return fmt.Errorf("open app-shell window property store: HRESULT 0x%08x (%v)", uint32(result), callErr)
	}
	defer func() {
		_, _, _ = syscall.SyscallN(store.VTable.Release, uintptr(unsafe.Pointer(store)))
	}()
	for _, value := range []struct {
		id    uint32
		value string
	}{
		{id: 2, value: identity.RelaunchCommand},
		{id: 4, value: identity.DisplayName},
		{id: 3, value: iconResource},
		// Set AppUserModelID last so the taskbar refresh observes the complete
		// relaunch tuple rather than briefly falling back to chrome.exe.
		{id: 5, value: identity.ID},
	} {
		if err := setWindowStringProperty(store, propertyKey{FormatID: appModelFormatID, ID: value.id}, value.value); err != nil {
			return err
		}
	}
	result, _, _ = syscall.SyscallN(store.VTable.Commit, uintptr(unsafe.Pointer(store)))
	if int32(result) < 0 {
		return fmt.Errorf("commit app-shell application identity: HRESULT 0x%08x", uint32(result))
	}
	return nil
}

func setWindowStringProperty(store *propertyStore, key propertyKey, value string) error {
	pointer, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return fmt.Errorf("encode app-shell application identity: %w", err)
	}
	variant := propertyVariant{Type: variantWideString, Pointer: uintptr(unsafe.Pointer(pointer))}
	result, _, _ := syscall.SyscallN(
		store.VTable.SetValue,
		uintptr(unsafe.Pointer(store)),
		uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(&variant)),
	)
	runtime.KeepAlive(pointer)
	if int32(result) < 0 {
		return fmt.Errorf("set app-shell application identity property %d: HRESULT 0x%08x", key.ID, uint32(result))
	}
	return nil
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
