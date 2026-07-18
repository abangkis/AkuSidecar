package domain

import "strings"

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
	NativeHosts                 []string                 `json:"nativeHosts"`
	NativePathTokens            []string                 `json:"nativePathTokens"`
	IdentityFormat              string                   `json:"identityFormat,omitempty"`
	AvatarFallback              string                   `json:"avatarFallback"`
	PassiveMediaCapability      string                   `json:"passiveMediaCapability,omitempty"`
	EngagementMetrics           []SourceEngagementMetric `json:"engagementMetrics"`
}

type SourceEngagementMetric struct {
	Key  string `json:"key"`
	Icon string `json:"icon"`
}

var sourceRegistry = []SourceDescriptor{
	{ID: SourceX, DisplayName: "X", ShortLabel: "X", IconText: "X", IconBackground: "#e7e9ea", IconForeground: "#0f1419", OnboardingDescription: "Your home timeline", PresentationStyle: "compact", SocialContextPlacement: "content", DefaultActive: true, AdapterVersion: "x-dom-v19", MediaEvidenceAdapterVersion: "x-response-evidence-v2", NativeHosts: []string{"x.com"}, NativePathTokens: []string{"/status/"}, IdentityFormat: "display_handle", AvatarFallback: "source_icon", PassiveMediaCapability: "x_response", EngagementMetrics: []SourceEngagementMetric{{Key: "reply", Icon: "○"}, {Key: "repost", Icon: "↻"}, {Key: "like", Icon: "♡"}, {Key: "view", Icon: "▥"}}},
	{ID: SourceLinkedIn, DisplayName: "LinkedIn", ShortLabel: "in", IconText: "in", IconBackground: "#0a66c2", IconForeground: "#ffffff", OnboardingDescription: "Your professional feed", PresentationStyle: "professional", SocialContextPlacement: "above", DefaultActive: true, AdapterVersion: "linkedin-dom-v15", ContinuationOverlapRequired: true, NativeHosts: []string{"www.linkedin.com"}, NativePathTokens: []string{"/posts/", "/feed/update/"}, AvatarFallback: "initials", EngagementMetrics: []SourceEngagementMetric{{Key: "like", Icon: "👍"}, {Key: "comment", Icon: "💬"}, {Key: "repost", Icon: "↻"}}},
	{ID: SourceFacebook, DisplayName: "Facebook", ShortLabel: "f", IconText: "f", IconBackground: "#0866ff", IconForeground: "#ffffff", OnboardingDescription: "Your Home Feed", PresentationStyle: "social", SocialContextPlacement: "above", DefaultActive: false, AdapterVersion: "facebook-dom-v1", NativeHosts: []string{"facebook.com", "www.facebook.com", "m.facebook.com"}, NativePathTokens: []string{"/posts/", "/permalink/", "/story.php", "/photo", "/videos/", "/reel/"}, AvatarFallback: "initials", EngagementMetrics: []SourceEngagementMetric{{Key: "like", Icon: "👍"}, {Key: "comment", Icon: "💬"}, {Key: "repost", Icon: "↻"}}},
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
