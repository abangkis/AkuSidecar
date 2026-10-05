package collection

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type RuntimeStatus struct {
	Available                bool                 `json:"available"`
	Requested                string               `json:"requested"`
	Effective                string               `json:"effective"`
	Pending                  bool                 `json:"pending"`
	State                    captureruntime.State `json:"state"`
	Failure                  string               `json:"failure,omitempty"`
	Generation               uint64               `json:"generation"`
	ActiveLeases             int                  `json:"activeLeases"`
	HeadlessAvailable        bool                 `json:"headlessAvailable"`
	QuietAvailable           bool                 `json:"quietAvailable"`
	SupportedSources         []domain.Source      `json:"supportedSources"`
	AuthorizedSources        []domain.Source      `json:"authorizedSources,omitempty"`
	NativeReaderOnly         bool                 `json:"nativeReaderOnly,omitempty"`
	NativeReaderBlockedSince string               `json:"nativeReaderBlockedSince,omitempty"`
	CollectionBorrowSource   domain.Source        `json:"collectionBorrowSource,omitempty"`
	CollectionBorrowFailure  string               `json:"collectionBorrowFailure,omitempty"`
}

const (
	BackendBridge   = "bridge"
	BackendQuiet    = "browser_quiet_hidden"
	BackendHeadless = "headless"
)

type CaptureBackend interface {
	Capture(context.Context, domain.Source, map[string]any) (domain.Observation, error)
}

// Implemented opt-in collectors; this is capability, not source parity certification.
func HeadlessSourceSupported(source domain.Source) bool {
	return source == domain.SourceX || source == domain.SourceFacebook || source == domain.SourceInstagram || source == domain.SourceLinkedIn
}

// CaptureAvailability is an optional, nonblocking capability for backends
// whose readiness can change after they are bound to a browser generation.
type CaptureAvailability interface {
	CaptureAvailable() bool
}

func captureBackendAvailable(backend CaptureBackend) bool {
	if backend == nil {
		return false
	}
	if readiness, ok := backend.(CaptureAvailability); ok {
		return readiness.CaptureAvailable()
	}
	return true
}

type Coordinator struct {
	mu                        sync.Mutex
	owner                     *captureruntime.Manager
	requested                 string
	interactive               int
	nativeReaders             int
	nativeReaderGeneration    uint64
	nativeReaderSince         time.Time
	browserCollection         int
	browserCollectionFailures map[string]string
	launch                    func(context.Context, string, uint64) (captureruntime.Process, error)
	validate                  func() error
	readiness                 func() error
	headless                  *headless.Process
	browserCollector          CaptureBackend
	browserGeneration         uint64
	failure                   string
	retry                     bool
	headlessAvailable         bool
	wake                      chan struct{}
	wakeAt                    time.Time
	timingObserver            func(string, time.Duration)
}

// Bind once for the initial browser owner and again from its replacement
// factory. Availability also checks the manager generation; a prepared next
// backend must never service a command pinned to the previous owner.
func (c *Coordinator) SetBrowserCollector(generation uint64, backend CaptureBackend) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.browserGeneration, c.browserCollector = generation, backend
}

func (c *Coordinator) BrowserCollectorAvailable(source domain.Source) bool {
	c.mu.Lock()
	backend, generation := c.browserCollector, c.browserGeneration
	c.mu.Unlock()
	s := c.owner.Snapshot()
	return (source == "x" || source == "facebook") && captureBackendAvailable(backend) && generation == s.Generation && s.State == captureruntime.Ready && s.Driver == "browser"
}

func NewCoordinator(owner *captureruntime.Manager, launch func(context.Context, string, uint64) (captureruntime.Process, error), validate func() error) *Coordinator {
	return &Coordinator{owner: owner, requested: "browser", launch: launch, validate: validate, headlessAvailable: validate() == nil, wake: make(chan struct{}, 1)}
}

// Bind before Start. Intent changes wake the single reconciliation loop;
// the timer remains a fallback for lease/process lifecycle changes.
func (c *Coordinator) SetTimingObserver(observer func(string, time.Duration)) {
	c.timingObserver = observer
}
func (c *Coordinator) notify() {
	c.mu.Lock()
	if c.wakeAt.IsZero() {
		c.wakeAt = time.Now()
	}
	c.mu.Unlock()
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// Bound before Start: retained permissions must be confirmed before handoff.
func (c *Coordinator) SetHeadlessReadiness(check func() error) { c.readiness = check }
func (c *Coordinator) ValidateSelection(mode string, sources []domain.Source) error {
	if mode != "browser" && mode != "headless" {
		return errors.New("unsupported collection mode")
	}
	if mode == "headless" {
		if err := c.validate(); err != nil {
			return err
		}
		for _, source := range sources {
			if !HeadlessSourceSupported(source) {
				return errors.New("selected source has no headless collector")
			}
		}
	}
	return nil
}
func (c *Coordinator) Request(mode string) {
	c.mu.Lock()
	c.requested = mode
	c.retry = true
	c.failure = ""
	c.mu.Unlock()
	c.notify()
}

// Retry wakes reconciliation without changing the user's selected mode.
func (c *Coordinator) Retry() {
	c.mu.Lock()
	c.retry = true
	c.failure = ""
	c.mu.Unlock()
	c.notify()
}
func (c *Coordinator) Status() RuntimeStatus {
	c.mu.Lock()
	requested, interactive, collectionBorrow, failure, available := c.requested, c.interactive, c.browserCollection, c.failure, c.headlessAvailable
	nativeGeneration := c.nativeReaderGeneration
	nativeSince := c.nativeReaderSince
	backend, generation := c.browserCollector, c.browserGeneration
	var collectionFailure string
	if len(c.browserCollectionFailures) > 0 {
		ids := make([]string, 0, len(c.browserCollectionFailures))
		for id := range c.browserCollectionFailures {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		collectionFailure = c.browserCollectionFailures[ids[0]]
	}
	c.mu.Unlock()
	s := c.owner.Snapshot()
	effective := s.Driver
	if s.State != captureruntime.Ready {
		effective = ""
	}
	quietAvailable := captureBackendAvailable(backend) && generation == s.Generation && effective == "browser"
	status := RuntimeStatus{Available: true, Requested: requested, Effective: effective, Pending: requested != effective || interactive > 0 || collectionBorrow > 0, State: s.State, Failure: failure, Generation: s.Generation, ActiveLeases: s.ActiveLeases, HeadlessAvailable: available, QuietAvailable: quietAvailable, SupportedSources: []domain.Source{domain.SourceX, domain.SourceFacebook, domain.SourceInstagram, domain.SourceLinkedIn}}
	status.NativeReaderOnly = nativeGeneration != 0 && nativeGeneration == s.Generation
	if status.NativeReaderOnly {
		status.Pending = true
		if !nativeSince.IsZero() {
			status.NativeReaderBlockedSince = nativeSince.UTC().Format(time.RFC3339Nano)
		}
	}
	if collectionBorrow > 0 {
		status.CollectionBorrowSource = domain.SourceFacebook
		status.CollectionBorrowFailure = collectionFailure
	}
	return status
}

// Each collection lease keeps its own cleanup error. One successful release
// cannot hide another outstanding cleanup failure or release profile ownership.
func (c *Coordinator) SetBrowserCollectionFailure(leaseID, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if message == "" {
		delete(c.browserCollectionFailures, leaseID)
		return
	}
	if c.browserCollection == 0 {
		return
	}
	if c.browserCollectionFailures == nil {
		c.browserCollectionFailures = map[string]string{}
	}
	c.browserCollectionFailures[leaseID] = message
}
func (c *Coordinator) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.reconcile(ctx)
			case <-c.wake:
				c.reconcile(ctx)
			}
		}
	}()
}
func (c *Coordinator) reconcile(parent context.Context) {
	c.mu.Lock()
	wakeAt := c.wakeAt
	c.wakeAt = time.Time{}
	mode := c.requested
	if c.interactive > 0 || c.browserCollection > 0 {
		mode = "browser"
	}
	retry := c.retry
	nativeOnly := c.nativeReaderGeneration != 0 && c.nativeReaderGeneration == c.owner.Snapshot().Generation
	launchReader := c.requested == "headless" && c.nativeReaders > 0 && c.interactive == c.nativeReaders && c.browserCollection == 0
	c.mu.Unlock()
	if !wakeAt.IsZero() && c.timingObserver != nil {
		c.timingObserver("coordinator_wait", time.Since(wakeAt))
	}
	s := c.owner.Snapshot()
	retiring := c.owner.Retiring()
	if s.State == captureruntime.Ready && s.Driver == mode && !(nativeOnly && !launchReader) {
		return
	}
	if s.ActiveLeases > 0 {
		return
	}
	if (s.State == captureruntime.Blocked || s.State == captureruntime.Failed) && !retry && !retiring && !nativeOnly {
		return
	}
	// Validate assets before releasing a healthy authenticated owner.
	if mode == "headless" {
		if err := c.validate(); err != nil {
			c.mu.Lock()
			c.failure, c.retry = err.Error(), false
			c.mu.Unlock()
			return
		}
		if c.readiness != nil && !retiring {
			if err := c.readiness(); err != nil {
				c.mu.Lock()
				c.failure = err.Error()
				c.mu.Unlock()
				return
			}
		}
	}
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	factory := func(ctx context.Context, generation uint64) (captureruntime.Process, error) {
		launchMode := mode
		if mode == "browser" && launchReader {
			launchMode = "native_reader"
		}
		process, err := c.launch(ctx, launchMode, generation)
		if err == nil {
			c.mu.Lock()
			c.headless, _ = process.(*headless.Process)
			c.nativeReaderGeneration = 0
			c.nativeReaderSince = time.Time{}
			if launchMode == "native_reader" {
				c.nativeReaderGeneration = generation
				c.nativeReaderSince = time.Now().UTC()
			}
			c.mu.Unlock()
		}
		return process, err
	}
	var err error
	if s.State == captureruntime.Blocked || s.State == captureruntime.Failed {
		err = c.owner.Recover(ctx, factory)
	} else {
		err = c.owner.Replace(ctx, factory)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.failure = err.Error()
		c.retry = false
	} else {
		c.failure = ""
		c.retry = false
	}
}
func (c *Coordinator) BorrowBrowser(ctx context.Context) (*captureruntime.Lease, func(), error) {
	return c.borrowBrowser(ctx, false)
}

func (c *Coordinator) BorrowNativeReader(ctx context.Context) (*captureruntime.Lease, func(), error) {
	c.mu.Lock()
	c.nativeReaders++
	c.mu.Unlock()
	lease, release, err := c.borrowBrowserMode(ctx, false, true)
	var once sync.Once
	done := func() {
		once.Do(func() {
			if release != nil {
				release()
			}
			c.mu.Lock()
			c.nativeReaders--
			c.retry = true
			c.mu.Unlock()
		})
	}
	if err != nil {
		done()
		return nil, nil, err
	}
	return lease, done, nil
}

// A Facebook batch borrows the same exclusive profile without pretending to be
// a login/reader interaction. Its intent and process lease both prevent return.
func (c *Coordinator) BorrowBrowserCollection(ctx context.Context, source domain.Source) (*captureruntime.Lease, func(), error) {
	if source != domain.SourceFacebook {
		return nil, nil, errors.New("source has no browser collection exception")
	}
	return c.borrowBrowser(ctx, true)
}

// BeginBrowserCollection records an asynchronous collection intent. Admission
// must drain/release its previous session lease before waiting for replacement;
// acquiring a Browser lease while retaining a headless lease would deadlock.
func (c *Coordinator) BeginBrowserCollection(source domain.Source) (func(), error) {
	if source != domain.SourceFacebook {
		return nil, errors.New("source has no browser collection exception")
	}
	return c.beginBrowserBorrow(true), nil
}

func (c *Coordinator) borrowBrowser(ctx context.Context, collectionBorrow bool) (*captureruntime.Lease, func(), error) {
	return c.borrowBrowserMode(ctx, collectionBorrow, false)
}
func (c *Coordinator) borrowBrowserMode(ctx context.Context, collectionBorrow, nativeReader bool) (*captureruntime.Lease, func(), error) {
	releaseIntent := c.beginBrowserBorrow(collectionBorrow)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		s := c.owner.Snapshot()
		if s.State == captureruntime.Ready && s.Driver == "browser" {
			if c.Status().NativeReaderOnly && !nativeReader {
				releaseIntent()
				return nil, nil, errors.New("close native post windows before opening a source or collecting through Browser")
			}
			lease, err := c.owner.Acquire()
			if err != nil {
				releaseIntent()
				return nil, nil, err
			}
			if lease.Driver() != "browser" {
				lease.Release()
				continue
			}
			return lease, releaseIntent, nil
		}
		c.mu.Lock()
		failure := c.failure
		c.mu.Unlock()
		if failure != "" {
			releaseIntent()
			return nil, nil, errors.New(failure)
		}
		select {
		case <-ctx.Done():
			releaseIntent()
			return nil, nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *Coordinator) beginBrowserBorrow(collectionBorrow bool) func() {
	c.mu.Lock()
	if collectionBorrow {
		c.browserCollection++
	} else {
		c.interactive++
	}
	c.retry = true
	c.failure = ""
	c.mu.Unlock()
	c.notify()
	var once sync.Once
	releaseIntent := func() {
		once.Do(func() {
			c.mu.Lock()
			if collectionBorrow {
				c.browserCollection--
				if c.browserCollection == 0 {
					c.browserCollectionFailures = nil
				}
			} else {
				c.interactive--
			}
			c.mu.Unlock()
			c.notify()
		})
	}
	return releaseIntent
}
func (c *Coordinator) Capture(ctx context.Context, source domain.Source, payload map[string]any) (domain.Observation, error) {
	c.mu.Lock()
	s := c.owner.Snapshot()
	var process CaptureBackend
	if s.State == captureruntime.Ready && s.Driver == "headless" && c.headless != nil {
		process = c.headless
	} else if s.State == captureruntime.Ready && s.Driver == "browser" && c.browserGeneration == s.Generation && (source == "x" || source == "facebook") {
		process = c.browserCollector
	}
	c.mu.Unlock()
	if process == nil {
		return domain.Observation{}, errors.New("selected collection backend is unavailable")
	}
	return process.Capture(ctx, source, payload)
}
