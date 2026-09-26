package domain

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const DirectContextLimit = 8

// DirectContext is captured structural evidence. The owning Block is the root
// post; target is its quote/parent or the feed actor's comment, and parent is
// at most one ancestor comment. It does not assert acquaintance or feed rank.
type DirectContext struct {
	Kind         string         `json:"kind"`
	Actor        string         `json:"actor,omitempty"`
	ActorURL     string         `json:"actorUrl,omitempty"`
	ObservedText string         `json:"observedText,omitempty"`
	Provenance   string         `json:"provenance"`
	CapturedAt   string         `json:"capturedAt,omitempty"`
	Target       ContextObject  `json:"target"`
	Parent       *ContextObject `json:"parent,omitempty"`
}

type ContextObject struct {
	Kind         string `json:"kind"`
	ID           string `json:"id,omitempty"`
	Permalink    string `json:"permalink,omitempty"`
	Author       string `json:"author,omitempty"`
	Text         string `json:"text,omitempty"`
	HasMedia     bool   `json:"hasMedia,omitempty"`
	Availability string `json:"availability"`
	// EvidenceOrigin is set by local resolution, never by a provider.
	EvidenceOrigin string `json:"evidenceOrigin,omitempty"`
	CapturedAt     string `json:"capturedAt,omitempty"`
}

func ValidateDirectContext(source Source, values []DirectContext) error {
	if len(values) > DirectContextLimit {
		return errors.New("too many direct context relations")
	}
	for _, value := range values {
		if source == SourceX && value.Kind != "quotes" && value.Kind != "replies_to" ||
			source == SourceLinkedIn && value.Kind != "feed_comment" && value.Kind != "feed_reply" ||
			source != SourceX && source != SourceLinkedIn {
			return errors.New("unsupported direct context relation")
		}
		if value.Provenance != "observed_dom" && value.Provenance != "observed_response" && value.Provenance != "legacy_capture" {
			return errors.New("invalid direct context provenance")
		}
		if len([]rune(value.Actor)) > 300 || len([]rune(value.ObservedText)) > 500 {
			return errors.New("direct context text exceeds limit")
		}
		if value.CapturedAt != "" {
			if _, err := time.Parse(time.RFC3339Nano, value.CapturedAt); err != nil {
				return errors.New("invalid direct context capture time")
			}
		}
		if value.ActorURL != "" {
			u, err := url.Parse(value.ActorURL)
			if err != nil || u.Scheme != "https" || u.Host != "www.linkedin.com" || u.User != nil || !strings.HasPrefix(u.Path, "/in/") || len(value.ActorURL) > 2048 {
				return errors.New("invalid direct context actor URL")
			}
		}
		objects := []ContextObject{value.Target}
		if value.Parent != nil {
			if value.Kind != "feed_reply" {
				return errors.New("parent only applies to comment replies")
			}
			objects = append(objects, *value.Parent)
		}
		for _, object := range objects {
			expected := "post"
			if source == SourceLinkedIn {
				expected = "comment"
			}
			if object.Kind != expected || len(object.ID) > 200 || len(object.Permalink) > 2048 || len([]rune(object.Author)) > 300 || len([]rune(object.Text)) > 4000 {
				return errors.New("invalid direct context object")
			}
			if object.Availability != "captured" && object.Availability != "partial" && object.Availability != "reference_only" {
				return errors.New("invalid direct context availability")
			}
			if object.Permalink != "" {
				if _, ok := CanonicalSourceURL(source, object.Permalink); !ok {
					return errors.New("invalid direct context native URL")
				}
			}
			if object.CapturedAt != "" {
				if _, err := time.Parse(time.RFC3339Nano, object.CapturedAt); err != nil {
					return errors.New("invalid context object capture time")
				}
			}
			if source == SourceX && object.ID != "" {
				id := strings.TrimPrefix(object.ID, "x:status:")
				if !directStatusIDPattern.MatchString(id) {
					return errors.New("invalid X context object ID")
				}
				if object.Permalink != "" && ContextObjectIdentity(ContextObject{Kind: "post", Permalink: object.Permalink}) != id {
					return errors.New("context object ID and permalink disagree")
				}
			}
			if object.Availability == "captured" && object.Text == "" && !object.HasMedia {
				return errors.New("captured context object has no body evidence")
			}
			if object.Availability == "reference_only" && (object.Text != "" || object.HasMedia) {
				return errors.New("reference-only context object contains body evidence")
			}
			if object.EvidenceOrigin != "" && object.EvidenceOrigin != "embedded_capture" && object.EvidenceOrigin != "local_timeline" && object.EvidenceOrigin != "local_memory" {
				return errors.New("invalid direct context evidence origin")
			}
		}
	}
	return nil
}

var directStatusIDPattern = regexp.MustCompile(`^\d{5,30}$`)

// ContextObjectIdentity folds X username/i URL aliases to the actual status ID.
// Unknown identities remain empty, so missing IDs never bind unrelated bodies.
func ContextObjectIdentity(object ContextObject) string {
	if object.Kind == "post" {
		if object.ID != "" {
			id := strings.TrimPrefix(object.ID, "x:status:")
			if directStatusIDPattern.MatchString(id) {
				return id
			}
			return ""
		}
		if canonical, ok := CanonicalSourceURL(SourceX, object.Permalink); ok {
			parsed, _ := url.Parse(canonical)
			match := xNativeStatusPathPattern.FindStringSubmatch(parsed.Path)
			if len(match) == 3 {
				return match[2]
			}
		}
		return ""
	}
	if object.ID != "" {
		return strings.TrimPrefix(object.ID, "urn:li:comment:")
	}
	return object.Permalink
}

// Merge by stable target and actor identity. Reference-only recapture must not
// erase an already captured body. A genuinely refreshed body replaces it.
func MergeDirectContext(previous, current []DirectContext) []DirectContext {
	result := make([]DirectContext, 0, DirectContextLimit)
	seen := map[string]int{}
	for _, group := range [][]DirectContext{current, previous} {
		if len(group) > DirectContextLimit {
			group = group[:DirectContextLimit]
		}
		for _, value := range group {
			if value.Target.CapturedAt == "" && (value.Target.Text != "" || value.Target.HasMedia) {
				value.Target.CapturedAt = value.CapturedAt
			}
			if value.Parent != nil {
				copy := *value.Parent
				if copy.CapturedAt == "" && (copy.Text != "" || copy.HasMedia) {
					copy.CapturedAt = value.CapturedAt
				}
				value.Parent = &copy
			}
			identity := ContextObjectIdentity(value.Target)
			actor := value.ActorURL
			if actor == "" {
				actor = value.Actor
			}
			key := value.Kind + "\n" + actor + "\n" + identity
			if identity == "" {
				key += "\n" + value.ObservedText + "\n" + value.Target.Author + "\n" + value.Target.Text
			}
			if index, ok := seen[key]; ok {
				if identity != "" {
					// Older retained observations cannot replace a newer
					// recapture override. Unknown timestamps assert no freshness.
					if contextCaptureNewer(value.CapturedAt, result[index].CapturedAt) {
						prior := result[index]
						result[index] = value
						value = prior
					}
					result[index].Target = mergeContextObject(value.Target, result[index].Target)
					if result[index].Parent == nil && value.Parent != nil {
						copy := *value.Parent
						result[index].Parent = &copy
					} else if result[index].Parent != nil && value.Parent != nil {
						merged := mergeContextObject(*value.Parent, *result[index].Parent)
						result[index].Parent = &merged
					}
				}
				continue
			}
			if len(result) == DirectContextLimit {
				continue
			}
			seen[key] = len(result)
			result = append(result, value)
		}
	}
	return result
}

func mergeContextObject(previous, current ContextObject) ContextObject {
	identity := ContextObjectIdentity(current)
	if identity == "" || identity != ContextObjectIdentity(previous) {
		return current
	}
	if (previous.Text != "" || previous.HasMedia) && contextCaptureNewer(previous.CapturedAt, current.CapturedAt) {
		return previous
	}
	if (current.Availability == "reference_only" || current.Availability == "partial") && current.Text == "" && !current.HasMedia && (previous.Text != "" || previous.HasMedia) {
		return previous
	}
	return current
}

func contextCaptureNewer(candidate, current string) bool {
	candidateTime, err := time.Parse(time.RFC3339Nano, candidate)
	if err != nil {
		return false
	}
	currentTime, err := time.Parse(time.RFC3339Nano, current)
	return err != nil || candidateTime.After(currentTime)
}
