package engine

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func profileMergeSet(prefix string) domain.ReasoningProfileSet {
	return domain.ReasoningProfileSet{Acquisition: prefix + "_acq", Evaluation: prefix + "_eval", Semantic: prefix + "_semantic", AIDeep: prefix + "_deep"}
}

func TestSaveSettingsReasoningProfileMergeSameProvider(t *testing.T) {
	for _, emptyActive := range []bool{false, true} {
		t.Run(fmt.Sprintf("empty_active_%t", emptyActive), func(t *testing.T) {
			runtime, state := swapTestEngine(t)
			ctx := context.Background()
			current, err := state.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			current.ApplyReasoningProfileSet(profileMergeSet("current"))
			current.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
				"deterministic": profileMergeSet("stale_active"), "shared": profileMergeSet("stored_shared"), "retained": profileMergeSet("stored_only"),
			}
			if err := state.SaveSettings(ctx, current); err != nil {
				t.Fatal(err)
			}
			value := current
			active := profileMergeSet("submitted_active")
			if emptyActive {
				value.ApplyReasoningProfileSet(domain.ReasoningProfileSet{})
				active = domain.ReasoningProfileSet{Acquisition: domain.DefaultReasoningAcquisition, Evaluation: domain.DefaultReasoningEvaluation, Semantic: domain.DefaultReasoningSemantic, AIDeep: domain.DefaultReasoningAIDeep}
			} else {
				value.ApplyReasoningProfileSet(active)
			}
			partial := domain.ReasoningProfileSet{Acquisition: "submitted_acq"}
			value.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
				"deterministic": profileMergeSet("submitted_map"), "shared": partial, "added": profileMergeSet("submitted_only"),
			}
			inputMap := maps.Clone(value.ReasoningProfilesByProvider)
			activeProvider := runtime.provider
			saved, err := runtime.SaveSettings(ctx, value)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]domain.ReasoningProfileSet{
				"deterministic": active, "shared": partial, "retained": profileMergeSet("stored_only"), "added": profileMergeSet("submitted_only"),
			}
			if saved.ActiveReasoningProfileSet() != active || !reflect.DeepEqual(saved.ReasoningProfilesByProvider, want) {
				t.Fatalf("profile merge precedence/defaults changed: active=%+v map=%+v want=%+v", saved.ActiveReasoningProfileSet(), saved.ReasoningProfilesByProvider, want)
			}
			persisted, err := state.GetSettings(ctx)
			if err != nil || !reflect.DeepEqual(persisted, saved) {
				t.Fatalf("saved projection did not persist: settings=%+v err=%v", persisted, err)
			}
			if runtime.provider != activeProvider || !reflect.DeepEqual(value.ReasoningProfilesByProvider, inputMap) {
				t.Fatal("same-provider save replaced the runtime or mutated the submitted profile map")
			}
		})
	}
}

func TestSaveSettingsReasoningProfileMergeProviderSwitch(t *testing.T) {
	runtime, state := swapTestEngine(t)
	ctx := context.Background()
	cfg := runtime.config
	if err := cfg.Reasoning.Select("gemini-flash-lite"); err != nil {
		t.Fatal(err)
	}
	probe, err := reasoning.NewProvider(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = probe.(interface{ Close() error }).Close() }()
	options := probe.(reasoning.ProfileProvider).ProfileOptions()
	if len(options) < 2 {
		t.Fatal("switch fixture requires two valid profiles to distinguish restoration from fallback")
	}
	set := func(id string) domain.ReasoningProfileSet {
		return domain.ReasoningProfileSet{Acquisition: id, Evaluation: id, Semantic: id, AIDeep: id}
	}
	storedTarget, submittedTarget := set(options[0].ID), set(options[len(options)-1].ID)
	current, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	oldActive := profileMergeSet("old_active")
	current.ApplyReasoningProfileSet(oldActive)
	current.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
		"deterministic": profileMergeSet("stale_old"), "gemini-flash-lite": storedTarget, "retained": profileMergeSet("stored_only"),
	}
	if err := state.SaveSettings(ctx, current); err != nil {
		t.Fatal(err)
	}
	value := current
	value.ReasoningProvider = "gemini-flash-lite"
	value.ApplyReasoningProfileSet(storedTarget)
	value.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
		"deterministic": profileMergeSet("overwrite_attempt"), "gemini-flash-lite": submittedTarget, "added": profileMergeSet("submitted_only"),
	}
	inputMap := maps.Clone(value.ReasoningProfilesByProvider)
	saved, err := runtime.SaveSettings(ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.provider.(interface{ Close() error }).Close() })
	want := map[string]domain.ReasoningProfileSet{
		"deterministic": oldActive, "gemini-flash-lite": submittedTarget, "retained": profileMergeSet("stored_only"), "added": profileMergeSet("submitted_only"),
	}
	if runtime.ProviderName() != "gemini-flash-lite" || saved.ActiveReasoningProfileSet() != submittedTarget || !reflect.DeepEqual(saved.ReasoningProfilesByProvider, want) {
		t.Fatalf("provider switch changed profile ownership: active=%+v map=%+v want=%+v", saved.ActiveReasoningProfileSet(), saved.ReasoningProfilesByProvider, want)
	}
	persisted, err := state.GetSettings(ctx)
	if err != nil || !reflect.DeepEqual(persisted, saved) || !reflect.DeepEqual(value.ReasoningProfilesByProvider, inputMap) {
		t.Fatalf("provider switch changed persistence/input ownership: settings=%+v err=%v", persisted, err)
	}
}

func TestSaveSettingsReasoningProfileMergeValidation(t *testing.T) {
	for _, tc := range []struct {
		name, wantError string
		entries         int
	}{
		{name: "invalid_provider", wantError: `reasoningProfilesByProvider contains an invalid provider id "invalid provider"`},
		{name: "invalid_profile", wantError: `reasoningProfilesByProvider["inactive"].acquisition contains an invalid profile id`},
		{name: "max_providers", entries: 31},
		{name: "too_many_providers", entries: 32, wantError: "reasoningProfilesByProvider contains too many providers"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, state := swapTestEngine(t)
			ctx := context.Background()
			before, err := state.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			activeProvider := runtime.provider
			value := before
			value.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{}
			switch tc.name {
			case "invalid_provider":
				value.ReasoningProfilesByProvider["invalid provider"] = profileMergeSet("valid")
			case "invalid_profile":
				value.ReasoningProfilesByProvider["inactive"] = domain.ReasoningProfileSet{Acquisition: "invalid profile"}
			default:
				for i := 0; i < tc.entries; i++ {
					value.ReasoningProfilesByProvider[fmt.Sprintf("provider_%02d", i)] = profileMergeSet("valid")
				}
			}
			saved, err := runtime.SaveSettings(ctx, value)
			if tc.wantError == "" {
				if err != nil || len(saved.ReasoningProfilesByProvider) != 32 {
					t.Fatalf("32-provider boundary rejected: settings=%+v err=%v", saved, err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantError {
				t.Fatalf("profile validation error changed: got=%v want=%q", err, tc.wantError)
			}
			after, readErr := state.GetSettings(ctx)
			if readErr != nil || !reflect.DeepEqual(before, after) || runtime.provider != activeProvider {
				t.Fatalf("rejected profile map changed durable/runtime state: settings=%+v err=%v", after, readErr)
			}
		})
	}
}
