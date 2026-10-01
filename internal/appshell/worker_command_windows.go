//go:build windows

package appshell

import (
	"os/exec"
	"syscall"
)

func prepareWorkerCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
