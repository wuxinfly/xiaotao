package workbuddy

import (
	"context"
	"fmt"
	"io"
	"time"

	"xiaotao/xt-client/internal/providers"
)

type Collector struct{}

const (
	idleTimeout  = 300 * time.Second
	totalTimeout = 30 * time.Minute
)

var errStreamIdleTimeout = fmt.Errorf("WorkBuddy stream idle timeout: %w", context.DeadlineExceeded)

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
	return streamWithTimeouts(ctx, cred, model, body, ev, idleTimeout, totalTimeout)
}

func streamWithTimeouts(ctx context.Context, cred *providers.Credential, model string, body map[string]any, ev *providers.ChatEvents, idle, total time.Duration) error {
	ctx, cancelTotal := context.WithTimeout(ctx, total)
	defer cancelTotal()
	requestCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	body["model"] = model
	body["stream"] = true
	if _, ok := body["stream_options"]; !ok {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	resp, err := streamChat(requestCtx, "", cred, body)
	if err != nil {
		if cause := context.Cause(requestCtx); cause != nil {
			return cause
		}
		return err
	}
	defer resp.Body.Close()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	reader := newIdleTimeoutReader(resp.Body, idle, func() {
		cancel(errStreamIdleTimeout)
	})
	defer reader.Stop()
	err = readSSE(requestCtx, reader, func(event string, data []byte) error {
		return dispatchChatEvent(model, event, data, ev)
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cause := context.Cause(requestCtx); cause != nil {
			return cause
		}
	}
	return err
}

// idleTimeoutReader cancels the request if no data arrives within the timeout window.
type idleTimeoutReader struct {
	r       io.Reader
	timeout time.Duration
	timer   *time.Timer
}

func newIdleTimeoutReader(r io.Reader, timeout time.Duration, onTimeout func()) *idleTimeoutReader {
	reader := &idleTimeoutReader{r: r, timeout: timeout}
	reader.timer = time.AfterFunc(timeout, onTimeout)
	return reader
}

func (i *idleTimeoutReader) Read(p []byte) (int, error) {
	n, err := i.r.Read(p)
	if n > 0 {
		i.timer.Reset(i.timeout)
	}
	return n, err
}

func (i *idleTimeoutReader) Stop() {
	i.timer.Stop()
}
