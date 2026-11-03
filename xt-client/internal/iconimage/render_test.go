package iconimage

import (
	"image"
	"image/color"
	"testing"
)

func TestRenderFitsArtworkAndPreservesTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 20; y < 80; y++ {
		for x := 25; x < 75; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: 240, A: 128})
		}
	}
	for _, size := range []int{16, 20, 24, 32, 48, 256} {
		got := Render(src, size)
		center := got.NRGBAAt(size/2, size/2)
		if center.R < 239 || center.R > 240 || center.A != 128 {
			t.Fatalf("size %d: straight alpha changed: %v", size, center)
		}
		if got.NRGBAAt(0, 0).A != 0 {
			t.Fatalf("size %d: transparent corner lost", size)
		}
		if got.NRGBAAt(size/2, size/8).A == 0 {
			t.Fatalf("size %d: artwork still has excessive padding", size)
		}
	}
}
