package engine

import (
	"maps"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestMergeReasoningProfilesByProviderPrecedence(t *testing.T) {
	a, b, c := profileMergeSet("stored"), profileMergeSet("submitted"), profileMergeSet("active")
	partial := domain.ReasoningProfileSet{Acquisition: "partial"}
	settings := func(provider string, active domain.ReasoningProfileSet, profiles map[string]domain.ReasoningProfileSet) domain.Settings {
		value := domain.Settings{ReasoningProvider: provider, ReasoningProfilesByProvider: profiles}
		value.ApplyReasoningProfileSet(active)
		return value
	}
	for _, tc := range []struct {
		name           string
		current, value domain.Settings
		want           map[string]domain.ReasoningProfileSet
	}{
		{
			name:    "disjoint_entries",
			current: settings("", a, map[string]domain.ReasoningProfileSet{"stored_only": a}),
			value:   settings("", b, map[string]domain.ReasoningProfileSet{"submitted_only": b}),
			want:    map[string]domain.ReasoningProfileSet{"stored_only": a, "submitted_only": b},
		},
		{
			name:    "inactive_entry_replaced_whole",
			current: settings("active", c, map[string]domain.ReasoningProfileSet{"inactive": a}),
			value:   settings("target", b, map[string]domain.ReasoningProfileSet{"inactive": partial}),
			want:    map[string]domain.ReasoningProfileSet{"active": c, "inactive": partial},
		},
		{
			name:    "switch_preserves_old_active_and_target_snapshot",
			current: settings("old", c, map[string]domain.ReasoningProfileSet{"old": a, "target": a}),
			value:   settings("target", a, map[string]domain.ReasoningProfileSet{"old": b, "target": b}),
			want:    map[string]domain.ReasoningProfileSet{"old": c, "target": b},
		},
		{
			name:    "same_provider_uses_submitted_active_projection",
			current: settings("active", a, map[string]domain.ReasoningProfileSet{"active": a, "other": a}),
			value:   settings("active", c, map[string]domain.ReasoningProfileSet{"active": b}),
			want:    map[string]domain.ReasoningProfileSet{"active": c, "other": a},
		},
		{
			name: "nil_maps_still_return_non_nil_map",
			want: map[string]domain.ReasoningProfileSet{},
		},
		{
			name:    "empty_provider_does_not_invent_empty_key",
			current: settings("", c, map[string]domain.ReasoningProfileSet{}),
			value:   settings("", b, map[string]domain.ReasoningProfileSet{"inactive": partial}),
			want:    map[string]domain.ReasoningProfileSet{"inactive": partial},
		},
		{
			name:  "raw_keys_and_values_remain_for_validation",
			value: settings("", b, map[string]domain.ReasoningProfileSet{" odd ": {Acquisition: " Raw "}, "": partial}),
			want:  map[string]domain.ReasoningProfileSet{" odd ": {Acquisition: " Raw "}, "": partial},
		},
		{
			name:    "helper_does_not_normalize_active_projection",
			current: settings("active", partial, nil),
			want:    map[string]domain.ReasoningProfileSet{"active": partial},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			currentBefore, valueBefore := maps.Clone(tc.current.ReasoningProfilesByProvider), maps.Clone(tc.value.ReasoningProfilesByProvider)
			got := mergeReasoningProfilesByProvider(tc.current, tc.value)
			if got == nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("merged profiles=%+v want=%+v", got, tc.want)
			}
			if !reflect.DeepEqual(tc.current.ReasoningProfilesByProvider, currentBefore) || !reflect.DeepEqual(tc.value.ReasoningProfilesByProvider, valueBefore) {
				t.Fatal("merge mutated an input profile map")
			}
		})
	}
}

func TestMergeReasoningProfilesByProviderDoesNotAliasInputs(t *testing.T) {
	a, b := profileMergeSet("stored"), profileMergeSet("submitted")
	current := domain.Settings{ReasoningProfilesByProvider: map[string]domain.ReasoningProfileSet{"stored": a}}
	value := domain.Settings{ReasoningProfilesByProvider: map[string]domain.ReasoningProfileSet{"submitted": b}}
	first := mergeReasoningProfilesByProvider(current, value)
	second := mergeReasoningProfilesByProvider(current, value)
	first["stored"] = b
	first["submitted"] = a
	first["added"] = a
	if !reflect.DeepEqual(current.ReasoningProfilesByProvider, map[string]domain.ReasoningProfileSet{"stored": a}) || !reflect.DeepEqual(value.ReasoningProfilesByProvider, map[string]domain.ReasoningProfileSet{"submitted": b}) {
		t.Fatal("mutating merge result changed an input map")
	}
	current.ReasoningProfilesByProvider["stored"] = b
	value.ReasoningProfilesByProvider["submitted"] = a
	if !reflect.DeepEqual(second, map[string]domain.ReasoningProfileSet{"stored": a, "submitted": b}) {
		t.Fatal("merge result aliases an input map or another result")
	}
}
