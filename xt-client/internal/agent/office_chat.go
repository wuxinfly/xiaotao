package agent

import (
	"fmt"
	"strings"
)

func (s *Service) prepareTurnInstructions(workspace *Workspace, history *Session) (string, string, string, error) {
	if history.ReadOnly {
		return "", "", "", nil
	}
	instructions := ""
	var err error
	if workspace != nil {
		instructions, err = workspace.projectInstructions()
		if err != nil {
			return "", "", "", err
		}
	}
	core, err := prepareCore(s.dataDir, len(history.Messages) == 0)
	if err != nil {
		return "", "", "", fmt.Errorf("准备小涛 Core: %w", err)
	}
	skills, err := s.localSkillsInstructions()
	if err != nil {
		return "", "", "", fmt.Errorf("准备本地 Skills: %w", err)
	}
	return core, skills, instructions, nil
}

func officeChatRole(session *Session) string {
	if session.Member != nil {
		identity := "\n你是项目办公室成员" + session.Member.Name + "。"
		if responsibility := strings.TrimSpace(session.Member.Responsibility); responsibility != "" {
			identity += "你的职责是：" + responsibility + "。"
		}
		if session.ReadOnly {
			return identity + "当前是直接对话，你没有任何工具，无法读取项目文件或执行操作；不要声称看过未提供的文件或已经执行操作。"
		}
		return identity + "当前是你已接受的项目任务。职责说明不是额外权限；按用户确认的目标和现有工具规则执行，不声称代表其他成员工作。"
	}
	if !session.ReadOnly {
		return ""
	}
	role := "员工"
	switch session.Role {
	case "office_chat_xiaotao":
		role = "小涛"
	case "office_chat_designer":
		role = "设计师"
	case "office_chat_developer":
		role = "开发者"
	}
	return "\n你是项目办公室的" + role + "。当前是直接对话，你没有任何工具，无法读取项目文件或执行操作；不要声称看过未提供的文件或已经执行操作。"
}
