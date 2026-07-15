//go:build !windows

package reasoning

import "os/exec"

func configureProcess(_ *exec.Cmd) {}
