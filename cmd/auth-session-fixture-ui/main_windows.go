//go:build windows && auth_session_fixture

// The isolated fixture owns a real foreground Win32 window and launches the
// unchanged native-message entrypoint directly. It accepts fixed actions only.
package main

import (
	"context"
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
	callback := syscall.NewCallback(func(hwnd uintptr, id uint32, w, l uintptr) uintptr {
		if id == 0x8002 {
			user.NewProc("EnableWindow").Call(retryButton, 1)
			emit(map[string]any{"armed": true})
			return 0
		}
		if id == 0x111 && w&0xffff == 100 && w>>16 == 0 {
			retryMu.Lock()
			next := retry
			retry = nil
			retryMu.Unlock()
			user.NewProc("EnableWindow").Call(retryButton, 0)
			if next != nil && readerbroker.ForegroundPID() == uint32(os.Getpid()) {
				emit(map[string]any{"clicked": true, "focused": true, "pid": os.Getpid(), "hwnd": uint64(hwnd)})
				go runHelper(*next)
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
	buttonTitle, _ := windows.UTF16PtrFromString("Retry explicit reader")
	retryButton, _, _ = user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(buttonClass)), uintptr(unsafe.Pointer(buttonTitle)), 0x50000001, 40, 35, 340, 55, hwnd, 100, instance, 0)
	if retryButton == 0 {
		os.Exit(8)
	}
	user.NewProc("EnableWindow").Call(retryButton, 0)
	user.NewProc("ShowWindow").Call(hwnd, 5)
	user.NewProc("SetForegroundWindow").Call(hwnd)
	emit(map[string]any{"ready": true, "focused": readerbroker.ForegroundPID() == uint32(os.Getpid()), "pid": os.Getpid(), "hwnd": uint64(hwnd)})
	go func() {
		for {
			var next command
			if readerbroker.Read(os.Stdin, &next) != nil {
				user.NewProc("PostMessageW").Call(hwnd, 0x10, 0, 0)
				return
			}
			switch next.Op {
			case "arm_retry":
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
