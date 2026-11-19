package office

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/logging"
)

type CommandReview struct {
	ID      string `json:"id"`
	Command string `json:"command"`
	Status  string `json:"status"`
	Output  string `json:"output,omitempty"`
}

// Delivery covers one turn. A running action without a matching end event
// remains unknown across a crash, even if the file system later changed.
type Delivery struct {
	Files           []FileChange      `json:"files,omitempty"`
	FileNote        string            `json:"fileNote,omitempty"`
	Commands        []CommandReview   `json:"commands,omitempty"`
	CommandsClipped bool              `json:"commandsClipped,omitempty"`
	Confirmed       string            `json:"confirmed,omitempty"`
	Pending         map[string]string `json:"pending,omitempty"`
}

func clipReview(value string, limit int) string {
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return value
}

func (r *Delivery) observe(a *agent.Activity) {
	label := strings.TrimSpace(a.Title + " " + a.Detail)
	if a.Status == "running" || a.Status == "unknown" {
		r.Pending[a.ID] = label
	} else {
		delete(r.Pending, a.ID)
		if a.Status == "completed" {
			r.Confirmed = label
		}
	}
	if a.Kind != "run_command" {
		return
	}
	command := a.Detail
	var input struct{ Command string }
	if json.Unmarshal([]byte(a.Input), &input) == nil && input.Command != "" {
		command = input.Command
	}
	item := CommandReview{ID: a.ID, Command: clipReview(command, 1024), Status: a.Status, Output: clipReview(a.Output, 600)}
	for i := range r.Commands {
		if r.Commands[i].ID == a.ID {
			r.Commands[i] = item
			return
		}
	}
	if len(r.Commands) >= 100 {
		r.CommandsClipped = true
		return
	}
	r.Commands = append(r.Commands, item)
}

func (s *Service) saveDelivery(id string, r *Delivery) {
	data, err := json.Marshal(r)
	if err == nil {
		var db *sql.DB
		db, err = s.openStore()
		if err == nil {
			defer db.Close()
			_, err = db.Exec("INSERT OR REPLACE INTO office_delivery(task_id,data) VALUES(?,?)", id, string(data))
		}
	}
	if err != nil {
		logging.Default().For(logging.ModuleAgent).Warn("office delivery save failed", logging.F("task", id), logging.F("error", err))
	}
}

func (s *Service) loadDelivery(id string) (*Delivery, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRow("SELECT data FROM office_delivery WHERE task_id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r Delivery
	if err = json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Service) reviewedTurn(ctx context.Context, taskID, sessionID, text string, resuming bool, emit func(agent.Event)) error {
	task, err := s.loadTask(taskID)
	if err != nil {
		return err
	}
	r := &Delivery{FileNote: "本轮尚未完成文件比对", Pending: map[string]string{}}
	s.saveDelivery(taskID, r)
	before, baselineErr := captureWorkspace(ctx, task.ProjectPath)
	if resuming {
		if baselineErr != nil {
			text += "\n本次只读 Git 检查：" + baselineErr.Error()
		} else {
			paths := make([]string, 0, len(before.dirty))
			for path := range before.dirty {
				paths = append(paths, path)
			}
			slices.Sort(paths)
			data, _ := json.Marshal(paths)
			text += "\n接续前只读检查发现的 Git 变更路径（供核对来源）：" + string(data)
		}
	}
	// Hold terminal status until the receipt is durable so the UI's final
	// refresh cannot race the file comparison.
	var terminal *agent.Event
	err = s.exec.RunTurn(ctx, sessionID, text, "", nil, func(e agent.Event) {
		if e.Type == "activity" && e.Activity != nil {
			activity := *e.Activity
			// Cancellation can stop the observer after a command has already
			// produced external effects. Its exit error does not undo them.
			if ctx.Err() != nil && activity.Status == "failed" {
				activity.Status = "unknown"
			}
			r.observe(&activity)
			s.saveDelivery(taskID, r)
		}
		if e.Type == "status" && isTerminal(e.Status) {
			copy := e
			terminal = &copy
			return
		}
		emit(e)
	})
	if baselineErr == nil {
		var after *workspaceSample
		after, baselineErr = captureWorkspace(s.rootCtx, task.ProjectPath)
		if baselineErr == nil {
			r.Files, baselineErr = compareWorkspace(s.rootCtx, before, after)
		}
	}
	r.FileNote = ""
	if baselineErr != nil {
		r.FileNote = baselineErr.Error()
	}
	s.saveDelivery(taskID, r)
	if terminal != nil {
		emit(*terminal)
	}
	return err
}

func canRecover(task *Task) bool {
	return task.Status == StatusFailed && (task.Interrupted || task.Error == "服务重启导致任务中断，本轮无法自动恢复")
}

func recoveryPrompt(task *Task, r *Delivery, text string) string {
	lines := []string{text, "上轮执行中断。先只读核对当前文件、历史和外部操作结果。仅继续原授权范围内、已确认安全的未完成步骤。无法核实的写入、发布、扣费等操作先向用户确认，禁止盲目重放。"}
	if r != nil {
		if r.Confirmed != "" {
			lines = append(lines, "最后确认完成："+r.Confirmed)
		}
		pending := make([]string, 0, len(r.Pending))
		for _, label := range r.Pending {
			pending = append(pending, label)
		}
		slices.Sort(pending)
		for _, label := range pending {
			lines = append(lines, "结果待核对："+label)
		}
	} else if a := task.LatestActivity; a != nil {
		lines = append(lines, "上轮最后记录（"+a.Status+"）："+a.Title+" "+a.Detail)
	}
	return strings.Join(lines, "\n")
}
