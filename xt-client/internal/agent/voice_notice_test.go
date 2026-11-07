package agent

import (
	"strings"
	"testing"
)

func TestSystemPromptIncludesAnswerGuidanceWithAndWithoutProject(t *testing.T) {
	for _, root := range []string{"", "E:/fixture"} {
		prompt := systemPrompt(root)
		for _, guidance := range []string{"先直接回答", "用户要求深入时充分展开", "风险、未完成项和不确定性", "用户指定格式"} {
			if !strings.Contains(prompt, guidance) {
				t.Errorf("systemPrompt(%q) missing guidance %q", root, guidance)
			}
		}
	}
}
