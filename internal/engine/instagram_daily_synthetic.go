//go:build auth_session_dev

package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// This adapter is deliberately synthetic-only. It exercises the real daily
// controller and Store fences without claiming a qualified native IG manifest.
// The private composition is not an RPC/API accepting caller-supplied proof.
type instagramSyntheticChallenge struct {
	Nonce, ProfileScope                 string
	ReaderProfileID, CollectorProfileID string
	Generation                          uint64
}
type instagramSyntheticEvidence struct {
	Domain, Nonce, ProfileScope, Account, Revision string
	Role, OwnerProfileID                           string
	Source                                         domain.Source
	Generation                                     uint64
	Authenticated                                  bool
	ObservedAt                                     time.Time
}
type instagramSyntheticTransport interface {
	ReaderEvidence(context.Context, instagramSyntheticChallenge) (instagramSyntheticEvidence, error)
	Derive(context.Context, instagramSyntheticChallenge, instagramSyntheticEvidence, time.Time) (instagramSyntheticCollector, error)
}
type instagramSyntheticCollector interface {
	Evidence(context.Context, instagramSyntheticChallenge) (instagramSyntheticEvidence, error)
	Capture(context.Context, map[string]any) (domain.Observation, error)
	// Close must close and verify every owned capture/recovery target. It must
	// never close the reader; uncertainty is returned and latches the controller.
	Close(context.Context) error
}

type instagramSyntheticOwner struct {
	engine           *Engine
	transport        instagramSyntheticTransport
	profile          string
	mu               sync.Mutex
	account          string
	generation       uint64
	closed           bool
	cleanupUncertain bool
}
type instagramSyntheticBatch struct {
	owner     *instagramSyntheticOwner
	collector instagramSyntheticCollector
	run       domain.Run
	challenge instagramSyntheticChallenge
	proof     instagramSyntheticEvidence
	started   time.Time
	permit    *store.ManualPilotPermit
	parent    context.Context
	closed    bool
}

// Only offline trusted Go composition can attach this invented evidence source.
// Native IG selection alone therefore cannot manufacture a factory or a grant.
func (e *Engine) attachInstagramSynthetic(parent context.Context, profile string, transport instagramSyntheticTransport) error {
	d := e.dailyController(domain.SourceInstagram)
	if !e.config.Dev || !d.configured() || transport == nil || !validXDailyProfileScope(profile) || parent == nil {
		return errors.New("synthetic Instagram composition rejected")
	}
	d.state.mu.Lock()
	if d.state.attached || d.state.closed {
		d.state.mu.Unlock()
		return errors.New("Instagram already attached")
	}
	d.state.attached = true
	d.state.mu.Unlock()
	return d.configure(parent, func(context.Context, context.Context) (xDailyNativeOwner, error) {
		return &instagramSyntheticOwner{engine: e, transport: transport, profile: profile}, nil
	}, profile)
}

func (o *instagramSyntheticOwner) challenge(generation uint64) instagramSyntheticChallenge {
	return instagramSyntheticChallenge{Nonce: domain.NewID("synthetic-ig-proof"), ProfileScope: o.profile, Generation: generation,
		ReaderProfileID: "synthetic:reader:" + o.profile, CollectorProfileID: "synthetic:collector:" + o.profile + ":" + strconv.FormatUint(generation, 10)}
}
func (o *instagramSyntheticOwner) validate(v instagramSyntheticEvidence, c instagramSyntheticChallenge, role string) error {
	now := o.engine.store.Now()
	profile := c.ReaderProfileID
	if role == "collector" {
		profile = c.CollectorProfileID
	}
	if v.Role != role || v.OwnerProfileID != profile || c.ReaderProfileID == c.CollectorProfileID {
		return errors.New("synthetic Instagram owner binding rejected")
	}
	if v.Domain != "synthetic-fixture" || v.Source != domain.SourceInstagram || v.Nonce != c.Nonce || v.ProfileScope != c.ProfileScope || v.Generation != c.Generation || !v.Authenticated || !strings.HasPrefix(v.Account, "synthetic:") || len(v.Account) <= 10 || !strings.HasPrefix(v.Revision, "synthetic:") || len(v.Revision) <= 10 || v.ObservedAt.IsZero() || v.ObservedAt.After(now) || now.Sub(v.ObservedAt) > nativePilotAge {
		return errors.New("synthetic Instagram proof rejected")
	}
	return nil
}
func (o *instagramSyntheticOwner) CheckReader(ctx context.Context) error {
	o.mu.Lock()
	closed, generation, account := o.closed, o.generation, o.account
	o.mu.Unlock()
	if closed || ctx.Err() != nil {
		return errors.New("synthetic Instagram reader unavailable")
	}
	c := o.challenge(generation)
	v, err := o.transport.ReaderEvidence(ctx, c)
	if err != nil {
		return err
	}
	if err = o.validate(v, c, "reader"); err != nil {
		return err
	}
	if account != "" && v.Account != account {
		return errors.New("synthetic Instagram account changed")
	}
	return nil
}
func (o *instagramSyntheticOwner) PrepareReader(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error) {
	return readerbroker.NativePostPreparation{}, errors.New("synthetic reader has no native action")
}
func (o *instagramSyntheticOwner) Close(context.Context) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.closed = true
	if o.cleanupUncertain {
		return errors.New("synthetic Instagram cleanup remains unverified")
	}
	return nil
}

func (o *instagramSyntheticOwner) BeginBatch(ctx, parent context.Context, generation uint64, run domain.Run) (xDailyNativeBatch, error) {
	if run.Source != domain.SourceInstagram || generation == 0 || parent == nil || parent.Err() != nil {
		return nil, errors.New("synthetic Instagram batch rejected")
	}
	o.mu.Lock()
	if o.closed || generation <= o.generation {
		o.mu.Unlock()
		return nil, errors.New("synthetic Instagram generation rejected")
	}
	o.generation = generation
	account := o.account
	o.mu.Unlock()
	c := o.challenge(generation)
	v, err := o.transport.ReaderEvidence(ctx, c)
	if err != nil {
		return nil, err
	}
	if err = o.validate(v, c, "reader"); err != nil {
		return nil, err
	}
	if account != "" && account != v.Account {
		return nil, errors.New("synthetic Instagram account changed")
	}
	b := &instagramSyntheticBatch{owner: o, run: run, challenge: c, proof: v, started: o.engine.store.Now(), parent: parent}
	// Returning the partially owned batch makes failed derivation cleanup part
	// of the same controller path as X; no target can escape on startup failure.
	b.collector, err = o.transport.Derive(ctx, c, v, b.Deadline())
	if err != nil {
		return b, err
	}
	if b.collector == nil {
		return b, errors.New("synthetic Instagram collector missing")
	}
	if err = b.verify(ctx); err != nil {
		return b, err
	}
	o.mu.Lock()
	o.account = v.Account
	o.mu.Unlock()
	raw, _ := json.Marshal(struct {
		Source                            domain.Source
		Profile, Account, Revision, Nonce string
		Generation                        uint64
	}{domain.SourceInstagram, o.profile, v.Account, v.Revision, c.Nonce, generation})
	hash := sha256.Sum256(raw)
	binding := store.DevelopmentManualPilotBinding{Digest: hex.EncodeToString(hash[:]), Attempt: c.Nonce, OwnerGeneration: generation, StartedAt: b.started, Deadline: b.Deadline()}
	b.permit, err = o.engine.store.NewDevelopmentSourceDailyPilotIssuer(domain.SourceInstagram, func(call context.Context) (store.DevelopmentManualPilotBinding, error) {
		if err := b.verify(call); err != nil {
			return store.DevelopmentManualPilotBinding{}, err
		}
		return binding, nil
	})(ctx, run.SessionID, run.ID)
	return b, err
}
func (b *instagramSyntheticBatch) StartedAt() time.Time { return b.started }
func (b *instagramSyntheticBatch) Deadline() time.Time  { return b.started.Add(nativePilotLife) }
func (b *instagramSyntheticBatch) verify(ctx context.Context) error {
	if b.closed || b.parent.Err() != nil || ctx.Err() != nil || !b.owner.engine.store.Now().Before(b.Deadline()) {
		return errors.New("synthetic Instagram batch expired")
	}
	c := b.owner.challenge(b.challenge.Generation)
	reader, err := b.owner.transport.ReaderEvidence(ctx, c)
	if err != nil {
		return err
	}
	collector, err := b.collector.Evidence(ctx, c)
	if err != nil {
		return err
	}
	for role, v := range map[string]instagramSyntheticEvidence{"reader": reader, "collector": collector} {
		if err = b.owner.validate(v, c, role); err != nil {
			return err
		}
		if v.Account != b.proof.Account || v.Revision != b.proof.Revision {
			return errors.New("synthetic Instagram paired proof changed")
		}
	}
	return nil
}
func (b *instagramSyntheticBatch) Capture(ctx context.Context, command string) (domain.Run, error) {
	if err := b.verify(ctx); err != nil {
		return domain.Run{}, err
	}
	claim, err := b.owner.engine.store.ClaimManualPilot(ctx, b.permit, command, "instagram-claim:"+command)
	if err != nil {
		return domain.Run{}, err
	}
	if claim.Handle == nil || claim.Command == nil {
		return domain.Run{}, errors.New("synthetic Instagram claim unavailable")
	}
	observation, err := b.collector.Capture(ctx, claim.Command.Payload)
	if err != nil {
		return domain.Run{}, err
	}
	if err = b.verify(ctx); err != nil {
		return domain.Run{}, err
	}
	result, err := b.owner.engine.store.CommitManualPilotResult(ctx, claim.Handle, "instagram-result:"+command, observation)
	if err != nil {
		return domain.Run{}, err
	}
	if result != store.Committed {
		return domain.Run{}, errors.New("synthetic Instagram commit unavailable")
	}
	return b.owner.engine.afterAcceptedObservation(ctx, b.run)
}
func (b *instagramSyntheticBatch) Close(ctx context.Context) error {
	b.closed = true
	var cleanup, revoke error
	if b.collector != nil {
		cleanup = b.collector.Close(ctx)
	}
	if b.permit != nil {
		_, revoke = b.owner.engine.store.RevokeManualPilot(ctx, b.permit, "instagram-revoke:"+b.run.ID)
	}
	if cleanup != nil || revoke != nil {
		b.owner.mu.Lock()
		b.owner.cleanupUncertain = true
		b.owner.mu.Unlock()
	}
	return errors.Join(cleanup, revoke)
}
