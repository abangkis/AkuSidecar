package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/activationguard"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	collectionbridge "github.com/abangkis/AkuSidecar/internal/collection/bridge"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
	"github.com/abangkis/AkuSidecar/internal/livingtopics"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	ExpectedBridgeVersion       = "0.9.2"
	ExpectedBridgeRevision      = "source-adapters-v118"
	ExpectedBridgeID            = "aku-bridge-chrome-mv3-v0"
	BridgeProtocolMajor         = 2
	BridgeProtocolMinor         = 0
	MinimumBridgeProtocolMinor  = 0
	MaximumBridgeProtocolMinor  = 65535
	MaxBridgeUpdateCapabilities = 32
	MaxBridgeCapabilityLength   = 64
)

var expectedBridgeActions = []string{
	"probe_readiness", "probe_freshness", "recover_source_freshness",
	"collect_visible", "detect_pending_content", "report_adapter_health",
	"dispatch_background_commands",
	"report_capture_quality", "acquire_missing_media", "recapture_missing_media",
	"cache_passive_media_evidence", "lookup_passive_media_evidence", "observe_response_media_evidence", "extract_source_semantics",
	"report_frontier", "manage_source_tab_lifecycle", "manage_capture_window",
	"open_native_reader_window", "release_capture_surface", "preserve_working_tab", "report_source_events", "reload_self",
}

type Engine struct {
	nativeDerivationHold                     *activationguard.Guard
	store                                    *store.Store
	provider                                 reasoning.Provider
	config                                   config.Config
	epoch                                    string
	mu                                       sync.RWMutex
	operation                                sync.Mutex
	instagramReaderDiagnosticUsed            bool // guarded by operation
	instagramReaderDiagnosticMu              sync.RWMutex
	instagramReaderDiagnosticDone            chan struct{}         // guarded by mu
	instagramDiagnosticAttempt               *igdiagnostic.Attempt // latest admitted attempt; guarded by mu
	captureMu                                sync.Mutex
	captureOwner                             *captureruntime.Manager
	captureSessions                          map[string]*captureruntime.Lease
	captureRecaptures                        map[string]*captureruntime.Lease
	collectionIntents                        map[string]func()
	collectionCleanupPending                 map[string]bool
	collectionCleanupDone                    map[string]bool
	browserCollectionCleanup                 func(context.Context, string, uint64) error
	browserRecaptureDispatch                 func(context.Context, string, uint64) error
	browserRecaptureDispatchRunning          map[string]bool
	browserRecaptureDispatchStarted          map[string]bool
	browserRecaptureDispatchConfirmed        map[string]bool
	collectionRuntime                        *collection.Coordinator
	fixtureAdmission                         *fixtureAdmissionGate
	developmentSources                       map[domain.Source]bool
	developmentPause                         func(context.Context) error
	developmentVerifyBrowser                 func(context.Context) error
	developmentCheckpointMu                  sync.Mutex
	developmentCoordinatorDone               <-chan struct{}
	developmentCheckpoint                    *store.SyntheticAccountCheckpoint
	developmentCheckpointFence               *store.SyntheticCheckpointFence
	developmentCheckpointAuthorizedAtStartup bool
	developmentCheckpointAuthorize           func(context.Context, func() error) error
	developmentCheckpointRevoke              func(context.Context) error
	schedule                                 sync.Mutex
	heartbeat                                *domain.BridgeHeartbeat
	headlessAccess                           []domain.Source
	bridgeOrigins                            map[string]time.Time
	work                                     engineWorkState
	logger                                   Logger
	reloads                                  *ReloadActions
	events                                   *semanticengine.Engine
	aiFast                                   aidetector.FastDetector
	aiDeep                                   aidetector.Resolver
	mediaOrigin                              mediaprovenance.Inspector
	topics                                   livingtopics.Resolver
	scheduler                                autoUpdateLifecycle
	captureBehaviorMu                        sync.Mutex
	xManual                                  xManualRouting
	xDaily                                   xDailyRouting
	instagramDaily                           xDailyRouting
	captureWork                              map[string]context.CancelFunc
}

type Logger interface{ Printf(string, ...any) }

func New(state *store.Store, provider reasoning.Provider, cfg config.Config, logger Logger, eventEngines ...*semanticengine.Engine) *Engine {
	var events *semanticengine.Engine
	if len(eventEngines) > 0 {
		events = eventEngines[0]
	}
	return &Engine{store: state, provider: provider, config: cfg, epoch: domain.NewID("epoch"), work: newEngineWorkState(), bridgeOrigins: map[string]time.Time{}, logger: logger, reloads: NewReloadActions(15 * time.Second), events: events, scheduler: newAutoUpdateLifecycle()}
}
func (e *Engine) SetLivingTopicsResolver(value livingtopics.Resolver) { e.topics = value }
func (e *Engine) Epoch() string                                       { return e.epoch }
func (e *Engine) ProviderName() string                                { return e.provider.Name() }
func (e *Engine) Settings(ctx context.Context) (domain.Settings, error) {
	return e.store.GetSettings(ctx)
}
func (e *Engine) Onboarding(ctx context.Context) (domain.OnboardingState, error) {
	return e.store.Onboarding(ctx)
}
func (e *Engine) CompleteOnboarding(ctx context.Context, sources []domain.Source) (domain.OnboardingState, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	current, err := e.store.Onboarding(ctx)
	if err != nil {
		return domain.OnboardingState{}, err
	}
	if current.Status == "not_started" && len(e.config.Reasoning.Providers) > 0 {
		settings, err := e.store.GetSettings(ctx)
		if err != nil {
			return domain.OnboardingState{}, err
		}
		providerName := strings.TrimSpace(settings.ReasoningProvider)
		if providerName == "" {
			providerName = strings.TrimSpace(e.config.Reasoning.ActiveProvider)
		}
		if providerName != "" {
			if err := e.ensureReasoningProviderReady(ctx, providerName); err != nil {
				return domain.OnboardingState{}, err
			}
		}
	}
	return e.store.CompleteOnboarding(ctx, sources)
}

func (e *Engine) RecordHeartbeat(value domain.BridgeHeartbeat) BridgeStatus {
	value.ReceivedAt = domain.Now()
	e.mu.Lock()
	now := time.Now()
	for origin, seenAt := range e.bridgeOrigins {
		if now.Sub(seenAt) > 2*time.Minute {
			delete(e.bridgeOrigins, origin)
		}
	}
	if value.ExtensionOrigin != "" {
		e.bridgeOrigins[value.ExtensionOrigin] = now
	}
	e.heartbeat = &value
	e.mu.Unlock()
	status := e.BridgeStatus()
	e.mu.Lock()
	if e.heartbeat == &value {
		// A newer browser report can revoke retained headless authority. Never
		// wait for process rotation to apply that revocation.
		e.headlessAccess = headlessGrantedSources(status)
	}
	e.mu.Unlock()
	if err := e.recoverExpiredBridgeCommands(context.Background(), time.Now()); err != nil {
		e.logger.Printf("recover expired Bridge command: %v", err)
	}
	e.reloads.Observe(value)
	e.scheduler.notify()
	return e.BridgeStatus()
}

func (e *Engine) RequestBridgeReload(requestID string, actor any, reason string) (ReloadAction, error) {
	e.mu.RLock()
	heartbeat := e.heartbeat
	e.mu.RUnlock()
	previous := ""
	if heartbeat != nil {
		previous = heartbeat.BuildID
	}
	return e.reloads.Request(requestID, actor, reason, previous)
}
func (e *Engine) NextBridgeAction(wait time.Duration, done <-chan struct{}) (*ReloadAction, error) {
	return e.reloads.Next(wait, done)
}
func (e *Engine) AcceptBridgeAction(id string) (ReloadAction, error) { return e.reloads.Accept(id) }
func (e *Engine) BridgeAction(id string) (ReloadAction, error)       { return e.reloads.Get(id) }
func capturePayload(run domain.Run, leaseID string, settings domain.Settings, round int, continuation map[string]any, reason string) map[string]any {
	return (collectionbridge.Builder{}).Build(collection.Request{
		Source:               run.Source,
		LeaseID:              leaseID,
		HydrationTimeoutMS:   settings.SourceHydrationTimeout(run.Source),
		MaxScrolls:           settings.MaxScrolls,
		QualityRetrySettleMS: settings.QualityRetrySettleMS,
		OpenMissingSource:    settings.OpenMissingSource,
		Round:                round,
		Continuation:         continuation,
		FollowUpReason:       reason,
		Browser:              collection.BrowserOptions{Visibility: settings.CaptureVisibility},
	})
}

func (e *Engine) Session(ctx context.Context, id string) (domain.Session, error) {
	return e.store.GetSession(ctx, id)
}
func (e *Engine) ActiveSession(ctx context.Context) (*domain.Session, error) {
	return e.store.ActiveSession(ctx)
}
func (e *Engine) Run(ctx context.Context, id string) (domain.Run, error) {
	return e.store.GetRun(ctx, id)
}
func (e *Engine) Timeline(ctx context.Context, limit, offset int) ([]domain.TimelineItem, error) {
	return e.store.ListTimeline(ctx, limit, offset)
}
func (e *Engine) ContentContext(ctx context.Context, timelineID string, limit int) (domain.ContentContextResult, error) {
	return e.store.ContentContext(ctx, timelineID, limit)
}

func (e *Engine) AddContentContextFeedback(ctx context.Context, timelineID string, input domain.ContentContextFeedbackInput) (domain.ContentContextFeedbackEvent, error) {
	return e.store.AddContentContextFeedback(ctx, timelineID, input)
}

func (e *Engine) UndoContentContextFeedback(ctx context.Context, id string) (domain.ContentContextFeedbackEvent, error) {
	return e.store.UndoContentContextFeedback(ctx, id)
}
func (e *Engine) Library(ctx context.Context, query domain.MemoryLibraryQuery) (domain.MemoryLibraryResult, error) {
	return e.store.ListMemoryLibrary(ctx, query)
}
func (e *Engine) LibraryTopicKnowledge(ctx context.Context, query string, limit int) ([]domain.ContentContextTopicInsight, error) {
	return e.store.SearchLivingTopicKnowledge(ctx, query, limit)
}
func (e *Engine) SavedLibrary(ctx context.Context, query domain.MemoryLibraryQuery) (domain.MemoryLibraryResult, error) {
	return e.store.ListSavedMemory(ctx, query)
}
func (e *Engine) LibraryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	return e.store.MemoryLibraryItem(ctx, id)
}
func (e *Engine) LibraryStorage(ctx context.Context, limit int) (domain.MemoryStorageReport, error) {
	return e.store.MemoryStorageReport(ctx, limit)
}
func (e *Engine) KeepTimelineFullCopy(ctx context.Context, timelineID string) (domain.MemoryItem, bool, error) {
	return e.store.KeepTimelineFullCopy(ctx, timelineID)
}
func (e *Engine) ReadLaterTimeline(ctx context.Context, timelineID string) (domain.MemoryItem, bool, error) {
	return e.store.ReadLaterTimeline(ctx, timelineID)
}
func (e *Engine) KeepMemoryInLibrary(ctx context.Context, id string) (domain.MemoryItem, error) {
	return e.store.KeepMemoryInLibrary(ctx, id)
}
func (e *Engine) DoneSavedMemory(ctx context.Context, id string) (domain.MemoryItem, error) {
	return e.store.DoneSavedMemory(ctx, id)
}
func (e *Engine) ReleaseLibraryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	return e.store.ReleaseMemoryFullCopy(ctx, id)
}
func (e *Engine) RemoveLibraryItem(ctx context.Context, id string) error {
	topicIDs, err := e.store.LivingTopicIDsForMemory(ctx, id)
	if err != nil {
		return err
	}
	if err := e.store.RemoveMemory(ctx, id); err != nil {
		return err
	}
	for _, topicID := range topicIDs {
		e.launchLivingTopicUnderstanding(topicID, "evidence_removed")
	}
	return nil
}
func (e *Engine) ForgetLibraryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	topicIDs, err := e.store.LivingTopicIDsForMemory(ctx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	item, err := e.store.ForgetMemory(ctx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	for _, topicID := range topicIDs {
		e.launchLivingTopicUnderstanding(topicID, "evidence_forgotten")
	}
	return item, nil
}
func (e *Engine) LatestTimelineCheck(ctx context.Context) (*domain.TimelineCheckSummary, error) {
	return e.store.LatestTimelineCheck(ctx)
}

func (e *Engine) TimelineBatchSummaries(ctx context.Context) ([]domain.TimelineBatchSummary, error) {
	return e.store.TimelineBatchSummaries(ctx)
}
func (e *Engine) Inbox(ctx context.Context, limit, offset int) ([]domain.InboxSession, int, error) {
	return e.store.ListInboxSessions(ctx, limit, offset)
}
func (e *Engine) SessionModelUsage(ctx context.Context, sessionID string) (domain.ModelUsageReport, error) {
	return e.store.SessionModelUsage(ctx, sessionID)
}
func (e *Engine) AggregateModelUsage(ctx context.Context, windowDays int) (domain.ModelUsageReport, error) {
	return e.store.AggregateModelUsage(ctx, windowDays)
}
func (e *Engine) InboxRunTrace(ctx context.Context, runID, stage string, limit, offset int) (domain.InboxFlowTrace, error) {
	return e.store.InboxRunTrace(ctx, runID, stage, limit, offset)
}

func (e *Engine) VisionEvaluationJobs(ctx context.Context, runID string) ([]domain.VisionEvaluationJob, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	return e.store.ListVisionEvaluations(ctx, runID, run.Source)
}

func (e *Engine) RetryVisionEvaluation(ctx context.Context, jobID string) error {
	return e.store.RetryVisionEvaluation(ctx, jobID)
}

func (e *Engine) RecordCaptureSurfaceEvent(ctx context.Context, value domain.CaptureSurfaceEvent) (domain.CaptureSurfaceEvent, error) {
	return e.store.RecordCaptureSurfaceEvent(ctx, value)
}

func (e *Engine) RecordSplitActionAudit(ctx context.Context, value store.SplitActionAudit) error {
	return e.store.RecordSplitActionAudit(ctx, value)
}

func (e *Engine) ReadSplitActionAudit(ctx context.Context, since string, limit int) ([]store.SplitActionAudit, error) {
	return e.store.ReadSplitActionAudit(ctx, since, limit)
}

func (e *Engine) launch(runID string) {
	e.launchProcess(runID, true)
}

func integerCoverageValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func booleanCoverageValue(value any) bool {
	typed, _ := value.(bool)
	return typed
}

func stringCoverageValue(value any) string {
	typed, _ := value.(string)
	return typed
}

func (e *Engine) AddFeedback(ctx context.Context, timelineID string, value domain.Feedback) (domain.Feedback, error) {
	return e.store.AddFeedback(ctx, timelineID, value)
}

func (e *Engine) ReevaluateFailedRun(ctx context.Context, runID string) (domain.Run, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.Run{}, err
	} else if active != nil {
		return domain.Run{}, errors.New("finish the active update before re-evaluating an earlier run")
	}
	run, err := e.store.RetryFailedRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	e.launchProcess(runID, false)
	return run, nil
}

func (e *Engine) ApplyPassiveXMediaEvidence(ctx context.Context, timelineID, bridgeID string, value domain.PassiveXMediaEvidence) (domain.MediaRecapture, bool, error) {
	if e.developmentSources != nil {
		return domain.MediaRecapture{}, false, errors.New("development session passive media recapture unavailable")
	}
	defer e.releaseTerminalCaptureSessions(context.Background())
	e.operation.Lock()
	defer e.operation.Unlock()
	recapture, updated, err := e.store.ApplyPassiveXMediaEvidence(ctx, timelineID, bridgeID, value)
	if err == nil && updated {
		if item, loadErr := e.store.TimelineItem(ctx, timelineID); loadErr == nil {
			e.launchMediaProvenanceItems([]domain.TimelineItem{item})
		}
	}
	return recapture, updated, err
}

func (e *Engine) SemanticEventSuggestions(ctx context.Context, timelineID string, limit int) ([]domain.EventSuggestion, error) {
	return e.store.SuggestSemanticEvents(ctx, timelineID, limit)
}
func (e *Engine) CorrectSemanticEvent(ctx context.Context, timelineID, action, targetEventID, targetRelation string) (domain.EventCorrection, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.CorrectSemanticEvent(ctx, timelineID, action, targetEventID, targetRelation)
}
func (e *Engine) UndoSemanticCorrection(ctx context.Context, id string) (domain.EventCorrection, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.UndoSemanticCorrection(ctx, id)
}
func (e *Engine) ResetLearning(ctx context.Context) error {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.ResetLearning(ctx)
}
func (e *Engine) FullReset(ctx context.Context) (store.FullResetResult, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	e.cancelDeepDetections()
	defaults := domain.DefaultSettings(e.config.Capture.Profile, e.config.Capture.Visibility, e.config.Preference.Mode, e.config.Capture.OpenMissingSource)
	target := strings.TrimSpace(e.config.Reasoning.ActiveProvider)
	if target == "" {
		target = e.ProviderName()
	}
	defaults.ReasoningProvider = target
	defaults.ReasoningExecutablePath = e.ReasoningRuntime().ExecutablePath
	var swap *providerSwapPlan
	var err error
	if target != e.ProviderName() && len(e.config.Reasoning.Providers) > 0 {
		if err := e.ensureIdleForProviderSwap(ctx); err != nil {
			return store.FullResetResult{}, err
		}
		swap, err = e.planProviderSwap(ctx, target, &defaults)
		if err != nil {
			return store.FullResetResult{}, err
		}
	}
	defer func() {
		if swap != nil {
			swap.closeCandidate()
		}
	}()
	result, err := e.store.FullReset(ctx, defaults)
	if err != nil {
		return store.FullResetResult{}, err
	}
	if e.collectionRuntime != nil {
		e.collectionRuntime.Request("browser")
	}
	e.mu.Lock()
	e.headlessAccess = nil
	e.mu.Unlock()
	if swap != nil {
		swap.apply(e)
		swap = nil
	}
	return result, nil
}

func (e *Engine) CleanDatabase(ctx context.Context, request store.DatabaseCleanupRequest) (store.DatabaseCleanupResult, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveMediaRecapture(ctx); err != nil {
		return store.DatabaseCleanupResult{}, err
	} else if active {
		return store.DatabaseCleanupResult{}, errors.New("finish the active media recapture before database cleanup")
	}
	return e.store.CleanDatabase(ctx, request)
}
