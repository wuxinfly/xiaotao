// Package iconimage prepares the mascot for Windows icon slots.
package iconimage

import (
	"image"
	"image/color"
)

// Render fits the visible artwork into a square, keeping a small safety margin.
// Area sampling preserves fine details and straight alpha at tray sizes.
func Render(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	visible := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			if a > 16*257 {
				visible = visible.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	if visible.Empty() {
		return dst
	}
	side := max(visible.Dx(), visible.Dy()) + 4
	left := (visible.Min.X + visible.Max.X - side) / 2
	top := (visible.Min.Y + visible.Max.Y - side) / 2
	// Supersample the source footprint rather than selecting one nearest pixel.
	const samples = 16
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, blue, alpha uint64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := left + (x*samples+sx)*side/(size*samples)
					py := top + (y*samples+sy)*side/(size*samples)
					if !image.Pt(px, py).In(b) {
						continue
					}
					rr, gg, bb, aa := src.At(px, py).RGBA()
					r += uint64(rr)
					g += uint64(gg)
					blue += uint64(bb)
					alpha += uint64(aa)
				}
			}
			if alpha != 0 {
				dst.SetNRGBA(x, y, color.NRGBA{uint8(r * 255 / alpha), uint8(g * 255 / alpha), uint8(blue * 255 / alpha), uint8(alpha / (samples * samples * 257))})
			}
		}
	}
	return dst
}
