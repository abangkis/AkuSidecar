//go:build windows && auth_session_fixture

package readerbroker

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unsafe"
)

type FixtureNativeHostReceipt struct {
	TaskRoot, HostName, ManifestPath, ManifestSHA256, HelperPath, HelperSHA256, MainPath, MainSHA256, Pipe, RegistryPath, RegistryView, DefaultValue string
}

var fixtureQAHostPattern = regexp.MustCompile(`^com\.akubrowser\.qa_reader_[a-f0-9]{32}$`)

const fixtureQARegistryParent = `Software\Google\Chrome\NativeMessagingHosts`

func ValidateFixtureQAHostName(name string) error {
	if !fixtureQAHostPattern.MatchString(name) {
		return errors.New("QA native host name rejected")
	}
	return nil
}
func FixtureFileHash(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
func ReadFixtureNativeHostReceipt(path string) (FixtureNativeHostReceipt, error) {
	var v FixtureNativeHostReceipt
	raw, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(raw, &v)
	return v, err
}

// This validator only reads the exact QA namespace. It never writes registry
// state or consults a production host name/pipe.
func (v FixtureNativeHostReceipt) ValidateFiles(taskRoot string) error {
	if err := ValidateFixtureQAHostName(v.HostName); err != nil {
		return err
	}
	clean := filepath.Clean(taskRoot)
	if !filepath.IsAbs(clean) || !strings.EqualFold(v.TaskRoot, clean) || v.RegistryView != "Registry64" || v.RegistryPath != `HKCU\`+fixtureQARegistryParent+`\`+v.HostName || v.DefaultValue != v.ManifestPath {
		return errors.New("QA receipt root/registry scope rejected")
	}
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || !strings.EqualFold(resolved, clean) {
		return errors.New("QA receipt root resolves elsewhere")
	}
	for path, expected := range map[string]string{v.ManifestPath: v.ManifestSHA256, v.HelperPath: v.HelperSHA256, v.MainPath: v.MainSHA256} {
		if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Dir(path), clean) {
			return errors.New("QA receipt file escaped root")
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !strings.EqualFold(resolved, path) {
			return errors.New("QA receipt file resolves elsewhere")
		}
		hash, err := FixtureFileHash(path)
		if err != nil || len(expected) != 64 || hash != expected {
			return errors.New("QA receipt binary/manifest hash mismatch")
		}
	}
	if filepath.Base(v.ManifestPath) != v.HostName+".json" || !strings.EqualFold(filepath.Base(v.HelperPath), "aku-reader-broker.exe") || !strings.EqualFold(filepath.Base(v.MainPath), "akusidecar.exe") {
		return errors.New("QA receipt file identity rejected")
	}
	if err := ValidateFixturePipe(v.Pipe); err != nil {
		return err
	}
	raw, err := os.ReadFile(v.ManifestPath)
	if err != nil {
		return err
	}
	var manifest struct {
		Name, Description, Path, Type string
		AllowedOrigins                []string `json:"allowed_origins"`
	}
	if json.Unmarshal(raw, &manifest) != nil || manifest.Name != v.HostName || manifest.Description == "" || manifest.Path != v.HelperPath || manifest.Type != "stdio" || len(manifest.AllowedOrigins) != 1 || manifest.AllowedOrigins[0] != "chrome-extension://"+ExtensionID+"/" {
		return errors.New("QA manifest contract rejected")
	}
	return nil
}

func (v FixtureNativeHostReceipt) ValidateApplied(taskRoot string) error {
	if err := v.ValidateFiles(taskRoot); err != nil {
		return err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, fixtureQARegistryParent+`\`+v.HostName, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS|registry.WOW64_64KEY)
	if err != nil {
		return errors.New("QA native registration missing; Chrome startup denied")
	}
	defer key.Close()
	value, kind, err := key.GetStringValue("")
	if err != nil || kind != registry.SZ || value != v.ManifestPath {
		return errors.New("QA native registration value mismatch")
	}
	info, err := key.Stat()
	if err != nil || info.ValueCount != 1 || info.SubKeyCount != 0 {
		return errors.New("QA native registration contents changed")
	}
	raw, err := os.ReadFile(filepath.Join(taskRoot, "qa-host-applied.json"))
	if err != nil {
		return errors.New("QA applied ownership receipt missing")
	}
	var owned struct {
		Stage, RegistryPath, RegistryView, ManifestSHA256, DefaultValue, CanonicalKeySHA256, LastWrite string
		CreatedNewLeaf, HKCU32Alias                                                                    bool
		AbsentBefore                                                                                   []string
	}
	if json.Unmarshal(raw, &owned) != nil || owned.Stage != "APPLIED" || !owned.CreatedNewLeaf || owned.RegistryPath != v.RegistryPath || owned.RegistryView != v.RegistryView || owned.ManifestSHA256 != v.ManifestSHA256 || owned.DefaultValue != v.DefaultValue || len(owned.AbsentBefore) != 4 {
		return errors.New("QA applied ownership proof rejected")
	}
	seen := map[string]bool{}
	for _, scope := range owned.AbsentBefore {
		seen[scope] = true
	}
	for _, scope := range []string{"HKCU/Registry32", "HKCU/Registry64", "HKLM/Registry32", "HKLM/Registry64"} {
		if !seen[scope] {
			return errors.New("QA registry absence preflight missing")
		}
	}
	identity, err := fixtureRegistryKeyIdentity(key)
	if err != nil || identity != owned.CanonicalKeySHA256 || fixtureRegistryWriteTime(info.ModTime()) != owned.LastWrite {
		return errors.New("QA native registry ownership epoch changed")
	}
	// Refuse shadowing or a second registration for this exact new QA name.
	for _, check := range []struct {
		h    registry.Key
		view uint32
	}{{registry.CURRENT_USER, registry.WOW64_32KEY}, {registry.LOCAL_MACHINE, registry.WOW64_32KEY}, {registry.LOCAL_MACHINE, registry.WOW64_64KEY}} {
		other, err := registry.OpenKey(check.h, fixtureQARegistryParent+`\`+v.HostName, registry.QUERY_VALUE|check.view)
		if err == registry.ErrNotExist {
			continue
		}
		if err != nil {
			return errors.New("QA native registry collision check unavailable")
		}
		current, kind, valueErr := other.GetStringValue("")
		aliasIdentity, identityErr := fixtureRegistryKeyIdentity(other)
		otherInfo, infoErr := other.Stat()
		other.Close()
		// HKCU Software can be shared between views. Only the same exact value is
		// allowed there; an HKLM host is always a collision.
		if check.h != registry.CURRENT_USER || !owned.HKCU32Alias || valueErr != nil || kind != registry.SZ || current != v.ManifestPath || identityErr != nil || aliasIdentity != identity || infoErr != nil || otherInfo.ValueCount != 1 || otherInfo.SubKeyCount != 0 || fixtureRegistryWriteTime(otherInfo.ModTime()) != owned.LastWrite {
			return errors.New("QA native registry view collision")
		}
	}
	return nil
}

func fixtureRegistryWriteTime(t time.Time) string {
	ft := windows.NsecToFiletime(t.UnixNano())
	return fmt.Sprintf("%08x%08x", ft.HighDateTime, ft.LowDateTime)
}
func fixtureRegistryKeyIdentity(key registry.Key) (string, error) {
	query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryKey")
	var size uint32
	_, _, _ = query.Call(uintptr(key), 3, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size < 4 || size > 65536 {
		return "", errors.New("QA key canonical identity unavailable")
	}
	raw := make([]byte, size)
	status, _, _ := query.Call(uintptr(key), 3, uintptr(unsafe.Pointer(&raw[0])), uintptr(size), uintptr(unsafe.Pointer(&size)))
	if status != 0 {
		return "", errors.New("QA key canonical identity read failed")
	}
	length := binary.LittleEndian.Uint32(raw[:4])
	if length%2 != 0 || length > uint32(len(raw)-4) {
		return "", errors.New("QA key canonical identity invalid")
	}
	name := make([]uint16, length/2)
	for i := range name {
		name[i] = binary.LittleEndian.Uint16(raw[4+i*2 : 6+i*2])
	}
	h := sha256.Sum256([]byte(strings.ToLower(windows.UTF16ToString(name))))
	return hex.EncodeToString(h[:]), nil
}
