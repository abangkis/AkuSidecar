//go:build auth_session_fixture

package collection

import (
	"context"
	"errors"
)

// SyntheticExclusiveHooks is fixture-only wiring, absent from ordinary builds.
// Bind before Start. Authentication and physical profile ownership remain with
// injected fixture adapters and the existing capture Manager, respectively.
type SyntheticExclusiveHooks struct {
	BeforeReader   func(context.Context) error
	BeforeHeadless func(context.Context) error
	HeadlessReady  func(context.Context, uint64) error
	Paused         func() bool
}

func (c *Coordinator) BindSyntheticExclusive(h SyntheticExclusiveHooks) error {
	if c.fixtureHooks != nil || h.BeforeReader == nil || h.BeforeHeadless == nil || h.HeadlessReady == nil || h.Paused == nil {
		return errors.New("invalid synthetic exclusive hooks")
	}
	c.fixtureHooks = &syntheticExclusiveHooks{h.BeforeReader, h.BeforeHeadless, h.HeadlessReady, h.Paused}
	return nil
}

func (c *Coordinator) SetSyntheticHeadlessCollector(generation uint64, backend CaptureBackend) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fixtureHeadlessGeneration, c.fixtureHeadless = generation, backend
}

func (c *Coordinator) SyntheticReaderPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interactive > 0 || (c.nativeReaderGeneration != 0 && c.nativeReaderGeneration == c.owner.Snapshot().Generation)
}
