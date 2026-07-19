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
	}
	for _, test := range tests {
		if got, ok := CanonicalSourceURL(test.source, test.url); !ok || got != test.url {
			t.Fatalf("CanonicalSourceURL(%s,%q)=%q,%v", test.source, test.url, got, ok)
		}
	}
	for _, raw := range []string{
		"https://attacker.example/example/posts/12345",
		"https://www.facebook.com/story.php?id=1",
		"http://x.com/example/status/12345",
	} {
		if got, ok := CanonicalSourceURL(SourceFacebook, raw); ok || got != "" {
			t.Fatalf("untrusted Facebook URL admitted: %q", raw)
		}
	}
}
