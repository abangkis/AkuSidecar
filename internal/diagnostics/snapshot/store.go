// Package snapshot provides independent, concurrency-safe latest-value stores.
package snapshot

import "sync"

// Cloner returns an independent copy, including any mutable nested values.
// Clone must not mutate its receiver and must be safe for concurrent calls.
type Cloner[T any] interface {
	Clone() T
}

// Store retains one value and clones it on both write and read boundaries.
// Its zero value is ready to use. A Store must not be copied after first use.
type Store[T Cloner[T]] struct {
	mu    sync.RWMutex
	value T
	set   bool
}

// Save replaces the latest value. Callers may mutate their value after Save
// returns, but must not mutate it while Clone is reading it.
func (s *Store[T]) Save(value T) {
	copy := value.Clone()
	s.mu.Lock()
	s.value = copy
	s.set = true
	s.mu.Unlock()
}

// Latest returns an independent copy and whether a value has been saved.
func (s *Store[T]) Latest() (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.set {
		var zero T
		return zero, false
	}
	return s.value.Clone(), true
}
