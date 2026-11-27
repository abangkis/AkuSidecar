package store

import (
	"context"
	"encoding/json"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type timelinePostKey struct {
	source domain.Source
	id     string
}

type timelinePostIndexEntry struct {
	timelineID string
	count      int
}

type timelineReplyParent struct {
	id, provenance string
	ambiguous      bool
}

func (p *timelineReplyParent) observe(source domain.Source, owner string, relations []domain.DirectContext) {
	for _, relation := range relations {
		if relation.Kind != "replies_to" || relation.Target.Kind != "post" || domain.ValidateDirectContext(source, []domain.DirectContext{relation}) != nil {
			continue
		}
		id := domain.ReplyPostIdentity(source, relation.Target.ID, relation.Target.Permalink)
		if id == "" || id == owner {
			continue
		}
		if p.id != "" && p.id != id {
			p.ambiguous = true
		}
		p.id = id
		if p.provenance == "" || relation.Provenance == "observed_response" || (p.provenance == "legacy_capture" && relation.Provenance == "observed_dom") {
			p.provenance = relation.Provenance
		}
	}
}

// A targeted capture can complete without producing a new Timeline card.
// Match its durable target and captured block to the visible post's native ID.
func (s *Store) timelineRecapturedParents(ctx context.Context, items []domain.TimelineItem) (map[string]timelineReplyParent, error) {
	var ids []string
	for _, item := range items {
		if id := timelineItemPostIdentity(item); item.Source == domain.SourceX && id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	pageIDs, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `WITH targets AS (
		SELECT s.id, json_extract(s.coverage_json,'$.captureTarget.permalink') AS url,
			substr(json_extract(s.coverage_json,'$.captureTarget.permalink'),length('https://x.com/')+1) AS path
		FROM sessions s LEFT JOIN auto_update_batches batch ON batch.session_id=s.id
		WHERE s.status IN ('completed','partial') AND s.completed_at IS NOT NULL
		AND (batch.state IS NULL OR batch.state='visible')
		AND json_extract(s.coverage_json,'$.captureTarget.source')='x'
	)
	SELECT targets.url, coalesce(json_extract(o.observation_json,'$.pageUrl'),''), b.value FROM targets
	CROSS JOIN runs r ON r.session_id=targets.id AND r.source='x' AND r.status='completed'
	CROSS JOIN observations o ON o.run_id=r.id AND o.source='x'
	CROSS JOIN json_each(o.observation_json,'$.snapshots') snap
	CROSS JOIN json_each(snap.value,'$.blocks') b
	WHERE substr(targets.path,instr(targets.path,'/status/')+length('/status/')) IN (SELECT value FROM json_each(?))
	AND (json_array_length(b.value,'$.directContext')>0 OR json_extract(b.value,'$.presentation.directContextStatus')='ambiguous')`, string(pageIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	parents := make(map[string]timelineReplyParent)
	for rows.Next() {
		var target, pageURL, raw string
		if err := rows.Scan(&target, &pageURL, &raw); err != nil {
			return nil, err
		}
		var block domain.Block
		if json.Unmarshal([]byte(raw), &block) != nil || len(block.DirectContext) > domain.DirectContextLimit {
			continue
		}
		id := domain.ReplyPostIdentity(domain.SourceX, block.PlatformID, block.Permalink)
		if id == "" || id != domain.ReplyPostIdentity(domain.SourceX, "", target) || id != domain.ReplyPostIdentity(domain.SourceX, "", pageURL) {
			continue
		}
		parent := parents[id]
		parent.observe(domain.SourceX, id, block.DirectContext)
		if block.Presentation["directContextStatus"] == "ambiguous" {
			parent.ambiguous = true
		}
		parents[id] = parent
	}
	return parents, rows.Err()
}

// projectTimelineRelations attaches reply edges only when their exact native
// identity can be related to an item in this already-selected Timeline page.
// It performs no store reads and only writes the response projection field.
func projectTimelineRelations(items []domain.TimelineItem, recaptured map[string]timelineReplyParent) {
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
		if identities[index] == "" || item.Evidence == nil || len(item.Evidence.DirectContext) > domain.DirectContextLimit {
			continue
		}

		var parent timelineReplyParent
		if item.Source == domain.SourceX {
			parent = recaptured[identities[index]]
			if item.Evidence.Presentation["directContextStatus"] == "ambiguous" {
				parent.ambiguous = true
			}
		}
		parent.observe(item.Source, identities[index], item.Evidence.DirectContext)
		if parent.id == "" {
			continue
		}

		projection := &domain.TimelineConversation{
			Kind:       "replies_to",
			Provenance: parent.provenance,
		}
		if parent.ambiguous {
			projection.Status = "ambiguous"
			item.Conversation = projection
			continue
		}
		entry, exists := parents[timelinePostKey{source: item.Source, id: parent.id}]
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
