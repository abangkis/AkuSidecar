//go:build windows

package appshell

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type vsFixedFileInfo struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
}

var (
	modversion                 = windows.NewLazySystemDLL("version.dll")
	procGetFileVersionInfoSize = modversion.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfo     = modversion.NewProc("GetFileVersionInfoW")
	procVerQueryValue          = modversion.NewProc("VerQueryValueW")
)

func platformVersion(path string) (string, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", fmt.Errorf("encode executable path: %w", err)
	}
	size, _, _ := procGetFileVersionInfoSize.Call(uintptr(unsafe.Pointer(pathPtr)), 0)
	if size == 0 {
		return "", fmt.Errorf("executable carries no version resource")
	}
	data := make([]byte, size)
	infoPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	result, _, _ := procGetFileVersionInfo.Call(
		uintptr(unsafe.Pointer(infoPtr)),
		0,
		size,
		uintptr(unsafe.Pointer(&data[0])),
	)
	if result == 0 {
		return "", fmt.Errorf("read version resource: %w", syscall.GetLastError())
	}
	subBlock, err := windows.UTF16PtrFromString("\\")
	if err != nil {
		return "", err
	}
	var buffer unsafe.Pointer
	var bufferLen uint32
	result, _, _ = procVerQueryValue.Call(
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(unsafe.Pointer(subBlock)),
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bufferLen)),
	)
	if result == 0 || buffer == nil || uint64(bufferLen) < uint64(unsafe.Sizeof(vsFixedFileInfo{})) {
		return "", fmt.Errorf("version resource carries no fixed file info")
	}
	info := (*vsFixedFileInfo)(buffer)
	version := fmt.Sprintf(
		"%d.%d.%d.%d",
		info.FileVersionMS>>16&0xFFFF,
		info.FileVersionMS&0xFFFF,
		info.FileVersionLS>>16&0xFFFF,
		info.FileVersionLS&0xFFFF,
	)
	if versionPattern.MatchString(version) == false || version == "0.0.0.0" {
		return "", fmt.Errorf("version resource value %q is not a recognizable browser version", version)
	}
	return version, nil
}
