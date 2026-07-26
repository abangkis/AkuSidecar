package domain

import "testing"

func TestSourceRegistryOwnsGenericProductAndBridgeContracts(t *testing.T) {
	descriptors := Sources()
	if len(descriptors) != 3 {
		t.Fatalf("source count=%d want=3", len(descriptors))
	}
	want := []Source{SourceX, SourceLinkedIn, SourceFacebook}
	for index, descriptor := range descriptors {
		if descriptor.ID != want[index] || !descriptor.ID.Valid() {
			t.Fatalf("source[%d]=%+v", index, descriptor)
		}
		if descriptor.DisplayName == "" || descriptor.AdapterVersion == "" || descriptor.IconText == "" {
			t.Fatalf("source descriptor is incomplete: %+v", descriptor)
		}
		if len(descriptor.NativeHosts) == 0 || len(descriptor.NativePathTokens) == 0 || len(descriptor.EngagementMetrics) == 0 {
			t.Fatalf("source presentation policy is incomplete: %+v", descriptor)
		}
		if descriptor.HydrationTimeoutDefaultMS <= 0 || descriptor.HydrationTimeoutMinMS != descriptor.HydrationTimeoutDefaultMS-5000 || descriptor.HydrationTimeoutMaxMS != descriptor.HydrationTimeoutDefaultMS+5000 {
			t.Fatalf("source hydration policy is incomplete: %+v", descriptor)
		}
	}
	defaults := DefaultSources()
	if len(defaults) != 3 || defaults[0] != SourceX || defaults[1] != SourceLinkedIn || defaults[2] != SourceFacebook {
		t.Fatalf("default sources=%v", defaults)
	}
	if descriptor, ok := SourceByID(SourceFacebook); !ok || !descriptor.DefaultActive {
		t.Fatalf("Facebook must be available and preselected: %+v ok=%v", descriptor, ok)
	}
	if descriptor, _ := SourceByID(SourceX); descriptor.PassiveMediaCapability != "x_response" || descriptor.MediaEvidenceAdapterVersion != "x-response-evidence-v2" {
		t.Fatalf("X media capability drifted: %+v", descriptor)
	}
	if descriptor, _ := SourceByID(SourceFacebook); descriptor.FollowUpPlanningPolicy != "local_frontier" {
		t.Fatalf("Facebook follow-up planning policy drifted: %+v", descriptor)
	}
	if descriptor, _ := SourceByID(SourceLinkedIn); descriptor.FollowUpPlanningPolicy != "local_frontier" || !descriptor.ContinuationOverlapRequired || !descriptor.FrontierRequiresComplete {
		t.Fatalf("LinkedIn guarded frontier or overlap policy drifted: %+v", descriptor)
	}
	if descriptor, _ := SourceByID(SourceFacebook); descriptor.FrontierRequiresComplete {
		t.Fatalf("Facebook must retain its established frontier behavior: %+v", descriptor)
	}
	if descriptor, _ := SourceByID(SourceX); descriptor.FollowUpPlanningPolicy != "" {
		t.Fatalf("X must retain model acquisition planning: %+v", descriptor)
	}
}

func TestDefaultSourceHydrationTimeoutsFollowRegistry(t *testing.T) {
	defaults := DefaultSourceHydrationTimeouts()
	if defaults[SourceX] != 12000 || defaults[SourceLinkedIn] != 18000 || defaults[SourceFacebook] != 25000 {
		t.Fatalf("hydration defaults=%v", defaults)
	}
}

func TestCanonicalSourceURLSupportsEveryRegisteredSource(t *testing.T) {
	tests := []struct {
		source Source
		url    string
	}{
		{SourceX, "https://x.com/example/status/12345"},
		{SourceLinkedIn, "https://www.linkedin.com/feed/update/urn:li:activity:12345"},
		{SourceFacebook, "https://www.facebook.com/example/posts/12345"},
		{SourceFacebook, "https://www.facebook.com/story.php?story_fbid=12345&id=1"},
		{SourceFacebook, "https://www.facebook.com/reel/12345/"},
	}
	for _, test := range tests {
		if got, ok := CanonicalSourceURL(test.source, test.url); !ok || got != test.url {
			t.Fatalf("CanonicalSourceURL(%s,%q)=%q,%v", test.source, test.url, got, ok)
		}
	}
	for _, raw := range []string{
		"https://attacker.example/example/posts/12345",
		"https://www.facebook.com/story.php?id=1",
		"https://www.facebook.com/reel/not-a-native-id/",
		"http://x.com/example/status/12345",
	} {
		if got, ok := CanonicalSourceURL(SourceFacebook, raw); ok || got != "" {
			t.Fatalf("untrusted Facebook URL admitted: %q", raw)
		}
	}
}

func TestLinkedInPermalinkProvesTheSameNativeIdentityAsPlatformID(t *testing.T) {
	for _, test := range []struct {
		url  string
		want string
	}{
		{
			url:  "https://www.linkedin.com/feed/update/urn:li:activity:7411111111111111111/",
			want: "linkedin:activity:7411111111111111111",
		},
		{
			url:  "https://www.linkedin.com/posts/example_activity-7422222222222222222",
			want: "linkedin:activity:7422222222222222222",
		},
	} {
		if got := NativeIdentityFromPermalink(SourceLinkedIn, test.url); got != test.want {
			t.Fatalf("NativeIdentityFromPermalink(%q)=%q want=%q", test.url, got, test.want)
		}
	}
	if got := NativeIdentityFromPermalink(SourceX, "https://x.com/example/status/123"); got != "" {
		t.Fatalf("X must not infer an unsupported native identity: %q", got)
	}
	if got := NormalizeNativeIdentity(SourceLinkedIn, "urn:li:activity:7411111111111111111"); got != "linkedin:activity:7411111111111111111" {
		t.Fatalf("LinkedIn platform identity was not normalized: %q", got)
	}
}
