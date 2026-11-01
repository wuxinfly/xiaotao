package trae

import (
	"context"
	"time"

	"xiaotao/xt-client/internal/providers"
)

func Exchange(region, refreshToken string) (*providers.Credential, error) {
	return exchangeToken(refreshToken, region)
}

func UserInfo(cred *providers.Credential) (string, string, error) {
	return userInfo(cred.AccessToken, cred.Region)
}

func JWTUserID(token string) string { return decodeJWTUserID(token) }

func CollectChat(ctx context.Context, cred *providers.Credential, machineID, deviceID, model string, body map[string]any) (*ChatResult, error) {
	return collectChat(ctx, cred, machineID, deviceID, model, body)
}

type Collector struct{}

type StreamProvider struct{}

func (StreamProvider) Stream(ctx context.Context, cred *providers.Credential, model string, body map[string]any, ev *providers.ChatEvents) error {
	firstTokenFired := false
	var upstreamErrMsg string
	err := StreamChat(ctx, cred, cred.MachineID, cred.DeviceID, model, body, &StreamEvents{
		OnFirstToken: func(time.Time) {
			if !firstTokenFired && ev.OnFirstToken != nil {
				firstTokenFired = true
				ev.OnFirstToken()
			}
		},
		OnDelta: func(delta map[string]any) {
			if ev.OnDelta != nil {
				ev.OnDelta(delta)
			}
		},
		OnError: func(_ any, msg string) {
			upstreamErrMsg = msg
		},
		OnUsage: func(usage map[string]any) {
			if ev.OnUsage != nil {
				ev.OnUsage(usage)
			}
		},
	})
	if err != nil {
		return err
	}
	if upstreamErrMsg != "" {
		return &providers.UpstreamError{Status: 502, Message: upstreamErrMsg}
	}
	return nil
}

func (Collector) Collect(ctx context.Context, cred *providers.Credential, model string, body map[string]any) (map[string]any, error) {
	result, err := collectChat(ctx, cred, cred.MachineID, cred.DeviceID, model, body)
	if err != nil {
		return nil, err
	}
	if result.ErrMsg != "" {
		return nil, &providers.UpstreamError{Status: 502, Message: result.ErrMsg}
	}
	finish := "stop"
	if len(result.Tools) > 0 {
		finish = "tool_calls"
	}
	message := map[string]any{"role": "assistant", "content": result.Content}
	if len(result.Tools) > 0 {
		message["tool_calls"] = result.Tools
	}
	return map[string]any{
		"id":      "trae-" + randHex(),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}},
		"usage":   result.Usage,
	}, nil
}
