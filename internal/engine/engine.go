package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/selection"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	ExpectedBridgeVersion   = "0.6.0"
	ExpectedBridgeRevision  = "source-fidelity-v47"
	ExpectedBridgeID        = "aku-bridge-chrome-mv3-v0"
	ExpectedXAdapter        = "x-dom-v16"
	ExpectedLinkedInAdapter = "linkedin-dom-v13"
)

var expectedBridgeSources = []string{"x", "linkedin"}
var expectedBridgeActions = []string{
	"probe_readiness", "probe_freshness", "recover_source_freshness",
	"collect_visible", "detect_pending_content", "report_adapter_health",
	"report_capture_quality", "recover_missing_media", "extract_source_semantics",
	"report_frontier", "manage_source_tab_lifecycle", "manage_capture_window",
	"release_capture_surface", "preserve_working_tab", "report_source_events", "reload_self",
}

type Engine struct {
	store     *store.Store
	provider  reasoning.Provider
	config    config.Config
	epoch     string
	mu        sync.RWMutex
	heartbeat *domain.BridgeHeartbeat
	active    map[string]context.CancelFunc
	logger    Logger
	reloads   *ReloadActions
}

type Logger interface{ Printf(string, ...any) }

func New(state *store.Store, provider reasoning.Provider, cfg config.Config, logger Logger) *Engine {
	return &Engine{store: state, provider: provider, config: cfg, epoch: domain.NewID("epoch"), active: map[string]context.CancelFunc{}, logger: logger, reloads: NewReloadActions(15 * time.Second)}
}
func (e *Engine) Epoch() string        { return e.epoch }
func (e *Engine) ProviderName() string { return e.provider.Name() }
func (e *Engine) Settings(ctx context.Context) (domain.Settings, error) {
	return e.store.GetSettings(ctx)
}
func (e *Engine) SaveSettings(ctx context.Context, value domain.Settings) (domain.Settings, error) {
	value.ApplyProfile()
	if err := e.store.SaveSettings(ctx, value); err != nil {
		return domain.Settings{}, err
	}
	return e.store.GetSettings(ctx)
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
	status := BridgeStatus{State: "reconnecting", Expected: map[string]any{"bridgeId": ExpectedBridgeID, "extensionVersion": ExpectedBridgeVersion, "runtimeRevision": ExpectedBridgeRevision, "buildId": ExpectedBridgeBuildID, "adapterVersions": map[string]string{"x": ExpectedXAdapter, "linkedin": ExpectedLinkedInAdapter}, "contract": domain.BridgeContractVersion, "manifestVersion": 3, "sources": expectedBridgeSources, "actions": expectedBridgeActions, "authority": "read_only_bounded", "captureLimits": domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20}}}
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
	return domain.BridgeHeartbeat{BridgeID: ExpectedBridgeID, ExtensionVersion: ExpectedBridgeVersion, RuntimeRevision: ExpectedBridgeRevision, BuildID: ExpectedBridgeBuildID, AdapterVersions: map[string]string{"x": ExpectedXAdapter, "linkedin": ExpectedLinkedInAdapter}, ContractVersion: domain.BridgeContractVersion, ManifestVersion: 3, Sources: append([]string(nil), expectedBridgeSources...), Actions: append([]string(nil), expectedBridgeActions...), Authority: "read_only_bounded", CaptureLimits: domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20}}
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
	status := e.BridgeStatus()
	if !status.Compatible {
		return domain.Session{}, fmt.Errorf("AkuBridge v2 is not ready: %s", strings.Join(status.Reasons, "; "))
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Session{}, err
	}
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
	if err != nil || run == nil {
		return run, err
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
	e.mu.Lock()
	if _, exists := e.active[runID]; exists {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.active[runID] = cancel
	e.mu.Unlock()
	go func() {
		defer func() { e.mu.Lock(); delete(e.active, runID); e.mu.Unlock() }()
		if err := e.process(ctx, runID); err != nil {
			e.logger.Printf("run %s failed: %v", runID, err)
			failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning", Message: err.Error(), Retryable: true}
			_ = e.store.FailRun(context.Background(), runID, failure)
			run, _ := e.store.GetRun(context.Background(), runID)
			_, _ = e.startNext(context.Background(), run.SessionID)
		}
	}()
}

func (e *Engine) process(ctx context.Context, runID string) error {
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
	knowledge, err := e.store.Knowledge(ctx, run.Source, 20)
	if err != nil {
		return err
	}
	if len(observations) == 1 && e.config.Capture.MaxAcquisitionRounds > 1 {
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
	signals, err := e.store.PreferenceSignals(ctx)
	if err != nil {
		return err
	}
	converted := make([]preference.Signal, 0, len(signals))
	for _, signal := range signals {
		converted = append(converted, preference.Signal{Direction: signal.Direction, Reason: signal.Reason, Facets: signal.Assessment.TopicFacets})
	}
	profile := preference.Fit(converted)
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	scored := selection.Select(result.CandidateAssessments, profile, settings.MaxItemsPerSource, settings.PreferenceEligibilityMode)
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
	sort.SliceStable(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
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
	if err := e.store.FailCommand(ctx, commandID, runID, failure); err != nil {
		return domain.Run{}, err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err == nil {
		_, _ = e.startNext(ctx, run.SessionID)
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
func (e *Engine) Reset(ctx context.Context) error { return e.store.Reset(ctx) }

func (e *Engine) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, cancel := range e.active {
		cancel()
		delete(e.active, id)
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
