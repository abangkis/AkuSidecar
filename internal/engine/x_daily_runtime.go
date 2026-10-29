package engine

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// Private trusted composition supplies these ports. The process reader and
// its authorization outlive successful batches; every collector is disposable.
type sourceDailyOwner interface {
	BeginBatch(context.Context, context.Context, uint64, domain.Run) (xDailyNativeBatch, error)
	PrepareReader(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error)
	CheckReader(context.Context) error
	Close(context.Context) error
}
type sourceDailyBatch interface {
	StartedAt() time.Time
	Deadline() time.Time
	Capture(context.Context, string) (domain.Run, error)
	Close(context.Context) error
}

// Compatibility aliases keep the existing X provider and API contract unchanged.
type xDailyNativeOwner = sourceDailyOwner
type xDailyNativeBatch = sourceDailyBatch

type XDailyStartRequest struct {
	AcknowledgeLimitations        bool `json:"acknowledgeLimitations"`
	AllowProcessSessionDerivation bool `json:"allowProcessSessionDerivation"`
}

func (e *Engine) configureXDailyFactory(parent context.Context, factory func(context.Context, context.Context) (xDailyNativeOwner, error), profileScopes ...string) error {
	return e.dailyController(domain.SourceX).configure(parent, factory, profileScopes...)
}

func (e *Engine) StartXDaily(ctx context.Context, request XDailyStartRequest) error {
	_, err := e.startXDaily(ctx, request, nil)
	return err
}

// startXDaily checks a restore's read revision in the same critical section
// that claims startup. A newer explicit decision supersedes the restore.
func (e *Engine) startXDaily(ctx context.Context, request XDailyStartRequest, restoreRevision *uint64) (bool, error) {
	return e.dailyController(domain.SourceX).start(ctx, request, restoreRevision)
}

// RevokeXDailyPermission persists explicit opt-out before ending the current
// process grant. Ordinary shutdown intentionally leaves the preference intact.
func (e *Engine) RevokeXDailyPermission(ctx context.Context) error {
	return e.dailyController(domain.SourceX).revoke(ctx)
}

func (e *Engine) installXDailyOwner(parent context.Context, owner xDailyNativeOwner) error {
	return e.dailyController(domain.SourceX).install(parent, owner)
}

func (e *Engine) xDailyReady() bool {
	return e.dailyController(domain.SourceX).ready()
}

func (e *Engine) prepareXDailyReader(ctx context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
	return e.dailyController(domain.SourceX).prepareReader(ctx, req, marker)
}

func (e *Engine) queueXDailyBatch(run domain.Run, payload map[string]any) error {
	return e.dailyController(domain.SourceX).queue(run, payload)
}

func xDailyClosedFailureCode(err error) string {
	var carrier interface{ dailyFailureCode() string }
	if errors.As(err, &carrier) {
		return carrier.dailyFailureCode()
	}
	return ""
}

func xDailyProfileScope(root, directory, rootID, directoryID string) string {
	hash := sha256.New()
	for _, value := range []string{
		"aku.x.daily-profile.v1",
		strings.ToLower(filepath.Clean(root)),
		strings.ToLower(filepath.Clean(directory)),
		strings.ToLower(rootID),
		strings.ToLower(directoryID),
	} {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(value)))
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func validXDailyProfileScope(profileScope string) bool {
	if len(profileScope) != 64 {
		return false
	}
	for _, char := range profileScope {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func (e *Engine) stopXDaily(ctx context.Context) error {
	return e.dailyController(domain.SourceX).stop(ctx)
}

func (e *Engine) closeXDaily(ctx context.Context) error {
	return e.dailyController(domain.SourceX).close(ctx)
}
