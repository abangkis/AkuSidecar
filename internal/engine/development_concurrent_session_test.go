//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type concurrentTestClock struct {
	mu  sync.Mutex
	now time.Time
}

func newConcurrentTestClock() *concurrentTestClock {
	return &concurrentTestClock{now: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)}
}

func (c *concurrentTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *concurrentTestClock) Advance(by time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(by)
	c.mu.Unlock()
}

type fakeConcurrentOwnerPort struct {
	mu             sync.Mutex
	owners         map[concurrentRole]concurrentOwner
	windows        map[string]map[string]bool
	createCalls    int
	openCalls      int
	closeCalls     int
	closeOwnerCall int
	closeFailures  map[concurrentRole]bool
	ownerAbsent    bool
	createHook     func()
	transform      func(concurrentOwner) concurrentOwner
	blockCurrent   concurrentRole
	currentEntered chan struct{}
	currentRelease chan struct{}
}

func newFakeConcurrentOwners() *fakeConcurrentOwnerPort {
	return &fakeConcurrentOwnerPort{
		owners: make(map[concurrentRole]concurrentOwner), windows: make(map[string]map[string]bool),
		closeFailures: make(map[concurrentRole]bool),
	}
}

func (p *fakeConcurrentOwnerPort) CreateOwner(_ context.Context, spec concurrentOwnerSpec) (concurrentOwner, error) {
	p.mu.Lock()
	p.createCalls++
	sequence := p.createCalls
	owner := concurrentOwner{
		ID: fmt.Sprintf("synthetic:owner:%d", sequence), Role: spec.Role, ProfileID: spec.ProfileID,
		DirectoryID: spec.DirectoryID, PID: 7000 + sequence,
		CreationIdentity: fmt.Sprintf("synthetic:creation:%d", sequence), ParentID: spec.ParentID,
		Generation: uint64(sequence), Driver: spec.Driver,
	}
	if p.transform != nil {
		owner = p.transform(owner)
	}
	p.owners[spec.Role] = owner
	p.windows[owner.ID] = make(map[string]bool)
	hook := p.createHook
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	return owner, nil
}

func (p *fakeConcurrentOwnerPort) CurrentOwner(_ context.Context, role concurrentRole) (concurrentOwner, bool, error) {
	p.mu.Lock()
	block := p.blockCurrent == role && p.currentEntered != nil
	entered, release := p.currentEntered, p.currentRelease
	p.mu.Unlock()
	if block {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	owner, ok := p.owners[role]
	return owner, ok, nil
}

func (p *fakeConcurrentOwnerPort) OpenReaderWindow(_ context.Context, owner concurrentOwner, id, _ string, _ bool) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if current, ok := p.owners[concurrentReader]; !ok || !sameOwner(current, owner) {
		return errConcurrentStale
	}
	p.openCalls++
	p.windows[owner.ID][id] = true
	return nil
}

func (p *fakeConcurrentOwnerPort) CloseReaderWindow(_ context.Context, owner concurrentOwner, id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeCalls++
	delete(p.windows[owner.ID], id)
	return nil
}

func (p *fakeConcurrentOwnerPort) ReaderInventoryEmpty(_ context.Context, owner concurrentOwner) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.windows[owner.ID]) == 0, nil
}

func (p *fakeConcurrentOwnerPort) CloseOwner(_ context.Context, owner concurrentOwner) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeOwnerCall++
	if p.closeFailures[owner.Role] {
		return errors.New("synthetic close failure")
	}
	if current, ok := p.owners[owner.Role]; ok && sameOwner(current, owner) {
		delete(p.owners, owner.Role)
		delete(p.windows, owner.ID)
	}
	return nil
}

func (p *fakeConcurrentOwnerPort) OwnerAbsent(_ context.Context, role concurrentRole, id string) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ownerAbsent {
		return false, nil
	}
	owner, ok := p.owners[role]
	return !ok || owner.ID != id, nil
}

func (p *fakeConcurrentOwnerPort) mutate(role concurrentRole, update func(concurrentOwner) concurrentOwner) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.owners[role] = update(p.owners[role])
}

func (p *fakeConcurrentOwnerPort) counts() (creates, closes, opens int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.createCalls, p.closeOwnerCall, p.openCalls
}

func (p *fakeConcurrentOwnerPort) has(role concurrentRole) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.owners[role]
	return ok
}

type fakeConcurrentVerifier struct {
	mu                 sync.Mutex
	clock              *concurrentTestClock
	account            string
	revision           string
	state              map[concurrentRole]concurrentAuthState
	wrongChallenge     bool
	wrongAccount       bool
	wrongRevision      bool
	wrongOwner         bool
	staleBy            time.Duration
	replay             bool
	last               concurrentProof
	calls              []concurrentProofRequest
	advanceAfter       concurrentRole
	advanceBy          time.Duration
	blockRole          concurrentRole
	entered            chan struct{}
	release            chan struct{}
	seenCancellation   chan struct{}
	cancelWhileBlocked bool
}

func newFakeConcurrentVerifier(clock *concurrentTestClock) *fakeConcurrentVerifier {
	return &fakeConcurrentVerifier{
		clock: clock, account: "synthetic:account:expected", revision: "synthetic:revision:accepted",
		state: make(map[concurrentRole]concurrentAuthState),
	}
}

func (p *fakeConcurrentVerifier) Prove(ctx context.Context, request concurrentProofRequest) (concurrentProof, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return concurrentProof{}, errors.New("proof context has no binding deadline")
	}
	if time.Until(deadline) > concurrentBindingDeadline {
		return concurrentProof{}, errors.New("proof context deadline exceeds binding window")
	}
	p.mu.Lock()
	p.calls = append(p.calls, request)
	block := request.Role == p.blockRole && p.entered != nil
	entered, release := p.entered, p.release
	p.mu.Unlock()
	if block {
		select {
		case entered <- struct{}{}:
		default:
		}
		if p.cancelWhileBlocked {
			select {
			case <-ctx.Done():
				if p.seenCancellation != nil {
					select {
					case p.seenCancellation <- struct{}{}:
					default:
					}
				}
				return concurrentProof{}, ctx.Err()
			case <-release:
			}
		} else {
			<-release
		}
	}
	if ctx.Err() != nil && p.seenCancellation != nil {
		select {
		case p.seenCancellation <- struct{}{}:
		default:
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.replay && p.last.Challenge != "" {
		return p.last, nil
	}
	proof := concurrentProof{
		Provider: request.Provider, ControllerID: request.ControllerID, AttemptID: request.AttemptID, Epoch: request.Epoch,
		PairGeneration: request.PairGeneration, Role: request.Role, Challenge: request.Challenge,
		Owner: request.Owner, AuthState: p.state[request.Role], AccountID: p.account,
		Revision: p.revision, ObservedAt: request.RequestedAt.Add(-p.staleBy),
	}
	if proof.AuthState == "" {
		proof.AuthState = concurrentAuthenticated
	}
	if p.wrongChallenge {
		proof.Challenge = "synthetic:replayed-challenge"
	}
	if p.wrongAccount {
		proof.AccountID = "synthetic:account:other"
	}
	if p.wrongRevision {
		proof.Revision = "synthetic:revision:other"
	}
	if p.wrongOwner {
		proof.Owner.PID++
	}
	if p.advanceAfter == request.Role && p.advanceBy > 0 {
		p.clock.Advance(p.advanceBy)
	}
	p.last = proof
	return proof, nil
}

func (p *fakeConcurrentVerifier) requestCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

type fakeConcurrentRevocation struct {
	ID    string
	Epoch uint64
}

type fakeConcurrentPermit struct {
	mu            sync.Mutex
	grants        []concurrentPermitRequest
	revokes       []fakeConcurrentRevocation
	deny          bool
	revokeError   bool
	revokeEntered chan struct{}
	revokeRelease chan struct{}
	clock         *concurrentTestClock
	advance       time.Duration
	grantHook     func()
}

func (p *fakeConcurrentPermit) Grant(ctx context.Context, request concurrentPermitRequest) (concurrentPermit, error) {
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > concurrentBindingDeadline {
		return concurrentPermit{}, errors.New("permit context lacks binding deadline")
	}
	p.mu.Lock()
	p.grants = append(p.grants, request)
	permit := concurrentPermit{ID: fmt.Sprintf("synthetic:permit:%d", len(p.grants)), Granted: !p.deny}
	if p.advance > 0 {
		p.clock.Advance(p.advance)
	}
	hook := p.grantHook
	p.mu.Unlock()
	if hook != nil {
		hook()
	}
	if !permit.Granted {
		return concurrentPermit{}, nil
	}
	return permit, nil
}

func (p *fakeConcurrentPermit) Revoke(_ context.Context, id string, epoch uint64) error {
	p.mu.Lock()
	p.revokes = append(p.revokes, fakeConcurrentRevocation{ID: id, Epoch: epoch})
	shouldFail := p.revokeError
	entered, release := p.revokeEntered, p.revokeRelease
	p.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
	}
	if shouldFail {
		return errors.New("synthetic revoke failure")
	}
	return nil
}

func (p *fakeConcurrentPermit) grantCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.grants)
}

func (p *fakeConcurrentPermit) revocationSnapshot() []fakeConcurrentRevocation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]fakeConcurrentRevocation(nil), p.revokes...)
}

func (p *fakeConcurrentPermit) lastGrant() concurrentPermitRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grants[len(p.grants)-1]
}

func concurrentTestConfig(parent context.Context, clock *concurrentTestClock, independent bool) concurrentConfig {
	return concurrentConfig{
		Enabled: true, DevBuild: true, FixtureBuild: true, OwnerGrant: true, ControllerGrant: true,
		Provider: concurrentProvider, AttemptID: "synthetic:attempt:one", ParentID: "synthetic:parent:one",
		ExpectedAccountID: "synthetic:account:expected", ExpectedRevision: "synthetic:revision:accepted",
		ReaderProfileID: "synthetic:profile:reader", ReaderDirectoryID: "synthetic:directory:reader",
		CollectorProfileID: "synthetic:profile:collector", CollectorDirectoryID: "synthetic:directory:collector",
		IndependentCollector: independent, now: clock.Now,
	}
}

func newConcurrentTestController(t *testing.T, independent bool) (*concurrentController, context.CancelFunc, *concurrentTestClock, *fakeConcurrentOwnerPort, *fakeConcurrentVerifier, *fakeConcurrentPermit) {
	return newConcurrentTestControllerWithCleanupExpectation(t, independent, false)
}

func newConcurrentTestControllerExpectingCleanupFailure(t *testing.T, independent bool) (*concurrentController, context.CancelFunc, *concurrentTestClock, *fakeConcurrentOwnerPort, *fakeConcurrentVerifier, *fakeConcurrentPermit) {
	return newConcurrentTestControllerWithCleanupExpectation(t, independent, true)
}

func newConcurrentTestControllerWithCleanupExpectation(t *testing.T, independent, expectCleanupFailure bool) (*concurrentController, context.CancelFunc, *concurrentTestClock, *fakeConcurrentOwnerPort, *fakeConcurrentVerifier, *fakeConcurrentPermit) {
	t.Helper()
	parent, cancel := context.WithCancel(context.Background())
	clock := newConcurrentTestClock()
	owners := newFakeConcurrentOwners()
	verifier := newFakeConcurrentVerifier(clock)
	permit := &fakeConcurrentPermit{clock: clock}
	controller, err := newConcurrentController(parent, concurrentTestConfig(parent, clock, independent), concurrentPorts{owners: owners, verifier: verifier, permit: permit})
	if err != nil {
		t.Fatalf("newConcurrentController: %v", err)
	}
	cancelAndWait := func() {
		cancel()
		ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		err := controller.WaitParentCleanup(ctx)
		if expectCleanupFailure {
			if !errors.Is(err, errConcurrentCleanup) {
				t.Errorf("parent cleanup error = %v, want unverified cleanup", err)
			}
		} else if err != nil {
			t.Errorf("parent cleanup: %v", err)
		}
	}
	return controller, cancelAndWait, clock, owners, verifier, permit
}

func startConcurrentTest(t *testing.T, controller *concurrentController) {
	t.Helper()
	if err := controller.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestConcurrentControllerDefaultOffAndInvalidConfigArePortFree(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	owners := newFakeConcurrentOwners()
	clock := newConcurrentTestClock()
	verifier := newFakeConcurrentVerifier(clock)
	permit := &fakeConcurrentPermit{clock: clock}
	ports := concurrentPorts{owners: owners, verifier: verifier, permit: permit}
	controller, err := newConcurrentController(nil, concurrentConfig{}, ports)
	if err != nil {
		t.Fatalf("default-off constructor: %v", err)
	}
	if err := controller.Start(context.Background()); !errors.Is(err, errConcurrentDisabled) {
		t.Fatalf("default-off Start error = %v", err)
	}
	defaultOffCalls := []struct {
		name string
		call func() error
	}{
		{"claim", func() error { return controller.Claim(context.Background(), concurrentTicket{}) }},
		{"complete", func() error {
			return controller.Complete(context.Background(), concurrentTicket{}, "synthetic:result:none")
		}},
		{"cancel", func() error { return controller.Cancel(concurrentTicket{}) }},
		{"open reader", func() error {
			return controller.OpenReaderWindow(context.Background(), "synthetic:window:none", "synthetic:target:none", false)
		}},
		{"close reader", func() error { return controller.CloseReaderWindow(context.Background(), "synthetic:window:none") }},
		{"disable", func() error { return controller.Disable(context.Background()) }},
		{"wait cleanup", func() error { return controller.WaitParentCleanup(context.Background()) }},
	}
	for _, operation := range defaultOffCalls {
		if err := operation.call(); !errors.Is(err, errConcurrentDisabled) {
			t.Errorf("default-off %s error = %v", operation.name, err)
		}
	}
	cases := map[string]func(*concurrentConfig){
		"dev grant":        func(c *concurrentConfig) { c.DevBuild = false },
		"fixture grant":    func(c *concurrentConfig) { c.FixtureBuild = false },
		"owner grant":      func(c *concurrentConfig) { c.OwnerGrant = false },
		"controller grant": func(c *concurrentConfig) { c.ControllerGrant = false },
		"native provider":  func(c *concurrentConfig) { c.Provider = "native-x" },
		"profile alias":    func(c *concurrentConfig) { c.CollectorProfileID = c.ReaderProfileID },
		"directory alias":  func(c *concurrentConfig) { c.CollectorDirectoryID = c.ReaderDirectoryID },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := concurrentTestConfig(parent, clock, false)
			mutate(&cfg)
			if _, err := newConcurrentController(parent, cfg, ports); !errors.Is(err, errConcurrentUnavailable) {
				t.Fatalf("constructor error = %v", err)
			}
		})
	}
	creates, closes, opens := owners.counts()
	if creates != 0 || closes != 0 || opens != 0 || verifier.requestCount() != 0 || permit.grantCount() != 0 {
		t.Fatalf("invalid/default config called a port: creates=%d closes=%d opens=%d probes=%d grants=%d", creates, closes, opens, verifier.requestCount(), permit.grantCount())
	}
}

func TestConcurrentStartClosesAdmissionAndReaderWindowsDoNotTouchCollector(t *testing.T) {
	controller, cancel, _, owners, verifier, permit := newConcurrentTestController(t, true)
	defer cancel()
	owners.createHook = func() {
		if controller.Snapshot().AdmissionOpen {
			t.Error("admission opened before owners and proof were bound")
		}
	}
	startConcurrentTest(t, controller)
	if verifier.requestCount() != 2 || permit.grantCount() != 1 {
		t.Fatalf("calls probes=%d grants=%d", verifier.requestCount(), permit.grantCount())
	}
	grant := permit.lastGrant()
	if grant.Mode != "reader-and-collector" || len(grant.Owners) != 2 || grant.Owners[0].ID == grant.Owners[1].ID {
		t.Fatalf("permit not bound to distinct owner pair: %+v", grant)
	}
	proofMu := &verifier.mu
	proofMu.Lock()
	firstChallenge, secondChallenge := verifier.calls[0].Challenge, verifier.calls[1].Challenge
	firstRole, secondRole := verifier.calls[0].Role, verifier.calls[1].Role
	proofMu.Unlock()
	if firstChallenge == secondChallenge || firstRole == secondRole {
		t.Fatalf("owner proofs were not independent: %s/%s %s/%s", firstRole, firstChallenge, secondRole, secondChallenge)
	}

	workID, err := controller.QueueWork("synthetic:work:payload")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := controller.Ticket(workID)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Claim(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	before := controller.Snapshot()
	if err := controller.OpenReaderWindow(context.Background(), "synthetic:window:bootstrap", "synthetic:target:bootstrap", true); err != nil {
		t.Fatal(err)
	}
	if err := controller.OpenReaderWindow(context.Background(), "synthetic:window:post", "synthetic:target:post", false); err != nil {
		t.Fatal(err)
	}
	after := controller.Snapshot()
	if before.Collector != after.Collector || before.Active != after.Active || !after.AdmissionOpen || after.ReaderWindows != 2 {
		t.Fatalf("reader opens disturbed collection: before=%+v after=%+v", before, after)
	}
	if err := controller.Complete(context.Background(), ticket, "synthetic:result:done"); err != nil {
		t.Fatal(err)
	}
	if state, _ := controller.WorkState(workID); state != concurrentWorkComplete {
		t.Fatalf("work state = %v", state)
	}
	if result, ok := controller.SinkResult(workID); !ok || result != "synthetic:result:done" {
		t.Fatalf("sink = %q, %v", result, ok)
	}
	if snapshot := controller.Snapshot(); snapshot.ReaderWindows != 2 || !snapshot.HasReader || !snapshot.HasCollector || !snapshot.AdmissionOpen {
		t.Fatalf("completion did not retain reader/collector: %+v", snapshot)
	}
	if err := controller.CloseReaderWindow(context.Background(), "synthetic:window:bootstrap"); err != nil {
		t.Fatal(err)
	}
	if snapshot := controller.Snapshot(); snapshot.ReaderWindows != 1 || !snapshot.HasReader || !snapshot.AdmissionOpen {
		t.Fatalf("closing one bootstrap/window retired reader: %+v", snapshot)
	}
	if err := controller.CloseReaderWindow(context.Background(), "synthetic:window:post"); err != nil {
		t.Fatal(err)
	}
	if snapshot := controller.Snapshot(); snapshot.ReaderWindows != 0 || snapshot.HasReader || !snapshot.AdmissionOpen {
		t.Fatalf("fresh independent collector proof did not preserve collection: %+v", snapshot)
	}
	if owners.has(concurrentReader) == true {
		t.Fatal("retired reader owner remains")
	}
	continuedID, err := controller.QueueWork("synthetic:after-independent-reader-retirement")
	if err != nil {
		t.Fatal(err)
	}
	continuedTicket, err := controller.Ticket(continuedID)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Claim(context.Background(), continuedTicket); err != nil {
		t.Fatalf("independent collector claim: %v", err)
	}
	if err := controller.Complete(context.Background(), continuedTicket, "synthetic:independent-result"); err != nil {
		t.Fatalf("independent collector completion: %v", err)
	}
	if result, ok := controller.SinkResult(continuedID); !ok || result != "synthetic:independent-result" {
		t.Fatalf("independent result = %q, %v", result, ok)
	}
}

func TestConcurrentPermitRevocationFailurePreventsIndependentContinuation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	clock := newConcurrentTestClock()
	owners := newFakeConcurrentOwners()
	verifier := newFakeConcurrentVerifier(clock)
	revokeEntered := make(chan struct{}, 1)
	revokeRelease := make(chan struct{})
	permit := &fakeConcurrentPermit{
		clock: clock, revokeError: true,
		revokeEntered: revokeEntered, revokeRelease: revokeRelease,
	}
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(revokeRelease) })
	controller, err := newConcurrentController(parent, concurrentTestConfig(parent, clock, true), concurrentPorts{owners: owners, verifier: verifier, permit: permit})
	if err != nil {
		t.Fatalf("newConcurrentController: %v", err)
	}
	startConcurrentTest(t, controller)
	if err := controller.OpenReaderWindow(context.Background(), "synthetic:window:revoke-failure", "synthetic:target:revoke-failure", false); err != nil {
		t.Fatal(err)
	}
	workID, err := controller.QueueWork("synthetic:queued:revoke-failure")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := controller.Ticket(workID)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Claim(context.Background(), ticket); err != nil {
		t.Fatal(err)
	}
	revokeDone := make(chan struct{})
	go func() {
		controller.Revoke("synthetic:external-revoke")
		close(revokeDone)
	}()
	select {
	case <-revokeEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("revoke port did not block")
	}
	if snapshot := controller.Snapshot(); snapshot.AdmissionOpen || snapshot.PendingRevocations != 1 {
		t.Fatalf("pending revoke did not fence admission: %+v", snapshot)
	}
	if err := controller.CloseReaderWindow(context.Background(), "synthetic:window:revoke-failure"); !errors.Is(err, errConcurrentCleanup) {
		t.Fatalf("last reader close with failed revocation = %v", err)
	}
	snapshot := controller.Snapshot()
	if snapshot.AdmissionOpen || snapshot.PendingRevocations != 1 || !snapshot.HasCollector || snapshot.HasReader || snapshot.Queued != 0 || snapshot.Active != 0 {
		t.Fatalf("failed revoke allowed continuation or lost queued work: %+v", snapshot)
	}
	if !owners.has(concurrentCollector) || owners.has(concurrentReader) {
		t.Fatal("unexpected owners after revocation failure")
	}
	if permit.grantCount() != 1 {
		t.Fatalf("failed revocation was followed by a grant: %d grants", permit.grantCount())
	}
	if err := controller.Complete(context.Background(), ticket, "synthetic:late-after-failed-revoke"); !errors.Is(err, errConcurrentStale) {
		t.Fatalf("active result after pending revocation = %v", err)
	}
	releaseOnce.Do(func() { close(revokeRelease) })
	select {
	case <-revokeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked revoke did not return")
	}
	snapshot = controller.Snapshot()
	if snapshot.AdmissionOpen || snapshot.PendingRevocations != 0 || snapshot.Lifecycle != concurrentCleanupFailed || !snapshot.HasCollector {
		t.Fatalf("late revoke failure did not fail closed: %+v", snapshot)
	}
	if state, _ := controller.WorkState(workID); state != concurrentWorkCancelled {
		t.Fatalf("active work after revoke failure = %v", state)
	}
	cancelParent()
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := controller.WaitParentCleanup(ctx); !errors.Is(err, errConcurrentCleanup) {
		t.Fatalf("parent cleanup after failed revocation = %v", err)
	}
}

func TestConcurrentLastReaderCloseWithoutIndependentCapabilityHoldsCollector(t *testing.T) {
	controller, cancel, _, owners, _, _ := newConcurrentTestController(t, false)
	defer cancel()
	startConcurrentTest(t, controller)
	if err := controller.OpenReaderWindow(context.Background(), "synthetic:window:one", "synthetic:target:one", false); err != nil {
		t.Fatal(err)
	}
	workID, err := controller.QueueWork("synthetic:queued:one")
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.CloseReaderWindow(context.Background(), "synthetic:window:one"); !errors.Is(err, errConcurrentReaderRetained) {
		t.Fatalf("last close error = %v", err)
	}
	snapshot := controller.Snapshot()
	if snapshot.AdmissionOpen || !snapshot.HasCollector || snapshot.HasReader || snapshot.Queued != 1 || !owners.has(concurrentCollector) {
		t.Fatalf("unexpected hold state: %+v", snapshot)
	}
	if state, _ := controller.WorkState(workID); state != concurrentWorkQueued {
		t.Fatalf("queued work changed to %v", state)
	}
}

func TestConcurrentProofFailuresFailClosedBeforePermit(t *testing.T) {
	cases := map[string]func(*fakeConcurrentVerifier, *concurrentTestClock){
		"unknown":   func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.state[concurrentReader] = concurrentUnknown },
		"challenge": func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.wrongChallenge = true },
		"logged out": func(v *fakeConcurrentVerifier, _ *concurrentTestClock) {
			v.state[concurrentReader] = concurrentLoggedOut
		},
		"account mismatch":       func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.wrongAccount = true },
		"revision mismatch":      func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.wrongRevision = true },
		"owner mismatch":         func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.wrongOwner = true },
		"pre-challenge evidence": func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.staleBy = time.Second },
		"stale proof": func(v *fakeConcurrentVerifier, _ *concurrentTestClock) {
			v.staleBy = concurrentProofMaxAge + time.Second
		},
		"binding deadline": func(v *fakeConcurrentVerifier, clock *concurrentTestClock) {
			v.advanceAfter = concurrentReader
			v.advanceBy = concurrentBindingDeadline + time.Second
			_ = clock
		},
		"challenge replay": func(v *fakeConcurrentVerifier, _ *concurrentTestClock) { v.replay = true },
	}
	for name, configure := range cases {
		t.Run(name, func(t *testing.T) {
			controller, cancel, clock, owners, verifier, permit := newConcurrentTestController(t, false)
			defer cancel()
			configure(verifier, clock)
			if err := controller.Start(context.Background()); err == nil {
				t.Fatal("Start accepted invalid owner evidence")
			}
			if snapshot := controller.Snapshot(); snapshot.AdmissionOpen || snapshot.Lifecycle == concurrentRunning {
				t.Fatalf("failure opened admission: %+v", snapshot)
			}
			if permit.grantCount() != 0 {
				t.Fatalf("invalid evidence reached permit port: %d grants", permit.grantCount())
			}
			if owners.has(concurrentReader) || owners.has(concurrentCollector) {
				t.Fatal("startup failure left an owned fake tree behind")
			}
		})
	}
}

func TestConcurrentForeignOwnerReferencesAreNeverClosed(t *testing.T) {
	cases := map[string]func(concurrentOwner) concurrentOwner{
		"foreign reader parent": func(owner concurrentOwner) concurrentOwner {
			if owner.Role == concurrentReader {
				owner.ParentID = "synthetic:other-parent"
			}
			return owner
		},
		"foreign collector directory": func(owner concurrentOwner) concurrentOwner {
			if owner.Role == concurrentCollector {
				owner.DirectoryID = "synthetic:foreign-directory"
			}
			return owner
		},
		"ambiguous duplicate pid": func(owner concurrentOwner) concurrentOwner {
			if owner.Role == concurrentCollector {
				owner.PID = 7001
			}
			return owner
		},
	}
	for name, transform := range cases {
		t.Run(name, func(t *testing.T) {
			controller, cancel, _, owners, _, _ := newConcurrentTestControllerExpectingCleanupFailure(t, false)
			defer cancel()
			owners.mu.Lock()
			owners.transform = transform
			owners.mu.Unlock()
			if err := controller.Start(context.Background()); err == nil {
				t.Fatal("Start accepted foreign or ambiguous ownership")
			}
			if snapshot := controller.Snapshot(); snapshot.Lifecycle != concurrentCleanupFailed {
				t.Fatalf("uncertain ownership was reported clean: %+v", snapshot)
			}
			owners.mu.Lock()
			closed := owners.closeOwnerCall
			readerLive := owners.owners[concurrentReader].ID != ""
			collectorLive := owners.owners[concurrentCollector].ID != ""
			owners.mu.Unlock()
			if name == "foreign reader parent" && (closed != 0 || !readerLive) {
				t.Fatalf("foreign reader reference was touched: closed=%d readerLive=%v", closed, readerLive)
			}
			if name == "foreign collector directory" && (closed != 1 || !collectorLive || readerLive) {
				t.Fatalf("cleanup crossed foreign collector boundary: closed=%d readerLive=%v collectorLive=%v", closed, readerLive, collectorLive)
			}
			if name == "ambiguous duplicate pid" && (closed != 0 || !readerLive || !collectorLive) {
				t.Fatalf("ambiguous pair was closed: closed=%d readerLive=%v collectorLive=%v", closed, readerLive, collectorLive)
			}
		})
	}
}

func TestConcurrentPermitIsRevokedWhenPostGrantChecksFail(t *testing.T) {
	t.Run("deadline expires during grant", func(t *testing.T) {
		controller, cancel, _, _, _, permit := newConcurrentTestController(t, false)
		defer cancel()
		permit.advance = concurrentBindingDeadline + time.Millisecond
		if err := controller.Start(context.Background()); err == nil {
			t.Fatal("Start accepted a permit after the binding window")
		}
		revokes := permit.revocationSnapshot()
		if len(revokes) != 1 || revokes[0].ID != "synthetic:permit:1" || revokes[0].Epoch != controller.Snapshot().Epoch-1 {
			t.Fatalf("returned permit was not revoked under its original fence: %+v state=%+v", revokes, controller.Snapshot())
		}
	})
	t.Run("owner changes during grant", func(t *testing.T) {
		controller, cancel, _, owners, _, permit := newConcurrentTestControllerExpectingCleanupFailure(t, false)
		defer cancel()
		permit.grantHook = func() {
			owners.mutate(concurrentCollector, func(owner concurrentOwner) concurrentOwner {
				owner.PID++
				return owner
			})
		}
		if err := controller.Start(context.Background()); err == nil {
			t.Fatal("Start accepted an owner that changed during permit grant")
		}
		if controller.Snapshot().AdmissionOpen {
			t.Fatal("post-grant owner drift left admission open")
		}
		revokes := permit.revocationSnapshot()
		if len(revokes) != 1 || revokes[0].ID != "synthetic:permit:1" {
			t.Fatalf("permit leaked after post-grant owner drift: %+v", revokes)
		}
		if controller.Snapshot().Lifecycle != concurrentCleanupFailed || !owners.has(concurrentCollector) {
			t.Fatalf("foreign drifted owner was falsely cleaned: state=%+v", controller.Snapshot())
		}
	})
}

func TestConcurrentTicketPinsEpochGenerationPIDAndDriver(t *testing.T) {
	controller, cancel, _, _, _, _ := newConcurrentTestController(t, false)
	defer cancel()
	startConcurrentTest(t, controller)
	workID, err := controller.QueueWork("synthetic:ticket:payload")
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := controller.Ticket(workID)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*concurrentTicket){
		"controller id":        func(t *concurrentTicket) { t.ControllerID = "synthetic:controller:other" },
		"attempt id":           func(t *concurrentTicket) { t.AttemptID = "synthetic:attempt:other" },
		"epoch":                func(t *concurrentTicket) { t.Epoch++ },
		"pair generation":      func(t *concurrentTicket) { t.PairGeneration++ },
		"collector pid":        func(t *concurrentTicket) { t.CollectorPID++ },
		"collector creation":   func(t *concurrentTicket) { t.CollectorCreation = "synthetic:creation:other" },
		"collector generation": func(t *concurrentTicket) { t.CollectorGeneration++ },
		"driver":               func(t *concurrentTicket) { t.CollectorDriver = "synthetic-other-driver" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			stale := ticket
			mutate(&stale)
			if err := controller.Claim(context.Background(), stale); !errors.Is(err, errConcurrentStale) {
				t.Fatalf("Claim error = %v", err)
			}
		})
	}
}

func TestConcurrentTicketCannotCrossControllerEvenWithMatchingFakeOwnerIDs(t *testing.T) {
	first, cancelFirst, _, _, _, _ := newConcurrentTestController(t, false)
	defer cancelFirst()
	second, cancelSecond, _, _, _, _ := newConcurrentTestController(t, false)
	defer cancelSecond()
	startConcurrentTest(t, first)
	startConcurrentTest(t, second)
	firstWork, _ := first.QueueWork("synthetic:first-payload")
	secondWork, _ := second.QueueWork("synthetic:second-payload")
	firstTicket, err := first.Ticket(firstWork)
	if err != nil {
		t.Fatal(err)
	}
	secondTicket, err := second.Ticket(secondWork)
	if err != nil {
		t.Fatal(err)
	}
	if firstTicket.CollectorID != secondTicket.CollectorID || firstTicket.Epoch == secondTicket.Epoch || firstTicket.ControllerID == secondTicket.ControllerID {
		t.Fatalf("fixture did not create matching physical IDs under distinct fences: first=%+v second=%+v", firstTicket, secondTicket)
	}
	if err := second.Claim(context.Background(), firstTicket); !errors.Is(err, errConcurrentStale) {
		t.Fatalf("cross-controller claim error = %v", err)
	}
	if state, _ := second.WorkState(secondWork); state != concurrentWorkQueued || !second.Snapshot().AdmissionOpen {
		t.Fatalf("foreign ticket changed current queue or admission: %+v", second.Snapshot())
	}
}

func TestConcurrentOwnerDriftAndProofExpiryCloseAdmission(t *testing.T) {
	t.Run("owner drift", func(t *testing.T) {
		controller, cancel, _, owners, _, _ := newConcurrentTestControllerExpectingCleanupFailure(t, false)
		defer cancel()
		startConcurrentTest(t, controller)
		workID, _ := controller.QueueWork("synthetic:owner-drift")
		ticket, _ := controller.Ticket(workID)
		owners.mutate(concurrentCollector, func(owner concurrentOwner) concurrentOwner { owner.PID++; return owner })
		if err := controller.Claim(context.Background(), ticket); !errors.Is(err, errConcurrentStale) {
			t.Fatalf("Claim error = %v", err)
		}
		if controller.Snapshot().AdmissionOpen {
			t.Fatal("owner drift left admission open")
		}
	})
	t.Run("proof expiry", func(t *testing.T) {
		controller, cancel, clock, _, _, _ := newConcurrentTestController(t, false)
		defer cancel()
		startConcurrentTest(t, controller)
		workID, _ := controller.QueueWork("synthetic:proof-expiry")
		ticket, _ := controller.Ticket(workID)
		clock.Advance(concurrentProofMaxAge + time.Millisecond)
		if err := controller.Claim(context.Background(), ticket); !errors.Is(err, errConcurrentProof) {
			t.Fatalf("Claim error = %v", err)
		}
		if controller.Snapshot().AdmissionOpen {
			t.Fatal("expired owner evidence left admission open")
		}
	})
}

func TestConcurrentCancelDuplicateAndRevokeResultLinearization(t *testing.T) {
	controller, cancel, _, owners, _, _ := newConcurrentTestController(t, false)
	defer cancel()
	startConcurrentTest(t, controller)
	cancelledID, _ := controller.QueueWork("synthetic:cancelled")
	cancelledTicket, _ := controller.Ticket(cancelledID)
	if err := controller.Claim(context.Background(), cancelledTicket); err != nil {
		t.Fatal(err)
	}
	if err := controller.Cancel(cancelledTicket); err != nil {
		t.Fatal(err)
	}
	if err := controller.Complete(context.Background(), cancelledTicket, "synthetic:late-result"); !errors.Is(err, errConcurrentStale) {
		t.Fatalf("cancelled result error = %v", err)
	}
	if _, ok := controller.SinkResult(cancelledID); ok {
		t.Fatal("cancelled work reached sink")
	}
	completedID, _ := controller.QueueWork("synthetic:completed")
	completedTicket, _ := controller.Ticket(completedID)
	if err := controller.Claim(context.Background(), completedTicket); err != nil {
		t.Fatal(err)
	}
	if err := controller.Complete(context.Background(), completedTicket, "synthetic:first"); err != nil {
		t.Fatal(err)
	}
	if err := controller.Complete(context.Background(), completedTicket, "synthetic:duplicate"); !errors.Is(err, errConcurrentStale) {
		t.Fatalf("duplicate result error = %v", err)
	}

	raceID, _ := controller.QueueWork("synthetic:race")
	raceTicket, _ := controller.Ticket(raceID)
	if err := controller.Claim(context.Background(), raceTicket); err != nil {
		t.Fatal(err)
	}
	owners.mu.Lock()
	owners.blockCurrent = concurrentCollector
	owners.currentEntered = make(chan struct{}, 1)
	owners.currentRelease = make(chan struct{})
	entered, release := owners.currentEntered, owners.currentRelease
	owners.mu.Unlock()
	completeDone := make(chan error, 1)
	go func() {
		completeDone <- controller.Complete(context.Background(), raceTicket, "synthetic:raced-result")
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("result did not reach owner recheck")
	}
	controller.Revoke("test-revocation")
	close(release)
	if err := <-completeDone; !errors.Is(err, errConcurrentStale) {
		t.Fatalf("result after revoke error = %v", err)
	}
	if _, ok := controller.SinkResult(raceID); ok {
		t.Fatal("result committed after revocation")
	}
	if state, _ := controller.WorkState(raceID); state != concurrentWorkCancelled {
		t.Fatalf("revoked active work state = %v", state)
	}
}

func TestConcurrentCollectorLossAndDisablePreserveReaderUntilUserClose(t *testing.T) {
	t.Run("collector loss", func(t *testing.T) {
		controller, cancel, _, owners, _, _ := newConcurrentTestController(t, false)
		defer cancel()
		startConcurrentTest(t, controller)
		if err := controller.OpenReaderWindow(context.Background(), "synthetic:window:kept", "synthetic:target:kept", true); err != nil {
			t.Fatal(err)
		}
		owners.mu.Lock()
		delete(owners.owners, concurrentCollector)
		owners.mu.Unlock()
		controller.CollectorLost()
		snapshot := controller.Snapshot()
		if snapshot.AdmissionOpen || !snapshot.HasReader || !snapshot.HasCollector || snapshot.ReaderWindows != 1 || !owners.has(concurrentReader) {
			t.Fatalf("collector loss did not preserve reader: %+v", snapshot)
		}
	})
	t.Run("opt out", func(t *testing.T) {
		controller, cancel, _, owners, _, _ := newConcurrentTestController(t, false)
		defer cancel()
		startConcurrentTest(t, controller)
		if err := controller.OpenReaderWindow(context.Background(), "synthetic:window:retained", "synthetic:target:retained", false); err != nil {
			t.Fatal(err)
		}
		if err := controller.Disable(context.Background()); err != nil {
			t.Fatal(err)
		}
		snapshot := controller.Snapshot()
		if snapshot.AdmissionOpen || snapshot.Lifecycle != concurrentDisabledReaderRetained || snapshot.ReaderWindows != 1 || snapshot.HasCollector || !snapshot.HasReader {
			t.Fatalf("disable state = %+v", snapshot)
		}
		if owners.has(concurrentCollector) || !owners.has(concurrentReader) {
			t.Fatal("disable did not close only the collector owner")
		}
		if err := controller.CloseReaderWindow(context.Background(), "synthetic:window:retained"); err != nil {
			t.Fatal(err)
		}
		if snapshot = controller.Snapshot(); snapshot.Lifecycle != concurrentCleanupComplete || snapshot.HasReader || snapshot.HasCollector {
			t.Fatalf("final reader close state = %+v", snapshot)
		}
	})
}

func TestConcurrentParentDeathInvalidatesWhileProbeBlockedAndCleansOwnedTree(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := newConcurrentTestClock()
	owners := newFakeConcurrentOwners()
	verifier := newFakeConcurrentVerifier(clock)
	verifier.blockRole = concurrentReader
	verifier.entered = make(chan struct{}, 1)
	verifier.release = make(chan struct{})
	verifier.seenCancellation = make(chan struct{}, 1)
	verifier.cancelWhileBlocked = true
	permit := &fakeConcurrentPermit{clock: clock}
	controller, err := newConcurrentController(parent, concurrentTestConfig(parent, clock, false), concurrentPorts{owners: owners, verifier: verifier, permit: permit})
	if err != nil {
		t.Fatal(err)
	}
	initialEpoch := controller.Snapshot().Epoch
	startDone := make(chan error, 1)
	go func() { startDone <- controller.Start(context.Background()) }()
	select {
	case <-verifier.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("startup did not block in the reader probe")
	}
	cancel()
	waitForConcurrent(t, func() bool { return controller.Snapshot().Epoch > initialEpoch })
	if controller.Snapshot().AdmissionOpen {
		t.Fatal("parent death left admission open during a blocked probe")
	}
	select {
	case <-verifier.seenCancellation:
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not reach the blocked probe context")
	}
	close(verifier.release)
	if err := <-startDone; err == nil {
		t.Fatal("startup succeeded after parent cancellation")
	}
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := controller.WaitParentCleanup(ctx); err != nil {
		t.Fatalf("WaitParentCleanup: %v", err)
	}
	if owners.has(concurrentReader) || owners.has(concurrentCollector) || controller.Snapshot().Lifecycle != concurrentCleanupComplete {
		t.Fatalf("parent cleanup incomplete: %+v", controller.Snapshot())
	}
	if _, err := controller.QueueWork("synthetic:after-parent-death"); !errors.Is(err, errConcurrentUnavailable) {
		t.Fatalf("QueueWork after parent death error = %v", err)
	}
}

func TestConcurrentCleanupFailureBlocksRestartAndReplacement(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	clock := newConcurrentTestClock()
	owners := newFakeConcurrentOwners()
	verifier := newFakeConcurrentVerifier(clock)
	permit := &fakeConcurrentPermit{clock: clock}
	controller, err := newConcurrentController(parent, concurrentTestConfig(parent, clock, false), concurrentPorts{owners: owners, verifier: verifier, permit: permit})
	if err != nil {
		t.Fatalf("newConcurrentController: %v", err)
	}
	startConcurrentTest(t, controller)
	owners.mu.Lock()
	owners.closeFailures[concurrentCollector] = true
	createsBefore := owners.createCalls
	owners.mu.Unlock()
	if err := controller.Disable(context.Background()); !errors.Is(err, errConcurrentCleanup) {
		t.Fatalf("Disable error = %v", err)
	}
	if snapshot := controller.Snapshot(); snapshot.Lifecycle != concurrentCleanupFailed || !snapshot.HasCollector {
		t.Fatalf("failed cleanup was reported clean: %+v", snapshot)
	}
	if err := controller.Start(context.Background()); !errors.Is(err, errConcurrentUnavailable) {
		t.Fatalf("restart error = %v", err)
	}
	createsAfter, _, _ := owners.counts()
	if createsAfter != createsBefore {
		t.Fatalf("replacement owner started after failed cleanup: %d -> %d", createsBefore, createsAfter)
	}
	cancelParent()
	ctx, done := context.WithTimeout(context.Background(), 2*time.Second)
	defer done()
	if err := controller.WaitParentCleanup(ctx); !errors.Is(err, errConcurrentCleanup) {
		t.Fatalf("parent cleanup failure = %v, want unverified cleanup", err)
	}
	if snapshot := controller.Snapshot(); snapshot.Lifecycle != concurrentCleanupFailed || !owners.has(concurrentCollector) {
		t.Fatalf("parent cleanup falsely reported uncertain owner clean: %+v", snapshot)
	}
}

func waitForConcurrent(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for concurrent controller state")
}
