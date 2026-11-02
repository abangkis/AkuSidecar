//go:build windows && auth_session_dev

package engine

import (
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestEngineObservationCharacterizationNativeDecoder(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		for _, mode := range []string{"valid", "missing-key", "missing-coverage", "wrong-source", "unknown-field", "duplicate-field"} {
			t.Run(string(source)+"/"+mode, func(t *testing.T) {
				value := observationContractFixture()
				value.Source = source
				value.Snapshots[0].Blocks[0].Permalink = ""
				value.Snapshots[0].Blocks[0].PlatformID = "42"
				if mode == "missing-key" {
					value.Snapshots[0].Blocks[0].EvidenceKey = ""
				}
				if mode == "missing-coverage" {
					value.Coverage = nil
				}
				if mode == "wrong-source" {
					value.Source = domain.SourceFacebook
				}
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "unknown-field" {
					raw = append([]byte(`{"private":"fixture",`), raw[1:]...)
				}
				if mode == "duplicate-field" {
					raw = append([]byte(`{"source":"invalid",`), raw[1:]...)
				}
				var out domain.Observation
				err = decodeSourceNativeObservation(raw, source, &out)
				if (err == nil) != (mode == "valid") {
					t.Fatalf("native decode=%v", err)
				}
				if mode == "missing-key" && len(out.Snapshots) > 0 && out.Snapshots[0].Blocks[0].EvidenceKey != "" {
					t.Fatal("native decoder silently normalized missing evidence identity")
				}
			})
		}
	}
}
