package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	ApplicationVersion    = "1.0.0-dev.3"
	BridgeContractVersion = "aku-browser.bridge.v2"
)

type Source string

const (
	SourceX        Source = "x"
	SourceLinkedIn Source = "linkedin"
)

func (s Source) Valid() bool { return s == SourceX || s == SourceLinkedIn }

type Settings struct {
	LoadProfile               string   `json:"loadProfile"`
	CaptureVisibility         string   `json:"captureVisibility"`
	OpenMissingSource         bool     `json:"openMissingSource"`
	ActiveSources             []Source `json:"activeSources"`
	TimelineCapacity          int      `json:"timelineCapacity"`
	MaxItemsPerSource         int      `json:"maxItemsPerSource"`
	MaxItemsTotal             int      `json:"maxItemsTotal"`
	MaxScrolls                int      `json:"maxScrolls"`
	QualityRetrySettleMS      int      `json:"qualityRetrySettleMs"`
	PreferenceEligibilityMode string   `json:"preferenceEligibilityMode"`
	CalibrationEnabled        bool     `json:"calibrationEnabled"`
	CalibrationBatchSize      int      `json:"calibrationBatchSize"`
	DefaultPresentation       string   `json:"defaultPresentation"`
	StreamWidth               string   `json:"streamWidth"`
}

func DefaultSettings(profile, visibility, preferenceMode string, openMissing bool) Settings {
	settings := Settings{
		LoadProfile:               profile,
		CaptureVisibility:         visibility,
		OpenMissingSource:         openMissing,
		ActiveSources:             []Source{SourceX, SourceLinkedIn},
		PreferenceEligibilityMode: preferenceMode,
		CalibrationEnabled:        true,
		CalibrationBatchSize:      10,
		DefaultPresentation:       "source",
		StreamWidth:               "social",
	}
	settings.ApplyProfile()
	return settings
}

func (s *Settings) ApplyProfile() {
	switch s.LoadProfile {
	case "standard":
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 2, 5, 10
		s.TimelineCapacity, s.QualityRetrySettleMS = 12, 300
	case "stress":
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 6, 15, 30
		s.TimelineCapacity, s.QualityRetrySettleMS = 36, 1000
	case "custom":
		// Custom keeps its explicit, policy-bounded values.
	default:
		s.LoadProfile = "expanded"
		s.MaxScrolls, s.MaxItemsPerSource, s.MaxItemsTotal = 4, 10, 20
		s.TimelineCapacity, s.QualityRetrySettleMS = 24, 1000
	}
}

func (s *Settings) Normalize() {
	if s.DefaultPresentation == "" {
		s.DefaultPresentation = "source"
	}
	if s.StreamWidth == "" {
		s.StreamWidth = "social"
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
	if s.PreferenceEligibilityMode != "rank_only" && s.PreferenceEligibilityMode != "promote_unused_budget" && s.PreferenceEligibilityMode != "guarded_live" {
		return fmt.Errorf("unsupported preference mode %q", s.PreferenceEligibilityMode)
	}
	if s.DefaultPresentation != "source" && s.DefaultPresentation != "brief" {
		return fmt.Errorf("unsupported default presentation %q", s.DefaultPresentation)
	}
	if s.StreamWidth != "compact" && s.StreamWidth != "social" && s.StreamWidth != "comfortable" && s.StreamWidth != "wide" {
		return fmt.Errorf("unsupported stream width %q", s.StreamWidth)
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
	if len(s.ActiveSources) == 0 || len(s.ActiveSources) > 2 {
		return errors.New("activeSources must contain one or two sources")
	}
	seen := map[Source]bool{}
	for _, source := range s.ActiveSources {
		if !source.Valid() || seen[source] {
			return fmt.Errorf("invalid active source %q", source)
		}
		seen[source] = true
	}
	return nil
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
	ID          string         `json:"id"`
	SessionID   string         `json:"sessionId"`
	Source      Source         `json:"source"`
	Ordinal     int            `json:"ordinal"`
	Status      string         `json:"status"`
	Stage       string         `json:"stage"`
	CreatedAt   string         `json:"createdAt"`
	StartedAt   *string        `json:"startedAt"`
	CompletedAt *string        `json:"completedAt"`
	Summary     string         `json:"summary"`
	Coverage    map[string]any `json:"coverage"`
	Error       *Failure       `json:"error"`
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

type BridgeHeartbeat struct {
	BridgeID         string              `json:"bridgeId"`
	ExtensionVersion string              `json:"extensionVersion"`
	RuntimeRevision  string              `json:"runtimeRevision"`
	BuildID          string              `json:"buildId"`
	AdapterVersions  map[string]string   `json:"adapterVersions"`
	ContractVersion  string              `json:"contractVersion"`
	ManifestVersion  int                 `json:"manifestVersion"`
	Sources          []string            `json:"sources"`
	Actions          []string            `json:"actions"`
	Authority        string              `json:"authority"`
	CaptureLimits    BridgeCaptureLimits `json:"captureLimits"`
	ReceivedAt       string              `json:"receivedAt,omitempty"`
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
	Media            []map[string]any `json:"media"`
	Links            []map[string]any `json:"links"`
	MediaRecovery    map[string]any   `json:"mediaRecovery"`
	CaptureQuality   map[string]any   `json:"captureQuality"`
	FeedPosition     int              `json:"feedPosition"`
}

type CandidateAssessment struct {
	EvidenceKey      string   `json:"evidenceKey"`
	TopicTags        []string `json:"topicTags"`
	TopicFacets      []string `json:"topicFacets"`
	ContentType      string   `json:"contentType"`
	Novelty          float64  `json:"novelty"`
	Urgency          float64  `json:"urgency"`
	Actionability    float64  `json:"actionability"`
	Materiality      float64  `json:"materiality"`
	EvidenceStrength float64  `json:"evidenceStrength"`
	Rationale        string   `json:"rationale"`
}

type ReasonedItem struct {
	ID             string  `json:"id"`
	WhatChanged    string  `json:"whatChanged"`
	WhyItMatters   string  `json:"whyItMatters"`
	Source         Source  `json:"source"`
	SourceURL      string  `json:"sourceUrl"`
	SourceURLKind  string  `json:"sourceUrlKind"`
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
	ID          string              `json:"id"`
	SessionID   string              `json:"sessionId"`
	RunID       string              `json:"runId"`
	Source      Source              `json:"source"`
	EvidenceKey string              `json:"evidenceKey"`
	Rank        int                 `json:"rank"`
	Item        ReasonedItem        `json:"item"`
	Assessment  CandidateAssessment `json:"assessment"`
	Evidence    *Block              `json:"evidence,omitempty"`
	Coverage    map[string]any      `json:"coverage"`
	CreatedAt   string              `json:"createdAt"`
}

type Feedback struct {
	ID          string  `json:"id"`
	TimelineID  string  `json:"timelineId"`
	SessionID   string  `json:"sessionId"`
	RunID       string  `json:"runId"`
	EvidenceKey string  `json:"evidenceKey"`
	Direction   string  `json:"direction"`
	Reason      *string `json:"reason"`
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
	if f.Reason == nil {
		return nil
	}
	allowed := map[string]bool{
		"not_interested": true,
		"already_knew":   true,
		"old_info":       true,
		"duplicate":      true,
	}
	if !allowed[*f.Reason] {
		return fmt.Errorf("unsupported feedback reason %q", *f.Reason)
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
