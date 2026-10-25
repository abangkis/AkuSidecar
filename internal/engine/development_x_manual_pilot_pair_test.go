//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Invented owners only; no process, profile, credentials or Store is opened.
func testXManualPair(t *testing.T) (*testXManualOwnerBackend, *xManualPilotOwnerPorts, [2]nativePilotPreparedRole) {
	t.Helper()
	cfg, backend := testXManualOwnerConfig()
	b, ports, d := newXManualPilotOwnerBrokerPorts(context.Background(), cfg, backend)
	if b == nil || ports == nil || b.ports.inspectPair == nil || d.Reason != "admission-closed" {
		t.Fatal("combined owner composition unavailable")
	}
	var prepared [2]nativePilotPreparedRole
	for i, role := range []string{"reader", "collector"} {
		value, err := ports.prepare(context.Background(), nativePilotPrepareRequest{role: role, profile: backend.core[role].profile, parent: backend.core[role].parent, attempt: cfg.pilot.binding.attempt})
		if err != nil {
			t.Fatal("invented preparation failed")
		}
		prepared[i] = value
	}
	return backend, ports, prepared
}

func TestXManualPilotOwnerPairParallelInventoryAndFreshness(t *testing.T) {
	f, p, prepared := testXManualPair(t)
	var snapshots atomic.Int32
	entered := make(chan string, 2)
	release := make(chan struct{})
	p.backend = &testXManualInspectBackend{testXManualOwnerBackend: f,
		snapshotHook: func(ctx context.Context, role string) (nativePilotOwner, error) {
			snapshots.Add(1)
			return f.snapshot(ctx, role)
		},
		rpcHook: func(ctx context.Context, role, op string, fields map[string]any, out any) error {
			if op == "manual_inspect" {
				entered <- role
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return f.rpc(ctx, role, op, fields, out)
		},
	}
	policy := p.cfg.policy
	reads := 0
	p.cfg.policy = func(ctx context.Context) (nativePilotBrokerState, error) {
		reads++
		s, err := policy(ctx)
		s.readerWindows = 99
		return s, err
	}
	f.windows = 3
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type output struct {
		pair nativePilotPairInspection
		err  error
	}
	result := make(chan output, 1)
	go func() { pair, err := p.inspectPair(ctx, prepared[0], prepared[1]); result <- output{pair, err} }()
	seen := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case role := <-entered:
			seen[role] = true
		case <-ctx.Done():
			t.Fatal("pair inspections serialized")
		}
	}
	close(release)
	out := <-result
	if out.err != nil || out.pair.reason != "" || out.pair.reader != prepared[0].owner || out.pair.collector != prepared[1].owner || len(seen) != 2 {
		t.Fatal("parallel inspection failed or changed ownership")
	}
	if snapshots.Load() != 4 || f.ops["manual_inspect"] != 2 || f.ops["manual_inventory"] != 0 || out.pair.state.readerWindows != 3 || reads != 2 {
		t.Fatal("inspection work duplicated or policy inventory trusted")
	}
	f.windows = 4
	again, err := p.inspectPair(ctx, prepared[0], prepared[1])
	if err != nil || again.reason != "" || again.state.readerWindows != 4 || snapshots.Load() != 8 || reads != 4 {
		t.Fatal("combined checkpoint reused cached ownership or inventory")
	}
}

func TestXManualPilotOwnerPairPolicyBeforeAndAfter(t *testing.T) {
	for _, phase := range []int{1, 2} {
		for _, fault := range []string{"account", "enrollment", "pair", "attempt", "derivation", "policy", "consent", "optout", "parent", "channel", "observer", "inventory", "error"} {
			t.Run(fmt.Sprintf("%d/%s", phase, fault), func(t *testing.T) {
				f, p, prepared := testXManualPair(t)
				original := p.cfg.policy
				calls := 0
				p.cfg.policy = func(ctx context.Context) (nativePilotBrokerState, error) {
					calls++
					s, err := original(ctx)
					if calls == phase {
						switch fault {
						case "account":
							s.account = "other"
						case "enrollment":
							s.enrollment = "other"
						case "pair":
							s.pair = "other"
						case "attempt":
							s.attempt = "other"
						case "derivation":
							s.derivation = "other"
						case "policy":
							s.policy++
						case "consent":
							s.consent++
						case "optout":
							s.optedIn = false
						case "parent":
							s.parentAlive = false
						case "channel":
							s.channelHealthy = false
						case "observer":
							s.invalidationsReady = false
						case "inventory":
							s.completeInventory = false
						case "error":
							return s, errors.New("PRIVATE_POLICY")
						}
					}
					return s, err
				}
				pair, err := p.inspectPair(context.Background(), prepared[0], prepared[1])
				if err == nil && pair.reason == "" {
					t.Fatal("policy drift accepted")
				}
				if phase == 1 && f.ops["manual_inspect"] != 0 {
					t.Fatal("invalid preflight inspected owners")
				}
				if strings.Contains(fmt.Sprintf("%v %v", pair, err), "PRIVATE") {
					t.Fatal("policy error leaked")
				}
			})
		}
	}
}

func TestXManualPilotOwnerPairFailureCancelsPeerAndPreservesCause(t *testing.T) {
	for _, badRole := range []string{"reader", "collector"} {
		t.Run(badRole, func(t *testing.T) {
			f, p, prepared := testXManualPair(t)
			peerEntered, peerLeft := make(chan struct{}), make(chan struct{})
			p.backend = &testXManualInspectBackend{testXManualOwnerBackend: f, snapshotHook: f.snapshot,
				rpcHook: func(ctx context.Context, role, op string, fields map[string]any, out any) error {
					if role == badRole {
						select {
						case <-peerEntered:
						case <-ctx.Done():
							return ctx.Err()
						}
						return nativePilotInitialRPC("manual_initial_" + role + "_settings_source_unavailable_primary")
					}
					close(peerEntered)
					<-ctx.Done()
					close(peerLeft)
					return ctx.Err()
				},
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			pair, err := p.inspectPair(ctx, prepared[0], prepared[1])
			d := nativePilotInitialOf(err, "pair", "joint")
			if err == nil || d.role != badRole || d.phase != "settings" || pair.reader != (nativePilotOwner{}) || pair.collector != (nativePilotOwner{}) {
				t.Fatal("failed pair was published or first cause replaced")
			}
			select {
			case <-peerLeft:
			case <-ctx.Done():
				t.Fatal("failed pair left its peer waiting")
			}
		})
	}
}

func TestXManualPilotOwnerPairDeadlineRejectsLateInspections(t *testing.T) {
	f, p, prepared := testXManualPair(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	left := make(chan struct{}, 2)
	p.backend = &testXManualInspectBackend{testXManualOwnerBackend: f, snapshotHook: f.snapshot,
		rpcHook: func(ctx context.Context, role, op string, fields map[string]any, out any) error {
			entered <- struct{}{}
			<-release
			defer func() { left <- struct{}{} }()
			return f.rpc(ctx, role, op, fields, out)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := p.inspectPair(ctx, prepared[0], prepared[1]); done <- err }()
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("late-inspection fixture did not reach both roles")
		}
	}
	<-ctx.Done()
	if err := <-done; err == nil {
		t.Fatal("expired pair checkpoint accepted")
	}
	close(release)
	released = true
	<-left
	<-left
}

func TestXManualPilotOwnerPairPolicyCannotFinishAfterCancellation(t *testing.T) {
	_, p, prepared := testXManualPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := p.cfg.policy
	reads := 0
	p.cfg.policy = func(ctx context.Context) (nativePilotBrokerState, error) {
		reads++
		s, err := original(ctx)
		if reads == 2 {
			cancel()
		}
		return s, err
	}
	if _, err := p.inspectPair(ctx, prepared[0], prepared[1]); err == nil {
		t.Fatal("late policy read authorized a canceled checkpoint")
	}
}
