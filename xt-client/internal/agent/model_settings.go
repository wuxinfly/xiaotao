package agent

import (
	"fmt"
	"strings"
)

func (s *Service) AvailableModels() []ModelOption {
	if s.models == nil {
		return []ModelOption{}
	}
	return s.models()
}

func (s *Service) supportsImages(model string) bool {
	for _, option := range s.AvailableModels() {
		if option.Model == model {
			return option.SupportsImages
		}
	}
	return false
}

func (s *Service) contextWindow(model string) int64 {
	for _, option := range s.AvailableModels() {
		if option.Model == model {
			return option.ContextWindow
		}
	}
	return 0
}

func (s *Service) modelDefaults(model string) *ModelSettings {
	for _, option := range s.AvailableModels() {
		if option.Model == model && (option.ContextWindow > 0 || option.MaxOutputTokens > 0) {
			return &ModelSettings{ContextWindow: option.ContextWindow, MaxOutputTokens: option.MaxOutputTokens}
		}
	}
	return nil
}

func normalizeModelSettings(input, fallback *ModelSettings) (*ModelSettings, error) {
	if input == nil {
		if fallback == nil {
			return nil, nil
		}
		copy := *fallback
		return &copy, nil
	}
	contextWindow, output := input.ContextWindow, input.MaxOutputTokens
	effortValue := strings.TrimSpace(input.ReasoningEffort)
	if fallback != nil {
		if contextWindow == 0 {
			contextWindow = fallback.ContextWindow
		}
		if output == 0 {
			output = fallback.MaxOutputTokens
		}
		if effortValue == "" {
			effortValue = fallback.ReasoningEffort
		}
	}
	if input.ContextWindow < 0 || input.MaxOutputTokens < 0 {
		return nil, fmt.Errorf("模型参数不能为负数")
	}
	if contextWindow > 0 && contextWindow < 1024 {
		return nil, fmt.Errorf("上下文窗口不能小于 1024 tokens")
	}
	if output > 0 && output < 16 {
		return nil, fmt.Errorf("最大输出不能小于 16 tokens")
	}
	if contextWindow > 0 && output >= contextWindow {
		return nil, fmt.Errorf("最大输出必须小于上下文窗口")
	}
	if contextWindow == 0 && output == 0 && effortValue == "" {
		return nil, nil
	}
	return &ModelSettings{ContextWindow: contextWindow, MaxOutputTokens: output, ReasoningEffort: effortValue}, nil
}
