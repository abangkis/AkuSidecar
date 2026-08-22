//go:build !windows

package appshell

import "errors"

func platformVersion(path string) (string, error) {
	return "", errors.New("platform version probe is unavailable")
}
