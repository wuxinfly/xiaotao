package office

import (
	"context"
	"fmt"

	"xiaotao/xt-client/internal/agent"
)

type imageTurnExecutor interface {
	StoreMessageImages([]agent.ImageInput) ([]agent.ImageAttachment, error)
	RunTurnWithImages(context.Context, string, string, string, *agent.ModelSettings, []agent.ImageAttachment, func(agent.Event)) error
}

func (s *Service) storeTurnImages(model string, inputs []agent.ImageInput) ([]agent.ImageAttachment, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	executor, ok := s.exec.(imageTurnExecutor)
	if !ok {
		return nil, fmt.Errorf("当前执行器不支持图片")
	}
	for _, option := range s.exec.AvailableModels() {
		if option.Model == model && option.SupportsImages {
			return executor.StoreMessageImages(inputs)
		}
	}
	return nil, fmt.Errorf("当前员工模型不支持图片，请使用支持图片的模型")
}

func (s *Service) runEmployeeTurn(ctx context.Context, run conversationRun, text string, emit func(agent.Event)) error {
	if len(run.images) == 0 {
		return s.exec.RunTurn(ctx, run.session, text, run.model, run.settings, emit)
	}
	executor, ok := s.exec.(imageTurnExecutor)
	if !ok {
		return fmt.Errorf("当前执行器不支持图片")
	}
	return executor.RunTurnWithImages(ctx, run.session, text, run.model, run.settings, run.images, emit)
}
