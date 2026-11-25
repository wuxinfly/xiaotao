package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestOfficeMemberIdentityInModelRequest(t *testing.T) {
	type modelRequest struct {
		Messages []struct{ Role, Content string }
		Tools    []json.RawMessage
	}
	requests := make(chan modelRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input modelRequest
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requests <- input
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"我是小涛"}}]}`))
	}))
	defer server.Close()
	svc := NewService(t.TempDir(), server.URL, nil, server.Client(), nil)
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("项目约定测试标记"), 0600); err != nil {
		t.Fatal(err)
	}
	product := &MemberIdentity{ID: "role_product", Name: "产品", Responsibility: "拆需求和定义验收标准"}
	for _, tc := range []struct {
		name     string
		options  Options
		wants    []string
		excludes []string
		tools    bool
	}{
		{"主会话", Options{}, []string{"你是小涛", "阶段保存契约"}, []string{"成员「产品」"}, true},
		{"员工对话", Options{Role: "office_chat_role_product", Member: product, ReadOnly: true},
			[]string{"成员「产品」", product.Responsibility, "没有任何工具"}, []string{"你是小涛", "阶段保存契约", "项目约定测试标记", mcpEnableToolName}, false},
		{"员工任务", Options{Role: "office_member_role_product", Member: product},
			[]string{"成员「产品」", product.Responsibility, "项目约定测试标记"}, []string{"你是小涛", "阶段保存契约"}, true},
		{"旧版员工对话", Options{Role: "office_chat_designer", ReadOnly: true},
			[]string{"成员「设计师」", "没有任何工具"}, []string{"你是小涛", "阶段保存契约"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.options.Model, tc.options.ProjectPath = "test/model", project
			session, err := svc.Create(tc.options)
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.Send(context.Background(), session.ID, "你是谁？", func(Event) {}); err != nil {
				t.Fatal(err)
			}
			input := <-requests
			if len(input.Messages) == 0 || input.Messages[0].Role != "system" {
				t.Fatal("请求缺少系统提示")
			}
			prompt := input.Messages[0].Content
			for _, want := range tc.wants {
				if !strings.Contains(prompt, want) {
					t.Errorf("系统提示缺少 %q", want)
				}
			}
			for _, exclude := range tc.excludes {
				if strings.Contains(prompt, exclude) {
					t.Errorf("系统提示混入 %q", exclude)
				}
			}
			if (len(input.Tools) > 0) != tc.tools {
				t.Errorf("工具数量 %d，预期有工具为 %v", len(input.Tools), tc.tools)
			}
			if tc.options.Member != nil && tc.options.ReadOnly {
				if err := svc.Send(context.Background(), session.ID, "再说一次你是谁？", func(Event) {}); err != nil {
					t.Fatal(err)
				}
				continued := <-requests
				if continued.Messages[0].Content != prompt ||
					len(continued.Messages) < 4 ||
					continued.Messages[2].Role != "assistant" ||
					continued.Messages[2].Content != "我是小涛" {
					t.Fatal("续聊时应保留历史，并继续使用员工的系统提示")
				}
			}
		})
	}
}
