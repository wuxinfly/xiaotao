package agent

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 80, G: 200, B: 180, A: 255})
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func TestSendImagePersistsAndProjectsMultimodalContent(t *testing.T) {
	data := t.TempDir()
	svc := NewService(data, "http://127.0.0.1:9999", nil, nil, nil)
	session, err := svc.Create(Options{Model: "test/vision"})
	if err != nil {
		t.Fatal(err)
	}
	raw := testPNG(t)
	images := []ImageInput{{Data: base64.StdEncoding.EncodeToString(raw), MimeType: "image/png", Width: 2, Height: 2}}
	attachments, err := svc.saveImages(images, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	history.Messages = append(history.Messages, Message{ID: "u", Role: "user", Content: "看这张图", Images: attachments, CreatedAt: time.Now()})
	if err = svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	history, err = svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Messages) != 1 || len(history.Messages[0].Images) != 1 {
		t.Fatalf("image message not persisted: %+v", history.Messages)
	}
	image := history.Messages[0].Images[0]
	if image.Width != 2 || image.Height != 2 || !strings.HasPrefix(image.URL, "/api/agent/attachments/") {
		t.Fatalf("unexpected image metadata: %+v", image)
	}
	if _, err = os.Stat(filepath.Join(data, "attachments", filepath.Base(image.URL))); err != nil {
		t.Fatal(err)
	}
	messages, err := modelHistoryWith(history.Messages, history.Messages[0].ID, svc.imageDataURLs)
	if err != nil {
		t.Fatal(err)
	}
	content, ok := messages[0]["content"].([]map[string]any)
	if !ok || len(content) != 2 || content[0]["type"] != "text" || content[1]["type"] != "image_url" {
		t.Fatalf("unexpected multimodal projection: %#v", messages[0]["content"])
	}
}

func TestSendImageRequiresModelCapability(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, func() []ModelOption {
		return []ModelOption{{Model: "test/text", Label: "Text"}}
	})
	session, err := svc.Create(Options{Model: "test/text"})
	if err != nil {
		t.Fatal(err)
	}
	raw := testPNG(t)
	err = svc.SendWithImages(t.Context(), session.ID, "", "", nil, []ImageInput{{Data: base64.StdEncoding.EncodeToString(raw), MimeType: "image/png"}}, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "未声明支持图片") {
		t.Fatalf("expected capability error, got %v", err)
	}
}

func TestSendWithHistoryImagesRequiresModelCapability(t *testing.T) {
	svc := NewService(t.TempDir(), "http://127.0.0.1:9999", nil, nil, func() []ModelOption {
		return []ModelOption{
			{Model: "test/vision", Label: "Vision", SupportsImages: true},
			{Model: "test/text", Label: "Text"},
		}
	})
	session, err := svc.Create(Options{Model: "test/vision"})
	if err != nil {
		t.Fatal(err)
	}
	raw := testPNG(t)
	attachments, err := svc.saveImages([]ImageInput{{
		Data: base64.StdEncoding.EncodeToString(raw), MimeType: "image/png",
	}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	history, err := svc.Get(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	history.Messages = append(history.Messages, Message{
		ID: "image", Role: "user", Content: "看图", Images: attachments, CreatedAt: time.Now(),
	})
	if err = svc.saveSession(history); err != nil {
		t.Fatal(err)
	}
	err = svc.SendWithImages(t.Context(), session.ID, "继续", "test/text", nil, nil, func(Event) {})
	if err == nil || !strings.Contains(err.Error(), "会话包含图片") {
		t.Fatalf("expected history image capability error, got %v", err)
	}
}
