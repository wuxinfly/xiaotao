package workbuddy

import (
	"context"
	"io"
	"time"

	"xiaotao/xt-client/internal/providers"
)

type Collector struct{}

const (
	idleTimeout  = 300 * time.Second
	totalTimeout = 30 * time.Minute
)

func statusError(status int, message string) error {
	return &providers.UpstreamError{Status: status, Message: message}
}

func (Collector) Collect(ctx context.Context, cred *providers.Credential, model string, body map[string]any) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()
	body["model"] = model
	body["stream"] = true
	if _, ok := body["stream_options"]; !ok {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	payload, err := postChat(ctx, "", cred, body)
	if err != nil {
		return nil, err
	}
	return collectUpstream(model, payload)
}

type Streamer struct{}

func (Streamer) Stream(ctx context.Context, cred *providers.Credential, model string, body map[string]any, ev *providers.ChatEvents) error {
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()
	body["model"] = model
	body["stream"] = true
	if _, ok := body["stream_options"]; !ok {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	resp, err := streamChat(ctx, "", cred, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return readSSE(ctx, newIdleTimeoutReader(resp.Body, idleTimeout), func(event string, data []byte) error {
		return dispatchChatEvent(model, event, data, ev)
	})
}

// idleTimeoutReader errors if no data arrives within the timeout window.
type idleTimeoutReader struct {
	r        io.Reader
	timeout  time.Duration
	lastData time.Time
}

func newIdleTimeoutReader(r io.Reader, timeout time.Duration) *idleTimeoutReader {
	return &idleTimeoutReader{r: r, timeout: timeout, lastData: time.Now()}
}

func (i *idleTimeoutReader) Read(p []byte) (int, error) {
	if time.Since(i.lastData) > i.timeout {
		return 0, context.DeadlineExceeded
	}
	n, err := i.r.Read(p)
	if n > 0 {
		i.lastData = time.Now()
	}
	return n, err
}
