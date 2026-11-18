package store

import "github.com/abangkis/AkuSidecar/internal/domain"

type timelinePostKey struct {
	source domain.Source
	id     string
}

type timelinePostIndexEntry struct {
	timelineID string
	count      int
}

// projectTimelineRelations attaches reply edges only when their exact native
// identity can be related to an item in this already-selected Timeline page.
// It performs no store reads and only writes the response projection field.
func projectTimelineRelations(items []domain.TimelineItem) {
	parents := make(map[timelinePostKey]timelinePostIndexEntry, len(items))
	identities := make([]string, len(items))
	for index := range items {
		item := &items[index]
		// This value is derived here, never trusted from persisted or provider data.
		item.Conversation = nil
		identity := timelineItemPostIdentity(*item)
		identities[index] = identity
		if identity == "" {
			continue
		}
		key := timelinePostKey{source: item.Source, id: identity}
		entry := parents[key]
		if entry.count == 0 {
			entry.timelineID = item.ID
		}
		entry.count++
		parents[key] = entry
	}

	for index := range items {
		item := &items[index]
		if identities[index] == "" || item.Evidence == nil || len(item.Evidence.DirectContext) == 0 || len(item.Evidence.DirectContext) > domain.DirectContextLimit {
			continue
		}

		parentIDs := make(map[string]struct{})
		provenance := ""
		for _, relation := range item.Evidence.DirectContext {
			if relation.Kind != "replies_to" || relation.Target.Kind != "post" {
				continue
			}
			if domain.ValidateDirectContext(item.Source, []domain.DirectContext{relation}) != nil {
				continue
			}
			nativeID := domain.ReplyPostIdentity(item.Source, relation.Target.ID, relation.Target.Permalink)
			if nativeID == "" || nativeID == identities[index] {
				continue
			}
			parentIDs[nativeID] = struct{}{}
			// Multiple valid observations of the same edge corroborate it. Keep
			// the strongest observed provenance rather than erase the relation.
			if provenance == "" || relation.Provenance == "observed_response" ||
				(provenance == "legacy_capture" && relation.Provenance == "observed_dom") {
				provenance = relation.Provenance
			}
		}
		if len(parentIDs) == 0 {
			continue
		}

		projection := &domain.TimelineConversation{
			Kind:       "replies_to",
			Provenance: provenance,
		}
		if len(parentIDs) > 1 {
			projection.Status = "ambiguous"
			item.Conversation = projection
			continue
		}
		var parentID string
		for identity := range parentIDs {
			parentID = identity
		}
		entry, exists := parents[timelinePostKey{source: item.Source, id: parentID}]
		switch {
		case !exists:
			projection.Status = "not_in_view"
		case entry.count > 1:
			projection.Status = "ambiguous"
		default:
			projection.Status = "resolved"
			projection.TargetTimelineID = entry.timelineID
		}
		item.Conversation = projection
	}
}

func timelineItemPostIdentity(item domain.TimelineItem) string {
	if item.Evidence == nil {
		return ""
	}
	return domain.ReplyPostIdentity(item.Source, item.Evidence.PlatformID, item.Evidence.Permalink)
}
