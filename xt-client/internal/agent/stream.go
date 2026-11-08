package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// readModelStream forwards visible text as it arrives and assembles complete
// tool calls before they can be executed. A truncated stream is never replayed.
func readModelStream(r io.Reader, onDelta func(string)) (*completion, error) {
	type toolPart struct {
		ID       string `json:"id"`
		Index    *int   `json:"index"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	type chunk struct {
		Choices []struct {
			Delta struct {
				Content          string     `json:"content"`
				ReasoningContent string     `json:"reasoning_content"`
				ToolCalls        []toolPart `json:"tool_calls"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
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
	type assembled struct{ id, name, args string }
	var text strings.Builder
	var reasoning strings.Builder
	var calls []*assembled
	var firstOutputAt *time.Time
	var outputTokens int64
	var inputTokens *int64
	var cachedTokens *int64
	byIndex := map[int]*assembled{}
	byID := map[string]*assembled{}
	done, finished := false, false
	scanner := bufio.NewScanner(io.LimitReader(r, 16<<20))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	var lines []string
	handle := func() error {
		if len(lines) == 0 {
			return nil
		}
		data := strings.Join(lines, "\n")
		lines = nil
		if data == "[DONE]" {
			done = true
			return nil
		}
		var frame chunk
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			return fmt.Errorf("解析模型事件失败: %w", err)
		}
		if frame.Error != nil && frame.Error.Message != "" {
			return fmt.Errorf("%s", frame.Error.Message)
		}
		for _, choice := range frame.Choices {
			if choice.FinishReason != nil {
				finished = true
			}
			if choice.Delta.ReasoningContent != "" {
				reasoning.WriteString(choice.Delta.ReasoningContent)
			}
			if choice.Delta.Content != "" {
				if firstOutputAt == nil {
					now := time.Now()
					firstOutputAt = &now
				}
				text.WriteString(choice.Delta.Content)
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
			for _, part := range choice.Delta.ToolCalls {
				var call *assembled
				if part.Index != nil {
					call = byIndex[*part.Index]
				}
				if call == nil && part.ID != "" {
					call = byID[part.ID]
				}
				if call == nil {
					call = &assembled{}
					calls = append(calls, call)
				}
				if part.Index != nil {
					byIndex[*part.Index] = call
				}
				if part.ID != "" {
					if call.id != "" && call.id != part.ID {
						return fmt.Errorf("模型工具调用 ID 不一致")
					}
					call.id = part.ID
					byID[part.ID] = call
				}
				call.name += part.Function.Name
				call.args += part.Function.Arguments
			}
		}
		if frame.Usage.CompletionTokens > 0 {
			outputTokens = frame.Usage.CompletionTokens
		}
		if frame.Usage.PromptTokens != nil {
			inputTokens = frame.Usage.PromptTokens
		}
		if frame.Usage.CachedTokens != nil {
			cachedTokens = frame.Usage.CachedTokens
		} else if frame.Usage.PromptTokensDetails != nil && frame.Usage.PromptTokensDetails.CachedTokens != nil {
			cachedTokens = frame.Usage.PromptTokensDetails.CachedTokens
		}
		return nil
	}
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := handle(); err != nil {
				return nil, err
			}
			if done {
				break
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			lines = append(lines, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取模型流失败: %w", err)
	}
	if !done && len(lines) > 0 {
		if err := handle(); err != nil {
			return nil, err
		}
	}
	if !done || !finished {
		return nil, fmt.Errorf("模型流意外中断")
	}
	result := &completion{
		Content:       text.String(),
		Reasoning:     reasoning.String(),
		OutputTokens:  outputTokens,
		InputTokens:   inputTokens,
		CachedTokens:  cachedTokens,
		FirstOutputAt: firstOutputAt,
	}
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
