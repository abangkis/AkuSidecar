package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Reuse only relations previously issued by internal recapture. No coverage
// claim on a newly submitted observation establishes a persistent alias.
func savedPhotoParentKey(ctx context.Context, tx *sql.Tx, block domain.Block) (string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT e.evidence_json,t.evidence_key FROM timeline_evidence_overrides e
 JOIN timeline_items t ON t.id=e.timeline_id JOIN media_recaptures r ON r.id=e.recapture_id
 WHERE t.source='facebook' AND r.status='completed' AND json_extract(e.evidence_json,'$.platformId')=? LIMIT 3`, block.PlatformID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	key := ""
	count := 0
	for rows.Next() {
		count++
		var raw, canonical string
		if err := rows.Scan(&raw, &canonical); err != nil {
			return "", err
		}
		var stored domain.Block
		if json.Unmarshal([]byte(raw), &stored) != nil {
			continue
		}
		proof, ok := stored.MediaRecovery["photoParentResolution"].(map[string]any)
		if !ok || proof["status"] != "verified" || proof["provenance"] != "internal_headless_and_saved_photo_evidence" || proof["parentPlatformId"] != block.PlatformID {
			continue
		}
		if stored.EvidenceKey != canonical || canonical == "" || stored.Permalink != block.Permalink || strings.Join(strings.Fields(stored.Author), " ") != strings.Join(strings.Fields(block.Author), " ") {
			continue
		}
		if key != "" && key != canonical {
			return "", nil
		}
		key = canonical
	}
	if count >= 3 {
		return "", nil
	} // Bounded ambiguity, never choose an arbitrary saved item.
	return key, rows.Err()
}

var photoIdentityDigits = regexp.MustCompile(`^[0-9]{1,32}$`)
var photoParentPath = regexp.MustCompile(`^/[^/]+/posts/(pfbid[A-Za-z0-9]+|[0-9]+)/?$`)

func exactFacebookPhotoID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "www.facebook.com" || u.User != nil {
		return ""
	}
	if u.Path != "/photo" && u.Path != "/photo/" && u.Path != "/photo.php" {
		return ""
	}
	values := append(u.Query()["fbid"], u.Query()["photo_id"]...)
	if len(values) == 0 || !photoIdentityDigits.MatchString(values[0]) {
		return ""
	}
	for _, v := range values {
		if v != values[0] {
			return ""
		}
	}
	return values[0]
}

func photoMediaPath(raw any) string {
	s, _ := raw.(string)
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || !(u.Hostname() == "fbcdn.net" || strings.HasSuffix(u.Hostname(), ".fbcdn.net")) {
		return ""
	}
	return u.Scheme + "://" + u.Host + u.EscapedPath()
}

// This matcher is only invoked through the internal headless completion path.
// It corroborates worker evidence against the stored item, never text alone.
func recapturedPhotoParent(observation domain.Observation, job domain.MediaRecapture, original domain.Block) (domain.Block, bool) {
	fail := func() (domain.Block, bool) { return domain.Block{}, false }
	collector, err := CaptureCollector(job.Payload)
	if err != nil || collector != "headless" || job.Source != domain.SourceFacebook || observation.Source != job.Source {
		return fail()
	}
	photoID := exactFacebookPhotoID(job.TargetURL)
	if photoID == "" || exactFacebookPhotoID(original.Permalink) != photoID || original.EvidenceKey != job.EvidenceKey {
		return fail()
	}
	relation, ok := observation.Coverage["photoParentResolution"].(map[string]any)
	if !ok || relation["status"] != "verified" || relation["photoId"] != photoID || relation["provenance"] != "structured_photo_parent_and_matching_native_post" {
		return fail()
	}
	parentID, _ := relation["parentPlatformId"].(string)
	u, err := url.Parse(observation.PageURL)
	if err != nil || u.Scheme != "https" || u.Host != "www.facebook.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fail()
	}
	match := photoParentPath.FindStringSubmatch(u.Path)
	if len(match) != 2 || parentID != "facebook:post:"+match[1] {
		return fail()
	}
	norm := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	if norm(original.Author) == "" {
		return fail()
	}
	images := map[string]bool{}
	for _, m := range original.Media {
		if m["kind"] == "image" {
			if p := photoMediaPath(m["url"]); p != "" {
				images[p] = true
			}
		}
	}
	if len(images) == 0 && !(len(original.Media) == 0 && original.MediaRecovery["outcome"] == "unavailable" && norm(original.Text) != "") {
		return fail()
	}
	var selected domain.Block
	found := false
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			if block.PlatformID != parentID {
				continue
			}
			if block.Permalink != observation.PageURL || norm(block.Author) != norm(original.Author) || norm(block.Text) != norm(original.Text) {
				return fail()
			}
			present := map[string]bool{}
			for _, m := range block.Media {
				if m["kind"] == "image" {
					present[photoMediaPath(m["url"])] = true
				}
			}
			for p := range images {
				if !present[p] {
					return fail()
				}
			}
			delete(present, "")
			if len(present) == 0 {
				return fail()
			}
			selected = block
			found = true
		}
	}
	if !found {
		return fail()
	}
	// Preserve the saved item's key while retaining the real parent platform ID.
	selected.EvidenceKey = job.EvidenceKey
	selected.MediaRecovery = mergeAnyValues(selected.MediaRecovery, map[string]any{"photoParentResolution": map[string]any{
		"status": "verified", "photoId": photoID, "parentPlatformId": parentID, "provenance": "internal_headless_and_saved_photo_evidence",
	}})
	return selected, true
}
