// Package xreply validates diagnostic observations without granting reply authority.
package xreply

import (
	"bytes"
	"encoding/json"
	"maps"
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

const (
	CoverageKey = "xReplyDiagnostics"
	schema      = "aku.x.reply-diagnostics.v1"
	maxBytes    = 16 * 1024
)

var candidateID = regexp.MustCompile(`^x:status:(\d{5,30})$`)

type report struct {
	Schema     string      `json:"schema"`
	Complete   *bool       `json:"complete"`
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	SnapshotIndex *int   `json:"snapshotIndex"`
	CandidateID   string `json:"candidateId"`
	DOM           string `json:"dom"`
	Lookup        string `json:"lookup"`
	Merge         string `json:"merge"`
}

// Bind returns only closed diagnostic outcomes for exact candidates in the
// retained observation. A malformed optional report never fails capture.
func Bind(observation domain.Observation, value any) *report {
	if observation.Source != domain.SourceX || value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > maxBytes {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result report
	if decoder.Decode(&result) != nil || result.Schema != schema || result.Complete == nil || !validCount(len(result.Candidates)) {
		return nil
	}
	type locator struct {
		index int
		id    string
	}
	seen := make(map[locator]bool, len(result.Candidates))
	candidates := result.Candidates[:0]
	for _, entry := range result.Candidates {
		match := candidateID.FindStringSubmatch(entry.CandidateID)
		if len(match) != 2 || entry.SnapshotIndex == nil || !validOutcomes(entry) {
			return nil
		}
		key := locator{*entry.SnapshotIndex, entry.CandidateID}
		if seen[key] {
			return nil
		}
		seen[key] = true
		if key.index < 0 || key.index >= len(observation.Snapshots) ||
			!uniqueOwner(observation.Snapshots[key.index], entry.CandidateID, match[1]) {
			*result.Complete = false
			continue
		}
		candidates = append(candidates, entry)
	}
	if len(candidates) == 0 {
		return nil
	}
	result.Candidates = candidates
	return &result
}

// WithoutReport detaches the report from merged or public run-level coverage.
func WithoutReport(coverage map[string]any) map[string]any {
	if _, ok := coverage[CoverageKey]; !ok {
		return coverage
	}
	result := maps.Clone(coverage)
	delete(result, CoverageKey)
	return result
}

func validCount(count int) bool {
	minimum, maximum := int64(1), int64(64)
	policy, _ := d.NewCountPolicy(&minimum, &maximum)
	return d.BoundedCount(d.Observed[int64]{Value: int64(count), Known: true}, policy).Known
}

func uniqueOwner(snapshot domain.Snapshot, candidate, id string) bool {
	count := 0
	for _, block := range snapshot.Blocks {
		if block.PlatformID != candidate && block.PlatformID != id {
			continue
		}
		permalink, ok := domain.CanonicalSourceURL(domain.SourceX, block.Permalink)
		if ok && strings.HasSuffix(permalink, "/status/"+id) {
			count++
		}
	}
	return count == 1
}

func validOutcomes(entry candidate) bool {
	switch entry.DOM {
	case "not_checked", "missing", "marker_only", "ambiguous", "parent_found", "error":
	default:
		return false
	}
	switch entry.Lookup {
	case "not_checked", "unavailable", "miss", "expired", "hit", "error", "deadline":
	default:
		return false
	}
	switch entry.Merge {
	case "no_relation", "unresolved", "ambiguous", "retained_dom", "retained_response", "budget_skipped":
		return true
	default:
		return false
	}
}
