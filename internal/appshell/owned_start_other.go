//go:build !windows

package appshell

import "os/exec"

func (o *processOwnership) start(command *exec.Cmd, beforeAttach func(*exec.Cmd, *processOwnership) error) error {
	if err := command.Start(); err != nil {
		return err
	}
	if beforeAttach != nil {
		if err := beforeAttach(command, o); err != nil {
			_ = command.Process.Kill()
			_ = command.Wait()
			return err
		}
	}
	if err := o.attach(command); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return err
	}
	return nil
}
