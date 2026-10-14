//go:build windows && auth_session_fixture

package readerbroker

import (
	"encoding/binary"
	"encoding/json"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"
)

func TestFixtureNativeHostFilesAndUnknownRegistrationStayClosed(t *testing.T) {
	root := t.TempDir()
	host := "com.akubrowser.qa_reader_" + Secret()[:32]
	v := FixtureNativeHostReceipt{TaskRoot: root, HostName: host, ManifestPath: filepath.Join(root, host+".json"), HelperPath: filepath.Join(root, "aku-reader-broker.exe"), MainPath: filepath.Join(root, "akusidecar.exe"), Pipe: `\\.\pipe\AkuBrowser.fixture-reader.` + Secret()[:32], RegistryPath: `HKCU\` + fixtureQARegistryParent + `\` + host, RegistryView: "Registry64"}
	v.DefaultValue = v.ManifestPath
	for _, path := range []string{v.MainPath, v.HelperPath} {
		if err := os.WriteFile(path, []byte("inert file contract only"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	manifest, _ := json.Marshal(map[string]any{"name": host, "description": "Synthetic QA only", "path": v.HelperPath, "type": "stdio", "allowed_origins": []string{"chrome-extension://" + ExtensionID + "/"}})
	if err := os.WriteFile(v.ManifestPath, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	v.ManifestSHA256, _ = FixtureFileHash(v.ManifestPath)
	v.HelperSHA256, _ = FixtureFileHash(v.HelperPath)
	v.MainSHA256, _ = FixtureFileHash(v.MainPath)
	if err := v.ValidateFiles(root); err != nil {
		t.Fatal(err)
	}
	if err := v.ValidateApplied(root); err == nil || !strings.Contains(err.Error(), "registration missing") {
		t.Fatal("unregistered QA host did not stay closed", err)
	}
	for _, mutate := range []func(*FixtureNativeHostReceipt){func(x *FixtureNativeHostReceipt) { x.HostName = HostName }, func(x *FixtureNativeHostReceipt) { x.Pipe = `\\.\pipe\AkuBrowser.reader` }, func(x *FixtureNativeHostReceipt) { x.RegistryView = "Registry32" }, func(x *FixtureNativeHostReceipt) { x.MainSHA256 = strings.Repeat("0", 64) }, func(x *FixtureNativeHostReceipt) { x.TaskRoot = filepath.Dir(root) }, func(x *FixtureNativeHostReceipt) {
		x.HelperPath = filepath.Join(filepath.Dir(root), "aku-reader-broker.exe")
	}} {
		x := v
		mutate(&x)
		if x.ValidateFiles(root) == nil {
			t.Fatal("unscoped native receipt accepted")
		}
	}
	if err := os.WriteFile(v.HelperPath, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if v.ValidateFiles(root) == nil {
		t.Fatal("native helper tampering accepted")
	}
}

// Existing parent metadata only. No host leaf is created or modified. This
// checks the Windows ABI and Go timestamp representation used by the PowerShell
// applied receipt before the first authorized application.
func TestFixtureNativeHostReadOnlyRegistryIdentityABI(t *testing.T) {
	key, err := registry.OpenKey(registry.CURRENT_USER, fixtureQARegistryParent, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == registry.ErrNotExist {
		t.Skip("existing Chrome native-host parent unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer key.Close()
	identity, err := fixtureRegistryKeyIdentity(key)
	if err != nil || len(identity) != 64 {
		t.Fatal("canonical key identity unavailable", err)
	}
	info, err := key.Stat()
	if err != nil {
		t.Fatal(err)
	}
	query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryKey")
	var size uint32
	_, _, _ = query.Call(uintptr(key), 0, 0, 0, uintptr(unsafe.Pointer(&size)))
	if size < 8 || size > 65536 {
		t.Fatal("registry basic-info ABI size")
	}
	raw := make([]byte, size)
	status, _, _ := query.Call(uintptr(key), 0, uintptr(unsafe.Pointer(&raw[0])), uintptr(size), uintptr(unsafe.Pointer(&size)))
	if status != 0 {
		t.Fatal("read-only registry basic-info failed")
	}
	actual := binary.LittleEndian.Uint64(raw[:8])
	ft := windows.NsecToFiletime(info.ModTime().UnixNano())
	if actual != uint64(ft.HighDateTime)<<32|uint64(ft.LowDateTime) {
		t.Fatal("Go registry epoch is not exact Windows last-write")
	}
	t.Log("existingParentCanonicalIdentity=true exactLastWriteABI=true registryWrites=0")
}
