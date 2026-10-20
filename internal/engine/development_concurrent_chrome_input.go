//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
)

type concurrentChromeInput struct {
	Grant      string `json:"grant"`
	Scenario   string `json:"scenario"`
	Root       string `json:"root"`
	NodePath   string `json:"nodePath"`
	ChromePath string `json:"chromePath"`
}

var concurrentChromeRootName = regexp.MustCompile(`^run-concurrent-chrome-[a-f0-9]{32}$`)

func parseConcurrentChromeInput(input io.Reader) (concurrentChromeInput, error) {
	invalid := errors.New("synthetic concurrent Chrome input rejected")
	raw, err := io.ReadAll(io.LimitReader(input, 16385))
	if err != nil || len(raw) > 16384 {
		return concurrentChromeInput{}, invalid
	}
	var request concurrentChromeInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Grant != "synthetic-owned-chrome-fixture" {
		return request, invalid
	}
	allowed := map[string]bool{"overlap-last-close": true, "opt-out": true, "collector-loss": true, "last-close-denied": true, "mismatch": true, "expiry": true, "parent-cancel": true, "parent-process-death": true, "death-child": true}
	if !allowed[request.Scenario] {
		return request, invalid
	}
	for _, value := range []string{request.Root, request.NodePath, request.ChromePath} {
		if len(value) == 0 || len(value) > 4096 || !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return request, invalid
		}
	}
	if !concurrentChromeRootName.MatchString(filepath.Base(request.Root)) {
		return request, invalid
	}
	return request, nil
}
