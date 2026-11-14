package office

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"xiaotao/xt-client/internal/agent"
	"xiaotao/xt-client/internal/logging"
)

// Service owns project-scoped office tasks. Execution is detached from HTTP
// requests: the office goroutine owns the context, so leaving a page never
// cancels a task, while Close cancels every remaining run.
type Service struct {
	dataDir     string
	exec        Executor
	mu          sync.Mutex
	active      map[string]*Task
	runs        map[string]context.CancelFunc
	closed      bool
	wg          sync.WaitGroup
	now         func() time.Time
	teams       *TeamService
	subscribers map[string]map[chan agent.Event]struct{}
	// rootCtx is canceled by Close. Every turn derives from it, so shutdown
	// stops runs that started after Close snapshotted the run map.
	rootCtx    context.Context
	rootCancel context.CancelFunc
}

func New(dataDir string, exec Executor) (*Service, error) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	svc := &Service{
		dataDir:    dataDir,
		exec:       exec,
		active:     map[string]*Task{},
		runs:       map[string]context.CancelFunc{},
		now:        time.Now,
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		teams:      NewTeamService(NewRegistryStore(filepath.Dir(dataDir))),
	}
	db, err := svc.openStore()
	if err != nil {
		return nil, err
	}
	db.Close()
	svc.teams.SetActiveCheck(func(projectPath, roleID string) bool {
		return svc.memberBusy(projectPath, roleID)
	})
	svc.teams.store.SetProjectIndex(svc.knownProjects)
	// The kernel only registers sessions that were listed in this process, and
	// it marks stale running sessions failed while listing. Do that before any
	// office task is started so a pre-restart session can accept a new turn.
	// A failure here must not stop the whole client: the office still works for
	// new tasks, and existing tasks are reconciled below either way.
	if _, err := exec.ListSummaries(); err != nil {
		logging.Default().For(logging.ModuleAgent).Warn("office session registry refresh failed", logging.F("error", err))
	}
	if err := svc.recoverInterrupted(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *Service) Teams() *TeamService { return s.teams }

// recoverInterrupted reconciles tasks that were running when the service
// exited. The kernel cannot resume a half-finished turn, so they become
// failed with an explicit reason instead of pretending to continue.
func (s *Service) recoverInterrupted() error {
	tasks, err := s.loadAllTasks()
	if err != nil {
		return err
	}
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	now := s.now()
	for _, task := range tasks {
		if isTerminal(task.Status) {
			continue
		}
		task.Status = StatusFailed
		task.Error = "服务重启导致任务中断，本轮无法自动恢复"
		task.WaitingReason = ""
		task.Approval = nil
		task.CancelRequested = false
		task.EndedAt = &now
		task.UpdatedAt = now
		if err = s.writeTask(db, task); err != nil {
			return err
		}
		messages, err := s.loadMessages(task.ID)
		if err != nil {
			return err
		}
		for _, message := range messages {
			if message.Status != MessageQueued {
				continue
			}
			message.Status = MessageFailed
			message.Error = "服务重启，消息未送达"
			if err = s.writeMessage(db, message); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()
	// Cancel the root first: it reaches every turn, including one that claimed
	// its slot after the run map was inspected.
	s.rootCancel()
	s.closeTaskEvents()
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.runs))
	for _, cancel := range s.runs {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	s.wg.Wait()
}

func (s *Service) CreateTask(req CreateTaskRequest) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	prepared, err := s.prepareMemberTaskLocked(req)
	if err != nil {
		return nil, err
	}
	session, err := s.exec.CreateSession(prepared.options)
	if err != nil {
		return nil, err
	}
	now := s.now()
	task := &Task{
		ID:            "task_" + randomID(8),
		ProjectPath:   prepared.projectPath,
		ProjectName:   prepared.projectName,
		Title:         prepared.title,
		Goal:          prepared.goal,
		Status:        StatusQueued,
		Executor:      ExecutorInfo{Kind: prepared.kind, Name: prepared.name, Role: prepared.role, Model: prepared.model, ModelLabel: modelLabel(s.exec.AvailableModels(), prepared.model), SessionID: session.ID},
		SessionID:     session.ID,
		MemberID:      prepared.memberID,
		ChatSessionID: prepared.chatSessionID,
		Discussion:    prepared.discussion,
		ModelSettings: req.ModelSettings,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if err = s.writeTask(db, task); err != nil {
		return nil, err
	}
	s.active[task.ID] = task
	// Copy under the lock: once the run starts, the goroutine mutates the
	// shared pointer while the caller still needs its initial snapshot.
	copied := snapshotTask(task)
	s.startRunLocked(task.ID, session.ID, taskPrompt(prepared.title, prepared.goal)+prepared.prompt, "")
	return copied, nil
}

func (s *Service) startRunLocked(taskID, sessionID, text, messageID string) {
	if !s.claimRunLocked(taskID) {
		return
	}
	go func() {
		defer s.wg.Done()
		s.runLoop(taskID, sessionID, text, messageID)
	}()
}

func (s *Service) startRun(taskID, sessionID, text, messageID string) {
	if !s.claimRun(taskID) {
		return
	}
	go func() {
		defer s.wg.Done()
		s.runLoop(taskID, sessionID, text, messageID)
	}()
}

// claimRun reserves the task's single run slot. The no-op cancel func is
// replaced by the loop's real cancel; every entry point uses this claim so a
// message arriving at the end of a loop cannot start a second concurrent turn
// on the same session.
func (s *Service) claimRun(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimRunLocked(taskID)
}

func (s *Service) claimRunLocked(taskID string) bool {
	if s.closed {
		return false
	}
	if _, busy := s.runs[taskID]; busy {
		return false
	}
	s.runs[taskID] = func() {}
	// Add under the lock: Close takes the same lock before wg.Wait, so a
	// goroutine can never start after Close has already stopped waiting.
	s.wg.Add(1)
	return true
}

// runLoop executes one turn at a time and drains queued messages after a
// successful turn. It never delivers a message into a running model turn.
// The run entry stays registered for the whole loop so a message posted while
// the loop is finishing still queues instead of starting a second goroutine.
func (s *Service) runLoop(taskID, sessionID, firstText, firstMessageID string) {
	text, messageID := firstText, firstMessageID
	defer func() {
		// Claim and start the next queued turn while still holding the lock, so
		// a message posted concurrently either sees this run and queues, or
		// sees no run and starts the turn itself. Without this, a message that
		// lands as the loop exits can stay queued forever.
		s.mu.Lock()
		delete(s.runs, taskID)
		if !s.closed {
			if next, err := s.nextQueuedMessage(taskID); err == nil && next != nil {
				if task, err := s.loadTask(taskID); err == nil && task.Status == StatusCompleted {
					s.runs[taskID] = func() {}
					s.wg.Add(1)
					go func() {
						defer s.wg.Done()
						s.runLoop(taskID, sessionID, next.Content, next.ID)
					}()
				}
			}
		}
		s.mu.Unlock()
	}()
	for {
		ctx, cancel := context.WithCancel(s.rootCtx)
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			cancel()
			return
		}
		s.runs[taskID] = cancel
		s.mu.Unlock()
		if !s.beginTurn(taskID, messageID) {
			cancel()
			// Canceled before the kernel accepted the turn: finalize here so the
			// task does not stay queued, and mark any waiting message undelivered.
			s.endTurn(taskID, context.Canceled, false)
			return
		}
		delivered := false
		emit := func(event agent.Event) {
			delivered = s.handleEvent(taskID, event, messageID, delivered)
			s.publishTask(taskID, event)
		}
		err := s.exec.RunTurn(ctx, sessionID, text, "", nil, emit)
		cancel()
		next, loadErr := s.nextQueuedMessage(taskID)
		if loadErr != nil {
			next = nil
		}
		continuing := err == nil && next != nil
		status := s.endTurn(taskID, err, continuing)
		if status == StatusCanceled || status == StatusFailed || !continuing || next == nil {
			return
		}
		text, messageID = next.Content, next.ID
	}
}

// beginTurn marks the task running. It returns false when the task was
// canceled before the goroutine could start.
func (s *Service) beginTurn(taskID, messageID string) bool {
	now := s.now()
	proceed := false
	s.mutate(taskID, func(task *Task) {
		if task.CancelRequested {
			proceed = false
			return
		}
		proceed = true
		task.Status = StatusRunning
		if task.StartedAt == nil {
			task.StartedAt = &now
		}
		task.EndedAt = nil
		task.Error = ""
		task.WaitingReason = ""
		task.Approval = nil
	})
	// Delivery is recorded only when the kernel emits the user-message event
	// (handleEvent). A turn that fails before that point keeps the message
	// queued until endTurn marks it undelivered.
	return proceed
}

// endTurn finalizes a turn and returns the task status afterwards. When a
// queued message follows a successful turn, the task stays running instead of
// briefly reporting completion.
func (s *Service) endTurn(taskID string, runErr error, continuing bool) string {
	now := s.now()
	var status string
	s.mutate(taskID, func(task *Task) {
		switch {
		case runErr == nil && task.Status == StatusCompleted:
			// The kernel already reported a completed turn. A cancel request
			// that arrived after that must not relabel finished work.
			status = StatusCompleted
			task.Error = ""
		case task.CancelRequested || errors.Is(runErr, context.Canceled):
			status = StatusCanceled
			task.Error = "任务已取消"
		case runErr != nil:
			status = StatusFailed
			task.Error = runErr.Error()
		case task.Status == StatusFailed || task.Status == StatusCanceled:
			status = task.Status
		case continuing:
			status = StatusRunning
			task.Error = ""
		default:
			status = StatusCompleted
			task.Error = ""
		}
		task.Status = status
		task.WaitingReason = ""
		task.Approval = nil
		task.CancelRequested = false
		if !continuing {
			task.EndedAt = &now
		}
		if task.Stats.DurationMs == 0 && task.StartedAt != nil {
			task.Stats.DurationMs = now.Sub(*task.StartedAt).Milliseconds()
		}
	})
	// On success runLoop drains the queue (including messages that arrived just
	// before the turn finished). A failed or canceled turn has no next turn, so
	// its waiting messages must be reported as undelivered instead of stuck.
	if status == StatusFailed {
		s.failQueuedMessages(taskID, "本轮执行失败，消息未送达")
	} else if status == StatusCanceled {
		s.failQueuedMessages(taskID, "任务已取消，消息未送达")
	}
	return status
}

// handleEvent applies one kernel event to the task. It returns the updated
// delivery flag so the caller knows the user message reached the model.
func (s *Service) handleEvent(taskID string, event agent.Event, messageID string, delivered bool) bool {
	switch event.Type {
	case "session":
		s.mutate(taskID, func(task *Task) {
			if event.RunID != "" {
				task.RunID = event.RunID
			}
			if task.Status != StatusRunning {
				task.Status = StatusRunning
			}
		})
		if messageID != "" && !delivered {
			s.markMessage(taskID, messageID, func(message *TaskMessage) {
				if message.Status != MessageQueued {
					return
				}
				now := s.now()
				message.Status = MessageDelivered
				message.DeliveredAt = &now
			})
			delivered = true
		}
	case "activity":
		if event.Activity == nil {
			return delivered
		}
		activity := event.Activity
		s.mutate(taskID, func(task *Task) {
			task.LatestActivity = &TaskActivity{Kind: activity.Kind, Title: activity.Title, Detail: activity.Detail, Status: activity.Status, StartedAt: activity.StartedAt, EndedAt: activity.EndedAt}
		})
		if activity.Status == "completed" {
			s.recordArtifact(taskID, activity)
		}
	case "run_stats":
		if event.RunStats == nil {
			return delivered
		}
		// A final stats event can arrive after the terminal status event; it
		// must not move a finished task back to running.
		if event.RunStats.EndedAt != nil && !s.taskRunnable(taskID) {
			return delivered
		}
		stats := statsFromRun(*event.RunStats)
		s.mutate(taskID, func(task *Task) {
			task.Stats = stats
			if event.RunID != "" {
				task.RunID = event.RunID
			}
		})
	case "approval":
		if event.Approval == nil {
			return delivered
		}
		s.mutate(taskID, func(task *Task) {
			task.Status = StatusWaiting
			task.WaitingReason = "等待用户确认高危操作"
			task.Approval = &TaskApproval{ID: event.Approval.ID, Tool: event.Approval.Tool, Detail: event.Approval.Detail}
		})
	case "status":
		if event.Status == "" {
			return delivered
		}
		s.mutate(taskID, func(task *Task) {
			task.Status = event.Status
			task.Error = event.Error
			task.WaitingReason = ""
			task.Approval = nil
		})
	}
	return delivered
}

func (s *Service) CancelTask(projectPath, taskID string) error {
	task, err := s.projectTask(projectPath, taskID)
	if err != nil {
		return err
	}
	if isTerminal(task.Status) {
		return ErrNotRunning
	}
	s.mu.Lock()
	cancel := s.runs[taskID]
	s.mu.Unlock()
	if cancel != nil {
		s.mutate(taskID, func(task *Task) { task.CancelRequested = true })
		if task.SessionID != "" {
			s.exec.Cancel(task.SessionID)
		}
		cancel()
		return nil
	}
	now := s.now()
	s.mutate(taskID, func(task *Task) {
		task.Status = StatusCanceled
		task.Error = "任务已取消"
		task.EndedAt = &now
	})
	s.failQueuedMessages(taskID, "任务已取消，消息未送达")
	return nil
}

func (s *Service) failQueuedMessage(taskID, messageID, reason string) {
	if messageID == "" {
		return
	}
	s.markMessage(taskID, messageID, func(message *TaskMessage) {
		if message.Status != MessageQueued {
			return
		}
		message.Status = MessageFailed
		message.Error = reason
	})
}

func (s *Service) failQueuedMessages(taskID, reason string) {
	messages, err := s.loadQueuedMessages(taskID)
	if err != nil {
		return
	}
	for _, message := range messages {
		s.failQueuedMessage(taskID, message.ID, reason)
	}
}

func (s *Service) ResolveApproval(projectPath, taskID, approvalID string, approved bool) error {
	task, err := s.projectTask(projectPath, taskID)
	if err != nil {
		return err
	}
	if task.SessionID == "" {
		return fmt.Errorf("任务还没有执行会话")
	}
	if err = s.exec.ResolveApproval(task.SessionID, approvalID, approved); err != nil {
		return err
	}
	s.mutate(taskID, func(task *Task) {
		task.Approval = nil
		task.WaitingReason = ""
		if task.Status == StatusWaiting {
			task.Status = StatusRunning
		}
	})
	return nil
}

// mutate applies fn to the freshest copy of a task and persists it. Active
// tasks stay in memory so polling sees live counters between writes.
func (s *Service) mutate(taskID string, fn func(*Task)) *Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.active[taskID]
	if !ok {
		loaded, err := s.loadTask(taskID)
		if err != nil {
			return nil
		}
		task = loaded
	}
	fn(task)
	task.UpdatedAt = s.now()
	if isTerminal(task.Status) {
		delete(s.active, taskID)
	} else {
		s.active[taskID] = task
	}
	// Persist while still holding the lock: office writes are tiny, and an
	// out-of-order write from two goroutines would otherwise overwrite newer
	// state with an older snapshot.
	if db, err := s.openStore(); err == nil {
		if err = s.writeTask(db, task); err != nil {
			logging.Default().For(logging.ModuleAgent).Warn("office task save failed", logging.F("task", taskID), logging.F("error", err))
		}
		db.Close()
	}
	return snapshotTask(task)
}

func (s *Service) taskRunnable(taskID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.active[taskID]
	if !ok {
		return false
	}
	return !isTerminal(task.Status)
}

func snapshotTask(task *Task) *Task {
	copied := *task
	return &copied
}

func (s *Service) projectTask(projectPath, taskID string) (*Task, error) {
	task, err := s.loadTask(taskID)
	if err != nil {
		return nil, err
	}
	if projectKey(task.ProjectPath) != projectKey(projectPath) {
		return nil, ErrTaskNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if live, ok := s.active[task.ID]; ok {
		return snapshotTask(live), nil
	}
	return task, nil
}
