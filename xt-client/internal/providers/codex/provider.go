package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"xiaotao/xt-client/internal/providers"
	"xiaotao/xt-client/internal/storage"
)

type Provider struct{}

func init() {
	providers.Register(Provider{})
}

func (Provider) Info() providers.Info {
	return providers.Info{
		ID:          "codex",
		DisplayName: "Codex",
		LoginType:   "web",
		Regions:     []string{"global"},
		Billing:     "quota",
	}
}

func (Provider) ListModels(region string) []providers.ModelInfo {
	_ = region
	return DefaultModels()
}

func (Provider) FetchModels(ctx context.Context, cred *providers.Credential) ([]providers.ModelInfo, error) {
	return FetchCatalog(ctx, cred)
}

func (Provider) Refresh(ctx context.Context, cred *providers.Credential) (*providers.Credential, error) {
	return Refresh(ctx, cred)
}

func (Provider) FetchQuota(ctx context.Context, cred *providers.Credential) (*storage.QuotaSnapshot, error) {
	return FetchQuota(ctx, cred)
}

type Collector struct{}

func (Collector) Collect(ctx context.Context, cred *providers.Credential, model string, body map[string]any) (map[string]any, error) {
	upstreamBody := BuildUpstreamBody(body, model)
	resp, err := postResponses(ctx, cred, upstreamBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	reader := NewSSEReader(resp.Body)
	gate, err := GateFirstEvent(ctx, reader)
	if err != nil {
		return nil, &providers.UpstreamError{Status: 502, Message: "首事件读取失败: " + err.Error()}
	}
	if gate.AccountError {
		return nil, &providers.UpstreamError{Status: gate.StatusCode, Message: gate.ErrorMessage}
	}
	if gate.EOF {
		status := 502
		if gate.TimedOut {
			status = 504
		}
		return nil, &providers.UpstreamError{Status: status, Message: "首事件前断流或超时"}
	}

	stream := NewEventStream(gate.Buffered, reader)
	var finalResponse map[string]any

	for {
		ev, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, &providers.UpstreamError{Status: 502, Message: "响应流读取中断: " + err.Error()}
		}
		if ev.Name == "[DONE]" {
			break
		}
		if ev.Name == "response.completed" || ev.Name == "response.incomplete" {
			if r, ok := ev.Data["response"].(map[string]any); ok {
				finalResponse = r
			}
		} else if ev.Name == "response.failed" {
			if r, ok := ev.Data["response"].(map[string]any); ok {
				finalResponse = r
				break
			}
		}
	}

	if finalResponse == nil {
		return nil, &providers.UpstreamError{Status: 502, Message: "上游未返回完整 response 对象"}
	}
	return ResponsesToChatCompletion(finalResponse, model), nil
}

type Streamer struct{}

func (Streamer) Stream(ctx context.Context, cred *providers.Credential, model string, body map[string]any, ev *providers.ChatEvents) error {
	upstreamBody := BuildUpstreamBody(body, model)
	resp, err := postResponses(ctx, cred, upstreamBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	reader := NewSSEReader(resp.Body)
	gate, err := GateFirstEvent(ctx, reader)
	if err != nil {
		return &providers.UpstreamError{Status: 502, Message: "首事件读取失败: " + err.Error()}
	}
	if gate.AccountError {
		return &providers.UpstreamError{Status: gate.StatusCode, Message: gate.ErrorMessage}
	}
	if gate.EOF {
		status := 502
		if gate.TimedOut {
			status = 504
		}
		return &providers.UpstreamError{Status: status, Message: "首事件前断流或超时"}
	}

	state := NewStreamState(model)
	firstTokenEmitted := false
	stream := NewEventStream(gate.Buffered, reader)

	for {
		sseEv, err := stream.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return &providers.UpstreamError{Status: 502, Message: "流中断: " + err.Error()}
		}
		if sseEv.Name == "[DONE]" {
			break
		}

		deltas := state.Feed(sseEv.Name, sseEv.Data)
		for _, d := range deltas {
			if !firstTokenEmitted {
				firstTokenEmitted = true
				if ev.OnFirstToken != nil {
					ev.OnFirstToken()
				}
			}
			if ev.OnDelta != nil {
				ev.OnDelta(d)
			}
		}
	}

	if u := state.FinalUsage(); u != nil && ev.OnUsage != nil {
		ev.OnUsage(u)
	}
	return nil
}

func postResponses(ctx context.Context, cred *providers.Credential, upstreamBody map[string]any) (*http.Response, error) {
	if cred == nil || cred.AccessToken == "" {
		return nil, &providers.UpstreamError{Status: 401, Message: "missing access token"}
	}
	raw, err := json.Marshal(upstreamBody)
	if err != nil {
		return nil, err
	}
	url := normalizeBaseURL("") + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	accountID := cred.EnterpriseID
	sessionID := randHex(16)
	for k, v := range buildHeaders(cred.AccessToken, accountID, sessionID, true) {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Codex 上游失败: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var errPayload struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(b, &errPayload)
		msg := errPayload.Error.Message
		if msg == "" {
			msg = string(b)
		}
		return nil, &providers.UpstreamError{Status: resp.StatusCode, Message: msg}
	}
	return resp, nil
}
