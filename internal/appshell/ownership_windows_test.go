//go:build windows

package appshell

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
)

func TestCreateWindowIconAcceptsAkuBrowserPNG(t *testing.T) {
	encoded := testAkuBrowserPNG(t)
	handle, err := createWindowIcon(encoded, 128)
	if err != nil {
		t.Fatal(err)
	}
	destroyWindowIcon(handle)
}

func TestMaterializeRelaunchIconWrapsPNGAsICO(t *testing.T) {
	pngData := testAkuBrowserPNG(t)
	path, err := materializeRelaunchIcon(pngData, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 22+len(pngData) {
		t.Fatalf("ICO size=%d, want %d", len(data), 22+len(pngData))
	}
	if reserved := binary.LittleEndian.Uint16(data[0:2]); reserved != 0 {
		t.Fatalf("ICO reserved=%d", reserved)
	}
	if iconType := binary.LittleEndian.Uint16(data[2:4]); iconType != 1 {
		t.Fatalf("ICO type=%d", iconType)
	}
	if count := binary.LittleEndian.Uint16(data[4:6]); count != 1 {
		t.Fatalf("ICO image count=%d", count)
	}
	if offset := binary.LittleEndian.Uint32(data[18:22]); offset != 22 {
		t.Fatalf("ICO image offset=%d", offset)
	}
	if !bytes.Equal(data[22:], pngData) {
		t.Fatal("ICO payload does not preserve the AkuBrowser PNG")
	}
}

func testAkuBrowserPNG(t *testing.T) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := 0; y < 128; y++ {
		for x := 0; x < 128; x++ {
			value.Set(x, y, color.RGBA{R: 24, G: 32, B: 51, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}
