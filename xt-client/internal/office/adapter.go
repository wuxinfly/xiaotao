package office

import (
	"context"

	"xiaotao/xt-client/internal/agent"
)

// AgentExecutor adapts *agent.Service to the office Executor interface. It is
// the only production implementation; tests use a fake.
type AgentExecutor struct {
	svc *agent.Service
}

func NewAgentExecutor(svc *agent.Service) *AgentExecutor {
	return &AgentExecutor{svc: svc}
}

func (e *AgentExecutor) CreateSession(options agent.Options) (*agent.Session, error) {
	return e.svc.Create(options)
}

func (e *AgentExecutor) SetMember(sessionID string, member *agent.MemberIdentity) error {
	return e.svc.SetMember(sessionID, member)
}

func (e *AgentExecutor) RunTurn(ctx context.Context, sessionID, text, model string, settings *agent.ModelSettings, emit func(agent.Event)) error {
	if model == "" && settings == nil {
		return e.svc.Send(ctx, sessionID, text, emit)
	}
	return e.svc.SendWithModelSettings(ctx, sessionID, text, model, settings, nil, nil, emit)
}

func (e *AgentExecutor) Cancel(sessionID string) bool {
	return e.svc.Cancel(sessionID)
}

func (e *AgentExecutor) RunTurnWithImages(ctx context.Context, sessionID, text, model string, settings *agent.ModelSettings, images []agent.ImageAttachment, emit func(agent.Event)) error {
	return e.svc.SendWithStoredImages(ctx, sessionID, text, model, settings, images, emit)
}

func (e *AgentExecutor) StoreMessageImages(images []agent.ImageInput) ([]agent.ImageAttachment, error) {
	return e.svc.StoreMessageImages(images)
}

func (e *AgentExecutor) History(sessionID, before string) (*agent.HistoryPage, error) {
	return e.svc.History(sessionID, before)
}

func (e *AgentExecutor) ResolveApproval(sessionID, approvalID string, approved bool) error {
	return e.svc.ResolveApproval(sessionID, approvalID, approved)
}

// ListSummaries populates the kernel run registry and marks stale running
// sessions failed; the office calls it before serving so pre-restart sessions
// can still accept new turns.
func (e *AgentExecutor) ListSummaries() ([]*agent.Session, error) {
	return e.svc.ListSummaries()
}

func (e *AgentExecutor) AvailableModels() []agent.ModelOption {
	return e.svc.AvailableModels()
}

func (e *AgentExecutor) ParentSession(id string) (*agent.Session, error) {
	return e.svc.OfficeSessionMetadata(id)
}

func (e *AgentExecutor) OfficeComplete(ctx context.Context, model, instruction, material string, settings *agent.ModelSettings) (string, error) {
	return e.svc.OfficeComplete(ctx, model, instruction, material, settings)
}

func (e *AgentExecutor) AppendOfficeRequest(id, content, existingID string) (string, error) {
	return e.svc.AppendOfficeRequest(id, content, existingID)
}

func (e *AgentExecutor) SetOfficeContext(id, value string) error {
	return e.svc.SetOfficeContext(id, value)
}

func (e *AgentExecutor) DeleteSession(id string) error { return e.svc.DeleteOfficeChild(id) }

func (e *AgentExecutor) AccessMode() string {
	return e.svc.AccessMode()
}
