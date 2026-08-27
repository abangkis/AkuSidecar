package credentials

import (
	"errors"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/credentialstore"
)

type memoryStore struct {
	values map[credentialstore.Reference]string
}

func (store *memoryStore) Get(reference credentialstore.Reference) (string, error) {
	value, ok := store.values[reference]
	if !ok {
		return "", credentialstore.ErrNotFound
	}
	return value, nil
}

func (store *memoryStore) Put(reference credentialstore.Reference, value string) error {
	store.values[reference] = value
	return nil
}

func (store *memoryStore) Delete(reference credentialstore.Reference) error {
	delete(store.values, reference)
	return nil
}

type staticResolver string

func (resolver staticResolver) Resolve(string) (string, error) { return string(resolver), nil }

func TestManagerPrefersSecureStore(t *testing.T) {
	store := &memoryStore{values: map[credentialstore.Reference]string{"gemini.primary": "secure-key"}}
	manager := NewManager(store, staticResolver("development-key"))
	value, err := manager.Resolve("gemini.primary")
	if err != nil || value != "secure-key" {
		t.Fatalf("Resolve()=%q, %v", value, err)
	}
}

func TestManagerFallsBackAfterSecureStoreMiss(t *testing.T) {
	manager := NewManager(&memoryStore{values: map[credentialstore.Reference]string{}}, staticResolver("development-key"))
	value, err := manager.Resolve("gemini.primary")
	if err != nil || value != "development-key" {
		t.Fatalf("Resolve()=%q, %v", value, err)
	}
}

func TestManagerStatusDistinguishesSecureStoreFromDevelopmentFallback(t *testing.T) {
	secure := NewManager(&memoryStore{values: map[credentialstore.Reference]string{"gemini.primary": "secure-key"}}, staticResolver("development-key"))
	if status := secure.Status("gemini.primary"); !status.Available || !status.Secure || status.Source != SourceSecureStore {
		t.Fatalf("secure status=%+v", status)
	}

	fallback := NewManager(&memoryStore{values: map[credentialstore.Reference]string{}}, staticResolver("development-key"))
	if status := fallback.Status("gemini.primary"); !status.Available || status.Secure || status.Source != SourceDevelopmentFallback {
		t.Fatalf("fallback status=%+v", status)
	}
}

func TestManagerStatusReportsMissingCredential(t *testing.T) {
	manager := NewManager(&memoryStore{values: map[credentialstore.Reference]string{}}, nil)
	if status := manager.Status("gemini.primary"); status.Available || status.Secure || status.Source != SourceMissing {
		t.Fatalf("missing status=%+v", status)
	}
}

func TestManagerWritesOnlyToSecureStore(t *testing.T) {
	store := &memoryStore{values: map[credentialstore.Reference]string{}}
	manager := NewManager(store, nil)
	if err := manager.Put("gemini.primary", "  secret-value  "); err != nil {
		t.Fatal(err)
	}
	if got := store.values["gemini.primary"]; got != "secret-value" {
		t.Fatalf("stored value=%q", got)
	}
}

func TestManagerDoesNotLeakSecretInWriteError(t *testing.T) {
	manager := NewManager(failingStore{}, nil)
	const secret = "do-not-leak-this"
	if err := manager.Put("gemini.primary", secret); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe error=%v", err)
	}
}

type failingStore struct{}

func (failingStore) Get(credentialstore.Reference) (string, error) {
	return "", errors.New("read failed")
}
func (failingStore) Put(credentialstore.Reference, string) error { return errors.New("write failed") }
func (failingStore) Delete(credentialstore.Reference) error      { return errors.New("delete failed") }
