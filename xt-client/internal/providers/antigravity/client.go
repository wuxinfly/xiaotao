package antigravity

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	connectTimeout    = 15 * time.Second
	nonStreamTimeout  = 120 * time.Second
	streamIdleTimeout = 90 * time.Second
)

var endpoints = []string{
	"https://daily-cloudcode-pa.googleapis.com",
	"https://cloudcode-pa.googleapis.com",
}

// metadataHeaders mimic the community-verified Antigravity CLI request
// fingerprint.  These are the same values buddy-proxy ships with.
func metadataHeaders(token, model string) map[string]string {
	userAgent := "antigravity/2.0.3 win32/x64"
	if model != "" {
		userAgent += " " + model
	}
	return map[string]string{
		"Content-Type":      "application/json",
		"User-Agent":        userAgent,
		"X-Client-Name":     "antigravity",
		"X-Client-Version":  "1.110.0",
		"x-goog-api-client": "gl-node/18.18.2 fire/0.8.6 grpc/1.10.x",
		"Authorization":     "Bearer " + token,
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
	}
}

// postJSON sends a management API call (setup, onboarding).  It falls back
// from daily to prod endpoints like buddy-proxy does.
func postJSON(ctx context.Context, token, method string, body any) (map[string]any, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s encode failed: %w", method, err)
	}
	var lastErr error
	for _, base := range endpoints {
		url := base + "/v1internal:" + method
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		for k, v := range metadataHeaders(token, "") {
			req.Header.Set(k, v)
		}
		client := newHTTPClient(connectTimeout + nonStreamTimeout)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s request failed: %w", method, err)
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("%s HTTP %d: %s", method, resp.StatusCode, truncate(string(data), 300))
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			lastErr = fmt.Errorf("%s invalid JSON response", method)
			continue
		}
		return payload, nil
	}
	return nil, lastErr
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("agent-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:16])
}

// parseISO8601 accepts the subset Google returns (2026-10-02T20:28:46Z).
func parseISO8601(value string) int64 {
	layouts := []string{
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.000Z07:00",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Unix()
		}
	}
	return 0
}

func firstNumber(m map[string]any, key string) *float64 {
	if v, ok := m[key].(float64); ok {
		out := v
		return &out
	}
	return nil
}

func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, 1<<20))
}

func isEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}
