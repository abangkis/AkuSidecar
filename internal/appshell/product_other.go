//go:build !windows

package appshell

import "fmt"

func platformProductName(string) (string, error) {
	return "", fmt.Errorf("Windows product identity unavailable")
}
