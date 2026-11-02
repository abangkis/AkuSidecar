package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
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

func normalizeObservation(value *domain.Observation) {
	for snapshotIndex := range value.Snapshots {
		for blockIndex := range value.Snapshots[snapshotIndex].Blocks {
			block := &value.Snapshots[snapshotIndex].Blocks[blockIndex]
			if strings.TrimSpace(block.EvidenceKey) != "" {
				continue
			}
			identity := strings.TrimSpace(block.PlatformID)
			if identity == "" {
				identity = strings.TrimSpace(block.Permalink)
			}
			if identity == "" {
				author := strings.ToLower(strings.Join(strings.Fields(block.Author), " "))
				text := strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
				identity = author + "\x00" + text
				if strings.Trim(identity, "\x00") == "" {
					identity = ""
				}
			}
			if identity == "" {
				continue
			}
			digest := sha256.Sum256([]byte(string(value.Source) + "\x00" + identity))
			block.EvidenceKey = fmt.Sprintf("%s:%x", value.Source, digest[:12])
		}
	}
}

func validateObservation(value domain.Observation) error {
	if !value.Source.Valid() {
		return errors.New("invalid observation source")
	}
	if len(value.Snapshots) == 0 {
		return errors.New("observation has no snapshots")
	}
	seen := map[string]bool{}
	for _, snapshot := range value.Snapshots {
		for _, block := range snapshot.Blocks {
			if block.EvidenceKey == "" {
				return errors.New("captured block is missing evidenceKey")
			}
			if !blockHasEvidence(block) {
				return errors.New("captured block has no admissible text, media, attachment, or quoted-post evidence")
			}
			seen[block.EvidenceKey] = true
			if strings.TrimSpace(block.Permalink) != "" {
				if _, ok := domain.CanonicalSourceURL(value.Source, block.Permalink); !ok {
					return errors.New("captured block permalink is not a canonical native source URL")
				}
			}
			if err := domain.ValidateDirectContext(value.Source, block.DirectContext); err != nil {
				return err
			}
			if len(block.Attachments) > 3 {
				return errors.New("captured block exceeds the attachment limit")
			}
			for _, attachment := range block.Attachments {
				if err := attachment.Validate(); err != nil {
					return fmt.Errorf("captured block attachment is invalid: %w", err)
				}
			}
		}
	}
	if len(seen) == 0 {
		return errors.New("observation contains no evidence blocks")
	}
	if value.Coverage == nil {
		return errors.New("observation coverage is required")
	}
	return nil
}

func blockHasEvidence(block domain.Block) bool {
	if strings.TrimSpace(block.Text) != "" || len(block.Media) > 0 || len(block.Attachments) > 0 {
		return true
	}
	if len(block.QuotedPost) == 0 {
		return false
	}
	for _, key := range []string{"text", "media", "links"} {
		value, ok := block.QuotedPost[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		case []any:
			if len(typed) > 0 {
				return true
			}
		case []map[string]any:
			if len(typed) > 0 {
				return true
			}
		}
	}
	return false
}

func (e *Engine) launch(runID string) {
	e.launchProcess(runID, true)
}

func boundedFailureMessage(err error) string {
	return boundedText(err.Error(), 500)
}

func boundedText(value string, limit int) string {
	message := strings.TrimSpace(value)
	if len(message) > limit {
		return message[:limit]
	}
	return message
}

func providerFailureDetails(failure reasoning.ProviderFailure) map[string]any {
	details := map[string]any{"sdkCode": failure.Code, "sdkStage": failure.Stage, "sdkRetry": failure.Retry}
	if failure.Category != "" {
		details["sdkCategory"] = failure.Category
	}
	if failure.Reason != "" {
		details["sdkReason"] = failure.Reason
	}
	if failure.ProviderResponseStatus != "" {
		details["sdkProviderResponseStatus"] = failure.ProviderResponseStatus
	}
	if failure.PartialOutputSeen {
		details["sdkPartialOutputSeen"] = true
	}
	if len(failure.DiagnosticCodes) > 0 {
		details["sdkDiagnosticCodes"] = append([]string(nil), failure.DiagnosticCodes...)
	}
	if failure.ProviderStatus != 0 {
		details["sdkProviderStatus"] = failure.ProviderStatus
	}
	if failure.RequestID != "" {
		details["sdkRequestID"] = failure.RequestID
	}
	if failure.Operation != "" {
		details["sdkOperation"] = failure.Operation
	}
	if failure.RPCCode != 0 {
		details["sdkRPCCode"] = failure.RPCCode
	}
	if failure.ProcessExitCode != 0 {
		details["sdkProcessExitCode"] = failure.ProcessExitCode
	}
	return details
}

func (e *Engine) failRunAfterProcessError(ctx context.Context, runID string, processErr error) error {
	usageLimit := reasoning.IsUsageLimitError(processErr)
	failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning", Message: boundedFailureMessage(processErr), Retryable: true}
	if providerFailure, ok := reasoning.ProviderFailureFrom(processErr); ok {
		failure.Details = providerFailureDetails(providerFailure)
		if !providerFailure.RetryTransient {
			failure.Retryable = false
		}
		if providerFailure.Message != "" {
			failure.Message = boundedText(providerFailure.Message+" ("+processErr.Error()+")", 500)
		}
	}
	if usageLimit {
		failure.Code = "codex_usage_limit"
		failure.Retryable = false
	}
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	// Capture authority ends when the native collector is retired. Recording
	// a reasoning failure uses the durable accepted-capture receipt instead;
	// it must not reopen capture or pass through the generic mutation fence.
	if run.Status == "reasoning" {
		err = e.store.FailCapturedRunReasoning(ctx, run, failure)
	} else {
		err = e.store.FailRun(ctx, runID, failure)
	}
	if err != nil {
		return err
	}
	if usageLimit {
		e.pauseAutoUpdateForUsageLimit(ctx, run.SessionID, processErr)
		failedRun, loadErr := e.store.GetRun(ctx, runID)
		if loadErr != nil {
			return loadErr
		}
		if err := e.store.FailQueuedAfterReasoningFailure(ctx, failedRun, failure); err != nil {
			return err
		}
	}
	_, err = e.startNext(ctx, run.SessionID)
	return err
}

func (e *Engine) pauseAutoUpdateForUsageLimit(ctx context.Context, sessionID string, err error) bool {
	if !reasoning.IsUsageLimitError(err) {
		return false
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	pause := domain.AutoUpdateUsageLimitPause{Message: message, SessionID: sessionID}
	if pauseErr := e.store.PauseAutoUpdateForUsageLimit(ctx, pause); pauseErr != nil {
		e.logger.Printf("pause Auto Update after Codex usage limit: %v", pauseErr)
		return false
	}
	e.scheduler.notify()
	return true
}

func (e *Engine) clearAutoUpdateUsageLimitAfterSuccessfulUserSession(ctx context.Context, session domain.Session) {
	if session.Status != "completed" || session.Trigger != domain.UpdateTriggerUser || session.Delivery != domain.UpdateDeliveryVisible || session.BudgetAuthority != domain.BudgetAuthorityUser {
		return
	}
	pause, err := e.store.AutoUpdateUsageLimitPause(ctx)
	if err != nil {
		e.logger.Printf("read Auto Update usage-limit pause after successful user session %s: %v", session.ID, err)
		return
	}
	if pause == nil {
		return
	}
	report, err := e.store.SessionModelUsage(ctx, session.ID)
	if err != nil {
		e.logger.Printf("read model usage before restoring Auto Update after user session %s: %v", session.ID, err)
		return
	}
	if !modelUsageProvesCodexRestored(report) {
		return
	}
	if err := e.store.ConfirmAutoUpdateUsageRestored(ctx); err != nil {
		e.logger.Printf("restore Auto Update after successful user session %s: %v", session.ID, err)
		return
	}
	e.logger.Printf("restored Auto Update after model-backed user session %s completed successfully", session.ID)
	e.scheduler.notify()
}

func modelUsageProvesCodexRestored(report domain.ModelUsageReport) bool {
	completedModelInvocation := false
	for _, category := range report.Categories {
		for _, entry := range category.Entries {
			switch entry.Status {
			case "failed":
				return false
			case "completed":
				if strings.TrimSpace(entry.Provider) != "" && strings.TrimSpace(entry.Model) != "" && !strings.EqualFold(strings.TrimSpace(entry.Model), "none") {
					completedModelInvocation = true
				}
			}
		}
	}
	return completedModelInvocation
}

func bindReasoningSourceURLs(observation domain.Observation, result *domain.ReasoningResult) {
	permalinks := map[string]string{}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			if canonical, ok := domain.CanonicalSourceURL(observation.Source, block.Permalink); ok {
				permalinks[block.EvidenceKey] = canonical
			}
		}
	}
	for index := range result.Items {
		item := &result.Items[index]
		item.Source = observation.Source
		item.SourceURL = permalinks[item.EvidenceKey]
		if item.SourceURL == "" {
			item.SourceURLKind = ""
		} else {
			item.SourceURLKind = "native_post"
		}
	}
}

func splitVisionLaneCandidates(observation domain.Observation) (domain.Observation, []domain.VisionEvaluationInput) {
	result := observation
	result.Snapshots = make([]domain.Snapshot, len(observation.Snapshots))
	inputs := []domain.VisionEvaluationInput{}
	for snapshotIndex, snapshot := range observation.Snapshots {
		result.Snapshots[snapshotIndex] = snapshot
		result.Snapshots[snapshotIndex].Blocks = make([]domain.Block, 0, len(snapshot.Blocks))
		for _, block := range snapshot.Blocks {
			if domain.MediaOnlyVisionRequired(observation.Source, block) {
				inputs = append(inputs, domain.VisionEvaluationInput{EvidenceKey: block.EvidenceKey, Candidate: block})
				continue
			}
			result.Snapshots[snapshotIndex].Blocks = append(result.Snapshots[snapshotIndex].Blocks, block)
		}
	}
	return result, inputs
}

func visionEvaluationCoverage(value domain.InboxVisionEvaluation) map[string]any {
	return map[string]any{
		"policy": value.Policy, "available": value.Available, "availability": value.Availability,
		"capacity": value.Capacity, "active": value.Active, "pending": value.Pending,
		"deferred": value.Deferred, "evaluating": value.Evaluating, "retrying": value.Retrying,
		"ready": value.Ready, "failed": value.Failed,
	}
}

func validateReasoning(observation domain.Observation, result domain.ReasoningResult) error {
	keys := map[string]bool{}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			keys[block.EvidenceKey] = true
		}
	}
	assessed := map[string]bool{}
	for _, value := range result.CandidateAssessments {
		if !keys[value.EvidenceKey] {
			return fmt.Errorf("assessment references unknown evidence %s", value.EvidenceKey)
		}
		if assessed[value.EvidenceKey] {
			return fmt.Errorf("duplicate assessment %s", value.EvidenceKey)
		}
		assessed[value.EvidenceKey] = true
	}
	if len(assessed) != len(keys) {
		return fmt.Errorf("reasoning assessed %d of %d candidates", len(assessed), len(keys))
	}
	for _, item := range result.Items {
		if !keys[item.EvidenceKey] {
			return fmt.Errorf("item references unknown evidence %s", item.EvidenceKey)
		}
	}
	return nil
}

func selectedItems(run domain.Run, result domain.ReasoningResult, scored []store.ScoredAssessment, coverage map[string]any) []domain.TimelineItem {
	itemsByKey := map[string]domain.ReasonedItem{}
	for _, item := range result.Items {
		itemsByKey[item.EvidenceKey] = item
	}
	var items []domain.TimelineItem
	for _, value := range scored {
		if !value.Selected {
			continue
		}
		item, ok := itemsByKey[value.Assessment.EvidenceKey]
		if !ok {
			continue
		}
		items = append(items, domain.TimelineItem{ID: domain.NewID("timeline"), SessionID: run.SessionID, RunID: run.ID, Source: run.Source, EvidenceKey: item.EvidenceKey, Item: item, Assessment: value.Assessment, Coverage: coverage})
	}
	return items
}

func mergeObservations(values []domain.Observation) domain.Observation {
	result := values[len(values)-1]
	result.Snapshots = nil
	// Candidate evidence spans all rounds; navigation and planning telemetry
	// belongs exclusively to the last round. Missing latest fields stay unknown.
	result.Coverage = cloneCoverageMap(result.Coverage)
	result.Coverage["acquisitionRounds"] = len(values)
	rounds := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result.Snapshots = append(result.Snapshots, value.Snapshots...)
		rounds = append(rounds, value.Coverage)
	}
	result.Snapshots = reconcileCapturedSnapshots(result.Source, result.Snapshots)
	result.Coverage["rounds"] = rounds
	return result
}

func localFrontierFinishesAcquisition(source domain.Source, observation domain.Observation) bool {
	_, finished := localAcquisitionCompletionReason(source, observation, observationCandidateCount(observation))
	return finished
}

func localStructuredFallbackCompletionReason(source domain.Source, observation domain.Observation) (string, bool) {
	if source != domain.SourceInstagram || observationCandidateCount(observation) == 0 {
		return "", false
	}
	if strings.TrimSpace(stringCoverageValue(observation.Coverage["captureMethod"])) != "instagram_structured_feed_json" ||
		strings.TrimSpace(stringCoverageValue(observation.Coverage["scrollStopReason"])) != "structured_fallback" ||
		integerCoverageValue(observation.Coverage["performedScrolls"]) != 0 {
		return "", false
	}
	frontier, ok := observation.Coverage["frontier"].(map[string]any)
	if !ok || booleanCoverageValue(frontier["hasMoreCandidateSignal"]) || len(continuationAnchors(observation.Coverage)) > 0 {
		return "", false
	}
	return "The structured Instagram fallback produced bounded candidates but no navigable continuation frontier; repeating the same fallback would not extend coverage.", true
}

func localAcquisitionCompletionReason(source domain.Source, observation domain.Observation, eligibleCandidateCount int) (string, bool) {
	descriptor, ok := domain.SourceByID(source)
	if !ok || descriptor.FollowUpPlanningPolicy != "local_frontier" {
		return "", false
	}
	if integerCoverageValue(observation.Coverage["performedScrolls"]) < 1 {
		return "", false
	}
	if descriptor.FrontierRequiresComplete {
		captureQuality, ok := observation.Coverage["captureQuality"].(map[string]any)
		if !ok || strings.TrimSpace(stringCoverageValue(captureQuality["verdict"])) != "complete" {
			return "", false
		}
		if strings.TrimSpace(stringCoverageValue(observation.Coverage["scrollStopReason"])) == "deadline" {
			return "", false
		}
	}
	frontier, ok := observation.Coverage["frontier"].(map[string]any)
	if !ok {
		return "", false
	}
	if descriptor.InitialRoundCandidateTarget > 0 &&
		eligibleCandidateCount >= descriptor.InitialRoundCandidateTarget &&
		integerCoverageValue(frontier["newCandidateCount"]) >= descriptor.InitialRoundCandidateTarget {
		return "The complete first capture round already produced the configured bounded candidate target; older-feed coverage is deferred to the next finite check.", true
	}
	if booleanCoverageValue(frontier["hasMoreCandidateSignal"]) {
		return "", false
	}
	if integerCoverageValue(frontier["newCandidateCount"]) == 0 {
		return "The complete bounded source frontier produced no new candidate after scrolling.", true
	}
	return "", false
}

// localXFollowUpReason handles only the complete, budget-exhausted X frontier
// shape whose outcome is mechanically encoded by Bridge telemetry. Any missing,
// degraded, contradictory, or otherwise ambiguous signal remains model-planned.
func localXFollowUpReason(source domain.Source, observation domain.Observation) (string, bool) {
	descriptor, ok := domain.SourceByID(source)
	if !ok || descriptor.FollowUpPlanningPolicy != "guarded_request_frontier" || integerCoverageValue(observation.Coverage["performedScrolls"]) < 1 ||
		strings.TrimSpace(stringCoverageValue(observation.Coverage["scrollStopReason"])) != "budget_exhausted" {
		return "", false
	}
	quality, ok := observation.Coverage["captureQuality"].(map[string]any)
	if !ok || strings.TrimSpace(stringCoverageValue(quality["verdict"])) != "complete" {
		return "", false
	}
	frontier, ok := observation.Coverage["frontier"].(map[string]any)
	if !ok || !booleanCoverageValue(frontier["hasMoreCandidateSignal"]) || integerCoverageValue(frontier["newCandidateCount"]) < 1 ||
		len(continuationAnchors(observation.Coverage)) == 0 || continuationFrom(observation) == nil {
		return "", false
	}
	return "The complete X capture exhausted its bounded scroll budget while reporting a navigable older-feed frontier; queue one adjacent follow-up round.", true
}

func mergeDurableRunCoverage(target, durable map[string]any) {
	captureFields := map[string]bool{"rounds": true, "acquisitionRounds": true}
	if rounds, ok := target["rounds"].([]map[string]any); ok {
		for _, round := range rounds {
			for key := range round {
				captureFields[key] = true
			}
		}
	}
	for key, value := range durable {
		if captureFields[key] {
			continue
		}
		target[key] = value
	}
}

func cloneCoverageMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func followUpNewCandidateCount(observations []domain.Observation) int {
	if len(observations) < 2 {
		return 0
	}
	first := observationEvidenceKeys(observations[0].Source, observations[0].Snapshots)
	allSnapshots := []domain.Snapshot{}
	for _, observation := range observations {
		allSnapshots = append(allSnapshots, observation.Snapshots...)
	}
	all := observationEvidenceKeys(observations[0].Source, allSnapshots)
	count := 0
	for key := range all {
		if !first[key] {
			count++
		}
	}
	return count
}

func observationEvidenceKeys(source domain.Source, snapshots []domain.Snapshot) map[string]bool {
	result := map[string]bool{}
	for _, snapshot := range reconcileCapturedSnapshots(source, snapshots) {
		for _, block := range snapshot.Blocks {
			if key := strings.TrimSpace(block.EvidenceKey); key != "" {
				result[key] = true
			}
		}
	}
	return result
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

func continuationFrom(value domain.Observation) map[string]any {
	startScrollY := 0.0
	anchors := continuationAnchors(value.Coverage)
	if frontier, ok := value.Coverage["frontier"].(map[string]any); ok {
		if readiness, explicit := frontier["continuationReady"]; explicit {
			if ready, valid := readiness.(bool); !valid || !ready {
				return nil
			}
		}
		if scrollY, ok := frontier["scrollY"].(float64); ok && scrollY >= 0 {
			startScrollY = scrollY
		}
	}
	if len(value.Snapshots) == 0 {
		return nil
	}
	// Sources that virtualize and remeasure their feed after restoration resume
	// from the preceding observed snapshot. This proves continuity against a
	// bounded overlap without placing source-specific behavior in orchestration.
	descriptor, _ := domain.SourceByID(value.Source)
	if descriptor.ContinuationOverlapRequired && len(value.Snapshots) > 1 {
		for index := len(value.Snapshots) - 2; index >= 0; index-- {
			checkpoint := value.Snapshots[index]
			checkpointAnchors := snapshotContinuationAnchors(checkpoint)
			if len(checkpointAnchors) == 0 {
				continue
			}
			startScrollY = float64(checkpoint.ScrollY)
			anchors = checkpointAnchors
			break
		}
	}
	last := value.Snapshots[len(value.Snapshots)-1]
	if startScrollY == 0 && last.ScrollY > 0 {
		startScrollY = float64(last.ScrollY)
	}
	if len(anchors) == 0 {
		for _, block := range last.Blocks {
			identity := strings.TrimSpace(block.PlatformID)
			if identity == "" {
				identity = strings.TrimSpace(block.Permalink)
			}
			if identity == "" {
				identity = strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
				if len(identity) > 300 {
					identity = identity[:300]
				}
			}
			if identity != "" {
				anchors = append(anchors, identity)
			}
			if len(anchors) == 3 {
				break
			}
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	return map[string]any{"startScrollY": int(startScrollY), "anchorKeys": anchors, "settleMs": 900}
}

func snapshotContinuationAnchors(snapshot domain.Snapshot) []string {
	anchors := make([]string, 0, 3)
	for _, block := range snapshot.Blocks {
		identity := strings.TrimSpace(block.PlatformID)
		if identity == "" {
			identity = strings.TrimSpace(block.Permalink)
		}
		if identity == "" {
			identity = strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
			if len(identity) > 300 {
				identity = identity[:300]
			}
		}
		if identity != "" {
			anchors = append(anchors, identity)
		}
		if len(anchors) == 3 {
			break
		}
	}
	return anchors
}

func continuationAnchors(coverage map[string]any) []string {
	frontier, ok := coverage["frontier"].(map[string]any)
	if !ok {
		return nil
	}
	var values []string
	switch anchors := frontier["anchorKeys"].(type) {
	case []any:
		for _, value := range anchors {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				values = append(values, strings.TrimSpace(text))
			}
			if len(values) == 3 {
				break
			}
		}
	case []string:
		for _, value := range anchors {
			if strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
			if len(values) == 3 {
				break
			}
		}
	}
	return values
}

func (e *Engine) AddFeedback(ctx context.Context, timelineID string, value domain.Feedback) (domain.Feedback, error) {
	return e.store.AddFeedback(ctx, timelineID, value)
}

func (e *Engine) CorrectSelection(ctx context.Context, runID, candidateRef string) (domain.SelectionCorrection, domain.TimelineItem, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	} else if active != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, errors.New("finish the active update before correcting an earlier selection")
	}
	correction, item, err := e.store.CreateSelectionCorrection(ctx, runID, candidateRef)
	if err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return correction, item, err
	}
	if e.events != nil && settings.SemanticEventMode != "show_all" {
		if _, eventErr := e.events.ProcessTimelineItem(ctx, item.ID, settings); eventErr != nil {
			e.logger.Printf("semantic event resolution for selection correction %s degraded safely: %v", correction.ID, eventErr)
		}
	}
	if err := e.store.SaveSelectionCorrectionKnowledge(ctx, correction.ID); err != nil {
		e.logger.Printf("knowledge continuity for selection correction %s degraded safely: %v", correction.ID, err)
	}
	item, err = e.store.TimelineItem(ctx, item.ID)
	if err != nil {
		return correction, domain.TimelineItem{}, err
	}
	if settings.AIDetectionEnabled {
		if err := e.store.SaveAIAssessments(ctx, e.aiFast.Detect([]domain.TimelineItem{item})); err != nil {
			e.logger.Printf("AI Fast Detection for selection correction %s degraded safely: %v", correction.ID, err)
		}
	}
	item, err = e.store.TimelineItem(ctx, item.ID)
	if err != nil {
		return correction, domain.TimelineItem{}, err
	}
	if settings.AIDetectionEnabled {
		e.launchDeepDetectionItems(item.SessionID, []domain.TimelineItem{item})
		e.launchMediaProvenanceItems([]domain.TimelineItem{item})
	}
	e.launchLivingTopicRouting(item.SessionID)
	return correction, item, nil
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

func (e *Engine) UndoSelectionCorrection(ctx context.Context, id string) (domain.SelectionCorrection, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.SelectionCorrection{}, err
	} else if active != nil {
		return domain.SelectionCorrection{}, errors.New("finish the active update before undoing a selection correction")
	}
	value, err := e.store.UndoSelectionCorrection(ctx, id)
	if err != nil {
		return domain.SelectionCorrection{}, err
	}
	e.mu.Lock()
	for key, cancel := range e.work.active {
		if strings.HasPrefix(key, "ai:"+value.SessionID+":") {
			cancel()
		}
	}
	e.mu.Unlock()
	_ = e.store.RefreshEventResolutionCounts(ctx, value.SessionID)
	return value, nil
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
func (e *Engine) AddAIFeedback(ctx context.Context, timelineID string, input domain.AIFeedbackInput) (domain.AIFeedbackEvent, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	feedback, err := e.store.AddAIFeedback(ctx, timelineID, input)
	if err != nil || input.Verdict != "unsure" {
		return feedback, err
	}
	settings, settingsErr := e.store.GetSettings(ctx)
	if settingsErr != nil {
		e.logger.Printf("AI Deep Detection could not inspect settings for feedback %s: %v", feedback.ID, settingsErr)
		return feedback, nil
	}
	if !settings.AIDetectionEnabled {
		return feedback, nil
	}
	item, itemErr := e.store.TimelineItem(ctx, timelineID)
	if itemErr != nil {
		e.logger.Printf("AI Deep Detection could not load feedback item %s: %v", feedback.ID, itemErr)
		return feedback, nil
	}
	e.launchDeepDetectionItems(item.SessionID, []domain.TimelineItem{item})
	return feedback, nil
}
func (e *Engine) UndoAIFeedback(ctx context.Context, id string) (domain.AIFeedbackEvent, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.UndoAIFeedback(ctx, id)
}
func (e *Engine) AIFeedbackHistory(ctx context.Context, timelineID string) ([]domain.AIFeedbackEvent, error) {
	return e.store.AIFeedbackHistory(ctx, timelineID)
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
