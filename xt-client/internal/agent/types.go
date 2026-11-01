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
	Path string `json:"path"`
	Name string `json:"name"`
}

type Message struct {
	ID         string    `json:"id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	ToolCallID string    `json:"toolCallId,omitempty"`
	ToolName   string    `json:"toolName,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Activity struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Detail    string     `json:"detail,omitempty"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

type Session struct {
	ID        string     `json:"id"`
	Project   Project    `json:"project"`
	Title     string     `json:"title"`
	Model     string     `json:"model"`
	Status    string     `json:"status"`
	Messages  []Message  `json:"messages"`
	Activity  []Activity `json:"activity"`
	Error     string     `json:"error,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type Options struct {
	ProjectPath string `json:"projectPath"`
	ProjectName string `json:"projectName"`
	Model       string `json:"model"`
}

type ModelOption struct {
	Model string `json:"model"`
	Label string `json:"label"`
}

type Event struct {
	Type      string    `json:"type"`
	SessionID string    `json:"sessionId,omitempty"`
	Message   *Message  `json:"message,omitempty"`
	Activity  *Activity `json:"activity,omitempty"`
	Status    string    `json:"status,omitempty"`
	Error     string    `json:"error,omitempty"`
}
