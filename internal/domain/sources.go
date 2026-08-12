package domain

import (
	"net/url"
	"regexp"
	"strings"
)

// SourceDescriptor is the application-owned source contract. Source-specific
// facts live here; orchestration, selection, preference learning, semantic
// resolution, and presentation consume the descriptor generically.
type SourceDescriptor struct {
	ID                          Source                   `json:"id"`
	DisplayName                 string                   `json:"displayName"`
	ShortLabel                  string                   `json:"shortLabel"`
	IconText                    string                   `json:"iconText"`
	IconBackground              string                   `json:"iconBackground"`
	IconForeground              string                   `json:"iconForeground"`
	OnboardingDescription       string                   `json:"onboardingDescription"`
	PresentationStyle           string                   `json:"presentationStyle"`
	SocialContextPlacement      string                   `json:"socialContextPlacement"`
	DefaultActive               bool                     `json:"defaultActive"`
	AdapterVersion              string                   `json:"adapterVersion"`
	MediaEvidenceAdapterVersion string                   `json:"mediaEvidenceAdapterVersion,omitempty"`
	ContinuationOverlapRequired bool                     `json:"continuationOverlapRequired,omitempty"`
	FollowUpPlanningPolicy      string                   `json:"followUpPlanningPolicy,omitempty"`
	FrontierRequiresComplete    bool                     `json:"frontierRequiresComplete,omitempty"`
	InitialRoundCandidateTarget int                      `json:"initialRoundCandidateTarget,omitempty"`
	NativeHosts                 []string                 `json:"nativeHosts"`
	NativePathTokens            []string                 `json:"nativePathTokens"`
	IdentityFormat              string                   `json:"identityFormat,omitempty"`
	AvatarFallback              string                   `json:"avatarFallback"`
	PassiveMediaCapability      string                   `json:"passiveMediaCapability,omitempty"`
	PlaybackRecoveryCapability  string                   `json:"playbackRecoveryCapability,omitempty"`
	TrustedMediaHostSuffixes    []string                 `json:"trustedMediaHostSuffixes,omitempty"`
	HydrationTimeoutDefaultMS   int                      `json:"hydrationTimeoutDefaultMs"`
	HydrationTimeoutMinMS       int                      `json:"hydrationTimeoutMinMs"`
	HydrationTimeoutMaxMS       int                      `json:"hydrationTimeoutMaxMs"`
	EngagementMetrics           []SourceEngagementMetric `json:"engagementMetrics"`
}

type SourceEngagementMetric struct {
	Key  string `json:"key"`
	Icon string `json:"icon"`
}

var sourceRegistry = []SourceDescriptor{
	{ID: SourceX, DisplayName: "X", ShortLabel: "X", IconText: "X", IconBackground: "#e7e9ea", IconForeground: "#0f1419", OnboardingDescription: "Your home timeline", PresentationStyle: "compact", SocialContextPlacement: "content", DefaultActive: true, AdapterVersion: "x-dom-v22", MediaEvidenceAdapterVersion: "x-response-evidence-v2", NativeHosts: []string{"x.com"}, NativePathTokens: []string{"/status/"}, IdentityFormat: "display_handle", AvatarFallback: "source_icon", PassiveMediaCapability: "x_response", TrustedMediaHostSuffixes: []string{"pbs.twimg.com"}, HydrationTimeoutDefaultMS: 12000, HydrationTimeoutMinMS: 7000, HydrationTimeoutMaxMS: 17000, EngagementMetrics: []SourceEngagementMetric{{Key: "reply", Icon: "\u25cb"}, {Key: "repost", Icon: "\u21bb"}, {Key: "like", Icon: "\u2661"}, {Key: "view", Icon: "\u25a5"}}},
	{ID: SourceLinkedIn, DisplayName: "LinkedIn", ShortLabel: "in", IconText: "in", IconBackground: "#0a66c2", IconForeground: "#ffffff", OnboardingDescription: "Your professional feed", PresentationStyle: "professional", SocialContextPlacement: "above", DefaultActive: true, AdapterVersion: "linkedin-dom-v20", MediaEvidenceAdapterVersion: "linkedin-main-world-video-v1", PlaybackRecoveryCapability: "native_post_recapture", ContinuationOverlapRequired: true, FollowUpPlanningPolicy: "local_frontier", FrontierRequiresComplete: true, InitialRoundCandidateTarget: 1, NativeHosts: []string{"www.linkedin.com"}, NativePathTokens: []string{"/posts/", "/feed/update/"}, AvatarFallback: "initials", TrustedMediaHostSuffixes: []string{"media.licdn.com", "dms.licdn.com"}, HydrationTimeoutDefaultMS: 18000, HydrationTimeoutMinMS: 13000, HydrationTimeoutMaxMS: 23000, EngagementMetrics: []SourceEngagementMetric{{Key: "like", Icon: "\U0001f44d"}, {Key: "comment", Icon: "\U0001f4ac"}, {Key: "repost", Icon: "\u21bb"}}},
	{ID: SourceFacebook, DisplayName: "Facebook", ShortLabel: "f", IconText: "f", IconBackground: "#0866ff", IconForeground: "#ffffff", OnboardingDescription: "Your Home Feed", PresentationStyle: "social", SocialContextPlacement: "above", DefaultActive: true, AdapterVersion: "facebook-dom-v18", MediaEvidenceAdapterVersion: "facebook-structured-video-v1", PlaybackRecoveryCapability: "native_post_recapture", FollowUpPlanningPolicy: "local_frontier", NativeHosts: []string{"facebook.com", "www.facebook.com", "m.facebook.com"}, NativePathTokens: []string{"/posts/", "/permalink/", "/story.php", "/photo", "/watch/", "/video.php", "/videos/", "/reel/"}, AvatarFallback: "initials", TrustedMediaHostSuffixes: []string{"fbcdn.net", "fbsbx.com"}, HydrationTimeoutDefaultMS: 25000, HydrationTimeoutMinMS: 20000, HydrationTimeoutMaxMS: 30000, EngagementMetrics: []SourceEngagementMetric{{Key: "like", Icon: "\U0001f44d"}, {Key: "comment", Icon: "\U0001f4ac"}, {Key: "repost", Icon: "\u21bb"}}},
	{ID: SourceInstagram, DisplayName: "Instagram", ShortLabel: "ig", IconText: "ig", IconBackground: "#e1306c", IconForeground: "#ffffff", OnboardingDescription: "Your Instagram home feed", PresentationStyle: "social", SocialContextPlacement: "above", DefaultActive: true, AdapterVersion: "instagram-dom-v3", MediaEvidenceAdapterVersion: "instagram-structured-video-v1", PlaybackRecoveryCapability: "native_post_recapture", FollowUpPlanningPolicy: "local_frontier", NativeHosts: []string{"instagram.com", "www.instagram.com"}, NativePathTokens: []string{"/p/", "/reel/", "/tv/"}, IdentityFormat: "username", AvatarFallback: "initials", TrustedMediaHostSuffixes: []string{"fbcdn.net", "cdninstagram.com"}, HydrationTimeoutDefaultMS: 15000, HydrationTimeoutMinMS: 10000, HydrationTimeoutMaxMS: 20000, EngagementMetrics: []SourceEngagementMetric{{Key: "like", Icon: "\u2661"}, {Key: "comment", Icon: "\U0001f4ac"}}},
}

func Sources() []SourceDescriptor {
	result := make([]SourceDescriptor, len(sourceRegistry))
	copy(result, sourceRegistry)
	return result
}

func SourceByID(source Source) (SourceDescriptor, bool) {
	for _, descriptor := range sourceRegistry {
		if descriptor.ID == source {
			return descriptor, true
		}
	}
	return SourceDescriptor{}, false
}

func DefaultSources() []Source {
	result := make([]Source, 0, len(sourceRegistry))
	for _, descriptor := range sourceRegistry {
		if descriptor.DefaultActive {
			result = append(result, descriptor.ID)
		}
	}
	return result
}

func DefaultSourceHydrationTimeouts() map[Source]int {
	result := make(map[Source]int, len(sourceRegistry))
	for _, descriptor := range sourceRegistry {
		result[descriptor.ID] = descriptor.HydrationTimeoutDefaultMS
	}
	return result
}

func SourceIDs() []string {
	result := make([]string, 0, len(sourceRegistry))
	for _, descriptor := range sourceRegistry {
		result = append(result, string(descriptor.ID))
	}
	return result
}

func ExpectedAdapterVersions() map[string]string {
	result := make(map[string]string, len(sourceRegistry))
	for _, descriptor := range sourceRegistry {
		result[string(descriptor.ID)] = descriptor.AdapterVersion
	}
	return result
}

func ExpectedMediaEvidenceAdapterVersions() map[string]string {
	result := map[string]string{}
	for _, descriptor := range sourceRegistry {
		if strings.TrimSpace(descriptor.MediaEvidenceAdapterVersion) != "" {
			result[string(descriptor.ID)] = descriptor.MediaEvidenceAdapterVersion
		}
	}
	return result
}

var linkedInProgressivePlaybackPath = regexp.MustCompile(`/mp4-[0-9]{2,4}p(?:-|/)`)

func CanonicalInlinePlaybackURL(source Source, raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" {
		return "", false
	}
	hostname := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	switch source {
	case SourceLinkedIn:
		if hostname != "dms.licdn.com" || !strings.HasPrefix(path, "/playlist/") || !linkedInProgressivePlaybackPath.MatchString(path) {
			return "", false
		}
	case SourceFacebook:
		trustedHost := hostname == "fbcdn.net" || hostname == "fbsbx.com" || strings.HasSuffix(hostname, ".fbcdn.net") || strings.HasSuffix(hostname, ".fbsbx.com")
		if !trustedHost || !strings.HasSuffix(path, ".mp4") {
			return "", false
		}
	case SourceInstagram:
		trustedHost := hostname == "fbcdn.net" || hostname == "cdninstagram.com" || strings.HasSuffix(hostname, ".fbcdn.net") || strings.HasSuffix(hostname, ".cdninstagram.com")
		if !trustedHost || !strings.HasSuffix(path, ".mp4") {
			return "", false
		}
	default:
		return "", false
	}
	parsed.Fragment = ""
	return parsed.String(), true
}

func SupportsPlaybackErrorRecapture(source Source) bool {
	descriptor, ok := SourceByID(source)
	return ok && descriptor.PlaybackRecoveryCapability == "native_post_recapture"
}
