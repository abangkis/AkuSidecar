//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

var manualRuntimeScenarios = []string{"overlap", "duplicate-result", "expiry", "opt-out", "parent-cancel", "owner-drift", "last-window", "source-rejection", "broker-rejection", "prepare-failure"}

// This fixture process is entirely in memory. No OS process or profile exists.
type manualRuntimeFixtureProcess struct {
	driver           string
	pid              int
	done             chan error
	once             sync.Once
	entered, release chan struct{}
	probe            func() error
	owner            nativePilotOwner
	resultSource     domain.Source
}

func (p *manualRuntimeFixtureProcess) nativePilotCollectorOwner() nativePilotOwner { return p.owner }

func (p *manualRuntimeFixtureProcess) Driver() string                      { return p.driver }
func (p *manualRuntimeFixtureProcess) PID() int                            { return p.pid }
func (p *manualRuntimeFixtureProcess) Done() <-chan error                  { return p.done }
func (p *manualRuntimeFixtureProcess) Terminate()                          { p.once.Do(func() { close(p.done) }) }
func (p *manualRuntimeFixtureProcess) CloseForRetry(context.Context) error { p.Terminate(); return nil }
func (*manualRuntimeFixtureProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("offline process has no extensions")
}
func (*manualRuntimeFixtureProcess) ReplacementReadiness(context.Context) error {
	return errors.New("offline replacement forbidden")
}
func (p *manualRuntimeFixtureProcess) Capture(ctx context.Context, source domain.Source, _ map[string]any) (domain.Observation, error) {
	if p.driver != "headless" || source != domain.SourceX {
		return domain.Observation{}, errors.New("offline capture rejected")
	}
	if p.probe != nil {
		if err := p.probe(); err != nil {
			return domain.Observation{}, err
		}
	}
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return domain.Observation{}, ctx.Err()
	case <-p.done:
		return domain.Observation{}, errors.New("offline collector retired")
	case <-p.release:
		return domain.Observation{Source: p.resultSource}, nil
	}
}

type manualRuntimeFixture struct {
	mu                sync.Mutex
	now               time.Time
	state             nativePilotBrokerState
	actual            map[string]nativePilotOwner
	prepared          map[string]nativePilotPreparedRole
	acquired          map[string]int
	reader, collector *captureruntime.Manager
	backend           *manualRuntimeFixtureProcess
	runtime           *nativePilotRuntime
	broker            *nativePilotBroker
	cancel            context.CancelFunc
	prepareFailure    bool
	opens             int
}

func newManualRuntimeFixture(prepareFailure bool) (*manualRuntimeFixture, error) {
	ctx, cancel := context.WithCancel(context.Background())
	f := &manualRuntimeFixture{now: time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), actual: map[string]nativePilotOwner{}, prepared: map[string]nativePilotPreparedRole{}, acquired: map[string]int{}, cancel: cancel, prepareFailure: prepareFailure}
	rp := &manualRuntimeFixtureProcess{driver: "browser", pid: 4101, done: make(chan error)}
	f.backend = &manualRuntimeFixtureProcess{driver: "headless", pid: 4102, done: make(chan error), entered: make(chan struct{}, 1), release: make(chan struct{})}
	var err error
	if f.reader, err = captureruntime.New(rp); err != nil {
		cancel()
		return nil, err
	}
	if f.collector, err = captureruntime.New(f.backend); err != nil {
		f.reader.Terminate()
		cancel()
		return nil, err
	}
	for _, role := range []string{"reader", "collector"} {
		manager := f.reader
		if role == "collector" {
			manager = f.collector
		}
		f.actual[role] = nativePilotOwner{role: role, owner: "synthetic-owner-" + role, profile: "synthetic-profile-" + role, directory: "synthetic-directory-" + role, image: "synthetic-image", creation: "synthetic-creation-" + role, job: "synthetic-job-" + role, parent: "synthetic-parent", context: "synthetic-context-" + role, target: "synthetic-target-" + role, frame: "synthetic-frame-" + role, document: "synthetic-document-" + role, pid: manager.PID(), generation: manager.Snapshot().Generation}
	}
	f.backend.owner, f.backend.resultSource = f.actual["collector"], domain.SourceX
	binding := nativePilotBinding{account: "1908123456789012345", enrollment: "synthetic-enrollment", pair: "synthetic-pair", attempt: "synthetic-attempt", derivation: "synthetic-derivation", policy: 1, consent: 1}
	for _, role := range []string{"reader", "collector"} {
		o := nativePilotOwner{role: role, profile: f.actual[role].profile, parent: "synthetic-parent"}
		if role == "reader" {
			binding.reader = o
		} else {
			binding.collector = o
		}
	}
	// The held bootstrap reader window counts before any post request arrives.
	f.state = nativePilotBrokerState{account: binding.account, enrollment: binding.enrollment, pair: binding.pair, attempt: binding.attempt, derivation: binding.derivation, policy: binding.policy, consent: binding.consent, readerWindows: 1, completeInventory: true, parentAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}
	ports := nativePilotBrokerPorts{
		now: func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now },
		prepare: func(ctx context.Context, request nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if ctx.Err() != nil || f.prepareFailure {
				return nativePilotPreparedRole{}, errors.New("synthetic prepare failure")
			}
			o := f.actual[request.role]
			if request.profile != o.profile || request.parent != o.parent || request.attempt != binding.attempt {
				return nativePilotPreparedRole{}, errors.New("synthetic selection mismatch")
			}
			p := nativePilotPreparedRole{owner: o, handle: "synthetic-held-" + request.role}
			f.prepared[request.role] = p
			return p, nil
		},
		inspect: func(ctx context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if ctx.Err() != nil {
				return nativePilotOwner{}, ctx.Err()
			}
			return f.actual[p.owner.role], nil
		},
		state: func(ctx context.Context) (nativePilotBrokerState, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.state, ctx.Err()
		},
		abort: func(_ context.Context, p nativePilotPreparedRole, preserve bool) error {
			if p.owner.role == "collector" {
				f.collector.Terminate()
			} else if !preserve {
				f.reader.Terminate()
			}
			return nil
		},
	}
	ports.acquire = func(ctx context.Context, p nativePilotPreparedRole, ch nativePilotCaptureChallenge) ([]byte, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if ctx.Err() != nil || len(f.prepared) != 2 || f.prepared[p.owner.role] != p || f.actual[p.owner.role] != p.owner || f.acquired[p.owner.role] != 0 {
			return nil, errors.New("synthetic startup gate mismatch")
		}
		f.acquired[p.owner.role]++
		o, at := p.owner, float64(ch.challenge.started.UnixMilli())
		return json.Marshal(map[string]any{"schema": nativePilotStartupWireSchema, "challenge": ch.correlation, "role": o.role, "accountId": binding.account, "ownerId": o.owner, "profileId": o.profile, "contextId": o.context, "targetId": o.target, "frameId": o.frame, "documentId": o.document, "identityRequestId": "synthetic-identity-" + o.role, "settingsRequestId": "synthetic-settings-" + o.role, "identityObservedAtMS": at, "settingsObservedAtMS": at, "elapsedMS": 1.0, "qualification": nativePilotSettingsQualification})
	}
	f.broker, _ = newNativePilotBroker(ctx, nativePilotBrokerConfig{enabled: true, offlineTest: true, pilot: nativePilotConfig{contract: nativePilotContract, enabled: true, limitationAcknowledged: true, binding: binding, attemptStarted: f.now}}, ports)
	f.runtime, err = newNativePilotRuntime(f.broker, f.reader, f.collector, f.backend, func(ctx context.Context, _ readerbroker.Request) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		f.opens++
		f.state.readerWindows++
		return nil
	})
	if err != nil {
		f.close()
		return nil, err
	}
	return f, nil
}

func (f *manualRuntimeFixture) close() {
	f.cancel()
	if f.broker != nil {
		f.broker.invalidate("parent-exited")
	}
	f.reader.Terminate()
	f.collector.Terminate()
}
func (f *manualRuntimeFixture) route(ctx context.Context, request readerbroker.Request) error {
	var in, out bytes.Buffer
	if err := readerbroker.Write(&in, request); err != nil {
		return err
	}
	return readerbroker.RouteConcurrentFixture(ctx, &in, &out, f.runtime.openReader)
}
func manualRuntimeRequest() readerbroker.Request {
	return readerbroker.Request{RequestID: "broker_0123456789abcdef0123456789abcdef", Source: "x", URL: "https://x.com/synthetic/status/1"}
}

func (f *manualRuntimeFixture) run(ctx context.Context, scenario string) error {
	request := manualRuntimeRequest()
	if f.route(ctx, request) == nil {
		return errors.New("manual admission opened before startup")
	}
	d := f.broker.start(ctx)
	if scenario == "prepare-failure" {
		if nativePilotRuntimeAllowed(d) || d.Reason != "prepare-failed" || f.route(ctx, request) == nil {
			return errors.New("prepare failure admitted")
		}
		return nil
	}
	if !d.OfflineValidated || !nativePilotRuntimeAllowed(d) {
		return errors.New("manual startup validation failed")
	}
	if f.broker.start(ctx).Reason != "startup-already-attempted" {
		return errors.New("manual startup repeated")
	}
	if f.acquired["reader"] != 1 || f.acquired["collector"] != 1 || f.state.readerWindows != 1 {
		return errors.New("manual startup inventory mismatch")
	}
	if scenario == "source-rejection" {
		bad := request
		bad.Source, bad.URL = "facebook", "https://facebook.com/synthetic"
		if f.route(ctx, bad) == nil {
			return errors.New("non-X reader admitted")
		}
		if _, err := f.runtime.capture(ctx, "source-rejection", domain.SourceFacebook); err == nil {
			return errors.New("non-X capture admitted")
		}
		if _, _, err := f.runtime.coordinator.BorrowBrowser(ctx); err == nil {
			return errors.New("exclusive browser admitted")
		}
		if f.runtime.coordinator.ValidateSelection("browser", []domain.Source{domain.SourceX}) == nil {
			return errors.New("exclusive strategy admitted")
		}
		return nil
	}
	if scenario == "broker-rejection" {
		bad := request
		bad.RequestID = "invalid"
		if f.route(ctx, bad) == nil || f.runtime.openReader(ctx, bad) == nil {
			return errors.New("malformed broker request admitted")
		}
		if err := f.route(ctx, request); err != nil {
			return err
		}
		if f.route(ctx, request) == nil || f.opens != 1 {
			return errors.New("duplicate broker request admitted")
		}
		return nil
	}
	type result struct {
		ticket *nativePilotManualTicket
		err    error
	}
	done := make(chan result, 1)
	go func() { t, err := f.runtime.capture(ctx, "synthetic-work", domain.SourceX); done <- result{t, err} }()
	select {
	case <-f.backend.entered:
	case <-ctx.Done():
		return ctx.Err()
	}
	before, pid := f.collector.Snapshot(), f.collector.PID()
	if before.ActiveLeases != 1 || before.Driver != "headless" {
		return errors.New("collector lease missing")
	}
	if err := f.route(ctx, request); err != nil {
		return err
	}
	if f.collector.Snapshot() != before || f.collector.PID() != pid || f.state.readerWindows != 2 || f.opens != 1 {
		return errors.New("reader disturbed collector")
	}
	if f.route(ctx, request) == nil {
		return errors.New("duplicate reader accepted")
	}
	switch scenario {
	case "overlap", "duplicate-result":
		// Advance past startup freshness: V2 has a fixed 60-second attempt,
		// never the old five-second renewal loop.
		f.mu.Lock()
		f.now = f.now.Add(6 * time.Second)
		f.mu.Unlock()
	case "expiry":
		f.mu.Lock()
		f.now = f.now.Add(nativePilotLife)
		f.mu.Unlock()
	case "opt-out":
		f.mu.Lock()
		f.state.optedIn = false
		f.mu.Unlock()
	case "parent-cancel":
		f.cancel()
	case "owner-drift":
		f.mu.Lock()
		o := f.actual["collector"]
		o.document = "synthetic-drift"
		f.actual["collector"] = o
		f.mu.Unlock()
	case "last-window":
		f.mu.Lock()
		f.state.readerWindows = 1
		f.mu.Unlock()
		if !nativePilotRuntimeAllowed(f.broker.check(ctx)) {
			return errors.New("remaining bootstrap reader lost")
		}
		f.mu.Lock()
		f.state.readerWindows = 0
		f.mu.Unlock()
	default:
		return errors.New("manual fixture scenario unavailable")
	}
	valid := scenario == "overlap" || scenario == "duplicate-result"
	if nativePilotRuntimeAllowed(f.broker.check(ctx)) != valid {
		return errors.New("manual lifecycle gate mismatch")
	}
	close(f.backend.release)
	var out result
	select {
	case out = <-done:
	case <-ctx.Done():
		return ctx.Err()
	}
	if (out.err == nil) != valid {
		return errors.New("manual late result fence failed")
	}
	if f.collector.Snapshot().ActiveLeases != 0 {
		return errors.New("manual collector lease leaked")
	}
	if valid {
		if len(f.runtime.results) != 1 || f.collector.PID() != pid || f.collector.Snapshot().Generation != before.Generation {
			return errors.New("manual completion changed collector")
		}
		if f.runtime.complete(ctx, out.ticket, domain.Observation{Source: domain.SourceX}) == nil {
			return errors.New("manual duplicate result accepted")
		}
		stale := *out.ticket
		stale.version++
		if f.runtime.complete(ctx, &stale, domain.Observation{Source: domain.SourceX}) == nil {
			return errors.New("manual stale result accepted")
		}
	} else {
		if len(f.runtime.results) != 0 || f.route(ctx, request) == nil || f.runtime.complete(ctx, out.ticket, domain.Observation{Source: domain.SourceX}) == nil {
			return errors.New("manual revoked result or reader admitted")
		}
		if scenario != "parent-cancel" && f.reader.Snapshot().State != captureruntime.Ready {
			return errors.New("manual revocation retired reader")
		}
	}
	return nil
}

// RunDevelopmentXManualFixture accepts only a bounded selector, never evidence,
// trust flags, credentials, profiles or Store permits. All receipts stay private.
func RunDevelopmentXManualFixture(input io.Reader, output io.Writer) error {
	if input == nil || output == nil {
		return errors.New("manual fixture IO unavailable")
	}
	raw, err := io.ReadAll(io.LimitReader(input, 1025))
	if err != nil || len(raw) == 0 || len(raw) > 1024 || !nativePilotStartupUnicode(raw) {
		return errors.New("manual fixture input rejected")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return errors.New("manual fixture input rejected")
	}
	fields := map[string]string{}
	for d.More() {
		key, err := d.Token()
		name, ok := key.(string)
		if err != nil || !ok || (name != "grant" && name != "scenario") {
			return errors.New("manual fixture input rejected")
		}
		if _, exists := fields[name]; exists {
			return errors.New("manual fixture input rejected")
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || len(value) == 0 || value[0] != '"' {
			return errors.New("manual fixture input rejected")
		}
		var text string
		if json.Unmarshal(value, &text) != nil {
			return errors.New("manual fixture input rejected")
		}
		fields[name] = text
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || len(fields) != 2 || fields["grant"] != "synthetic-x-manual-owned" {
		return errors.New("manual fixture input rejected")
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("manual fixture input rejected")
	}
	scenario, found := fields["scenario"], false
	for _, candidate := range manualRuntimeScenarios {
		if scenario == candidate {
			found = true
		}
	}
	if !found {
		return errors.New("manual fixture scenario rejected")
	}
	f, err := newManualRuntimeFixture(scenario == "prepare-failure")
	if err != nil {
		return err
	}
	defer f.close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err = f.run(ctx, scenario); err != nil {
		return err
	}
	return json.NewEncoder(output).Encode(struct {
		Provider         string `json:"provider"`
		Scenario         string `json:"scenario"`
		Passed           bool   `json:"passed"`
		OfflineValidated bool   `json:"offlineValidated"`
		CanAuthorize     bool   `json:"canAuthorize"`
		NativeQualified  bool   `json:"nativeQualified"`
		StorePermit      bool   `json:"storePermit"`
	}{"synthetic-x-manual-fixture", scenario, true, true, false, false, false})
}
