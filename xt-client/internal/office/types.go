// Package office implements the project-scoped Agent Office: one task is
// assigned to one executor session, and the office records real status,
// transcript references, queued user messages and artifacts.
package office

import (
	"context"
	"errors"
	"time"

	"xiaotao/xt-client/internal/agent"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusWaiting   = "waiting"
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

const (
	MessageQueued    = "queued"
	MessageDelivered = "delivered"
	MessageFailed    = "failed"
)

const (
	executorKind   = "xiaotao"
	executorName   = "小涛"
	executorRole   = "executor"
	coordinatorID  = "xiaotao"
	maxTaskGoal    = 20000
	maxMessageText = 20000
)

var (
	ErrClosed           = errors.New("办公室服务已关闭")
	ErrTaskNotFound     = errors.New("任务不存在")
	ErrArtifactNotFound = errors.New("产物不存在")
	ErrNotRunning       = errors.New("任务当前不在运行")
	ErrMemberBusy       = errors.New("成员已有未结束任务，请先结束当前任务")
)

// Executor is the subset of the agent kernel the office needs. It is an
// interface so orchestration can be tested without model calls.
type Executor interface {
	CreateSession(options agent.Options) (*agent.Session, error)
	SetMember(sessionID string, member *agent.MemberIdentity) error
	RunTurn(ctx context.Context, sessionID, text, model string, settings *agent.ModelSettings, emit func(agent.Event)) error
	Cancel(sessionID string) bool
	History(sessionID, before string) (*agent.HistoryPage, error)
	ResolveApproval(sessionID, approvalID string, approved bool) error
	ListSummaries() ([]*agent.Session, error)
	AvailableModels() []agent.ModelOption
	AccessMode() string
}

type Task struct {
	ID              string               `json:"id"`
	ProjectPath     string               `json:"projectPath"`
	ProjectName     string               `json:"projectName"`
	Title           string               `json:"title"`
	Goal            string               `json:"goal"`
	Status          string               `json:"status"`
	Error           string               `json:"error,omitempty"`
	CancelRequested bool                 `json:"cancelRequested,omitempty"`
	WaitingReason   string               `json:"waitingReason,omitempty"`
	Approval        *TaskApproval        `json:"approval,omitempty"`
	Executor        ExecutorInfo         `json:"executor"`
	SessionID       string               `json:"sessionId,omitempty"`
	MemberID        string               `json:"memberId,omitempty"`
	ChatSessionID   string               `json:"chatSessionId,omitempty"`
	Discussion      string               `json:"discussion,omitempty"`
	RunID           string               `json:"runId,omitempty"`
	ModelSettings   *agent.ModelSettings `json:"modelSettings,omitempty"`
	Stats           TaskStats            `json:"stats"`
	LatestActivity  *TaskActivity        `json:"latestActivity,omitempty"`
	QueuedMessages  int                  `json:"queuedMessages,omitempty"`
	CreatedAt       time.Time            `json:"createdAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	StartedAt       *time.Time           `json:"startedAt,omitempty"`
	EndedAt         *time.Time           `json:"endedAt,omitempty"`
}

type ExecutorInfo struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	Model      string `json:"model"`
	ModelLabel string `json:"modelLabel,omitempty"`
	SessionID  string `json:"sessionId,omitempty"`
}

type TaskApproval struct {
	ID     string `json:"id"`
	Tool   string `json:"tool"`
	Detail string `json:"detail"`
}

type TaskActivity struct {
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Detail    string     `json:"detail,omitempty"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"startedAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
}

// TaskStats carries only counters the kernel actually measured. InputKnown
// and CacheKnown stay false when upstream reported no usage; ContextTokens is
// a local estimate and must be labelled as such in the UI.
type TaskStats struct {
	ToolCalls     int   `json:"toolCalls"`
	FilesRead     int   `json:"filesRead"`
	FilesWritten  int   `json:"filesWritten"`
	ModelRequests int   `json:"modelRequests,omitempty"`
	OutputTokens  int64 `json:"outputTokens,omitempty"`
	InputTokens   int64 `json:"inputTokens,omitempty"`
	InputKnown    bool  `json:"inputKnown"`
	CachedTokens  int64 `json:"cachedTokens,omitempty"`
	CacheKnown    bool  `json:"cacheKnown"`
	ContextTokens int64 `json:"contextTokens,omitempty"`
	ContextWindow int64 `json:"contextWindow,omitempty"`
	FirstTokenMs  int64 `json:"firstTokenMs,omitempty"`
	LiveTokens    int64 `json:"liveTokens,omitempty"`
	DurationMs    int64 `json:"durationMs,omitempty"`
}

type TaskMessage struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"taskId"`
	Content     string     `json:"content"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	DeliveredAt *time.Time `json:"deliveredAt,omitempty"`
}

type Artifact struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"taskId"`
	TaskTitle   string    `json:"taskTitle"`
	ProjectPath string    `json:"projectPath"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256,omitempty"`
	Tool        string    `json:"tool"`
	RunID       string    `json:"runId,omitempty"`
	ActivityID  string    `json:"activityId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ArtifactDetail struct {
	Artifact  *Artifact `json:"artifact"`
	Available bool      `json:"available"`
	Binary    bool      `json:"binary,omitempty"`
	Truncated bool      `json:"truncated,omitempty"`
	Content   string    `json:"content,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type ProjectInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

type Member struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Role            string           `json:"role"`
	Status          string           `json:"status"`
	Model           string           `json:"model,omitempty"`
	TaskID          string           `json:"taskId,omitempty"`
	TaskTitle       string           `json:"taskTitle,omitempty"`
	TaskStatus      string           `json:"taskStatus,omitempty"`
	TodayTasks      int              `json:"todayTasks"`
	LatestActivity  *TaskActivity    `json:"latestActivity,omitempty"`
	Responsibility  string           `json:"responsibility,omitempty"`
	Capabilities    []string         `json:"capabilities,omitempty"`
	Appearance      *Appearance      `json:"appearance,omitempty"`
	ModelPreference *ModelPreference `json:"modelPreference,omitempty"`
}

// Capabilities states what this build really supports. The UI renders these
// verbatim so a missing ability is never presented as available.
type Capabilities struct {
	MidRunMessages       bool     `json:"midRunMessages"`
	QueuedMessages       bool     `json:"queuedMessages"`
	Approval             bool     `json:"approval"`
	RestartResume        bool     `json:"restartResume"`
	SameProjectIsolation bool     `json:"sameProjectIsolation"`
	CommandArtifacts     bool     `json:"commandArtifacts"`
	ArtifactTools        []string `json:"artifactTools"`
	Notes                []string `json:"notes"`
}

type Snapshot struct {
	Project      ProjectInfo  `json:"project"`
	AccessMode   string       `json:"accessMode"`
	Tasks        []*Task      `json:"tasks"`
	Members      []Member     `json:"members"`
	Artifacts    []*Artifact  `json:"artifacts"`
	Capabilities Capabilities `json:"capabilities"`
}

type TaskDetail struct {
	Task         *Task              `json:"task"`
	History      *agent.HistoryPage `json:"history,omitempty"`
	HistoryError string             `json:"historyError,omitempty"`
	Artifacts    []*Artifact        `json:"artifacts"`
	Messages     []*TaskMessage     `json:"messages"`
}

type CreateTaskRequest struct {
	ProjectPath   string               `json:"projectPath"`
	ProjectName   string               `json:"projectName"`
	Title         string               `json:"title"`
	Goal          string               `json:"goal"`
	Model         string               `json:"model"`
	ModelSettings *agent.ModelSettings `json:"modelSettings"`
	MemberID      string               `json:"memberId,omitempty"`
	ChatSessionID string               `json:"chatSessionId,omitempty"`
}

func defaultCapabilities() Capabilities {
	return Capabilities{
		MidRunMessages:       false,
		QueuedMessages:       true,
		Approval:             true,
		RestartResume:        false,
		SameProjectIsolation: false,
		CommandArtifacts:     false,
		ArtifactTools:        []string{"write_file", "str_replace"},
		Notes: []string{
			"运行中的执行者不能接收新消息；消息会排队，并在本轮成功结束后自动送达。",
			"失败或取消后不会自动重发排队消息。",
			"产物只从 write_file / str_replace 活动识别；命令或外部工具创建的文件不会自动出现。",
			"同一项目的多个执行者共用工作目录，现有文件检查不是操作系统级隔离。",
			"服务重启后运行中任务会标记为中断失败，不会自动恢复。",
		},
	}
}

func isTerminal(status string) bool {
	return status == StatusCompleted || status == StatusFailed || status == StatusCanceled
}

func statsFromRun(run agent.RunStats) TaskStats {
	stats := TaskStats{
		ToolCalls:     run.ToolCalls,
		FilesRead:     run.FilesRead,
		FilesWritten:  run.FilesWritten,
		ModelRequests: run.ModelRequests,
		OutputTokens:  run.OutputTokens,
		InputTokens:   run.InputTokens,
		InputKnown:    run.InputKnown,
		CachedTokens:  run.CachedTokens,
		CacheKnown:    run.CacheKnown,
		ContextTokens: run.ContextTokens,
		ContextWindow: run.ContextWindow,
		FirstTokenMs:  run.FirstTokenMs,
		LiveTokens:    run.LiveTokens,
	}
	if run.EndedAt != nil {
		stats.DurationMs = run.EndedAt.Sub(run.StartedAt).Milliseconds()
	}
	return stats
}
