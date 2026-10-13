//go:build windows && auth_session_fixture

// The isolated fixture owns a real foreground Win32 window and launches the
// unchanged native-message entrypoint directly. It accepts fixed actions only.
package main

import (
	"context"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

type command struct {
	Op         string               `json:"op"`
	WrongImage bool                 `json:"wrongImage"`
	Indirect   bool                 `json:"indirect"`
	Present    bool                 `json:"present"`
	Request    readerbroker.Request `json:"request"`
}
type message struct {
	HWND           uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type windowClass struct {
	Size, Style                   uint32
	Proc                          uintptr
	ClassExtra, WindowExtra       int32
	Instance, Icon, Cursor, Brush uintptr
	Menu, Name                    *uint16
	SmallIcon                     uintptr
}

var user = windows.NewLazySystemDLL("user32.dll")
var outputMu sync.Mutex

type rect struct{ Left, Top, Right, Bottom int32 }

func nativeUIFacts(hwnd, button uintptr) map[string]any {
	visible, _, _ := user.NewProc("IsWindowVisible").Call(hwnd)
	iconic, _, _ := user.NewProc("IsIconic").Call(hwnd)
	var bounds, buttonBounds rect
	boundsOK, _, _ := user.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&bounds)))
	buttonBoundsOK, _, _ := user.NewProc("GetWindowRect").Call(button, uintptr(unsafe.Pointer(&buttonBounds)))
	buttonVisible, _, _ := user.NewProc("IsWindowVisible").Call(button)
	buttonEnabled, _, _ := user.NewProc("IsWindowEnabled").Call(button)
	exStyle, _, _ := user.NewProc("GetWindowLongPtrW").Call(hwnd, uintptr(^uint32(19))) // GWL_EXSTYLE=-20
	return map[string]any{"pid": os.Getpid(), "hwnd": uint64(hwnd), "focused": readerbroker.ForegroundPID() == uint32(os.Getpid()), "visible": visible != 0, "minimized": iconic != 0, "rectKnown": boundsOK != 0, "rect": bounds, "buttonVisible": buttonVisible != 0, "buttonEnabled": buttonEnabled != 0, "buttonRectKnown": buttonBoundsOK != 0, "buttonRect": buttonBounds, "topmost": exStyle&8 != 0}
}
func presentOwnFixtureUI(hwnd uintptr) (rect, error) {
	var pid uint32
	user.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != uint32(os.Getpid()) {
		return rect{}, fmt.Errorf("fixture presentation HWND ownership changed")
	}
	foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
	monitor, _, _ := user.NewProc("MonitorFromWindow").Call(foreground, 2)
	info := struct {
		Size          uint32
		Monitor, Work rect
		Flags         uint32
	}{}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, _ := user.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info)))
	if monitor == 0 || ok == 0 || info.Work.Right-info.Work.Left < 500 || info.Work.Bottom-info.Work.Top < 220 {
		return rect{}, fmt.Errorf("fixture presentation monitor work area unavailable")
	}
	x := info.Work.Left + (info.Work.Right-info.Work.Left-460)/2
	y := info.Work.Top + (info.Work.Bottom-info.Work.Top-180)/2
	ok, _, _ = user.NewProc("SetWindowPos").Call(hwnd, ^uintptr(0), uintptr(x), uintptr(y), 460, 180, 0x50) // TOPMOST, SHOWWINDOW|NOACTIVATE
	if ok == 0 {
		return info.Work, fmt.Errorf("fixture own UI presentation rejected")
	}
	return info.Work, nil
}

func emit(v any) { outputMu.Lock(); defer outputMu.Unlock(); _ = readerbroker.Write(os.Stdout, v) }
func main() {
	pipe := os.Getenv("AKU_BROKER_FIXTURE_PIPE")
	if readerbroker.ValidateFixturePipe(pipe) != nil {
		os.Exit(2)
	}
	if len(os.Args) == 2 && os.Args[1] == "--indirect" {
		var c command
		if readerbroker.Read(os.Stdin, &c) != nil || c.Op != "read" || !c.Indirect {
			os.Exit(6)
		}
		c.Indirect = false
		runHelper(c)
		return
	}
	if len(os.Args) != 1 {
		os.Exit(7)
	}
	// Private gate: the parent assigns this process to its UI Job before proceed.
	var first command
	if readerbroker.Read(os.Stdin, &first) != nil || first.Op != "start" {
		os.Exit(3)
	}
	runtime.LockOSThread()
	name, _ := windows.UTF16PtrFromString("AkuFixtureBrokerUI")
	title, _ := windows.UTF16PtrFromString("AkuBrowser Synthetic Reader Broker Fixture")
	var retryMu sync.Mutex
	var retry *command
	var retryButton uintptr
	presented := false
	callback := syscall.NewCallback(func(hwnd uintptr, id uint32, w, l uintptr) uintptr {
		if id == 0x8002 {
			retryMu.Lock()
			next := retry
			retryMu.Unlock()
			label := "Retry explicit reader"
			if next != nil && next.Op == "arm_open" {
				label = "Open explicit reader (klik sekarang)"
			}
			text, _ := windows.UTF16PtrFromString(label)
			user.NewProc("SetWindowTextW").Call(retryButton, uintptr(unsafe.Pointer(text)))
			user.NewProc("EnableWindow").Call(retryButton, 1)
			facts := nativeUIFacts(hwnd, retryButton)
			facts["armed"] = true
			emit(facts)
			return 0
		}
		if id == 0x111 && w&0xffff == 100 && w>>16 == 0 {
			retryMu.Lock()
			next := retry
			retry = nil
			retryMu.Unlock()
			user.NewProc("EnableWindow").Call(retryButton, 0)
			if next != nil {
				facts := nativeUIFacts(hwnd, retryButton)
				facts["clicked"] = true
				intent := "retry"
				if next.Op == "arm_open" {
					intent = "open"
				}
				facts["intent"] = intent
				if presented {
					ok, _, _ := user.NewProc("SetWindowPos").Call(hwnd, ^uintptr(1), 0, 0, 0, 0, 0x13) // NOTOPMOST, NOMOVE|NOSIZE|NOACTIVATE
					readback := nativeUIFacts(hwnd, retryButton)
					if ok == 0 || readback["topmost"] != false {
						facts["error"] = "fixture presentation removal rejected"
						emit(facts)
						return 0
					}
					presented = false
					facts["topmost"] = false
				}
				emit(facts)
				if readerbroker.ForegroundPID() == uint32(os.Getpid()) {
					go runHelper(*next)
				}
			}
			return 0
		}
		if id == 0x10 {
			user.NewProc("DestroyWindow").Call(hwnd)
			return 0
		}
		if id == 2 {
			user.NewProc("PostQuitMessage").Call(0)
			return 0
		}
		if id == 0x8001 {
			result, _, _ := user.NewProc("SetForegroundWindow").Call(hwnd)
			emit(map[string]any{"focused": readerbroker.ForegroundPID() == uint32(os.Getpid()), "applied": result != 0, "pid": os.Getpid(), "hwnd": uint64(hwnd)})
			return 0
		}
		result, _, _ := user.NewProc("DefWindowProcW").Call(hwnd, uintptr(id), w, l)
		return result
	})
	instance, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	wc := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: callback, Instance: instance, Name: name, Brush: 6}
	atom, _, _ := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		os.Exit(4)
	}
	hwnd, _, _ := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), 0xcf0000, 80, 80, 460, 180, 0, 0, instance, 0)
	if hwnd == 0 {
		os.Exit(5)
	}
	buttonClass, _ := windows.UTF16PtrFromString("BUTTON")
	buttonLabel := "Retry explicit reader"
	if first.Present {
		buttonLabel = "Open explicit reader (klik sekarang)"
	}
	buttonTitle, _ := windows.UTF16PtrFromString(buttonLabel)
	retryButton, _, _ = user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(buttonTitle)), 0x50000001, 40, 35, 340, 55, hwnd, 100, instance, 0)
	if retryButton == 0 {
		os.Exit(8)
	}
	if first.Present {
		staticClass, _ := windows.UTF16PtrFromString("STATIC")
		instruction, _ := windows.UTF16PtrFromString("Klik sekali saat tombol aktif; tidak perlu menunggu chat.")
		control, _, _ := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(staticClass)), uintptr(unsafe.Pointer(instruction)), 0x50000000, 20, 5, 400, 25, hwnd, 101, instance, 0)
		if control == 0 {
			os.Exit(9)
		}
	}
	user.NewProc("EnableWindow").Call(retryButton, 0)
	showCommand := uintptr(5) // Existing warm fixture behavior.
	if first.Present {
		showCommand = 4
	} // Cold presentation: SW_SHOWNOACTIVATE.
	user.NewProc("ShowWindow").Call(hwnd, showCommand)
	startupApplied := uintptr(0)
	if !first.Present {
		startupApplied, _, _ = user.NewProc("SetForegroundWindow").Call(hwnd)
	}
	initial := nativeUIFacts(hwnd, retryButton)
	var work rect
	var presentationErr error
	if first.Present {
		work, presentationErr = presentOwnFixtureUI(hwnd)
		presented = presentationErr == nil
	}
	facts := nativeUIFacts(hwnd, retryButton)
	facts["ready"] = presentationErr == nil
	facts["initialVisible"] = initial["visible"]
	facts["initialMinimized"] = initial["minimized"]
	facts["initialFocused"] = initial["focused"]
	facts["startupApplied"] = startupApplied != 0
	facts["presented"] = presented
	facts["workArea"] = work
	if presentationErr != nil {
		facts["error"] = presentationErr.Error()
	}
	emit(facts)
	go func() {
		for {
			var next command
			if readerbroker.Read(os.Stdin, &next) != nil {
				user.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0)
				return
			}
			switch next.Op {
			case "arm_retry", "arm_open":
				if next.WrongImage || next.Indirect || next.Request.Validate() != nil {
					emit(readerbroker.Reply{Message: "invalid fixture retry"})
					continue
				}
				retryMu.Lock()
				retry = &next
				retryMu.Unlock()
				user.NewProc("PostMessageW").Call(hwnd, 0x8002, 0, 0)
			case "focus":
				user.NewProc("PostMessageW").Call(hwnd, 0x8001, 0, 0)
			case "read":
				go runHelper(next)
			default:
				emit(readerbroker.Reply{Message: "invalid fixture UI operation"})
			}
		}
	}()
	var msg message
	for {
		r, _, _ := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			return
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}
func runHelper(c command) {
	self, _ := os.Executable()
	image := "aku-reader-broker.exe"
	if c.WrongImage {
		image = "wrong-helper.exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), readerbroker.PreparationLifetime)
	defer cancel()
	child := exec.CommandContext(ctx, filepath.Join(filepath.Dir(self), image), "chrome-extension://"+readerbroker.ExtensionID+"/")
	if c.Indirect {
		child = exec.CommandContext(ctx, self, "--indirect")
	}
	child.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	input, err := child.StdinPipe()
	if err != nil {
		emit(readerbroker.Reply{Message: "fixture helper stdin failed"})
		return
	}
	output, err := child.StdoutPipe()
	if err != nil {
		emit(readerbroker.Reply{Message: "fixture helper stdout failed"})
		return
	}
	if child.Start() != nil {
		emit(readerbroker.Reply{Message: "fixture helper start failed"})
		return
	}
	if c.Indirect {
		_ = readerbroker.Write(input, c)
	} else {
		_ = readerbroker.Write(input, c.Request)
	}
	input.Close()
	var reply readerbroker.Reply
	err = readerbroker.Read(output, &reply)
	io.Copy(io.Discard, output)
	waitErr := child.Wait()
	if err != nil || waitErr != nil {
		emit(readerbroker.Reply{Message: "fixture helper failed"})
		return
	}
	emit(reply)
}
