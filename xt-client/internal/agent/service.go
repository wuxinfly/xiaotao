package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const maxTurns = 40

type Service struct {
	dataDir  string
	endpoint string
	key      func() string
	client   *http.Client
	mu       sync.Mutex
	runs     map[string]*run
	models   func() []ModelOption
}

type run struct {
	cancel context.CancelFunc
}

type modelClient interface {
	Complete(ctx context.Context, model string, messages []map[string]any, tools []map[string]any) (*completion, error)
}

type httpModelClient struct {
	endpoint string
	key      func() string
	client   *http.Client
}

type completion struct {
	Content   string
	ToolCalls []toolCall
}

type toolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

type backendResponse struct {
	Choices []struct {
		Message struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func NewService(dataDir, endpoint string, key func() string, client *http.Client, models func() []ModelOption) *Service {
	return &Service{
		dataDir:  dataDir,
		endpoint: strings.TrimRight(endpoint, "/") + "/v1/chat/completions",
		key:      key,
		client:   client,
		runs:     map[string]*run{},
		models:   models,
	}
}

func (s *Service) AvailableModels() []ModelOption {
	if s.models == nil {
		return []ModelOption{}
	}
	return s.models()
}

func (s *Service) Create(options Options) (*Session, error) {
	project := Project{}
	projectName := strings.TrimSpace(options.ProjectName)
	if strings.TrimSpace(options.ProjectPath) == "" {
		path := filepath.Join(s.dataDir, "workspace")
		if err := os.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
		workspace, err := OpenWorkspace(path)
		if err != nil {
			return nil, err
		}
		if projectName == "" {
			projectName = "默认工作目录"
		}
		project = Project{Path: workspace.Root, Name: projectName}
	} else {
		var err error
		project, err = s.Project(options.ProjectPath)
		if err != nil {
			return nil, err
		}
		if projectName != "" {
			project.Name = projectName
		}
	}
	model := strings.TrimSpace(options.Model)
	if model == "" {
		return nil, fmt.Errorf("请先选择模型")
	}
	session := &Session{
		ID:        "run_" + randomID(8),
		Project:   project,
		Title:     "新任务",
		Model:     model,
		Status:    StatusIdle,
		Messages:  []Message{},
		Activity:  []Activity{},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	s.mu.Lock()
	s.runs[session.ID] = &run{}
	s.mu.Unlock()
	if err := s.saveSession(session); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Service) Get(id string) (*Session, error) {
	return s.loadSession(id)
}

func (s *Service) Project(path string) (Project, error) {
	workspace, err := OpenWorkspace(path)
	if err != nil {
		return Project{}, err
	}
	return Project{Path: workspace.Root, Name: filepath.Base(workspace.Root)}, nil
}

func (s *Service) List() ([]*Session, error) {
	out, err := s.listSessions()
	if err != nil {
		return nil, err
	}
	for _, session := range out {
		s.mu.Lock()
		_, known := s.runs[session.ID]
		if !known {
			s.runs[session.ID] = &run{}
		}
		s.mu.Unlock()
		if !known && session.Status == StatusRunning {
			session.Status = StatusFailed
			session.Error = "上次任务因服务退出而中断"
			if err := s.saveSession(session); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s *Service) Send(ctx context.Context, sessionID, text string, emit func(Event)) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("消息不能为空")
	}
	s.mu.Lock()
	r, ok := s.runs[sessionID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("任务不存在")
	}
	if r.cancel != nil {
		return fmt.Errorf("小涛还在处理上一条消息")
	}
	history, err := s.loadSession(sessionID)
	if err != nil {
		return err
	}
	var workspace *Workspace
	if history.Project.Path != "" {
		workspace, err = OpenWorkspace(history.Project.Path)
		if err != nil {
			return err
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	r.cancel = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		r.cancel = nil
		s.mu.Unlock()
	}()

	history.Status = StatusRunning
	history.Error = ""
	now := time.Now()
	userMessage := Message{ID: "msg_" + randomID(6), Role: "user", Content: text, CreatedAt: now}
	history.Messages = append(history.Messages, userMessage)
	history.UpdatedAt = now
	if history.Title == "新任务" {
		history.Title = titleFromPrompt(text)
	}
	if err := s.saveSession(history); err != nil {
		return err
	}
	emit(Event{Type: "session", SessionID: sessionID, Message: &userMessage, Status: StatusRunning})

	messages := []map[string]any{{"role": "system", "content": systemPrompt(history.Project.Path)}}
	for _, message := range history.Messages {
		messages = append(messages, messageToModel(message))
	}
	client := &httpModelClient{endpoint: s.endpoint, key: s.key, client: s.client}
	err = s.loop(runCtx, workspace, client, history, messages, emit)
	history.UpdatedAt = time.Now()
	if err != nil {
		if runCtx.Err() != nil {
			history.Status = StatusCanceled
			history.Error = "任务已停止"
		} else {
			history.Status = StatusFailed
			history.Error = err.Error()
		}
		emit(Event{Type: "status", SessionID: sessionID, Status: history.Status, Error: history.Error})
	} else {
		history.Status = StatusCompleted
		emit(Event{Type: "status", SessionID: sessionID, Status: StatusCompleted})
	}
	saveErr := s.saveSession(history)
	if err != nil {
		return err
	}
	return saveErr
}

func (s *Service) loop(ctx context.Context, workspace *Workspace, client modelClient, history *Session, messages []map[string]any, emit func(Event)) error {
	for turn := 0; turn < maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var tools []map[string]any
		if workspace != nil {
			tools = toolDefinitions
		}
		result, err := client.Complete(ctx, history.Model, messages, tools)
		if err != nil {
			return fmt.Errorf("模型调用失败: %w", err)
		}
		if len(result.ToolCalls) == 0 {
			content := strings.TrimSpace(result.Content)
			if content == "" {
				content = "任务处理完成。"
			}
			message := Message{ID: "msg_" + randomID(6), Role: "assistant", Content: content, CreatedAt: time.Now()}
			history.Messages = append(history.Messages, message)
			emit(Event{Type: "message", SessionID: history.ID, Message: &message})
			return nil
		}
		assistant := map[string]any{"role": "assistant", "content": result.Content}
		rawCalls := make([]any, 0, len(result.ToolCalls))
		for _, call := range result.ToolCalls {
			rawCalls = append(rawCalls, map[string]any{
				"id": call.ID, "type": "function",
				"function": map[string]any{"name": call.Name, "arguments": string(call.Arguments)},
			})
		}
		assistant["tool_calls"] = rawCalls
		messages = append(messages, assistant)
		if strings.TrimSpace(result.Content) != "" {
			history.Messages = append(history.Messages, Message{
				ID: "msg_" + randomID(6), Role: "assistant", Content: result.Content, CreatedAt: time.Now(),
			})
		}
		for _, call := range result.ToolCalls {
			if err := ctx.Err(); err != nil {
				return err
			}
			activity := Activity{
				ID:        "act_" + randomID(5),
				Kind:      call.Name,
				Title:     activityTitle(call.Name),
				Detail:    activityDetail(call.Arguments),
				Status:    "running",
				StartedAt: time.Now(),
			}
			history.Activity = append(history.Activity, activity)
			history.UpdatedAt = time.Now()
			emit(Event{Type: "activity", SessionID: history.ID, Activity: &activity})
			output := "请先关联项目才能使用文件和命令工具"
			var toolErr error
			if workspace != nil {
				output, toolErr = workspace.Execute(ctx, call.Name, call.Arguments)
			} else {
				toolErr = fmt.Errorf("%s", output)
			}
			end := time.Now()
			activity.EndedAt = &end
			activity.Status = "completed"
			if toolErr != nil {
				activity.Status = "failed"
				if output == "" {
					output = toolErr.Error()
				}
			}
			history.Activity[len(history.Activity)-1] = activity
			history.UpdatedAt = end
			emit(Event{Type: "activity", SessionID: history.ID, Activity: &activity})
			messages = append(messages, map[string]any{
				"role": "tool", "tool_call_id": call.ID, "name": call.Name, "content": output,
			})
			history.Messages = append(history.Messages, Message{
				ID: "msg_" + randomID(6), Role: "tool", Content: output,
				ToolCallID: call.ID, ToolName: call.Name, CreatedAt: end,
			})
			if err := s.saveSession(history); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("工具调用超过 %d 轮，已停止", maxTurns)
}

func (s *Service) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok || r.cancel == nil {
		return false
	}
	r.cancel()
	return true
}

func (c *httpModelClient) Complete(ctx context.Context, model string, messages []map[string]any, tools []map[string]any) (*completion, error) {
	payload := map[string]any{"model": model, "messages": messages, "stream": false}
	if len(tools) > 0 {
		payload["tools"] = tools
		payload["tool_choice"] = "auto"
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(string(b)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.key != nil {
		req.Header.Set("Authorization", "Bearer "+c.key())
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var body backendResponse
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("模型返回无法解析")
	}
	if resp.StatusCode >= 400 {
		if body.Error != nil && body.Error.Message != "" {
			return nil, fmt.Errorf("%s", body.Error.Message)
		}
		return nil, fmt.Errorf("模型请求失败（%d）", resp.StatusCode)
	}
	if len(body.Choices) == 0 {
		return nil, fmt.Errorf("模型没有返回结果")
	}
	message := body.Choices[0].Message
	out := &completion{Content: message.Content}
	for _, call := range message.ToolCalls {
		args := strings.TrimSpace(call.Function.Arguments)
		if args == "" {
			args = "{}"
		}
		out.ToolCalls = append(out.ToolCalls, toolCall{ID: call.ID, Name: call.Function.Name, Arguments: json.RawMessage(args)})
	}
	return out, nil
}

func systemPrompt(root string) string {
	if root == "" {
		return "你是小涛，用中文简洁自然地帮助用户。当前未关联项目，无法访问文件或运行命令；需要这些操作时请用户新建关联项目的会话。"
	}
	return "你是小涛，一个直接和用户一起做软件工作的 AI 工程师。使用中文，简洁、自然、不说套话。" +
		"\n当前项目根目录：" + root +
		"\n先调查再修改。优先使用 search_files 和 read_file 定位代码，写文件前确认目标。" +
		"\n修改后必须运行相关验证。不要声称执行过没有实际执行的命令。" +
		"\n不要访问项目目录以外的路径。不要泄露凭据、token 或隐私信息。" +
		"\n只有在工作完成、遇到阻碍或需要用户决定时才给最终答复。"
}

func messageToModel(message Message) map[string]any {
	if message.Role == "tool" {
		return map[string]any{
			"role": "tool", "tool_call_id": message.ToolCallID,
			"name": message.ToolName, "content": message.Content,
		}
	}
	out := map[string]any{"role": message.Role, "content": message.Content}
	return out
}

func titleFromPrompt(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	runes := []rune(text)
	if len(runes) > 28 {
		return string(runes[:28]) + "..."
	}
	return text
}

func activityTitle(name string) string {
	switch name {
	case "list_files":
		return "查看项目结构"
	case "search_files":
		return "搜索代码"
	case "read_file":
		return "阅读文件"
	case "write_file":
		return "更新文件"
	case "run_command":
		return "运行命令"
	default:
		return "调用工具"
	}
}

func activityDetail(args json.RawMessage) string {
	var value map[string]any
	if json.Unmarshal(args, &value) != nil {
		return ""
	}
	for _, key := range []string{"path", "command", "query"} {
		if text, ok := value[key].(string); ok && text != "" {
			runes := []rune(text)
			if len(runes) > 80 {
				return string(runes[:80]) + "..."
			}
			return text
		}
	}
	return ""
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
