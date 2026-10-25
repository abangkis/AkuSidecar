//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Invented native-shaped values exercise the private receipt schema only.
// Every positive test port has explicit offline provenance; no native source,
// enrollment, seed, runtime admission or qualification is exercised.
type testNativePilotSource struct {
	kind nativePilotProvenance
	read func(context.Context, nativePilotChallenge) (nativePilotReceipt, error)
}

func (s *testNativePilotSource) provenance() nativePilotProvenance { return s.kind }
func (s *testNativePilotSource) observe(ctx context.Context, c nativePilotChallenge) (nativePilotReceipt, error) {
	return s.read(ctx, c)
}

type testNativePilotFixture struct {
	mu     sync.Mutex
	now    time.Time
	state  nativePilotState
	cfg    nativePilotConfig
	source *testNativePilotSource
	ports  nativePilotPorts
	calls  int
	change func(nativePilotChallenge, *nativePilotReceipt)
}

func newTestNativePilotFixture() *testNativePilotFixture {
	f := &testNativePilotFixture{now: time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)}
	r := nativePilotOwner{role: "reader", owner: "private-owner-r", profile: "private-profile-r", directory: "physical-file-id-r", image: "owned-chrome-image", creation: "pid-creation-r", job: "owned-job-r", parent: "owned-sidecar", context: "native-context-r", target: "native-target-r", frame: "native-frame-r", document: "native-document-r", pid: 412, generation: 7}
	c := r
	c.role, c.owner, c.profile, c.directory, c.creation, c.job = "collector", "private-owner-c", "private-profile-c", "physical-file-id-c", "pid-creation-c", "owned-job-c"
	c.context, c.target, c.frame, c.document, c.pid = "native-context-c", "native-target-c", "native-frame-c", "native-document-c", 520
	b := nativePilotBinding{account: "1908123456789012345", enrollment: "private-confirmation", pair: "native-pair", attempt: "native-attempt", derivation: "private-reviewed-two-cookie-receipt", policy: 4, consent: 8, reader: r, collector: c}
	f.cfg = nativePilotConfig{contract: nativePilotContract, enabled: true, limitationAcknowledged: true, binding: b, attemptStarted: f.now}
	f.state = nativePilotState{binding: b, readerWindows: 2, completeInventory: true, parentAlive: true, ownersAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}
	f.source = &testNativePilotSource{kind: nativePilotOfflineOnly}
	f.source.read = func(ctx context.Context, ch nativePilotChallenge) (nativePilotReceipt, error) {
		f.mu.Lock()
		f.calls++
		now, change := f.now, f.change
		f.mu.Unlock()
		base := nativePilotResponse{challenge: ch, observed: now, validUntil: now.Add(nativePilotAge), elapsed: time.Millisecond, fresh: true, direct: true, wellFormed: true, clockValid: true, semanticSuccess: true}
		r := nativePilotReceipt{identity: base, settings: base}
		r.identity.request = "new-id-" + ch.role + strconv.FormatUint(ch.vector, 10)
		r.identity.account, r.identity.source, r.identity.currentSelf = b.account, nativePilotStartupIdentity, true
		r.settings.request = "new-settings-" + ch.role + strconv.FormatUint(ch.vector, 10)
		r.settings.method, r.settings.url, r.settings.qualification = "GET", nativePilotSettingsURL, nativePilotSettingsQualification
		if change != nil {
			change(ch, &r)
		}
		return r, nil
	}
	f.ports = nativePilotPorts{source: f.source, now: func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now }, owners: func(context.Context) (nativePilotState, error) { f.mu.Lock(); defer f.mu.Unlock(); return f.state, nil }}
	return f
}
func (f *testNativePilotFixture) advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	f.mu.Unlock()
}
func (f *testNativePilotFixture) adapter(t *testing.T) *nativePilotAdapter {
	t.Helper()
	a, d := newNativePilotAdapter(f.cfg, f.ports)
	if a == nil {
		t.Fatalf("constructor: %+v", d)
	}
	return a
}
func assertNativePilotClosed(t *testing.T, d nativePilotDiagnostic, reason string) {
	t.Helper()
	if d.Reason != reason || d.CanAuthorize || d.NativeQualified || d.StorePermit || d.ManualEligible {
		t.Fatalf("unexpected diagnostic: %+v", d)
	}
}
func initialNativePilotOffline(t *testing.T, a *nativePilotAdapter) *nativePilotVector {
	t.Helper()
	v, d := a.initialEvidence(context.Background())
	assertNativePilotClosed(t, d, "startup-validated")
	if v == nil || !d.OfflineValidated || v.provenance != nativePilotOfflineOnly {
		t.Fatal("offline receipt validation missing")
	}
	return v
}

func TestNativePilotDefaultAndUnqualifiedStartup(t *testing.T) {
	t.Run("default-off", func(t *testing.T) {
		f := newTestNativePilotFixture()
		f.cfg.enabled = false
		a, d := newNativePilotAdapter(f.cfg, f.ports)
		assertNativePilotClosed(t, d, "admission-disabled")
		if a != nil || f.calls != 0 {
			t.Fatal("disabled adapter had effects")
		}
	})
	for _, absent := range []bool{true, false} {
		t.Run(fmt.Sprint("unqualified-", absent), func(t *testing.T) {
			f := newTestNativePilotFixture()
			if absent {
				f.ports.source = nil
			} else {
				f.source.kind = nativePilotUnqualified
			}
			a := f.adapter(t)
			v, d := a.initialEvidence(context.Background())
			assertNativePilotClosed(t, d, "identity-unavailable")
			if v != nil || f.calls != 0 {
				t.Fatal("unqualified source was sampled or accepted")
			}
			_, d = a.initialEvidence(context.Background())
			assertNativePilotClosed(t, d, "identity-unavailable")
		})
	}
}

func TestNativePilotManualHorizonAndNoRenewal(t *testing.T) {
	f := newTestNativePilotFixture()
	a := f.adapter(t)
	v := initialNativePilotOffline(t, a)
	f.advance(59 * time.Second)
	d := a.valid(context.Background(), v)
	assertNativePilotClosed(t, d, "manual-attempt-valid")
	if !d.OfflineValidated || f.calls != 2 {
		t.Fatal("manual check resampled startup evidence")
	}
	_, d = a.initialEvidence(context.Background())
	assertNativePilotClosed(t, d, "startup-already-attempted")
	f.advance(time.Second)
	assertNativePilotClosed(t, a.valid(context.Background(), v), "attempt-expired")
	if a.current != nil {
		t.Fatal("expired vector retained")
	}
}

func TestNativePilotProcessScopedContextIDs(t *testing.T) {
	for _, kind := range []string{"equal-context", "wrong-process-receipt", "wrong-role-receipt", "same-process"} {
		t.Run(kind, func(t *testing.T) {
			f := newTestNativePilotFixture()
			f.cfg.binding.reader.context, f.cfg.binding.collector.context = "2", "2"
			f.state.binding = f.cfg.binding
			if kind == "same-process" {
				f.cfg.binding.collector.pid = f.cfg.binding.reader.pid
				a, d := newNativePilotAdapter(f.cfg, f.ports)
				assertNativePilotClosed(t, d, "invalid-configuration")
				if a != nil {
					t.Fatal("two roles sharing a process accepted")
				}
				return
			}
			f.change = func(c nativePilotChallenge, receipt *nativePilotReceipt) {
				if c.role != "reader" {
					return
				}
				switch kind {
				case "wrong-process-receipt":
					receipt.identity.challenge.owner.pid = c.binding.collector.pid
				case "wrong-role-receipt":
					receipt.identity.challenge.owner = c.binding.collector
				}
			}
			a := f.adapter(t)
			v, d := a.initialEvidence(context.Background())
			if kind == "equal-context" {
				assertNativePilotClosed(t, d, "startup-validated")
				if v == nil || !d.OfflineValidated {
					t.Fatal("distinct process-scoped contexts rejected")
				}
			} else {
				assertNativePilotClosed(t, d, "evidence-invalid")
				if v != nil {
					t.Fatal("wrong process or role escaped exact tuple validation")
				}
			}
		})
	}
}

func TestNativePilotAttemptStartBeforeSeedCannotReset(t *testing.T) {
	f := newTestNativePilotFixture()
	f.advance(59 * time.Second)
	a := f.adapter(t)
	v := initialNativePilotOffline(t, a)
	if v.expires != f.cfg.attemptStarted.Add(nativePilotLife) {
		t.Fatal("startup reset attempt horizon")
	}
	f.advance(time.Second)
	assertNativePilotClosed(t, a.valid(context.Background(), v), "attempt-expired")
}

func TestNativePilotEarlierExplicitHorizon(t *testing.T) {
	f := newTestNativePilotFixture()
	f.cfg.attemptValidUntil = f.now.Add(12 * time.Second)
	a := f.adapter(t)
	v := initialNativePilotOffline(t, a)
	f.advance(12 * time.Second)
	assertNativePilotClosed(t, a.valid(context.Background(), v), "attempt-expired")
}

func TestNativePilotConfigurationPins(t *testing.T) {
	cases := map[string]func(*nativePilotConfig){
		"old-policy":         func(c *nativePilotConfig) { c.contract = "aku.x.dev-pilot.snapshot.v1" },
		"missing-ack":        func(c *nativePilotConfig) { c.limitationAcknowledged = false },
		"no-enrollment":      func(c *nativePilotConfig) { c.binding.enrollment = "" },
		"no-derivation":      func(c *nativePilotConfig) { c.binding.derivation = "" },
		"same-profile":       func(c *nativePilotConfig) { c.binding.collector.profile = c.binding.reader.profile },
		"same-directory":     func(c *nativePilotConfig) { c.binding.collector.directory = c.binding.reader.directory },
		"same-process":       func(c *nativePilotConfig) { c.binding.collector.pid = c.binding.reader.pid },
		"missing-job":        func(c *nativePilotConfig) { c.binding.collector.job = "" },
		"missing-document":   func(c *nativePilotConfig) { c.binding.reader.document = "" },
		"wrong-role":         func(c *nativePilotConfig) { c.binding.reader.role = "collector" },
		"handle-account":     func(c *nativePilotConfig) { c.binding.account = "@user" },
		"synthetic-account":  func(c *nativePilotConfig) { c.binding.account = "synthetic:account" },
		"zero-prefix":        func(c *nativePilotConfig) { c.binding.account = "0123" },
		"oversize-account":   func(c *nativePilotConfig) { c.binding.account = strings.Repeat("1", 21) },
		"oversize-pin":       func(c *nativePilotConfig) { c.binding.reader.document = strings.Repeat("x", 513) },
		"oversize-directory": func(c *nativePilotConfig) { c.binding.reader.directory = strings.Repeat("x", 4097) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f := newTestNativePilotFixture()
			mutate(&f.cfg)
			a, d := newNativePilotAdapter(f.cfg, f.ports)
			assertNativePilotClosed(t, d, "invalid-configuration")
			if a != nil || f.calls != 0 {
				t.Fatal("invalid pins accepted")
			}
		})
	}
}

func TestNativePilotReceiptBindingAndFreshness(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(*testNativePilotFixture, nativePilotChallenge, *nativePilotReceipt)
	}{
		{"wrong-account", "account-mismatch", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.account = "1908123456789012346"
		}},
		{"old-bootstrap", "identity-unavailable", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.currentSelf = false
		}},
		{"wrong-source", "identity-unavailable", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.source = "home_document_fetch.v4"
		}},
		{"redirect", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.direct = false
		}},
		{"cache-or-sw", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.fresh = false
		}},
		{"malformed", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.wellFormed = false
		}},
		{"wrong-document", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.challenge.owner.document = "replacement"
		}},
		{"wrong-role", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.challenge.role = "other"
		}},
		{"no-correlation", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.request = ""
		}},
		{"duplicate-response", "request-replayed", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.request = r.identity.request
		}},
		{"http200-only", "authentication-rejected", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.semanticSuccess = false
		}},
		{"wrong-route", "authentication-rejected", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.url = "https://x.com/other"
		}},
		{"wrong-qualification", "authentication-rejected", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.qualification = "other"
		}},
		{"future-timestamp", "evidence-expired", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.observed = r.identity.observed.Add(time.Nanosecond)
		}},
		{"stale-timestamp", "evidence-expired", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.observed = r.identity.observed.Add(-nativePilotAge)
		}},
		{"source-expiry", "evidence-expired", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.identity.validUntil = r.identity.observed
		}},
		{"source-clock", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.clockValid = false
		}},
		{"source-elapsed", "evidence-invalid", func(_ *testNativePilotFixture, _ nativePilotChallenge, r *nativePilotReceipt) {
			r.settings.elapsed = nativePilotStartup
		}},
		{"exact-joint", "joint-deadline-exceeded", func(f *testNativePilotFixture, _ nativePilotChallenge, _ *nativePilotReceipt) {
			f.advance(nativePilotStartup)
		}},
		{"clock-rollback", "clock-regressed", func(f *testNativePilotFixture, _ nativePilotChallenge, _ *nativePilotReceipt) {
			f.advance(-time.Nanosecond)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestNativePilotFixture()
			f.change = func(c nativePilotChallenge, r *nativePilotReceipt) { tc.change(f, c, r) }
			a := f.adapter(t)
			v, d := a.initialEvidence(context.Background())
			assertNativePilotClosed(t, d, tc.reason)
			if v != nil || a.current != nil || !a.stopped {
				t.Fatal("failed initial vector survived")
			}
		})
	}
}

func TestNativePilotEarlierSampleDeadlinePublication(t *testing.T) {
	f := newTestNativePilotFixture()
	f.change = func(c nativePilotChallenge, r *nativePilotReceipt) {
		if c.role == "reader" {
			r.identity.validUntil = r.identity.observed.Add(100 * time.Millisecond)
		} else {
			f.advance(100 * time.Millisecond)
		}
	}
	a := f.adapter(t)
	v, d := a.initialEvidence(context.Background())
	assertNativePilotClosed(t, d, "evidence-expired")
	if v != nil {
		t.Fatal("late pair publication accepted")
	}
}

// Policy-clock advances exercise the boundaries without slow wall-clock sleeps.
// The first owner checkpoint is startup overhead; the second is publication.
func TestNativePilotStartupAndSampleFreshnessAreIndependent(t *testing.T) {
	for _, tc := range []struct {
		name           string
		prepare, final time.Duration
		valid          bool
	}{
		{"slow-startup-fresh-samples", 3 * time.Second, 1999 * time.Millisecond, true},
		{"stale-at-final-ownership", 3 * time.Second, 2 * time.Second, false},
		{"startup-last-instant", 5998 * time.Millisecond, time.Millisecond, true},
		{"startup-exact-limit", 5999 * time.Millisecond, time.Millisecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestNativePilotFixture()
			previous, checks := f.ports.owners, 0
			f.ports.owners = func(ctx context.Context) (nativePilotState, error) {
				checks++
				if checks == 1 {
					f.advance(tc.prepare)
				} else {
					f.advance(tc.final)
				}
				return previous(ctx)
			}
			// A source elapsed duration may exceed the sample freshness limit.
			// Its observed timestamps still come from the fresh fixture read.
			f.change = func(_ nativePilotChallenge, r *nativePilotReceipt) {
				r.identity.elapsed, r.settings.elapsed = tc.prepare, tc.prepare
			}
			a := f.adapter(t)
			v, d := a.initialEvidence(context.Background())
			if tc.valid {
				if v == nil || !d.OfflineValidated || !v.expires.Equal(f.cfg.attemptStarted.Add(60*time.Second)) {
					t.Fatal("fresh samples rejected or startup renewed the attempt clock")
				}
			} else if v != nil || d.OfflineValidated || a.current != nil || !a.stopped {
				t.Fatal("startup or sample freshness fence bypassed")
			}
		})
	}
}

func TestNativePilotSixSecondStartupCannotExtendEarlierAttemptLimit(t *testing.T) {
	f := newTestNativePilotFixture()
	f.cfg.attemptStarted = f.now.Add(-57 * time.Second)
	f.cfg.attemptValidUntil = f.now.Add(2 * time.Second)
	previous := f.ports.owners
	f.ports.owners = func(ctx context.Context) (nativePilotState, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Fatal("startup exceeded the shorter explicit attempt limit")
		}
		f.advance(2 * time.Second)
		return previous(ctx)
	}
	a := f.adapter(t)
	v, d := a.initialEvidence(context.Background())
	if v != nil || d.Reason != "attempt-expired" || d.OfflineValidated {
		t.Fatal("startup extended the earlier attempt limit")
	}
}

func TestNativePilotOwnerRecheckAndKnownInvalidations(t *testing.T) {
	cases := []struct {
		name, reason string
		mutate       func(*nativePilotState)
	}{
		{"optout", "opted-out", func(s *nativePilotState) { s.optedIn = false }},
		{"last-window", "reader-lost", func(s *nativePilotState) { s.readerWindows = 0 }},
		{"partial-inventory", "reader-lost", func(s *nativePilotState) { s.completeInventory = false }},
		{"parent-exit", "parent-exited", func(s *nativePilotState) { s.parentAlive = false }},
		{"channel-loss", "evidence-channel-lost", func(s *nativePilotState) { s.channelHealthy = false }},
		{"no-invalidation-observer", "evidence-channel-lost", func(s *nativePilotState) { s.invalidationsReady = false }},
		{"owner-dead", "owner-or-policy-changed", func(s *nativePilotState) { s.ownersAlive = false }},
		{"pid-reuse", "owner-or-policy-changed", func(s *nativePilotState) { s.binding.collector.creation = "new-creation" }},
		{"document-replaced", "owner-or-policy-changed", func(s *nativePilotState) { s.binding.reader.document = "new-document" }},
		{"policy-drift", "owner-or-policy-changed", func(s *nativePilotState) { s.binding.policy++ }},
		{"consent-drift", "owner-or-policy-changed", func(s *nativePilotState) { s.binding.consent++ }},
	}
	for _, tc := range cases {
		for _, phase := range []string{"after-call", "after-startup"} {
			t.Run(tc.name+"/"+phase, func(t *testing.T) {
				f := newTestNativePilotFixture()
				a := f.adapter(t)
				if phase == "after-call" {
					f.change = func(nativePilotChallenge, *nativePilotReceipt) { f.mu.Lock(); tc.mutate(&f.state); f.mu.Unlock() }
					v, d := a.initialEvidence(context.Background())
					assertNativePilotClosed(t, d, tc.reason)
					if v != nil {
						t.Fatal("owner drift vector accepted")
					}
				} else {
					v := initialNativePilotOffline(t, a)
					f.mu.Lock()
					tc.mutate(&f.state)
					f.mu.Unlock()
					assertNativePilotClosed(t, a.valid(context.Background(), v), tc.reason)
					if a.current != nil {
						t.Fatal("stale result vector retained")
					}
				}
			})
		}
	}
}

func TestNativePilotSerializedAndRevocationRace(t *testing.T) {
	f := newTestNativePilotFixture()
	entered, release := make(chan struct{}), make(chan struct{})
	f.change = func(c nativePilotChallenge, _ *nativePilotReceipt) {
		if c.role == "reader" {
			close(entered)
			<-release
		}
	}
	a := f.adapter(t)
	done := make(chan nativePilotDiagnostic, 1)
	go func() { _, d := a.initialEvidence(context.Background()); done <- d }()
	<-entered
	_, d := a.initialEvidence(context.Background())
	assertNativePilotClosed(t, d, "acquisition-busy")
	a.invalidate("opted-out")
	close(release)
	assertNativePilotClosed(t, <-done, "opted-out")
	if a.current != nil {
		t.Fatal("late callback revived invalidated vector")
	}
}

func TestNativePilotCanceledSourceAndPrivateErrors(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprint(during), func(t *testing.T) {
			f := newTestNativePilotFixture()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if during {
				f.change = func(nativePilotChallenge, *nativePilotReceipt) { cancel() }
			} else {
				cancel()
			}
			a := f.adapter(t)
			v, d := a.initialEvidence(ctx)
			assertNativePilotClosed(t, d, "canceled")
			if v != nil {
				t.Fatal("canceled vector accepted")
			}
		})
	}
	f := newTestNativePilotFixture()
	f.source.read = func(context.Context, nativePilotChallenge) (nativePilotReceipt, error) {
		return nativePilotReceipt{}, errors.New("private-token-secret")
	}
	a := f.adapter(t)
	_, d := a.initialEvidence(context.Background())
	assertNativePilotClosed(t, d, "source-unavailable")
}

func TestNativePilotVectorFencingAndPrivacy(t *testing.T) {
	f := newTestNativePilotFixture()
	a := f.adapter(t)
	v := initialNativePilotOffline(t, a)
	other := newTestNativePilotFixture().adapter(t)
	assertNativePilotClosed(t, other.valid(context.Background(), v), "stale-vector")
	forged := *v
	assertNativePilotClosed(t, a.valid(context.Background(), &forged), "stale-vector")
	for _, value := range []any{f.cfg, f.state, f.cfg.binding.reader, v, a, nativePilotReceipt{identity: nativePilotResponse{account: f.cfg.binding.account}}} {
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private structure serialized")
		}
		for _, formatted := range []string{fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if strings.Contains(formatted, f.cfg.binding.account) || strings.Contains(formatted, "private-token") || strings.Contains(formatted, f.cfg.binding.reader.document) {
				t.Fatal("private references formatted")
			}
		}
	}
	for _, event := range []string{"identity-mismatch", "authentication-rejected", "cleanup-uncertain", "candidate-cookie-write", "private-token-secret"} {
		g := newTestNativePilotFixture()
		b := g.adapter(t)
		token := initialNativePilotOffline(t, b)
		d := b.invalidate(event)
		if event == "private-token-secret" {
			event = "evidence-invalid"
		}
		assertNativePilotClosed(t, d, event)
		assertNativePilotClosed(t, b.valid(context.Background(), token), event)
	}
}
