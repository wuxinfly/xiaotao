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
}

type Message struct {
	ToolCalls  []map[string]any        `json:"toolCalls,omitempty"`
	ID         string                  `json:"id"`
	Role       string                  `json:"role"`
	Content    string                  `json:"content"`
	ToolCallID string                  `json:"toolCallId,omitempty"`
	ToolName   string                  `json:"toolName,omitempty"`
	RunID      string                  `json:"runId,omitempty"`
	References []ConversationReference `json:"references,omitempty"`
	CreatedAt  time.Time               `json:"createdAt"`
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
	ContextTokens int64      `json:"contextTokens"`
	ContextWindow int64      `json:"contextWindow,omitempty"`
}

type Session struct {
	ID        string        `json:"id"`
	Project   Project       `json:"project"`
	Title     string        `json:"title"`
	Model     string        `json:"model"`
	Status    string        `json:"status"`
	Messages  []Message     `json:"messages"`
	Activity  []Activity    `json:"activity"`
	Thoughts  []ThoughtStep `json:"thoughts"`
	RunStats  []RunStats    `json:"runStats"`
	Error     string        `json:"error,omitempty"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type Options struct {
	ProjectPath string `json:"projectPath"`
	ProjectName string `json:"projectName"`
	Model       string `json:"model"`
}

type ModelOption struct {
	PriceFactor *float64 `json:"priceFactor,omitempty"`
	IsFree      *bool    `json:"isFree,omitempty"`
	Model       string   `json:"model"`
	Label       string   `json:"label"`
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
