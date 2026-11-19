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
const permalinkFailureKey = "linkedinPermalinkRecoveryFailures"
const anchorMismatchKey = "linkedinAnchorMismatchPolls"
const anchorGeometryKey = "linkedinAnchorGeometrySamples"

type permalinkFailure struct {
	SnapshotIndex  *int   `json:"snapshotIndex"`
	CandidateIndex *int   `json:"candidateIndex"`
	Stage          string `json:"stage"`
	Reason         string `json:"reason"`
	Viewport       string `json:"viewport"`
	ScrollRoot     string `json:"scrollRoot"`
}

type anchorMismatch struct {
	SnapshotIndex        *int   `json:"snapshotIndex"`
	CandidateIndex       *int   `json:"candidateIndex"`
	MismatchPolls        *int   `json:"mismatchPolls"`
	HorizontalPolls      *int   `json:"horizontalPolls"`
	VerticalPolls        *int   `json:"verticalPolls"`
	Persistence          string `json:"persistence"`
	VerticalGapPolls     *int   `json:"verticalGapPolls,omitempty"`
	VerticalOverlapPolls *int   `json:"verticalOverlapPolls,omitempty"`
}

type anchorGeometryPoint struct {
	ScrollTop     *float64 `json:"scrollTop"`
	GapY          *float64 `json:"gapY"`
	CorrectedGapY *float64 `json:"correctedGapY,omitempty"`
}

type anchorGeometry struct {
	SnapshotIndex    *int                 `json:"snapshotIndex"`
	CandidateIndex   *int                 `json:"candidateIndex"`
	ScrollTopAtClick *float64             `json:"scrollTopAtClick"`
	First            *anchorGeometryPoint `json:"first"`
	Last             *anchorGeometryPoint `json:"last"`
}

var permalinkFailureReasons = map[string][]string{
	"candidate":     {"snapshot_budget", "owner_missing", "owner_changed"},
	"menu":          {"menu_not_observed", "multiple_menus"},
	"ownership":     {"foreign_reference", "control_already_expanded", "control_not_uniquely_expanded", "popover_not_anchored", "missing_geometry", "anchor_mismatch", "menu_replaced"},
	"menu_evidence": {"stale_urn", "conflicting_urns", "multiple_embed_actions", "no_visible_urn_or_embed_action"},
	"embed_dialog":  {"dialog_not_started", "dialog_not_observed", "multiple_dialogs", "foreign_reference", "dialog_replaced", "stale_urn", "conflicting_urns", "no_urn"},
	"revalidation":  {"binding_or_evidence_changed"},
}

var buttonVisibility = []string{"unavailable", "hidden", "outside", "partial", "visible"}

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

type priorRecoveryMenu struct {
	Slot                  *int   `json:"slot"`
	AttemptSnapshotIndex  *int   `json:"attemptSnapshotIndex"`
	AttemptCandidateIndex *int   `json:"attemptCandidateIndex"`
	SameButton            *bool  `json:"sameButton"`
	MenuBound             *bool  `json:"menuBound"`
	TriggerExpanded       *bool  `json:"triggerExpanded"`
	Cleanup               string `json:"cleanup"`
}

var components = []string{"messaging", "feed_sort", "post", "navigation", "header", "sidebar", "footer", "main"}
var actions = []string{"post_link", "save_post", "report_post", "account", "message", "feed_order", "navigation", "dismiss"}

// ProjectCoverage sanitizes the optional extension while preserving old
// reports with no extension. A diagnostic failure leaves captured posts intact.
func ProjectCoverage(source domain.Source, coverage map[string]any) map[string]any {
	value, exists := coverage[CoverageKey]
	failures, hasFailures := coverage[permalinkFailureKey]
	anchorPolls, hasAnchorPolls := coverage[anchorMismatchKey]
	geometry, hasGeometry := coverage[anchorGeometryKey]
	if !exists && !hasFailures && !hasAnchorPolls && !hasGeometry {
		return coverage
	}
	result := maps.Clone(coverage)
	if source != domain.SourceLinkedIn {
		delete(result, CoverageKey)
		delete(result, permalinkFailureKey)
		delete(result, anchorMismatchKey)
		delete(result, anchorGeometryKey)
		return result
	}
	if hasGeometry {
		if validated := projectAnchorGeometry(geometry); validated != nil {
			result[anchorGeometryKey] = validated
		} else {
			delete(result, anchorGeometryKey)
		}
	}
	if hasAnchorPolls {
		if validated := projectAnchorMismatchPolls(anchorPolls); validated != nil {
			result[anchorMismatchKey] = validated
		} else {
			delete(result, anchorMismatchKey)
		}
	}
	if hasFailures {
		if validated := projectPermalinkFailures(failures); validated != nil {
			result[permalinkFailureKey] = validated
		} else {
			delete(result, permalinkFailureKey)
		}
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
		rawInspection, hasInspection := entry["surfaceInspection"]
		rawPriorMenus, hasPriorMenus := entry["priorRecoveryMenus"]
		if !hasInspection && !hasPriorMenus {
			continue
		}
		entry = maps.Clone(entry)
		delete(entry, "surfaceInspection")
		delete(entry, "priorRecoveryMenus")
		if entry["code"] == "existing_surface" && entry["phase"] == "pre_recovery_guard" {
			if validated := projectInspection(rawInspection); hasInspection && validated != nil {
				entry["surfaceInspection"] = validated
			}
			if validated := projectPriorRecoveryMenus(rawPriorMenus, entry); hasPriorMenus && validated != nil {
				entry["priorRecoveryMenus"] = validated
			}
		}
		projected[index] = entry
	}
	result[CoverageKey] = projected
	return result
}

func projectAnchorGeometry(value any) []anchorGeometry {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 4096 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result []anchorGeometry
	if decoder.Decode(&result) != nil || len(result) == 0 || len(result) > 8 {
		return nil
	}
	validScroll := func(value *float64) bool {
		return value != nil && *value >= 0 && *value <= 10_000_000
	}
	validPoint := func(point *anchorGeometryPoint) bool {
		return point == nil || validScroll(point.ScrollTop) && point.GapY != nil && *point.GapY >= -16_384 && *point.GapY <= 16_384 &&
			(point.CorrectedGapY == nil || *point.CorrectedGapY >= -16_384 && *point.CorrectedGapY <= 16_384)
	}
	for _, entry := range result {
		if (entry.SnapshotIndex != nil && !countWithin(entry.SnapshotIndex, 1_000_000)) ||
			!countWithin(entry.CandidateIndex, 1_000_000) || !validScroll(entry.ScrollTopAtClick) ||
			(entry.First == nil) != (entry.Last == nil) || !validPoint(entry.First) || !validPoint(entry.Last) {
			return nil
		}
	}
	return result
}

func projectAnchorMismatchPolls(value any) []anchorMismatch {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 2048 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result []anchorMismatch
	if decoder.Decode(&result) != nil || len(result) == 0 || len(result) > 8 {
		return nil
	}
	for _, entry := range result {
		if (entry.SnapshotIndex != nil && !countWithin(entry.SnapshotIndex, 1_000_000)) ||
			!countWithin(entry.CandidateIndex, 1_000_000) || !countWithin(entry.MismatchPolls, 32) ||
			*entry.MismatchPolls == 0 || !countWithin(entry.HorizontalPolls, int64(*entry.MismatchPolls)) ||
			!countWithin(entry.VerticalPolls, int64(*entry.MismatchPolls)) ||
			*entry.HorizontalPolls+*entry.VerticalPolls < *entry.MismatchPolls ||
			!slices.Contains([]string{"single_sample", "repeated", "resolved", "interrupted"}, entry.Persistence) ||
			(entry.Persistence == "single_sample" && *entry.MismatchPolls != 1) ||
			(entry.Persistence == "repeated" && *entry.MismatchPolls < 2) {
			return nil
		}
		if entry.VerticalGapPolls != nil || entry.VerticalOverlapPolls != nil {
			if !countWithin(entry.VerticalGapPolls, int64(*entry.VerticalPolls)) ||
				!countWithin(entry.VerticalOverlapPolls, int64(*entry.VerticalPolls)) ||
				*entry.VerticalGapPolls+*entry.VerticalOverlapPolls != *entry.VerticalPolls {
				return nil
			}
		}
	}
	return result
}

func projectPermalinkFailures(value any) []permalinkFailure {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 2048 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result []permalinkFailure
	if decoder.Decode(&result) != nil || len(result) == 0 || len(result) > 8 {
		return nil
	}
	for _, failure := range result {
		if (failure.SnapshotIndex != nil && !countWithin(failure.SnapshotIndex, 1_000_000)) ||
			!countWithin(failure.CandidateIndex, 1_000_000) || !slices.Contains(permalinkFailureReasons[failure.Stage], failure.Reason) ||
			!slices.Contains(buttonVisibility, failure.Viewport) || !slices.Contains(buttonVisibility, failure.ScrollRoot) {
			return nil
		}
	}
	return result
}

func projectPriorRecoveryMenus(value any, blocker map[string]any) []priorRecoveryMenu {
	raw, err := json.Marshal(value)
	if err != nil || len(raw) > 1024 {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result []priorRecoveryMenu
	if decoder.Decode(&result) != nil || len(result) == 0 || len(result) > 2 {
		return nil
	}
	context, err := json.Marshal(map[string]any{"menuCount": blocker["menuCount"], "snapshotIndex": blocker["snapshotIndex"]})
	var bounds struct {
		MenuCount     int  `json:"menuCount"`
		SnapshotIndex *int `json:"snapshotIndex"`
	}
	if err != nil || json.Unmarshal(context, &bounds) != nil || bounds.MenuCount < 1 || bounds.MenuCount > 32 {
		return nil
	}
	previousSlot := -1
	for _, menu := range result {
		if !countWithin(menu.Slot, int64(min(1, bounds.MenuCount-1))) || *menu.Slot <= previousSlot ||
			(menu.AttemptSnapshotIndex != nil && (!countWithin(menu.AttemptSnapshotIndex, 1_000_000) ||
				bounds.SnapshotIndex != nil && *menu.AttemptSnapshotIndex > *bounds.SnapshotIndex)) ||
			(menu.AttemptCandidateIndex != nil && !countWithin(menu.AttemptCandidateIndex, 1_000_000)) ||
			menu.SameButton == nil || menu.MenuBound == nil || menu.TriggerExpanded == nil || !*menu.MenuBound && !*menu.TriggerExpanded ||
			!slices.Contains([]string{"not_attempted", "still_visible"}, menu.Cleanup) {
			return nil
		}
		previousSlot = *menu.Slot
	}
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
