package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const RuntimeVersionMarkerName = ".runtime-version"

var runtimeVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type NewerRuntimeDataError struct {
	DataVersion    string
	RuntimeVersion string
}

func (err *NewerRuntimeDataError) Error() string {
	return fmt.Sprintf(
		"AkuBrowser data belongs to newer runtime %s; installed runtime is %s; use the installer downgrade reset",
		err.DataVersion,
		err.RuntimeVersion,
	)
}

func validateDataRuntimeVersion(databasePath, runtimeVersion string) error {
	if !runtimeVersionPattern.MatchString(runtimeVersion) {
		return errors.New("runtime version is invalid")
	}
	markerPath := filepath.Join(filepath.Dir(databasePath), RuntimeVersionMarkerName)
	data, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read AkuBrowser data runtime marker: %w", err)
	}
	if len(data) > 128 {
		return errors.New("AkuBrowser data runtime marker is invalid")
	}
	dataVersion := strings.TrimSpace(string(data))
	if !runtimeVersionPattern.MatchString(dataVersion) {
		return errors.New("AkuBrowser data runtime marker is invalid")
	}
	if compareRuntimeVersions(dataVersion, runtimeVersion) > 0 {
		return &NewerRuntimeDataError{DataVersion: dataVersion, RuntimeVersion: runtimeVersion}
	}
	return nil
}

func writeDataRuntimeVersion(databasePath, runtimeVersion string) error {
	directory := filepath.Dir(databasePath)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create AkuBrowser data directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".runtime-version-*")
	if err != nil {
		return fmt.Errorf("create AkuBrowser data runtime marker: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(runtimeVersion + "\n"); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	markerPath := filepath.Join(directory, RuntimeVersionMarkerName)
	if err := os.Remove(markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace AkuBrowser data runtime marker: %w", err)
	}
	if err := os.Rename(temporaryPath, markerPath); err != nil {
		return fmt.Errorf("activate AkuBrowser data runtime marker: %w", err)
	}
	return nil
}

func compareRuntimeVersions(left, right string) int {
	parse := func(value string) ([3]int, string) {
		parts := strings.SplitN(value, "-", 2)
		coreParts := strings.Split(parts[0], ".")
		var core [3]int
		for index := range core {
			core[index], _ = strconv.Atoi(coreParts[index])
		}
		prerelease := ""
		if len(parts) == 2 {
			prerelease = parts[1]
		}
		return core, prerelease
	}
	leftCore, leftPrerelease := parse(left)
	rightCore, rightPrerelease := parse(right)
	for index := range leftCore {
		if leftCore[index] < rightCore[index] {
			return -1
		}
		if leftCore[index] > rightCore[index] {
			return 1
		}
	}
	if leftPrerelease == rightPrerelease {
		return 0
	}
	if leftPrerelease == "" {
		return 1
	}
	if rightPrerelease == "" {
		return -1
	}
	return strings.Compare(leftPrerelease, rightPrerelease)
}
