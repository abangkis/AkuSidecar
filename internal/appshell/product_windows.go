//go:build windows

package appshell

import (
	"fmt"
	"golang.org/x/sys/windows"
	"unsafe"
)

func platformProductName(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	size, _, _ := procGetFileVersionInfoSize.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return "", fmt.Errorf("version resource unavailable")
	}
	data := make([]byte, size)
	ok, _, _ := procGetFileVersionInfo.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&data[0])))
	if ok == 0 {
		return "", fmt.Errorf("version resource read failed")
	}
	query := func(key string) (unsafe.Pointer, uint32, bool) {
		k, _ := windows.UTF16PtrFromString(key)
		var value unsafe.Pointer
		var length uint32
		ok, _, _ := procVerQueryValue.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(k)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&length)))
		return value, length, ok != 0 && value != nil
	}
	translations, length, found := query(`\VarFileInfo\Translation`)
	if !found || length < 4 || length > 1024 {
		return "", fmt.Errorf("version translations unavailable")
	}
	words := unsafe.Slice((*uint16)(translations), int(length/2))
	for i := 0; i+1 < len(words); i += 2 {
		value, n, ok := query(fmt.Sprintf(`\StringFileInfo\%04x%04x\ProductName`, words[i], words[i+1]))
		if ok && n > 0 && n < 1024 {
			return windows.UTF16ToString(unsafe.Slice((*uint16)(value), int(n))), nil
		}
	}
	return "", fmt.Errorf("product name unavailable")
}
