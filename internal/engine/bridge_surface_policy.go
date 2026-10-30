package engine

import "github.com/abangkis/AkuSidecar/internal/domain"

const ExpectedBridgeSurfacePolicyFingerprint = "81de92d4b6289fba020601ad2e4629b85d193ce1be2e167cbb0c607dc2b523ad"

func expectedBridgeSurfacePolicy() domain.BridgeCaptureSurfacePolicy {
	return domain.BridgeCaptureSurfacePolicy{
		Schema:      "aku.surface-policy.v1",
		Fingerprint: ExpectedBridgeSurfacePolicyFingerprint,
	}
}

func bridgeSurfacePolicyMatches(value domain.BridgeCaptureSurfacePolicy) bool {
	return value == expectedBridgeSurfacePolicy()
}
