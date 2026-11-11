package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/capture"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// CompleteRun commits candidate assessments and completed status atomically.
// Observation/seen records and empty completed runs are not evaluation receipts.
func completedContinuityEvaluations(ctx context.Context, tx *sql.Tx, source domain.Source, blocks []domain.Block) (map[string][]string, error) {
	args := []any{source}
	for _, block := range blocks {
		args = append(args, strings.TrimSpace(block.EvidenceKey))
	}
	rows, err := tx.QueryContext(ctx, `SELECT a.evidence_key,a.run_id
		FROM candidate_assessments a JOIN runs r ON r.id=a.run_id
		WHERE a.source=? AND r.source=a.source AND r.status='completed'
		AND a.evidence_key IN (`+strings.TrimSuffix(strings.Repeat("?,", len(blocks)), ",")+`)
		ORDER BY a.created_at DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("read completed continuity evaluations: %w", err)
	}
	defer rows.Close()
	values := map[string][]string{}
	for rows.Next() {
		var key, runID string
		if err := rows.Scan(&key, &runID); err != nil {
			return nil, err
		}
		values[key] = append(values[key], runID)
	}
	return values, rows.Err()
}

func continuityVersionEvaluated(ctx context.Context, tx *sql.Tx, source domain.Source, current domain.Block, runIDs []string, cache map[string]map[string]domain.Block) (bool, error) {
	for _, runID := range runIDs {
		blocks, loaded := cache[runID]
		if !loaded {
			observations, err := observationsFrom(ctx, tx, runID)
			if err != nil {
				return false, fmt.Errorf("read evaluated continuity version: %w", err)
			}
			var snapshots []domain.Snapshot
			for _, observation := range observations {
				snapshots = append(snapshots, observation.Snapshots...)
			}
			blocks = map[string]domain.Block{}
			for _, block := range canonicalContinuityBlocks(domain.Observation{Snapshots: capture.ReconcileSnapshots(source, snapshots)}) {
				blocks[strings.TrimSpace(block.EvidenceKey)] = block
			}
			cache[runID] = blocks
		}
		previous, ok := blocks[strings.TrimSpace(current.EvidenceKey)]
		if !ok || continuityContentFingerprint(previous) != continuityContentFingerprint(current) {
			continue
		}
		if source == domain.SourceX && strings.TrimSpace(current.RelationshipType) == "quote" && !sameXQuoteText(previous, current) {
			continue
		}
		priorContext, currentContext := continuityContextFingerprint(previous), continuityContextFingerprint(current)
		if priorContext == currentContext {
			return true, nil
		}
		if source == domain.SourceX && strings.TrimSpace(current.RelationshipType) == "quote" {
			if _, hydrationOnly := reconcileXQuoteFingerprint(previous, current, priorContext, currentContext); hydrationOnly {
				return true, nil
			}
		}
	}
	return false, nil
}
