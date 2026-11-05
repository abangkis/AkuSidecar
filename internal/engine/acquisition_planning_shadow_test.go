package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestAcquisitionPlanningShadowDoesNotControlRun(t *testing.T) {
	for _, test := range []struct {
		name      string
		dev       bool
		scheduled bool
		local     bool
		wantCalls int32
	}{
		{"manual development update", true, false, false, 1},
		{"scheduled update", true, true, false, 0},
		{"local planning bypass", true, false, true, 0},
		{"ordinary runtime", false, false, false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			f := newBridgeCommandFixture(t, domain.SourceX)
			f.e.config.Dev = test.dev
			var logs bytes.Buffer
			f.e.logger = log.New(&logs, "", 0)
			if test.scheduled {
				f.exec(t, `UPDATE sessions SET coverage_json=json_set(coverage_json,'$.trigger','scheduler','$.delivery','prepared','$.budgetAuthority','automatic') WHERE id=?`, f.session.ID)
			} else {
				f.exec(t, `UPDATE sessions SET coverage_json=json_set(coverage_json,'$.trigger','user') WHERE id=?`, f.session.ID)
			}
			command := f.claim(t)
			observation := acquisitionPolicyObservation(domain.SourceX)
			observation.Coverage["captureQuality"] = map[string]any{"verdict": "usable_degraded"}
			if test.local {
				observation.Coverage["frontier"].(map[string]any)["continuationReady"] = false
			}
			if err := f.state.SaveObservation(ctx, command.ID, f.run.ID, observation); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			release := make(chan struct{})
			f.e.planningShadow = &acquisitionPlanningShadow{
				plan: func(ctx context.Context, _ reasoning.GLMPlanningShadowInput) (string, domain.ModelUsage, error) {
					calls.Add(1)
					select {
					case <-release:
						return "request_follow_up", domain.ModelUsage{}, nil
					case <-ctx.Done():
						return "", domain.ModelUsage{}, ctx.Err()
					}
				},
			}
			if err := f.e.process(ctx, f.run.ID, true); err != nil {
				t.Fatal(err)
			}
			// A second run offered while the shadow is in flight is dropped.
			if test.wantCalls == 1 {
				f.e.startPlanningShadow(ctx, f.run, observation, "fixture", reasoning.AcquisitionPlan{Decision: "finish"}, nil)
			}
			run, err := f.state.GetRun(ctx, f.run.ID)
			if err != nil || run.Status != "completed" {
				t.Fatalf("authoritative run=%+v err=%v", run, err)
			}
			receipt, _ := run.Coverage["acquisitionPlanning"].(map[string]any)
			if receipt["decision"] != "finish" || receipt["followUpQueued"] != false {
				t.Fatalf("shadow affected follow-up: %+v", receipt)
			}
			close(release)
			if !f.e.WaitForIdle(2 * time.Second) {
				t.Fatal("shadow did not drain")
			}
			if calls.Load() != test.wantCalls {
				t.Fatalf("shadow calls=%d want=%d", calls.Load(), test.wantCalls)
			}
			if test.wantCalls == 1 {
				var got planningShadowResult
				for _, line := range strings.Split(logs.String(), "\n") {
					if raw, ok := strings.CutPrefix(line, "planning_shadow "); ok {
						if err := json.Unmarshal([]byte(raw), &got); err != nil {
							t.Fatal(err)
						}
					}
				}
				if got.PrimaryDecision != "finish" || got.Decision != "request_follow_up" || got.Agreement != "different" {
					t.Fatalf("missing comparison: %+v", got)
				}
			}
			var commands int
			if err := f.db.QueryRow(`SELECT COUNT(*) FROM bridge_commands WHERE run_id=?`, f.run.ID).Scan(&commands); err != nil || commands != 1 {
				t.Fatalf("shadow queued work: commands=%d err=%v", commands, err)
			}
		})
	}
}

func TestAcquisitionPlanningShadowCancelsWithRuntime(t *testing.T) {
	for _, action := range []string{"cancel session", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			ctx := context.Background()
			f := newBridgeCommandFixture(t, domain.SourceX)
			f.e.config.Dev = true
			f.exec(t, `UPDATE sessions SET coverage_json=json_set(coverage_json,'$.trigger','user') WHERE id=?`, f.session.ID)
			started := make(chan struct{})
			stopped := make(chan error, 1)
			f.e.planningShadow = &acquisitionPlanningShadow{
				plan: func(ctx context.Context, _ reasoning.GLMPlanningShadowInput) (string, domain.ModelUsage, error) {
					close(started)
					<-ctx.Done()
					stopped <- ctx.Err()
					return "", domain.ModelUsage{}, ctx.Err()
				},
			}
			f.e.startPlanningShadow(ctx, f.run, acquisitionPolicyObservation(domain.SourceX), "fixture", reasoning.AcquisitionPlan{Decision: "finish"}, nil)
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("shadow did not start")
			}
			if action == "cancel session" {
				if err := f.e.CancelSession(ctx, f.session.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				f.e.Shutdown()
			}
			select {
			case err := <-stopped:
				if err != context.Canceled {
					t.Fatalf("shadow stopped with %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("shadow did not observe cancellation")
			}
		})
	}
}
