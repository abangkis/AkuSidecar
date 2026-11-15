package engine

import (
	"errors"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// scopeTargetObservation admits only an observed exact primary. A quote cannot
// satisfy the target, and missing timestamps are never inferred here.
func scopeTargetObservation(value *domain.Observation, target domain.CaptureTarget) error {
	page, ok := domain.CanonicalSourceURL(value.Source, value.PageURL)
	if value.Source != target.Source || !ok || page != target.Permalink {
		return errors.New("targeted capture landed outside the requested post")
	}
	found := false
	for i := range value.Snapshots {
		blocks := []domain.Block{}
		for _, block := range value.Snapshots[i].Blocks {
			canonical, valid := domain.CanonicalSourceURL(value.Source, block.Permalink)
			if valid && canonical == target.Permalink {
				blocks = append(blocks, block)
				found = true
			}
		}
		value.Snapshots[i].Blocks = blocks
	}
	if !found {
		return errors.New("targeted capture did not observe the requested primary post")
	}
	return nil
}
