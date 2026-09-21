package appshell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SplitProfilePaths preserves the capture profile in place and reserves a
// sibling UI directory. Refuse links/reparse aliases rather than risk opening
// the authenticated profile in the UI process.
func SplitProfilePaths(capture string) (string, string, error) {
	if strings.TrimSpace(capture) == "" {
		return "", "", fmt.Errorf("capture profile is required")
	}
	path, err := filepath.Abs(filepath.Clean(capture))
	if err != nil {
		return "", "", err
	}
	ui := path + "-ui-split"
	if info, err := os.Lstat(ui); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", fmt.Errorf("UI profile must be an ordinary directory")
		}
		resolved, err := filepath.EvalSymlinks(ui)
		if err != nil {
			return "", "", err
		}
		original, err := filepath.EvalSymlinks(path)
		if err != nil && !os.IsNotExist(err) {
			return "", "", err
		}
		if original != "" && strings.EqualFold(resolved, original) {
			return "", "", fmt.Errorf("UI and capture profiles must be distinct")
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	return path, ui, nil
}
