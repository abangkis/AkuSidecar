//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConcurrentChromeInputOffline(t *testing.T) {
	request := concurrentChromeInput{Grant: "synthetic-owned-chrome-fixture", Scenario: "overlap-last-close", Root: `C:\qa\run-concurrent-chrome-00000000000000000000000000000001`, NodePath: `C:\tools\node.exe`, ChromePath: `C:\tools\chrome.exe`}
	raw, _ := json.Marshal(request)
	if _, err := parseConcurrentChromeInput(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	for _, alter := range []func(*concurrentChromeInput){func(v *concurrentChromeInput) { v.Grant = "" }, func(v *concurrentChromeInput) { v.Scenario = "native" }, func(v *concurrentChromeInput) { v.Root = `C:\qa\retained` }, func(v *concurrentChromeInput) { v.NodePath = "node" }, func(v *concurrentChromeInput) {
		v.Root = `C:\qa\alias\..\run-concurrent-chrome-00000000000000000000000000000001`
	}} {
		copy := request
		alter(&copy)
		encoded, _ := json.Marshal(copy)
		if _, err := parseConcurrentChromeInput(strings.NewReader(string(encoded))); err == nil {
			t.Fatal("unscoped input accepted")
		}
	}
	for _, invalid := range []string{string(raw) + "{}", strings.TrimSuffix(string(raw), "}") + `,"cookie":"private"}`, strings.Repeat(" ", 16385)} {
		if _, err := parseConcurrentChromeInput(strings.NewReader(invalid)); err == nil {
			t.Fatal("unbounded or extended input accepted")
		}
	}
}
