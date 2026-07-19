package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"
)

const (
	ApplicationVersion              = "0.7.0-preview.2"
	BridgeContractVersion           = "aku-browser.bridge.v2"
	DefaultTimelineBatchGapPX       = 36
	DefaultTimelineBoundaryCueMode  = "follow"
	DefaultTimelineBoundaryReturnMS = 350
	DefaultSemanticShortlist        = 10
	DefaultSemanticMergeThreshold   = 0.92
	MinSemanticMergeThreshold       = 0.85
	MaxSemanticMergeThreshold       = 0.95
	DefaultRetentionDays            = 30
	DefaultStorageLimitMB           = 100
	DefaultAIDetectionPresentation  = "drawer"
	DefaultAIDetectionEnabled       = true
	DefaultResurfaceMode            = "smart"
	DefaultResurfaceCooldownDays    = 7
	DefaultReasoningAcquisition     = "luna_high"
	DefaultReasoningEvaluation      = "luna_xhigh"
	DefaultReasoningSemantic        = "luna_high"
	DefaultReasoningAIDeep          = "luna_high"
	DefaultSourceWaitMode           = "progressive_wait"
	AIHideConfirmationPhrase        = "HIDE STRONG AI SIGNALS"
	CurrentAIDeepDetectorVersion    = "codex-deep-v4"
)

type Source string

const (
	SourceX        Source = "x"
	SourceLinkedIn Source = "linkedin"
	SourceFacebook Source = "facebook"
)

func (s Source) Valid() bool {
	_, ok := SourceByID(s)
	return ok
}

type Settings struct {
	LoadProfile                 string         `json:"loadProfile"`
	CaptureVisibility           string         `json:"captureVisibility"`
	OpenMissingSource           bool           `json:"openMissingSource"`
	ActiveSources               []Source       `json:"activeSources"`
	SourceHydrationTimeoutMS    map[Source]int `json:"sourceHydrationTimeoutMs"`
	SourceWaitMode              string         `json:"sourceWaitMode"`
	TimelineCapacity            int            `json:"timelineCapacity"`
	MaxItemsPerSource           int            `json:"maxItemsPerSource"`
	MaxItemsTotal               int            `json:"maxItemsTotal"`
	MaxScrolls                  int            `json:"maxScrolls"`
	QualityRetrySettleMS        int            `json:"qualityRetrySettleMs"`
	PreferenceEligibilityMode   string         `json:"preferenceEligibilityMode"`
	CalibrationEnabled          bool           `json:"calibrationEnabled"`
	CalibrationBatchSize        int            `json:"calibrationBatchSize"`
	DefaultPresentation         string         `json:"defaultPresentation"`
	StreamWidth                 string         `json:"streamWidth"`
	TimelineBatchGapPX          int            `json:"timelineBatchGapPx"`
	TimelineBoundaryCueMode     string         `json:"timelineBoundaryCueMode"`
	TimelineBoundaryReturnMS    int            `json:"timelineBoundaryReturnMs"`
	SemanticEventMode           string         `json:"semanticEventMode"`
	SemanticEventShortlist      int            `json:"semanticEventShortlist"`
	SemanticEventMergeThreshold float64        `json:"semanticEventMergeThreshold"`
	KnowledgeRetentionDays      int            `json:"knowledgeRetentionDays"`
	KnowledgeStorageLimitMB     int            `json:"knowledgeStorageLimitMb"`
	AIDetectionPresentation     string         `json:"aiDetectionPresentation"`
	AIDetectionEnabled          bool           `json:"aiDetectionEnabled"`
	ResurfaceMode               string         `json:"resurfaceMode"`
	ResurfaceCooldownDays       int            `json:"resurfaceCooldownDays"`
	ReasoningExecutablePath     string         `json:"reasoningExecutablePath"`
	ReasoningAcquisitionProfile string         `json:"reasoningAcquisitionProfile"`
	ReasoningEvaluationProfile  string         `json:"reasoningEvaluationProfile"`
	ReasoningSemanticProfile    string         `json:"reasoningSemanticProfile"`
	ReasoningAIDeepProfile      string         `json:"reasoningAiDeepProfile"`
}

func DefaultSettings(profile, visibility, preferenceMode string, openMissing bool) Settings {
	settings := Settings{
		LoadProfile:                 profile,
		CaptureVisibility:           visibility,
		OpenMissingSource:           openMissing,
		ActiveSources:               DefaultSources(),
		SourceHydrationTimeoutMS:    DefaultSourceHydrationTimeouts(),
		SourceWaitMode:              DefaultSourceWaitMode,
		PreferenceEligibilityMode:   preferenceMode,
		CalibrationEnabled:          true,
		CalibrationBatchSize:        10,
		DefaultPresentation:         "source",
		StreamWidth:                 "social",
		TimelineBatchGapPX:          DefaultTimelineBatchGapPX,
		TimelineBoundaryCueMode:     DefaultTimelineBoundaryCueMode,
		TimelineBoundaryReturnMS:    DefaultTimelineBoundaryReturnMS,
		SemanticEventMode:           "collapse",
		SemanticEventShortlist:      DefaultSemanticShortlist,
		SemanticEventMergeThreshold: DefaultSemanticMergeThreshold,
		KnowledgeRetentionDays:      DefaultRetentionDays,
		KnowledgeStorageLimitMB:     DefaultStorageLimitMB,
		AIDetectionPresentation:     DefaultAIDetectionPresentation,
		AIDetectionEnabled:          DefaultAIDetectionEnabled,
		ResurfaceMode:               DefaultResurfaceMode,
		ResurfaceCooldownDays:       DefaultResurfaceCooldownDays,
		ReasoningAcquisitionProfile: DefaultReasoningAcquisition,
		ReasoningEvaluationProfile:  DefaultReasoningEvaluation,
		ReasoningSemanticProfile:    DefaultReasoningSemantic,
		ReasoningAIDeepProfile:      DefaultReasoningAIDeep,
	}
	settings.ApplyProfile()
	return settings
}

func (s *Settings) ApplyProfile() {
	switch s.LoadProfile {
	case "standard":
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 2, 5, 10
		s.TimelineCapacity, s.QualityRetrySettleMS = 12, 300
	case "expanded":
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 4, 10, 20
		s.TimelineCapacity, s.QualityRetrySettleMS = 24, 1000
	case "stress":
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 6, 15, 30
		s.TimelineCapacity, s.QualityRetrySettleMS = 36, 1000
	case "custom":
		// Custom keeps its explicit, policy-bounded values.
	default:
		s.LoadProfile = "standard"
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 2, 5, 10
		s.TimelineCapacity, s.QualityRetrySettleMS = 12, 300
	}
}

func (s *Settings) Normalize() {
	if s.SourceWaitMode == "" {
		s.SourceWaitMode = DefaultSourceWaitMode
	}
	if s.SourceHydrationTimeoutMS == nil {
		s.SourceHydrationTimeoutMS = DefaultSourceHydrationTimeouts()
	} else {
		for _, descriptor := range Sources() {
			if s.SourceHydrationTimeoutMS[descriptor.ID] == 0 {
				s.SourceHydrationTimeoutMS[descriptor.ID] = descriptor.HydrationTimeoutDefaultMS
			}
		}
	}
	if s.DefaultPresentation == "" {
		s.DefaultPresentation = "source"
	}
	if s.StreamWidth == "" {
		s.StreamWidth = "social"
	}
	if s.TimelineBatchGapPX == 0 {
		s.TimelineBatchGapPX = DefaultTimelineBatchGapPX
	}
	if s.TimelineBoundaryCueMode == "" {
		s.TimelineBoundaryCueMode = DefaultTimelineBoundaryCueMode
	}
	if s.TimelineBoundaryReturnMS == 0 {
		s.TimelineBoundaryReturnMS = DefaultTimelineBoundaryReturnMS
	}
	if s.SemanticEventMode == "" {
		s.SemanticEventMode = "collapse"
	}
	if s.SemanticEventShortlist == 0 {
		s.SemanticEventShortlist = DefaultSemanticShortlist
	}
	if s.SemanticEventMergeThreshold == 0 {
		s.SemanticEventMergeThreshold = DefaultSemanticMergeThreshold
	}
	if s.KnowledgeRetentionDays == 0 {
		s.KnowledgeRetentionDays = DefaultRetentionDays
	}
	if s.KnowledgeStorageLimitMB == 0 {
		s.KnowledgeStorageLimitMB = DefaultStorageLimitMB
	}
	if s.AIDetectionPresentation == "" {
		s.AIDetectionPresentation = DefaultAIDetectionPresentation
	}
	if s.ResurfaceMode == "" {
		s.ResurfaceMode = DefaultResurfaceMode
	}
	if s.ResurfaceCooldownDays == 0 {
		s.ResurfaceCooldownDays = DefaultResurfaceCooldownDays
	}
	if s.ReasoningAcquisitionProfile == "" {
		s.ReasoningAcquisitionProfile = DefaultReasoningAcquisition
	}
	if s.ReasoningEvaluationProfile == "" {
		s.ReasoningEvaluationProfile = DefaultReasoningEvaluation
	}
	if s.ReasoningSemanticProfile == "" {
		s.ReasoningSemanticProfile = DefaultReasoningSemantic
	}
	if s.ReasoningAIDeepProfile == "" {
		s.ReasoningAIDeepProfile = DefaultReasoningAIDeep
	}
	s.ApplyProfile()
}

func (s Settings) Validate() error {
	if s.LoadProfile != "standard" && s.LoadProfile != "expanded" && s.LoadProfile != "stress" && s.LoadProfile != "custom" {
		return fmt.Errorf("unsupported load profile %q", s.LoadProfile)
	}
	if s.CaptureVisibility != "quiet" && s.CaptureVisibility != "adaptive_fidelity" {
		return fmt.Errorf("unsupported capture visibility %q", s.CaptureVisibility)
	}
	if s.SourceWaitMode != "full_wait" && s.SourceWaitMode != "progressive_wait" {
		return fmt.Errorf("unsupported source wait mode %q", s.SourceWaitMode)
	}
	if s.PreferenceEligibilityMode != "rank_only" && s.PreferenceEligibilityMode != "promote_unused_budget" && s.PreferenceEligibilityMode != "guarded_live" {
		return fmt.Errorf("unsupported preference mode %q", s.PreferenceEligibilityMode)
	}
	if s.DefaultPresentation != "source" && s.DefaultPresentation != "brief" {
		return fmt.Errorf("unsupported default presentation %q", s.DefaultPresentation)
	}
	if s.StreamWidth != "compact" && s.StreamWidth != "social" && s.StreamWidth != "comfortable" && s.StreamWidth != "wide" {
		return fmt.Errorf("unsupported stream width %q", s.StreamWidth)
	}
	if s.TimelineBatchGapPX < 16 || s.TimelineBatchGapPX > 80 {
		return errors.New("timelineBatchGapPx must be between 16 and 80")
	}
	if s.TimelineBoundaryCueMode != "follow" && s.TimelineBoundaryCueMode != "static" {
		return fmt.Errorf("unsupported timeline boundary cue mode %q", s.TimelineBoundaryCueMode)
	}
	if s.TimelineBoundaryReturnMS < 100 || s.TimelineBoundaryReturnMS > 1000 {
		return errors.New("timelineBoundaryReturnMs must be between 100 and 1000")
	}
	if s.SemanticEventMode != "collapse" && s.SemanticEventMode != "show_all" && s.SemanticEventMode != "hide" {
		return fmt.Errorf("unsupported semantic event mode %q", s.SemanticEventMode)
	}
	if s.SemanticEventShortlist != 5 && s.SemanticEventShortlist != 10 && s.SemanticEventShortlist != 15 {
		return errors.New("semanticEventShortlist must be 5, 10, or 15")
	}
	if s.SemanticEventMergeThreshold < MinSemanticMergeThreshold || s.SemanticEventMergeThreshold > MaxSemanticMergeThreshold {
		return fmt.Errorf("semanticEventMergeThreshold must be between %.2f and %.2f", MinSemanticMergeThreshold, MaxSemanticMergeThreshold)
	}
	if scaled := s.SemanticEventMergeThreshold * 100; math.Abs(scaled-math.Round(scaled)) > 1e-9 {
		return errors.New("semanticEventMergeThreshold must use increments of 0.01")
	}
	if s.KnowledgeRetentionDays != 30 && s.KnowledgeRetentionDays != 60 && s.KnowledgeRetentionDays != 90 {
		return errors.New("knowledgeRetentionDays must be 30, 60, or 90")
	}
	if s.KnowledgeStorageLimitMB != 100 && s.KnowledgeStorageLimitMB != 200 && s.KnowledgeStorageLimitMB != 300 && s.KnowledgeStorageLimitMB != 400 && s.KnowledgeStorageLimitMB != 500 && s.KnowledgeStorageLimitMB != 1024 {
		return errors.New("knowledgeStorageLimitMb must be 100, 200, 300, 400, 500, or 1024")
	}
	if s.AIDetectionPresentation != "inline" && s.AIDetectionPresentation != "drawer" && s.AIDetectionPresentation != "hide" {
		return fmt.Errorf("unsupported AI detection presentation %q", s.AIDetectionPresentation)
	}
	if s.ResurfaceMode != "smart" && s.ResurfaceMode != "evaluate_all" {
		return fmt.Errorf("unsupported resurface mode %q", s.ResurfaceMode)
	}
	if s.ResurfaceCooldownDays != 1 && s.ResurfaceCooldownDays != 2 && s.ResurfaceCooldownDays != 7 && s.ResurfaceCooldownDays != 14 && s.ResurfaceCooldownDays != 30 {
		return errors.New("resurfaceCooldownDays must be 1, 2, 7, 14, or 30")
	}
	if len(s.ReasoningExecutablePath) > 4096 || strings.ContainsRune(s.ReasoningExecutablePath, '\x00') {
		return errors.New("reasoningExecutablePath is invalid")
	}
	for name, profile := range map[string]string{
		"reasoningAcquisitionProfile": s.ReasoningAcquisitionProfile,
		"reasoningEvaluationProfile":  s.ReasoningEvaluationProfile,
		"reasoningSemanticProfile":    s.ReasoningSemanticProfile,
		"reasoningAiDeepProfile":      s.ReasoningAIDeepProfile,
	} {
		if !validReasoningProfileID(profile) {
			return fmt.Errorf("%s contains an invalid profile id", name)
		}
	}
	if s.MaxScrolls < 0 || s.MaxScrolls > 6 {
		return errors.New("maxScrolls must be between 0 and 6")
	}
	if s.MaxItemsPerSource < 1 || s.MaxItemsPerSource > 15 {
		return errors.New("maxItemsPerSource must be between 1 and 15")
	}
	if s.MaxItemsTotal < 1 || s.MaxItemsTotal > 30 {
		return errors.New("maxItemsTotal must be between 1 and 30")
	}
	if s.TimelineCapacity < 1 || s.TimelineCapacity > 50 {
		return errors.New("timelineCapacity must be between 1 and 50")
	}
	if s.QualityRetrySettleMS < 0 || s.QualityRetrySettleMS > 5000 {
		return errors.New("qualityRetrySettleMs must be between 0 and 5000")
	}
	if s.CalibrationBatchSize < 2 || s.CalibrationBatchSize > 10 {
		return errors.New("calibrationBatchSize must be between 2 and 10")
	}
	if len(s.ActiveSources) == 0 || len(s.ActiveSources) > len(Sources()) {
		return fmt.Errorf("activeSources must contain between one and %d sources", len(Sources()))
	}
	seen := map[Source]bool{}
	for _, source := range s.ActiveSources {
		if !source.Valid() || seen[source] {
			return fmt.Errorf("invalid active source %q", source)
		}
		seen[source] = true
	}
	if len(s.SourceHydrationTimeoutMS) != len(Sources()) {
		return fmt.Errorf("sourceHydrationTimeoutMs must contain exactly %d registered sources", len(Sources()))
	}
	for source, timeoutMS := range s.SourceHydrationTimeoutMS {
		descriptor, ok := SourceByID(source)
		if !ok {
			return fmt.Errorf("sourceHydrationTimeoutMs contains unknown source %q", source)
		}
		if timeoutMS < descriptor.HydrationTimeoutMinMS || timeoutMS > descriptor.HydrationTimeoutMaxMS {
			return fmt.Errorf("sourceHydrationTimeoutMs.%s must be between %d and %d", source, descriptor.HydrationTimeoutMinMS, descriptor.HydrationTimeoutMaxMS)
		}
		if timeoutMS%1000 != 0 {
			return fmt.Errorf("sourceHydrationTimeoutMs.%s must use 1000 ms increments", source)
		}
	}
	return nil
}

func (s Settings) SourceHydrationTimeout(source Source) int {
	if value := s.SourceHydrationTimeoutMS[source]; value > 0 {
		return value
	}
	if descriptor, ok := SourceByID(source); ok {
		return descriptor.HydrationTimeoutDefaultMS
	}
	return 12000
}

func validReasoningProfileID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

type OnboardingProfile struct {
	Version       int      `json:"version"`
	Status        string   `json:"status"`
	Origin        string   `json:"origin"`
	ActiveSources []Source `json:"activeSources"`
	CompletedAt   string   `json:"completedAt"`
}

type OnboardingState struct {
	Status  string             `json:"status"`
	Profile *OnboardingProfile `json:"profile"`
}

type Session struct {
	ID                string         `json:"id"`
	Intent            string         `json:"intent"`
	Status            string         `json:"status"`
	ActiveSource      *Source        `json:"activeSource"`
	MaxItemsPerSource int            `json:"maxItemsPerSource"`
	MaxItemsTotal     int            `json:"maxItemsTotal"`
	CreatedAt         string         `json:"createdAt"`
	StartedAt         *string        `json:"startedAt"`
	CompletedAt       *string        `json:"completedAt"`
	Runs              []Run          `json:"runs"`
	Items             []TimelineItem `json:"items"`
	Coverage          map[string]any `json:"coverage"`
	Error             *Failure       `json:"error"`
}

type Run struct {
	ID                  string         `json:"id"`
	SessionID           string         `json:"sessionId"`
	Source              Source         `json:"source"`
	Ordinal             int            `json:"ordinal"`
	Status              string         `json:"status"`
	Stage               string         `json:"stage"`
	BridgeCommandStatus string         `json:"bridgeCommandStatus,omitempty"`
	CreatedAt           string         `json:"createdAt"`
	StartedAt           *string        `json:"startedAt"`
	CompletedAt         *string        `json:"completedAt"`
	Summary             string         `json:"summary"`
	Coverage            map[string]any `json:"coverage"`
	Error               *Failure       `json:"error"`
}

type InboxSession struct {
	ID                  string                    `json:"id"`
	Intent              string                    `json:"intent"`
	Status              string                    `json:"status"`
	CreatedAt           string                    `json:"createdAt"`
	StartedAt           *string                   `json:"startedAt"`
	CompletedAt         *string                   `json:"completedAt"`
	CapturedCandidates  int                       `json:"capturedCandidates"`
	EvaluatedCandidates int                       `json:"evaluatedCandidates"`
	SelectedCandidates  int                       `json:"selectedCandidates"`
	AddedItems          int                       `json:"addedItems"`
	DuplicateReports    int                       `json:"duplicateReports"`
	EventResolution     *EventResolutionSummary   `json:"eventResolution,omitempty"`
	AIDetection         *AIDetectionJob           `json:"aiDetection,omitempty"`
	PreferenceDecisions []InboxPreferenceDecision `json:"preferenceDecisions"`
	Runs                []InboxRun                `json:"runs"`
	Error               *Failure                  `json:"error"`
}

type InboxPreferenceDecision struct {
	TimelineID  string `json:"timelineId"`
	EvidenceKey string `json:"evidenceKey"`
	Source      Source `json:"source"`
	Author      string `json:"author"`
	Summary     string `json:"summary"`
	SourceURL   string `json:"sourceUrl"`
	Direction   string `json:"direction"`
	Origin      string `json:"origin"`
	UpdatedAt   string `json:"updatedAt"`
}

type TimelineCheckSummary struct {
	SessionID        string `json:"sessionId"`
	Status           string `json:"status"`
	CompletedAt      string `json:"completedAt"`
	AddedItems       int    `json:"addedItems"`
	DuplicateReports int    `json:"duplicateReports"`
}

type InboxRun struct {
	ID                  string           `json:"id"`
	Source              Source           `json:"source"`
	Status              string           `json:"status"`
	Stage               string           `json:"stage"`
	StartedAt           *string          `json:"startedAt"`
	CompletedAt         *string          `json:"completedAt"`
	Summary             string           `json:"summary"`
	CapturedCandidates  int              `json:"capturedCandidates"`
	EvaluatedCandidates int              `json:"evaluatedCandidates"`
	SelectedCandidates  int              `json:"selectedCandidates"`
	AddedItems          int              `json:"addedItems"`
	AcquisitionRounds   int              `json:"acquisitionRounds"`
	SnapshotCount       int              `json:"snapshotCount"`
	PerformedScrolls    int              `json:"performedScrolls"`
	ReasoningDurationMS int64            `json:"reasoningDurationMs"`
	TotalDurationMS     int64            `json:"totalDurationMs"`
	StageDurationsMS    map[string]int64 `json:"stageDurationsMs"`
	ResurfacedItems     int              `json:"resurfacedItems"`
	SkippedResurfaces   int              `json:"skippedResurfaces"`
	Error               *Failure         `json:"error"`
	FollowUpFallback    *Failure         `json:"followUpFallback,omitempty"`
}

type InboxFlowCounts struct {
	Captured  int `json:"captured"`
	Evaluated int `json:"evaluated"`
	Selected  int `json:"selected"`
	Added     int `json:"added"`
}

type InboxFlowTrace struct {
	RunID  string          `json:"runId"`
	Source Source          `json:"source"`
	Stage  string          `json:"stage"`
	Counts InboxFlowCounts `json:"counts"`
	Items  []InboxFlowItem `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

type InboxFlowItem struct {
	EvidenceKey      string               `json:"-"`
	CandidateRef     string               `json:"candidateRef,omitempty"`
	Author           string               `json:"author,omitempty"`
	Excerpt          string               `json:"excerpt"`
	SourceURL        string               `json:"sourceUrl,omitempty"`
	Outcome          string               `json:"outcome"`
	ContinuityStatus string               `json:"continuityStatus,omitempty"`
	ContinuityDetail string               `json:"continuityDetail,omitempty"`
	Reason           string               `json:"reason,omitempty"`
	Captured         bool                 `json:"captured"`
	Evaluated        bool                 `json:"evaluated"`
	Selected         bool                 `json:"selected"`
	Added            bool                 `json:"added"`
	Correction       *SelectionCorrection `json:"correction,omitempty"`
}

type SelectionCorrection struct {
	ID          string  `json:"id"`
	SessionID   string  `json:"sessionId"`
	RunID       string  `json:"runId"`
	EvidenceKey string  `json:"-"`
	TimelineID  string  `json:"timelineId,omitempty"`
	Action      string  `json:"action"`
	CreatedAt   string  `json:"createdAt"`
	UndoneAt    *string `json:"undoneAt,omitempty"`
}

type Failure struct {
	Code      string         `json:"code"`
	Stage     string         `json:"stage"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

type BridgeCommand struct {
	ID        string         `json:"id"`
	RunID     string         `json:"runId"`
	Type      string         `json:"type"`
	Status    string         `json:"status"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"createdAt"`
	ClaimedAt *string        `json:"claimedAt"`
}

type MediaRecaptureMode string

const (
	MediaRecaptureBackground MediaRecaptureMode = "background"
	MediaRecaptureForeground MediaRecaptureMode = "foreground"
)

type MediaRecapture struct {
	ID          string         `json:"id"`
	TimelineID  string         `json:"timelineId"`
	Source      Source         `json:"source"`
	TargetURL   string         `json:"targetUrl"`
	EvidenceKey string         `json:"evidenceKey"`
	Status      string         `json:"status"`
	Outcome     string         `json:"outcome,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	CreatedAt   string         `json:"createdAt"`
	ClaimedAt   *string        `json:"claimedAt,omitempty"`
	CompletedAt *string        `json:"completedAt,omitempty"`
	Error       *Failure       `json:"error,omitempty"`
}

// PassiveXMediaEvidence is the bounded, sanitized media-only contract accepted
// from AkuBridge's X evidence cache. It deliberately carries no post text or
// source response payload.
type PassiveXMediaEvidence struct {
	CandidateID string                   `json:"candidateId"`
	Media       []PassiveXMediaCandidate `json:"media"`
	Provenance  string                   `json:"provenance"`
}

type PassiveXMediaCandidate struct {
	Kind         string `json:"kind"`
	URL          string `json:"url"`
	PosterURL    string `json:"posterUrl,omitempty"`
	PlaybackURL  string `json:"playbackUrl,omitempty"`
	PlaybackMode string `json:"playbackMode,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	Provenance   string `json:"provenance,omitempty"`
	ObservedAtMS int64  `json:"observedAtMs,omitempty"`
}

type BridgeHeartbeat struct {
	BridgeID                     string              `json:"bridgeId"`
	ExtensionVersion             string              `json:"extensionVersion"`
	RuntimeRevision              string              `json:"runtimeRevision"`
	BuildID                      string              `json:"buildId"`
	AdapterVersions              map[string]string   `json:"adapterVersions"`
	MediaEvidenceAdapterVersions map[string]string   `json:"mediaEvidenceAdapterVersions"`
	ContractVersion              string              `json:"contractVersion"`
	ManifestVersion              int                 `json:"manifestVersion"`
	Sources                      []string            `json:"sources"`
	Actions                      []string            `json:"actions"`
	Authority                    string              `json:"authority"`
	CaptureLimits                BridgeCaptureLimits `json:"captureLimits"`
	ReceivedAt                   string              `json:"receivedAt,omitempty"`
}

type BridgeCaptureLimits struct {
	MaxScrolls           int `json:"maxScrolls"`
	MaxSnapshots         int `json:"maxSnapshots"`
	MaxBlocksPerSnapshot int `json:"maxBlocksPerSnapshot"`
}

type Observation struct {
	Source     Source         `json:"source"`
	PageURL    string         `json:"pageUrl"`
	PageTitle  string         `json:"pageTitle"`
	CapturedAt string         `json:"capturedAt"`
	Snapshots  []Snapshot     `json:"snapshots"`
	Coverage   map[string]any `json:"coverage"`
}

type Snapshot struct {
	Index                  int              `json:"index"`
	AdapterVersion         string           `json:"adapterVersion"`
	SelectorStrategy       string           `json:"selectorStrategy"`
	SelectorCounts         map[string]int   `json:"selectorCounts"`
	SelectorCandidateCount int              `json:"selectorCandidateCount"`
	VisibleContainerCount  int              `json:"visibleContainerCount"`
	CapturedAt             string           `json:"capturedAt"`
	ScrollY                int64            `json:"scrollY"`
	ViewportHeight         int64            `json:"viewportHeight"`
	NewCandidateCount      int              `json:"newCandidateCount"`
	Blocks                 []Block          `json:"blocks"`
	QualityReports         []map[string]any `json:"qualityReports"`
}

type Attachment struct {
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle,omitempty"`
	Detail      string `json:"detail,omitempty"`
	ActionLabel string `json:"actionLabel,omitempty"`
	Footnote    string `json:"footnote,omitempty"`
	URL         string `json:"url"`
	Domain      string `json:"domain,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	Verified    bool   `json:"verified,omitempty"`
}

func (a Attachment) Validate() error {
	if a.Kind != "job" && a.Kind != "link_preview" && a.Kind != "document" {
		return fmt.Errorf("unsupported attachment kind %q", a.Kind)
	}
	if strings.TrimSpace(a.Title) == "" || len([]rune(a.Title)) > 300 {
		return errors.New("attachment title must contain 1-300 characters")
	}
	for label, value := range map[string]string{
		"subtitle": a.Subtitle,
		"detail":   a.Detail,
		"footnote": a.Footnote,
		"domain":   a.Domain,
	} {
		if len([]rune(value)) > 300 {
			return fmt.Errorf("attachment %s cannot exceed 300 characters", label)
		}
	}
	if len([]rune(a.ActionLabel)) > 80 {
		return errors.New("attachment action label cannot exceed 80 characters")
	}
	target, err := url.Parse(a.URL)
	if err != nil || target.Scheme != "https" || target.Host == "" {
		return errors.New("attachment URL must use HTTPS")
	}
	if a.ImageURL != "" {
		image, err := url.Parse(a.ImageURL)
		if err != nil || image.Scheme != "https" || image.Host == "" {
			return errors.New("attachment image URL must use HTTPS")
		}
	}
	return nil
}

type Block struct {
	EvidenceKey      string           `json:"evidenceKey,omitempty"`
	Author           string           `json:"author"`
	AvatarURL        string           `json:"avatarUrl"`
	Text             string           `json:"text"`
	Permalink        string           `json:"permalink"`
	PublishedAt      *string          `json:"publishedAt"`
	PlatformID       string           `json:"platformId"`
	ContentKind      string           `json:"contentKind"`
	RelationshipType string           `json:"relationshipType"`
	ParentPermalink  string           `json:"parentPermalink"`
	QuotedPost       map[string]any   `json:"quotedPost"`
	Engagement       map[string]any   `json:"engagement"`
	Presentation     map[string]any   `json:"presentation"`
	Attachments      []Attachment     `json:"attachments"`
	Media            []map[string]any `json:"media"`
	Links            []map[string]any `json:"links"`
	MediaRecovery    map[string]any   `json:"mediaRecovery"`
	CaptureQuality   map[string]any   `json:"captureQuality"`
	FeedPosition     int              `json:"feedPosition"`
}

type CandidateAssessment struct {
	EvidenceKey       string   `json:"evidenceKey"`
	TopicTags         []string `json:"topicTags"`
	TopicFacets       []string `json:"topicFacets"`
	ContentType       string   `json:"contentType"`
	Novelty           float64  `json:"novelty"`
	Urgency           float64  `json:"urgency"`
	Actionability     float64  `json:"actionability"`
	Materiality       float64  `json:"materiality"`
	EvidenceStrength  float64  `json:"evidenceStrength"`
	KnowledgeRelation string   `json:"knowledgeRelation"`
	Rationale         string   `json:"rationale"`
}

type ContentContinuityDecision struct {
	EvidenceKey    string `json:"evidenceKey"`
	Status         string `json:"status"`
	Action         string `json:"action"`
	PreviousSeenAt string `json:"previousSeenAt,omitempty"`
	ObservedAt     string `json:"observedAt"`
	Reason         string `json:"reason"`
}

type ReasonedItem struct {
	ID             string  `json:"id"`
	WhatChanged    string  `json:"whatChanged"`
	WhyItMatters   string  `json:"whyItMatters"`
	Source         Source  `json:"source"`
	SourceURL      string  `json:"sourceUrl,omitempty"`
	SourceURLKind  string  `json:"sourceUrlKind,omitempty"`
	EvidenceKey    string  `json:"evidenceKey"`
	EventKey       string  `json:"eventKey"`
	KnowledgeDelta string  `json:"knowledgeDelta"`
	Author         string  `json:"author"`
	PublishedAt    *string `json:"publishedAt"`
	Confidence     float64 `json:"confidence"`
	EvidenceState  string  `json:"evidenceState"`
}

type ReasoningResult struct {
	Summary                 string                `json:"summary"`
	Items                   []ReasonedItem        `json:"items"`
	CandidateAssessments    []CandidateAssessment `json:"candidateAssessments"`
	RepeatedClaimsCollapsed int                   `json:"repeatedClaimsCollapsed"`
	DeferredByBudget        int                   `json:"deferredByBudget"`
	Limitations             []string              `json:"limitations"`
}

type ReasoningTelemetry struct {
	ID                    string `json:"id"`
	RunID                 string `json:"runId"`
	Phase                 string `json:"phase"`
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	Effort                string `json:"effort"`
	DurationMS            int64  `json:"durationMs"`
	Status                string `json:"status"`
	InputTokens           *int64 `json:"inputTokens"`
	CachedInputTokens     *int64 `json:"cachedInputTokens"`
	OutputTokens          *int64 `json:"outputTokens"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens"`
	CreatedAt             string `json:"createdAt"`
}

type TimelineItem struct {
	ID            string                 `json:"id"`
	SessionID     string                 `json:"sessionId"`
	RunID         string                 `json:"runId"`
	Source        Source                 `json:"source"`
	EvidenceKey   string                 `json:"evidenceKey"`
	Rank          int                    `json:"rank"`
	Item          ReasonedItem           `json:"item"`
	Assessment    CandidateAssessment    `json:"assessment"`
	Evidence      *Block                 `json:"evidence,omitempty"`
	SemanticEvent *TimelineSemanticEvent `json:"semanticEvent,omitempty"`
	AIDetection   *TimelineAIDetection   `json:"aiDetection,omitempty"`
	Feedback      *Feedback              `json:"feedback,omitempty"`
	Coverage      map[string]any         `json:"coverage"`
	CreatedAt     string                 `json:"createdAt"`
}

type AIAssessment struct {
	ID                 string   `json:"id"`
	TimelineID         string   `json:"timelineId"`
	SessionID          string   `json:"sessionId"`
	Stage              string   `json:"stage"`
	Status             string   `json:"status"`
	ConfidenceBand     string   `json:"confidenceBand"`
	EvidenceCodes      []string `json:"evidenceCodes"`
	AssessedObject     string   `json:"assessedObject"`
	SignalScope        string   `json:"signalScope"`
	Provider           string   `json:"provider"`
	DetectorVersion    string   `json:"detectorVersion"`
	ContentFingerprint string   `json:"contentFingerprint"`
	Rationale          string   `json:"rationale"`
	SupersedesID       string   `json:"supersedesId,omitempty"`
	CreatedAt          string   `json:"createdAt"`
	UndoneAt           *string  `json:"undoneAt,omitempty"`
}

func (a AIAssessment) Validate() error {
	if a.TimelineID == "" || a.SessionID == "" {
		return errors.New("AI assessment requires timelineId and sessionId")
	}
	if a.Stage != "fast" && a.Stage != "deep" && a.Stage != "user" {
		return fmt.Errorf("unsupported AI assessment stage %q", a.Stage)
	}
	validStatus := map[string]bool{
		"strong_signals":        true,
		"insufficient_evidence": true,
		"no_signal_detected":    true,
		"conflicting_evidence":  true,
		"user_marked_ai":        true,
		"user_marked_not_ai":    true,
	}
	if !validStatus[a.Status] {
		return fmt.Errorf("unsupported AI assessment status %q", a.Status)
	}
	userStatus := a.Status == "user_marked_ai" || a.Status == "user_marked_not_ai"
	if (a.Stage == "user") != userStatus {
		return errors.New("AI assessment stage and status authority do not match")
	}
	if a.ConfidenceBand != "low" && a.ConfidenceBand != "medium" && a.ConfidenceBand != "high" {
		return fmt.Errorf("unsupported AI confidence band %q", a.ConfidenceBand)
	}
	if len(a.EvidenceCodes) > 3 {
		return errors.New("AI assessment evidenceCodes cannot exceed three entries")
	}
	if a.AssessedObject != "social_post" {
		return fmt.Errorf("unsupported AI assessed object %q", a.AssessedObject)
	}
	validScope := map[string]bool{
		"social_post": true, "quoted_post": true, "external_artifact": true,
		"attached_media": true, "none": true, "mixed": true,
	}
	if !validScope[a.SignalScope] {
		return fmt.Errorf("unsupported AI signal scope %q", a.SignalScope)
	}
	if (a.Status == "strong_signals" || a.Status == "user_marked_ai") && a.SignalScope != "social_post" {
		return errors.New("a strong social-post assessment requires social_post signal scope")
	}
	return nil
}

type TimelineAIDetection struct {
	AssessmentID     string   `json:"assessmentId"`
	Stage            string   `json:"stage"`
	Status           string   `json:"status"`
	ConfidenceBand   string   `json:"confidenceBand"`
	EvidenceCodes    []string `json:"evidenceCodes"`
	AssessedObject   string   `json:"assessedObject,omitempty"`
	SignalScope      string   `json:"signalScope,omitempty"`
	BadgeLabel       string   `json:"badgeLabel,omitempty"`
	Detail           string   `json:"detail,omitempty"`
	RouteToSignals   bool     `json:"routeToSignals"`
	HideEligible     bool     `json:"hideEligible"`
	PendingDeep      bool     `json:"pendingDeep"`
	DeepStatus       string   `json:"deepStatus,omitempty"`
	HistoryCount     int      `json:"historyCount"`
	Corrected        bool     `json:"corrected"`
	UserOverride     bool     `json:"userOverride"`
	CorrectionID     string   `json:"correctionId,omitempty"`
	DetectorVersion  string   `json:"detectorVersion,omitempty"`
	LatestAssessedAt string   `json:"latestAssessedAt,omitempty"`
}

type AIDetectionJob struct {
	ID                    string `json:"id"`
	SessionID             string `json:"sessionId"`
	Status                string `json:"status"`
	Provider              string `json:"provider"`
	Model                 string `json:"model"`
	Effort                string `json:"effort"`
	CandidateCount        int    `json:"candidateCount"`
	DurationMS            int64  `json:"durationMs"`
	InputTokens           *int64 `json:"inputTokens,omitempty"`
	CachedInputTokens     *int64 `json:"cachedInputTokens,omitempty"`
	OutputTokens          *int64 `json:"outputTokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoningOutputTokens,omitempty"`
	Error                 string `json:"error,omitempty"`
	CreatedAt             string `json:"createdAt"`
	StartedAt             string `json:"startedAt,omitempty"`
	CompletedAt           string `json:"completedAt,omitempty"`
}

type DeepAIAssessment struct {
	Status         string   `json:"status"`
	ConfidenceBand string   `json:"confidenceBand"`
	EvidenceCodes  []string `json:"evidenceCodes"`
	AssessedObject string   `json:"assessedObject"`
	SignalScope    string   `json:"signalScope"`
	Rationale      string   `json:"rationale"`
}

type DeepAIResult struct {
	Assessments []DeepAIAssessment `json:"assessments"`
}

type SemanticEvent struct {
	ID             string   `json:"id"`
	CanonicalClaim string   `json:"canonicalClaim"`
	Actor          string   `json:"actor"`
	Action         string   `json:"action"`
	Object         string   `json:"object"`
	EventKind      string   `json:"eventKind"`
	EventStart     *string  `json:"eventStart"`
	EventEnd       *string  `json:"eventEnd"`
	Aliases        []string `json:"aliases"`
	ReportCount    int      `json:"reportCount"`
	FirstSeenAt    string   `json:"firstSeenAt"`
	LastSeenAt     string   `json:"lastSeenAt"`
}

type SemanticCandidate struct {
	Alias       string       `json:"alias"`
	TimelineID  string       `json:"timelineId"`
	SessionID   string       `json:"sessionId"`
	RunID       string       `json:"runId"`
	EvidenceKey string       `json:"evidenceKey"`
	Source      Source       `json:"source"`
	Author      string       `json:"author"`
	PublishedAt *string      `json:"publishedAt"`
	Text        string       `json:"text"`
	WhatChanged string       `json:"whatChanged"`
	EventKey    string       `json:"eventKey"`
	TopicTags   []string     `json:"topicTags"`
	Item        ReasonedItem `json:"-"`
}

type SemanticDecision struct {
	CandidateAlias string        `json:"candidateAlias"`
	Relation       string        `json:"relation"`
	TargetAlias    *string       `json:"targetAlias"`
	Confidence     float64       `json:"confidence"`
	Reason         string        `json:"reason"`
	Event          SemanticEvent `json:"event"`
}

type SemanticResolution struct {
	Decisions []SemanticDecision `json:"decisions"`
}

type ResolvedSemanticReport struct {
	Candidate  SemanticCandidate `json:"candidate"`
	Event      SemanticEvent     `json:"event"`
	Relation   string            `json:"relation"`
	Confidence float64           `json:"confidence"`
	Reason     string            `json:"reason"`
	Corrected  bool              `json:"corrected"`
}

type TimelineSemanticEvent struct {
	EventID        string  `json:"eventId"`
	CanonicalClaim string  `json:"canonicalClaim"`
	Relation       string  `json:"relation"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason"`
	ReportCount    int     `json:"reportCount"`
	Corrected      bool    `json:"corrected"`
	CorrectionID   string  `json:"correctionId,omitempty"`
}

type ModelUsage struct {
	Input           *int64 `json:"inputTokens"`
	CachedInput     *int64 `json:"cachedInputTokens"`
	Output          *int64 `json:"outputTokens"`
	ReasoningOutput *int64 `json:"reasoningOutputTokens"`
}

type EventResolutionSummary struct {
	SessionID            string     `json:"sessionId"`
	Status               string     `json:"status"`
	Provider             string     `json:"provider"`
	Model                string     `json:"model"`
	Effort               string     `json:"effort"`
	CandidateCount       int        `json:"candidateCount"`
	HistoricalEventCount int        `json:"historicalEventCount"`
	ShortlistCount       int        `json:"shortlistCount"`
	UniqueItems          int        `json:"uniqueItems"`
	DuplicateReports     int        `json:"duplicateReports"`
	UserSplitCorrections int        `json:"userSplitCorrections"`
	UserMergeCorrections int        `json:"userMergeCorrections"`
	ResolverInvoked      bool       `json:"resolverInvoked"`
	TriggerReason        string     `json:"triggerReason"`
	StrongestOverlap     int        `json:"strongestOverlap"`
	TriggerTokens        []string   `json:"triggerTokens"`
	DurationMS           int64      `json:"durationMs"`
	Usage                ModelUsage `json:"usage"`
	Error                *Failure   `json:"error,omitempty"`
	CreatedAt            string     `json:"createdAt"`
}

type EventSuggestion struct {
	EventID        string `json:"eventId"`
	CanonicalClaim string `json:"canonicalClaim"`
	Actor          string `json:"actor"`
	Object         string `json:"object"`
	ReportCount    int    `json:"reportCount"`
	LastSeenAt     string `json:"lastSeenAt"`
}

type EventCorrection struct {
	ID          string  `json:"id"`
	TimelineID  string  `json:"timelineId"`
	Action      string  `json:"action"`
	FromEventID string  `json:"fromEventId"`
	ToEventID   string  `json:"toEventId"`
	CreatedAt   string  `json:"createdAt"`
	UndoneAt    *string `json:"undoneAt,omitempty"`
}

type RetentionResult struct {
	RemovedSessions int   `json:"removedSessions"`
	RemovedEvents   int   `json:"removedEvents"`
	DatabaseBytes   int64 `json:"databaseBytes"`
	LimitBytes      int64 `json:"limitBytes"`
}

type Feedback struct {
	ID          string  `json:"id"`
	TimelineID  string  `json:"timelineId"`
	SessionID   string  `json:"sessionId"`
	RunID       string  `json:"runId"`
	EvidenceKey string  `json:"evidenceKey"`
	Direction   string  `json:"direction"`
	Reason      *string `json:"reason"`
	Origin      string  `json:"origin,omitempty"`
	CreatedAt   string  `json:"createdAt"`
}

type CalibrationCandidate struct {
	RunID        string              `json:"runId"`
	EvidenceKey  string              `json:"evidenceKey"`
	Source       Source              `json:"source"`
	FeedPosition int                 `json:"feedPosition"`
	Author       string              `json:"author"`
	AvatarURL    string              `json:"avatarUrl"`
	Text         string              `json:"text"`
	SourceURL    string              `json:"sourceUrl"`
	PublishedAt  *string             `json:"publishedAt"`
	ContentKind  string              `json:"contentKind"`
	QuotedPost   map[string]any      `json:"quotedPost"`
	Engagement   map[string]any      `json:"engagement"`
	Presentation map[string]any      `json:"presentation"`
	Attachments  []Attachment        `json:"attachments"`
	Media        []map[string]any    `json:"media"`
	Links        []map[string]any    `json:"links"`
	Assessment   CandidateAssessment `json:"assessment"`
}

type CalibrationSample struct {
	Ordinal     int                  `json:"ordinal"`
	RunID       string               `json:"runId"`
	EvidenceKey string               `json:"evidenceKey"`
	Source      Source               `json:"source"`
	Candidate   CalibrationCandidate `json:"candidate"`
	Label       *string              `json:"label"`
	IssueCode   *string              `json:"issueCode"`
	LabeledAt   *string              `json:"labeledAt"`
}

type CalibrationSnapshot struct {
	Version              int            `json:"version"`
	Origin               string         `json:"origin"`
	CalibrationSessionID string         `json:"calibrationSessionId"`
	CreatedAt            string         `json:"createdAt"`
	Labels               map[string]int `json:"labels"`
	Sources              []Source       `json:"sources"`
	LiveInfluence        bool           `json:"liveInfluence"`
	ActivationState      string         `json:"activationState"`
}

type CalibrationSession struct {
	ID               string               `json:"id"`
	UnifiedSessionID string               `json:"unifiedSessionId"`
	TriggerKind      string               `json:"triggerKind"`
	Status           string               `json:"status"`
	MaxItems         int                  `json:"maxItems"`
	SampleCount      int                  `json:"sampleCount"`
	ResolvedCount    int                  `json:"resolvedCount"`
	CurrentOrdinal   *int                 `json:"currentOrdinal"`
	CreatedAt        string               `json:"createdAt"`
	UpdatedAt        string               `json:"updatedAt"`
	CompletedAt      *string              `json:"completedAt"`
	Samples          []CalibrationSample  `json:"samples"`
	Snapshot         *CalibrationSnapshot `json:"snapshot"`
	LiveInfluence    bool                 `json:"liveInfluence"`
}

type CalibrationDecision struct {
	Label     *string `json:"label"`
	IssueCode *string `json:"issueCode"`
}

func (d CalibrationDecision) Validate() error {
	labels := map[string]bool{"more_like_this": true, "neutral": true, "less_like_this": true}
	issues := map[string]bool{"capture_incomplete": true, "wrong_source": true, "duplicate": true, "formatting": true}
	if d.Label != nil && d.IssueCode == nil && labels[*d.Label] {
		return nil
	}
	if d.IssueCode != nil && d.Label == nil && issues[*d.IssueCode] {
		return nil
	}
	return errors.New("calibration decision requires More, Neutral, Less, or a supported capture issue")
}

type CalibrationOverview struct {
	FirstRunStatus string              `json:"firstRunStatus"`
	Active         *CalibrationSession `json:"active"`
	Enabled        bool                `json:"enabled"`
	TriggerPolicy  string              `json:"triggerPolicy"`
	BatchSize      int                 `json:"batchSize"`
	LiveInfluence  bool                `json:"liveInfluence"`
}

func (f Feedback) Validate() error {
	if f.Direction != "more" && f.Direction != "less" {
		return errors.New("direction must be more or less")
	}
	if f.Direction == "more" && f.Reason == nil {
		return nil
	}
	if f.Direction == "more" {
		return errors.New("More like this does not accept a reason")
	}
	if f.Reason == nil || *f.Reason != "not_interested" {
		return errors.New("Less like this requires the not_interested reason")
	}
	return nil
}

func NewID(prefix string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(fmt.Sprintf("secure random ID: %v", err))
	}
	return prefix + "_" + hex.EncodeToString(value[:])
}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func ValidateIntent(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 1 || len(value) > 500 {
		return errors.New("intent must contain 1 to 500 characters")
	}
	return nil
}
