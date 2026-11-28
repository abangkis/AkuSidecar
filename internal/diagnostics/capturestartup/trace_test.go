package capturestartup

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestStartupReceiptsDistinguishMissingSampleAndFirstHeartbeat(t *testing.T) {
	var b bytes.Buffer
	trace := New(log.New(&b, "", 0))
	trace.Launch()
	trace.Sample(Document{Number: trace.HostServed(), Loaded: true}, false)
	trace.Sample(Document{Number: trace.HostServed(), Loaded: true, Reload: true, ScriptStarted: true}, false)
	trace.Heartbeat()
	trace.Heartbeat()
	trace.Retire()
	trace.Bootstrap()
	lines := strings.Split(strings.TrimSpace(b.String()), "\n")
	if len(lines) != 6 || !strings.Contains(lines[2], "phase=early_sample") || !strings.Contains(lines[2], "doc=1 loaded=true reload=false script_started=false") || !strings.Contains(lines[4], "doc=2 loaded=true reload=true script_started=true worker_receipt=false") || !strings.Contains(lines[5], "seq=6 phase=heartbeat_recorded") {
		t.Fatal(b.String())
	}
}
