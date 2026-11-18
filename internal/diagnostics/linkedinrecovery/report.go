// Package linkedinrecovery validates optional LinkedIn surface diagnostics.
package linkedinrecovery

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"

	"github.com/abangkis/AkuSidecar/internal/domain"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

const CoverageKey = "linkedinRecoveryBlockers"

type inspection struct {
	DialogCount *int      `json:"dialogCount"`
	Complete    *bool     `json:"complete"`
	Surfaces    []surface `json:"surfaces"`
}

type surface struct {
	Kind             string          `json:"kind"`
	Slot             *int            `json:"slot"`
	Trail            []string        `json:"trail"`
	ActionHints      []string        `json:"actionHints"`
	ActionCount      *int            `json:"actionCount"`
	OwnerRelation    string          `json:"ownerRelation"`
	TriggerBinding   string          `json:"triggerBinding"`
	TriggerTrail     []string        `json:"triggerTrail"`
	TriggerExpanded  json.RawMessage `json:"triggerExpanded"`
	OpenState        string          `json:"openState"`
	ControlOcclusion string          `json:"controlOcclusion"`
}

var components = []string{"messaging", "feed_sort", "post", "navigation", "header", "sidebar", "footer", "main"}
var actions = []string{"post_link", "save_post", "report_post", "account", "message", "feed_order", "navigation", "dismiss"}

// ProjectCoverage sanitizes the optional extension while preserving old
// reports with no extension. A diagnostic failure leaves captured posts intact.
func ProjectCoverage(source domain.Source, coverage map[string]any) map[string]any {
	value, exists := coverage[CoverageKey]
	if !exists {
		return coverage
	}
	result := maps.Clone(coverage)
	if source != domain.SourceLinkedIn {
		delete(result, CoverageKey)
		return result
	}
	entries, ok := value.([]any)
	if !ok || len(entries) > 8 {
		delete(result, CoverageKey)
		return result
	}
	projected := slices.Clone(entries)
	for index, value := range projected {
		entry, ok := value.(map[string]any)
		if !ok {
			continue
		}
		raw, exists := entry["surfaceInspection"]
		if !exists {
			continue
		}
		entry = maps.Clone(entry)
		delete(entry, "surfaceInspection")
		if entry["code"] == "existing_surface" && entry["phase"] == "pre_recovery_guard" {
			if validated := projectInspection(raw); validated != nil {
				entry["surfaceInspection"] = validated
			}
		}
		projected[index] = entry
	}
	result[CoverageKey] = projected
	return result
}

func projectInspection(value any) *inspection {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 4096 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result inspection
	if decoder.Decode(&result) != nil || !countWithin(result.DialogCount, 8) || result.Complete == nil ||
		result.Surfaces == nil || len(result.Surfaces) > 4 {
		return nil
	}
	dialogs := 0
	for index, entry := range result.Surfaces {
		if !validSurface(entry, index) {
			return nil
		}
		if entry.Kind == "dialog" {
			dialogs++
		}
	}
	if dialogs != min(*result.DialogCount, 2) || len(result.Surfaces)-dialogs > 2 || (*result.Complete && *result.DialogCount > 2) {
		return nil
	}
	return &result
}

func validSurface(entry surface, index int) bool {
	return slices.Contains([]string{"menu", "dialog"}, entry.Kind) && entry.Slot != nil && *entry.Slot == index &&
		countWithin(entry.ActionCount, 24) && validList(entry.ActionHints, actions, 8) &&
		validList(entry.Trail, components, 4) && validList(entry.TriggerTrail, components, 4) &&
		slices.Contains([]string{"candidate_post", "other_post", "unknown"}, entry.OwnerRelation) &&
		slices.Contains([]string{"target_reference", "label_reference", "component_toggle", "ambiguous", "unresolved"}, entry.TriggerBinding) &&
		slices.Contains([]string{"true", "false", "null"}, string(entry.TriggerExpanded)) &&
		slices.Contains([]string{"open", "closed", "unknown"}, entry.OpenState) &&
		slices.Contains([]string{"target_hit", "surface_hit", "other_hit", "unknown"}, entry.ControlOcclusion)
}

func countWithin(value *int, maximum int64) bool {
	if value == nil {
		return false
	}
	minimum := int64(0)
	policy, _ := d.NewCountPolicy(&minimum, &maximum)
	return d.BoundedCount(d.Observed[int64]{Value: int64(*value), Known: true}, policy).Known
}

func validList(values, allowed []string, limit int) bool {
	if values == nil || len(values) > limit {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if seen[value] || !slices.Contains(allowed, value) {
			return false
		}
		seen[value] = true
	}
	return true
}
