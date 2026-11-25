package office

import (
	"context"
	"errors"

	"xiaotao/xt-client/internal/agent"
)

var (
	ErrConversationBusy = errors.New("还有员工正在执行，是否停止并关闭？")
	ErrSummaryPending   = errors.New("员工记忆正在提炼，请稍后关闭")
	ErrSummaryFailed    = errors.New("员工记忆保存失败，请重试或确认放弃")
)

type Conversation struct {
	Historical bool `json:"historical,omitempty"`
	Chat
	MemberName          string        `json:"memberName,omitempty"`
	TaskID              string        `json:"taskId,omitempty"`
	TaskTitle           string        `json:"taskTitle,omitempty"`
	Request             string        `json:"request,omitempty"`
	SourceMessageID     string        `json:"sourceMessageId,omitempty"`
	Question            string        `json:"question,omitempty"`
	CoordinatorQuestion string        `json:"coordinatorQuestion,omitempty"`
	Reply               string        `json:"reply,omitempty"`
	LatestActivity      *TaskActivity `json:"latestActivity,omitempty"`
	SummaryStatus       string        `json:"summaryStatus,omitempty"`
	SummaryError        string        `json:"summaryError,omitempty"`
	TurnID              string        `json:"-"`
	SummarySession      string        `json:"-"`
	DeferredSummaries   []summaryTurn `json:"-"`
	ProjectKey          string        `json:"-"`
	Dispatching         bool          `json:"-"`
}

type summaryTurn struct {
	TurnID    string
	SessionID string
}

func (entry *Conversation) resetSummary() {
	if entry.SummaryStatus == "failed" && entry.TurnID != "" && entry.SummarySession != "" {
		entry.DeferredSummaries = append(entry.DeferredSummaries, summaryTurn{TurnID: entry.TurnID, SessionID: entry.SummarySession})
	}
	entry.SummaryStatus, entry.TurnID, entry.SummarySession = "", "", ""
	entry.SummaryError = ""
}

type DispatchInput struct {
	ChatInput
	MemberID        string `json:"memberId"`
	SourceMessageID string `json:"sourceMessageId,omitempty"`
}

type ReplyInput struct {
	MemberID      string               `json:"memberId"`
	Content       string               `json:"content,omitempty"`
	Delegate      bool                 `json:"delegate,omitempty"`
	Images        []agent.ImageInput   `json:"images,omitempty"`
	Model         string               `json:"model,omitempty"`
	ModelSettings *agent.ModelSettings `json:"modelSettings,omitempty"`
}

type CloseConversationInput struct {
	Force          bool `json:"force,omitempty"`
	DiscardSummary bool `json:"discardSummary,omitempty"`
}

type collaborationExecutor interface {
	ParentSession(id string) (*agent.Session, error)
	AppendOfficeRequest(id, content, existingID string) (string, error)
	SetOfficeContext(id, value string) error
	OfficeComplete(ctx context.Context, model, instruction, material string, settings *agent.ModelSettings) (string, error)
	DeleteSession(id string) error
}

type conversationScope struct {
	id       string
	path     string
	name     string
	model    string
	settings *agent.ModelSettings
}

type conversationRun struct {
	scope    conversationScope
	id       string
	session  string
	model    string
	settings *agent.ModelSettings
	images   []agent.ImageAttachment
}
