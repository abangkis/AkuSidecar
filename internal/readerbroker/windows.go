//go:build windows

package readerbroker

import (
	"context"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

// All pipe operations are overlapped and cancellable. A stalled/forged peer
// cannot pin the broker listener or extend an activation beyond its deadline.
type pipeConn struct {
	h   windows.Handle
	ctx context.Context
}

func (p *pipeConn) operation(start func(*windows.Overlapped, *uint32) error) (uint32, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	ov := windows.Overlapped{HEvent: event}
	var n uint32
	err = start(&ov, &n)
	if err != windows.ERROR_IO_PENDING {
		return n, err
	}
	for {
		if p.ctx.Err() != nil {
			windows.CancelIoEx(p.h, &ov)
			windows.GetOverlappedResult(p.h, &ov, &n, true)
			return 0, p.ctx.Err()
		}
		state, waitErr := windows.WaitForSingleObject(event, 25)
		if waitErr != nil {
			windows.CancelIoEx(p.h, &ov)
			windows.GetOverlappedResult(p.h, &ov, &n, true)
			return 0, waitErr
		}
		if state == windows.WAIT_OBJECT_0 {
			err := windows.GetOverlappedResult(p.h, &ov, &n, false)
			return n, err
		}
	}
}
func (p *pipeConn) Read(b []byte) (int, error) {
	n, err := p.operation(func(o *windows.Overlapped, n *uint32) error { return windows.ReadFile(p.h, b, n, o) })
	if err == nil && n == 0 {
		err = io.EOF
	}
	return int(n), err
}
func (p *pipeConn) Write(b []byte) (int, error) {
	n, err := p.operation(func(o *windows.Overlapped, n *uint32) error { return windows.WriteFile(p.h, b, n, o) })
	return int(n), err
}

var kernel = windows.NewLazySystemDLL("kernel32.dll")
var clientPID = kernel.NewProc("GetNamedPipeClientProcessId")
var serverPID = kernel.NewProc("GetNamedPipeServerProcessId")
var user = windows.NewLazySystemDLL("user32.dll")
var foreground = user.NewProc("GetForegroundWindow")
var windowPID = user.NewProc("GetWindowThreadProcessId")
var getProp = user.NewProc("GetPropW")
var show = user.NewProc("ShowWindowAsync")
var setForeground = user.NewProc("SetForegroundWindow")
var visible = user.NewProc("IsWindowVisible")
var iconic = user.NewProc("IsIconic")

func ProcessImage(pid uint32) (string, error) {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(p)
	var b [32768]uint16
	n := uint32(len(b))
	if err = windows.QueryFullProcessImageName(p, 0, &b[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(b[:n]), nil
}
func ParentPID(pid uint32) (uint32, error) {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	e := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(h, &e); err == nil; err = windows.Process32Next(h, &e) {
		if e.ProcessID == pid {
			return e.ParentProcessID, nil
		}
	}
	return 0, errors.New("reader helper process missing")
}
func ForegroundPID() uint32 {
	hwnd, _, _ := foreground.Call()
	var pid uint32
	windowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// Serve obtains the caller PID from the OS pipe connection, never JSON. Each
// conversation has a hard deadline and is serialized to bound native helpers.
func Serve(ctx context.Context, authorize func(uint32) error, handle func(context.Context, Request, func(Target) (Reply, error)) error) error {
	name, _ := windows.UTF16PtrFromString(PipeName)
	h, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_FIRST_PIPE_INSTANCE|windows.FILE_FLAG_OVERLAPPED, windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|0x8, 1, 8192, 8192, 0, nil)
	if err != nil {
		return fmt.Errorf("reader broker pipe: %w", err)
	}
	defer windows.CloseHandle(h)
	for {
		if ctx.Err() != nil {
			return nil
		}
		f := &pipeConn{h: h, ctx: ctx}
		_, err = f.operation(func(ov *windows.Overlapped, _ *uint32) error { return windows.ConnectNamedPipe(h, ov) })
		if err != nil && err != windows.ERROR_PIPE_CONNECTED {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		deadline, cancel := context.WithTimeout(ctx, Lifetime)
		f.ctx = deadline
		func() {
			var req Request
			if err = Read(f, &req); err != nil {
				return
			}
			// Wait for the client to consume the terminal frame (or disconnect),
			// then reuse this first-instance pipe. This avoids losing unread
			// replies and avoids an inter-conversation pipe-name takeover gap.
			defer func() { var ack Reply; Read(f, &ack) }()
			var pid uint32
			ok, _, _ := clientPID.Call(uintptr(h), uintptr(unsafe.Pointer(&pid)))
			if ok == 0 {
				return
			}
			if err = authorize(pid); err != nil {
				Write(f, Reply{Message: "Reader helper identity rejected: " + err.Error()})
				return
			}
			if err = req.Validate(); err != nil {
				Write(f, Reply{Message: err.Error()})
				return
			}
			err = handle(deadline, req, func(target Target) (Reply, error) {
				return Exchange(deadline, f, target, func() error { return authorize(pid) })
			})
			if err != nil {
				Write(f, Reply{Message: err.Error()})
			} else {
				Write(f, Reply{OK: true})
			}
		}()
		cancel()
		windows.DisconnectNamedPipe(h)
	}
}

func RunClient(ctx context.Context, req Request) Reply {
	if err := req.Validate(); err != nil {
		return Reply{Message: err.Error()}
	}
	parent, err := ParentPID(uint32(os.Getpid()))
	if err != nil || ForegroundPID() != parent {
		return Reply{Message: "AkuBrowser UI must remain foreground"}
	}
	name, _ := windows.UTF16PtrFromString(PipeName)
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return Reply{Message: "Reader broker is unavailable or busy"}
	}
	f := &pipeConn{h: h, ctx: ctx}
	defer windows.CloseHandle(h)
	var server uint32
	ok, _, _ := serverPID.Call(uintptr(h), uintptr(unsafe.Pointer(&server)))
	image, err := ProcessImage(server)
	self, _ := os.Executable()
	base := strings.ToLower(filepath.Base(image))
	if ok == 0 || err != nil || !strings.EqualFold(filepath.Dir(image), filepath.Dir(self)) || (base != "akusidecar.exe" && base != "aku-sidecar.exe") {
		return Reply{Message: "Reader broker server identity rejected"}
	}
	if err = Write(f, req); err != nil {
		return Reply{Message: "Reader broker request failed"}
	}
	var issue Reply
	if err = Read(f, &issue); err != nil {
		return Reply{Message: "Reader preparation timed out"}
	}
	if !issue.OK {
		Write(f, Reply{OK: true})
		return issue
	}
	if len(issue.Ticket) != 64 {
		return Reply{Message: "Reader ticket rejected"}
	}
	if err = Write(f, Reply{Ticket: issue.Ticket}); err != nil {
		return Reply{Message: "Reader claim failed"}
	}
	var binding Reply
	if err = Read(f, &binding); err != nil || !binding.OK || binding.Target == nil {
		return Reply{Message: "Reader binding unavailable"}
	}
	result := activate(ctx, *binding.Target, parent)
	if err = Write(f, result); err != nil {
		return Reply{Message: "Reader result failed"}
	}
	var final Reply
	if err = Read(f, &final); err != nil {
		return Reply{Message: "Reader verification failed"}
	}
	Write(f, Reply{OK: true})
	return final
}

func activate(ctx context.Context, t Target, uiPID uint32) Reply {
	prop, err := windows.UTF16PtrFromString(t.Property)
	if err != nil || t.HWND == 0 || t.PID == 0 || t.Value == 0 || !strings.HasPrefix(t.Property, "AkuBrowser.ExplicitReader.") {
		return Reply{Message: "Reader binding rejected"}
	}
	valid := func() bool {
		var pid uint32
		windowPID.Call(uintptr(t.HWND), uintptr(unsafe.Pointer(&pid)))
		v, _, _ := getProp.Call(uintptr(t.HWND), uintptr(unsafe.Pointer(prop)))
		return ctx.Err() == nil && time.Now().Before(t.Expires) && pid == t.PID && v == uintptr(t.Value)
	}
	if !valid() {
		return Reply{Message: "Reader binding expired or changed"}
	}
	fg, _, _ := foreground.Call()
	v, _, _ := visible.Call(uintptr(t.HWND))
	i, _, _ := iconic.Call(uintptr(t.HWND))
	if fg == uintptr(t.HWND) && v != 0 && i == 0 {
		return Reply{OK: true, Readback: true}
	}
	if ForegroundPID() != uiPID {
		return Reply{Message: "Reader intent expired or UI foreground changed"}
	}
	show.Call(uintptr(t.HWND), 9)
	applied, _, _ := setForeground.Call(uintptr(t.HWND))
	until := time.Now().Add(500 * time.Millisecond)
	for valid() && time.Now().Before(until) {
		fg, _, _ := foreground.Call()
		v, _, _ := visible.Call(uintptr(t.HWND))
		i, _, _ := iconic.Call(uintptr(t.HWND))
		if fg == uintptr(t.HWND) && v != 0 && i == 0 {
			return Reply{OK: true, Applied: applied != 0, Readback: true}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return Reply{Message: "Windows rejected reader activation", Applied: applied != 0}
}
