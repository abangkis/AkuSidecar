//go:build auth_session_fixture || auth_session_dev

package collection

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// ExclusiveSessionHooks is explicit development/fixture wiring, absent from ordinary builds.
// Bind before Start. Authentication and physical profile ownership remain with
// injected fixture adapters and the existing capture Manager, respectively.
type ExclusiveSessionHooks struct {
	BeforeReader   func(context.Context) error
	BeforeHeadless func(context.Context) error
	HeadlessReady  func(context.Context, uint64) error
	Paused         func() bool
}

func (c *Coordinator) BindExclusiveSession(h ExclusiveSessionHooks) error {
	if c.fixtureHooks != nil || h.BeforeReader == nil || h.BeforeHeadless == nil || h.HeadlessReady == nil || h.Paused == nil {
		return errors.New("invalid synthetic exclusive hooks")
	}
	c.fixtureHooks = &syntheticExclusiveHooks{h.BeforeReader, h.BeforeHeadless, h.HeadlessReady, h.Paused}
	return nil
}

func (c *Coordinator) SetExclusiveHeadlessCollector(generation uint64, backend CaptureBackend) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fixtureHeadlessGeneration, c.fixtureHeadless = generation, backend
}

func (c *Coordinator) ExclusiveReaderPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interactive > 0 || (c.nativeReaderGeneration != 0 && c.nativeReaderGeneration == c.owner.Snapshot().Generation)
}

func (c *Coordinator) SetDevelopmentSession(enabled bool, sources ...domain.Source) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.developmentSession = enabled
	c.developmentSources = append([]domain.Source(nil), sources...)
}

func (c *Coordinator) ExclusiveSessionBound() bool { return c.fixtureHooks != nil }
