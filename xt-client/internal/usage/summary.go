package usage

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func toSummary(r *Record, now time.Time) RequestSummary {
	startMs := r.CreatedAt.UnixMilli()

	var firstMs *int64
	var firstDurationMs *int64
	if r.FirstTokenAt != nil {
		f := r.FirstTokenAt.UnixMilli()
		firstMs = &f
		diff := f - startMs
		if diff >= 0 {
			firstDurationMs = &diff
		}
	}

	var endMs *int64
	var totalDurationMs int64
	if r.EndedAt != nil {
		e := r.EndedAt.UnixMilli()
		endMs = &e
		totalDurationMs = e - startMs
	} else {
		totalDurationMs = now.UnixMilli() - startMs
	}
	if totalDurationMs < 0 {
		totalDurationMs = 0
	}

	// Speed calculation: completed output tokens ÷ elapsed time since first token
	var speed *float64
	if r.CompletionTokens != nil && *r.CompletionTokens > 0 && r.FirstTokenAt != nil {
		end := now
		if r.EndedAt != nil {
			end = *r.EndedAt
		}
		sec := end.Sub(*r.FirstTokenAt).Seconds()
		if sec >= 0.05 {
			toks := float64(*r.CompletionTokens) / sec
			rounded := math.Round(toks*10) / 10
			speed = &rounded
		}
	}

	var totalTokens *int64
	if r.PromptTokens != nil || r.CompletionTokens != nil {
		var sum int64
		if r.PromptTokens != nil {
			sum += *r.PromptTokens
		}
		if r.CompletionTokens != nil {
			sum += *r.CompletionTokens
		}
		totalTokens = &sum
	}

	cleanModel := strings.TrimPrefix(r.Model, "trae/")

	channel := r.Channel
	if channel == "" {
		channel = "trae"
	}
	channelLabel := strings.ToUpper(channel)
	if strings.ToLower(channel) == "trae" {
		channelLabel = "Trae"
	}

	tag := r.ActionTag
	if tag == "" {
		tag = "用户"
	}
	title := r.ActionTitle
	if title == "" {
		title = "对话交互"
	}

	return RequestSummary{
		ID:                   r.RequestID,
		Channel:              channel,
		ChannelLabel:         channelLabel,
		Model:                cleanModel,
		AccountID:            r.AccountID,
		SourceLabel:          r.SourceLabel,
		ActionTag:            tag,
		ActionTitle:          title,
		Status:               r.Status,
		StatusText:           formatStatusText(r.Status, r.CreatedAt, now),
		StartedAt:            startMs,
		FirstTokenAt:         firstMs,
		EndedAt:              endMs,
		DurationMs:           totalDurationMs,
		FirstTokenDurationMs: firstDurationMs,
		Tokens: TokenMetrics{
			Prompt:     r.PromptTokens,
			Completion: r.CompletionTokens,
			Cached:     r.CachedTokens,
			Total:      totalTokens,
		},
		SpeedTokPerSec: speed,
		ErrorSummary:   r.ErrorSummary,
	}
}

func formatStatusText(status string, created, now time.Time) string {
	switch status {
	case StatusPending:
		sec := int(now.Sub(created).Seconds())
		if sec <= 0 {
			return "等待响应"
		}
		return fmt.Sprintf("等待响应 · %d秒", sec)
	case StatusStreaming:
		return "正在输出"
	case StatusCompleted:
		return "已完成"
	case StatusFailed:
		return "失败"
	case StatusInterrupted:
		return "中断"
	case StatusCanceled:
		return "已取消"
	default:
		return status
	}
}
