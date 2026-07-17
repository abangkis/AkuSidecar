package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/selection"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	ExpectedBridgeVersion         = "0.6.8"
	ExpectedBridgeRevision        = "source-fidelity-v58"
	ExpectedBridgeID              = "aku-bridge-chrome-mv3-v0"
	ExpectedXAdapter              = "x-dom-v18"
	ExpectedLinkedInAdapter       = "linkedin-dom-v15"
	ExpectedXMediaEvidenceAdapter = "x-response-evidence-v1"
)

var expectedBridgeSources = []string{"x", "linkedin"}
var expectedBridgeActions = []string{
	"probe_readiness", "probe_freshness", "recover_source_freshness",
	"collect_visible", "detect_pending_content", "report_adapter_health",
	"report_capture_quality", "acquire_missing_media", "recapture_missing_media",
	"cache_passive_media_evidence", "lookup_passive_media_evidence", "observe_response_media_evidence", "extract_source_semantics",
	"report_frontier", "manage_source_tab_lifecycle", "manage_capture_window",
	"release_capture_surface", "preserve_working_tab", "report_source_events", "reload_self",
}

type Engine struct {
	store        *store.Store
	provider     reasoning.Provider
	config       config.Config
	epoch        string
	mu           sync.RWMutex
	operation    sync.Mutex
	heartbeat    *domain.BridgeHeartbeat
	active       map[string]context.CancelFunc
	shuttingDown bool
	logger       Logger
	reloads      *ReloadActions
	events       *semanticengine.Engine
	aiFast       aidetector.FastDetector
	aiDeep       aidetector.Resolver
}

type Logger interface{ Printf(string, ...any) }

func New(state *store.Store, provider reasoning.Provider, cfg config.Config, logger Logger, eventEngines ...*semanticengine.Engine) *Engine {
	var events *semanticengine.Engine
	if len(eventEngines) > 0 {
		events = eventEngines[0]
	}
	return &Engine{store: state, provider: provider, config: cfg, epoch: domain.NewID("epoch"), active: map[string]context.CancelFunc{}, logger: logger, reloads: NewReloadActions(15 * time.Second), events: events}
}
func (e *Engine) SetAIDeepResolver(value aidetector.Resolver) { e.aiDeep = value }
func (e *Engine) Epoch() string                               { return e.epoch }
func (e *Engine) ProviderName() string                        { return e.provider.Name() }
func (e *Engine) Settings(ctx context.Context) (domain.Settings, error) {
	return e.store.GetSettings(ctx)
}
func (e *Engine) SaveSettings(ctx context.Context, value domain.Settings) (domain.Settings, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	value.Normalize()
	if err := e.store.SaveSettings(ctx, value); err != nil {
		return domain.Settings{}, err
	}
	saved, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	if _, err := e.store.EnforceRetention(ctx, saved); err != nil {
		return domain.Settings{}, fmt.Errorf("apply retention settings: %w", err)
	}
	return saved, nil
}
func (e *Engine) Onboarding(ctx context.Context) (domain.OnboardingState, error) {
	return e.store.Onboarding(ctx)
}
func (e *Engine) CompleteOnboarding(ctx context.Context, sources []domain.Source) (domain.OnboardingState, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.CompleteOnboarding(ctx, sources)
}

type BridgeStatus struct {
	State      string                  `json:"state"`
	Compatible bool                    `json:"compatible"`
	Reasons    []string                `json:"reasons"`
	Expected   map[string]any          `json:"expected"`
	Actual     *domain.BridgeHeartbeat `json:"actual"`
}

func (e *Engine) RecordHeartbeat(value domain.BridgeHeartbeat) BridgeStatus {
	value.ReceivedAt = domain.Now()
	e.mu.Lock()
	e.heartbeat = &value
	e.mu.Unlock()
	e.reloads.Observe(value)
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
func (e *Engine) BridgeStatus() BridgeStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()
	status := BridgeStatus{State: "reconnecting", Expected: map[string]any{"bridgeId": ExpectedBridgeID, "extensionVersion": ExpectedBridgeVersion, "runtimeRevision": ExpectedBridgeRevision, "buildId": ExpectedBridgeBuildID, "adapterVersions": map[string]string{"x": ExpectedXAdapter, "linkedin": ExpectedLinkedInAdapter}, "mediaEvidenceAdapterVersions": map[string]string{"x": ExpectedXMediaEvidenceAdapter}, "contract": domain.BridgeContractVersion, "manifestVersion": 3, "sources": expectedBridgeSources, "actions": expectedBridgeActions, "authority": "read_only_bounded", "captureLimits": domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20}}}
	if e.heartbeat == nil {
		return status
	}
	copy := *e.heartbeat
	status.Actual = &copy
	status.State = "healthy"
	if copy.BridgeID != ExpectedBridgeID {
		status.Reasons = append(status.Reasons, "bridge id mismatch")
	}
	if copy.ExtensionVersion != ExpectedBridgeVersion {
		status.Reasons = append(status.Reasons, "extension version mismatch")
	}
	if copy.RuntimeRevision != ExpectedBridgeRevision {
		status.Reasons = append(status.Reasons, "runtime revision mismatch")
	}
	if copy.ContractVersion != domain.BridgeContractVersion {
		status.Reasons = append(status.Reasons, "bridge contract mismatch")
	}
	if copy.BuildID != ExpectedBridgeBuildID {
		status.Reasons = append(status.Reasons, "bridge build mismatch")
	}
	if copy.AdapterVersions["x"] != ExpectedXAdapter || copy.AdapterVersions["linkedin"] != ExpectedLinkedInAdapter || len(copy.AdapterVersions) != 2 {
		status.Reasons = append(status.Reasons, "adapter version mismatch")
	}
	if copy.MediaEvidenceAdapterVersions["x"] != ExpectedXMediaEvidenceAdapter || len(copy.MediaEvidenceAdapterVersions) != 1 {
		status.Reasons = append(status.Reasons, "media evidence adapter version mismatch")
	}
	if copy.ManifestVersion != 3 {
		status.Reasons = append(status.Reasons, "manifest version mismatch")
	}
	if !sameStringSet(copy.Sources, expectedBridgeSources) {
		status.Reasons = append(status.Reasons, "bridge sources mismatch")
	}
	if !sameStringSet(copy.Actions, expectedBridgeActions) {
		status.Reasons = append(status.Reasons, "bridge actions mismatch")
	}
	if copy.Authority != "read_only_bounded" {
		status.Reasons = append(status.Reasons, "bridge authority mismatch")
	}
	if copy.CaptureLimits != (domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20}) {
		status.Reasons = append(status.Reasons, "capture limits mismatch")
	}
	status.Compatible = len(status.Reasons) == 0
	if !status.Compatible {
		status.State = "incompatible"
	}
	return status
}

func ExpectedHeartbeat() domain.BridgeHeartbeat {
	return domain.BridgeHeartbeat{BridgeID: ExpectedBridgeID, ExtensionVersion: ExpectedBridgeVersion, RuntimeRevision: ExpectedBridgeRevision, BuildID: ExpectedBridgeBuildID, AdapterVersions: map[string]string{"x": ExpectedXAdapter, "linkedin": ExpectedLinkedInAdapter}, MediaEvidenceAdapterVersions: map[string]string{"x": ExpectedXMediaEvidenceAdapter}, ContractVersion: domain.BridgeContractVersion, ManifestVersion: 3, Sources: append([]string(nil), expectedBridgeSources...), Actions: append([]string(nil), expectedBridgeActions...), Authority: "read_only_bounded", CaptureLimits: domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20}}
}

func sameStringSet(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	values := make(map[string]bool, len(actual))
	for _, value := range actual {
		values[value] = true
	}
	if len(values) != len(expected) {
		return false
	}
	for _, value := range expected {
		if !values[value] {
			return false
		}
	}
	return true
}

func (e *Engine) StartSession(ctx context.Context, intent string) (domain.Session, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	onboarding, err := e.store.Onboarding(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if onboarding.Status != "completed" {
		return domain.Session{}, errors.New("complete onboarding before starting an update")
	}
	if recapturing, err := e.store.ActiveMediaRecapture(ctx); err != nil {
		return domain.Session{}, err
	} else if recapturing {
		return domain.Session{}, errors.New("finish the active media recapture before starting an update")
	}
	calibration, err := e.store.ActiveCalibration(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if calibration != nil {
		return domain.Session{}, errors.New("complete the active calibration before starting another update")
	}
	status := e.BridgeStatus()
	if !status.Compatible {
		return domain.Session{}, fmt.Errorf("AkuBridge v2 is not ready: %s", strings.Join(status.Reasons, "; "))
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	e.cancelDeepDetections()
	session, err := e.store.CreateSession(ctx, intent, settings)
	if err != nil {
		return domain.Session{}, err
	}
	if _, err := e.startNext(ctx, session.ID); err != nil {
		return domain.Session{}, err
	}
	return e.store.GetSession(ctx, session.ID)
}

func (e *Engine) startNext(ctx context.Context, sessionID string) (*domain.Run, error) {
	run, err := e.store.AdvanceSession(ctx, sessionID)
	if err != nil {
		return run, err
	}
	if run == nil {
		settings, settingsErr := e.store.GetSettings(ctx)
		if settingsErr != nil {
			return nil, settingsErr
		}
		if e.events != nil && settings.SemanticEventMode != "show_all" {
			if _, eventErr := e.events.ProcessSession(ctx, sessionID, settings); eventErr != nil {
				e.logger.Printf("semantic event resolution for session %s degraded safely: %v", sessionID, eventErr)
			}
		}
		if composeErr := e.store.ComposeSession(ctx, sessionID); composeErr != nil {
			return nil, fmt.Errorf("compose unified Timeline: %w", composeErr)
		}
		if detectionErr := e.runFastDetection(ctx, sessionID); detectionErr != nil {
			e.logger.Printf("AI Fast Detection for session %s degraded safely: %v", sessionID, detectionErr)
		}
		if finalizeErr := e.store.FinalizeSession(ctx, sessionID); finalizeErr != nil {
			return nil, fmt.Errorf("finalize unified session: %w", finalizeErr)
		}
		if _, retentionErr := e.store.EnforceRetention(ctx, settings); retentionErr != nil {
			e.logger.Printf("retention after session %s failed: %v", sessionID, retentionErr)
		}
		if _, calibrationErr := e.ensurePendingFirstCalibration(ctx, sessionID); calibrationErr != nil {
			e.logger.Printf("first-run calibration for session %s could not start: %v", sessionID, calibrationErr)
		}
		e.launchDeepDetection(sessionID)
		return nil, nil
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	payload := capturePayload(*run, sessionID, settings, 1, nil, "")
	if _, err = e.store.StartRun(ctx, run.ID, payload); err != nil {
		return nil, err
	}
	started, err := e.store.GetRun(ctx, run.ID)
	return &started, err
}

func (e *Engine) runFastDetection(ctx context.Context, sessionID string) error {
	items, err := e.store.ListSessionItems(ctx, sessionID)
	if err != nil {
		return err
	}
	return e.store.SaveAIAssessments(ctx, e.aiFast.Detect(items))
}

func (e *Engine) launchDeepDetection(sessionID string) {
	resolver := e.aiDeep
	if resolver == nil {
		return
	}
	items, err := e.store.ListSessionItems(context.Background(), sessionID)
	if err != nil || len(items) == 0 {
		if err != nil {
			e.logger.Printf("AI Deep Detection could not load session %s: %v", sessionID, err)
		}
		return
	}
	items = aidetector.DeepCandidates(items)
	if len(items) == 0 {
		return
	}
	model := resolver.Model()
	job, err := e.store.CreateAIDetectionJob(context.Background(), domain.AIDetectionJob{
		SessionID: sessionID, Status: "queued", Provider: resolver.Name(), Model: model.Model,
		Effort: model.Effort, CandidateCount: len(items),
	})
	if err != nil {
		e.logger.Printf("AI Deep Detection could not queue session %s: %v", sessionID, err)
		return
	}
	key := "ai:" + sessionID
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.active[key] = cancel
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.active, key)
			e.mu.Unlock()
		}()
		if err := e.store.StartAIDetectionJob(ctx, job.ID); err != nil {
			status := "failed"
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				status = "cancelled"
			}
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, status, 0, domain.ModelUsage{}, err)
			if status != "cancelled" {
				e.logger.Printf("AI Deep Detection could not start session %s: %v", sessionID, err)
			}
			return
		}
		result, usage, duration, resolveErr := resolver.Resolve(ctx, items)
		if resolveErr != nil {
			status := "failed"
			if errors.Is(resolveErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				status = "cancelled"
			}
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, status, duration.Milliseconds(), usage, resolveErr)
			if status != "cancelled" {
				e.logger.Printf("AI Deep Detection for session %s degraded safely: %v", sessionID, resolveErr)
			}
			return
		}
		assessedAt := domain.Now()
		assessments := make([]domain.AIAssessment, 0, len(items))
		for index, value := range result.Assessments {
			supersedes := ""
			if items[index].AIDetection != nil {
				supersedes = items[index].AIDetection.AssessmentID
			}
			assessments = append(assessments, domain.AIAssessment{
				ID: domain.NewID("ai_assessment"), TimelineID: items[index].ID, SessionID: sessionID,
				Stage: "deep", Status: value.Status, ConfidenceBand: value.ConfidenceBand,
				EvidenceCodes: value.EvidenceCodes, AssessedObject: value.AssessedObject, SignalScope: value.SignalScope,
				Provider: resolver.Name(), DetectorVersion: aidetector.DeepDetectorVersion,
				Rationale: value.Rationale, SupersedesID: supersedes, CreatedAt: assessedAt,
			})
		}
		if err := e.store.SaveAIAssessments(context.Background(), assessments); err != nil {
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, "failed", duration.Milliseconds(), usage, err)
			e.logger.Printf("AI Deep Detection could not persist session %s: %v", sessionID, err)
			return
		}
		_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, "completed", duration.Milliseconds(), usage, nil)
	}()
}

func (e *Engine) cancelDeepDetections() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, cancel := range e.active {
		if strings.HasPrefix(id, "ai:") {
			cancel()
		}
	}
}

func capturePayload(run domain.Run, leaseID string, settings domain.Settings, round int, continuation map[string]any, reason string) map[string]any {
	return map[string]any{"mode": "catch_up", "source": run.Source, "scrolls": settings.MaxScrolls, "scrollFraction": 0.75, "scrollSettleMs": 900, "captureTimeoutMs": 45000, "pendingContentPolicy": map[bool]string{true: "reveal_if_present", false: "detect_only"}[round == 1], "sameTabMutationAllowed": round == 1, "pendingContentTimeoutMs": 5000, "pendingContentSettleMs": 700, "sourceFreshnessPolicy": map[bool]string{true: "wake_and_reveal", false: "preserve_frontier"}[round == 1], "captureVisibilityPolicy": settings.CaptureVisibility, "captureLeaseId": leaseID, "maxBlocksPerSnapshot": 20, "maxBlockCharacters": 4000, "qualityReportRequired": true, "qualityRetryBudget": 1, "qualityRetrySettleMs": settings.QualityRetrySettleMS, "openIfMissing": round == 1 && settings.OpenMissingSource, "tabLifecycle": map[string]any{"ownership": "shared", "openedTabDisposition": "preserve"}, "restoreScroll": true, "browserAdapter": "aku-bridge", "acquisitionRound": round, "maxAcquisitionRounds": 2, "continuation": continuation, "followUpReason": reason}
}

func (e *Engine) ClaimCommand(ctx context.Context, runID, bridgeID string) (*domain.BridgeCommand, error) {
	return e.store.ClaimCommand(ctx, runID, bridgeID)
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
func (e *Engine) LatestTimelineCheck(ctx context.Context) (*domain.TimelineCheckSummary, error) {
	return e.store.LatestTimelineCheck(ctx)
}
func (e *Engine) Inbox(ctx context.Context, limit, offset int) ([]domain.InboxSession, int, error) {
	return e.store.ListInboxSessions(ctx, limit, offset)
}

func (e *Engine) AcceptObservation(ctx context.Context, commandID, runID string, value domain.Observation) (domain.Run, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if value.Source != run.Source {
		return domain.Run{}, errors.New("observation source does not match run")
	}
	normalizeObservation(&value)
	if err := validateObservation(value); err != nil {
		return domain.Run{}, err
	}
	if err := e.store.SaveObservation(ctx, commandID, runID, value); err != nil {
		return domain.Run{}, err
	}
	e.launch(runID)
	return e.store.GetRun(ctx, runID)
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
				identity = strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
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
			seen[block.EvidenceKey] = true
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

func (e *Engine) launch(runID string) {
	e.launchProcess(runID, true)
}

func (e *Engine) launchProcess(runID string, allowPlanning bool) {
	e.mu.Lock()
	if e.shuttingDown {
		e.mu.Unlock()
		return
	}
	if _, exists := e.active[runID]; exists {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.active[runID] = cancel
	e.mu.Unlock()
	go func() {
		defer func() { e.mu.Lock(); delete(e.active, runID); e.mu.Unlock() }()
		if err := e.process(ctx, runID, allowPlanning); err != nil {
			if e.shouldPauseForShutdown(ctx, err) {
				e.logger.Printf("run %s paused during shutdown and will resume from accepted capture", runID)
				return
			}
			e.logger.Printf("run %s failed: %v", runID, err)
			failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning", Message: err.Error(), Retryable: true}
			_ = e.store.FailRun(context.Background(), runID, failure)
			run, _ := e.store.GetRun(context.Background(), runID)
			_, _ = e.startNext(context.Background(), run.SessionID)
		}
	}()
}

func (e *Engine) shouldPauseForShutdown(ctx context.Context, err error) bool {
	e.mu.RLock()
	shuttingDown := e.shuttingDown
	e.mu.RUnlock()
	return shuttingDown && (errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled))
}

// ResumePendingReasoning continues only from already accepted observations.
// Planning is not repeated because a restart must not request another browser
// acquisition for evidence that is already durable in SQLite.
func (e *Engine) ResumePendingReasoning(ctx context.Context) (int, error) {
	ids, err := e.store.ResumableReasoningRunIDs(ctx)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		e.launchProcess(id, false)
	}
	return len(ids), nil
}

func (e *Engine) process(ctx context.Context, runID string, allowPlanning bool) error {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	observations, err := e.store.Observations(ctx, runID)
	if err != nil {
		return err
	}
	if len(observations) == 0 {
		return errors.New("no accepted observation")
	}
	merged := mergeObservations(observations)
	knowledge, err := e.store.Knowledge(ctx, run.Source, 200)
	if err != nil {
		return err
	}
	if allowPlanning && len(observations) == 1 && e.config.Capture.MaxAcquisitionRounds > 1 {
		plan, telemetry, planErr := e.provider.Plan(ctx, run, merged, knowledge)
		_ = e.store.SaveTelemetry(context.Background(), telemetry)
		if planErr != nil {
			return planErr
		}
		if plan.Decision == "request_follow_up" {
			continuation := continuationFrom(merged)
			if continuation != nil {
				settings, _ := e.store.GetSettings(ctx)
				payload := capturePayload(run, run.SessionID, settings, 2, continuation, plan.Reason)
				_, err = e.store.QueueFollowUp(ctx, runID, payload)
				return err
			}
		}
	}
	result, telemetry, err := e.provider.Analyze(ctx, run, merged, knowledge)
	_ = e.store.SaveTelemetry(context.Background(), telemetry)
	if err != nil {
		return err
	}
	if err = validateReasoning(merged, result); err != nil {
		return err
	}
	profile, err := e.preferenceProfile(ctx, true)
	if err != nil {
		return err
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	evidenceKeys := make([]string, 0, len(result.CandidateAssessments))
	for _, assessment := range result.CandidateAssessments {
		evidenceKeys = append(evidenceKeys, assessment.EvidenceKey)
	}
	excluded, err := e.store.PreviouslyDeliveredEvidence(ctx, run.Source, evidenceKeys)
	if err != nil {
		return err
	}
	protected := map[string]bool{}
	eventKeys := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if item.EventKey != "" {
			eventKeys = append(eventKeys, item.EventKey)
		}
	}
	priorEvents, err := e.store.PreviouslyKnownEvents(ctx, run.Source, eventKeys)
	if err != nil {
		return err
	}
	for _, item := range result.Items {
		if excluded[item.EvidenceKey] {
			continue
		}
		if item.KnowledgeDelta == "material_update" || item.KnowledgeDelta == "contradiction" {
			protected[item.EvidenceKey] = true
			continue
		}
		if item.EventKey != "" && priorEvents[item.EventKey] {
			excluded[item.EvidenceKey] = true
		}
	}
	scored := selection.SelectWithOptions(result.CandidateAssessments, profile, selection.Options{
		Limit: settings.MaxItemsPerSource, Mode: settings.PreferenceEligibilityMode,
		ExcludedEvidence: excluded, ProtectedEvidence: protected,
	})
	items := selectedItems(run, result, scored, merged.Coverage)
	if err = e.store.CompleteRun(ctx, run, result, scored, items, merged.Coverage); err != nil {
		return err
	}
	_, err = e.startNext(ctx, run.SessionID)
	return err
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
	result.Coverage = map[string]any{"acquisitionRounds": len(values), "rounds": []map[string]any{}}
	rounds := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result.Snapshots = append(result.Snapshots, value.Snapshots...)
		rounds = append(rounds, value.Coverage)
	}
	result.Snapshots = reconcileCapturedSnapshots(result.Source, result.Snapshots)
	result.Coverage["rounds"] = rounds
	return result
}
func continuationFrom(value domain.Observation) map[string]any {
	startScrollY := 0.0
	anchors := continuationAnchors(value.Coverage)
	if frontier, ok := value.Coverage["frontier"].(map[string]any); ok {
		if scrollY, ok := frontier["scrollY"].(float64); ok && scrollY >= 0 {
			startScrollY = scrollY
		}
	}
	if len(value.Snapshots) == 0 {
		return nil
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

func (e *Engine) FailCommand(ctx context.Context, commandID, runID string, failure domain.Failure) (domain.Run, error) {
	if failure.Code == "" {
		failure.Code = "bridge_failure"
	}
	if failure.Stage == "" {
		failure.Stage = "capture"
	}
	fallback, err := e.store.FailCommand(ctx, commandID, runID, failure)
	if err != nil {
		return domain.Run{}, err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err == nil {
		if fallback {
			e.launchProcess(runID, false)
		} else {
			_, _ = e.startNext(ctx, run.SessionID)
		}
	}
	return run, err
}
func (e *Engine) CancelSession(ctx context.Context, id string) error {
	e.mu.Lock()
	for runID, cancel := range e.active {
		run, err := e.store.GetRun(ctx, runID)
		if err == nil && run.SessionID == id {
			cancel()
			delete(e.active, runID)
		}
	}
	e.mu.Unlock()
	return e.store.CancelSession(ctx, id)
}
func (e *Engine) AddFeedback(ctx context.Context, timelineID string, value domain.Feedback) (domain.Feedback, error) {
	return e.store.AddFeedback(ctx, timelineID, value)
}

func (e *Engine) QueueMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode) (domain.MediaRecapture, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.MediaRecapture{}, err
	} else if active != nil {
		return domain.MediaRecapture{}, errors.New("finish the active update before recapturing media")
	}
	status := e.BridgeStatus()
	if !status.Compatible {
		return domain.MediaRecapture{}, fmt.Errorf("AkuBridge v2 is not ready: %s", strings.Join(status.Reasons, "; "))
	}
	return e.store.CreateMediaRecapture(ctx, timelineID, mode)
}

func (e *Engine) ClaimMediaRecapture(ctx context.Context, id, bridgeID string) (domain.MediaRecapture, error) {
	return e.store.ClaimMediaRecapture(ctx, id, bridgeID)
}

func (e *Engine) AcceptMediaRecapture(ctx context.Context, id string, observation domain.Observation) (domain.MediaRecapture, error) {
	normalizeObservation(&observation)
	if err := validateObservation(observation); err != nil {
		return domain.MediaRecapture{}, err
	}
	return e.store.CompleteMediaRecapture(ctx, id, observation)
}

func (e *Engine) FailMediaRecapture(ctx context.Context, id string, failure domain.Failure) (domain.MediaRecapture, error) {
	return e.store.FailMediaRecapture(ctx, id, failure)
}

func (e *Engine) ApplyPassiveXMediaEvidence(ctx context.Context, timelineID, bridgeID string, value domain.PassiveXMediaEvidence) (domain.MediaRecapture, bool, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.ApplyPassiveXMediaEvidence(ctx, timelineID, bridgeID, value)
}

func (e *Engine) SemanticEventSuggestions(ctx context.Context, timelineID string, limit int) ([]domain.EventSuggestion, error) {
	return e.store.SuggestSemanticEvents(ctx, timelineID, limit)
}
func (e *Engine) CorrectSemanticEvent(ctx context.Context, timelineID, action, targetEventID string) (domain.EventCorrection, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.CorrectSemanticEvent(ctx, timelineID, action, targetEventID)
}
func (e *Engine) UndoSemanticCorrection(ctx context.Context, id string) (domain.EventCorrection, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.UndoSemanticCorrection(ctx, id)
}
func (e *Engine) CorrectAIDetection(ctx context.Context, timelineID, verdict string) (domain.AIAssessment, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.AddAICorrection(ctx, timelineID, verdict)
}
func (e *Engine) UndoAICorrection(ctx context.Context, id string) (domain.AIAssessment, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.UndoAICorrection(ctx, id)
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
	return e.store.FullReset(ctx, defaults)
}

func (e *Engine) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shuttingDown = true
	for _, cancel := range e.active {
		cancel()
	}
}
func (e *Engine) CloseProvider() error {
	if closer, ok := e.provider.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}
func (e *Engine) WaitForIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		e.mu.RLock()
		count := len(e.active)
		e.mu.RUnlock()
		if count == 0 {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}
