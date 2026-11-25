package office

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
)

type conversationMetadata struct {
	TurnID            string
	SummarySession    string
	DeferredSummaries []summaryTurn `json:",omitempty"`
	Dispatching       bool
	ProjectKey        string
}

type storedConversation struct {
	Conversation
	Metadata conversationMetadata
}

func decodeConversation(raw string) (*Conversation, error) {
	var record storedConversation
	if err := json.Unmarshal([]byte(raw), &record); err != nil {
		return nil, err
	}
	record.TurnID = record.Metadata.TurnID
	record.SummarySession = record.Metadata.SummarySession
	record.DeferredSummaries = record.Metadata.DeferredSummaries
	record.Dispatching = record.Metadata.Dispatching
	record.ProjectKey = record.Metadata.ProjectKey
	return &record.Conversation, nil
}

func (s *Service) readConversation(parent, id string) (*Conversation, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var raw string
	err = db.QueryRow("SELECT data FROM office_conversations WHERE parent_id=? AND member_id=?", parent, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return &Conversation{Chat: Chat{MemberID: id, Status: "idle"}}, nil
	}
	if err != nil {
		return nil, err
	}
	return decodeConversation(raw)
}

func (s *Service) listConversations(parent string) ([]Conversation, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT data FROM office_conversations WHERE parent_id=? ORDER BY rowid", parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		entry, err := decodeConversation(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, *entry)
	}
	return out, rows.Err()
}

type officeSQLWriter interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func writeConversation(db officeSQLWriter, parent string, entry *Conversation) error {
	metadata := conversationMetadata{
		TurnID: entry.TurnID, SummarySession: entry.SummarySession, DeferredSummaries: entry.DeferredSummaries,
		Dispatching: entry.Dispatching, ProjectKey: entry.ProjectKey,
	}
	data, err := json.Marshal(storedConversation{Conversation: *entry, Metadata: metadata})
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO office_conversations(parent_id, member_id, data) VALUES (?,?,?) ON CONFLICT(parent_id, member_id) DO UPDATE SET data=excluded.data", parent, entry.MemberID, string(data))
	return err
}

func (s *Service) saveConversation(parent string, entry *Conversation) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	return writeConversation(db, parent, entry)
}

func (s *Service) memoryForEmployee(path, id string) ([]string, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT summary FROM office_employee_memory WHERE project_key=? AND member_id=? ORDER BY created_at DESC, rowid DESC LIMIT 12", projectKey(path), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var item string
		if err = rows.Scan(&item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	slices.Reverse(result)
	return result, rows.Err()
}

func (s *Service) commitConversationMemory(scope conversationScope, entry Conversation, turnID, summary string) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if summary != "" {
		const insert = "INSERT OR IGNORE INTO office_employee_memory(project_key,member_id,turn_id,summary,created_at) SELECT ?,?,?,?,? WHERE NOT EXISTS (SELECT 1 FROM office_employee_memory WHERE project_key=? AND member_id=? AND summary=?)"
		if _, err = tx.Exec(insert, projectKey(scope.path), entry.MemberID, turnID, summary, encodeTime(s.now()), projectKey(scope.path), entry.MemberID, summary); err != nil {
			return err
		}
	}
	entry.SummaryStatus = "saved"
	if err = writeConversation(tx, scope.id, &entry); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) archiveConversation(parent string, entry Conversation) error {
	if entry.SourceMessageID == "" {
		return nil
	}
	entry = s.projectConversation(entry)
	entry.Historical = true
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("INSERT OR IGNORE INTO office_conversation_archive(parent_id,source_id,data) VALUES(?,?,?)", parent, entry.SourceMessageID, string(data))
	return err
}

func (s *Service) archivedConversations(parent string) ([]Conversation, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT archive.data FROM office_conversation_archive archive WHERE archive.parent_id=? AND NOT EXISTS (SELECT 1 FROM office_conversations current WHERE current.parent_id=archive.parent_id AND json_extract(current.data,'$.sourceMessageId')=archive.source_id) ORDER BY archive.rowid", parent)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Conversation{}
	for rows.Next() {
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		var entry Conversation
		if err = json.Unmarshal([]byte(data), &entry); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func (s *Service) projectConversationStatuses(path string, tasks []*Task) (map[string]string, error) {
	statuses := make(map[string]string)
	tasksByID := make(map[string]*Task, len(tasks))
	for _, task := range tasks {
		tasksByID[task.ID] = task
	}
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT data FROM office_conversations WHERE json_extract(data,'$.Metadata.ProjectKey')=?", projectKey(path))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		entry, err := decodeConversation(raw)
		if err != nil {
			return nil, err
		}
		status := entry.Status
		if task := tasksByID[entry.TaskID]; task != nil {
			status = task.Status
		}
		if entry.Question != "" && status == StatusCompleted {
			status = StatusWaiting
		}
		if collaborationStatusPriority(status) > collaborationStatusPriority(statuses[entry.MemberID]) {
			statuses[entry.MemberID] = status
		}
	}
	return statuses, rows.Err()
}

func collaborationStatusPriority(status string) int {
	switch status {
	case StatusWaiting:
		return 4
	case StatusRunning:
		return 3
	case StatusQueued:
		return 2
	case StatusFailed:
		return 1
	default:
		return 0
	}
}
