// Command provider is a minimal model-source plugin built on the public SDK.
// It echoes the last user message and reports a static quota; it never contacts
// a network and is meant as a template and protocol smoke test.
package main

import (
	"context"
	"encoding/json"

	"xiaotao/xt-client/sdk/plugin"
)

func main() {
	server := &plugin.Server{ID: "example.echo", Handlers: map[string]plugin.Handler{
		"models/refresh": models,
		"chat/start":     chat,
		"quota/get":      quota,
	}}
	server.Main()
}

func models(context.Context, plugin.Operation) (plugin.Result, error) {
	return plugin.Result{Models: []plugin.Model{{Key: "echo-1", UpstreamID: "echo-1", Label: "Echo 1"}}}, nil
}

func chat(_ context.Context, op plugin.Operation) (plugin.Result, error) {
	reply := "echo"
	if messages, ok := op.Body["messages"].([]any); ok && len(messages) > 0 {
		if last, ok := messages[len(messages)-1].(map[string]any); ok {
			if text, ok := last["content"].(string); ok {
				reply = "echo: " + text
			}
		}
	}
	// Streaming plugins call Server.Emit("chat/delta", chunk) then return an
	// empty result. This template only implements the non-streaming shape.
	value, _ := json.Marshal(map[string]any{
		"id": "echo", "object": "chat.completion", "model": op.Model,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": reply}, "finish_reason": "stop"}},
	})
	return plugin.Result{Value: value}, nil
}

func quota(context.Context, plugin.Operation) (plugin.Result, error) {
	percent := 25.0
	value, _ := json.Marshal(map[string]any{"status": "ok", "items": []any{map[string]any{"label": "示例额度", "used": 25, "total": 100, "unit": "credit"}}})
	return plugin.Result{Value: value, Display: &plugin.Display{
		Summary: "示例额度 25 / 100 积分",
		Items:   []plugin.DisplayItem{{Label: "示例额度", Value: "25 / 100 积分", Percent: &percent}},
	}}, nil
}