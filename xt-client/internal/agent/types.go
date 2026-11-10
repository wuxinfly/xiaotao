package agent

import "time"

const (
	StatusIdle      = "idle"
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

type Project struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault,omitempty"`
	Saved     bool   `json:"saved,omitempty"`
	// Removed marks sessions whose project entry was deleted: they keep their
	// working directory but are grouped under the recent list instead.
	Removed bool `json:"removed,omitempty"`
}

type SavedProject struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Message struct {
	ToolCalls  []map[string]any        `json:"toolCalls,omitempty"`
	ID         string                  `json:"id"`
	Role       string                  `json:"role"`
	Content    string                  `json:"content"`
	Images     []ImageAttachment       `json:"images,omitempty"`
	ToolCallID string                  `json:"toolCallId,omitempty"`
	ToolName   string                  `json:"toolName,omitempty"`
	RunID      string                  `json:"runId,omitempty"`
	References []ConversationReference `json:"references,omitempty"`
	CreatedAt  time.Time               `json:"createdAt"`
}

type ImageAttachment struct {
	ID        string    `json:"id"`
	MimeType  string    `json:"mimeType"`
	Size      int64     `json:"size"`
	URL       string    `json:"url"`
	Width     int       `json:"width,omitempty"`
	Height    int       `json:"height,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type Activity struct {
	ID        string     `json:"id"`
	RunID     string     `json:"runId,omitempty"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Detail    string     `json:"detail,omitempty"`
	Tool      string     `json:"tool,omitempty"`
	Input     string     `json:"input,omitempty"`
	Output    string     `json:"output,omitempty"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// ThoughtStep is a readable checkpoint in one user turn. It deliberately
// records what XiaoTao understood and decided, not private model reasoning.
type ThoughtStep struct {
	ID        string    `json:"id"`
	RunID     string    `json:"runId"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

type RunStats struct {
	Phase         string     `json:"phase,omitempty"`
	RunID         string     `json:"runId"`
	UserMessageID string     `json:"userMessageId"`
	StartedAt     time.Time  `json:"startedAt"`
	EndedAt       *time.Time `json:"endedAt,omitempty"`
	FilesRead     int        `json:"filesRead"`
	FilesWritten  int        `json:"filesWritten"`
	ToolCalls     int        `json:"toolCalls"`
	ModelRequests int        `json:"modelRequests,omitempty"`
	OutputTokens  int64      `json:"outputTokens,omitempty"`
	OutputSpeed   float64    `json:"outputSpeed,omitempty"`
	// InputTokens accumulates prompt tokens reported by the upstream usage
	// block; InputKnown stays false when no request reported usage.
	InputTokens int64 `json:"inputTokens,omitempty"`
	InputKnown  bool  `json:"inputKnown,omitempty"`
	// CachedTokens accumulates prompt cache hits reported by upstream usage.
	CachedTokens int64 `json:"cachedTokens,omitempty"`
	CacheKnown   bool  `json:"cacheKnown,omitempty"`
	// FirstTokenMs is the delay from the first model request of this run to
	// its first visible token. It stays 0 until that first token arrives.
	FirstTokenMs int64 `json:"firstTokenMs,omitempty"`
	// LiveTokens is the token count of the run while it is still streaming:
	// finished requests contribute their reported usage, the in-flight
	// request contributes an estimate of what it has produced so far.
	LiveTokens int64 `json:"liveTokens,omitempty"`
	// LiveSpeed is the rolling output speed of the request currently
	// streaming, in tokens per second.
	LiveSpeed     float64           `json:"liveSpeed,omitempty"`
	ContextTokens int64             `json:"contextTokens"`
	ContextWindow int64             `json:"contextWindow,omitempty"`
	Breakdown     *ContextBreakdown `json:"breakdown,omitempty"`
	// firstRequestAt is when the first model request of this run started.
	// It is process-local bookkeeping and never leaves the service.
	firstRequestAt time.Time
}

type Session struct {
	ID            string          `json:"id"`
	Project       Project         `json:"project"`
	Title         string          `json:"title"`
	Model         string          `json:"model"`
	ModelSettings *ModelSettings  `json:"modelSettings,omitempty"`
	Status        string          `json:"status"`
	Messages      []Message       `json:"messages"`
	Activity      []Activity      `json:"activity"`
	Thoughts      []ThoughtStep   `json:"thoughts"`
	RunStats      []RunStats      `json:"runStats"`
	Error         string          `json:"error,omitempty"`
	Summary       *ContextSummary `json:"summary,omitempty"`
	// HasImages lets the composer enforce image capability even when the
	// image message is outside the bounded history page.
	HasImages bool      `json:"hasImages,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Options struct {
	ProjectPath   string         `json:"projectPath"`
	ProjectName   string         `json:"projectName"`
	Model         string         `json:"model"`
	ModelSettings *ModelSettings `json:"modelSettings,omitempty"`
}

type ModelOption struct {
	SourceID          string   `json:"sourceId,omitempty"`
	SourceLabel       string   `json:"sourceLabel,omitempty"`
	UpstreamModelID   string   `json:"upstreamModelId,omitempty"`
	MaxOutputTokens   int64    `json:"maxOutputTokens,omitempty"`
	PriceFactor       *float64 `json:"priceFactor,omitempty"`
	IsFree            *bool    `json:"isFree,omitempty"`
	SupportsImages    bool     `json:"supportsImages,omitempty"`
	SupportsReasoning bool     `json:"supportsReasoning,omitempty"`
	ReasoningEfforts  []string `json:"reasoningEfforts,omitempty"`
	ContextWindow     int64    `json:"contextWindow,omitempty"`
	Model             string   `json:"model"`
	Label             string   `json:"label"`
}

// ModelSettings is snapshotted into a session and carried through each model
// request. ContextWindow is also the threshold for later context compaction.
type ModelSettings struct {
	ContextWindow   int64  `json:"contextWindow,omitempty"`
	MaxOutputTokens int64  `json:"maxOutputTokens,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
}

type ConversationReference struct {
	ID              string    `json:"id"`
	SourceSessionID string    `json:"sourceSessionId"`
	Title           string    `json:"title"`
	ProjectName     string    `json:"projectName,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	LastPosition    int64     `json:"lastPosition"`
	Truncated       bool      `json:"truncated,omitempty"`
	SourceAvailable bool      `json:"sourceAvailable,omitempty"`
	HasNewContent   bool      `json:"hasNewContent,omitempty"`
	SourceUpdatedAt time.Time `json:"sourceUpdatedAt"`
	CreatedAt       time.Time `json:"createdAt"`
}

type ConversationReferenceCandidate struct {
	SessionID string    `json:"sessionId"`
	Title     string    `json:"title"`
	Project   Project   `json:"project"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Event struct {
	Approval  *Approval    `json:"approval,omitempty"`
	RunID     string       `json:"runId,omitempty"`
	MessageID string       `json:"messageId,omitempty"`
	Type      string       `json:"type"`
	SessionID string       `json:"sessionId,omitempty"`
	Message   *Message     `json:"message,omitempty"`
	Activity  *Activity    `json:"activity,omitempty"`
	Thought   *ThoughtStep `json:"thought,omitempty"`
	RunStats  *RunStats    `json:"runStats,omitempty"`
	Status    string       `json:"status,omitempty"`
	Error     string       `json:"error,omitempty"`
}
