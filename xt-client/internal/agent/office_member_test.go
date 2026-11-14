package agent

import (
	"strings"
	"testing"
)

func TestOfficeMemberPromptPreservesExecutionBoundary(t *testing.T) {
	member := &MemberIdentity{ID: "role_designer", Name: "设计师", Responsibility: "输出界面方案"}
	chat := officeChatRole(&Session{ReadOnly: true, Member: member})
	if !strings.Contains(chat, "设计师") || !strings.Contains(chat, "没有任何工具") {
		t.Fatalf("read-only member prompt: %s", chat)
	}
	task := officeChatRole(&Session{Member: member})
	if !strings.Contains(task, "输出界面方案") || !strings.Contains(task, "不是额外权限") || strings.Contains(task, "没有任何工具") {
		t.Fatalf("executable member prompt: %s", task)
	}
	if prompt := officeChatRole(&Session{}); prompt != "" {
		t.Fatalf("ordinary session inherited member identity: %s", prompt)
	}
}
