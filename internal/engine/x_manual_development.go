package engine

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// Selection validates and pins isolated resources before ordinary configuration
// or SQLite access. Preparing/attaching it does not start an owner or derive a session.
type XManualDevelopmentSelection interface {
	ValidateConfig(config.Config) error
	OpenDatabase(domain.Settings) (*store.Store, error)
	Attach(context.Context, *Engine) error
	UIExtensionPath() string
	Close() error
}
