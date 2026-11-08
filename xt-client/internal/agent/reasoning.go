package agent

import "xiaotao/xt-client/internal/effort"

// reasoningEffort normalizes the configured level and snaps it to the levels
// the model actually offers. Models that declare no levels follow their
// provider default, so unknown values are dropped instead of being sent.
func (s *Service) reasoningEffort(model string, input string) string {
	level, ok := effort.Parse(input)
	if !ok {
		return ""
	}
	for _, option := range s.AvailableModels() {
		if option.Model != model {
			continue
		}
		if len(option.ReasoningEfforts) == 0 {
			return ""
		}
		return string(effort.Nearest(level, option.ReasoningEfforts))
	}
	return string(level)
}
