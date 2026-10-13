//go:build windows && auth_session_fixture

package main

import (
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"os"
)

func main() {
	if len(os.Args) == 3 && os.Args[1] == "--launcher-synthetic-child" {
		if err := engine.RunLauncherSyntheticChild(os.Args[2], os.Stdin); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	for _, arg := range os.Args[1:] {
		const prefix = "--launcher-synthetic-root="
		if len(arg) > len(prefix) && arg[:len(prefix)] == prefix {
			if err := engine.RunLauncherSyntheticRoot(arg[len(prefix):]); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}
	if err := engine.RunAuthSessionProcessFixture(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
