//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// This runner cannot select a native provider. All owners, profiles, proofs,
// permits, work and results are invented in memory. The unexported grant is
// minted after strict fixture opt-in and is never serialized or returned.
type concurrentFixtureGrant struct{ owned bool }

type concurrentFixtureInput struct {
	Grant    string `json:"grant"`
	Scenario string `json:"scenario"`
}

// RunDevelopmentConcurrentFixture is the early executable fixture entrypoint.
// It exposes fixed acceptance results, never private owner/proof values.
func RunDevelopmentConcurrentFixture(input io.Reader, output io.Writer) error {
	raw, err := io.ReadAll(io.LimitReader(input, 1025))
	if err != nil || len(raw) > 1024 {
		return errors.New("synthetic concurrent input rejected")
	}
	var request concurrentFixtureInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF || request.Grant != "synthetic-owned-fixture" {
		return errors.New("synthetic concurrent grant rejected")
	}
	allowed := map[string]bool{"overlap": true, "last-close": true, "last-close-denied": true, "stale": true, "cancel": true, "revoked": true, "parent-death": true, "opt-out": true, "strategy-gates": true, "broker-gates": true, "collector-loss": true}
	for _, name := range pilotOfflineScenarios {
		allowed[name] = true
	}
	for _, name := range authorizedOfflineScenarios {
		allowed[name] = true
	}
	if !allowed[request.Scenario] {
		return errors.New("synthetic concurrent scenario rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	fixture, err := newConcurrentIntegratedFixture(ctx, concurrentFixtureGrant{owned: true}, request.Scenario != "last-close-denied", request.Scenario)
	if err != nil {
		return err
	}
	runErr := fixture.run(ctx, request.Scenario)
	cleanupErr := fixture.close()
	if err := errors.Join(runErr, cleanupErr); err != nil {
		return fmt.Errorf("synthetic concurrent acceptance failed: %w", err)
	}
	if fixture.pilot {
		return json.NewEncoder(output).Encode(struct {
			Provider        string `json:"provider"`
			Scenario        string `json:"scenario"`
			Passed          bool   `json:"passed"`
			PilotSelected   bool   `json:"pilotSelected"`
			CanAuthorize    bool   `json:"canAuthorize"`
			NativeQualified bool   `json:"nativeQualified"`
			StorePermit     bool   `json:"storePermit"`
		}{"synthetic-concurrent-fixture", request.Scenario, true, true, false, false, false})
	}
	if fixture.authorized {
		return json.NewEncoder(output).Encode(struct {
			Provider              string `json:"provider"`
			Scenario              string `json:"scenario"`
			Passed                bool   `json:"passed"`
			AuthoritySelected     bool   `json:"authoritySelected"`
			AutomaticRenewals     uint64 `json:"automaticRenewals"`
			OriginalTicketVersion uint64 `json:"originalTicketVersion"`
			FinalEvidenceVersion  uint64 `json:"finalEvidenceVersion"`
			CanAuthorize          bool   `json:"canAuthorize"`
			NativeQualified       bool   `json:"nativeQualified"`
		}{"synthetic-concurrent-fixture", request.Scenario, true, true, fixture.renewal.count.Load(), fixture.originalVersion, fixture.finalVersion, false, false})
	}
	return json.NewEncoder(output).Encode(struct {
		Provider string `json:"provider"`
		Scenario string `json:"scenario"`
		Passed   bool   `json:"passed"`
	}{"synthetic-concurrent-fixture", request.Scenario, true})
}

type concurrentFixtureProcess struct {
	pid    int
	driver string
	done   chan error
	once   sync.Once
}

func (p *concurrentFixtureProcess) Done() <-chan error                  { return p.done }
func (p *concurrentFixtureProcess) PID() int                            { return p.pid }
func (p *concurrentFixtureProcess) Driver() string                      { return p.driver }
func (p *concurrentFixtureProcess) Terminate()                          { p.once.Do(func() { p.done <- nil; close(p.done) }) }
func (p *concurrentFixtureProcess) CloseForRetry(context.Context) error { p.Terminate(); return nil }
func (p *concurrentFixtureProcess) OpenExtensionsPage(context.Context) error {
	return errConcurrentUnavailable
}
func (p *concurrentFixtureProcess) ReplacementReadiness(context.Context) error { return nil }

type concurrentFixtureOwners struct {
	mu        sync.Mutex
	owners    map[concurrentRole]concurrentOwner
	managers  map[concurrentRole]*captureruntime.Manager
	processes map[concurrentRole]*concurrentFixtureProcess
	windows   map[string]bool
	creates   int
}

func (p *concurrentFixtureOwners) CreateOwner(_ context.Context, spec concurrentOwnerSpec) (concurrentOwner, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.creates++
	driver := "browser"
	if spec.Role == concurrentCollector {
		driver = "headless"
	}
	process := &concurrentFixtureProcess{pid: 8100 + p.creates, driver: driver, done: make(chan error, 1)}
	manager, err := captureruntime.New(process)
	if err != nil {
		return concurrentOwner{}, err
	}
	owner := concurrentOwner{ID: fmt.Sprintf("synthetic:owner:%d", p.creates), Role: spec.Role, ProfileID: spec.ProfileID, DirectoryID: spec.DirectoryID, PID: process.pid, CreationIdentity: fmt.Sprintf("synthetic:creation:%d", p.creates), ParentID: spec.ParentID, Generation: manager.Snapshot().Generation, Driver: spec.Driver}
	p.owners[spec.Role], p.managers[spec.Role] = owner, manager
	p.processes[spec.Role] = process
	return owner, nil
}

func (p *concurrentFixtureOwners) CurrentOwner(_ context.Context, role concurrentRole) (concurrentOwner, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	owner, exists := p.owners[role]
	if !exists {
		return owner, false, nil
	}
	manager := p.managers[role]
	state := manager.Snapshot()
	if state.State != captureruntime.Ready || manager.PID() != owner.PID || state.Generation != owner.Generation {
		return owner, false, nil
	}
	return owner, true, nil
}
func (p *concurrentFixtureOwners) OpenReaderWindow(_ context.Context, owner concurrentOwner, id, _ string, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current, ok := p.owners[concurrentReader]; !ok || !sameOwner(current, owner) {
		return errConcurrentStale
	}
	p.windows[id] = true
	return nil
}
func (p *concurrentFixtureOwners) CloseReaderWindow(_ context.Context, _ concurrentOwner, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.windows, id)
	return nil
}
func (p *concurrentFixtureOwners) ReaderInventoryEmpty(context.Context, concurrentOwner) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.windows) == 0, nil
}
func (p *concurrentFixtureOwners) CloseOwner(_ context.Context, owner concurrentOwner) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current, ok := p.owners[owner.Role]; !ok || !sameOwner(current, owner) {
		return errConcurrentStale
	}
	p.managers[owner.Role].Terminate()
	delete(p.owners, owner.Role)
	delete(p.processes, owner.Role)
	if owner.Role == concurrentReader {
		clear(p.windows)
	}
	return nil
}
func (p *concurrentFixtureOwners) OwnerAbsent(_ context.Context, role concurrentRole, id string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	owner, exists := p.owners[role]
	return !exists || owner.ID != id, nil
}

type concurrentFixtureProofs struct {
	now   func() time.Time
	calls int
}

func (p *concurrentFixtureProofs) Prove(_ context.Context, r concurrentProofRequest) (concurrentProof, error) {
	p.calls++
	return concurrentProof{Provider: r.Provider, ControllerID: r.ControllerID, AttemptID: r.AttemptID, Epoch: r.Epoch, PairGeneration: r.PairGeneration, Role: r.Role, Challenge: r.Challenge, Owner: r.Owner, AuthState: concurrentAuthenticated, AccountID: "synthetic:account:fixture", Revision: "synthetic:revision:fixture", ObservedAt: p.now()}, nil
}

type concurrentFixturePermits struct {
	grants  int
	revokes int
}

func (p *concurrentFixturePermits) Grant(_ context.Context, _ concurrentPermitRequest) (concurrentPermit, error) {
	p.grants++
	return concurrentPermit{ID: fmt.Sprintf("synthetic:permit:%d", p.grants), Granted: true}, nil
}
func (p *concurrentFixturePermits) Revoke(context.Context, string, uint64) error {
	p.revokes++
	return nil
}

type concurrentFixtureCapture struct {
	entered chan struct{}
	release chan struct{}
}

func (p *concurrentFixtureCapture) Capture(ctx context.Context, source domain.Source, _ map[string]any) (domain.Observation, error) {
	if source != domain.SourceX {
		return domain.Observation{}, errConcurrentUnavailable
	}
	select {
	case p.entered <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return domain.Observation{}, ctx.Err()
	case <-p.release:
		return domain.Observation{}, nil
	}
}

type concurrentIntegratedFixture struct {
	c                             *concurrentController
	owners                        *concurrentFixtureOwners
	proofs                        *concurrentFixtureProofs
	permits                       *concurrentFixturePermits
	coordinator                   *collection.Coordinator
	backend                       *concurrentFixtureCapture
	collector, reader             *captureruntime.Manager
	cancel                        context.CancelFunc
	done                          <-chan struct{}
	now                           time.Time
	gateWork                      string
	launches                      int
	ctx                           context.Context
	clockMu                       sync.RWMutex
	captureDone                   <-chan struct{}
	collectorExited               chan struct{}
	pilot                         bool
	pilotScenario                 string
	authorized                    bool
	lifecycle                     *controlledSyntheticLifecycle
	renewal                       *fixtureRenewalLoop
	originalVersion, finalVersion uint64
}

func newConcurrentIntegratedFixture(parent context.Context, grant concurrentFixtureGrant, independent bool, scenarios ...string) (*concurrentIntegratedFixture, error) {
	if !grant.owned {
		return nil, errConcurrentUnavailable
	}
	ctx, cancel := context.WithCancel(parent)
	f := &concurrentIntegratedFixture{ctx: ctx, cancel: cancel, now: time.Now().UTC(), collectorExited: make(chan struct{})}
	f.owners = &concurrentFixtureOwners{owners: make(map[concurrentRole]concurrentOwner), managers: make(map[concurrentRole]*captureruntime.Manager), processes: make(map[concurrentRole]*concurrentFixtureProcess), windows: make(map[string]bool)}
	f.proofs = &concurrentFixtureProofs{now: f.fixtureNow}
	f.permits = &concurrentFixturePermits{}
	f.authorized = len(scenarios) == 1 && strings.HasPrefix(scenarios[0], "authorized-")
	if len(scenarios) == 1 {
		for _, name := range pilotOfflineScenarios {
			if scenarios[0] == name {
				f.pilot = true
				f.pilotScenario = name
			}
		}
	}
	var err error
	cfg := concurrentConfig{Enabled: true, DevBuild: true, FixtureBuild: true, OwnerGrant: grant.owned, ControllerGrant: grant.owned, Provider: concurrentProvider, AttemptID: "synthetic:attempt:fixture", ParentID: "synthetic:parent:fixture", ExpectedAccountID: "synthetic:account:fixture", ExpectedRevision: "synthetic:revision:fixture", ReaderProfileID: "synthetic:profile:reader", ReaderDirectoryID: "synthetic:directory:reader", CollectorProfileID: "synthetic:profile:collector", CollectorDirectoryID: "synthetic:directory:collector", IndependentCollector: independent, now: f.proofs.now}
	if f.pilot {
		cfg.ExpectedRevision = ""
		cfg.IndependentCollector = false
		f.c, err = newPilotConcurrentController(ctx, cfg, concurrentPorts{owners: f.owners}, f.pilotFixture())
	} else if f.authorized {
		cfg.now, cfg.ExpectedRevision, cfg.IndependentCollector = time.Now, "", false
		f.lifecycle = newControlledSyntheticLifecycle("fixture")
		f.c, err = newAuthorizedConcurrentController(ctx, cfg, concurrentPorts{owners: f.owners}, controlledFixtureAuthorization("fixture", f.lifecycle, f.collectAuthorized))
	} else {
		f.c, err = newConcurrentController(ctx, cfg, concurrentPorts{owners: f.owners, verifier: f.proofs, permit: f.permits})
	}
	if err != nil {
		cancel()
		return nil, err
	}
	if f.c.Snapshot().AdmissionOpen {
		cancel()
		return nil, errors.New("startup admission open")
	}
	if err = f.c.Start(ctx); err != nil {
		cancel()
		return nil, err
	}
	f.collector, f.reader = f.owners.managers[concurrentCollector], f.owners.managers[concurrentReader]
	f.collector.SetExitObserver(func(uint64) {
		// The fake Done event proves this invented tree is gone. Retire only its
		// owned registry entry; this does not model native root-exit cleanup.
		f.owners.mu.Lock()
		f.collector.Terminate()
		delete(f.owners.owners, concurrentCollector)
		delete(f.owners.processes, concurrentCollector)
		f.owners.mu.Unlock()
		f.c.CollectorLost()
		close(f.collectorExited)
	})
	f.reader.SetExitObserver(func(uint64) { f.c.Revoke("reader-owner-exit") })
	f.gateWork, err = f.c.QueueWork("synthetic:admission-check")
	if err != nil {
		f.close()
		return nil, err
	}
	f.backend = &concurrentFixtureCapture{entered: make(chan struct{}, 1), release: make(chan struct{})}
	f.coordinator = collection.NewCoordinator(f.collector, func(context.Context, string, uint64) (captureruntime.Process, error) {
		f.launches++
		return nil, errConcurrentUnavailable
	}, func() error { return nil })
	gate := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := f.c.Ticket(f.gateWork)
		return err
	}
	if err = f.coordinator.BindConcurrentSession(f.reader, f.collector.Snapshot().Generation, f.backend, gate, gate); err != nil {
		f.close()
		return nil, err
	}
	f.done = f.coordinator.StartWithDone(ctx)
	f.coordinator.Request("headless")
	return f, nil
}

func (f *concurrentIntegratedFixture) close() error {
	f.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := f.renewal.stop(ctx); err != nil {
		return err
	}
	if f.done != nil {
		select {
		case <-f.done:
		case <-ctx.Done():
			return errors.New("coordinator did not drain")
		}
	}
	if f.captureDone != nil {
		select {
		case <-f.captureDone:
		case <-ctx.Done():
			return errors.New("capture did not drain")
		}
	}
	if err := f.c.WaitParentCleanup(ctx); err != nil {
		return err
	}
	f.owners.mu.Lock()
	defer f.owners.mu.Unlock()
	if len(f.owners.owners) != 0 || len(f.owners.windows) != 0 || len(f.owners.processes) != 0 {
		return errors.New("owned fixture references remain")
	}
	for _, manager := range f.owners.managers {
		if state := manager.Snapshot(); state.State != captureruntime.Stopped || state.ActiveLeases != 0 {
			return errors.New("owned manager or lease remains")
		}
	}
	return nil
}

func (f *concurrentIntegratedFixture) open(ctx context.Context, sequence int, bootstrap bool) error {
	request := readerbroker.Request{RequestID: fmt.Sprintf("broker_%032x", sequence), Source: "x", URL: "https://x.com/synthetic/status/123"}
	var input, output bytes.Buffer
	if err := readerbroker.Write(&input, request); err != nil {
		return err
	}
	err := readerbroker.RouteConcurrentFixture(ctx, &input, &output, func(ctx context.Context, _ readerbroker.Request) error {
		lease, release, err := f.coordinator.BorrowNativeReader(ctx)
		if err != nil {
			return err
		}
		defer release()
		defer lease.Release()
		if lease.Driver() != "browser" || lease.Generation() != f.reader.Snapshot().Generation || f.reader.PID() == f.collector.PID() {
			return errors.New("reader borrowed collector")
		}
		return f.c.OpenReaderWindow(ctx, fmt.Sprintf("synthetic:window:%d", sequence), fmt.Sprintf("synthetic:target:%d", sequence), bootstrap)
	})
	if err != nil {
		return err
	}
	var reply struct {
		Accepted bool `json:"syntheticAccepted"`
	}
	if err := readerbroker.Read(&output, &reply); err != nil || !reply.Accepted {
		return errors.New("synthetic broker reply rejected")
	}
	return nil
}

func (f *concurrentIntegratedFixture) captureStart(ctx context.Context) (concurrentTicket, *captureruntime.Lease, <-chan error, error) {
	id, err := f.c.QueueWork("synthetic:work:fixture")
	if err != nil {
		return concurrentTicket{}, nil, nil, err
	}
	ticket, err := f.c.Ticket(id)
	if err != nil {
		return ticket, nil, nil, err
	}
	if err = f.c.Claim(ctx, ticket); err != nil {
		return ticket, nil, nil, err
	}
	lease, err := f.collector.Acquire()
	if err != nil {
		return ticket, nil, nil, err
	}
	if lease.Driver() != "headless" || lease.Generation() != ticket.CollectorGeneration {
		lease.Release()
		return ticket, nil, nil, errors.New("capture owner stamp rejected")
	}
	done := make(chan error, 1)
	drained := make(chan struct{})
	f.captureDone = drained
	captureCtx, cancelCapture := context.WithCancel(f.ctx)
	stopCaller := context.AfterFunc(ctx, cancelCapture)
	go func() {
		defer close(drained)
		defer cancelCapture()
		defer stopCaller()
		_, err := f.coordinator.Capture(captureCtx, domain.SourceX, map[string]any{"synthetic": true})
		done <- err
	}()
	select {
	case <-f.backend.entered:
		return ticket, lease, done, nil
	case err := <-done:
		lease.Release()
		return ticket, nil, nil, fmt.Errorf("capture did not start: %v", err)
	case <-ctx.Done():
		lease.Release()
		return ticket, nil, nil, ctx.Err()
	}
}

func (f *concurrentIntegratedFixture) run(ctx context.Context, scenario string) error {
	if f.pilot {
		return f.runPilot(ctx, scenario)
	}
	if f.authorized {
		return f.runAuthorized(ctx, scenario)
	}
	if scenario == "strategy-gates" {
		return f.strategyGates(ctx)
	}
	if scenario == "broker-gates" {
		return f.brokerGates(ctx)
	}
	if err := f.open(ctx, 1, true); err != nil {
		return err
	}
	if scenario == "last-close" || scenario == "last-close-denied" {
		before, pid := f.collector.Snapshot(), f.collector.PID()
		proofs, grants := f.proofs.calls, f.permits.grants
		err := f.c.CloseReaderWindow(ctx, "synthetic:window:1")
		if scenario == "last-close" {
			if err != nil || !f.c.Snapshot().AdmissionOpen || f.proofs.calls != proofs+1 || f.permits.grants != grants+1 {
				return errors.New("fresh independent continuation missing")
			}
		} else if err == nil || f.c.Snapshot().AdmissionOpen || f.proofs.calls != proofs || f.permits.grants != grants {
			return errors.New("dependent continuation admitted")
		}
		if f.collector.Snapshot() != before || f.collector.PID() != pid || f.reader.Snapshot().State != captureruntime.Stopped || f.open(ctx, 2, false) == nil {
			return errors.New("last-close owner boundary failed")
		}
		if scenario == "last-close" {
			ticket, lease, done, err := f.captureStart(ctx)
			if err != nil {
				return err
			}
			defer lease.Release()
			close(f.backend.release)
			if err := <-done; err != nil {
				return err
			}
			lease.Release()
			if err := f.c.Complete(ctx, ticket, "synthetic:independent-result"); err != nil {
				return err
			}
			if _, ok := f.c.SinkResult(ticket.WorkID); !ok {
				return errors.New("independent result missing")
			}
		}
		return nil
	}
	ticket, lease, done, err := f.captureStart(ctx)
	if err != nil {
		return err
	}
	defer lease.Release()
	before, pid := f.collector.Snapshot(), f.collector.PID()
	if err := f.open(ctx, 2, false); err != nil {
		return err
	}
	if f.collector.Snapshot() != before || f.collector.PID() != pid || before.ActiveLeases != 1 || f.launches != 0 || f.coordinator.Status().BatchPaused || f.c.Snapshot().ReaderWindows != 2 {
		return errors.New("reader disturbed active collector")
	}
	if err := f.c.CloseReaderWindow(ctx, "synthetic:window:2"); err != nil || f.c.Snapshot().ReaderWindows != 1 {
		return errors.New("multiwindow accounting failed")
	}
	switch scenario {
	case "stale":
		f.advanceFixtureClock(concurrentProofMaxAge + time.Second)
	case "cancel":
		if f.c.Cancel(ticket) != nil {
			return errors.New("work cancellation failed")
		}
	case "revoked":
		f.c.Revoke("synthetic-revocation")
	case "parent-death":
		f.cancel()
		if err := f.c.WaitParentCleanup(ctx); err != nil {
			return err
		}
		select {
		case <-f.captureDone:
		case <-ctx.Done():
			return errors.New("parent did not cancel capture")
		}
	case "opt-out":
		if err := f.c.Disable(ctx); err != nil || f.c.Snapshot().AdmissionOpen || f.reader.Snapshot().State != captureruntime.Ready || f.c.Snapshot().ReaderWindows != 1 || f.collector.Snapshot().State != captureruntime.Stopped {
			return errors.New("opt-out ownership failed")
		}
	case "collector-loss":
		f.owners.processes[concurrentCollector].Terminate()
		select {
		case <-f.collectorExited:
		case <-ctx.Done():
			return errors.New("manager exit observer did not fence collector")
		}
		if f.reader.Snapshot().State != captureruntime.Ready || f.c.Snapshot().AdmissionOpen {
			return errors.New("collector loss closed reader or admitted work")
		}
	}
	close(f.backend.release)
	captureErr := <-done
	if scenario == "parent-death" && !errors.Is(captureErr, context.Canceled) {
		return errors.New("parent cancellation did not reach backend")
	}
	lease.Release()
	completeErr := f.c.Complete(ctx, ticket, "synthetic:result:fixture")
	_, accepted := f.c.SinkResult(ticket.WorkID)
	if scenario == "overlap" {
		if captureErr != nil || completeErr != nil || !accepted || f.reader.Snapshot().State != captureruntime.Ready || f.c.Snapshot().ReaderWindows != 1 || f.collector.Snapshot().ActiveLeases != 0 || f.collector.PID() != pid || f.collector.Snapshot().Generation != before.Generation || f.launches != 0 {
			return errors.New("capture completion disturbed reader or collector")
		}
		if f.c.Complete(ctx, ticket, "synthetic:duplicate") == nil {
			return errors.New("duplicate accepted")
		}
	} else {
		if completeErr == nil || accepted {
			return errors.New("fenced result accepted")
		}
		if scenario != "cancel" && f.open(ctx, 3, false) == nil {
			return errors.New("fenced reader admitted")
		}
		if scenario == "parent-death" && (f.reader.Snapshot().State != captureruntime.Stopped || f.collector.Snapshot().State != captureruntime.Stopped) {
			return errors.New("parent left owned child")
		}
	}
	return nil
}

func (f *concurrentIntegratedFixture) strategyGates(ctx context.Context) error {
	if f.coordinator.ValidateSelection("browser", []domain.Source{domain.SourceX}) == nil || f.coordinator.ValidateSelection("headless", []domain.Source{domain.SourceX, domain.SourceFacebook}) == nil || f.coordinator.ValidateSelection("headless", []domain.Source{domain.SourceFacebook}) == nil {
		return errors.New("selection bypass")
	}
	f.coordinator.Request("browser")
	if f.coordinator.Status().Requested != "headless" {
		return errors.New("browser request bypass")
	}
	if _, _, err := f.coordinator.BorrowBrowser(ctx); err == nil {
		return errors.New("browser borrow bypass")
	}
	if _, _, err := f.coordinator.BorrowBrowserCollection(ctx, domain.SourceFacebook); err == nil {
		return errors.New("collection borrow bypass")
	}
	if _, err := f.coordinator.BeginBrowserCollection(domain.SourceFacebook); err == nil {
		return errors.New("collection intent bypass")
	}
	if _, err := f.coordinator.Capture(ctx, domain.SourceFacebook, nil); err == nil {
		return errors.New("capture source bypass")
	}
	hooks := collection.ExclusiveSessionHooks{BeforeReader: func(context.Context) error { return nil }, BeforeHeadless: func(context.Context) error { return nil }, HeadlessReady: func(context.Context, uint64) error { return nil }, Paused: func() bool { return false }}
	if f.coordinator.BindExclusiveSession(hooks) == nil || f.coordinator.SetDevelopmentSession(true, domain.SourceX) == nil {
		return errors.New("concurrent binding bypass")
	}
	if f.coordinator.SetExclusiveHeadlessCollector(f.collector.Snapshot().Generation+1, f.backend) == nil {
		return errors.New("exclusive backend mutation bypass")
	}
	if f.coordinator.SetSyntheticHeadlessCollector(f.collector.Snapshot().Generation+1, f.backend) == nil {
		return errors.New("synthetic backend alias mutation bypass")
	}
	for _, development := range []bool{false, true} {
		other := collection.NewCoordinator(f.collector, nil, func() error { return nil })
		if development {
			if err := other.SetDevelopmentSession(true, domain.SourceX); err != nil {
				return err
			}
		} else if err := other.BindExclusiveSession(hooks); err != nil {
			return err
		}
		if other.BindConcurrentSession(f.reader, f.collector.Snapshot().Generation, f.backend, func(context.Context) error { return nil }, func(context.Context) error { return nil }) == nil {
			return errors.New("reverse binding bypass")
		}
	}
	if f.coordinator.BindConcurrentSession(f.reader, f.collector.Snapshot().Generation, f.backend, func(context.Context) error { return nil }, func(context.Context) error { return nil }) == nil {
		return errors.New("duplicate binding bypass")
	}
	if f.launches != 0 || f.collector.Snapshot().Driver != "headless" {
		return errors.New("strategy replaced collector")
	}
	return nil
}

func (f *concurrentIntegratedFixture) brokerGates(ctx context.Context) error {
	callbacks := 0
	requests := []readerbroker.Request{{RequestID: "invalid", Source: "x", URL: "https://x.com/a"}, {RequestID: "broker_00000000000000000000000000000001", Source: "facebook", URL: "https://facebook.com/a"}, {RequestID: "broker_00000000000000000000000000000001", Source: "x", URL: "https://example.com/a"}}
	for _, request := range requests {
		var input, output bytes.Buffer
		if err := readerbroker.Write(&input, request); err != nil {
			return err
		}
		if readerbroker.RouteConcurrentFixture(ctx, &input, &output, func(context.Context, readerbroker.Request) error {
			callbacks++
			return errors.New("callback must not run")
		}) == nil || output.Len() != 0 {
			return errors.New("invalid broker request admitted")
		}
	}
	var canceledInput, canceledOutput bytes.Buffer
	if err := readerbroker.Write(&canceledInput, readerbroker.Request{RequestID: "broker_00000000000000000000000000000001", Source: "x", URL: "https://x.com/a"}); err != nil {
		return err
	}
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if readerbroker.RouteConcurrentFixture(canceledCtx, &canceledInput, &canceledOutput, func(context.Context, readerbroker.Request) error { callbacks++; return nil }) == nil || canceledOutput.Len() != 0 {
		return errors.New("canceled broker request admitted")
	}
	var output bytes.Buffer
	if readerbroker.RouteConcurrentFixture(ctx, bytes.NewReader([]byte{1, 32, 0, 0}), &output, nil) == nil {
		return errors.New("oversized broker frame admitted")
	}
	if callbacks != 0 || f.c.Snapshot().ReaderWindows != 0 {
		return errors.New("rejected broker created reader")
	}
	return nil
}
