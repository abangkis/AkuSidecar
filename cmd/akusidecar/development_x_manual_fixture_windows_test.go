//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func TestDevelopmentXManualDispatchBeforeOrdinaryStartup(t *testing.T) {
	options := config.Options{Dev: true, DevelopmentSessionReuse: "synthetic-x-manual-fixture"}
	var output bytes.Buffer
	handled, err := dispatchDevelopmentSession(options, strings.NewReader(`{"grant":"synthetic-x-manual-owned","scenario":"overlap"}`), &output)
	if !handled || err != nil {
		t.Fatalf("manual connector early dispatch failed: %v", err)
	}
	var report map[string]any
	if json.Unmarshal(output.Bytes(), &report) != nil || len(report) != 7 || report["provider"] != options.DevelopmentSessionReuse || report["scenario"] != "overlap" || report["passed"] != true || report["offlineValidated"] != true || report["canAuthorize"] != false || report["nativeQualified"] != false || report["storePermit"] != false {
		t.Fatal("manual synthetic dispatch widened report or authority")
	}
}

func TestDevelopmentXManualDispatchGates(t *testing.T) {
	for _, payload := range []string{`{}`, `{"grant":"wrong","scenario":"overlap"}`, `{"grant":"synthetic-x-manual-owned","scenario":"native"}`, `{"grant":"synthetic-x-manual-owned","scenario":"overlap","canAuthorize":true}`} {
		var output bytes.Buffer
		handled, err := dispatchDevelopmentSession(config.Options{Dev: true, DevelopmentSessionReuse: "synthetic-x-manual-fixture"}, strings.NewReader(payload), &output)
		if !handled || err == nil || output.Len() != 0 {
			t.Fatal("ungranted manual fixture reached ordinary startup or emitted success")
		}
	}
	var output bytes.Buffer
	if handled, err := dispatchDevelopmentSession(config.Options{DevelopmentSessionReuse: "synthetic-x-manual-fixture"}, strings.NewReader(""), &output); handled || err == nil || output.Len() != 0 {
		t.Fatal("manual provider enabled without explicit dev selection")
	}
}
