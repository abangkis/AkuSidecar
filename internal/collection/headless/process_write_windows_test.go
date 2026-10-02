//go:build windows

package headless

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

func TestOwnedWorkerBlockedWriteIsRetired(t *testing.T) {
	node := os.Getenv("AKU_HEADLESS_WRITE_SMOKE_NODE")
	if node == "" {
		t.Skip("explicit packaged Node required; no Chrome/profile is used")
	}
	cmd := exec.Command(node, "-e", `process.stdout.write('ready\n');setInterval(()=>{},1000)`)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := appshell.StartOwnedCommand(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Terminate()
	ready := make(chan bool, 1)
	go func() {
		line, err := bufio.NewReader(output).ReadString('\n')
		ready <- err == nil && line == "ready\n"
	}()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("fixture did not start")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("fixture startup timed out")
	}
	p := &Process{owner: owner, input: input, replies: make(chan reply)}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = p.call(ctx, map[string]any{"type": "init", "padding": strings.Repeat("x", 256<<10)})
	if err == nil || !strings.Contains(err.Error(), "write timed out") {
		t.Fatalf("expected write timeout, got %v", err)
	}
	select {
	case <-owner.Closed():
	case <-time.After(time.Second):
		t.Fatal("failed worker ownership was not released")
	}
}
