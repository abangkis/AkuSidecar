package engine

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/capture"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func reconcileCapturedSnapshots(source domain.Source, snapshots []domain.Snapshot) []domain.Snapshot {
	return capture.ReconcileSnapshots(source, snapshots)
}

func normalizeObservation(value *domain.Observation) {
	for snapshotIndex := range value.Snapshots {
		for blockIndex := range value.Snapshots[snapshotIndex].Blocks {
			block := &value.Snapshots[snapshotIndex].Blocks[blockIndex]
			if strings.TrimSpace(block.EvidenceKey) != "" {
				continue
			}
			identity := strings.TrimSpace(block.PlatformID)
			if identity == "" {
				identity = strings.TrimSpace(block.Permalink)
			}
			if identity == "" {
				author := strings.ToLower(strings.Join(strings.Fields(block.Author), " "))
				text := strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
				identity = author + "\x00" + text
				if strings.Trim(identity, "\x00") == "" {
					identity = ""
				}
			}
			if identity == "" {
				continue
			}
			digest := sha256.Sum256([]byte(string(value.Source) + "\x00" + identity))
			block.EvidenceKey = fmt.Sprintf("%s:%x", value.Source, digest[:12])
		}
	}
}

func validateObservation(value domain.Observation) error {
	if !value.Source.Valid() {
		return errors.New("invalid observation source")
	}
	if len(value.Snapshots) == 0 {
		return errors.New("observation has no snapshots")
	}
	seen := map[string]bool{}
	for _, snapshot := range value.Snapshots {
		for _, block := range snapshot.Blocks {
			if block.EvidenceKey == "" {
				return errors.New("captured block is missing evidenceKey")
			}
			if !blockHasEvidence(block) {
				return errors.New("captured block has no admissible text, media, attachment, or quoted-post evidence")
			}
			seen[block.EvidenceKey] = true
			if strings.TrimSpace(block.Permalink) != "" {
				if _, ok := domain.CanonicalSourceURL(value.Source, block.Permalink); !ok {
					return errors.New("captured block permalink is not a canonical native source URL")
				}
			}
			if err := domain.ValidateDirectContext(value.Source, block.DirectContext); err != nil {
				return err
			}
			if len(block.Attachments) > 3 {
				return errors.New("captured block exceeds the attachment limit")
			}
			for _, attachment := range block.Attachments {
				if err := attachment.Validate(); err != nil {
					return fmt.Errorf("captured block attachment is invalid: %w", err)
				}
			}
		}
	}
	if len(seen) == 0 {
		return errors.New("observation contains no evidence blocks")
	}
	if value.Coverage == nil {
		return errors.New("observation coverage is required")
	}
	return nil
}

func blockHasEvidence(block domain.Block) bool {
	if strings.TrimSpace(block.Text) != "" || len(block.Media) > 0 || len(block.Attachments) > 0 {
		return true
	}
	if len(block.QuotedPost) == 0 {
		return false
	}
	for _, key := range []string{"text", "media", "links"} {
		value, ok := block.QuotedPost[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return true
			}
		case []any:
			if len(typed) > 0 {
				return true
			}
		case []map[string]any:
			if len(typed) > 0 {
				return true
			}
		}
	}
	return false
}
