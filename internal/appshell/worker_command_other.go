//go:build !windows

package appshell

import "os/exec"

func prepareWorkerCommand(command *exec.Cmd) { prepareCommand(command) }
