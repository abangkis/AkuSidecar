// Package activationguard keeps operational maintenance admission separate from
// saved profile permission. Clearing a hold never supplies account/session proof.
package activationguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sync"
)

const Schema = "aku.native-derivation-hold.v1"
const FileSuffix = ".native-derivation-hold.json"

var ErrState = errors.New("native derivation maintenance state unavailable")
var activationID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type state struct {
	Schema       string   `json:"schema"`
	ActivationID string   `json:"activationId"`
	Sources      []string `json:"sources"`
}

type Guard struct {
	mu      sync.Mutex
	path    string
	current state
}

func read(path string) (state, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return state{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2048 {
		return state{}, ErrState
	}
	file, err := os.Open(path)
	if err != nil {
		return state{}, ErrState
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, 2049))
	if err != nil || len(raw) > 2048 {
		return state{}, ErrState
	}
	// Ambiguous duplicate keys must not silently replace the held sources.
	scan := json.NewDecoder(bytes.NewReader(raw))
	if token, err := scan.Token(); err != nil || token != json.Delim('{') {
		return state{}, ErrState
	}
	keys := map[string]bool{}
	for scan.More() {
		token, err := scan.Token()
		if err != nil {
			return state{}, ErrState
		}
		key, ok := token.(string)
		if !ok || keys[key] {
			return state{}, ErrState
		}
		keys[key] = true
		var skip json.RawMessage
		if scan.Decode(&skip) != nil {
			return state{}, ErrState
		}
	}
	if _, err := scan.Token(); err != nil {
		return state{}, ErrState
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value state
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF ||
		value.Schema != Schema || !activationID.MatchString(value.ActivationID) || value.Sources == nil || len(value.Sources) > 2 {
		return state{}, ErrState
	}
	seen := map[string]bool{}
	for _, source := range value.Sources {
		if (source != "x" && source != "instagram") || seen[source] {
			return state{}, ErrState
		}
		seen[source] = true
	}
	return value, nil
}

// Missing state is the ordinary daily startup path. Invalid state fails closed.
// A hold has no automatic expiry and survives restarts until explicit release.
func Open(path string) (*Guard, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrState
	}
	value, err := read(path)
	if os.IsNotExist(err) {
		return &Guard{path: path}, nil
	}
	if err != nil {
		return nil, ErrState
	}
	return &Guard{path: path, current: value}, nil
}

func (g *Guard) Held(source string) bool {
	if g == nil {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Contains(g.current.Sources, source)
}

// Release is called only by an explicitly acknowledged source Start action.
// Preserve the other source's hold and reject stale activation generations.
func (g *Guard) Release(source string) error {
	if g == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !slices.Contains(g.current.Sources, source) {
		return nil
	}
	actual, err := read(g.path)
	if err != nil || actual.ActivationID != g.current.ActivationID || !slices.Equal(actual.Sources, g.current.Sources) {
		return ErrState
	}
	next := actual
	next.Sources = make([]string, 0, len(actual.Sources)-1)
	for _, item := range actual.Sources {
		if item != source {
			next.Sources = append(next.Sources, item)
		}
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return ErrState
	}
	file, err := os.CreateTemp(filepath.Dir(g.path), ".native-hold-write-*")
	if err != nil {
		return ErrState
	}
	name := file.Name()
	defer os.Remove(name)
	_, writeErr := file.Write(raw)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil || os.Rename(name, g.path) != nil {
		return ErrState
	}
	g.current = next
	return nil
}
