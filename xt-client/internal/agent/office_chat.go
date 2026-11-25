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
	core := ""
	if !isOfficeMember(history) {
		core, err = prepareCore(s.dataDir, len(history.Messages) == 0)
		if err != nil {
			return "", "", "", fmt.Errorf("准备小涛 Core: %w", err)
		}
	}
	if !employeeToolPermissions(history).allows("read_file") {
		return core, "", instructions, nil
	}
	skills, err := s.localSkillsInstructions()
	if err != nil {
		return "", "", "", fmt.Errorf("准备本地 Skills: %w", err)
	}
	return core, skills, instructions, nil
}

func isOfficeMember(session *Session) bool {
	return session.Member != nil || strings.HasPrefix(session.Role, "office_member_")
}

func officeChatRole(session *Session) string {
	if !isOfficeMember(session) && !session.ReadOnly {
		return ""
	}
	name, responsibility := "员工", ""
	if session.Member != nil {
		name, responsibility = session.Member.Name, strings.TrimSpace(session.Member.Responsibility)
	} else {
		switch session.Role {
		case "office_chat_xiaotao":
			name = "小涛"
		case "office_chat_designer":
			name = "设计师"
		case "office_chat_developer":
			name = "开发者"
		}
	}
	identity := "你是项目办公室成员「" + name + "」。"
	if responsibility != "" {
		identity += "你的职责是：" + responsibility + "。"
	}
	identity += "被问到身份时使用当前成员名称和职责，以这里的身份为准。"
	if session.ReadOnly {
		return identity + "当前是直接对话，你没有任何工具，无法读取项目文件或执行操作；不要声称看过未提供的文件或已经执行操作。" + session.OfficeContext
	}
	return identity + "当前是你已接受的项目任务。职责说明不是额外权限；按用户确认的目标和现有工具规则执行，不声称代表其他成员工作。" + session.OfficeContext
}

func turnSystemPrompt(session *Session, core, skills, instructions string) string {
	role := officeChatRole(session)
	if role == "" {
		return systemPrompt(session.Project.Path) + core + skills + "\n" + instructions
	}
	prompt := role + "\n使用中文，简洁、自然、不说套话。" + voiceNotice()
	if session.ReadOnly {
		return prompt
	}
	prompt += projectToolNotice(session.Project.Path)
	if permissions := employeeToolPermissions(session); permissions != nil {
		prompt += "\n本任务以当前开放的工具列表为准，未开放的工具不可调用。"
	}
	if employeeToolPermissions(session).allows(mcpEnableToolName) {
		prompt += browserNotice()
	}
	return prompt + skills + "\n" + instructions
}

func projectToolNotice(root string) string {
	if root == "" {
		return "\n当前未关联项目，无法访问文件或运行命令；需要这些操作时请在关联项目的会话中处理。"
	}
	return "\n当前项目根目录：" + root +
		"\n先调查再修改。优先使用 search_files 和 read_file 定位代码，写文件前确认目标。" +
		"\n操作子目录前检查并遵守该目录及其上级目录中的 AGENTS.override.md 或 AGENTS.md；同目录优先前者。" +
		"\n修改后必须运行相关验证。不要声称执行过没有实际执行的命令。" +
		"\n工作目录是默认位置；可以按用户要求访问其他路径。不要泄露凭据、token 或隐私信息。" +
		"\n只有在工作完成、遇到阻碍或需要用户决定时才给最终答复。"
}
