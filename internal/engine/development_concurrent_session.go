//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	concurrentProvider        = "synthetic-concurrent"
	concurrentProofMaxAge     = 5 * time.Second
	concurrentBindingDeadline = 2 * time.Second
	concurrentStartTimeout    = 10 * time.Second
	concurrentCleanupTimeout  = 5 * time.Second
)

var (
	errConcurrentDisabled       = errors.New("synthetic concurrent controller is disabled")
	errConcurrentUnavailable    = errors.New("synthetic concurrent controller unavailable")
	errConcurrentStale          = errors.New("synthetic concurrent handle is stale")
	errConcurrentProof          = errors.New("synthetic concurrent owner proof rejected")
	errConcurrentCleanup        = errors.New("synthetic concurrent cleanup unverified")
	errConcurrentReaderRetained = errors.New("reader retired; collector has no independent authority")
)

type concurrentRole string

const (
	concurrentReader    concurrentRole = "reader"
	concurrentCollector concurrentRole = "collector"
)

type concurrentAuthState string

const (
	concurrentAuthenticated concurrentAuthState = "authenticated"
	concurrentLoggedOut     concurrentAuthState = "logged_out"
	concurrentChallenge     concurrentAuthState = "challenge"
	concurrentUnknown       concurrentAuthState = "unknown"
)

type concurrentOwnerSpec struct {
	Role        concurrentRole
	ProfileID   string
	DirectoryID string
	ParentID    string
	Driver      string
}

// concurrentOwner is an in-memory fixture binding. Values returned by the
// private fixture port are not OS process or Job ownership attestations.
type concurrentOwner struct {
	ID               string
	Role             concurrentRole
	ProfileID        string
	DirectoryID      string
	PID              int
	CreationIdentity string
	ParentID         string
	Generation       uint64
	Driver           string
}

type concurrentProofRequest struct {
	Provider       string
	ControllerID   string
	AttemptID      string
	Epoch          uint64
	PairGeneration uint64
	Role           concurrentRole
	Challenge      string
	Owner          concurrentOwner
	RequestedAt    time.Time
}

type concurrentProof struct {
	Provider       string
	ControllerID   string
	AttemptID      string
	Epoch          uint64
	PairGeneration uint64
	Role           concurrentRole
	Challenge      string
	Owner          concurrentOwner
	AuthState      concurrentAuthState
	AccountID      string
	Revision       string
	ObservedAt     time.Time
}

type concurrentPermitRequest struct {
	Provider       string
	ControllerID   string
	AttemptID      string
	Epoch          uint64
	PairGeneration uint64
	AccountID      string
	Revision       string
	Mode           string
	Owners         []concurrentOwner
}

type concurrentPermit struct {
	ID      string
	Granted bool
}

// These ports are intentionally private and fixture-only. They must be backed
// by in-memory fakes in this increment; none represents a public attestation.
type concurrentOwnerPort interface {
	CreateOwner(context.Context, concurrentOwnerSpec) (concurrentOwner, error)
	CurrentOwner(context.Context, concurrentRole) (concurrentOwner, bool, error)
	OpenReaderWindow(context.Context, concurrentOwner, string, string, bool) error
	CloseReaderWindow(context.Context, concurrentOwner, string) error
	ReaderInventoryEmpty(context.Context, concurrentOwner) (bool, error)
	CloseOwner(context.Context, concurrentOwner) error
	OwnerAbsent(context.Context, concurrentRole, string) (bool, error)
}

type concurrentVerifierPort interface {
	Prove(context.Context, concurrentProofRequest) (concurrentProof, error)
}

type concurrentPermitPort interface {
	Grant(context.Context, concurrentPermitRequest) (concurrentPermit, error)
	Revoke(context.Context, string, uint64) error
}

type concurrentPorts struct {
	owners   concurrentOwnerPort
	verifier concurrentVerifierPort
	permit   concurrentPermitPort
}

// concurrentAuthorizationBinding is the controller's exact in-memory fence
// snapshot passed to the optional private authorization seam. It carries no
// native process attestation and is only consumed by the dual-tag fixture.
type concurrentAuthorizationBinding struct {
	ControllerID   string
	AttemptID      string
	ParentID       string
	Epoch          uint64
	PairGeneration uint64
	AccountID      string
	Reader         concurrentOwner
	Collector      concurrentOwner
	ReaderWindows  []string
}

type concurrentAuthorizationTicket struct {
	ID              string
	EvidenceVersion uint64
	WorkID          string
}

// The seam is optional so the original controller source and tests remain
// independently named-file testable. Commit callbacks run with c.mu already
// held and must be local, non-reentrant mutations; implementations must never
// call back to reacquire c.mu while holding authority state locks.
type concurrentAuthorizationSeam interface {
	Start(context.Context, concurrentAuthorizationBinding) error
	RenewEvidence(context.Context, concurrentAuthorizationBinding, func(context.Context) error) error
	Ticket(string) (concurrentAuthorizationTicket, error)
	Claim(concurrentAuthorizationTicket, func() error) error
	Complete(concurrentAuthorizationTicket, string, func() error) error
	Cancel(concurrentAuthorizationTicket, func() error) error
	Check() error
	Revoke(string)
	PolicyChanged(uint64, uint64, bool)
	OwnerChanged(concurrentRole)
	UpdateReaderWindows([]string) error
	RequestIndependentCollector() error
}

type concurrentConfig struct {
	Enabled              bool
	DevBuild             bool
	FixtureBuild         bool
	OwnerGrant           bool
	ControllerGrant      bool
	Provider             string
	AttemptID            string
	ParentID             string
	ExpectedAccountID    string
	ExpectedRevision     string
	ReaderProfileID      string
	ReaderDirectoryID    string
	CollectorProfileID   string
	CollectorDirectoryID string
	IndependentCollector bool
	now                  func() time.Time
}

type concurrentWorkState uint8

const (
	concurrentWorkQueued concurrentWorkState = iota
	concurrentWorkActive
	concurrentWorkComplete
	concurrentWorkCancelled
)

type concurrentWork struct {
	id      string
	payload string
	state   concurrentWorkState
}

// concurrentTicket pins work to one controller fence and collector owner.
type concurrentTicket struct {
	ControllerID          string
	AttemptID             string
	WorkID                string
	Epoch                 uint64
	PairGeneration        uint64
	CollectorID           string
	CollectorGeneration   uint64
	CollectorPID          int
	CollectorCreation     string
	CollectorDriver       string
	EvidenceVersion       uint64
	authorizationTicketID string
}

type concurrentLifecycle string

const (
	concurrentNew                    concurrentLifecycle = "new"
	concurrentStarting               concurrentLifecycle = "starting"
	concurrentRunning                concurrentLifecycle = "running"
	concurrentDisabledReaderRetained concurrentLifecycle = "disabled-reader-retained"
	concurrentCollectorLost          concurrentLifecycle = "collector-lost"
	concurrentCleanupComplete        concurrentLifecycle = "complete-cleanup"
	concurrentCleanupFailed          concurrentLifecycle = "cleanup-failed"
	concurrentClosed                 concurrentLifecycle = "closed"
)

type concurrentSnapshot struct {
	Enabled            bool
	AdmissionOpen      bool
	PendingRevocations int
	Epoch              uint64
	PairGeneration     uint64
	Lifecycle          concurrentLifecycle
	ReaderWindows      int
	Queued             int
	Active             int
	Collector          concurrentOwner
	HasCollector       bool
	Reader             concurrentOwner
	HasReader          bool
}

type concurrentController struct {
	parent        context.Context
	cfg           concurrentConfig
	ports         concurrentPorts
	authorization concurrentAuthorizationSeam

	opMu sync.Mutex
	mu   sync.Mutex

	enabled                bool
	admissionOpen          bool
	permit                 concurrentPermit
	permitOpen             bool
	permitEpoch            uint64
	controllerID           string
	epoch                  uint64
	pairGeneration         uint64
	attempt                uint64
	lifecycle              concurrentLifecycle
	revoked                bool
	continuationBlocked    bool
	started                bool
	disabled               bool
	cleanupFailed          bool
	pendingRevocations     int
	pendingRevocationsDone chan struct{}
	readerRetired          bool
	reader                 *concurrentOwner
	collector              *concurrentOwner
	readerWindows          map[string]string
	proofs                 map[concurrentRole]concurrentProof
	usedChallenges         map[string]bool
	work                   map[string]*concurrentWork
	workOrder              []string
	sink                   map[string]string
	nextID                 uint64

	parentWatchOnce sync.Once
	parentDone      chan struct{}
}

// newConcurrentController is private to the dual-tag fixture. An all-zero
// config creates a default-off controller without touching any port.
func newConcurrentController(parent context.Context, cfg concurrentConfig, ports concurrentPorts) (*concurrentController, error) {
	c := newConcurrentControllerShell(parent, cfg, ports)
	if !cfg.Enabled {
		return c, nil
	}
	if parent == nil || !cfg.DevBuild || !cfg.FixtureBuild || !cfg.OwnerGrant || !cfg.ControllerGrant || cfg.Provider != concurrentProvider {
		return nil, errConcurrentUnavailable
	}
	if !isSyntheticID(cfg.AttemptID) || !isSyntheticID(cfg.ParentID) || !isSyntheticID(cfg.ExpectedAccountID) ||
		!isSyntheticID(cfg.ExpectedRevision) || !isSyntheticID(cfg.ReaderProfileID) || !isSyntheticID(cfg.ReaderDirectoryID) ||
		!isSyntheticID(cfg.CollectorProfileID) || !isSyntheticID(cfg.CollectorDirectoryID) ||
		cfg.ReaderProfileID == cfg.CollectorProfileID || cfg.ReaderDirectoryID == cfg.CollectorDirectoryID {
		return nil, errConcurrentUnavailable
	}
	if ports.owners == nil || ports.verifier == nil || ports.permit == nil {
		return nil, errConcurrentUnavailable
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errConcurrentUnavailable
	}
	c.controllerID = "synthetic:controller:" + hex.EncodeToString(nonce[:])
	c.epoch = uint64(nonce[0]) | uint64(nonce[1])<<8 | uint64(nonce[2])<<16 | uint64(nonce[3])<<24 |
		uint64(nonce[4])<<32 | uint64(nonce[5])<<40 | uint64(nonce[6])<<48 | uint64(nonce[7])<<56
	if c.epoch == 0 {
		c.epoch = 1
	}
	if cfg.now == nil {
		c.cfg.now = time.Now
	}
	c.enabled = true
	return c, nil
}

func newConcurrentControllerShell(parent context.Context, cfg concurrentConfig, ports concurrentPorts) *concurrentController {
	return &concurrentController{
		parent:         parent,
		cfg:            cfg,
		ports:          ports,
		lifecycle:      concurrentNew,
		readerWindows:  make(map[string]string),
		proofs:         make(map[concurrentRole]concurrentProof),
		usedChallenges: make(map[string]bool),
		work:           make(map[string]*concurrentWork),
		sink:           make(map[string]string),
		parentDone:     make(chan struct{}),
	}
}

// newConcurrentControllerWithAuthorization is a separate fixture-only
// constructor. The optional seam intentionally bypasses the legacy shared
// ExpectedRevision gate; its own one-shot derivation pins role revisions.
func newConcurrentControllerWithAuthorization(parent context.Context, cfg concurrentConfig, ports concurrentPorts, authorization concurrentAuthorizationSeam) (*concurrentController, error) {
	c := newConcurrentControllerShell(parent, cfg, ports)
	if !cfg.Enabled {
		return c, nil
	}
	if parent == nil || !cfg.DevBuild || !cfg.FixtureBuild || !cfg.OwnerGrant || !cfg.ControllerGrant || cfg.Provider != concurrentProvider || authorization == nil || cfg.IndependentCollector {
		return nil, errConcurrentUnavailable
	}
	if !isSyntheticID(cfg.AttemptID) || !isSyntheticID(cfg.ParentID) || !isSyntheticID(cfg.ExpectedAccountID) ||
		!isSyntheticID(cfg.ReaderProfileID) || !isSyntheticID(cfg.ReaderDirectoryID) ||
		!isSyntheticID(cfg.CollectorProfileID) || !isSyntheticID(cfg.CollectorDirectoryID) ||
		cfg.ReaderProfileID == cfg.CollectorProfileID || cfg.ReaderDirectoryID == cfg.CollectorDirectoryID || ports.owners == nil {
		return nil, errConcurrentUnavailable
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, errConcurrentUnavailable
	}
	c.controllerID = "synthetic:controller:" + hex.EncodeToString(nonce[:])
	c.epoch = uint64(nonce[0]) | uint64(nonce[1])<<8 | uint64(nonce[2])<<16 | uint64(nonce[3])<<24 |
		uint64(nonce[4])<<32 | uint64(nonce[5])<<40 | uint64(nonce[6])<<48 | uint64(nonce[7])<<56
	if c.epoch == 0 {
		c.epoch = 1
	}
	if cfg.now == nil {
		c.cfg.now = time.Now
	}
	c.authorization = authorization
	c.enabled = true
	return c, nil
}

func isSyntheticID(value string) bool {
	return strings.HasPrefix(value, "synthetic:") && len(strings.TrimSpace(strings.TrimPrefix(value, "synthetic:"))) > 0
}

func (c *concurrentController) now() time.Time { return c.cfg.now() }

func (c *concurrentController) operationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	linked, cancelLinked := context.WithCancel(parent)
	var stopParent func() bool
	if c.parent != nil {
		stopParent = context.AfterFunc(c.parent, cancelLinked)
	}
	bounded, cancelBounded := context.WithTimeout(linked, timeout)
	return bounded, func() {
		cancelBounded()
		if stopParent != nil {
			stopParent()
		}
		cancelLinked()
	}
}

func (c *concurrentController) ensureParentWatcher() {
	c.parentWatchOnce.Do(func() {
		go func() {
			<-c.parent.Done()
			c.invalidate("parent-cancelled")
			c.opMu.Lock()
			cleanupCtx, cancel := context.WithTimeout(context.Background(), concurrentCleanupTimeout)
			if err := c.waitRevocations(cleanupCtx); err != nil {
				c.markCleanupFailed()
			}
			_ = c.closeOwnedTrees(cleanupCtx)
			cancel()
			c.opMu.Unlock()
			close(c.parentDone)
		}()
	})
}

// Start establishes both fake owners while admission is closed, consumes a
// fresh challenge for each role, then asks the separate synthetic permit port.
func (c *concurrentController) Start(ctx context.Context) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	if ctx == nil {
		ctx = c.parent
	}
	startCtx, cancelStart := c.operationContext(ctx, concurrentStartTimeout)
	defer cancelStart()
	ctx = startCtx
	c.ensureParentWatcher()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	if c.started || c.disabled || c.cleanupFailed || c.parent.Err() != nil {
		c.mu.Unlock()
		return errConcurrentUnavailable
	}
	c.started = true
	c.attempt++
	c.pairGeneration++
	c.revoked = false
	c.lifecycle = concurrentStarting
	epoch, pairGeneration := c.epoch, c.pairGeneration
	c.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return c.failStart(err)
	}
	reader, err := c.ports.owners.CreateOwner(ctx, concurrentOwnerSpec{
		Role: concurrentReader, ProfileID: c.cfg.ReaderProfileID, DirectoryID: c.cfg.ReaderDirectoryID,
		ParentID: c.cfg.ParentID, Driver: "synthetic-reader",
	})
	if err != nil {
		return c.failStart(err)
	}
	readerSpec := concurrentOwnerSpec{Role: concurrentReader, ProfileID: c.cfg.ReaderProfileID, DirectoryID: c.cfg.ReaderDirectoryID, ParentID: c.cfg.ParentID, Driver: "synthetic-reader"}
	if !validOwner(reader, readerSpec) {
		c.markCleanupFailed()
		return c.failStart(errConcurrentUnavailable)
	}
	c.mu.Lock()
	c.reader = &reader
	c.mu.Unlock()
	if err := c.checkStartFence(epoch, pairGeneration, ctx); err != nil {
		return c.failStart(err)
	}
	collector, err := c.ports.owners.CreateOwner(ctx, concurrentOwnerSpec{
		Role: concurrentCollector, ProfileID: c.cfg.CollectorProfileID, DirectoryID: c.cfg.CollectorDirectoryID,
		ParentID: c.cfg.ParentID, Driver: "synthetic-headless",
	})
	if err != nil {
		return c.failStart(err)
	}
	collectorSpec := concurrentOwnerSpec{Role: concurrentCollector, ProfileID: c.cfg.CollectorProfileID, DirectoryID: c.cfg.CollectorDirectoryID, ParentID: c.cfg.ParentID, Driver: "synthetic-headless"}
	if !validOwner(collector, collectorSpec) {
		c.markCleanupFailed()
		return c.failStart(errConcurrentUnavailable)
	}
	c.mu.Lock()
	c.collector = &collector
	c.mu.Unlock()
	if err := validateOwnerPair(reader, collector, c.cfg); err != nil {
		return c.failStart(err)
	}
	if err := c.checkStartFence(epoch, pairGeneration, ctx); err != nil {
		return c.failStart(err)
	}
	if c.authorization != nil {
		return c.startAuthorized(ctx, epoch, pairGeneration, reader, collector)
	}

	bindingStart := c.now()
	bindingCtx, cancelBinding := context.WithTimeout(ctx, concurrentBindingDeadline)
	defer cancelBinding()
	readerProof, err := c.prove(bindingCtx, epoch, pairGeneration, concurrentReader, reader)
	if err != nil {
		return c.failStart(err)
	}
	if err := c.checkStartFence(epoch, pairGeneration, bindingCtx); err != nil {
		return c.failStart(err)
	}
	collectorProof, err := c.prove(bindingCtx, epoch, pairGeneration, concurrentCollector, collector)
	if err != nil {
		return c.failStart(err)
	}
	if err := c.checkStartFence(epoch, pairGeneration, bindingCtx); err != nil {
		return c.failStart(err)
	}
	if c.now().Sub(bindingStart) > concurrentBindingDeadline {
		return c.failStart(errConcurrentProof)
	}
	if err := c.checkOwnerCurrent(bindingCtx, reader); err != nil {
		return c.failStart(err)
	}
	if err := c.checkStartFence(epoch, pairGeneration, bindingCtx); err != nil {
		return c.failStart(err)
	}
	if err := c.checkOwnerCurrent(bindingCtx, collector); err != nil {
		return c.failStart(err)
	}
	if err := c.checkStartFence(epoch, pairGeneration, bindingCtx); err != nil {
		return c.failStart(err)
	}
	if !proofFresh(readerProof, c.now()) || !proofFresh(collectorProof, c.now()) {
		return c.failStart(errConcurrentProof)
	}
	permit, err := c.ports.permit.Grant(bindingCtx, concurrentPermitRequest{
		Provider: concurrentProvider, ControllerID: c.controllerID, AttemptID: c.cfg.AttemptID, Epoch: epoch, PairGeneration: pairGeneration,
		AccountID: c.cfg.ExpectedAccountID, Revision: c.cfg.ExpectedRevision, Mode: "reader-and-collector",
		Owners: []concurrentOwner{reader, collector},
	})
	c.rememberPermit(permit, epoch)
	if err != nil || !permit.Granted || !isSyntheticID(permit.ID) || bindingCtx.Err() != nil || c.now().Sub(bindingStart) > concurrentBindingDeadline {
		if err == nil {
			err = errConcurrentProof
		}
		return c.failStart(err)
	}
	if err := c.checkOwnerCurrent(bindingCtx, reader); err != nil {
		return c.failStart(err)
	}
	if err := c.checkOwnerCurrent(bindingCtx, collector); err != nil {
		return c.failStart(err)
	}
	if err := c.checkStartFence(epoch, pairGeneration, bindingCtx); err != nil {
		return c.failStart(err)
	}
	c.mu.Lock()
	if c.epoch != epoch || c.pairGeneration != pairGeneration || c.parent.Err() != nil || bindingCtx.Err() != nil || c.revoked || !proofFresh(readerProof, c.now()) || !proofFresh(collectorProof, c.now()) {
		c.mu.Unlock()
		c.invalidate("startup-stale-after-grant")
		return c.failStart(errConcurrentStale)
	}
	c.proofs[concurrentReader] = readerProof
	c.proofs[concurrentCollector] = collectorProof
	c.admissionOpen = true
	c.lifecycle = concurrentRunning
	c.mu.Unlock()
	return nil
}

func (c *concurrentController) startAuthorized(ctx context.Context, epoch, pairGeneration uint64, reader, collector concurrentOwner) error {
	started := c.now()
	bindingCtx, cancel := context.WithTimeout(ctx, concurrentBindingDeadline)
	defer cancel()
	binding := c.authorizationBinding(epoch, pairGeneration, reader, collector)
	if err := c.checkOwnerCurrent(bindingCtx, reader); err != nil {
		return c.failStart(errConcurrentStale)
	}
	if err := c.checkOwnerCurrent(bindingCtx, collector); err != nil {
		return c.failStart(errConcurrentStale)
	}
	if err := c.authorization.Start(bindingCtx, binding); err != nil {
		return c.failStart(err)
	}
	if err := c.checkOwnerCurrent(bindingCtx, reader); err != nil {
		return c.failStart(errConcurrentStale)
	}
	if err := c.checkOwnerCurrent(bindingCtx, collector); err != nil {
		return c.failStart(errConcurrentStale)
	}
	if bindingCtx.Err() != nil || c.now().Sub(started) >= concurrentBindingDeadline {
		return c.failStart(errConcurrentProof)
	}
	c.mu.Lock()
	if c.epoch != epoch || c.pairGeneration != pairGeneration || c.parent.Err() != nil || bindingCtx.Err() != nil || c.revoked || c.disabled || c.authorization.Check() != nil {
		c.mu.Unlock()
		return c.failStart(errConcurrentStale)
	}
	c.admissionOpen = true
	c.lifecycle = concurrentRunning
	c.mu.Unlock()
	return nil
}

func (c *concurrentController) authorizationBinding(epoch, pairGeneration uint64, reader, collector concurrentOwner) concurrentAuthorizationBinding {
	c.mu.Lock()
	windows := c.authorizationWindowsLocked()
	c.mu.Unlock()
	return concurrentAuthorizationBinding{
		ControllerID: c.controllerID, AttemptID: c.cfg.AttemptID, ParentID: c.cfg.ParentID,
		Epoch: epoch, PairGeneration: pairGeneration, AccountID: c.cfg.ExpectedAccountID,
		Reader: reader, Collector: collector, ReaderWindows: windows,
	}
}

func (c *concurrentController) checkStartFence(epoch, pairGeneration uint64, ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.parent.Err() != nil || c.epoch != epoch || c.pairGeneration != pairGeneration || c.revoked || c.disabled {
		return errConcurrentStale
	}
	return nil
}

func (c *concurrentController) prove(ctx context.Context, epoch, pairGeneration uint64, role concurrentRole, owner concurrentOwner) (concurrentProof, error) {
	c.mu.Lock()
	c.nextID++
	challenge := fmt.Sprintf("synthetic:challenge:%d:%d:%s", pairGeneration, c.nextID, role)
	c.usedChallenges[challenge] = false
	attemptID := c.cfg.AttemptID
	c.mu.Unlock()
	request := concurrentProofRequest{
		Provider: concurrentProvider, ControllerID: c.controllerID, AttemptID: attemptID, Epoch: epoch, PairGeneration: pairGeneration,
		Role: role, Challenge: challenge, Owner: owner, RequestedAt: c.now(),
	}
	proof, err := c.ports.verifier.Prove(ctx, request)
	if err != nil {
		return concurrentProof{}, err
	}
	if err := c.consumeProof(request, proof); err != nil {
		return concurrentProof{}, err
	}
	return proof, nil
}

func (c *concurrentController) consumeProof(request concurrentProofRequest, proof concurrentProof) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	used, exists := c.usedChallenges[request.Challenge]
	if !exists || used {
		return errConcurrentProof
	}
	c.usedChallenges[request.Challenge] = true
	if proof.Provider != request.Provider || proof.ControllerID != request.ControllerID || proof.AttemptID != request.AttemptID || proof.Epoch != request.Epoch ||
		proof.PairGeneration != request.PairGeneration || proof.Role != request.Role || proof.Challenge != request.Challenge ||
		!sameOwner(proof.Owner, request.Owner) || proof.AuthState != concurrentAuthenticated ||
		proof.AccountID != c.cfg.ExpectedAccountID || proof.Revision != c.cfg.ExpectedRevision || !isSyntheticID(proof.AccountID) ||
		!isSyntheticID(proof.Revision) || proof.ObservedAt.Before(request.RequestedAt) || !proofFresh(proof, c.now()) {
		return errConcurrentProof
	}
	return nil
}

func proofFresh(proof concurrentProof, now time.Time) bool {
	age := now.Sub(proof.ObservedAt)
	return age >= 0 && age <= concurrentProofMaxAge
}

func validateOwnerPair(reader, collector concurrentOwner, cfg concurrentConfig) error {
	if !validOwner(reader, concurrentOwnerSpec{Role: concurrentReader, ProfileID: cfg.ReaderProfileID, DirectoryID: cfg.ReaderDirectoryID, ParentID: cfg.ParentID, Driver: "synthetic-reader"}) ||
		!validOwner(collector, concurrentOwnerSpec{Role: concurrentCollector, ProfileID: cfg.CollectorProfileID, DirectoryID: cfg.CollectorDirectoryID, ParentID: cfg.ParentID, Driver: "synthetic-headless"}) ||
		reader.ID == collector.ID || reader.PID == collector.PID || reader.CreationIdentity == collector.CreationIdentity ||
		reader.ProfileID == collector.ProfileID || reader.DirectoryID == collector.DirectoryID {
		return errConcurrentUnavailable
	}
	return nil
}

func validOwner(owner concurrentOwner, expected concurrentOwnerSpec) bool {
	return isSyntheticID(owner.ID) && owner.Role == expected.Role && owner.ProfileID == expected.ProfileID &&
		owner.DirectoryID == expected.DirectoryID && owner.ParentID == expected.ParentID && owner.Driver == expected.Driver &&
		owner.PID > 0 && isSyntheticID(owner.CreationIdentity) && owner.Generation > 0
}

func sameOwner(a, b concurrentOwner) bool {
	return a.ID == b.ID && a.Role == b.Role && a.ProfileID == b.ProfileID && a.DirectoryID == b.DirectoryID &&
		a.PID == b.PID && a.CreationIdentity == b.CreationIdentity && a.ParentID == b.ParentID &&
		a.Generation == b.Generation && a.Driver == b.Driver
}

func (c *concurrentController) checkOwnerCurrent(ctx context.Context, expected concurrentOwner) error {
	current, exists, err := c.ports.owners.CurrentOwner(ctx, expected.Role)
	if err != nil || !exists || !sameOwner(current, expected) {
		return errConcurrentStale
	}
	return nil
}

// QueueWork stores synthetic-only payloads in memory. Queued entries survive
// revocation; an issued ticket is always bound to the current collector fence.
func (c *concurrentController) QueueWork(payload string) (string, error) {
	if !isSyntheticID(payload) {
		return "", errConcurrentUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabled || c.cleanupFailed || c.disabled || c.parent.Err() != nil || c.lifecycle == concurrentCleanupComplete || c.lifecycle == concurrentClosed {
		return "", errConcurrentUnavailable
	}
	c.nextID++
	id := fmt.Sprintf("synthetic:work:%d", c.nextID)
	c.work[id] = &concurrentWork{id: id, payload: payload, state: concurrentWorkQueued}
	c.workOrder = append(c.workOrder, id)
	return id, nil
}

func (c *concurrentController) Ticket(workID string) (concurrentTicket, error) {
	c.mu.Lock()
	work := c.work[workID]
	if c.admissionOpen && !c.admissionFreshLocked() {
		c.mu.Unlock()
		c.invalidate("proof-expired")
		return concurrentTicket{}, errConcurrentProof
	}
	if !c.admissionOpen || c.parent.Err() != nil || c.pendingRevocations > 0 || c.continuationBlocked || work == nil || work.state != concurrentWorkQueued || c.collector == nil {
		c.mu.Unlock()
		return concurrentTicket{}, errConcurrentUnavailable
	}
	ticket := c.ticketLocked(workID)
	if c.authorization != nil {
		authorityTicket, err := c.authorization.Ticket(workID)
		if err != nil {
			c.mu.Unlock()
			return concurrentTicket{}, errors.Join(errConcurrentProof, err)
		}
		ticket.EvidenceVersion = authorityTicket.EvidenceVersion
		ticket.authorizationTicketID = authorityTicket.ID
	}
	c.mu.Unlock()
	return ticket, nil
}

func (c *concurrentController) ticketLocked(workID string) concurrentTicket {
	owner := *c.collector
	return concurrentTicket{
		ControllerID: c.controllerID, AttemptID: c.cfg.AttemptID, WorkID: workID, Epoch: c.epoch, PairGeneration: c.pairGeneration, CollectorID: owner.ID,
		CollectorGeneration: owner.Generation, CollectorPID: owner.PID, CollectorCreation: owner.CreationIdentity,
		CollectorDriver: owner.Driver,
	}
}

func (c *concurrentController) proofsFreshLocked(now time.Time) bool {
	collector, ok := c.proofs[concurrentCollector]
	if !ok || !proofFresh(collector, now) {
		return false
	}
	if c.readerRetired {
		return c.cfg.IndependentCollector
	}
	reader, ok := c.proofs[concurrentReader]
	return ok && proofFresh(reader, now)
}

// admissionFreshLocked must be called with c.mu held. The authority adapter
// performs only a local, in-memory check and never calls back into c.
func (c *concurrentController) admissionFreshLocked() bool {
	if c.authorization != nil {
		return c.authorization.Check() == nil
	}
	return c.proofsFreshLocked(c.now())
}

func (c *concurrentController) Claim(ctx context.Context, ticket concurrentTicket) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	ctx, cancel := c.operationContext(ctx, concurrentCleanupTimeout)
	defer cancel()
	c.mu.Lock()
	if c.admissionOpen && !c.admissionFreshLocked() {
		c.mu.Unlock()
		c.invalidate("proof-expired")
		return errConcurrentProof
	}
	if !c.ticketCurrentLocked(ticket) {
		c.mu.Unlock()
		return errConcurrentStale
	}
	collector := *c.collector
	reader, readerActive := c.reader, !c.readerRetired
	var readerOwner concurrentOwner
	if reader != nil {
		readerOwner = *reader
	}
	c.mu.Unlock()
	if err := c.checkOwnerCurrent(ctx, collector); err != nil {
		c.invalidate("collector-owner-drift")
		c.markCleanupFailed()
		return errConcurrentStale
	}
	if readerActive {
		if reader == nil || c.checkOwnerCurrent(ctx, readerOwner) != nil {
			c.invalidate("reader-owner-drift")
			c.markCleanupFailed()
			return errConcurrentStale
		}
	}
	c.mu.Lock()
	work := c.work[ticket.WorkID]
	if c.admissionOpen && !c.admissionFreshLocked() {
		c.mu.Unlock()
		c.invalidate("proof-expired")
		return errConcurrentProof
	}
	if ctx.Err() != nil || !c.ticketCurrentLocked(ticket) || work == nil || work.state != concurrentWorkQueued {
		c.mu.Unlock()
		return errConcurrentStale
	}
	if c.authorization != nil {
		authorityTicket := concurrentAuthorizationTicket{ID: ticket.authorizationTicketID, EvidenceVersion: ticket.EvidenceVersion, WorkID: ticket.WorkID}
		err := c.authorization.Claim(authorityTicket, func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !c.ticketCurrentLocked(ticket) || work.state != concurrentWorkQueued {
				return errConcurrentStale
			}
			work.state = concurrentWorkActive
			return nil
		})
		c.mu.Unlock()
		if err != nil {
			return errors.Join(errConcurrentProof, err)
		}
		return nil
	}
	work.state = concurrentWorkActive
	c.mu.Unlock()
	return nil
}

func (c *concurrentController) ticketCurrentLocked(ticket concurrentTicket) bool {
	if !c.admissionOpen || c.parent.Err() != nil || c.pendingRevocations > 0 || c.continuationBlocked || c.collector == nil || ticket.ControllerID != c.controllerID || ticket.AttemptID != c.cfg.AttemptID ||
		ticket.Epoch != c.epoch || ticket.PairGeneration != c.pairGeneration {
		return false
	}
	if c.authorization != nil && (ticket.authorizationTicketID == "" || ticket.EvidenceVersion == 0) {
		return false
	}
	owner := *c.collector
	return ticket.CollectorID == owner.ID && ticket.CollectorGeneration == owner.Generation && ticket.CollectorPID == owner.PID &&
		ticket.CollectorCreation == owner.CreationIdentity && ticket.CollectorDriver == owner.Driver
}

// Complete writes the sink and marks work complete under the same mutex used
// by Revoke. Therefore either a result commits before revocation or is rejected.
func (c *concurrentController) Complete(ctx context.Context, ticket concurrentTicket, result string) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	if !isSyntheticID(result) {
		return errConcurrentUnavailable
	}
	ctx, cancel := c.operationContext(ctx, concurrentCleanupTimeout)
	defer cancel()
	c.mu.Lock()
	if c.admissionOpen && !c.admissionFreshLocked() {
		c.mu.Unlock()
		c.invalidate("proof-expired")
		return errConcurrentProof
	}
	if !c.ticketCurrentLocked(ticket) {
		c.mu.Unlock()
		return errConcurrentStale
	}
	collector := *c.collector
	readerActive := !c.readerRetired
	var reader concurrentOwner
	if c.reader != nil {
		reader = *c.reader
	}
	c.mu.Unlock()
	if c.checkOwnerCurrent(ctx, collector) != nil {
		c.invalidate("collector-owner-drift")
		c.markCleanupFailed()
		return errConcurrentStale
	}
	if readerActive && c.checkOwnerCurrent(ctx, reader) != nil {
		c.invalidate("reader-owner-drift")
		c.markCleanupFailed()
		return errConcurrentStale
	}
	c.mu.Lock()
	work := c.work[ticket.WorkID]
	if c.admissionOpen && !c.admissionFreshLocked() {
		c.mu.Unlock()
		c.invalidate("proof-expired")
		return errConcurrentProof
	}
	if ctx.Err() != nil || !c.ticketCurrentLocked(ticket) || work == nil || work.state != concurrentWorkActive {
		c.mu.Unlock()
		return errConcurrentStale
	}
	if c.authorization != nil {
		authorityTicket := concurrentAuthorizationTicket{ID: ticket.authorizationTicketID, EvidenceVersion: ticket.EvidenceVersion, WorkID: ticket.WorkID}
		err := c.authorization.Complete(authorityTicket, result, func() error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !c.ticketCurrentLocked(ticket) || work.state != concurrentWorkActive {
				return errConcurrentStale
			}
			c.sink[ticket.WorkID] = result
			work.state = concurrentWorkComplete
			return nil
		})
		c.mu.Unlock()
		if err != nil {
			return errors.Join(errConcurrentProof, err)
		}
		return nil
	}
	c.sink[ticket.WorkID] = result
	work.state = concurrentWorkComplete
	c.mu.Unlock()
	return nil
}

func (c *concurrentController) Cancel(ticket concurrentTicket) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	work := c.work[ticket.WorkID]
	if work == nil || work.state == concurrentWorkComplete || work.state == concurrentWorkCancelled {
		return errConcurrentStale
	}
	if ticket.ControllerID != c.controllerID || ticket.AttemptID != c.cfg.AttemptID || ticket.Epoch != c.epoch ||
		ticket.PairGeneration != c.pairGeneration || c.collector == nil || !c.ticketMatchesOwnerLocked(ticket) {
		return errConcurrentStale
	}
	if c.authorization != nil {
		authorityTicket := concurrentAuthorizationTicket{ID: ticket.authorizationTicketID, EvidenceVersion: ticket.EvidenceVersion, WorkID: ticket.WorkID}
		err := c.authorization.Cancel(authorityTicket, func() error {
			if work.state == concurrentWorkComplete || work.state == concurrentWorkCancelled {
				return errConcurrentStale
			}
			work.state = concurrentWorkCancelled
			return nil
		})
		if err != nil {
			return errors.Join(errConcurrentProof, err)
		}
		return nil
	}
	work.state = concurrentWorkCancelled
	return nil
}

func (c *concurrentController) ticketMatchesOwnerLocked(ticket concurrentTicket) bool {
	owner := *c.collector
	return ticket.CollectorID == owner.ID && ticket.CollectorGeneration == owner.Generation && ticket.CollectorPID == owner.PID &&
		ticket.CollectorCreation == owner.CreationIdentity && ticket.CollectorDriver == owner.Driver
}

func (c *concurrentController) authorizationWindowsLocked() []string {
	windows := make([]string, 0, len(c.readerWindows))
	for id := range c.readerWindows {
		windows = append(windows, id)
	}
	sort.Strings(windows)
	return windows
}

// OpenReaderWindow changes only the tracked reader inventory. It never calls a
// collector lifecycle method, so an active capture lease remains intact.
func (c *concurrentController) OpenReaderWindow(ctx context.Context, id, target string, bootstrap bool) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	if !isSyntheticID(id) || !isSyntheticID(target) {
		return errConcurrentUnavailable
	}
	ctx, cancel := c.operationContext(ctx, concurrentCleanupTimeout)
	defer cancel()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	if c.admissionOpen && !c.admissionFreshLocked() {
		c.mu.Unlock()
		c.invalidate("proof-expired")
		return errConcurrentProof
	}
	if !c.admissionOpen || c.pendingRevocations > 0 || c.continuationBlocked || c.reader == nil || c.readerRetired {
		c.mu.Unlock()
		return errConcurrentUnavailable
	}
	if _, exists := c.readerWindows[id]; exists {
		c.mu.Unlock()
		return errConcurrentUnavailable
	}
	epoch, pairGeneration := c.epoch, c.pairGeneration
	reader := *c.reader
	c.mu.Unlock()
	if err := c.checkOwnerCurrent(ctx, reader); err != nil {
		c.invalidate("reader-owner-drift")
		c.markCleanupFailed()
		return errConcurrentStale
	}
	if err := c.ports.owners.OpenReaderWindow(ctx, reader, id, target, bootstrap); err != nil {
		return err
	}
	c.mu.Lock()
	if ctx.Err() != nil || c.parent.Err() != nil || c.pendingRevocations > 0 || c.continuationBlocked || !c.admissionOpen || c.epoch != epoch || c.pairGeneration != pairGeneration || c.reader == nil || !sameOwner(*c.reader, reader) {
		c.mu.Unlock()
		_ = c.ports.owners.CloseReaderWindow(ctx, reader, id)
		return errConcurrentStale
	}
	c.readerWindows[id] = target
	if c.authorization != nil {
		if err := c.authorization.UpdateReaderWindows(c.authorizationWindowsLocked()); err != nil {
			delete(c.readerWindows, id)
			c.mu.Unlock()
			_ = c.ports.owners.CloseReaderWindow(ctx, reader, id)
			c.invalidate("reader-inventory-update-failed")
			return errConcurrentProof
		}
	}
	c.mu.Unlock()
	return nil
}

func (c *concurrentController) CloseReaderWindow(ctx context.Context, id string) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	ctx, cancel := c.operationContext(ctx, concurrentCleanupTimeout)
	defer cancel()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.mu.Lock()
	reader := c.reader
	_, exists := c.readerWindows[id]
	if !exists || reader == nil {
		c.mu.Unlock()
		return errConcurrentUnavailable
	}
	owner := *reader
	last := len(c.readerWindows) == 1
	c.mu.Unlock()
	if last {
		c.invalidate("last-reader-closing")
	}
	if err := c.ports.owners.CloseReaderWindow(ctx, owner, id); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.readerWindows, id)
	if !last && c.authorization != nil {
		if err := c.authorization.UpdateReaderWindows(c.authorizationWindowsLocked()); err != nil {
			c.mu.Unlock()
			c.invalidate("reader-inventory-update-failed")
			return errConcurrentProof
		}
	}
	c.mu.Unlock()
	if !last {
		return nil
	}
	empty, err := c.ports.owners.ReaderInventoryEmpty(ctx, owner)
	if err != nil || !empty {
		c.invalidate("reader-inventory-not-empty")
		return errConcurrentCleanup
	}
	if err := c.closeRole(ctx, concurrentReader, owner); err != nil {
		c.invalidate("reader-cleanup-failed")
		return err
	}
	c.mu.Lock()
	c.reader = nil
	c.readerRetired = true
	c.mu.Unlock()
	return c.readerRetiredDecision(ctx)
}

func (c *concurrentController) readerRetiredDecision(ctx context.Context) error {
	c.mu.Lock()
	if c.cleanupFailed || c.pendingRevocations > 0 {
		c.mu.Unlock()
		return errConcurrentCleanup
	}
	if c.continuationBlocked {
		c.mu.Unlock()
		return errConcurrentUnavailable
	}
	if c.disabled {
		c.updateCleanupStateLocked()
		c.mu.Unlock()
		return nil
	}
	if c.authorization != nil {
		authorization := c.authorization
		c.mu.Unlock()
		return authorization.RequestIndependentCollector()
	}
	if !c.cfg.IndependentCollector || c.collector == nil {
		c.mu.Unlock()
		c.invalidate("reader-retired-without-independent-authority")
		return errConcurrentReaderRetained
	}
	epoch, pairGeneration := c.epoch, c.pairGeneration
	collector := *c.collector
	c.mu.Unlock()
	started := c.now()
	bindingCtx, cancel := context.WithTimeout(ctx, concurrentBindingDeadline)
	defer cancel()
	proof, err := c.prove(bindingCtx, epoch, pairGeneration, concurrentCollector, collector)
	if err != nil || bindingCtx.Err() != nil || c.now().Sub(started) > concurrentBindingDeadline || c.checkOwnerCurrent(bindingCtx, collector) != nil {
		c.invalidate("independent-collector-proof-failed")
		return errConcurrentProof
	}
	permit, err := c.ports.permit.Grant(bindingCtx, concurrentPermitRequest{
		Provider: concurrentProvider, ControllerID: c.controllerID, AttemptID: c.cfg.AttemptID, Epoch: epoch, PairGeneration: pairGeneration,
		AccountID: c.cfg.ExpectedAccountID, Revision: c.cfg.ExpectedRevision, Mode: "independent-collector",
		Owners: []concurrentOwner{collector},
	})
	c.rememberPermit(permit, epoch)
	if err != nil || !permit.Granted || !isSyntheticID(permit.ID) || !proofFresh(proof, c.now()) || bindingCtx.Err() != nil || c.now().Sub(started) > concurrentBindingDeadline {
		c.invalidate("independent-collector-permit-failed")
		return errConcurrentProof
	}
	if c.checkOwnerCurrent(bindingCtx, collector) != nil {
		c.invalidate("independent-collector-owner-drift")
		return errConcurrentStale
	}
	c.mu.Lock()
	if c.epoch != epoch || c.pairGeneration != pairGeneration || c.disabled || c.cleanupFailed || c.pendingRevocations > 0 || c.continuationBlocked || c.parent.Err() != nil || bindingCtx.Err() != nil || !proofFresh(proof, c.now()) {
		c.mu.Unlock()
		c.invalidate("independent-collector-stale-after-grant")
		return errConcurrentStale
	}
	c.proofs = map[concurrentRole]concurrentProof{concurrentCollector: proof}
	c.admissionOpen = true
	c.revoked = false
	c.lifecycle = concurrentRunning
	c.mu.Unlock()
	return nil
}

// Disable revokes first, then closes only the collector. Existing reader
// windows remain available for user closure; no replacement owner is started.
func (c *concurrentController) Disable(ctx context.Context) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	ctx, cancel := c.operationContext(ctx, concurrentCleanupTimeout)
	defer cancel()
	c.opMu.Lock()
	defer c.opMu.Unlock()
	revokeErr := c.invalidate("disabled")
	c.mu.Lock()
	c.disabled = true
	collector := c.collector
	reader := c.reader
	c.lifecycle = concurrentDisabledReaderRetained
	c.mu.Unlock()
	if collector != nil {
		if err := c.closeRole(ctx, concurrentCollector, *collector); err != nil {
			c.markCleanupFailed()
			return err
		}
		c.mu.Lock()
		c.collector = nil
		c.mu.Unlock()
	}
	if reader != nil && c.readerWindowCount() == 0 {
		if empty, err := c.ports.owners.ReaderInventoryEmpty(ctx, *reader); err != nil || !empty {
			c.markCleanupFailed()
			return errConcurrentCleanup
		}
		if err := c.closeRole(ctx, concurrentReader, *reader); err != nil {
			c.markCleanupFailed()
			return err
		}
		c.mu.Lock()
		c.reader = nil
		c.readerRetired = true
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.updateCleanupStateLocked()
	c.mu.Unlock()
	if revokeErr != nil {
		return errors.Join(revokeErr, errConcurrentCleanup)
	}
	return nil
}

func (c *concurrentController) readerWindowCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.readerWindows)
}

func (c *concurrentController) closeRole(ctx context.Context, role concurrentRole, owner concurrentOwner) error {
	current, exists, err := c.ports.owners.CurrentOwner(ctx, role)
	if err != nil {
		return errConcurrentCleanup
	}
	if exists {
		if !sameOwner(current, owner) {
			return errConcurrentCleanup
		}
		if err := c.ports.owners.CloseOwner(ctx, current); err != nil {
			return errConcurrentCleanup
		}
	}
	absent, err := c.ports.owners.OwnerAbsent(ctx, role, owner.ID)
	if err != nil || !absent {
		return errConcurrentCleanup
	}
	return nil
}

func (c *concurrentController) CollectorLost() {
	if !c.enabled {
		return
	}
	c.invalidate("collector-lost")
	c.mu.Lock()
	if !c.cleanupFailed {
		c.lifecycle = concurrentCollectorLost
	}
	c.mu.Unlock()
}

func (c *concurrentController) Revoke(reason string) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	c.continuationBlocked = true
	c.mu.Unlock()
	c.invalidate(reason)
}

// PolicyChanged fences the selected authority synchronously. Policy values
// are scoped fixture inputs and never written to a Store or external service.
func (c *concurrentController) PolicyChanged(policyGeneration, holdGeneration uint64, enabled bool) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	c.continuationBlocked = true
	if c.authorization != nil {
		c.authorization.PolicyChanged(policyGeneration, holdGeneration, enabled)
	}
	c.mu.Unlock()
	c.invalidate("policy-changed")
}

// OwnerChanged is the explicit owner-fence notification used by the private
// fixture seam. It never closes or replaces an owner itself.
func (c *concurrentController) OwnerChanged(role concurrentRole) {
	if !c.enabled {
		return
	}
	c.mu.Lock()
	c.continuationBlocked = true
	if c.authorization != nil {
		c.authorization.OwnerChanged(role)
	}
	c.mu.Unlock()
	c.invalidate("owner-changed")
}

// RenewEvidence advances the selected pair authority without holding opMu or
// c.mu during evidence acquisition. The validator runs after collection and
// before the authority publishes its next evidence version.
func (c *concurrentController) RenewEvidence(ctx context.Context) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	if c.authorization == nil {
		return errConcurrentUnavailable
	}
	renewCtx, cancel := c.operationContext(ctx, concurrentBindingDeadline)
	defer cancel()
	c.mu.Lock()
	if !c.admissionOpen || c.disabled || c.cleanupFailed || c.revoked || c.parent.Err() != nil || c.reader == nil || c.collector == nil || c.pendingRevocations > 0 {
		c.mu.Unlock()
		return errConcurrentUnavailable
	}
	if c.authorization.Check() != nil {
		c.mu.Unlock()
		c.invalidate("authority-expired-before-renewal")
		return errConcurrentProof
	}
	epoch, pairGeneration := c.epoch, c.pairGeneration
	reader, collector := *c.reader, *c.collector
	c.mu.Unlock()
	binding := c.authorizationBinding(epoch, pairGeneration, reader, collector)

	validate := func(checkCtx context.Context) error {
		if err := checkCtx.Err(); err != nil {
			return err
		}
		if err := c.checkOwnerCurrent(checkCtx, reader); err != nil {
			return errConcurrentStale
		}
		if err := c.checkOwnerCurrent(checkCtx, collector); err != nil {
			return errConcurrentStale
		}
		c.mu.Lock()
		valid := c.admissionOpen && !c.disabled && !c.cleanupFailed && !c.revoked && c.parent.Err() == nil && c.pendingRevocations == 0 &&
			c.epoch == epoch && c.pairGeneration == pairGeneration && c.reader != nil && c.collector != nil &&
			sameOwner(*c.reader, reader) && sameOwner(*c.collector, collector)
		c.mu.Unlock()
		if !valid {
			return errConcurrentStale
		}
		return nil
	}
	if err := validate(renewCtx); err != nil {
		c.invalidate("owner-or-controller-drift-before-renewal")
		return errors.Join(errConcurrentStale, err)
	}
	if err := c.authorization.RenewEvidence(renewCtx, binding, validate); err != nil {
		c.invalidate("evidence-renewal-failed")
		return errors.Join(errConcurrentProof, err)
	}
	if err := validate(renewCtx); err != nil {
		c.invalidate("owner-or-controller-drift-after-renewal")
		return errors.Join(errConcurrentStale, err)
	}
	c.mu.Lock()
	valid := c.admissionOpen && c.epoch == epoch && c.pairGeneration == pairGeneration && c.parent.Err() == nil && c.authorization.Check() == nil
	c.mu.Unlock()
	if !valid {
		c.invalidate("authority-changed-after-renewal")
		return errConcurrentStale
	}
	return nil
}

func (c *concurrentController) rememberPermit(permit concurrentPermit, epoch uint64) {
	if !permit.Granted || permit.ID == "" {
		return
	}
	c.mu.Lock()
	c.permit = permit
	c.permitOpen = true
	c.permitEpoch = epoch
	c.mu.Unlock()
}

// invalidate is the admission linearization point. The legacy external permit
// port runs outside mu; the optional authority is strictly local and non-reentrant.
// Active work is cancelled and queued work remains pending.
func (c *concurrentController) invalidate(reason string) error {
	c.mu.Lock()
	wasOpen, permit, epoch := c.invalidateLocked(reason)
	c.mu.Unlock()
	if wasOpen {
		revokeCtx, cancel := context.WithTimeout(context.Background(), concurrentCleanupTimeout)
		err := c.ports.permit.Revoke(revokeCtx, permit.ID, epoch)
		cancel()
		c.mu.Lock()
		c.pendingRevocations--
		if c.pendingRevocations == 0 && c.pendingRevocationsDone != nil {
			close(c.pendingRevocationsDone)
			c.pendingRevocationsDone = nil
		}
		if err != nil {
			c.markCleanupFailedLocked()
		}
		c.mu.Unlock()
		if err != nil {
			return errors.Join(errConcurrentCleanup, err)
		}
	}
	return nil
}

// invalidateLocked is shared with snapshots that discover local expiry. It
// revokes the optional authority while preserving c.mu -> authority lock
// ordering; the authority implementation must not callback into c.
func (c *concurrentController) invalidateLocked(reason string) (bool, concurrentPermit, uint64) {
	if c.authorization != nil {
		c.authorization.Revoke(reason)
	}
	wasOpen := c.permitOpen && c.ports.permit != nil
	permit := c.permit
	epoch := c.permitEpoch
	if wasOpen {
		if c.pendingRevocations == 0 {
			c.pendingRevocationsDone = make(chan struct{})
		}
		c.pendingRevocations++
	}
	c.admissionOpen = false
	c.permitOpen = false
	c.permit = concurrentPermit{}
	c.permitEpoch = 0
	c.epoch++
	c.pairGeneration++
	c.revoked = true
	for _, work := range c.work {
		if work.state == concurrentWorkActive {
			work.state = concurrentWorkCancelled
		}
	}
	if !c.disabled && c.lifecycle != concurrentCleanupFailed && c.lifecycle != concurrentCollectorLost {
		c.lifecycle = concurrentStarting
	}
	return wasOpen, permit, epoch
}

func (c *concurrentController) failStart(cause error) error {
	revokeErr := c.invalidate("startup-failed")
	cleanupCtx, cancel := context.WithTimeout(context.Background(), concurrentCleanupTimeout)
	err := c.closeOwnedTrees(cleanupCtx)
	cancel()
	if err != nil || revokeErr != nil {
		return errors.Join(cause, err, revokeErr)
	}
	return cause
}

func (c *concurrentController) closeOwnedTrees(ctx context.Context) error {
	c.mu.Lock()
	reader := c.reader
	collector := c.collector
	c.mu.Unlock()
	var errs []error
	if reader != nil && collector != nil && !distinctOwnerPair(*reader, *collector) {
		c.markCleanupFailed()
		errs = append(errs, errConcurrentCleanup)
		reader, collector = nil, nil
	}
	if collector != nil {
		if err := c.closeRole(ctx, concurrentCollector, *collector); err != nil {
			errs = append(errs, err)
		} else {
			c.mu.Lock()
			c.collector = nil
			c.mu.Unlock()
		}
	}
	if reader != nil {
		if err := c.closeRole(ctx, concurrentReader, *reader); err != nil {
			errs = append(errs, err)
		} else {
			c.mu.Lock()
			c.reader = nil
			c.readerRetired = true
			c.readerWindows = make(map[string]string)
			c.mu.Unlock()
		}
	}
	c.mu.Lock()
	if len(errs) > 0 || c.pendingRevocations > 0 {
		c.markCleanupFailedLocked()
		if c.pendingRevocations > 0 {
			errs = append(errs, errConcurrentCleanup)
		}
	} else if c.cleanupFailed {
		c.lifecycle = concurrentCleanupFailed
		errs = append(errs, errConcurrentCleanup)
	} else if c.reader == nil && c.collector == nil {
		c.lifecycle = concurrentCleanupComplete
	}
	c.mu.Unlock()
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (c *concurrentController) markCleanupFailed() {
	c.mu.Lock()
	c.markCleanupFailedLocked()
	c.mu.Unlock()
}

func (c *concurrentController) markCleanupFailedLocked() {
	if c.authorization != nil {
		c.authorization.Revoke("cleanup-failed")
	}
	c.cleanupFailed = true
	c.admissionOpen = false
	c.continuationBlocked = true
	c.epoch++
	c.pairGeneration++
	c.revoked = true
	for _, work := range c.work {
		if work.state == concurrentWorkActive {
			work.state = concurrentWorkCancelled
		}
	}
	c.lifecycle = concurrentCleanupFailed
}

func distinctOwnerPair(reader, collector concurrentOwner) bool {
	return reader.ID != collector.ID && reader.PID != collector.PID && reader.CreationIdentity != collector.CreationIdentity &&
		reader.ProfileID != collector.ProfileID && reader.DirectoryID != collector.DirectoryID &&
		reader.Role == concurrentReader && collector.Role == concurrentCollector
}

func (c *concurrentController) updateCleanupStateLocked() {
	if c.pendingRevocations > 0 {
		c.markCleanupFailedLocked()
		return
	}
	if c.cleanupFailed {
		c.lifecycle = concurrentCleanupFailed
		return
	}
	if c.reader == nil && c.collector == nil && len(c.readerWindows) == 0 {
		c.lifecycle = concurrentCleanupComplete
		return
	}
	if c.disabled && len(c.readerWindows) > 0 {
		c.lifecycle = concurrentDisabledReaderRetained
	}
}

func (c *concurrentController) Snapshot() concurrentSnapshot {
	c.mu.Lock()
	authorization, admission := c.authorization, c.admissionOpen
	c.mu.Unlock()
	if authorization != nil && admission && authorization.Check() != nil {
		c.invalidate("authorization-expired")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	snapshot := concurrentSnapshot{
		Enabled: c.enabled, AdmissionOpen: c.admissionOpen, PendingRevocations: c.pendingRevocations, Epoch: c.epoch, PairGeneration: c.pairGeneration,
		Lifecycle: c.lifecycle, ReaderWindows: len(c.readerWindows),
	}
	for _, work := range c.work {
		switch work.state {
		case concurrentWorkQueued:
			snapshot.Queued++
		case concurrentWorkActive:
			snapshot.Active++
		}
	}
	if c.reader != nil {
		snapshot.Reader, snapshot.HasReader = *c.reader, true
	}
	if c.collector != nil {
		snapshot.Collector, snapshot.HasCollector = *c.collector, true
	}
	return snapshot
}

func (c *concurrentController) SinkResult(workID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result, ok := c.sink[workID]
	return result, ok
}

func (c *concurrentController) WorkState(workID string) (concurrentWorkState, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	work, ok := c.work[workID]
	if !ok {
		return concurrentWorkQueued, false
	}
	return work.state, true
}

// WaitParentCleanup waits until the parent-cancellation handler has verified
// cleanup of both owned fixture trees. It does not claim OS Job semantics.
func (c *concurrentController) WaitParentCleanup(ctx context.Context) error {
	if !c.enabled {
		return errConcurrentDisabled
	}
	select {
	case <-c.parentDone:
		if c.Snapshot().Lifecycle == concurrentCleanupFailed {
			return errConcurrentCleanup
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *concurrentController) waitRevocations(ctx context.Context) error {
	c.mu.Lock()
	done := c.pendingRevocationsDone
	if c.pendingRevocations == 0 {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
