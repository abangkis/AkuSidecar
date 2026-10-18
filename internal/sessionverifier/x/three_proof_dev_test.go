//go:build auth_session_dev && auth_session_fixture

package x

import (
	"reflect"
	"testing"
	"time"
)

const threeProofAccountID = "1234567890123456789"
const threeProofOtherAccountID = "2234567890123456789"

func newThreeProofVerifier(now *time.Time, namespace string) *Verifier {
	return &Verifier{
		pin: Pin{
			ProfileID: "fixture-profile", OwnerID: "fixture-owner", OwnerEpoch: "fixture-epoch",
			OwnerGeneration: 7, AccountID: threeProofAccountID, LineageGeneration: 3,
		},
		now:       func() time.Time { return *now },
		namespace: namespace,
		pending:   make(map[string]Request),
	}
}

func makeThreeProofInput(t *testing.T, verifier *Verifier, observedAt time.Time, initialBody []byte, initialStatus int, recheckBody []byte, recheckStatus int) ThreeProofInput {
	t.Helper()
	initialRequest, err := verifier.Issue(PhaseInitial)
	if err != nil {
		t.Fatal(err)
	}
	recheckRequest, err := verifier.Issue(PhaseRecheck)
	if err != nil {
		t.Fatal(err)
	}
	initial := ThreeProofEvidence{
		Attempt:  Attempt{Request: initialRequest, Envelope: fixtureEnvelope(initialRequest, observedAt, initialStatus), Body: initialBody},
		Metadata: sessionEvidenceMetadata(initialRequest, observedAt, "session-initial", "browser-self-authority"),
	}
	recheck := ThreeProofEvidence{
		Attempt:  Attempt{Request: recheckRequest, Envelope: fixtureEnvelope(recheckRequest, observedAt, recheckStatus), Body: recheckBody},
		Metadata: sessionEvidenceMetadata(recheckRequest, observedAt, "session-recheck", "browser-self-authority"),
	}
	lifecycle := &LifecycleEvidence{
		Metadata: EvidenceMetadata{
			Provenance: FixtureEvidenceProvenance, Schema: ThreeProofLifecycleSchema,
			Kind: EvidenceKindLifecycle, EvidenceID: "lifecycle-coverage", Authority: "lifecycle-recorder",
			Request: recheckRequest, ObservedAt: observedAt, Freshness: EvidenceFresh,
			Cache: EvidenceCacheNotApplicable,
		},
		BaselineEvidenceID: initial.Metadata.EvidenceID, BaselineAcceptedAt: observedAt,
		CoveredFrom: observedAt, CoveredThrough: observedAt,
		BaselineGeneration: verifier.pin.LineageGeneration, CurrentGeneration: verifier.pin.LineageGeneration,
		CoverageComplete: true,
	}
	return ThreeProofInput{
		Initial: initial, Recheck: recheck, Lifecycle: lifecycle,
		Policy: EnrollmentPolicy{ExpectedAccountID: verifier.pin.AccountID, Authority: "enrollment-authority"},
	}
}

func sessionEvidenceMetadata(request Request, observedAt time.Time, evidenceID, authority string) EvidenceMetadata {
	return EvidenceMetadata{
		Provenance: FixtureEvidenceProvenance, Schema: ThreeProofEvidenceSchema,
		Kind: EvidenceKindSessionSelf, EvidenceID: evidenceID, Authority: authority,
		Request: request, ObservedAt: observedAt, Freshness: EvidenceFresh,
		Cache: EvidenceCacheBypass,
	}
}

func TestThreeProofStableSelfIdentityAndFixtureSuccessStayNonAuthorizing(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "stable")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "old_handle", "Old Name"), 200,
		authenticatedFixtureBody(threeProofAccountID, "new_handle", "New Name"), 200)
	result := verifier.VerifyThreeProof(input)

	if result.Identity.State != IdentityMatch || result.Identity.AccountID != threeProofAccountID {
		t.Fatalf("stable exact self ID was not accepted: %+v", result.Identity)
	}
	if !result.Authentication.Stable || result.Authentication.State != Authenticated ||
		!result.Authentication.InitialProven || !result.Authentication.RecheckProven {
		t.Fatalf("explicit authentication state was not preserved: %+v", result.Authentication)
	}
	if result.Continuity.State != ContinuitySame || !result.Continuity.CoverageComplete {
		t.Fatalf("complete fixture lifecycle did not preserve continuity: %+v", result.Continuity)
	}
	if result.Identity.InitialSource.EvidenceID != result.Authentication.InitialSource.EvidenceID ||
		result.Identity.InitialSource.Authority != result.Authentication.InitialSource.Authority {
		t.Fatal("the same self response was mislabeled as two authorities for identity and authentication")
	}
	if result.DistinctEvidenceAuthorities != 2 || !reflect.DeepEqual(result.EvidenceAuthorityIDs, []string{"browser-self-authority", "lifecycle-recorder"}) {
		t.Fatalf("expected deduplicated session and lifecycle authorities, got %+v", result.EvidenceAuthorityIDs)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofMissingIdentityDoesNotEraseExplicitAuthentication(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "missing-id")
	input := makeThreeProofInput(t, verifier, testStart, fixtureBody("authenticated"), 200, fixtureBody("authenticated"), 200)
	result := verifier.VerifyThreeProof(input)
	if result.Identity.State != IdentityUnavailable || result.Identity.AccountID != "" {
		t.Fatalf("missing ID should leave identity unavailable: %+v", result.Identity)
	}
	if !result.Authentication.Stable || result.Authentication.State != Authenticated ||
		!result.Authentication.InitialProven || !result.Authentication.RecheckProven {
		t.Fatalf("missing ID erased explicit authenticated state: %+v", result.Authentication)
	}
	if result.Continuity.State != ContinuityUnavailable {
		t.Fatalf("continuity without an accepted identity baseline should be unavailable: %+v", result.Continuity)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofAccountMismatchDoesNotInventLogout(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "account-mismatch")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofOtherAccountID, "other", "Other"), 200,
		authenticatedFixtureBody(threeProofOtherAccountID, "other", "Other"), 200)
	result := verifier.VerifyThreeProof(input)
	if result.Identity.State != IdentityMismatch || result.Identity.AccountID != "" {
		t.Fatalf("account mismatch was not kept as an identity result: %+v", result.Identity)
	}
	if result.Authentication.State != Authenticated || !result.Authentication.Stable {
		t.Fatalf("account mismatch was incorrectly converted to logout: %+v", result.Authentication)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofLifecycleGenerationDetectsLogoutAndReloginForSameID(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "logout-relogin")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	// This synthetic counter represents a covered logout followed by login to
	// the same account between the accepted baseline and the recheck.
	input.Lifecycle.CurrentGeneration++
	result := verifier.VerifyThreeProof(input)
	if result.Identity.State != IdentityMatch || result.Authentication.State != Authenticated ||
		result.Continuity.State != ContinuityChanged {
		t.Fatalf("same-account logout/login lifecycle was not detected: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofCookieHintsDoNotEstablishContinuity(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "cookie-hint")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	input.Initial.Metadata.Hints = EvidenceHints{CookiePresent: true, CookieRotationHint: true}
	input.Recheck.Metadata.Hints = EvidenceHints{CookiePresent: true, CookieRotationHint: true}
	result := verifier.VerifyThreeProof(input)
	if result.Continuity.State != ContinuitySame || result.Authentication.State != Authenticated {
		t.Fatalf("cookie hints overrode self and lifecycle evidence: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofCookiePresenceAloneIsNotAuthentication(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "cookie-alone")
	input := makeThreeProofInput(t, verifier, testStart, fixtureBody("unknown"), 200, fixtureBody("unknown"), 200)
	input.Initial.Metadata.Hints.CookiePresent = true
	input.Recheck.Metadata.Hints.CookiePresent = true
	result := verifier.VerifyThreeProof(input)
	if result.Authentication.State != Unknown || result.Identity.State != IdentityUnavailable {
		t.Fatalf("cookie presence became identity or authentication proof: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofCachedHTTPErrorDoesNotProveAuthentication(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "cached-error")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 500,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 500)
	input.Initial.Metadata.Cache = EvidenceCacheHit
	input.Recheck.Metadata.Cache = EvidenceCacheHit
	result := verifier.VerifyThreeProof(input)
	if result.Authentication.State != Unknown || result.Authentication.InitialProven || result.Authentication.RecheckProven {
		t.Fatalf("cached HTTP errors were treated as authentication: %+v", result.Authentication)
	}
	if result.Identity.State != IdentityUnavailable || result.Continuity.State != ContinuityUnavailable {
		t.Fatalf("failed cached responses retained identity or continuity: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofFreshCacheHitDoesNotProveAuthentication(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "fresh-cache-hit")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	input.Initial.Metadata.Cache = EvidenceCacheHit
	input.Recheck.Metadata.Cache = EvidenceCacheHit
	result := verifier.VerifyThreeProof(input)
	if result.Authentication.State != Unknown || result.Authentication.InitialProven || result.Authentication.RecheckProven {
		t.Fatalf("fresh HTTP 200 cache hits were treated as current authentication: %+v", result.Authentication)
	}
	if result.Identity.State != IdentityUnavailable || result.Continuity.State != ContinuityUnavailable {
		t.Fatalf("fresh cache hits retained identity or continuity: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofCookieOnlyWithoutLifecycleIsUnavailable(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "cookie-no-lifecycle")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	input.Initial.Metadata.Hints = EvidenceHints{CookiePresent: true, CookieRotationHint: true}
	input.Recheck.Metadata.Hints = EvidenceHints{CookiePresent: true, CookieRotationHint: true}
	input.Lifecycle = nil
	result := verifier.VerifyThreeProof(input)
	if result.Identity.State != IdentityMatch || result.Authentication.State != Authenticated ||
		result.Continuity.State != ContinuityUnavailable || result.Continuity.CoverageComplete {
		t.Fatalf("cookie-only evidence established continuity without lifecycle coverage: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofObservationGapsDenyIdenticalSnapshotContinuity(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "aba-gap")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	input.Lifecycle.CoverageComplete = false
	input.Lifecycle.Gap = LifecycleGapOverflow
	result := verifier.VerifyThreeProof(input)
	if result.Identity.State != IdentityMatch || result.Continuity.State != ContinuityUnavailable || result.Continuity.CoverageComplete {
		t.Fatalf("identical snapshots hid an unobserved lifecycle gap: %+v", result.Continuity)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofAllLifecycleGapsDenyCompleteCoverage(t *testing.T) {
	gaps := []struct {
		name string
		gap  LifecycleGap
		want Continuity
	}{
		{name: "startup unobserved", gap: LifecycleGapStartupUnobserved, want: ContinuityUnavailable},
		{name: "restart", gap: LifecycleGapRestart, want: ContinuityUnavailable},
		{name: "detach", gap: LifecycleGapDetach, want: ContinuityUnavailable},
		{name: "overflow", gap: LifecycleGapOverflow, want: ContinuityUnavailable},
		{name: "shared worker unsupported", gap: LifecycleGapUnsupportedSharedWorker, want: ContinuityUnsupported},
		{name: "service worker unsupported", gap: LifecycleGapUnsupportedServiceWorker, want: ContinuityUnsupported},
	}
	for i, test := range gaps {
		t.Run(test.name, func(t *testing.T) {
			now := testStart
			verifier := newThreeProofVerifier(&now, "lifecycle-gap-"+string(rune('a'+i)))
			input := makeThreeProofInput(t, verifier, testStart,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
			input.Lifecycle.CoverageComplete = true
			input.Lifecycle.Gap = test.gap
			result := verifier.VerifyThreeProof(input)
			if result.Continuity.State != test.want || result.Continuity.CoverageComplete {
				t.Fatalf("gap did not deny complete coverage: %+v", result.Continuity)
			}
			assertNonAuthorizingThreeProof(t, result)
		})
	}
}

func TestThreeProofCrossInstanceReplayIsRejected(t *testing.T) {
	now := testStart
	firstVerifier := newThreeProofVerifier(&now, "replay-source")
	input := makeThreeProofInput(t, firstVerifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	secondVerifier := newThreeProofVerifier(&now, "replay-target")
	result := secondVerifier.VerifyThreeProof(input)
	if result.Identity.State == IdentityMatch || result.Authentication.InitialProven || result.Authentication.RecheckProven ||
		result.Continuity.State != ContinuityUnavailable {
		t.Fatalf("cross-instance stale evidence was accepted: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofMetadataMismatchesPreserveOnlyValidPhase(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ThreeProofInput)
	}{
		{name: "provenance", mutate: func(input *ThreeProofInput) { input.Recheck.Metadata.Provenance = "native-source" }},
		{name: "kind", mutate: func(input *ThreeProofInput) { input.Recheck.Metadata.Kind = EvidenceKindLifecycle }},
		{name: "schema", mutate: func(input *ThreeProofInput) { input.Recheck.Metadata.Schema = "other-schema" }},
		{name: "source binding", mutate: func(input *ThreeProofInput) { input.Recheck.Metadata.Request.Source = "other-source" }},
		{name: "owner scope", mutate: func(input *ThreeProofInput) { input.Recheck.Metadata.Request.OwnerGeneration++ }},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := testStart
			verifier := newThreeProofVerifier(&now, "metadata-"+string(rune('a'+i)))
			input := makeThreeProofInput(t, verifier, testStart,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
			test.mutate(&input)
			result := verifier.VerifyThreeProof(input)
			if result.Authentication.Initial != Authenticated || !result.Authentication.InitialProven ||
				result.Authentication.Recheck != Unsupported || result.Authentication.RecheckProven {
				t.Fatalf("invalid recheck metadata erased or replaced the valid initial phase: %+v", result.Authentication)
			}
			if result.Identity.State != IdentityUnsupported || result.Continuity.State != ContinuityUnavailable {
				t.Fatalf("invalid metadata produced identity or continuity proof: %+v", result)
			}
			assertNonAuthorizingThreeProof(t, result)
		})
	}
}

func TestThreeProofPolicyExpectationCannotOverridePinnedIdentity(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "policy-mismatch")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofOtherAccountID, "other", "Other"), 200,
		authenticatedFixtureBody(threeProofOtherAccountID, "other", "Other"), 200)
	input.Policy.ExpectedAccountID = threeProofOtherAccountID
	result := verifier.VerifyThreeProof(input)
	if result.Identity.State != IdentityUnsupported || result.Identity.Reason != ReasonPolicyInvalid {
		t.Fatalf("inconsistent enrollment expectation overrode pin: %+v", result.Identity)
	}
	if result.Authentication.State != Authenticated || !result.Authentication.Stable {
		t.Fatalf("policy inconsistency erased independent auth state: %+v", result.Authentication)
	}
	if result.Continuity.State != ContinuityUnavailable {
		t.Fatalf("policy inconsistency produced continuity: %+v", result.Continuity)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofAuthorityEchoIsDeduplicatedAndEvidenceIDsMustBeDistinct(t *testing.T) {
	t.Run("enrollment authority echo", func(t *testing.T) {
		now := testStart
		verifier := newThreeProofVerifier(&now, "authority-echo")
		input := makeThreeProofInput(t, verifier, testStart,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
		input.Initial.Metadata.Authority = "enrollment-authority"
		input.Recheck.Metadata.Authority = "enrollment-authority"
		input.Lifecycle.Metadata.Authority = "enrollment-authority"
		result := verifier.VerifyThreeProof(input)
		if !result.Authentication.Stable || result.Continuity.State != ContinuitySame ||
			result.DistinctEvidenceAuthorities != 1 || !reflect.DeepEqual(result.EvidenceAuthorityIDs, []string{"enrollment-authority"}) {
			t.Fatalf("authority echo was counted as independent sources: %+v", result)
		}
		assertNonAuthorizingThreeProof(t, result)
	})

	t.Run("same artifact cannot be two phases", func(t *testing.T) {
		now := testStart
		verifier := newThreeProofVerifier(&now, "artifact-echo")
		input := makeThreeProofInput(t, verifier, testStart,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
		input.Recheck.Metadata.EvidenceID = input.Initial.Metadata.EvidenceID
		result := verifier.VerifyThreeProof(input)
		if result.Identity.State != IdentityUnsupported || result.Authentication.Recheck != Unsupported ||
			result.Authentication.Stable || result.Continuity.State != ContinuityUnavailable {
			t.Fatalf("one response artifact was accepted as two observations: %+v", result)
		}
		assertNonAuthorizingThreeProof(t, result)
	})
}

func TestThreeProofLifecycleEchoAndInvalidBindingAreNotCountedAsAuthorities(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ThreeProofInput)
		want   Reason
	}{
		{
			name: "lifecycle echoes request ID",
			mutate: func(input *ThreeProofInput) {
				input.Lifecycle.Metadata.EvidenceID = input.Recheck.Attempt.Request.RequestID
			},
			want: ReasonEvidenceReplay,
		},
		{
			name: "lifecycle owner binding mismatch",
			mutate: func(input *ThreeProofInput) {
				input.Lifecycle.Metadata.Request.OwnerGeneration++
			},
			want: ReasonBindingMismatch,
		},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := testStart
			verifier := newThreeProofVerifier(&now, "lifecycle-source-"+string(rune('a'+i)))
			input := makeThreeProofInput(t, verifier, testStart,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
			input.Lifecycle.Metadata.Authority = "rejected-lifecycle-authority"
			test.mutate(&input)
			result := verifier.VerifyThreeProof(input)
			if result.Continuity.State == ContinuitySame || result.Continuity.State == ContinuityChanged ||
				result.Continuity.CoverageComplete || result.Continuity.Reason != test.want {
				t.Fatalf("invalid lifecycle witness retained continuity: %+v", result.Continuity)
			}
			if result.DistinctEvidenceAuthorities != 1 ||
				!reflect.DeepEqual(result.EvidenceAuthorityIDs, []string{"browser-self-authority"}) {
				t.Fatalf("rejected lifecycle label counted as a corroborating authority: %+v", result.EvidenceAuthorityIDs)
			}
			assertNonAuthorizingThreeProof(t, result)
		})
	}
}

func TestThreeProofRecheckCannotPredateAcceptedInitialAndCoverageMustReachCompletion(t *testing.T) {
	t.Run("recheck predates initial", func(t *testing.T) {
		now := testStart
		verifier := newThreeProofVerifier(&now, "temporal-order")
		input := makeThreeProofInput(t, verifier, testStart,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
		initialAt := testStart.Add(2 * time.Second)
		recheckAt := testStart.Add(time.Second)
		now = testStart.Add(3 * time.Second)
		input.Initial.Attempt.Envelope.ObservedAt = initialAt
		input.Initial.Metadata.ObservedAt = initialAt
		input.Recheck.Attempt.Envelope.ObservedAt = recheckAt
		input.Recheck.Metadata.ObservedAt = recheckAt
		input.Lifecycle.BaselineAcceptedAt = initialAt
		input.Lifecycle.CoveredFrom = initialAt
		input.Lifecycle.CoveredThrough = now
		input.Lifecycle.Metadata.ObservedAt = now
		result := verifier.VerifyThreeProof(input)
		if result.Identity.State != IdentityUnavailable || result.Authentication.Stable ||
			result.Continuity.State != ContinuityUnavailable || result.Authentication.Reason != ReasonTemporalOrder {
			t.Fatalf("out-of-order phase evidence was accepted: %+v", result)
		}
		assertNonAuthorizingThreeProof(t, result)
	})

	t.Run("coverage stops before completion", func(t *testing.T) {
		now := testStart
		verifier := newThreeProofVerifier(&now, "coverage-end")
		input := makeThreeProofInput(t, verifier, testStart,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
		now = testStart.Add(time.Second)
		result := verifier.VerifyThreeProof(input)
		if result.Identity.State != IdentityMatch || result.Continuity.State != ContinuityUnavailable || result.Continuity.CoverageComplete {
			t.Fatalf("lifecycle interval that ended before completion was accepted: %+v", result.Continuity)
		}
		assertNonAuthorizingThreeProof(t, result)
	})

	t.Run("valid nonzero baseline to completion interval", func(t *testing.T) {
		now := testStart
		verifier := newThreeProofVerifier(&now, "coverage-nonzero")
		input := makeThreeProofInput(t, verifier, testStart,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
			authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
		initialAt := testStart
		recheckAt := testStart.Add(time.Second)
		completedAt := testStart.Add(2 * time.Second)
		input.Recheck.Attempt.Envelope.ObservedAt = recheckAt
		input.Recheck.Metadata.ObservedAt = recheckAt
		input.Lifecycle.BaselineAcceptedAt = initialAt
		input.Lifecycle.CoveredFrom = initialAt
		input.Lifecycle.CoveredThrough = completedAt
		input.Lifecycle.Metadata.ObservedAt = completedAt
		now = completedAt
		result := verifier.VerifyThreeProof(input)
		if result.Identity.State != IdentityMatch || result.Authentication.State != Authenticated ||
			result.Continuity.State != ContinuitySame || !result.Continuity.CoverageComplete ||
			!result.Continuity.CoveredFrom.Equal(initialAt) || !result.Continuity.CoveredThrough.Equal(completedAt) {
			t.Fatalf("valid nonzero lifecycle interval was rejected: %+v", result)
		}
		assertNonAuthorizingThreeProof(t, result)
	})
}

func TestThreeProofSameInstanceReplayIsRejected(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "same-instance-replay")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	first := verifier.VerifyThreeProof(input)
	if first.Identity.State != IdentityMatch || first.Authentication.State != Authenticated || first.Continuity.State != ContinuitySame {
		t.Fatalf("valid initial fixture unexpectedly failed: %+v", first)
	}
	second := verifier.VerifyThreeProof(input)
	if second.Identity.State == IdentityMatch || second.Authentication.InitialProven || second.Authentication.RecheckProven ||
		second.Continuity.State != ContinuityUnavailable {
		t.Fatalf("spent challenges allowed regular same-instance replay: %+v", second)
	}
	assertNonAuthorizingThreeProof(t, second)
}

func TestThreeProofUnknownAndChallengeAreNotLogoutEvidence(t *testing.T) {
	for i, test := range []struct {
		name   string
		body   []byte
		status int
	}{
		{name: "unknown", body: fixtureBody("unknown"), status: 200},
		{name: "challenge", body: fixtureBody("challenge"), status: 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := testStart
			verifier := newThreeProofVerifier(&now, "unknown-state-"+string(rune('a'+i)))
			input := makeThreeProofInput(t, verifier, testStart,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
				test.body, test.status)
			result := verifier.VerifyThreeProof(input)
			if result.Authentication.Initial != Authenticated || result.Authentication.Recheck != AuthState(test.name) ||
				result.Continuity.State != ContinuityUnavailable {
				t.Fatalf("unknown/challenge was treated as logout or session change: %+v", result)
			}
			assertNonAuthorizingThreeProof(t, result)
		})
	}
}

func TestThreeProofExplicitLogoutWithCoverageIsChanged(t *testing.T) {
	now := testStart
	verifier := newThreeProofVerifier(&now, "explicit-logout")
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		fixtureBody("logged_out"), 401)
	result := verifier.VerifyThreeProof(input)
	if result.Authentication.Initial != Authenticated || result.Authentication.Recheck != LoggedOut ||
		result.Continuity.State != ContinuityChanged {
		t.Fatalf("explicit covered logout was not classified as changed: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofExpiresAtCompletion(t *testing.T) {
	clockReads := 0
	now := testStart
	verifier := newThreeProofVerifier(&now, "completion-expiry")
	verifier.now = func() time.Time {
		clockReads++
		if clockReads <= 4 {
			return testStart
		}
		return testStart.Add(MaxEvidenceAge + time.Second)
	}
	input := makeThreeProofInput(t, verifier, testStart,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
		authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
	result := verifier.VerifyThreeProof(input)
	if clockReads != 5 {
		t.Fatalf("expected two issue, two observation, and one completion clock reads; got %d", clockReads)
	}
	if result.Identity.State != IdentityUnavailable || result.Authentication.State != Unknown ||
		result.Continuity.State != ContinuityUnavailable || result.Continuity.CoverageComplete {
		t.Fatalf("expired proof remained qualified at completion: %+v", result)
	}
	assertNonAuthorizingThreeProof(t, result)
}

func TestThreeProofLifecycleRejectsZeroFutureAndReversedBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*LifecycleEvidence)
	}{
		{name: "zero baseline time", mutate: func(w *LifecycleEvidence) { w.BaselineAcceptedAt = time.Time{} }},
		{name: "zero coverage start", mutate: func(w *LifecycleEvidence) { w.CoveredFrom = time.Time{} }},
		{name: "future coverage end", mutate: func(w *LifecycleEvidence) {
			w.CoveredThrough = testStart.Add(time.Second)
			w.Metadata.ObservedAt = w.CoveredThrough
		}},
		{name: "reversed coverage interval", mutate: func(w *LifecycleEvidence) {
			w.CoveredFrom = testStart.Add(time.Second)
		}},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			now := testStart
			verifier := newThreeProofVerifier(&now, "lifecycle-bounds-"+string(rune('a'+i)))
			input := makeThreeProofInput(t, verifier, testStart,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200,
				authenticatedFixtureBody(threeProofAccountID, "same", "Same"), 200)
			test.mutate(input.Lifecycle)
			result := verifier.VerifyThreeProof(input)
			if result.Continuity.State == ContinuitySame || result.Continuity.State == ContinuityChanged || result.Continuity.CoverageComplete {
				t.Fatalf("invalid lifecycle interval remained complete: %+v", result.Continuity)
			}
			assertNonAuthorizingThreeProof(t, result)
		})
	}
}

func assertNonAuthorizingThreeProof(t *testing.T, result ThreeProofResult) {
	t.Helper()
	if result.CanAuthorize() || result.CaptureAuthorized || result.NativeQualified || result.ProductionQualified {
		t.Fatal("three-proof fixture result became authorization or qualification evidence")
	}
}
