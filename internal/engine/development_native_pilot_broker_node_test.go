//go:build auth_session_dev

package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Accountless persistent Node actor, not a Chrome/OS ownership attestation.
// Prepare and acquire use the SAME child and paused synthetic document.
type nativePilotOwnerNode struct {
	mu     sync.Mutex
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Reader
	done   chan struct{}
	closed bool
}

func startNativePilotOwnerNode(ctx context.Context, node, script, role string) (*nativePilotOwnerNode, error) {
	cmd := exec.CommandContext(ctx, node, script, "--offline-manual-owner", role)
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("offline-owner-unavailable")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return nil, errors.New("offline-owner-unavailable")
	}
	if cmd.Start() != nil {
		in.Close()
		out.Close()
		return nil, errors.New("offline-owner-unavailable")
	}
	n := &nativePilotOwnerNode{cmd: cmd, in: in, out: bufio.NewReaderSize(out, nativePilotStartupWireMax+1), done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(n.done) }()
	return n, nil
}

func (n *nativePilotOwnerNode) rpc(ctx context.Context, input any) ([]byte, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil, errors.New("offline-owner-closed")
	}
	wire, err := json.Marshal(input)
	if err != nil || len(wire) > nativePilotStartupWireMax {
		return nil, errors.New("offline-owner-request-invalid")
	}
	defer clear(wire)
	stop := context.AfterFunc(ctx, func() { _ = n.cmd.Process.Kill() })
	defer stop()
	if _, err = n.in.Write(append(wire, '\n')); err != nil {
		return nil, errors.New("offline-owner-write-failed")
	}
	line, err := n.out.ReadSlice('\n')
	if err != nil || len(line) < 2 || len(line)-1 > nativePilotStartupWireMax {
		return nil, errors.New("offline-owner-reply-invalid")
	}
	result := bytes.Clone(line[:len(line)-1])
	clear(line)
	return result, nil
}

func (n *nativePilotOwnerNode) close() {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return
	}
	n.closed = true
	_ = n.in.Close()
	n.mu.Unlock()
	select {
	case <-n.done:
	case <-time.After(time.Second):
		_ = n.cmd.Process.Kill()
		<-n.done
	}
}

func TestNativePilotBrokerNodePreparedDocumentIntegration(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("offline Node runtime unavailable")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("offline fixture location unavailable")
	}
	script := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "auth-session-fixture", "x-manual-startup-offline-fixture.mjs")
	for _, wrongDocument := range []bool{false, true} {
		t.Run(map[bool]string{false: "same-owner-prepare-acquire-result", true: "wrong-frozen-document"}[wrongDocument], func(t *testing.T) {
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newTestNativePilotFixture()
			cfg := f.cfg
			cfg.attemptStarted = time.Now()
			cfg.binding.reader = nativePilotOwner{role: "reader", profile: "offline-profile-reader", parent: "owned-sidecar"}
			cfg.binding.collector = nativePilotOwner{role: "collector", profile: "offline-profile-collector", parent: "owned-sidecar"}
			var mu sync.Mutex
			nodes := make(map[string]*nativePilotOwnerNode)
			var preparedCount, acquiredCount atomic.Int32
			ports := nativePilotBrokerPorts{
				now: time.Now,
				prepare: func(ctx context.Context, req nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
					n, err := startNativePilotOwnerNode(parent, node, script, req.role)
					if err != nil {
						return nativePilotPreparedRole{}, err
					}
					t.Cleanup(n.close)
					raw, err := n.rpc(ctx, map[string]any{"op": "prepare"})
					if err != nil {
						n.close()
						return nativePilotPreparedRole{}, err
					}
					defer clear(raw)
					var ready struct {
						Handle string `json:"handle"`
						Pins   struct {
							OwnerID    string `json:"ownerId"`
							ProfileID  string `json:"profileId"`
							ContextID  string `json:"contextId"`
							TargetID   string `json:"targetId"`
							FrameID    string `json:"frameId"`
							DocumentID string `json:"documentId"`
						} `json:"pins"`
					}
					decoder := json.NewDecoder(bytes.NewReader(raw))
					decoder.DisallowUnknownFields()
					if decoder.Decode(&ready) != nil {
						n.close()
						return nativePilotPreparedRole{}, errors.New("offline-ready-invalid")
					}
					o := f.cfg.binding.reader
					if req.role == "collector" {
						o = f.cfg.binding.collector
					}
					o.owner, o.profile, o.context, o.target, o.frame, o.document = ready.Pins.OwnerID, ready.Pins.ProfileID, ready.Pins.ContextID, ready.Pins.TargetID, ready.Pins.FrameID, ready.Pins.DocumentID
					if wrongDocument && req.role == "reader" {
						o.document = "wrong-frozen-document"
					}
					mu.Lock()
					nodes[ready.Handle] = n
					mu.Unlock()
					preparedCount.Add(1)
					return nativePilotPreparedRole{owner: o, handle: ready.Handle}, nil
				},
				inspect: func(ctx context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
					mu.Lock()
					n := nodes[p.handle]
					mu.Unlock()
					if n == nil || ctx.Err() != nil {
						return nativePilotOwner{}, errors.New("offline-inspect-invalid")
					}
					select {
					case <-n.done:
						return nativePilotOwner{}, errors.New("offline-owner-exited")
					default:
					}
					return p.owner, nil // Invented OS facts; actual document came from prepare.
				},
				acquire: func(ctx context.Context, p nativePilotPreparedRole, ch nativePilotCaptureChallenge) ([]byte, error) {
					if preparedCount.Load() != 2 {
						return nil, errors.New("offline-pair-not-ready")
					}
					mu.Lock()
					n := nodes[p.handle]
					mu.Unlock()
					if n == nil {
						return nil, errors.New("offline-owner-missing")
					}
					request, err := ch.requestJSON()
					if err != nil {
						return nil, err
					}
					defer clear(request)
					acquiredCount.Add(1)
					return n.rpc(ctx, map[string]any{"op": "acquire", "handle": p.handle, "request": json.RawMessage(request)})
				},
				state: func(context.Context) (nativePilotBrokerState, error) {
					m := cfg.binding
					return nativePilotBrokerState{account: m.account, enrollment: m.enrollment, pair: m.pair, attempt: m.attempt, derivation: m.derivation, policy: m.policy, consent: m.consent,
						readerWindows: 2, completeInventory: true, parentAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}, nil
				},
				abort: func(ctx context.Context, p nativePilotPreparedRole, preserve bool) error {
					mu.Lock()
					n := nodes[p.handle]
					mu.Unlock()
					if n == nil {
						return nil
					}
					raw, _ := n.rpc(ctx, map[string]any{"op": "abort", "handle": p.handle, "preserveReader": preserve})
					clear(raw)
					n.close()
					return nil // Test cleanup, never a native Job-cleanup receipt.
				},
			}
			b, d := newNativePilotBroker(parent, nativePilotBrokerConfig{enabled: true, offlineTest: true, pilot: cfg}, ports)
			if b == nil {
				t.Fatalf("broker unavailable: %+v", d)
			}
			t.Cleanup(func() { b.invalidate("parent-exited") })
			if d := b.openReader(context.Background(), func() bool { t.Fatal("early reader admitted"); return true }); d.Reason != "admission-closed" {
				t.Fatal("broker admission was not closed")
			}
			d = b.start(context.Background())
			if wrongDocument {
				if d.OfflineValidated || d.ManualEligible {
					t.Fatal("new document substituted for frozen one")
				}
				return
			}
			assertNativePilotClosed(t, d, "manual-attempt-valid")
			if !d.OfflineValidated || preparedCount.Load() != 2 || acquiredCount.Load() != 2 {
				t.Fatal("two-phase broker did not acquire the prepared pair")
			}
			readerCalls := 0
			assertNativePilotClosed(t, b.openReader(context.Background(), func() bool { readerCalls++; return true }), "reader-admitted")
			ticket, d := b.dispatch(context.Background(), "prepared-document-work")
			assertNativePilotClosed(t, d, "work-dispatched")
			assertNativePilotClosed(t, b.claim(context.Background(), ticket, func() bool { return true }), "work-claimed")
			results := 0
			assertNativePilotClosed(t, b.complete(context.Background(), ticket, func() bool { results++; return true }), "result-completed")
			assertNativePilotClosed(t, b.complete(context.Background(), ticket, func() bool { results++; return true }), "result-duplicate")
			if results != 1 || readerCalls != 1 {
				t.Fatal("reader/result effects were duplicated")
			}
			b.invalidate("opted-out")
			if d := b.openReader(context.Background(), func() bool { t.Fatal("revoked reader request admitted"); return true }); d.Reason != "opted-out" {
				t.Fatal("broker revocation lost")
			}
		})
	}
}
