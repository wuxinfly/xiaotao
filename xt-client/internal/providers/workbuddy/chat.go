package workbuddy

import (
	"context"

	"xiaotao/xt-client/internal/providers"
)

type Collector struct{}

type httpStatusError struct {
	status  int
	message string
}

func (e *httpStatusError) Error() string { return e.message }

func statusError(status int, message string) error {
	return &httpStatusError{status: status, message: message}
}

func (Collector) Collect(ctx context.Context, cred *providers.Credential, model string, body map[string]any) (map[string]any, error) {
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
	return readSSE(ctx, resp.Body, func(event string, data []byte) error {
		return dispatchChatEvent(model, event, data, ev)
	})
}
