package engine

import (
	"github.com/abangkis/AkuSidecar/internal/capture"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func reconcileCapturedSnapshots(source domain.Source, snapshots []domain.Snapshot) []domain.Snapshot {
	return capture.ReconcileSnapshots(source, snapshots)
}
