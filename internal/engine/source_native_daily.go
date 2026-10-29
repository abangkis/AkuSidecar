//go:build auth_session_dev

package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const instagramNativeRecipe = "instagram-native-v1"

// Qualification is a reviewed package decision, never an RPC/configuration bit.
// Offline fixtures prove the implementation contract, not native qualification.
const instagramNativeQualified = false

func sourceNativeNonce() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

type sourceNativeChallenge struct {
	Nonce        string `json:"nonce"`
	ProfileScope string `json:"profileScope"`
	Generation   uint64 `json:"generation"`
}
type sourceNativeProof struct {
	Source          domain.Source `json:"source"`
	Domain          string        `json:"domain"`
	Recipe          string        `json:"recipe"`
	Nonce           string        `json:"nonce"`
	ProfileScope    string        `json:"profileScope"`
	Generation      uint64        `json:"generation"`
	Role            string        `json:"role"`
	OwnerID         string        `json:"ownerID"`
	ProfileID       string        `json:"profileID"`
	AccountKey      string        `json:"accountKey"`
	SessionRevision string        `json:"sessionRevision"`
	Authenticated   bool          `json:"authenticated"`
	ObservedAtMS    int64         `json:"observedAtMS"`
}
type sourceNativePair struct {
	Reader    sourceNativeProof `json:"reader"`
	Collector sourceNativeProof `json:"collector"`
}

// Only trusted native composition implements these ports. CheckOwnership must
// independently verify OS owners and pinned directories, never trust proof JSON.
type sourceNativeTransport interface {
	CheckReader(context.Context) error
	Begin(context.Context, sourceNativeChallenge, time.Time) (sourceNativeCollector, error)
	Close(context.Context) error
}
type sourceNativeCollector interface {
	Seed(context.Context, sourceNativeChallenge) (sourceNativePair, error)
	Proof(context.Context, sourceNativeChallenge) (sourceNativePair, error)
	CheckOwnership(context.Context, sourceNativePair) error
	Capture(context.Context, map[string]any) (domain.Observation, error)
	Close(context.Context) error
}
type sourceNativeOwner struct {
	engine          *Engine
	source          domain.Source
	recipe, profile string
	transport       sourceNativeTransport
	mu              sync.Mutex
	generation      uint64
	account         string
	closed, failed  bool
	batch           *sourceNativeBatch
}
type sourceNativeBatch struct {
	owner     *sourceNativeOwner
	collector sourceNativeCollector
	challenge sourceNativeChallenge
	pair      sourceNativePair
	run       domain.Run
	parent    context.Context
	started   time.Time
	permit    *store.ManualPilotPermit
	mu        sync.Mutex
	closed    bool
	once      sync.Once
	closeErr  error
}

func (o *sourceNativeOwner) CheckReader(ctx context.Context) error {
	o.mu.Lock()
	closed := o.closed || o.failed
	o.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || closed {
		return sourceNativeFail(nil, "ownership", "unavailable")
	}
	if err := o.transport.CheckReader(ctx); err != nil {
		return sourceNativeFail(err, "ownership", "changed")
	}
	return nil
}
func (*sourceNativeOwner) PrepareReader(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error) {
	return readerbroker.NativePostPreparation{}, errors.New("native source reader action unavailable")
}
func (o *sourceNativeOwner) BeginBatch(ctx, parent context.Context, generation uint64, run domain.Run) (sourceDailyBatch, error) {
	if ctx == nil || parent == nil || ctx.Err() != nil || parent.Err() != nil || run.Source != o.source || run.ID == "" || run.SessionID == "" {
		return nil, sourceNativeFail(nil, "admission", "scope")
	}
	o.mu.Lock()
	if o.closed || o.failed || o.batch != nil || generation == 0 || generation != o.generation+1 {
		o.mu.Unlock()
		return nil, sourceNativeFail(nil, "admission", "generation")
	}
	o.generation = generation
	nonce, err := sourceNativeNonce()
	if err != nil {
		o.failed = true
		o.mu.Unlock()
		return nil, err
	}
	b := &sourceNativeBatch{owner: o, run: run, parent: parent, started: o.engine.store.Now(), challenge: sourceNativeChallenge{Nonce: nonce, ProfileScope: o.profile, Generation: generation}}
	o.batch = b
	o.mu.Unlock()
	ctx, cancel := context.WithDeadline(ctx, b.Deadline())
	defer cancel()
	fail := func(err error) (sourceDailyBatch, error) {
		o.mu.Lock()
		o.failed = true
		o.mu.Unlock()
		return b, sourceNativeFail(err, "startup", "unavailable")
	}
	if err := o.transport.CheckReader(ctx); err != nil {
		return fail(err)
	}
	b.collector, err = o.transport.Begin(ctx, b.challenge, b.Deadline())
	if err != nil {
		return fail(err)
	}
	if b.collector == nil {
		return fail(sourceNativeFail(nil, "collector", "unavailable"))
	}
	// Begin independently pins empty collector ownership before Seed is allowed.
	b.pair, err = b.collector.Seed(ctx, b.challenge)
	if err != nil {
		return fail(err)
	}
	if err = b.validate(ctx, b.challenge, b.pair); err != nil {
		return fail(err)
	}
	o.mu.Lock()
	changed := o.account != "" && o.account != b.pair.Reader.AccountKey
	if !changed {
		o.account = b.pair.Reader.AccountKey
	}
	o.mu.Unlock()
	if changed {
		return fail(sourceNativeFail(nil, "evidence", "changed"))
	}
	raw, _ := json.Marshal(struct {
		Source          domain.Source
		Profile, Recipe string
		Pair            sourceNativePair
	}{o.source, o.profile, o.recipe, b.pair})
	hash := sha256.Sum256(raw)
	clear(raw)
	binding := store.DevelopmentManualPilotBinding{Digest: hex.EncodeToString(hash[:]), Attempt: b.challenge.Nonce, OwnerGeneration: generation, StartedAt: b.started, Deadline: b.Deadline()}
	b.permit, err = o.engine.store.NewDevelopmentSourceDailyPilotIssuer(o.source, func(call context.Context) (store.DevelopmentManualPilotBinding, error) {
		if e := b.verify(call); e != nil {
			return store.DevelopmentManualPilotBinding{}, e
		}
		return binding, nil
	})(ctx, run.SessionID, run.ID)
	if err != nil {
		return fail(err)
	}
	return b, nil
}
func (b *sourceNativeBatch) StartedAt() time.Time { return b.started }
func (b *sourceNativeBatch) Deadline() time.Time  { return b.started.Add(nativePilotLife) }
func (b *sourceNativeBatch) validate(ctx context.Context, c sourceNativeChallenge, p sourceNativePair) error {
	now := b.owner.engine.store.Now()
	for role, v := range map[string]sourceNativeProof{"reader": p.Reader, "collector": p.Collector} {
		observed := time.UnixMilli(v.ObservedAtMS)
		if v.Source != b.owner.source || v.Domain != "native-browser" || v.Recipe != b.owner.recipe || v.Nonce != c.Nonce || v.ProfileScope != c.ProfileScope || v.Generation != c.Generation || v.Role != role || v.OwnerID == "" || v.ProfileID == "" || !v.Authenticated || !validXDailyProfileScope(v.AccountKey) || !validXDailyProfileScope(v.SessionRevision) || v.ObservedAtMS <= 0 || observed.After(now) || now.Sub(observed) > nativePilotAge {
			return sourceNativeFail(nil, "evidence", "proof")
		}
	}
	if p.Reader.OwnerID == p.Collector.OwnerID || p.Reader.ProfileID == p.Collector.ProfileID || p.Reader.AccountKey != p.Collector.AccountKey || p.Reader.SessionRevision != p.Collector.SessionRevision {
		return sourceNativeFail(nil, "evidence", "pair")
	}
	if err := b.collector.CheckOwnership(ctx, p); err != nil {
		return sourceNativeFail(err, "ownership", "changed")
	}
	return nil
}
func (b *sourceNativeBatch) verify(ctx context.Context) error {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if ctx == nil || ctx.Err() != nil || b.parent.Err() != nil || closed || !b.owner.engine.store.Now().Before(b.Deadline()) {
		return sourceNativeFail(nil, "lifetime", "deadline")
	}
	if err := b.collector.CheckOwnership(ctx, b.pair); err != nil {
		return sourceNativeFail(err, "ownership", "changed")
	}
	c := b.challenge
	var err error
	c.Nonce, err = sourceNativeNonce()
	if err != nil {
		return err
	}
	p, err := b.collector.Proof(ctx, c)
	if err != nil {
		return err
	}
	if err = b.validate(ctx, c, p); err != nil {
		return err
	}
	for _, pair := range [][2]sourceNativeProof{{b.pair.Reader, p.Reader}, {b.pair.Collector, p.Collector}} {
		if pair[0].OwnerID != pair[1].OwnerID || pair[0].ProfileID != pair[1].ProfileID || pair[0].AccountKey != pair[1].AccountKey || pair[0].SessionRevision != pair[1].SessionRevision {
			return sourceNativeFail(nil, "evidence", "changed")
		}
	}
	return nil
}
func (b *sourceNativeBatch) Capture(ctx context.Context, command string) (domain.Run, error) {
	run, err := captureSourceDailyPermit(ctx, b.owner.engine, b.owner.source, b.run, b.permit, command, b.verify, b.collector.Capture)
	if err != nil {
		return run, sourceNativeFail(err, "capture", "unavailable")
	}
	return run, nil
}
func (b *sourceNativeBatch) Close(ctx context.Context) error {
	b.once.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		var cleanup, revoke error
		if b.collector != nil {
			cleanup = b.collector.Close(ctx)
		}
		if b.permit != nil {
			_, revoke = b.owner.engine.store.RevokeManualPilot(ctx, b.permit, string(b.owner.source)+"-revoke:"+b.run.ID)
		}
		b.closeErr = errors.Join(cleanup, revoke)
		b.owner.mu.Lock()
		if b.closeErr != nil {
			b.owner.failed = true
		} else if b.owner.batch == b {
			b.owner.batch = nil
		}
		b.owner.mu.Unlock()
	})
	return b.closeErr
}
func (o *sourceNativeOwner) Close(ctx context.Context) error {
	o.mu.Lock()
	o.closed = true
	b := o.batch
	o.mu.Unlock()
	var batchErr error
	if b != nil {
		batchErr = b.Close(ctx)
	}
	return errors.Join(batchErr, o.transport.Close(ctx))
}

// Shared Store fencing is independent of native versus synthetic evidence.
// The provider must revalidate its own evidence around the one-use claim.
func captureSourceDailyPermit(ctx context.Context, e *Engine, source domain.Source, run domain.Run, permit *store.ManualPilotPermit, command string, verify func(context.Context) error, capture func(context.Context, map[string]any) (domain.Observation, error)) (domain.Run, error) {
	if err := verify(ctx); err != nil {
		return domain.Run{}, err
	}
	claim, err := e.store.ClaimManualPilot(ctx, permit, command, string(source)+"-claim:"+command)
	if err != nil {
		return domain.Run{}, err
	}
	if claim.Handle == nil || claim.Command == nil {
		return domain.Run{}, errors.New("daily source claim unavailable")
	}
	observation, err := capture(ctx, claim.Command.Payload)
	if err != nil {
		return domain.Run{}, err
	}
	if err = verify(ctx); err != nil {
		return domain.Run{}, err
	}
	result, err := e.store.CommitManualPilotResult(ctx, claim.Handle, string(source)+"-result:"+command, observation)
	if err != nil {
		return domain.Run{}, err
	}
	if result != store.Committed {
		return domain.Run{}, errors.New("daily source commit unavailable")
	}
	return e.afterAcceptedObservation(ctx, run)
}
