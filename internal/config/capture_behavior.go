package config

import (
	"errors"
	"math"
)

// CaptureBehaviorConfig is a startup policy for bounded acquisition. It never
// changes source identity, browser fingerprints, authentication or access gates.
// Zero values retain documented defaults; Enabled must be explicitly selected.
type CaptureBehaviorConfig struct {
	Enabled                bool    `json:"enabled"`
	MinCaptureIntervalMS   int     `json:"minCaptureIntervalMs"`
	AdditionalJitterMS     int     `json:"additionalJitterMs"`
	ScrollFraction         float64 `json:"scrollFraction"`
	ScrollSettleMS         int     `json:"scrollSettleMs"`
	TemporaryBackoffBaseMS int     `json:"temporaryBackoffBaseMs"`
	TemporaryBackoffMaxMS  int     `json:"temporaryBackoffMaxMs"`
}

func (b CaptureBehaviorConfig) Effective() CaptureBehaviorConfig {
	if b.MinCaptureIntervalMS == 0 {
		b.MinCaptureIntervalMS = 5000
	}
	if b.ScrollFraction == 0 {
		b.ScrollFraction = 0.75
	}
	if b.ScrollSettleMS == 0 {
		b.ScrollSettleMS = 900
	}
	if b.TemporaryBackoffBaseMS == 0 {
		b.TemporaryBackoffBaseMS = 10000
	}
	if b.TemporaryBackoffMaxMS == 0 {
		b.TemporaryBackoffMaxMS = 120000
	}
	return b
}

func (b CaptureBehaviorConfig) Validate() error {
	b = b.Effective()
	if b.MinCaptureIntervalMS < 1000 || b.MinCaptureIntervalMS > 900000 {
		return errors.New("capture.behavior.minCaptureIntervalMs must be between 1000 and 900000")
	}
	if b.AdditionalJitterMS < 0 || b.AdditionalJitterMS > 60000 {
		return errors.New("capture.behavior.additionalJitterMs must be between 0 and 60000")
	}
	if math.IsNaN(b.ScrollFraction) || math.IsInf(b.ScrollFraction, 0) || b.ScrollFraction < 0.5 || b.ScrollFraction > 1 {
		return errors.New("capture.behavior.scrollFraction must be between 0.5 and 1")
	}
	if b.ScrollSettleMS < 100 || b.ScrollSettleMS > 2000 {
		return errors.New("capture.behavior.scrollSettleMs must be between 100 and 2000")
	}
	if b.TemporaryBackoffBaseMS < 1000 || b.TemporaryBackoffBaseMS > 300000 {
		return errors.New("capture.behavior.temporaryBackoffBaseMs must be between 1000 and 300000")
	}
	if b.TemporaryBackoffMaxMS < b.TemporaryBackoffBaseMS || b.TemporaryBackoffMaxMS > 900000 {
		return errors.New("capture.behavior.temporaryBackoffMaxMs must be at least the base and at most 900000")
	}
	return nil
}
