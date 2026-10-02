package appshell

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

var profileDirectoryName = regexp.MustCompile(`^[A-Za-z0-9 _-]{1,64}$`)

// ResolveProfileDirectory pins the same Chrome subprofile for headed and
// headless launches. It never creates or copies authentication data.
func ResolveProfileDirectory(userDataDir string) (string, error) {
	file, err := os.Open(filepath.Join(userDataDir, "Local State"))
	if errors.Is(err, os.ErrNotExist) {
		return "Default", nil // First launch of a new user-data directory.
	}
	if err != nil {
		return "", errors.New("Chrome profile selection is unavailable")
	}
	defer file.Close()
	const maxBytes = 8 << 20
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil || len(raw) > maxBytes {
		return "", errors.New("Chrome profile selection exceeds its read limit")
	}
	var state struct {
		Profile struct {
			LastUsed string `json:"last_used"`
		} `json:"profile"`
	}
	if json.Unmarshal(raw, &state) != nil {
		return "", errors.New("Chrome profile selection is invalid")
	}
	name := state.Profile.LastUsed
	if name == "" {
		name = "Default"
	}
	if !profileDirectoryName.MatchString(name) {
		return "", errors.New("Chrome subprofile name is invalid")
	}
	info, err := os.Stat(filepath.Join(userDataDir, name))
	if err != nil || !info.IsDir() {
		return "", errors.New("selected Chrome subprofile is unavailable")
	}
	return name, nil
}

func ValidProfileDirectory(name string) bool { return profileDirectoryName.MatchString(name) }
