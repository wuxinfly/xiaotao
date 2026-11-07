package agent

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// The browser re-encodes pasted images before upload, so the server receives
// either a PNG or a JPEG. The declared mime type must match the sniffed bytes,
// otherwise the upload is rejected.
func TestSaveImagesAcceptsReencodedJPEG(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 120, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := jpeg.Encode(&raw, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}

	attachments, err := svc.saveImages([]ImageInput{{
		Data:     base64.StdEncoding.EncodeToString(raw.Bytes()),
		MimeType: "image/jpeg",
		Width:    8,
		Height:   8,
	}}, time.Now())
	if err != nil {
		t.Fatalf("JPEG upload must be accepted: %v", err)
	}
	if len(attachments) != 1 || attachments[0].MimeType != "image/jpeg" {
		t.Fatalf("unexpected attachments: %+v", attachments)
	}
}

// A re-encoded image that keeps its PNG container must still be accepted, and
// the stored extension must match the sniffed type.
func TestSaveImagesAcceptsReencodedPNG(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}

	attachments, err := svc.saveImages([]ImageInput{{
		Data:     base64.StdEncoding.EncodeToString(raw.Bytes()),
		MimeType: "image/png",
		Width:    4,
		Height:   4,
	}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].MimeType != "image/png" {
		t.Fatalf("unexpected attachments: %+v", attachments)
	}
}

// A declared type that disagrees with the bytes is still rejected: the browser
// must report the real type after re-encoding.
func TestSaveImagesRejectsMismatchedMimeType(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, nil)
	raw := testPNG(t)
	_, err := svc.saveImages([]ImageInput{{
		Data:     base64.StdEncoding.EncodeToString(raw),
		MimeType: "image/jpeg",
		Width:    2,
		Height:   2,
	}}, time.Now())
	if err == nil {
		t.Fatal("a PNG declared as JPEG must be rejected")
	}
}
