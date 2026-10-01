package collection

import (
	"context"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type quietFixtureBackend struct{ calls int }

func (b *quietFixtureBackend) Capture(context.Context, domain.Source, map[string]any) (domain.Observation, error) {
	b.calls++
	return domain.Observation{}, errors.New("fixture unavailable")
}

func TestQuietBackendDoesNotCrossBrowserGenerations(t *testing.T) {
	m, err := captureruntime.New(proc("browser"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Terminate()
	c := NewCoordinator(m, func(context.Context, string, uint64) (captureruntime.Process, error) { return proc("browser"), nil }, func() error { return nil })
	b := &quietFixtureBackend{}
	c.SetBrowserCollector(1, b)
	if !c.BrowserCollectorAvailable("x") || c.BrowserCollectorAvailable("linkedin") {
		t.Fatal("incorrect source capability")
	}
	_, _ = c.Capture(context.Background(), "x", nil)
	if b.calls != 1 {
		t.Fatal("browser backend not called")
	}
	if err := m.Replace(context.Background(), func(context.Context, uint64) (captureruntime.Process, error) { return proc("browser"), nil }); err != nil {
		t.Fatal(err)
	}
	if c.BrowserCollectorAvailable("x") {
		t.Fatal("stale backend available")
	}
	if _, err := c.Capture(context.Background(), "x", nil); err == nil {
		t.Fatal("stale backend captured")
	}
	if b.calls != 1 {
		t.Fatal("old backend used by new owner")
	}
	c.SetBrowserCollector(2, &quietFixtureBackend{})
	if !c.Status().QuietAvailable {
		t.Fatal("new browser backend unavailable")
	}
}
