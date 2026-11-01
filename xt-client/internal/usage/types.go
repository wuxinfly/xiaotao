package usage

import "time"

const (
	StatusPending     = "pending"     // 等待响应
	StatusStreaming   = "streaming"   // 正在输出
	StatusCompleted   = "completed"   // 已完成
	StatusFailed      = "failed"      // 失败
	StatusInterrupted = "interrupted" // 中断
	StatusCanceled    = "canceled"    // 已取消
)

// Record represents a single request log stored in SQLite.
type Record struct {
	RequestID        string     `json:"requestId"`
	BatchID          string     `json:"batchId"`
	Channel          string     `json:"channel"`
	Model            string     `json:"model"`
	AccountID        string     `json:"accountId"`
	ActionTag        string     `json:"actionTag"`
	ActionTitle      string     `json:"actionTitle"`
	CreatedAt        time.Time  `json:"createdAt"`
	FirstTokenAt     *time.Time `json:"firstTokenAt,omitempty"`
	EndedAt          *time.Time `json:"endedAt,omitempty"`
	Status           string     `json:"status"`
	PromptTokens     *int64     `json:"promptTokens,omitempty"`
	CompletionTokens *int64     `json:"completionTokens,omitempty"`
	CachedTokens     *int64     `json:"cachedTokens,omitempty"`
	ErrorSummary     string     `json:"errorSummary,omitempty"`
}

// DailyUsage represents aggregated stats for one calendar day (Asia/Shanghai).
type DailyUsage struct {
	Date                 string `json:"date"` // YYYY-MM-DD
	PromptTokens         int64  `json:"promptTokens"`
	CompletionTokens     int64  `json:"completionTokens"`
	CachedTokens         int64  `json:"cachedTokens"`
	TotalRequests        int64  `json:"totalRequests"`
	CompletedRequests    int64  `json:"completedRequests"`
	FailedRequests       int64  `json:"failedRequests"`
	InterruptedRequests  int64  `json:"interruptedRequests"`
	MissingUsageRequests int64  `json:"missingUsageRequests"`
}

func (d *DailyUsage) TotalTokens() int64 {
	return d.PromptTokens + d.CompletionTokens
}

// TokenMetrics carries detailed token metrics.
type TokenMetrics struct {
	Prompt     *int64 `json:"prompt"`
	Completion *int64 `json:"completion"`
	Cached     *int64 `json:"cached"`
	Total      *int64 `json:"total"`
}

// RequestSummary is the view-model sent to the dashboard UI.
type RequestSummary struct {
	ID                   string       `json:"id"`
	Channel              string       `json:"channel"`
	ChannelLabel         string       `json:"channelLabel"`
	Model                string       `json:"model"`
	AccountID            string       `json:"accountId"`
	ActionTag            string       `json:"actionTag"`
	ActionTitle          string       `json:"actionTitle"`
	Status               string       `json:"status"` // pending, streaming, completed, failed, interrupted, canceled
	StatusText           string       `json:"statusText"`
	StartedAt            int64        `json:"startedAt"`              // unix ms
	FirstTokenAt         *int64       `json:"firstTokenAt,omitempty"` // unix ms
	EndedAt              *int64       `json:"endedAt,omitempty"`      // unix ms
	DurationMs           int64        `json:"durationMs"`
	FirstTokenDurationMs *int64       `json:"firstTokenDurationMs,omitempty"`
	Tokens               TokenMetrics `json:"tokens"`
	SpeedTokPerSec       *float64     `json:"speedTokPerSec,omitempty"`
	ErrorSummary         string       `json:"errorSummary,omitempty"`
}

// SessionStats represents this launch's in-memory token totals.
type SessionStats struct {
	TotalTokens      int64 `json:"totalTokens"`
	PromptTokens     int64 `json:"promptTokens"`
	CompletionTokens int64 `json:"completionTokens"`
	CachedTokens     int64 `json:"cachedTokens"`
}

// CumulativeStats represents historical totals across all days.
type CumulativeStats struct {
	TotalTokens      int64 `json:"totalTokens"`
	PromptTokens     int64 `json:"promptTokens"`
	CompletionTokens int64 `json:"completionTokens"`
	CachedTokens     int64 `json:"cachedTokens"`
	TotalRequests    int64 `json:"totalRequests"`
}

// HeatmapDay represents one day's data in the heatmap.
type HeatmapDay struct {
	TotalTokens      int64 `json:"totalTokens"`
	PromptTokens     int64 `json:"promptTokens"`
	CompletionTokens int64 `json:"completionTokens"`
	CachedTokens     int64 `json:"cachedTokens"`
	TotalRequests    int64 `json:"totalRequests"`
	HasMissingUsage  bool  `json:"hasMissingUsage"`
}

// HeatmapData is the annual daily aggregation.
type HeatmapData struct {
	StartDate         string                 `json:"startDate"`         // YYYY-MM-DD
	EndDate           string                 `json:"endDate"`           // YYYY-MM-DD
	TrackingStartDate string                 `json:"trackingStartDate"` // YYYY-MM-DD
	Days              map[string]*HeatmapDay `json:"days"`
}

// StorageStatus indicates DB health.
type StorageStatus struct {
	Status  string `json:"status"` // "healthy" | "degraded" | "error"
	Message string `json:"message,omitempty"`
}

// LaunchInfo contains runtime metadata for the current launch.
type LaunchInfo struct {
	StartedAt     int64  `json:"startedAt"` // unix seconds
	UptimeSeconds int64  `json:"uptimeSeconds"`
	BatchID       string `json:"batchId"`
}

// DashboardSnapshot is the complete payload for GET /api/dashboard and SSE initial state.
type DashboardSnapshot struct {
	Version        string           `json:"version"`
	Launch         LaunchInfo       `json:"launch"`
	CurrentSession SessionStats     `json:"currentSession"`
	Today          DailyUsage       `json:"today"`
	Cumulative     CumulativeStats  `json:"cumulative"`
	RecentRequests []RequestSummary `json:"recentRequests"`
	ActiveCount    int              `json:"activeCount"`
	ActiveRequests []RequestSummary `json:"activeRequests"`
	Heatmap        HeatmapData      `json:"heatmap"`
	Storage        StorageStatus    `json:"storage"`
	VersionSeq     int64            `json:"versionSeq"`
}
