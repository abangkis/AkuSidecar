//go:build windows

package appshell

import (
	"os/exec"
	"syscall"
)

func prepareWorkerCommand(command *exec.Cmd) {
	attributes := syscall.SysProcAttr{}
	if command.SysProcAttr != nil {
		attributes = *command.SysProcAttr
	}
	attributes.HideWindow = true
	command.SysProcAttr = &attributes
}
