//go:build windows

package credentialstore

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric         = 1
	credentialPersistLocalMachine = 2
	maxCredentialBlobBytes        = 2560
)

var (
	advapi32        = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW   = advapi32.NewProc("CredReadW")
	procCredWriteW  = advapi32.NewProc("CredWriteW")
	procCredDeleteW = advapi32.NewProc("CredDeleteW")
	procCredFree    = advapi32.NewProc("CredFree")
)

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsStore struct {
	namespace string
}

// OpenOS opens the current user's Windows Credential Manager namespace.
func OpenOS(namespace string) (Store, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("%w: namespace is required", ErrUnavailable)
	}
	return &windowsStore{namespace: namespace}, nil
}

func (store *windowsStore) target(reference Reference) (*uint16, error) {
	parsed, err := ParseReference(reference.String())
	if err != nil {
		return nil, err
	}
	return windows.UTF16PtrFromString(store.namespace + ":" + parsed.String())
}

func (store *windowsStore) Get(reference Reference) (string, error) {
	target, err := store.target(reference)
	if err != nil {
		return "", fmt.Errorf("credential target is invalid: %w", err)
	}
	var raw *windowsCredential
	result, _, callErr := procCredReadW.Call(
		uintptr(unsafe.Pointer(target)),
		credentialTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&raw)),
	)
	if result == 0 {
		if errors.Is(callErr, syscall.Errno(windows.ERROR_NOT_FOUND)) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read Windows credential: %w", callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(raw)))
	if raw == nil || raw.CredentialBlob == nil || raw.CredentialBlobSize == 0 {
		return "", ErrNotFound
	}
	value := unsafe.Slice(raw.CredentialBlob, int(raw.CredentialBlobSize))
	return string(value), nil
}

func (store *windowsStore) Put(reference Reference, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("credential value is required")
	}
	blob := []byte(value)
	defer func() {
		for index := range blob {
			blob[index] = 0
		}
	}()
	if len(blob) > maxCredentialBlobBytes {
		return fmt.Errorf("credential value exceeds %d bytes", maxCredentialBlobBytes)
	}
	target, err := store.target(reference)
	if err != nil {
		return fmt.Errorf("credential target is invalid: %w", err)
	}
	username, err := windows.UTF16PtrFromString(store.namespace)
	if err != nil {
		return fmt.Errorf("credential username is invalid: %w", err)
	}
	credential := windowsCredential{
		Type:               credentialTypeGeneric,
		TargetName:         target,
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credentialPersistLocalMachine,
		UserName:           username,
	}
	result, _, callErr := procCredWriteW.Call(uintptr(unsafe.Pointer(&credential)), 0)
	if result == 0 {
		return fmt.Errorf("write Windows credential: %w", callErr)
	}
	return nil
}

func (store *windowsStore) Delete(reference Reference) error {
	target, err := store.target(reference)
	if err != nil {
		return fmt.Errorf("credential target is invalid: %w", err)
	}
	result, _, callErr := procCredDeleteW.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0)
	if result == 0 && !errors.Is(callErr, syscall.Errno(windows.ERROR_NOT_FOUND)) {
		return fmt.Errorf("delete Windows credential: %w", callErr)
	}
	return nil
}
