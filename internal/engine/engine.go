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
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/selection"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	ExpectedBridgeVersion       = "0.8.0"
	ExpectedBridgeRevision      = "source-adapters-v104"
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
	"release_capture_surface", "preserve_working_tab", "report_source_events", "reload_self",
}

type Engine struct {
	store         *store.Store
	provider      reasoning.Provider
	config        config.Config
	epoch         string
	mu            sync.RWMutex
	operation     sync.Mutex
	schedule      sync.Mutex
	heartbeat     *domain.BridgeHeartbeat
	bridgeOrigins map[string]time.Time
	active        map[string]context.CancelFunc
	pending       map[string]bool
	cancelled     map[string]bool
	shuttingDown  bool
	logger        Logger
	reloads       *ReloadActions
	events        *semanticengine.Engine
	aiFast        aidetector.FastDetector
	aiDeep        aidetector.Resolver
	mediaOrigin   mediaprovenance.Inspector
	autoCancel    context.CancelFunc
	autoWake      chan struct{}
}

type Logger interface{ Printf(string, ...any) }

func New(state *store.Store, provider reasoning.Provider, cfg config.Config, logger Logger, eventEngines ...*semanticengine.Engine) *Engine {
	var events *semanticengine.Engine
	if len(eventEngines) > 0 {
		events = eventEngines[0]
	}
	return &Engine{store: state, provider: provider, config: cfg, epoch: domain.NewID("epoch"), active: map[string]context.CancelFunc{}, pending: map[string]bool{}, cancelled: map[string]bool{}, bridgeOrigins: map[string]time.Time{}, logger: logger, reloads: NewReloadActions(15 * time.Second), events: events, autoWake: make(chan struct{}, 1)}
}
func (e *Engine) SetAIDeepResolver(value aidetector.Resolver) { e.aiDeep = value }
func (e *Engine) SetMediaProvenanceInspector(value mediaprovenance.Inspector) {
	e.mediaOrigin = value
}
func (e *Engine) MediaProvenanceRuntime() domain.MediaProvenanceRuntime {
	if e.mediaOrigin == nil {
		return domain.MediaProvenanceRuntime{Provider: mediaprovenance.ProviderName, Version: mediaprovenance.VerifierVersion}
	}
	return domain.MediaProvenanceRuntime{
		Provider: e.mediaOrigin.Name(), Version: e.mediaOrigin.Version(), Available: e.mediaOrigin.Available(),
	}
}
func (e *Engine) ResumeMediaProvenance(ctx context.Context) error {
	if e.mediaOrigin == nil || !e.mediaOrigin.Available() {
		return nil
	}
	if err := e.store.CancelRunningMediaProvenance(ctx); err != nil {
		return err
	}
	e.launchMediaProvenanceItems(nil)
	return nil
}
func (e *Engine) Epoch() string        { return e.epoch }
func (e *Engine) ProviderName() string { return e.provider.Name() }
func (e *Engine) Settings(ctx context.Context) (domain.Settings, error) {
	return e.store.GetSettings(ctx)
}
func (e *Engine) SaveSettings(ctx context.Context, value domain.Settings) (domain.Settings, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	current, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	var executableRuntime reasoning.ExecutableRuntime
	var resolvedExecutable string
	if strings.TrimSpace(value.ReasoningExecutablePath) != strings.TrimSpace(current.ReasoningExecutablePath) {
		var ok bool
		executableRuntime, ok = e.provider.(reasoning.ExecutableRuntime)
		if !ok {
			return domain.Settings{}, fmt.Errorf("%s does not expose an editable executable", e.ProviderName())
		}
		active, activeErr := e.store.ActiveSession(ctx)
		if activeErr != nil {
			return domain.Settings{}, activeErr
		}
		e.mu.RLock()
		activeWork := len(e.active) > 0
		e.mu.RUnlock()
		if active != nil || activeWork {
			return domain.Settings{}, errors.New("finish the active update or AI Deep Detection before changing the reasoning executable")
		}
		resolvedExecutable, err = executableRuntime.DiscoverExecutable(ctx, strings.TrimSpace(value.ReasoningExecutablePath))
		if err != nil {
			return domain.Settings{}, fmt.Errorf("validate reasoning executable: %w", err)
		}
		value.ReasoningExecutablePath = resolvedExecutable
	}
	// Keep the currently running provider's projection before accepting a
	// provider switch. The next runtime can restore the target provider's
	// projection without reusing (or overwriting) this one.
	profilesByProvider := make(map[string]domain.ReasoningProfileSet, len(current.ReasoningProfilesByProvider)+len(value.ReasoningProfilesByProvider)+1)
	for provider, profiles := range current.ReasoningProfilesByProvider {
		profilesByProvider[provider] = profiles
	}
	for provider, profiles := range value.ReasoningProfilesByProvider {
		profilesByProvider[provider] = profiles
	}
	if current.ReasoningProvider != "" {
		profilesByProvider[current.ReasoningProvider] = current.ActiveReasoningProfileSet()
	}
	if value.ReasoningProvider == current.ReasoningProvider && value.ReasoningProvider != "" {
		profilesByProvider[value.ReasoningProvider] = value.ActiveReasoningProfileSet()
	}
	value.ReasoningProfilesByProvider = profilesByProvider
	value.Normalize()
	if err := e.validateReasoningProfiles(value); err != nil {
		return domain.Settings{}, err
	}
	if err := e.store.SaveSettings(ctx, value); err != nil {
		return domain.Settings{}, err
	}
	if current.AIDetectionEnabled && !value.AIDetectionEnabled {
		e.cancelDeepDetections()
	}
	if executableRuntime != nil {
		executableRuntime.UseExecutable(resolvedExecutable)
	}
	saved, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	if _, err := e.store.EnforceRetention(ctx, saved); err != nil {
		return domain.Settings{}, fmt.Errorf("apply retention settings: %w", err)
	}
	select {
	case e.autoWake <- struct{}{}:
	default:
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
	State              string                     `json:"state"`
	Compatible         bool                       `json:"compatible"`
	Reasons            []string                   `json:"reasons"`
	Warnings           []string                   `json:"warnings"`
	Expected           map[string]any             `json:"expected"`
	Actual             *domain.BridgeHeartbeat    `json:"actual"`
	NegotiatedProtocol *BridgeProtocolNegotiation `json:"negotiatedProtocol,omitempty"`
}

type BridgeProtocolNegotiation struct {
	Major           int  `json:"major"`
	Minor           int  `json:"minor"`
	LegacyHeartbeat bool `json:"legacyHeartbeat"`
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
	if err := e.recoverExpiredBridgeCommands(context.Background(), time.Now()); err != nil {
		e.logger.Printf("recover expired Bridge command: %v", err)
	}
	e.reloads.Observe(value)
	select {
	case e.autoWake <- struct{}{}:
	default:
	}
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
	expectedSources := domain.SourceIDs()
	expectedAdapters := domain.ExpectedAdapterVersions()
	expectedMediaAdapters := domain.ExpectedMediaEvidenceAdapterVersions()
	requiredCapabilities := append([]string(nil), expectedBridgeActions...)
	requiredCapabilities = append(requiredCapabilities, "authority.read_only_bounded", "capture.bounded")
	status := BridgeStatus{State: "reconnecting", Expected: map[string]any{"bridgeId": ExpectedBridgeID, "extensionVersion": ExpectedBridgeVersion, "runtimeRevision": ExpectedBridgeRevision, "buildId": ExpectedBridgeBuildID, "protocolMajor": BridgeProtocolMajor, "minimumProtocolMinor": MinimumBridgeProtocolMinor, "supportedProtocolMinor": BridgeProtocolMinor, "adapterVersions": expectedAdapters, "mediaEvidenceAdapterVersions": expectedMediaAdapters, "contract": domain.BridgeContractVersion, "manifestVersion": 3, "sources": expectedSources, "actions": expectedBridgeActions, "requiredCapabilities": requiredCapabilities, "authority": "read_only_bounded", "captureLimits": domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}}}
	if e.heartbeat == nil {
		return status
	}
	copy := *e.heartbeat
	status.Actual = &copy
	status.State = "healthy"
	protocolMajor, protocolMinor, legacyHeartbeat := bridgeProtocol(copy)
	negotiatedMinor := protocolMinor
	if negotiatedMinor > BridgeProtocolMinor {
		negotiatedMinor = BridgeProtocolMinor
	}
	status.NegotiatedProtocol = &BridgeProtocolNegotiation{Major: protocolMajor, Minor: negotiatedMinor, LegacyHeartbeat: legacyHeartbeat}
	if copy.BridgeID != ExpectedBridgeID {
		status.Reasons = append(status.Reasons, "bridge id mismatch")
	}
	if protocolMajor != BridgeProtocolMajor {
		status.Reasons = append(status.Reasons, "bridge protocol major mismatch")
	}
	if protocolMinor < MinimumBridgeProtocolMinor {
		status.Reasons = append(status.Reasons, "bridge protocol minor is too old")
	}
	if protocolMinor > MaximumBridgeProtocolMinor {
		status.Reasons = append(status.Reasons, "bridge protocol minor is out of bounds")
	}
	if !validBoundedBridgeCapabilities(copy.UpdateCapabilities) {
		status.Reasons = append(status.Reasons, "bridge update capabilities are malformed or out of bounds")
	}
	if legacyHeartbeat {
		status.Warnings = append(status.Warnings, "legacy Bridge v2 heartbeat inferred as protocol 2.0")
	} else if protocolMinor > BridgeProtocolMinor {
		status.Warnings = append(status.Warnings, "bridge protocol minor is newer than this Sidecar")
	}
	if copy.ExtensionVersion != ExpectedBridgeVersion {
		status.Warnings = append(status.Warnings, "extension version differs from the Sidecar release reference")
	}
	if copy.RuntimeRevision != ExpectedBridgeRevision {
		status.Warnings = append(status.Warnings, "runtime revision differs from the Sidecar release reference")
	}
	if copy.ContractVersion != domain.BridgeContractVersion {
		status.Reasons = append(status.Reasons, "bridge contract mismatch")
	}
	if copy.BuildID != ExpectedBridgeBuildID {
		status.Warnings = append(status.Warnings, "bridge build differs from the Sidecar release reference")
	}
	if !hasRequiredStringMapKeys(copy.AdapterVersions, expectedAdapters) {
		status.Reasons = append(status.Reasons, "required source adapter capability missing")
	} else if !sameStringMap(copy.AdapterVersions, expectedAdapters) {
		status.Warnings = append(status.Warnings, "source adapter versions differ from the Sidecar release reference")
	}
	if !hasRequiredStringMapKeys(copy.MediaEvidenceAdapterVersions, expectedMediaAdapters) {
		status.Reasons = append(status.Reasons, "required media evidence capability missing")
	} else if !sameStringMap(copy.MediaEvidenceAdapterVersions, expectedMediaAdapters) {
		status.Warnings = append(status.Warnings, "media evidence adapter versions differ from the Sidecar release reference")
	}
	if copy.ManifestVersion != 3 {
		status.Reasons = append(status.Reasons, "manifest version mismatch")
	}
	if !containsRequiredStrings(copy.Sources, expectedSources) {
		status.Reasons = append(status.Reasons, "required bridge source capability missing")
	}
	if !containsRequiredStrings(copy.Actions, expectedBridgeActions) {
		status.Reasons = append(status.Reasons, "required bridge action capability missing")
	}
	if copy.Authority != "read_only_bounded" {
		status.Reasons = append(status.Reasons, "bridge authority mismatch")
	}
	if copy.CaptureLimits != (domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}) {
		status.Reasons = append(status.Reasons, "capture limits mismatch")
	}
	if !validBridgeSourceReadiness(copy.SourceAccess.Sources, copy.Sources) {
		status.Reasons = append(status.Reasons, "source readiness contract mismatch")
	}
	if len(e.config.Bridge.TrustedExtensionOrigins) > 0 && (copy.ExtensionOrigin == "" || !e.trustedBridgeExtensionOrigin(copy.ExtensionOrigin)) {
		status.Reasons = append(status.Reasons, "AkuBridge extension origin is not trusted; keep the Chrome Web Store extension or configure one exact development origin")
	}
	if len(e.bridgeOrigins) > 1 {
		status.Reasons = append(status.Reasons, "multiple AkuBridge extension origins are active; disable the legacy unpacked extension and keep only the intended AkuBridge installation")
	}
	status.Compatible = len(status.Reasons) == 0
	if !status.Compatible {
		status.State = "incompatible"
	} else if len(status.Warnings) > 0 {
		status.State = "degraded"
	}
	return status
}

func (e *Engine) trustedBridgeExtensionOrigin(actual string) bool {
	actual = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(actual)), "/")
	// Directly constructed engines are used by deterministic unit tests. The
	// executable configuration path always validates a non-empty allowlist.
	if len(e.config.Bridge.TrustedExtensionOrigins) == 0 {
		return actual == ""
	}
	for _, allowed := range e.config.Bridge.TrustedExtensionOrigins {
		if actual == strings.TrimSuffix(strings.ToLower(strings.TrimSpace(allowed)), "/") {
			return true
		}
	}
	return false
}

func ExpectedHeartbeat() domain.BridgeHeartbeat {
	readiness := make([]domain.BridgeSourceReadiness, 0, len(domain.SourceIDs()))
	for _, source := range domain.SourceIDs() {
		readiness = append(readiness, domain.BridgeSourceReadiness{Source: source, PermissionGranted: true, ScriptRegistered: true, Ready: true, Reason: "ready"})
	}
	return domain.BridgeHeartbeat{BridgeID: ExpectedBridgeID, ExtensionVersion: ExpectedBridgeVersion, RuntimeRevision: ExpectedBridgeRevision, BuildID: ExpectedBridgeBuildID, ProtocolMajor: BridgeProtocolMajor, ProtocolMinor: BridgeProtocolMinor, UpdateCapabilities: []string{"background_check", "staged_apply", "idle_deferral", "rollback_status"}, AdapterVersions: domain.ExpectedAdapterVersions(), MediaEvidenceAdapterVersions: domain.ExpectedMediaEvidenceAdapterVersions(), ContractVersion: domain.BridgeContractVersion, ManifestVersion: 3, Sources: domain.SourceIDs(), Actions: append([]string(nil), expectedBridgeActions...), Authority: "read_only_bounded", CaptureLimits: domain.BridgeCaptureLimits{MaxScrolls: 6, MaxSnapshots: 7, MaxBlocksPerSnapshot: 20, MaxMediaPerBlock: 20}, SourceAccess: domain.BridgeSourceAccess{GrantedSources: domain.SourceIDs(), Sources: readiness, ObservedAt: domain.Now()}}
}

func bridgeProtocol(value domain.BridgeHeartbeat) (major, minor int, legacy bool) {
	if value.ProtocolMajor == 0 && value.ProtocolMinor == 0 && value.ContractVersion == domain.BridgeContractVersion {
		return BridgeProtocolMajor, 0, true
	}
	return value.ProtocolMajor, value.ProtocolMinor, false
}

func sameStringMap(actual, expected map[string]string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func hasRequiredStringMapKeys(actual, required map[string]string) bool {
	for key := range required {
		if strings.TrimSpace(actual[key]) == "" {
			return false
		}
	}
	return true
}

func containsRequiredStrings(actual, required []string) bool {
	values := make(map[string]bool, len(actual))
	for _, value := range actual {
		values[value] = true
	}
	for _, value := range required {
		if !values[value] {
			return false
		}
	}
	return true
}

func validBoundedBridgeCapabilities(values []string) bool {
	if len(values) > MaxBridgeUpdateCapabilities {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) || len(value) > MaxBridgeCapabilityLength || seen[value] {
			return false
		}
		for index := 0; index < len(value); index++ {
			character := value[index]
			if index == 0 && (character < 'a' || character > 'z') {
				return false
			}
			if index > 0 && (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
				return false
			}
		}
		seen[value] = true
	}
	return true
}

func validBridgeSourceReadiness(actual []domain.BridgeSourceReadiness, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	values := make(map[string]domain.BridgeSourceReadiness, len(actual))
	for _, value := range actual {
		if _, duplicate := values[value.Source]; duplicate {
			return false
		}
		if value.Ready != (value.PermissionGranted && value.ScriptRegistered) {
			return false
		}
		values[value.Source] = value
	}
	for _, source := range expected {
		if _, ok := values[source]; !ok {
			return false
		}
	}
	return true
}

func (e *Engine) StartVisibleUpdate(ctx context.Context, intent string) (domain.Session, error) {
	trigger := domain.UpdateTriggerUser
	firstRunStatus, err := e.store.CalibrationFirstRunStatus(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if firstRunStatus == "pending" {
		trigger = domain.UpdateTriggerOnboarding
	}
	return e.startSession(ctx, intent, domain.UpdatePolicy{
		Trigger: trigger, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
}

func (e *Engine) grantedActiveSources(settings domain.Settings) []domain.Source {
	status := e.BridgeStatus()
	if !status.Compatible || status.Actual == nil {
		return nil
	}
	ready := make(map[string]bool, len(status.Actual.SourceAccess.Sources))
	for _, source := range status.Actual.SourceAccess.Sources {
		ready[source.Source] = source.Ready
	}
	valid := make([]domain.Source, 0, len(settings.ActiveSources))
	for _, source := range settings.ActiveSources {
		if ready[string(source)] {
			valid = append(valid, source)
		}
	}
	return valid
}

func noGrantedActiveSourceError() error {
	return errors.New("No active source is ready in AkuBridge. Open AkuBridge setup, enable at least one active source, accept Chrome's permission prompt, and let its capture script register.")
}

func (e *Engine) startSession(ctx context.Context, intent string, policy domain.UpdatePolicy) (domain.Session, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if err := policy.Validate(); err != nil {
		return domain.Session{}, err
	}
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
	validSources := e.grantedActiveSources(settings)
	if len(validSources) == 0 {
		return domain.Session{}, noGrantedActiveSourceError()
	}
	settings.ActiveSources = validSources
	e.cancelDeepDetections()
	session, err := e.store.CreateUpdateSession(ctx, intent, settings, policy)
	if err != nil {
		return domain.Session{}, err
	}
	if _, err := e.startNext(ctx, session.ID); err != nil {
		return domain.Session{}, err
	}
	return e.store.GetSession(ctx, session.ID)
}

func (e *Engine) startNext(ctx context.Context, sessionID string) (*domain.Run, error) {
	e.schedule.Lock()
	defer e.schedule.Unlock()
	session, err := e.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != "queued" && session.Status != "running" {
		return nil, nil
	}
	run, err := e.store.AdvanceSession(ctx, sessionID)
	if err != nil {
		return run, err
	}
	if run == nil {
		for _, candidate := range session.Runs {
			switch candidate.Status {
			case "completed", "failed", "cancelled":
			default:
				return nil, nil
			}
		}
		settings, settingsErr := e.store.GetSettings(ctx)
		if settingsErr != nil {
			return nil, settingsErr
		}
		calibrationFirstRunStatus, calibrationStatusErr := e.store.CalibrationFirstRunStatus(ctx)
		if calibrationStatusErr != nil {
			return nil, calibrationStatusErr
		}
		onboardingFastPath := calibrationFirstRunStatus == "pending"
		if e.events != nil && settings.SemanticEventMode != "show_all" {
			if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "semantic_event_resolution"); stageErr != nil {
				return nil, stageErr
			}
			var eventErr error
			if onboardingFastPath {
				_, eventErr = e.events.ProcessOnboardingSession(ctx, sessionID, settings)
			} else {
				_, eventErr = e.events.ProcessSession(ctx, sessionID, settings)
			}
			if eventErr != nil {
				e.logger.Printf("semantic event resolution for session %s degraded safely: %v", sessionID, eventErr)
				e.pauseAutoUpdateForUsageLimit(context.Background(), sessionID, eventErr)
			}
		}
		if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "timeline_composition"); stageErr != nil {
			return nil, stageErr
		}
		if composeErr := e.store.ComposeSession(ctx, sessionID); composeErr != nil {
			return nil, fmt.Errorf("compose unified Timeline: %w", composeErr)
		}
		if settings.AIDetectionEnabled && !onboardingFastPath {
			if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "ai_fast_detection"); stageErr != nil {
				return nil, stageErr
			}
			if detectionErr := e.runFastDetection(ctx, sessionID); detectionErr != nil {
				e.logger.Printf("AI Fast Detection for session %s degraded safely: %v", sessionID, detectionErr)
			}
		}
		if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "finalizing"); stageErr != nil {
			return nil, stageErr
		}
		if finalizeErr := e.store.FinalizeSession(ctx, sessionID); finalizeErr != nil {
			return nil, fmt.Errorf("finalize unified session: %w", finalizeErr)
		}
		completedSession, policyErr := e.store.GetSession(ctx, sessionID)
		if policyErr != nil {
			return nil, policyErr
		}
		if completedSession.Delivery == domain.UpdateDeliveryPrepared {
			if recordErr := e.store.RecordAutoUpdateSuccess(ctx); recordErr != nil {
				e.logger.Printf("record auto update success for session %s failed: %v", sessionID, recordErr)
			}
			select {
			case e.autoWake <- struct{}{}:
			default:
			}
		}
		if _, retentionErr := e.store.EnforceRetention(ctx, settings); retentionErr != nil {
			e.logger.Printf("retention after session %s failed: %v", sessionID, retentionErr)
		}
		if _, calibrationErr := e.ensurePendingFirstCalibration(ctx, sessionID); calibrationErr != nil {
			e.logger.Printf("first-run calibration for session %s could not start: %v", sessionID, calibrationErr)
		}
		if settings.AIDetectionEnabled && !onboardingFastPath && (completedSession.BudgetAuthority != domain.BudgetAuthorityAutomatic || e.autoDeepDetectionAllowed(ctx, settings)) {
			e.launchDeepDetection(sessionID)
		}
		if settings.AIDetectionEnabled && !onboardingFastPath {
			e.launchMediaProvenance(sessionID)
		}
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
	items, err := e.store.ListSessionItems(context.Background(), sessionID)
	if err != nil || len(items) == 0 {
		if err != nil {
			e.logger.Printf("AI Deep Detection could not load session %s: %v", sessionID, err)
		}
		return
	}
	e.launchDeepDetectionItems(sessionID, items)
}

func (e *Engine) launchMediaProvenance(sessionID string) {
	inspector := e.mediaOrigin
	if inspector == nil || !inspector.Available() {
		return
	}
	items, err := e.store.ListSessionItems(context.Background(), sessionID)
	if err != nil {
		e.logger.Printf("C2PA image provenance could not load session %s: %v", sessionID, err)
		return
	}
	e.launchMediaProvenanceItems(items)
}

func (e *Engine) launchMediaProvenanceItems(items []domain.TimelineItem) {
	inspector := e.mediaOrigin
	if inspector == nil || !inspector.Available() {
		return
	}
	if len(items) > 0 {
		if _, err := e.store.QueueMediaProvenance(context.Background(), items, inspector.Name(), inspector.Version()); err != nil {
			e.logger.Printf("C2PA image provenance queue degraded safely: %v", err)
			return
		}
	}
	const key = "media-provenance"
	e.mu.Lock()
	if _, active := e.active[key]; active {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.active[key] = cancel
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.active, key)
			e.mu.Unlock()
		}()
		for {
			assessment, err := e.store.ClaimMediaProvenance(ctx, inspector.Version())
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.logger.Printf("C2PA image provenance claim degraded safely: %v", err)
				}
				return
			}
			if assessment == nil {
				return
			}
			descriptor, ok := domain.SourceByID(assessment.Source)
			if !ok {
				_ = e.store.FinishMediaProvenance(context.Background(), assessment.ID, mediaprovenance.Result{
					ManifestState: "unsupported", TrustState: "not_applicable", AIOrigin: "unknown",
				}, fmt.Errorf("source %q has no media provenance contract", assessment.Source))
				continue
			}
			result, inspectErr := inspector.Inspect(ctx, *assessment, descriptor.TrustedMediaHostSuffixes)
			if finishErr := e.store.FinishMediaProvenance(context.Background(), assessment.ID, result, inspectErr); finishErr != nil {
				e.logger.Printf("C2PA image provenance result %s could not persist: %v", assessment.ID, finishErr)
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
		}
	}()
}

func (e *Engine) launchDeepDetectionItems(sessionID string, items []domain.TimelineItem) {
	resolver := e.aiDeep
	if resolver == nil {
		return
	}
	items = aidetector.DeepCandidates(items)
	if len(items) == 0 {
		return
	}
	e.mu.Lock()
	for id, cancel := range e.active {
		if strings.HasPrefix(id, "ai:"+sessionID+":") {
			cancel()
		}
	}
	e.mu.Unlock()
	settings, settingsErr := e.store.GetSettings(context.Background())
	if settingsErr != nil {
		e.logger.Printf("AI Deep Detection could not load reasoning profile for session %s: %v", sessionID, settingsErr)
		return
	}
	if !settings.AIDetectionEnabled {
		return
	}
	model := resolver.Model()
	if profiled, ok := resolver.(aidetector.ProfiledResolver); ok {
		model = profiled.ModelForProfile(settings.ReasoningAIDeepProfile)
	}
	job, err := e.store.CreateAIDetectionJob(context.Background(), domain.AIDetectionJob{
		SessionID: sessionID, Status: "queued", Provider: resolver.Name(), Model: model.Model,
		Effort: model.Effort, CandidateCount: len(items),
	})
	if err != nil {
		e.logger.Printf("AI Deep Detection could not queue session %s: %v", sessionID, err)
		return
	}
	key := "ai:" + sessionID + ":" + job.ID
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
		var result domain.DeepAIResult
		var usage domain.ModelUsage
		var duration time.Duration
		var resolveErr error
		if profiled, ok := resolver.(aidetector.ProfiledResolver); ok {
			result, usage, duration, resolveErr = profiled.ResolveWithProfile(ctx, items, settings.ReasoningAIDeepProfile)
		} else {
			result, usage, duration, resolveErr = resolver.Resolve(ctx, items)
		}
		if resolveErr != nil {
			status := "failed"
			if errors.Is(resolveErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				status = "cancelled"
			}
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, status, duration.Milliseconds(), usage, resolveErr)
			if status != "cancelled" {
				e.logger.Printf("AI Deep Detection for session %s degraded safely: %v", sessionID, resolveErr)
				e.pauseAutoUpdateForUsageLimit(context.Background(), sessionID, resolveErr)
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
		if strings.HasPrefix(id, "ai:") || id == "media-provenance" {
			cancel()
		}
	}
}

func capturePayload(run domain.Run, leaseID string, settings domain.Settings, round int, continuation map[string]any, reason string) map[string]any {
	return map[string]any{"mode": "catch_up", "source": run.Source, "sourceHydrationTimeoutMs": settings.SourceHydrationTimeout(run.Source), "scrolls": settings.MaxScrolls, "scrollFraction": 0.75, "scrollSettleMs": 900, "captureTimeoutMs": 45000, "pendingContentPolicy": map[bool]string{true: "reveal_if_present", false: "detect_only"}[round == 1], "sameTabMutationAllowed": round == 1, "pendingContentTimeoutMs": 5000, "pendingContentSettleMs": 700, "sourceFreshnessPolicy": map[bool]string{true: "wake_and_reveal", false: "preserve_frontier"}[round == 1], "captureVisibilityPolicy": settings.CaptureVisibility, "captureLeaseId": leaseID, "maxBlocksPerSnapshot": 20, "maxBlockCharacters": 4000, "qualityReportRequired": true, "qualityRetryBudget": 1, "qualityRetrySettleMs": settings.QualityRetrySettleMS, "openIfMissing": round == 1 && settings.OpenMissingSource, "tabLifecycle": map[string]any{"ownership": "shared", "openedTabDisposition": "preserve"}, "restoreScroll": true, "browserAdapter": "aku-bridge", "acquisitionRound": round, "maxAcquisitionRounds": 2, "continuation": continuation, "followUpReason": reason}
}

func (e *Engine) ClaimCommand(ctx context.Context, runID, bridgeID string) (*domain.BridgeCommand, error) {
	if err := e.recoverExpiredBridgeCommands(ctx, time.Now()); err != nil {
		return nil, err
	}
	return e.store.ClaimCommand(ctx, runID, bridgeID)
}
func (e *Engine) PendingBridgeRunID(ctx context.Context) (string, error) {
	if err := e.recoverExpiredBridgeCommands(ctx, time.Now()); err != nil {
		return "", err
	}
	return e.store.PendingBridgeRunID(ctx)
}

func (e *Engine) recoverExpiredBridgeCommands(ctx context.Context, now time.Time) error {
	commands, err := e.store.ExpiredBridgeCommands(ctx, now)
	if err != nil {
		return err
	}
	for _, command := range commands {
		failure := domain.Failure{
			Code:      "capture_lease_expired",
			Stage:     "capture",
			Message:   "AkuBridge did not finish the claimed capture before its bounded lease expired.",
			Retryable: true,
			Details: map[string]any{
				"commandId": command.ID,
				"claimedAt": command.ClaimedAt,
			},
		}
		if _, err := e.FailCommand(ctx, command.ID, command.RunID, failure); err != nil {
			if strings.Contains(err.Error(), "bridge command is not active") {
				continue
			}
			return err
		}
	}
	return nil
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

func (e *Engine) RecordCaptureSurfaceEvent(ctx context.Context, value domain.CaptureSurfaceEvent) (domain.CaptureSurfaceEvent, error) {
	return e.store.RecordCaptureSurfaceEvent(ctx, value)
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
	session, err := e.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return domain.Run{}, err
	}
	if sessionSourceWaitMode(session) == "progressive_wait" {
		if _, err := e.startNext(ctx, run.SessionID); err != nil {
			return domain.Run{}, err
		}
	}
	allowPlanning := true
	if firstRunStatus, statusErr := e.store.CalibrationFirstRunStatus(ctx); statusErr != nil {
		return domain.Run{}, statusErr
	} else if firstRunStatus == "pending" {
		allowPlanning = false
	}
	e.launchProcessAfterObservation(runID, allowPlanning)
	return e.store.GetRun(ctx, runID)
}

func sessionSourceWaitMode(session domain.Session) string {
	if value, ok := session.Coverage["sourceWaitMode"].(string); ok && value != "" {
		return value
	}
	return "full_wait"
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

func (e *Engine) launchProcess(runID string, allowPlanning bool) {
	e.launchProcessWithPolicy(runID, allowPlanning, false)
}

func (e *Engine) launchProcessAfterObservation(runID string, allowPlanning bool) {
	e.launchProcessWithPolicy(runID, allowPlanning, true)
}

func (e *Engine) launchProcessWithPolicy(runID string, allowPlanning, queueIfActive bool) {
	e.mu.Lock()
	if e.shuttingDown {
		e.mu.Unlock()
		return
	}
	if e.cancelled[runID] {
		e.mu.Unlock()
		return
	}
	if _, exists := e.active[runID]; exists {
		if queueIfActive {
			queuedAllowPlanning, queued := e.pending[runID]
			e.pending[runID] = allowPlanning || (queued && queuedAllowPlanning)
		}
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.active[runID] = cancel
	e.mu.Unlock()
	go func() {
		succeeded := false
		defer func() {
			e.mu.Lock()
			delete(e.active, runID)
			pendingAllowPlanning, pending := e.pending[runID]
			delete(e.pending, runID)
			relaunch := succeeded && pending && !e.cancelled[runID] && !e.shuttingDown
			e.mu.Unlock()
			if relaunch {
				e.launchProcess(runID, pendingAllowPlanning)
			}
		}()
		if err := e.process(ctx, runID, allowPlanning); err != nil {
			if e.shouldPauseForShutdown(ctx, err) {
				e.logger.Printf("run %s paused during shutdown and will resume from accepted capture", runID)
				return
			}
			e.logger.Printf("run %s failed: %v", runID, err)
			if failErr := e.failRunAfterProcessError(context.Background(), runID, err); failErr != nil {
				e.logger.Printf("record failed run %s: %v", runID, failErr)
			}
			return
		}
		succeeded = true
	}()
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

func (e *Engine) failRunAfterProcessError(ctx context.Context, runID string, processErr error) error {
	usageLimit := reasoning.IsUsageLimitError(processErr)
	failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning", Message: boundedFailureMessage(processErr), Retryable: true}
	if providerFailure, ok := reasoning.ProviderFailureFrom(processErr); ok {
		failure.Details = map[string]any{"sdkCode": providerFailure.Code, "sdkStage": providerFailure.Stage, "sdkRetry": providerFailure.Retry}
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
	if err := e.store.FailRun(ctx, runID, failure); err != nil {
		return err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if usageLimit {
		e.pauseAutoUpdateForUsageLimit(ctx, run.SessionID, processErr)
		if err := e.store.FailQueuedRuns(ctx, run.SessionID, failure); err != nil {
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
	select {
	case e.autoWake <- struct{}{}:
	default:
	}
	return true
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
	progressiveSessions := map[string]bool{}
	for _, id := range ids {
		run, runErr := e.store.GetRun(ctx, id)
		if runErr != nil {
			return 0, runErr
		}
		if !progressiveSessions[run.SessionID] {
			session, sessionErr := e.store.GetSession(ctx, run.SessionID)
			if sessionErr != nil {
				return 0, sessionErr
			}
			if sessionSourceWaitMode(session) == "progressive_wait" {
				if _, scheduleErr := e.startNext(ctx, run.SessionID); scheduleErr != nil {
					return 0, scheduleErr
				}
			}
			progressiveSessions[run.SessionID] = true
		}
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
	mergeDurableRunCoverage(merged.Coverage, run.Coverage)
	if len(observations) > 1 {
		if receipt, ok := merged.Coverage["acquisitionPlanning"].(map[string]any); ok {
			receipt = cloneCoverageMap(receipt)
			receipt["followUpNewCandidates"] = followUpNewCandidateCount(observations)
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return fmt.Errorf("save acquisition planning yield: %w", err)
			}
		}
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	continuity, err := e.store.ClassifyContentContinuity(ctx, run, merged, settings)
	if err != nil {
		return fmt.Errorf("classify resurfaced content: %w", err)
	}
	merged = filterResurfacedObservation(merged, continuity)
	if observationCandidateCount(merged) == 0 {
		skipped := skippedResurfaceCount(continuity)
		if _, exists := merged.Coverage["acquisitionPlanning"]; !exists {
			receipt := map[string]any{
				"mode":                  "continuity_bypass",
				"decision":              "finish",
				"reason":                "All captured candidates were resolved as unchanged native resurfaces before acquisition planning.",
				"followUpQueued":        false,
				"followUpNewCandidates": 0,
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return fmt.Errorf("save continuity bypass receipt: %w", err)
			}
		}
		result := domain.ReasoningResult{
			Summary: fmt.Sprintf("%d unchanged resurfaced item%s skipped before reasoning inside the configured cooldown.", skipped, map[bool]string{true: "", false: "s"}[skipped == 1]),
			Items:   []domain.ReasonedItem{}, CandidateAssessments: []domain.CandidateAssessment{}, Limitations: []string{},
		}
		addedStarted := time.Now()
		if err = e.store.CompleteRun(ctx, run, result, nil, nil, merged.Coverage); err != nil {
			return err
		}
		_ = e.store.SaveRunStageTiming(context.Background(), runID, "added", time.Since(addedStarted))
		_, err = e.startNext(ctx, run.SessionID)
		return err
	}
	knowledge, err := e.store.Knowledge(ctx, run.Source, 200)
	if err != nil {
		return err
	}
	if allowPlanning && len(observations) == 1 && e.config.Capture.MaxAcquisitionRounds > 1 {
		localMode := "local_frontier"
		localReason, localFinish := localStructuredFallbackCompletionReason(run.Source, observations[0])
		if localFinish {
			localMode = "structured_fallback"
		} else {
			localReason, localFinish = localAcquisitionCompletionReason(run.Source, observations[0], observationCandidateCount(merged))
		}
		if localFinish {
			receipt := map[string]any{
				"mode":                  localMode,
				"decision":              "finish",
				"reason":                localReason,
				"followUpQueued":        false,
				"followUpNewCandidates": 0,
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return fmt.Errorf("save local acquisition planning receipt: %w", err)
			}
		} else {
			if err := e.store.SetRunPipelineStage(ctx, runID, "acquisition_planning"); err != nil {
				return err
			}
			plan, telemetry, planErr := e.planWithProfile(ctx, run, merged, knowledge, settings.ReasoningAcquisitionProfile)
			_ = e.store.SaveTelemetry(context.Background(), telemetry)
			if planErr != nil {
				return planErr
			}
			receipt := map[string]any{
				"mode":                  "model",
				"decision":              plan.Decision,
				"reason":                plan.Reason,
				"followUpQueued":        false,
				"followUpNewCandidates": 0,
			}
			if plan.Decision == "request_follow_up" {
				continuation := continuationFrom(merged)
				if continuation != nil {
					payload := capturePayload(run, run.SessionID, settings, 2, continuation, plan.Reason)
					if _, err = e.store.QueueFollowUp(ctx, runID, payload); err != nil {
						return err
					}
					receipt["followUpQueued"] = true
				}
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return fmt.Errorf("save model acquisition planning receipt: %w", err)
			}
			if receipt["followUpQueued"] == true {
				return nil
			}
		}
	}
	if err := e.store.SetRunPipelineStage(ctx, runID, "candidate_evaluation"); err != nil {
		return err
	}
	result, telemetry, err := e.analyzeWithProfile(ctx, run, merged, knowledge, settings.ReasoningEvaluationProfile)
	_ = e.store.SaveTelemetry(context.Background(), telemetry)
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "evaluated", time.Duration(telemetry.DurationMS)*time.Millisecond)
	if err != nil {
		return err
	}
	bindReasoningSourceURLs(merged, &result)
	if err = validateReasoning(merged, result); err != nil {
		return err
	}
	profile, err := e.preferenceProfile(ctx, true)
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
	for evidenceKey, decision := range continuity {
		if decision.Action == "evaluate" && (decision.Status == "resurfaced_changed" || decision.Status == "resurfaced_after_cooldown") {
			delete(excluded, evidenceKey)
		}
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
	selectionStarted := time.Now()
	scored := selection.SelectWithOptions(result.CandidateAssessments, profile, selection.Options{
		Limit: settings.MaxItemsPerSource, Mode: settings.PreferenceEligibilityMode,
		ExcludedEvidence: excluded, ProtectedEvidence: protected,
	})
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "selected", time.Since(selectionStarted))
	items := selectedItems(run, result, scored, merged.Coverage)
	addedStarted := time.Now()
	if err = e.store.CompleteRun(ctx, run, result, scored, items, merged.Coverage); err != nil {
		return err
	}
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "added", time.Since(addedStarted))
	_, err = e.startNext(ctx, run.SessionID)
	return err
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

func mergeDurableRunCoverage(target, durable map[string]any) {
	for key, value := range durable {
		if key == "rounds" || key == "acquisitionRounds" {
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
	session, err := e.store.GetSession(ctx, id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	if e.cancelled == nil {
		e.cancelled = map[string]bool{}
	}
	for _, run := range session.Runs {
		e.cancelled[run.ID] = true
		delete(e.pending, run.ID)
		if cancel, active := e.active[run.ID]; active {
			cancel()
		}
	}
	e.mu.Unlock()
	return e.store.CancelSession(ctx, id)
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
	for key, cancel := range e.active {
		if strings.HasPrefix(key, "ai:"+value.SessionID+":") {
			cancel()
		}
	}
	e.mu.Unlock()
	_ = e.store.RefreshEventResolutionCounts(ctx, value.SessionID)
	return value, nil
}

func (e *Engine) QueueMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode) (domain.MediaRecapture, error) {
	return e.QueueMediaRecaptureForReason(ctx, timelineID, mode, domain.MediaRecaptureMissingMedia)
}

func (e *Engine) QueueMediaRecaptureForReason(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode, reason domain.MediaRecaptureReason) (domain.MediaRecapture, error) {
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
	return e.store.CreateMediaRecaptureForReason(ctx, timelineID, mode, reason)
}

func (e *Engine) ClaimMediaRecapture(ctx context.Context, id, bridgeID string) (domain.MediaRecapture, error) {
	return e.store.ClaimMediaRecapture(ctx, id, bridgeID)
}

func (e *Engine) AcceptMediaRecapture(ctx context.Context, id string, observation domain.Observation) (domain.MediaRecapture, error) {
	normalizeObservation(&observation)
	if err := validateObservation(observation); err != nil {
		return domain.MediaRecapture{}, err
	}
	value, err := e.store.CompleteMediaRecapture(ctx, id, observation)
	if err == nil {
		if item, loadErr := e.store.TimelineItem(ctx, value.TimelineID); loadErr == nil {
			e.launchMediaProvenanceItems([]domain.TimelineItem{item})
		}
	}
	return value, err
}

func (e *Engine) FailMediaRecapture(ctx context.Context, id string, failure domain.Failure) (domain.MediaRecapture, error) {
	return e.store.FailMediaRecapture(ctx, id, failure)
}

func (e *Engine) ApplyPassiveXMediaEvidence(ctx context.Context, timelineID, bridgeID string, value domain.PassiveXMediaEvidence) (domain.MediaRecapture, bool, error) {
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
	defaults.ReasoningExecutablePath = e.ReasoningRuntime().ExecutablePath
	return e.store.FullReset(ctx, defaults)
}

func (e *Engine) Shutdown() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.shuttingDown = true
	if e.autoCancel != nil {
		e.autoCancel()
	}
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

func (e *Engine) DiagnosticsExport(ctx context.Context, since, until string, sessionID string) (domain.DiagnosticsExport, error) {
	var sinceTime, untilTime time.Time
	var err error
	if since != "" {
		sinceTime, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
	}
	if until != "" {
		untilTime, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
	} else {
		untilTime = time.Now().UTC()
	}
	if sinceTime.IsZero() {
		sinceTime = untilTime.Add(-24 * time.Hour)
	}

	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.DiagnosticsExport{}, err
	}

	activeSession, _ := e.ActiveSession(ctx)
	var targetSession *domain.Session
	if sessionID != "" {
		s, err := e.Session(ctx, sessionID)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
		targetSession = &s
	} else if activeSession != nil {
		targetSession = activeSession
	}

	var sessions []domain.SessionExport
	if targetSession != nil {
		sessions = []domain.SessionExport{exportSession(*targetSession)}
	} else {
		inbox, _, err := e.store.ListInboxSessions(ctx, 10, 0)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
		sessions = make([]domain.SessionExport, len(inbox))
		for i, is := range inbox {
			sessions[i] = exportInboxSession(is)
		}
	}

	reasoningTelemetry := e.collectReasoningTelemetry(ctx, sessions)
	modelUsage, _ := e.store.AggregateModelUsage(ctx, int(untilTime.Sub(sinceTime).Hours()/24+1))
	bridgeStatus := e.BridgeStatus()

	return domain.DiagnosticsExport{
		Metadata: domain.DiagnosticsMetadata{
			Version:       domain.ApplicationVersion,
			InstanceEpoch: e.epoch,
			TimeRange: domain.TimeRange{
				Since: sinceTime.Format(time.RFC3339),
				Until: untilTime.Format(time.RFC3339),
			},
			PIIRedacted: true,
			SessionID:   sessionID,
		},
		Config:      exportConfig(settings, e),
		Sessions:    sessions,
		ModelUsage:  exportModelUsage(modelUsage),
		Reasoning:   reasoningTelemetry,
		Bridge:      convertBridgeStatus(bridgeStatus),
		Errors:      []domain.ErrorEvent{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func convertBridgeStatus(bs BridgeStatus) domain.BridgeStatus {
	return domain.BridgeStatus{
		State:              bs.State,
		Compatible:         bs.Compatible,
		Reasons:            bs.Reasons,
		Warnings:           bs.Warnings,
		Expected:           fmt.Sprintf("%v", bs.Expected),
		Actual:             fmt.Sprintf("%v", bs.Actual),
		NegotiatedProtocol: fmt.Sprintf("%v", bs.NegotiatedProtocol),
	}
}

func exportSession(s domain.Session) domain.SessionExport {
	runs := make([]domain.RunExport, len(s.Runs))
	for i, r := range s.Runs {
		runs[i] = exportRun(r)
	}
	return domain.SessionExport{
		ID:            s.ID,
		Intent:        s.Intent,
		Status:        s.Status,
		CreatedAt:     s.CreatedAt,
		CompletedAt:   s.CompletedAt,
		Runs:          runs,
		Coverage:      s.Coverage,
		PipelineStage: getPipelineStage(s),
	}
}

func exportInboxSession(is domain.InboxSession) domain.SessionExport {
	runs := make([]domain.RunExport, len(is.Runs))
	for i, r := range is.Runs {
		runs[i] = domain.RunExport{
			ID:            r.ID,
			Source:        r.Source,
			Status:        r.Status,
			Stage:         r.Stage,
			ElapsedMillis: r.TotalDurationMS,
		}
	}
	return domain.SessionExport{
		ID:          is.ID,
		Intent:      is.Intent,
		Status:      is.Status,
		CreatedAt:   is.CreatedAt,
		CompletedAt: is.CompletedAt,
		Runs:        runs,
	}
}

func exportRun(r domain.Run) domain.RunExport {
	re := domain.RunExport{
		ID:     r.ID,
		Source: r.Source,
		Status: r.Status,
		Stage:  r.Stage,
	}
	if r.Error != nil {
		re.Error = &domain.RunError{
			Code:    r.Error.Code,
			Message: r.Error.Message,
			Stage:   r.Stage,
		}
	}
	return re
}

func exportConfig(settings domain.Settings, e *Engine) domain.RedactedConfig {
	processes := e.ReasoningProcesses(settings)
	procInfo := make([]domain.ReasoningProcessInfo, len(processes))
	for i, p := range processes {
		procInfo[i] = domain.ReasoningProcessInfo{
			ID:        p.ID,
			Label:     p.Label,
			Provider:  p.Provider,
			Model:     p.Model,
			Effort:    p.Effort,
			Execution: p.Execution,
			ProfileID: p.ProfileID,
		}
	}
	return domain.RedactedConfig{
		ReasoningProvider:       settings.ReasoningProvider,
		ReasoningProcesses:      procInfo,
		ActiveSources:           settings.ActiveSources,
		TimelineCapacity:        settings.TimelineCapacity,
		AutoUpdateEnabled:       settings.AutoUpdateEnabled,
		AutoUpdateMode:          settings.AutoUpdateMode,
		LoadProfile:             settings.LoadProfile,
		AIDetectionEnabled:      settings.AIDetectionEnabled,
		ResurfaceMode:           settings.ResurfaceMode,
		BridgeContractVersion:   domain.BridgeContractVersion,
		DatabaseSchemaVersion:   store.SchemaVersion,
		ReasoningExecutablePath: settings.ReasoningExecutablePath,
	}
}

func exportModelUsage(mu domain.ModelUsageReport) domain.ModelUsageExport {
	cats := make([]domain.ModelUsageCategory, len(mu.Categories))
	for i, c := range mu.Categories {
		entries := make([]domain.ModelUsageEntry, len(c.Entries))
		for j, e := range c.Entries {
			entries[j] = domain.ModelUsageEntry{
				ID:                  e.ID,
				CategoryID:          c.ID,
				Source:              e.Source,
				Status:              e.Status,
				Provider:            e.Provider,
				Model:               e.Model,
				Effort:              e.Effort,
				InvocationCount:     e.InvocationCount,
				DurationMS:          e.DurationMS,
				CallerLatencyMS:     e.CallerLatencyMS,
				QueueWaitMS:         e.QueueWaitMS,
				ProviderExecutionMS: e.ProviderExecutionMS,
				ResponseTotalMS:     e.ResponseTotalMS,
				Usage:               e.Usage,
				UsageCoverage:       e.UsageCoverage,
				CreatedAt:           e.CreatedAt,
			}
		}
		cats[i] = domain.ModelUsageCategory{
			ID:              c.ID,
			Label:           c.Label,
			Execution:       c.Execution,
			Status:          c.Status,
			InvocationCount: c.InvocationCount,
			DurationMS:      c.DurationMS,
			Usage:           c.Usage,
			UsageCoverage:   c.UsageCoverage,
			Note:            c.Note,
			Entries:         entries,
		}
	}
	agg := mu.Usage
	return domain.ModelUsageExport{
		WindowDays:   mu.WindowDays,
		SessionCount: mu.SessionCount,
		GeneratedAt:  mu.GeneratedAt,
		Status:       mu.Status,
		AggregateTokens: domain.TokenUsage{
			InputTokens:           deref(agg.Input),
			CachedInputTokens:     deref(agg.CachedInput),
			OutputTokens:          deref(agg.Output),
			ReasoningOutputTokens: deref(agg.ReasoningOutput),
		},
		Categories: cats,
	}
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func (e *Engine) collectReasoningTelemetry(ctx context.Context, sessions []domain.SessionExport) []domain.ReasoningTelemetry {
	var telemetry []domain.ReasoningTelemetry
	for i := range sessions {
		invocations, err := e.store.ReasoningInvocationsBySession(ctx, sessions[i].ID)
		if err != nil {
			continue
		}
		byRun := map[string][]domain.ReasoningTelemetry{}
		for _, inv := range invocations {
			byRun[inv.RunID] = append(byRun[inv.RunID], inv)
		}
		for j := range sessions[i].Runs {
			runID := sessions[i].Runs[j].ID
			inv, ok := byRun[runID]
			if !ok || len(inv) == 0 {
				continue
			}
			var elapsed int64
			for _, t := range inv {
				elapsed += t.DurationMS
			}
			re := &sessions[i].Runs[j]
			re.Provider = inv[0].Provider
			re.Model = inv[0].Model
			re.Effort = inv[0].Effort
			re.ElapsedMillis = elapsed
			last := inv[len(inv)-1]
			re.Telemetry = &last
			telemetry = append(telemetry, inv...)
		}
	}
	return telemetry
}

func getPipelineStage(s domain.Session) string {
	if stage, ok := s.Coverage["pipelineStage"].(string); ok {
		return stage
	}
	return ""
}

func (e *Engine) RuntimeUpdateReadiness(ctx context.Context) (bool, string, error) {
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return false, "state_unavailable", err
	}
	if active != nil {
		return false, "active_session", nil
	}
	calibration, err := e.store.ActiveCalibration(ctx)
	if err != nil {
		return false, "state_unavailable", err
	}
	if calibration != nil {
		return false, "active_calibration", nil
	}
	recapturing, err := e.store.ActiveMediaRecapture(ctx)
	if err != nil {
		return false, "state_unavailable", err
	}
	if recapturing {
		return false, "active_media_recapture", nil
	}
	e.mu.RLock()
	activeWorkers := len(e.active)
	shuttingDown := e.shuttingDown
	e.mu.RUnlock()
	if shuttingDown {
		return false, "shutdown_in_progress", nil
	}
	if activeWorkers > 0 {
		return false, "background_work", nil
	}
	return true, "idle", nil
}
