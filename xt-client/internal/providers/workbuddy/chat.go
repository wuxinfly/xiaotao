package workbuddy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
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

// Only replace the known leading client signatures on WorkBuddy requests.
func prepareIdentity(body map[string]any) {
	messages, ok := body["messages"].([]map[string]any)
	if !ok || len(messages) == 0 {
		return
	}
	// WorkBuddy rejects the developer role even for an ordinary prompt.
	copied := append([]map[string]any(nil), messages...)
	for i, message := range messages {
		if message["role"] == "developer" {
			clone := make(map[string]any, len(message))
			for key, value := range message {
				clone[key] = value
			}
			clone["role"] = "system"
			copied[i] = clone
		}
	}
	body["messages"] = copied
	messages = copied
	if messages[0]["role"] != "system" {
		return
	}
	content, ok := messages[0]["content"].(string)
	if !ok {
		return
	}
	head := strings.TrimLeft(content, " ")
	for _, signature := range []string{"You are an AI agent powered by DeepSeek Harness.", "You are Claude Code, Anthropic's official CLI for Claude."} {
		if strings.HasPrefix(head, signature) {
			first := make(map[string]any, len(messages[0]))
			for key, value := range messages[0] {
				first[key] = value
			}
			first["content"] = content[:len(content)-len(head)] + "[redacted: client identity]" + strings.TrimPrefix(head, signature)
			copied[0] = first
			body["messages"] = copied
			return
		}
	}
}

func prepareTools(body map[string]any) error {
	tools, ok := body["tools"].([]map[string]any)
	if !ok {
		return nil
	}
	seen := make(map[string]map[string]any)
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		name = strings.TrimSpace(name)
		if name != "" {
			if previous, exists := seen[name]; exists {
				if !reflect.DeepEqual(previous, tool) {
					return statusError(400, "WorkBuddy 工具定义冲突：同名工具 "+name+" 的定义不一致，请在客户端合并工具定义")
				}
				continue
			}
			seen[name] = tool
		}
		result = append(result, tool)
	}
	body["tools"] = result
	return nil
}

func chatHTTPError(status int, raw []byte) error {
	var payload struct {
		Code  json.RawMessage `json:"code"`
		Error struct {
			Code json.RawMessage `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &payload) == nil {
		if strings.Trim(string(payload.Code), "\"") == "11128" || strings.Trim(string(payload.Error.Code), "\"") == "11128" {
			return statusError(status, "WorkBuddy 上游拒绝了客户端渠道身份（11128）；请检查客户端及其提示词是否受该渠道支持。")
		}
	}
	return statusError(status, fmt.Sprintf("HTTP %d: %s", status, truncate(string(raw), 200)))
}

func (Collector) Collect(ctx context.Context, cred *providers.Credential, model string, body map[string]any) (map[string]any, error) {
	if err := prepareTools(body); err != nil {
		return nil, err
	}
	prepareIdentity(body)
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
	if err := prepareTools(body); err != nil {
		return err
	}
	prepareIdentity(body)
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
