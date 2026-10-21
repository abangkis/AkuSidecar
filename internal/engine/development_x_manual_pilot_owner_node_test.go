//go:build auth_session_dev

package engine

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// One accountless Node actor services BOTH roles concurrently over correlated
// stdio. It cannot launch Chrome, open a profile or select native provenance.
type testXManualOwnerNode struct {
	mu, writeMu sync.Mutex
	cmd         *exec.Cmd
	in          io.WriteCloser
	pending     map[int64]chan json.RawMessage
	next        int64
	done        chan struct{}
	core        map[string]nativePilotOwner
}

func (*testXManualOwnerNode) provenance() nativePilotProvenance { return nativePilotOfflineOnly }
func (*testXManualOwnerNode) now() time.Time                    { return time.Now() }
func (n *testXManualOwnerNode) snapshot(ctx context.Context, role string) (nativePilotOwner, error) {
	if ctx.Err() != nil {
		return nativePilotOwner{}, ctx.Err()
	}
	select {
	case <-n.done:
		return nativePilotOwner{}, errors.New("offline-node-exited")
	default:
	}
	o, ok := n.core[role]
	if !ok {
		return o, errors.New("offline-role-invalid")
	}
	return o, nil
}
func (n *testXManualOwnerNode) rpc(ctx context.Context, role, op string, fields map[string]any, out any) error {
	n.mu.Lock()
	n.next++
	id := n.next
	reply := make(chan json.RawMessage, 1)
	n.pending[id] = reply
	n.mu.Unlock()
	defer func() { n.mu.Lock(); delete(n.pending, id); n.mu.Unlock() }()
	message := map[string]any{"id": id, "op": op, "owner": 1, "role": role}
	for key, value := range fields {
		message[key] = value
	}
	raw, err := json.Marshal(message)
	if err != nil || len(raw) > nativePilotStartupWireMax {
		return errors.New("offline-request-invalid")
	}
	defer clear(raw)
	n.writeMu.Lock()
	_, err = n.in.Write(append(raw, '\n'))
	n.writeMu.Unlock()
	if err != nil {
		return errors.New("offline-request-failed")
	}
	select {
	case result := <-reply:
		if len(result) == 0 {
			return errors.New("offline-reply-rejected")
		}
		dest, ok := out.(*json.RawMessage)
		if !ok {
			clear(result)
			return errors.New("offline-output-invalid")
		}
		*dest = result
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-n.done:
		return errors.New("offline-node-exited")
	}
}
func (n *testXManualOwnerNode) cleanup(ctx context.Context, role, handle string, preserve bool) error {
	var token any = handle
	if handle == "" {
		token = nil
	}
	var raw json.RawMessage
	if n.rpc(ctx, role, "manual_abort", map[string]any{"handle": token, "preserveReader": preserve}, &raw) != nil {
		return errors.New("offline-cleanup-failed")
	}
	defer clear(raw)
	var result struct {
		Closed *bool `json:"closed"`
	}
	if xManualPilotDecode(raw, &result) != nil || result.Closed == nil || !*result.Closed {
		return errors.New("offline-cleanup-failed")
	}
	return nil
}

func TestXManualPilotOwnerNodeSharedProtocol(t *testing.T) {
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			t.Fatal("offline Node unavailable")
		}
	}
	project := os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT")
	if project == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("offline source path unavailable")
		}
		project = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join(project, "scripts", "auth-session-fixture", "x-native-manual-owner-offline.mjs"))
	cmd.Env = append(os.Environ(), "AKU_X_NATIVE_MANUAL_OWNER_OFFLINE=1")
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal("offline stdio unavailable")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		t.Fatal("offline stdio unavailable")
	}
	n := &testXManualOwnerNode{cmd: cmd, in: in, pending: map[int64]chan json.RawMessage{}, done: make(chan struct{}), core: map[string]nativePilotOwner{}}
	cfg, fake := testXManualOwnerConfig()
	for index, role := range []string{"reader", "collector"} {
		core := fake.core[role]
		core.pid = index + 1
		core.owner = "1"
		core.profile = "offline-profile-" + role
		if index == 1 {
			core.owner = "2"
		}
		n.core[role] = core
		selected := nativePilotOwner{role: role, profile: core.profile, parent: core.parent}
		if role == "reader" {
			cfg.pilot.binding.reader = selected
		} else {
			cfg.pilot.binding.collector = selected
		}
	}
	if cmd.Start() != nil {
		in.Close()
		t.Fatal("offline Node did not start")
	}
	go func() { _ = cmd.Wait(); close(n.done) }()
	go func() {
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), 2*nativePilotStartupWireMax)
		for scanner.Scan() {
			var reply struct {
				ID     int64           `json:"id"`
				OK     bool            `json:"ok"`
				Code   string          `json:"code"`
				Result json.RawMessage `json:"result"`
			}
			if xManualPilotDecode(scanner.Bytes(), &reply) != nil {
				continue
			}
			n.mu.Lock()
			dest := n.pending[reply.ID]
			n.mu.Unlock()
			if dest != nil {
				var raw json.RawMessage
				if reply.OK {
					raw = append(json.RawMessage{}, reply.Result...)
				}
				dest <- raw
			}
			clear(reply.Result)
		}
	}()
	t.Cleanup(func() { n.writeMu.Lock(); _ = in.Close(); n.writeMu.Unlock(); cancel(); <-n.done })
	b, d := newXManualPilotOwnerBrokerWithBackend(ctx, cfg, n)
	if b == nil {
		t.Fatal("offline shared Node composition unavailable")
	}
	d = b.start(ctx)
	assertXManualOffline(t, d, "manual-attempt-valid")
	if !d.OfflineValidated {
		t.Fatal("offline Node did not produce initial vector")
	}
	ticket, d := b.dispatch(ctx, "node-capture")
	assertXManualOffline(t, d, "work-dispatched")
	assertXManualOffline(t, b.claim(ctx, ticket, func() bool { return true }), "work-claimed")
	results := 0
	assertXManualOffline(t, b.complete(ctx, ticket, func() bool { results++; return true }), "result-completed")
	assertXManualOffline(t, b.complete(ctx, ticket, func() bool { results++; return true }), "result-duplicate")
	if results != 1 {
		t.Fatal("shared Node result repeated")
	}
	assertXManualOffline(t, b.invalidate("opted-out"), "opted-out")
}
