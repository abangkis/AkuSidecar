//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Invented private-port evidence tests orchestration only. It cannot qualify a
// native manifest or be selected by the production registration function.
type sourceNativeOffline struct {
	engine             *Engine
	mutate             func(*sourceNativePair)
	ownership, cleanup bool
	closed, captures   int
	changeAfterCapture bool
}

func (f *sourceNativeOffline) CheckReader(context.Context) error { return nil }
func (f *sourceNativeOffline) Begin(context.Context, sourceNativeChallenge, time.Time) (sourceNativeCollector, error) {
	return f, nil
}
func (f *sourceNativeOffline) Close(context.Context) error {
	f.closed++
	if f.cleanup {
		return errors.New("offline cleanup uncertain")
	}
	return nil
}
func (f *sourceNativeOffline) pair(c sourceNativeChallenge) sourceNativePair {
	v := sourceNativeProof{Source: domain.SourceInstagram, Domain: "native-browser", Recipe: instagramNativeRecipe, Nonce: c.Nonce, ProfileScope: c.ProfileScope, Generation: c.Generation, Role: "reader", OwnerID: "100", ProfileID: "offline-reader", AccountKey: strings.Repeat("a", 64), SessionRevision: strings.Repeat("b", 64), Authenticated: true, ObservedAtMS: f.engine.store.Now().UnixMilli()}
	p := sourceNativePair{Reader: v, Collector: v}
	p.Collector.Role = "collector"
	p.Collector.OwnerID = "200"
	p.Collector.ProfileID = "offline-collector"
	if f.mutate != nil {
		f.mutate(&p)
	}
	return p
}
func (f *sourceNativeOffline) Seed(_ context.Context, c sourceNativeChallenge) (sourceNativePair, error) {
	return f.pair(c), nil
}
func (f *sourceNativeOffline) Proof(_ context.Context, c sourceNativeChallenge) (sourceNativePair, error) {
	return f.pair(c), nil
}
func (f *sourceNativeOffline) CheckOwnership(context.Context, sourceNativePair) error {
	if f.ownership {
		return errors.New("offline ownership changed")
	}
	return nil
}
func (f *sourceNativeOffline) Capture(context.Context, map[string]any) (domain.Observation, error) {
	f.captures++
	if f.changeAfterCapture {
		f.mutate = func(p *sourceNativePair) {
			p.Reader.SessionRevision = strings.Repeat("c", 64)
			p.Collector.SessionRevision = p.Reader.SessionRevision
		}
	}
	return domain.Observation{Source: domain.SourceInstagram, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "instagram:offline-native-contract", Text: "Offline native-port contract fixture"}}}}}, nil
}
func sourceNativeFixture(t *testing.T) (*sourceNativeOwner, *sourceNativeOffline, domain.Run, string) {
	t.Helper()
	e, _ := testEngine(t)
	e.config.Dev = true
	ctx := context.Background()
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceInstagram}
	settings.CollectionMode = "headless"
	settings.CalibrationEnabled = false
	settings.AIDetectionEnabled = false
	s, err := e.store.CreateUpdateSession(ctx, "offline native contract", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	run := s.Runs[0]
	cmd, err := e.store.StartRun(ctx, run.ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	f := &sourceNativeOffline{engine: e}
	o := &sourceNativeOwner{engine: e, source: domain.SourceInstagram, recipe: instagramNativeRecipe, profile: strings.Repeat("d", 64), transport: f}
	return o, f, run, cmd.ID
}
func TestSourceNativeOfflineProofRejection(t *testing.T) {
	cases := map[string]func(*sourceNativePair){
		"source":          func(p *sourceNativePair) { p.Collector.Source = domain.SourceX },
		"domain":          func(p *sourceNativePair) { p.Reader.Domain = "synthetic-fixture" },
		"manifest":        func(p *sourceNativePair) { p.Reader.Recipe = "x-native-v1" },
		"nonce":           func(p *sourceNativePair) { p.Reader.Nonce = "replay" },
		"generation":      func(p *sourceNativePair) { p.Reader.Generation++ },
		"scope":           func(p *sourceNativePair) { p.Reader.ProfileScope = "other" },
		"role":            func(p *sourceNativePair) { p.Collector.Role = "reader" },
		"same-owner":      func(p *sourceNativePair) { p.Collector.OwnerID = p.Reader.OwnerID },
		"same-profile":    func(p *sourceNativePair) { p.Collector.ProfileID = p.Reader.ProfileID },
		"account":         func(p *sourceNativePair) { p.Collector.AccountKey = strings.Repeat("f", 64) },
		"revision":        func(p *sourceNativePair) { p.Collector.SessionRevision = strings.Repeat("f", 64) },
		"unauthenticated": func(p *sourceNativePair) { p.Reader.Authenticated = false },
		"stale":           func(p *sourceNativePair) { p.Reader.ObservedAtMS -= int64(time.Minute / time.Millisecond) },
		"future":          func(p *sourceNativePair) { p.Reader.ObservedAtMS += int64(time.Minute / time.Millisecond) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			o, f, run, _ := sourceNativeFixture(t)
			f.mutate = mutate
			b, err := o.BeginBatch(context.Background(), context.Background(), 1, run)
			if err == nil {
				t.Fatal("invalid native proof accepted")
			}
			if b == nil {
				t.Fatal("partial collector ownership lost")
			}
			if err = b.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			if f.captures != 0 || f.closed != 1 {
				t.Fatal("invalid proof captured or leaked collector")
			}
		})
	}
}
func TestSourceNativeOfflineCommitFences(t *testing.T) {
	for _, mode := range []string{"valid", "ownership", "revision-after-capture", "expired", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			o, f, run, command := sourceNativeFixture(t)
			ctx := context.Background()
			parent, cancel := context.WithCancel(ctx)
			defer cancel()
			batch, err := o.BeginBatch(ctx, parent, 1, run)
			if err != nil {
				t.Fatal(err)
			}
			b := batch.(*sourceNativeBatch)
			switch mode {
			case "ownership":
				f.ownership = true
			case "revision-after-capture":
				f.changeAfterCapture = true
			case "expired":
				b.started = b.started.Add(-time.Minute)
			case "cancelled":
				cancel()
			}
			_, err = b.Capture(ctx, command)
			if (err == nil) != (mode == "valid") {
				t.Fatalf("capture mode %s: %v", mode, err)
			}
			observations, readErr := o.engine.store.Observations(ctx, run.ID)
			if readErr != nil {
				t.Fatal(readErr)
			}
			want := 0
			if mode == "valid" {
				want = 1
			}
			if len(observations) != want {
				t.Fatalf("persisted %d observations", len(observations))
			}
			if _, err = b.Capture(ctx, command); err == nil {
				t.Fatal("claim replay accepted")
			}
			if err = b.Close(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestSourceNativeOfflineGenerationAndCleanup(t *testing.T) {
	o, f, run, _ := sourceNativeFixture(t)
	ctx := context.Background()
	if _, err := o.BeginBatch(ctx, ctx, 2, run); err == nil {
		t.Fatal("generation skip accepted")
	}
	b, err := o.BeginBatch(ctx, ctx, 1, run)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.BeginBatch(ctx, ctx, 2, run); err == nil {
		t.Fatal("overlapping collector accepted")
	}
	f.cleanup = true
	if err = b.Close(ctx); err == nil {
		t.Fatal("cleanup uncertainty erased")
	}
	if err = b.Close(ctx); err == nil || f.closed != 1 {
		t.Fatal("cleanup replay or uncertainty lost")
	}
	if _, err = o.BeginBatch(ctx, ctx, 2, run); err == nil {
		t.Fatal("uncertain owner reused")
	}
}
