package store

import (
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Internal media-only evidence may refresh a saved photo, never replace it with
// its containing post. Saved author, caption, identity and relationships survive.
func recapturedPhotoMedia(o domain.Observation, job domain.MediaRecapture, original domain.Block) (domain.Block, bool) {
	fail := func() (domain.Block, bool) { return domain.Block{}, false }
	collector, err := CaptureCollector(job.Payload)
	id := exactFacebookPhotoID(job.TargetURL)
	if err != nil || collector != "headless" || job.Source != domain.SourceFacebook || o.Source != job.Source ||
		id == "" || exactFacebookPhotoID(original.Permalink) != id || exactFacebookPhotoID(o.PageURL) != id ||
		original.EvidenceKey != job.EvidenceKey || mediaRecaptureReason(job.Payload) != domain.MediaRecaptureMissingMedia {
		return fail()
	}
	proof, ok := o.Coverage["photoMediaRecapture"].(map[string]any)
	if !ok || proof["status"] != "verified" || proof["photoId"] != id || proof["provenance"] != "exact_photo_metadata_and_visible_image" {
		return fail()
	}
	owner, _ := proof["ownerId"].(string)
	if !photoIdentityDigits.MatchString(owner) || len(o.Snapshots) != 1 || len(o.Snapshots[0].Blocks) != 1 {
		return fail()
	}
	b := o.Snapshots[0].Blocks[0]
	if b.PlatformID != "facebook:photo:"+id || exactFacebookPhotoID(b.Permalink) != id || len(b.Media) != 1 ||
		b.Media[0]["kind"] != "image" || photoMediaPath(b.Media[0]["url"]) == "" {
		return fail()
	}
	// Missing-media jobs cannot replace existing images or widen an album's scope.
	if len(original.Media) != 0 || original.MediaRecovery["outcome"] != "unavailable" {
		return fail()
	}
	m := b.Media[0]
	original.Media = []map[string]any{{"kind": "image", "url": m["url"]}}
	original.MediaRecovery = mergeAnyValues(original.MediaRecovery, map[string]any{
		"outcome": "recovered", "recoveredCount": 1, "method": "exact_photo_media_recapture",
		"foregroundRequired": false, "foregroundAuthorized": false,
	})
	delete(original.MediaRecovery, "limitation")
	delete(original.MediaRecovery, "visibilityRequirement")
	return original, true
}
