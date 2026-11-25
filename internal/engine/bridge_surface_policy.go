package engine

import "github.com/abangkis/AkuSidecar/internal/domain"

const ExpectedBridgeSurfacePolicyFingerprint = "aabbcff3b55ee21285177c6dc05f328c236419d209a08d1f90f8d47b0e0a0352"

func expectedBridgeSurfacePolicy() domain.BridgeCaptureSurfacePolicy {
	return domain.BridgeCaptureSurfacePolicy{
		Schema:      "aku.surface-policy.v1",
		Fingerprint: ExpectedBridgeSurfacePolicyFingerprint,
	}
}

func bridgeSurfacePolicyMatches(value domain.BridgeCaptureSurfacePolicy) bool {
	return value == expectedBridgeSurfacePolicy()
}
