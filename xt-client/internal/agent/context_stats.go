package agent

import (
	"time"
	"math"
	"encoding/json"
	"fmt"
	"strings"
)

// ContextBreakdown classifies what occupies the model's context window for the
// upcoming request. All token fields use the same byte-to-token metric so their
// sum equals TotalTokens.
type ContextBreakdown struct {
	SystemTokens      int64 `json:"systemTokens"`
	ToolsTokens       int64 `json:"toolsTokens"`
	MessagesTokens    int64 `json:"messagesTokens"`
	ToolResultsTokens int64 `json:"toolResultsTokens"`
	ReferenceTokens   int64 `json:"referenceTokens"`
	ImageTokens       int64 `json:"imageTokens"`
	HasImages         bool  `json:"hasImages"`
	TotalTokens       int64 `json:"totalTokens"`
}

// estimatedImageTokens approximates what one inline image costs in the
// upstream prompt. Vision encoders price images by pixel area, not by the size
// of the base64 payload, so a large PNG and a thumbnail of the same image cost
// roughly the same. Counting a flat allowance per image keeps the context
// meter from treating encoded bytes as if they were prompt text.
const estimatedImageTokens = 1500

func bytesToTokens(b int64) int64 {
	if b <= 0 {
		return 0
	}
	return (b + 2) / 3
}

// inspectContentParts inspects text bytes and image count from message content
// regardless of whether it is a string, a multipart []map[string]any, or []any.
func inspectContentParts(value any) (textBytes int64, imgCount int) {
	switch content := value.(type) {
	case string:
		return int64(len(content)), 0
	case []any:
		for _, part := range content {
			if item, ok := part.(map[string]any); ok {
				if item["type"] == "image_url" {
					imgCount++
					continue
				}
				textBytes += estimateContentBytes(item["text"])
			}
		}
		return textBytes, imgCount
	case []map[string]any:
		for _, item := range content {
			if item["type"] == "image_url" {
				imgCount++
				continue
			}
			textBytes += estimateContentBytes(item["text"])
		}
		return textBytes, imgCount
	default:
		if value != nil {
			return int64(len(fmt.Sprint(value))), 0
		}
		return 0, 0
	}
}

// calculateContextBreakdown provides an itemized estimate of what is being sent
// to the upstream model, including system instructions, tools schema, dialogue
// messages, tool calls/results, references, and images.
func calculateContextBreakdown(messages []map[string]any, tools []map[string]any) ContextBreakdown {
	var breakdown ContextBreakdown
	var systemBytes int64
	var messagesBytes int64
	var toolResultsBytes int64
	var referenceBytes int64
	var totalImages int

	for _, tool := range tools {
		if raw, err := json.Marshal(tool); err == nil {
			breakdown.ToolsTokens += bytesToTokens(int64(len(raw)))
		}
	}

	for _, message := range messages {
		role, _ := message["role"].(string)
		switch role {
		case "system", "developer":
			b := estimateContentBytes(message["content"])
			systemBytes += b
		case "tool":
			b := estimateContentBytes(message["content"])
			if name, ok := message["name"].(string); ok {
				b += int64(len(name))
			}
			toolResultsBytes += b
		case "user":
			contentStr, isStr := message["content"].(string)
			if isStr && strings.HasPrefix(contentStr, "[引用历史对话：") {
				referenceBytes += int64(len(contentStr))
				continue
			}
			tb, imgs := inspectContentParts(message["content"])
			messagesBytes += tb
			totalImages += imgs
		case "assistant":
			tb, imgs := inspectContentParts(message["content"])
			messagesBytes += tb
			totalImages += imgs
			// Account for assistant tool calls (function names and JSON arguments).
			var toolCallBytes int64
			if calls, ok := message["tool_calls"].([]map[string]any); ok {
				for _, c := range calls {
					if fn, ok := c["function"].(map[string]any); ok {
						toolCallBytes += int64(len(fmt.Sprint(fn["name"])))
						toolCallBytes += int64(len(fmt.Sprint(fn["arguments"])))
					}
				}
			} else if callsAny, ok := message["tool_calls"].([]any); ok {
				for _, c := range callsAny {
					if cm, ok := c.(map[string]any); ok {
						if fn, ok := cm["function"].(map[string]any); ok {
							toolCallBytes += int64(len(fmt.Sprint(fn["name"])))
							toolCallBytes += int64(len(fmt.Sprint(fn["arguments"])))
						}
					}
				}
			}
			toolResultsBytes += toolCallBytes
		default:
			tb, imgs := inspectContentParts(message["content"])
			messagesBytes += tb
			totalImages += imgs
		}
	}

	breakdown.SystemTokens = bytesToTokens(systemBytes)
	breakdown.MessagesTokens = bytesToTokens(messagesBytes)
	breakdown.ToolResultsTokens = bytesToTokens(toolResultsBytes)
	breakdown.ReferenceTokens = bytesToTokens(referenceBytes)
	if totalImages > 0 {
		breakdown.HasImages = true
		breakdown.ImageTokens = int64(totalImages) * estimatedImageTokens
	}

	breakdown.TotalTokens = breakdown.SystemTokens + breakdown.ToolsTokens +
		breakdown.MessagesTokens + breakdown.ToolResultsTokens +
		breakdown.ReferenceTokens + breakdown.ImageTokens

	return breakdown
}

func estimateContextTokens(messages []map[string]any) int64 {
	return calculateContextBreakdown(messages, nil).TotalTokens
}

func estimateContentBytes(value any) int64 {
	switch content := value.(type) {
	case string:
		return int64(len(content))
	case []any:
		var total int64
		for _, part := range content {
			if item, ok := part.(map[string]any); ok {
				if item["type"] == "image_url" {
					total += estimatedImageTokens * 3
					continue
				}
				total += estimateContentBytes(item["text"])
			}
		}
		return total
	case []map[string]any:
		var total int64
		for _, item := range content {
			if item["type"] == "image_url" {
				total += estimatedImageTokens * 3
				continue
			}
			total += estimateContentBytes(item["text"])
		}
		return total
	default:
		if value != nil {
			return int64(len(fmt.Sprint(content)))
		}
		return 0
	}
}

func effectiveContextWindow(settings *ModelSettings, catalogWindow int64) int64 {
	if settings != nil && settings.ContextWindow > 0 {
		return settings.ContextWindow
	}
	return catalogWindow
}

func derefModelSettings(settings *ModelSettings) ModelSettings {
	if settings == nil {
		return ModelSettings{}
	}
	return *settings
}


// liveStatsInterval throttles run_stats updates while a model response streams.
const liveStatsInterval = 200 * time.Millisecond

// updateLiveOutput keeps the run statistics usable while a response is still
// streaming: finished requests contribute their reported usage and the
// in-flight request contributes an estimate of what it produced so far.
func updateLiveOutput(stats *RunStats, completedTokens int64, requestChars int, requestFirstOutputAt, now time.Time) {
	if stats == nil {
		return
	}
	liveTokens := int64(requestChars) / 3
	stats.LiveTokens = completedTokens + liveTokens
	if requestFirstOutputAt.IsZero() {
		stats.LiveSpeed = 0
	}
	if !requestFirstOutputAt.IsZero() {
		if elapsed := now.Sub(requestFirstOutputAt).Seconds(); elapsed > 0 {
			stats.LiveSpeed = math.Round(float64(liveTokens)/elapsed*10) / 10
		}
	}
}

func recordCompletionStats(stats *RunStats, result *completion, outputSeconds *float64) {
	if stats == nil || result == nil || outputSeconds == nil {
		return
	}
	stats.OutputTokens += result.OutputTokens
	if result.InputTokens != nil {
		stats.InputTokens += *result.InputTokens
		stats.InputKnown = true
	}
	if result.CachedTokens != nil {
		stats.CachedTokens += *result.CachedTokens
		stats.CacheKnown = true
	}
	if stats.FirstTokenMs == 0 && result.FirstOutputAt != nil && !stats.firstRequestAt.IsZero() {
		if elapsed := result.FirstOutputAt.Sub(stats.firstRequestAt).Milliseconds(); elapsed > 0 {
			stats.FirstTokenMs = elapsed
		}
	}
	if result.FirstOutputAt != nil && !result.RequestEndedAt.IsZero() {
		if elapsed := result.RequestEndedAt.Sub(*result.FirstOutputAt).Seconds(); elapsed >= 0.05 {
			*outputSeconds += elapsed
		}
	}
	if stats.OutputTokens > 0 && *outputSeconds > 0 {
		stats.OutputSpeed = math.Round(float64(stats.OutputTokens) / *outputSeconds * 10) / 10
	}
}