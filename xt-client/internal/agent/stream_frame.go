package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Model stream frames keep usage separate from choices: an empty choices
// array reports usage without implying a finish reason.
type modelStreamToolPart struct {
	ID       string `json:"id"`
	Index    *int   `json:"index"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}
type modelStreamChunk struct {
	Choices []struct {
		Delta        modelStreamDelta `json:"delta"`
		FinishReason *string          `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Usage struct {
		CompletionTokens    int64  `json:"completion_tokens"`
		PromptTokens        *int64 `json:"prompt_tokens"`
		CachedTokens        *int64 `json:"cached_tokens"`
		PromptTokensDetails *struct {
			CachedTokens *int64 `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
}
type modelStreamCall struct{ id, name, args string }

type modelStreamDelta struct {
	Content          string                `json:"content"`
	ReasoningContent string                `json:"reasoning_content"`
	ToolCalls        []modelStreamToolPart  `json:"tool_calls"`
}

// Tool metadata marks first output; only arguments contribute to estimates.
func (d modelStreamDelta) outputProgress() (int, bool) {
	chars := len(d.Content) + len(d.ReasoningContent)
	hasOutput := chars > 0
	for _, part := range d.ToolCalls {
		chars += len(part.Function.Arguments)
		hasOutput = hasOutput || part.ID != "" || part.Function.Name != "" || part.Function.Arguments != ""
	}
	return chars, hasOutput
}

func finishModelStream(result *completion, calls []*modelStreamCall) (*completion, error) {
	ids := map[string]bool{}
	for _, call := range calls {
		args := strings.TrimSpace(call.args)
		if args == "" {
			args = "{}"
		}
		if call.id == "" || call.name == "" || !json.Valid([]byte(args)) || ids[call.id] {
			return nil, fmt.Errorf("模型工具调用不完整或无效")
		}
		ids[call.id] = true
		result.ToolCalls = append(result.ToolCalls, toolCall{ID: call.id, Name: call.name, Arguments: json.RawMessage(args)})
	}
	return result, nil
}
