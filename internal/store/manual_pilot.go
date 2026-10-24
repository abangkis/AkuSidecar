package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// ManualPilotOutcome describes durable knowledge, not a retry instruction.
type ManualPilotOutcome string

const (
	Committed          ManualPilotOutcome = "committed"
	AlreadyCommitted   ManualPilotOutcome = "already_committed"
	RejectedNoMutation ManualPilotOutcome = "rejected_no_mutation"
	Indeterminate      ManualPilotOutcome = "indeterminate"
)
const manualPilotSessionPrefix = "manual_pilot_session_v1:"
const manualPilotOpPrefix = "manual_pilot_op_v1:"

var errManualPilot = errors.New("manual pilot authority rejected")

// ManualPilotPermit is process-local authority. Serialization cannot transfer it.
type ManualPilotPermit struct {
	store    *Store
	state    manualPilotState
	deadline time.Time
	revoked  atomic.Bool
}

func (*ManualPilotPermit) String() string               { return "[manual pilot permit redacted]" }
func (*ManualPilotPermit) GoString() string             { return "[manual pilot permit redacted]" }
func (*ManualPilotPermit) MarshalJSON() ([]byte, error) { return nil, errManualPilot }
func (*ManualPilotPermit) UnmarshalJSON([]byte) error   { return errManualPilot }

type ManualPilotClaim struct {
	permit                        *ManualPilotPermit
	commandID, claimOpID, binding string
}

func (*ManualPilotClaim) String() string               { return "[manual pilot claim redacted]" }
func (*ManualPilotClaim) GoString() string             { return "[manual pilot claim redacted]" }
func (*ManualPilotClaim) MarshalJSON() ([]byte, error) { return nil, errManualPilot }
func (*ManualPilotClaim) UnmarshalJSON([]byte) error   { return errManualPilot }

type ManualPilotClaimResult struct {
	Outcome ManualPilotOutcome
	Command *domain.BridgeCommand
	Handle  *ManualPilotClaim
}

// ManualPilotOperation is a read-only durable receipt, never dispatch authority.
type ManualPilotOperation struct {
	OperationID string             `json:"operationId"`
	Kind        string             `json:"kind"`
	SessionID   string             `json:"sessionId"`
	RunID       string             `json:"runId"`
	CommandID   string             `json:"commandId,omitempty"`
	Digest      string             `json:"digest"`
	Binding     string             `json:"binding"`
	DecisionAt  string             `json:"decisionAt"`
	Outcome     ManualPilotOutcome `json:"outcome"`
}
type manualPilotState struct {
	SessionID       string        `json:"sessionId"`
	RunID           string        `json:"runId"`
	Source          domain.Source `json:"source"`
	Incarnation     string        `json:"incarnation"`
	Epoch           string        `json:"epoch"`
	Attempt         string        `json:"attempt"`
	OwnerRef        string        `json:"ownerRef"`
	PairRef         string        `json:"pairRef"`
	OwnerGeneration uint64        `json:"ownerGeneration"`
	Revision        uint64        `json:"revision"`
	Closed          bool          `json:"closed"`
}

func rejectManualPilotRun(ctx context.Context, tx *sql.Tx, runID string) error {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs r JOIN meta m ON m.key=?||r.session_id WHERE r.id=?`, manualPilotSessionPrefix, runID).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errManualPilot
	}
	return nil
}
func readManualState(ctx context.Context, tx *sql.Tx, id string) (manualPilotState, string, error) {
	var v manualPilotState
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, manualPilotSessionPrefix+id).Scan(&raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &v)
	}
	return v, raw, err
}
func writeManualState(ctx context.Context, tx *sql.Tx, v manualPilotState, old string) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	r, err := tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key=? AND value=?`, string(raw), manualPilotSessionPrefix+v.SessionID, old)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errManualPilot
	}
	return nil
}
func revokeManualPilotSessionTx(ctx context.Context, tx *sql.Tx, id string) error {
	v, raw, err := readManualState(ctx, tx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	v.Closed = true
	v.Revision++
	return writeManualState(ctx, tx, v, raw)
}
func (s *Store) liveManualPilot(ctx context.Context, tx *sql.Tx, p *ManualPilotPermit) error {
	if p == nil || p.store != s || p.revoked.Load() || !s.Now().Before(p.deadline) || s.manualPilots[p.state.SessionID] != p {
		return errManualPilot
	}
	v, _, err := readManualState(ctx, tx, p.state.SessionID)
	if err != nil {
		return err
	}
	if v != p.state || v.Closed {
		return errManualPilot
	}
	var incarnation, source, session, status, sessionStatus, coverage string
	if err = tx.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='manual_pilot_incarnation_v1'`).Scan(&incarnation); err != nil {
		return err
	}
	if incarnation != v.Incarnation {
		return errManualPilot
	}
	if err = tx.QueryRowContext(ctx, `SELECT r.source,r.session_id,r.status,s.status,s.coverage_json FROM runs r JOIN sessions s ON s.id=r.session_id WHERE r.id=?`, v.RunID).Scan(&source, &session, &status, &sessionStatus, &coverage); err != nil {
		return err
	}
	if source != string(domain.SourceX) || session != v.SessionID || status == "cancelled" || status == "failed" || status == "completed" || sessionStatus == "cancelled" || sessionStatus == "completed" || sessionStatus == "failed" {
		return errManualPilot
	}
	var plan struct {
		Policy  string            `json:"collectionPolicy"`
		Drivers map[string]string `json:"collectionSourceDrivers"`
	}
	if json.Unmarshal([]byte(coverage), &plan) != nil || plan.Policy != domain.SessionCollectionPolicyHybridHeadlessV1 || plan.Drivers["x"] != "headless" {
		return errManualPilot
	}
	// SQLite reads can wait. Recheck elapsed authority after the last database
	// access, immediately before the caller's commit boundary.
	if p.revoked.Load() || !s.Now().Before(p.deadline) {
		return errManualPilot
	}
	return nil
}
func manualDigest(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func readManualOp(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (ManualPilotOperation, error) {
	var v ManualPilotOperation
	var raw string
	e := q.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, manualPilotOpPrefix+id).Scan(&raw)
	if e == nil {
		e = json.Unmarshal([]byte(raw), &v)
	}
	return v, e
}
func (s *Store) LookupManualPilotOp(ctx context.Context, id string) (ManualPilotOperation, error) {
	v, e := readManualOp(ctx, s.db, id)
	if e != nil {
		v.Outcome = Indeterminate
	}
	return v, e
}
func matchingManualOp(a, b ManualPilotOperation) bool {
	return a.OperationID == b.OperationID && a.Kind == b.Kind && a.SessionID == b.SessionID && a.RunID == b.RunID && a.CommandID == b.CommandID && a.Digest == b.Digest && a.Binding == b.Binding && a.Outcome == Committed
}
func writeManualOp(ctx context.Context, tx *sql.Tx, v ManualPilotOperation) error {
	raw, e := json.Marshal(v)
	if e != nil {
		return e
	}
	_, e = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, manualPilotOpPrefix+v.OperationID, string(raw))
	return e
}
func (s *Store) finishManualTx(ctx context.Context, tx *sql.Tx, op ManualPilotOperation) (ManualPilotOutcome, error) {
	var err error
	if s.manualPilotCommit != nil {
		err = s.manualPilotCommit(tx)
	} else {
		err = tx.Commit()
	}
	if err == nil {
		return Committed, nil
	}
	// Release a still-open transaction before using a fresh read. An absent receipt
	// after a COMMIT error alone does not establish a safe recapture decision.
	_ = tx.Rollback()
	got, readErr := readManualOp(ctx, s.db, op.OperationID)
	if readErr == nil && matchingManualOp(got, op) {
		return AlreadyCommitted, nil
	}
	return Indeterminate, fmt.Errorf("manual pilot commit outcome unresolved: %w", err)
}
func (s *Store) ClaimManualPilot(ctx context.Context, p *ManualPilotPermit, commandID, opID string) (ManualPilotClaimResult, error) {
	result := ManualPilotClaimResult{Outcome: RejectedNoMutation}
	if p == nil || p.store != s || strings.TrimSpace(opID) == "" || commandID == "" {
		return result, errManualPilot
	}
	s.manualPilotMu.Lock()
	defer s.manualPilotMu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return result, e
	}
	defer tx.Rollback()
	digest, _ := manualDigest([]string{p.state.Attempt, commandID})
	op := ManualPilotOperation{OperationID: opID, Kind: "claim", SessionID: p.state.SessionID, RunID: p.state.RunID, CommandID: commandID, Digest: digest, Binding: p.state.Attempt, Outcome: Committed}
	prior, pe := readManualOp(ctx, tx, opID)
	if pe == nil && !matchingManualOp(prior, op) {
		return result, errManualPilot
	}
	if pe != nil && !errors.Is(pe, sql.ErrNoRows) {
		return result, pe
	}
	if pe == nil {
		result.Outcome = AlreadyCommitted
		return result, nil
	}
	if e = s.liveManualPilot(ctx, tx, p); e != nil {
		return result, e
	}
	var cmd domain.BridgeCommand
	var raw string
	e = tx.QueryRowContext(ctx, `SELECT id,run_id,type,status,payload_json,created_at FROM bridge_commands WHERE id=?`, commandID).Scan(&cmd.ID, &cmd.RunID, &cmd.Type, &cmd.Status, &raw, &cmd.CreatedAt)
	if e != nil {
		return result, e
	}
	if cmd.RunID != p.state.RunID {
		return result, errManualPilot
	}
	cmd.Payload, e = decodeCapturePayload(raw)
	if e != nil {
		return result, e
	}
	collector, e := CaptureCollector(cmd.Payload)
	if e != nil || collector != "headless" {
		return result, errManualPilot
	}
	runtime, ok := cmd.Payload["captureRuntime"].(map[string]any)
	if !ok || runtime["driver"] != "headless" {
		return result, errManualPilot
	}
	{
		var n int
		e = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.session_id=? AND c.status='claimed'`, p.state.SessionID).Scan(&n)
		if e != nil {
			return result, e
		}
		if n != 0 || cmd.Status != "queued" {
			return result, errManualPilot
		}
		now := s.Now().UTC().Format(time.RFC3339Nano)
		r, e := tx.ExecContext(ctx, `UPDATE bridge_commands SET status='claimed',claimed_by=?,claimed_at=? WHERE id=? AND run_id=? AND status='queued'`, p.state.Attempt, now, commandID, p.state.RunID)
		if e != nil {
			return result, e
		}
		n64, _ := r.RowsAffected()
		if n64 != 1 {
			return result, errManualPilot
		}
		if s.manualPilotBeforeCommit != nil {
			s.manualPilotBeforeCommit()
		}
		if e = s.liveManualPilot(ctx, tx, p); e != nil {
			return result, e
		}
		op.DecisionAt = s.Now().UTC().Format(time.RFC3339Nano)
		if e = writeManualOp(ctx, tx, op); e != nil {
			return result, e
		}
		if e = s.liveManualPilot(ctx, tx, p); e != nil {
			return result, e
		}
		result.Outcome, e = s.finishManualTx(ctx, tx, op)
		if e != nil {
			return result, e
		}
		cmd.Status = "claimed"
		cmd.ClaimedAt = &now
	}
	result.Command = &cmd
	result.Handle = &ManualPilotClaim{permit: p, commandID: commandID, claimOpID: opID, binding: p.state.Attempt}
	return result, nil
}

func (s *Store) CommitManualPilotResult(ctx context.Context, h *ManualPilotClaim, opID string, observation domain.Observation) (ManualPilotOutcome, error) {
	if h == nil || h.permit == nil || h.permit.store != s || opID == "" {
		return RejectedNoMutation, errManualPilot
	}
	// Clone before storage augmentation. A missing timestamp stays missing in the
	// digest, so an identical retry does not acquire a new wall-clock identity.
	raw, e := json.Marshal(observation)
	if e != nil {
		return RejectedNoMutation, e
	}
	var clone domain.Observation
	if e = json.Unmarshal(raw, &clone); e != nil {
		return RejectedNoMutation, e
	}
	digest, e := manualDigest(clone)
	if e != nil {
		return RejectedNoMutation, e
	}
	p := h.permit
	op := ManualPilotOperation{OperationID: opID, Kind: "result", SessionID: p.state.SessionID, RunID: p.state.RunID, CommandID: h.commandID, Digest: digest, Binding: h.binding, Outcome: Committed}
	s.manualPilotMu.Lock()
	defer s.manualPilotMu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return RejectedNoMutation, e
	}
	defer tx.Rollback()
	prior, pe := readManualOp(ctx, tx, opID)
	if pe == nil {
		if matchingManualOp(prior, op) {
			return AlreadyCommitted, nil
		}
		return RejectedNoMutation, errManualPilot
	}
	if !errors.Is(pe, sql.ErrNoRows) {
		return RejectedNoMutation, pe
	}
	if e = s.liveManualPilot(ctx, tx, p); e != nil {
		return RejectedNoMutation, e
	}
	if clone.Source != domain.SourceX || h.binding != p.state.Attempt {
		return RejectedNoMutation, errManualPilot
	}
	claim, e := readManualOp(ctx, tx, h.claimOpID)
	if e != nil || claim.Kind != "claim" || claim.CommandID != h.commandID || claim.Binding != h.binding || claim.RunID != p.state.RunID {
		return RejectedNoMutation, errManualPilot
	}
	var owner string
	if e = tx.QueryRowContext(ctx, `SELECT claimed_by FROM bridge_commands WHERE id=? AND run_id=? AND status='claimed'`, h.commandID, p.state.RunID).Scan(&owner); e != nil || owner != p.state.Attempt {
		return RejectedNoMutation, errManualPilot
	}
	if e = s.saveObservationTx(ctx, tx, h.commandID, p.state.RunID, clone, s.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return RejectedNoMutation, e
	}
	if s.manualPilotBeforeCommit != nil {
		s.manualPilotBeforeCommit()
	}
	if e = s.liveManualPilot(ctx, tx, p); e != nil {
		return RejectedNoMutation, e
	}
	op.DecisionAt = s.Now().UTC().Format(time.RFC3339Nano)
	if e = writeManualOp(ctx, tx, op); e != nil {
		return RejectedNoMutation, e
	}
	if e = s.liveManualPilot(ctx, tx, p); e != nil {
		return RejectedNoMutation, e
	}
	return s.finishManualTx(ctx, tx, op)
}

func (s *Store) RevokeManualPilot(ctx context.Context, p *ManualPilotPermit, opID string) (ManualPilotOutcome, error) {
	if p == nil || p.store != s || opID == "" {
		return RejectedNoMutation, errManualPilot
	}
	// Close before waiting for an in-progress transaction's final authorization.
	p.revoked.Store(true)
	s.manualPilotMu.Lock()
	defer s.manualPilotMu.Unlock()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return Indeterminate, e
	}
	defer tx.Rollback()
	digest, _ := manualDigest(p.state)
	op := ManualPilotOperation{OperationID: opID, Kind: "revoke", SessionID: p.state.SessionID, RunID: p.state.RunID, Digest: digest, Binding: p.state.Attempt, Outcome: Committed}
	prior, pe := readManualOp(ctx, tx, opID)
	if pe == nil {
		if matchingManualOp(prior, op) {
			return AlreadyCommitted, nil
		}
		return RejectedNoMutation, errManualPilot
	}
	if !errors.Is(pe, sql.ErrNoRows) {
		return Indeterminate, pe
	}
	v, raw, e := readManualState(ctx, tx, p.state.SessionID)
	if e != nil {
		return Indeterminate, e
	}
	if v.Attempt != p.state.Attempt || v.Incarnation != p.state.Incarnation {
		return RejectedNoMutation, errManualPilot
	}
	v.Closed = true
	v.Revision++
	if e = writeManualState(ctx, tx, v, raw); e != nil {
		return Indeterminate, e
	}
	op.DecisionAt = s.Now().UTC().Format(time.RFC3339Nano)
	if e = writeManualOp(ctx, tx, op); e != nil {
		return Indeterminate, e
	}
	return s.finishManualTx(ctx, tx, op)
}
