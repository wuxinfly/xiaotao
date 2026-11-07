package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxImageBytes       = 5 << 20
	maxImagesPerMessage = 4
	maxImagesTotalBytes = 12 << 20
)

type ImageInput struct {
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
}

func (s *Service) saveImages(inputs []ImageInput, now time.Time) ([]ImageAttachment, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	if len(inputs) > maxImagesPerMessage {
		return nil, fmt.Errorf("每条消息最多添加 %d 张图片", maxImagesPerMessage)
	}
	out := make([]ImageAttachment, 0, len(inputs))
	var total int64
	for _, input := range inputs {
		raw, mimeType, err := decodeImageInput(input)
		if err != nil {
			return nil, err
		}
		total += int64(len(raw))
		if total > maxImagesTotalBytes {
			return nil, fmt.Errorf("图片合计不能超过 12 MiB")
		}
		width, height, err := imageSize(raw, mimeType, input.Width, input.Height)
		if err != nil {
			return nil, err
		}
		if width <= 0 || height <= 0 || int64(width)*int64(height) > 40_000_000 {
			return nil, fmt.Errorf("图片尺寸无效或过大")
		}
		sum := sha256.Sum256(raw)
		id := hex.EncodeToString(sum[:12])
		ext := extensionForImage(mimeType)
		dir := filepath.Join(s.dataDir, "attachments")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		name := id + ext
		path := filepath.Join(dir, name)
		if err = os.WriteFile(path, raw, 0600); err != nil {
			return nil, err
		}
		out = append(out, ImageAttachment{
			ID: id, MimeType: mimeType, Size: int64(len(raw)),
			URL: "/api/agent/attachments/" + name, Width: width, Height: height,
			CreatedAt: now,
		})
	}
	return out, nil
}

func imageSize(raw []byte, mimeType string, declaredWidth, declaredHeight int) (int, int, error) {
	if mimeType == "image/webp" {
		// The standard library has no WebP decoder. Keep the browser's
		// dimensions optional rather than rejecting a valid pasted WebP.
		if declaredWidth > 0 && declaredHeight > 0 && int64(declaredWidth)*int64(declaredHeight) <= 40_000_000 {
			return declaredWidth, declaredHeight, nil
		}
		return 0, 0, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return 0, 0, fmt.Errorf("无法识别图片内容")
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40_000_000 {
		return 0, 0, fmt.Errorf("图片尺寸无效或过大")
	}
	return config.Width, config.Height, nil
}

func decodeImageInput(input ImageInput) ([]byte, string, error) {
	data := strings.TrimSpace(input.Data)
	if comma := strings.IndexByte(data, ','); strings.HasPrefix(data, "data:") && comma >= 0 {
		data = data[comma+1:]
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", fmt.Errorf("图片数据无效")
	}
	if len(raw) == 0 {
		return nil, "", fmt.Errorf("图片内容为空")
	}
	if len(raw) > maxImageBytes {
		return nil, "", fmt.Errorf("单张图片不能超过 5 MiB")
	}
	detected := detectImageMime(raw)
	if detected == "" {
		return nil, "", fmt.Errorf("仅支持 PNG、JPEG、GIF 和 WebP 图片")
	}
	declared := strings.ToLower(strings.TrimSpace(input.MimeType))
	if declared != "" && declared != detected {
		return nil, "", fmt.Errorf("图片格式与声明不一致")
	}
	return raw, detected, nil
}

func detectImageMime(raw []byte) string {
	if len(raw) >= 8 && bytes.Equal(raw[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return "image/png"
	}
	if len(raw) >= 3 && raw[0] == 0xff && raw[1] == 0xd8 && raw[2] == 0xff {
		return "image/jpeg"
	}
	if len(raw) >= 6 && (string(raw[:6]) == "GIF87a" || string(raw[:6]) == "GIF89a") {
		return "image/gif"
	}
	if len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		return "image/webp"
	}
	return ""
}

func extensionForImage(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".png"
	}
}

func (s *Service) AttachmentPath(name string) (string, error) {
	if name == "" || name != filepath.Base(name) {
		return "", fmt.Errorf("附件不存在")
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".gif", ".webp":
	default:
		return "", fmt.Errorf("附件不存在")
	}
	path := filepath.Join(s.dataDir, "attachments", name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("附件不存在")
	}
	return path, nil
}

func (s *Service) imageDataURLs(images []ImageAttachment) ([]string, error) {
	out := make([]string, 0, len(images))
	for _, image := range images {
		path, err := s.AttachmentPath(filepath.Base(image.URL))
		if err != nil {
			return nil, fmt.Errorf("图片附件缺失: %w", err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		out = append(out, "data:"+image.MimeType+";base64,"+base64.StdEncoding.EncodeToString(raw))
	}
	return out, nil
}
