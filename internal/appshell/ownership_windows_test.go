//go:build windows

package appshell

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestCreateWindowIconAcceptsAkuBrowserPNG(t *testing.T) {
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
	handle, err := createWindowIcon(encoded.Bytes(), 128)
	if err != nil {
		t.Fatal(err)
	}
	destroyWindowIcon(handle)
}
