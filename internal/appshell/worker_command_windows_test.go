//go:build windows

package appshell

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestPrepareWorkerCommandPreservesCallerProcessAttributes(t *testing.T) {
	attributes := &syscall.SysProcAttr{CreationFlags: 0x08000000, NoInheritHandles: true, AdditionalInheritedHandles: []syscall.Handle{17, 23}, CmdLine: "explicit command line"}
	command := exec.Command("unused.exe")
	command.SysProcAttr = attributes
	prepareWorkerCommand(command)
	if command.SysProcAttr == attributes || attributes.HideWindow {
		t.Fatal("worker preparation mutated caller attributes")
	}
	if !command.SysProcAttr.HideWindow || command.SysProcAttr.CreationFlags != attributes.CreationFlags || command.SysProcAttr.NoInheritHandles != attributes.NoInheritHandles || command.SysProcAttr.CmdLine != attributes.CmdLine || len(command.SysProcAttr.AdditionalInheritedHandles) != 2 || command.SysProcAttr.AdditionalInheritedHandles[0] != 17 || command.SysProcAttr.AdditionalInheritedHandles[1] != 23 {
		t.Fatal("worker preparation lost creation or inherited-handle configuration")
	}
}
