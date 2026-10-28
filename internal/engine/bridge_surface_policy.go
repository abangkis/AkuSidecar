package engine

import "github.com/abangkis/AkuSidecar/internal/domain"

const ExpectedBridgeSurfacePolicyFingerprint = "e8439a2c931876aed1b7156eb58a61a828f9e431183fbf76a37308653f4c40f7"

func expectedBridgeSurfacePolicy() domain.BridgeCaptureSurfacePolicy {
	return domain.BridgeCaptureSurfacePolicy{
		Schema:      "aku.surface-policy.v1",
		Fingerprint: ExpectedBridgeSurfacePolicyFingerprint,
	}
}

func bridgeSurfacePolicyMatches(value domain.BridgeCaptureSurfacePolicy) bool {
	return value == expectedBridgeSurfacePolicy()
}
