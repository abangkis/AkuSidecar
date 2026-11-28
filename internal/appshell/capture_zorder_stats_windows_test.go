//go:build windows

package appshell

import (
	"bytes"
	"encoding/json"
	"log"
	"strings"
	"testing"
)

func TestCaptureZOrderFlushAggregatesOnlyActiveIntervals(t *testing.T) {
	var out bytes.Buffer
	c := captureZOrder{logger: log.New(&out, "", 0)}
	c.logStats()
	c.foregroundCycles, c.attempted, c.applied, c.readback, c.failed, c.lastFailure = 12, 8, 7, 6, 1, "set_window_pos_rejected"
	c.logStats()
	c.logStats()
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatal("idle interval emitted an extra event", out.String())
	}
	var row struct {
		Operation, Phase, Outcome string
		Detail                    map[string]any
	}
	if json.Unmarshal(bytes.TrimPrefix(out.Bytes(), []byte("sidecar_activity ")), &row) != nil || row.Operation != "capture_zorder" || row.Phase != "counters" || row.Outcome != "observed" {
		t.Fatal("unexpected aggregate", out.String())
	}
	for k, want := range map[string]any{"foreground_cycles": 12.0, "attempted": 8.0, "applied": 7.0, "readback": 6.0, "failed": 1.0, "last_failure": "set_window_pos_rejected"} {
		if row.Detail[k] != want {
			t.Errorf("%s = %v, want %v", k, row.Detail[k], want)
		}
	}
}
