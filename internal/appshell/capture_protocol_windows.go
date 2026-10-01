//go:build windows

package appshell

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type windowsCaptureProtocolLaunch struct {
	transport        *captureProtocolTransport
	childInputRead   windows.Handle
	childOutputWrite windows.Handle
	pipeArgs         []string
}

func newCaptureProtocolLaunch() (captureProtocolLaunch, error) {
	var childInputRead, parentInputWrite windows.Handle
	var parentOutputRead, childOutputWrite windows.Handle
	closeHandles := func() {
		for _, handle := range []windows.Handle{childInputRead, parentInputWrite, parentOutputRead, childOutputWrite} {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
	}
	attributes := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	if err := windows.CreatePipe(&childInputRead, &parentInputWrite, attributes, 0); err != nil {
		return nil, errors.New("create private Chrome input pipe failed")
	}
	if err := windows.CreatePipe(&parentOutputRead, &childOutputWrite, attributes, 0); err != nil {
		closeHandles()
		return nil, errors.New("create private Chrome output pipe failed")
	}
	if err := windows.SetHandleInformation(parentInputWrite, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		closeHandles()
		return nil, errors.New("restrict private Chrome input pipe inheritance failed")
	}
	if err := windows.SetHandleInformation(parentOutputRead, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		closeHandles()
		return nil, errors.New("restrict private Chrome output pipe inheritance failed")
	}
	inputText, err := formatCDPPipeHandle(childInputRead)
	if err != nil {
		closeHandles()
		return nil, err
	}
	outputText, err := formatCDPPipeHandle(childOutputWrite)
	if err != nil {
		closeHandles()
		return nil, err
	}
	parentInput := os.NewFile(uintptr(parentInputWrite), "aku-private-cdp-input")
	if parentInput != nil {
		parentInputWrite = 0
	}
	parentOutput := os.NewFile(uintptr(parentOutputRead), "aku-private-cdp-output")
	if parentOutput != nil {
		parentOutputRead = 0
	}
	if parentInput == nil || parentOutput == nil {
		if parentInput != nil {
			_ = parentInput.Close()
		}
		if parentOutput != nil {
			_ = parentOutput.Close()
		}
		closeHandles()
		return nil, errors.New("wrap private Chrome pipes failed")
	}
	return &windowsCaptureProtocolLaunch{
		transport:        newCaptureProtocolTransport(parentOutput, parentInput),
		childInputRead:   childInputRead,
		childOutputWrite: childOutputWrite,
		pipeArgs: []string{
			"--remote-debugging-pipe",
			"--remote-debugging-io-pipes=" + inputText + "," + outputText,
		},
	}, nil
}

func formatCDPPipeHandle(handle windows.Handle) (string, error) {
	value := uint64(handle)
	if handle == 0 || value > uint64(^uint32(0)) {
		return "", errors.New("private Chrome pipe handle is invalid")
	}
	return strconv.FormatUint(value, 10), nil
}

func (p *windowsCaptureProtocolLaunch) args() []string {
	return append([]string(nil), p.pipeArgs...)
}

func (p *windowsCaptureProtocolLaunch) configure(command *exec.Cmd) error {
	if p == nil || command == nil {
		return errors.New("private Chrome command is unavailable")
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.HideWindow = true
	command.SysProcAttr.AdditionalInheritedHandles = append(command.SysProcAttr.AdditionalInheritedHandles,
		syscall.Handle(p.childInputRead), syscall.Handle(p.childOutputWrite))
	return nil
}

func (p *windowsCaptureProtocolLaunch) childStarted() error {
	if p == nil {
		return nil
	}
	if p.childInputRead != 0 {
		if err := windows.CloseHandle(p.childInputRead); err != nil {
			return errors.New("close parent copy of private Chrome input pipe failed")
		}
		p.childInputRead = 0
	}
	if p.childOutputWrite != 0 {
		if err := windows.CloseHandle(p.childOutputWrite); err != nil {
			return errors.New("close parent copy of private Chrome output pipe failed")
		}
		p.childOutputWrite = 0
	}
	return nil
}

func (p *windowsCaptureProtocolLaunch) protocol() CaptureProtocol {
	if p == nil {
		return nil
	}
	return p.transport
}

// closeAfterOwnerExit must only be called after the owning Job is empty or
// after its verified shutdown path completes.
func (p *windowsCaptureProtocolLaunch) closeAfterOwnerExit() error {
	if p == nil {
		return nil
	}
	var handleErr error
	for _, handle := range []*windows.Handle{&p.childInputRead, &p.childOutputWrite} {
		if *handle != 0 {
			if err := windows.CloseHandle(*handle); err != nil && handleErr == nil {
				handleErr = errors.New("close private Chrome pipe handle failed")
			}
			*handle = 0
		}
	}
	if err := p.transport.closeAfterOwnerExit(); err != nil && handleErr == nil {
		handleErr = err
	}
	return handleErr
}
