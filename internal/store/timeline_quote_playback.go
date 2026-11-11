package store

import (
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Keep the first observation's content precedence while accepting playback
// hydration for an already identified quote. This is a read-only projection.
func enrichTimelineQuotePlayback(source domain.Source, previous, current domain.Block) domain.Block {
	if source != domain.SourceX || previous.EvidenceKey != current.EvidenceKey ||
		previous.RelationshipType != "quote" || current.RelationshipType != "quote" ||
		previous.Text != current.Text || !strings.EqualFold(previous.Author, current.Author) ||
		!sameXQuoteText(previous, current) {
		return previous
	}
	primary := timelineItemPostIdentity(domain.TimelineItem{Source: source, Evidence: &previous})
	if primary == "" || primary != timelineItemPostIdentity(domain.TimelineItem{Source: source, Evidence: &current}) {
		return previous
	}
	quote := directPostIdentity(source, stringValue(previous.QuotedPost, "permalink"))
	if quote == "" || quote == primary || quote != directPostIdentity(source, stringValue(current.QuotedPost, "permalink")) {
		return previous
	}
	for _, block := range []domain.Block{previous, current} {
		if block.PlatformID != "" && domain.ContextObjectIdentity(domain.ContextObject{Kind: "post", ID: block.PlatformID}) == "" {
			return previous
		}
		if parent, compatible := xQuoteParentID(block, primary); !compatible || parent != quote {
			return previous
		}
	}
	media := timelineQuoteMedia(previous.QuotedPost["media"])
	incoming := timelineQuoteMedia(current.QuotedPost["media"])
	if len(media) == 0 || len(media) > 4 || len(incoming) > 4 {
		return previous
	}
	var updated []map[string]any
	for index, original := range media {
		asset := timelineQuoteVideoAsset(original)
		if asset == "" || timelineQuotePlaybackValid(original, asset) {
			continue
		}
		for _, candidate := range incoming {
			if timelineQuoteVideoAsset(candidate) != asset || !timelineQuotePlaybackValid(candidate, asset) {
				continue
			}
			if updated == nil {
				updated = append([]map[string]any(nil), media...)
			}
			copy := mergeAnyValues(nil, original)
			copy["kind"] = "video"
			copy["playbackMode"] = "inline"
			copy["playbackUrl"] = candidate["playbackUrl"]
			updated[index] = copy
			break
		}
	}
	if updated != nil {
		previous.QuotedPost = mergeAnyValues(nil, previous.QuotedPost)
		previous.QuotedPost["media"] = updated
	}
	return previous
}

func timelineQuoteMedia(value any) []map[string]any {
	switch values := value.(type) {
	case []map[string]any:
		return values
	case []any:
		if len(values) > 4 {
			return nil
		}
		result := make([]map[string]any, 0, len(values))
		for _, value := range values {
			entry, ok := value.(map[string]any)
			if !ok {
				return nil
			}
			result = append(result, entry)
		}
		return result
	default:
		return nil
	}
}

func timelineQuoteVideoAsset(media map[string]any) string {
	if media["kind"] != "video" && media["kind"] != "video_poster" {
		return ""
	}
	poster := stringValue(media, "posterUrl")
	if poster == "" {
		poster = stringValue(media, "url")
	}
	match := xHLSPosterPath.FindStringSubmatch(xHLSCDNPath(poster, "pbs.twimg.com"))
	if match == nil {
		return ""
	}
	return "/" + match[1] + "/" + match[2] + "/"
}

func timelineQuotePlaybackValid(media map[string]any, asset string) bool {
	if media["kind"] != "video" || media["playbackMode"] != "inline" {
		return false
	}
	block := domain.Block{Media: []map[string]any{media}}
	if hasInlineXHLSPlayback(block) {
		return true
	}
	playback, ok := domain.CanonicalInlinePlaybackURL(domain.SourceX, stringValue(media, "playbackUrl"))
	return ok && strings.HasPrefix(xHLSCDNPath(playback, "video.twimg.com"), asset)
}
