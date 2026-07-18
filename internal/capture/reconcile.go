package capture

import (
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var punctuationSpacing = regexp.MustCompile(`\s*([.,!?;:])\s*`)

// ReconcileSnapshots gives repeated capture shapes one canonical evidence
// identity before reasoning, diagnostics, or user correction can act on them.
func ReconcileSnapshots(source domain.Source, snapshots []domain.Snapshot) []domain.Snapshot {
	bestBySignature := map[string]domain.Block{}
	for _, snapshot := range snapshots {
		for _, block := range snapshot.Blocks {
			signature := contentSignature(source, block)
			if signature == "" {
				continue
			}
			previous, exists := bestBySignature[signature]
			if !exists {
				bestBySignature[signature] = block
				continue
			}
			merged := mergeBlock(previous, block)
			if hasStableIdentity(previous) && !hasStableIdentity(block) {
				merged.Permalink = previous.Permalink
				merged.PlatformID = previous.PlatformID
				merged.EvidenceKey = previous.EvidenceKey
				merged.Presentation = mergeMaps(merged.Presentation, previous.Presentation)
			}
			bestBySignature[signature] = merged
		}
	}

	result := make([]domain.Snapshot, len(snapshots))
	for snapshotIndex, snapshot := range snapshots {
		result[snapshotIndex] = snapshot
		result[snapshotIndex].Blocks = make([]domain.Block, len(snapshot.Blocks))
		for blockIndex, block := range snapshot.Blocks {
			best, exists := bestBySignature[contentSignature(source, block)]
			if !exists || !hasStableIdentity(best) {
				result[snapshotIndex].Blocks[blockIndex] = block
				continue
			}
			merged := mergeBlock(block, best)
			merged.FeedPosition = block.FeedPosition
			merged.Permalink = best.Permalink
			if best.PlatformID != "" {
				merged.PlatformID = best.PlatformID
			}
			merged.EvidenceKey = best.EvidenceKey
			merged.Presentation = mergeMaps(block.Presentation, best.Presentation)
			result[snapshotIndex].Blocks[blockIndex] = merged
		}
	}
	return result
}

func contentSignature(source domain.Source, block domain.Block) string {
	author := strings.ToLower(strings.Join(strings.Fields(block.Author), " "))
	text := strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
	text = punctuationSpacing.ReplaceAllString(text, "$1")
	text = strings.TrimSpace(text)
	if author == "" || len(text) < 80 {
		return ""
	}
	if len(text) > 500 {
		text = text[:500]
	}
	return string(source) + "\x00" + author + "\x00" + text
}

func hasStableIdentity(block domain.Block) bool {
	return block.EvidenceKey != "" && (block.PlatformID != "" || block.Permalink != "")
}

func mergeBlock(previous, current domain.Block) domain.Block {
	result := current
	if result.Author == "" {
		result.Author = previous.Author
	}
	if result.AvatarURL == "" {
		result.AvatarURL = previous.AvatarURL
	}
	if len(previous.Text) > len(result.Text) {
		result.Text = previous.Text
	}
	if result.Permalink == "" {
		result.Permalink = previous.Permalink
	}
	if result.PlatformID == "" {
		result.PlatformID = previous.PlatformID
	}
	if result.PublishedAt == nil {
		result.PublishedAt = previous.PublishedAt
	}
	if result.ContentKind == "" {
		result.ContentKind = previous.ContentKind
	}
	if result.RelationshipType == "" {
		result.RelationshipType = previous.RelationshipType
	}
	if result.ParentPermalink == "" {
		result.ParentPermalink = previous.ParentPermalink
	}
	result.FeedPosition = earlierFeedPosition(previous.FeedPosition, current.FeedPosition)
	result.Engagement = mergeMaps(previous.Engagement, current.Engagement)
	result.Presentation = mergeMaps(previous.Presentation, current.Presentation)
	result.QuotedPost = mergeMaps(previous.QuotedPost, current.QuotedPost)
	result.CaptureQuality = mergeMaps(previous.CaptureQuality, current.CaptureQuality)
	result.Attachments = mergeAttachments(previous.Attachments, current.Attachments)
	if len(previous.Media) > len(current.Media) {
		result.Media = previous.Media
		result.MediaRecovery = previous.MediaRecovery
	} else if len(result.MediaRecovery) == 0 {
		result.MediaRecovery = previous.MediaRecovery
	}
	result.Links = mergeLinks(previous.Links, current.Links)
	return result
}

func earlierFeedPosition(previous, current int) int {
	if previous <= 0 {
		return current
	}
	if current <= 0 || previous < current {
		return previous
	}
	return current
}

func mergeMaps(previous, current map[string]any) map[string]any {
	if len(previous) == 0 && len(current) == 0 {
		return nil
	}
	result := make(map[string]any, len(previous)+len(current))
	for key, value := range previous {
		result[key] = value
	}
	for key, value := range current {
		result[key] = value
	}
	return result
}

func mergeLinks(previous, current []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(previous)+len(current))
	seen := map[string]bool{}
	for _, values := range [][]map[string]any{previous, current} {
		for _, link := range values {
			href, _ := link["href"].(string)
			key := strings.TrimSpace(href)
			if key == "" {
				key, _ = link["url"].(string)
				key = strings.TrimSpace(key)
			}
			if key != "" && seen[key] {
				continue
			}
			if key != "" {
				seen[key] = true
			}
			result = append(result, link)
			if len(result) == 10 {
				return result
			}
		}
	}
	return result
}

func mergeAttachments(previous, current []domain.Attachment) []domain.Attachment {
	result := make([]domain.Attachment, 0, len(previous)+len(current))
	seen := map[string]bool{}
	for _, values := range [][]domain.Attachment{previous, current} {
		for _, attachment := range values {
			key := strings.TrimSpace(attachment.URL)
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, attachment)
			if len(result) >= 3 {
				return result
			}
		}
	}
	return result
}
