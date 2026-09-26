package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var directXIdentity = regexp.MustCompile(`/status/(\d+)`)

func directPostIdentity(source domain.Source, raw string) string {
	canonical, ok := domain.CanonicalSourceURL(source, raw)
	if !ok {
		return ""
	}
	if source == domain.SourceX {
		m := directXIdentity.FindStringSubmatch(canonical)
		if len(m) == 2 {
			return m[1]
		}
	}
	return domain.NormalizeNativeIdentity(source, canonical)
}

// A local projection only: no new provider/Bridge work, persistence, or memory
// enrollment. Relationships do not depend on lexical topic eligibility.
func (s *Store) directContext(ctx context.Context, item domain.TimelineItem) ([]domain.DirectContext, error) {
	result := []domain.DirectContext{}
	if item.Evidence == nil {
		return result, nil
	}
	block := item.Evidence
	values, err := s.retainedDirectContext(ctx, item)
	if err != nil {
		return nil, err
	}
	// X quote evidence stays on the Timeline Block for inline display, but the
	// Related Context projection omits quote relations and legacy quotedPost.
	values = domain.MergeDirectContext(nil, values)
	for _, value := range values {
		if item.Source == domain.SourceX && value.Kind == "quotes" {
			continue
		}
		if domain.ValidateDirectContext(item.Source, []domain.DirectContext{value}) != nil {
			continue
		}
		targetID := domain.ContextObjectIdentity(value.Target)
		ownID := directPostIdentity(item.Source, block.Permalink)
		if ownID == "" {
			ownID = domain.ContextObjectIdentity(domain.ContextObject{Kind: "post", ID: block.PlatformID})
		}
		if value.Target.Kind == "post" && targetID != "" && targetID == ownID {
			continue
		}
		if value.Target.Kind == "post" && value.Target.Permalink == "" && targetID != "" {
			value.Target.Permalink = "https://x.com/i/status/" + targetID
		}
		value.Target.EvidenceOrigin = "embedded_capture"
		if value.Parent != nil {
			copy := *value.Parent
			copy.EvidenceOrigin = "embedded_capture"
			value.Parent = &copy
		}
		if value.Target.Kind == "post" && value.Target.Text == "" && !value.Target.HasMedia && targetID != "" {
			resolved, err := s.resolveDirectPost(ctx, item.Source, targetID, value.Target)
			if err != nil {
				return nil, err
			}
			value.Target = resolved
		}
		result = append(result, value)
	}
	return result, nil
}

func (s *Store) resolveDirectPost(ctx context.Context, source domain.Source, identity string, target domain.ContextObject) (domain.ContextObject, error) {
	// Filter native identities before hydration; age and unrelated newer rows
	// cannot hide a retained parent. Only a small exact-match set is hydrated.
	pattern := "https://x.com/%/status/" + identity
	rows, err := s.db.QueryContext(ctx, `SELECT t.id FROM timeline_items t JOIN sessions s ON s.id=t.session_id
 LEFT JOIN auto_update_batches b ON b.session_id=s.id
 LEFT JOIN timeline_evidence_overrides e ON e.timeline_id=t.id
 WHERE t.source=? AND s.status IN ('completed','partial') AND s.completed_at IS NOT NULL
 AND (b.state IS NULL OR b.state='visible') AND (
 COALESCE(json_extract(e.evidence_json,'$.permalink'),json_extract(t.evidence_snapshot_json,'$.permalink'),json_extract(t.item_json,'$.sourceUrl')) LIKE ? OR
 COALESCE(json_extract(e.evidence_json,'$.permalink'),json_extract(t.evidence_snapshot_json,'$.permalink'),json_extract(t.item_json,'$.sourceUrl')) LIKE ? OR
 COALESCE(json_extract(e.evidence_json,'$.permalink'),json_extract(t.evidence_snapshot_json,'$.permalink'),json_extract(t.item_json,'$.sourceUrl')) LIKE ?)
 ORDER BY t.created_at DESC LIMIT 8`, source, pattern, pattern+"/%", pattern+"?%")
	if err != nil {
		return target, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return target, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return target, err
	}
	for _, id := range ids {
		tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return target, err
		}
		item, err := timelineItemForRetentionTx(ctx, tx, id, false)
		tx.Rollback()
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return target, err
		}
		if item.Evidence != nil && directPostIdentity(source, item.Evidence.Permalink) == identity {
			b := item.Evidence
			if b.Text == "" && len(b.Media) == 0 {
				continue
			}
			target.Text = boundedDirectText(b.Text)
			target.Author = boundedDirectAuthor(b.Author)
			target.HasMedia = len(b.Media) > 0
			target.Availability = "captured"
			target.EvidenceOrigin = "local_timeline"
			// Timeline creation is not the evidence capture time.
			target.CapturedAt = ""
			return target, nil
		}
	}
	rows, err = s.db.QueryContext(ctx, `SELECT m.canonical_permalink,m.author,v.content,v.captured_at FROM memory_items m
 JOIN memory_content_versions v ON v.id=m.full_content_version_id AND v.released_at IS NULL
 WHERE m.source=? AND m.lifecycle_state='active' AND m.retention_tier='full_copy'
 AND (m.canonical_permalink LIKE ? OR m.canonical_permalink LIKE ? OR m.canonical_permalink LIKE ?)
 ORDER BY m.updated_at DESC LIMIT 8`, source, pattern, pattern+"/%", pattern+"?%")
	if err != nil {
		return target, err
	}
	defer rows.Close()
	for rows.Next() {
		var permalink, author, content, capturedAt string
		if err = rows.Scan(&permalink, &author, &content, &capturedAt); err != nil {
			return target, err
		}
		if directPostIdentity(source, permalink) == identity && strings.TrimSpace(content) != "" {
			// Memory full copies store authored text, not a serialized Block.
			target.Text = boundedDirectText(content)
			target.Author = boundedDirectAuthor(author)
			target.Availability = "captured"
			target.EvidenceOrigin = "local_memory"
			target.CapturedAt = capturedAt
			return target, nil
		}
	}
	return target, rows.Err()
}
func boundedDirectText(value string) string {
	r := []rune(value)
	if len(r) > 4000 {
		return string(r[:4000])
	}
	return value
}
func boundedDirectAuthor(value string) string {
	r := []rune(value)
	if len(r) > 300 {
		return string(r[:300])
	}
	return value
}

// Read newer observations of this same native post even if normal continuity
// skipped reevaluation. Actor changes never cause provider work from this path.
func (s *Store) retainedDirectContext(ctx context.Context, item domain.TimelineItem) ([]domain.DirectContext, error) {
	block := item.Evidence
	result := domain.MergeDirectContext(nil, block.DirectContext)
	identity := directPostIdentity(item.Source, block.Permalink)
	if identity == "" {
		return result, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT b.value FROM observations o
 JOIN runs r ON r.id=o.run_id JOIN sessions s ON s.id=r.session_id
 LEFT JOIN auto_update_batches batch ON batch.session_id=s.id,
 json_each(o.observation_json,'$.snapshots') snapshot,
 json_each(snapshot.value,'$.blocks') b
 WHERE o.source=? AND s.status IN ('completed','partial') AND s.completed_at IS NOT NULL
 AND (batch.state IS NULL OR batch.state='visible')
 AND (json_extract(b.value,'$.permalink')=? OR (?<>'' AND json_extract(b.value,'$.platformId')=?) OR (?<>'' AND json_extract(b.value,'$.evidenceKey')=?))
 AND json_array_length(json_extract(b.value,'$.directContext'))>0
 ORDER BY o.captured_at DESC,o.created_at DESC LIMIT 32`, item.Source, block.Permalink, block.PlatformID, block.PlatformID, block.EvidenceKey, block.EvidenceKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	observations := [][]domain.DirectContext{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var candidate domain.Block
		if json.Unmarshal([]byte(raw), &candidate) != nil || directPostIdentity(item.Source, candidate.Permalink) != identity {
			continue
		}
		if domain.ValidateDirectContext(item.Source, candidate.DirectContext) != nil {
			continue
		}
		observations = append(observations, candidate.DirectContext)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for index := len(observations) - 1; index >= 0; index-- {
		result = domain.MergeDirectContext(result, observations[index])
	}
	return result, nil
}
