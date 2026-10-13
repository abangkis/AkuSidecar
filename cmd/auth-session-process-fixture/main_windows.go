//go:build windows && auth_session_fixture

package main

import (
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"os"
)

func main() {
	if err := engine.RunAuthSessionProcessFixture(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
