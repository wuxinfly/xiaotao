package office

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

const schema = `
PRAGMA busy_timeout=5000;
PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS office_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS office_chats (project_key TEXT NOT NULL, member_id TEXT NOT NULL, data TEXT NOT NULL, PRIMARY KEY(project_key, member_id));
CREATE TABLE IF NOT EXISTS office_tasks (
	id TEXT PRIMARY KEY,
	project_key TEXT NOT NULL,
	project_path TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS office_tasks_project ON office_tasks(project_key, updated_at DESC);
CREATE TABLE IF NOT EXISTS office_messages (
	id TEXT PRIMARY KEY,
	task_id TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	data TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS office_messages_task ON office_messages(task_id, created_at);
CREATE TABLE IF NOT EXISTS office_artifacts (
	task_id TEXT NOT NULL,
	path TEXT NOT NULL,
	data TEXT NOT NULL,
	PRIMARY KEY(task_id, path)
);
CREATE INDEX IF NOT EXISTS office_artifacts_task ON office_artifacts(task_id);
`

func (s *Service) openStore() (*sql.DB, error) {
	if err := os.MkdirAll(s.dataDir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(s.dataDir, "office.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化办公室数据库: %w", err)
	}
	if _, err = db.Exec("INSERT OR IGNORE INTO office_meta(key,value) VALUES('schema_version',?)", fmt.Sprint(schemaVersion)); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func projectKey(path string) string {
	return strings.ToLower(strings.ReplaceAll(filepath.Clean(path), `\`, "/"))
}

func encodeTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Service) writeTask(db *sql.DB, task *Task) error {
	raw, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO office_tasks(id,project_key,project_path,updated_at,data) VALUES(?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET project_key=excluded.project_key,project_path=excluded.project_path,updated_at=excluded.updated_at,data=excluded.data`,
		task.ID, projectKey(task.ProjectPath), task.ProjectPath, encodeTime(task.UpdatedAt), string(raw))
	return err
}

func readTaskRow(scan func(...any) error) (*Task, error) {
	var raw string
	if err := scan(&raw); err != nil {
		return nil, err
	}
	var task Task
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		return nil, err
	}
	return &task, nil
}

func (s *Service) loadTask(id string) (*Task, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	row := db.QueryRow("SELECT data FROM office_tasks WHERE id=?", id)
	task, err := readTaskRow(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrTaskNotFound
	}
	return task, err
}

func (s *Service) loadProjectTasks(path string) ([]*Task, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT data FROM office_tasks WHERE project_key=? ORDER BY updated_at DESC", projectKey(path))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Task{}
	for rows.Next() {
		task, err := readTaskRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *Service) loadAllTasks() ([]*Task, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT data FROM office_tasks ORDER BY updated_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Task{}
	for rows.Next() {
		task, err := readTaskRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *Service) writeMessage(db *sql.DB, message *TaskMessage) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO office_messages(id,task_id,status,created_at,data) VALUES(?,?,?,?,?)
ON CONFLICT(id) DO UPDATE SET status=excluded.status,data=excluded.data`,
		message.ID, message.TaskID, message.Status, encodeTime(message.CreatedAt), string(raw))
	return err
}

func (s *Service) loadMessages(taskID string) ([]*TaskMessage, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT data FROM office_messages WHERE task_id=? ORDER BY created_at", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*TaskMessage{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var message TaskMessage
		if err = json.Unmarshal([]byte(raw), &message); err != nil {
			return nil, err
		}
		out = append(out, &message)
	}
	return out, rows.Err()
}

func (s *Service) loadQueuedMessages(taskID string) ([]*TaskMessage, error) {
	all, err := s.loadMessages(taskID)
	if err != nil {
		return nil, err
	}
	out := []*TaskMessage{}
	for _, message := range all {
		if message.Status == MessageQueued {
			out = append(out, message)
		}
	}
	return out, nil
}

// queuedCounts returns undelivered message counts per task for one project.
func (s *Service) queuedCounts(projectPath string) (map[string]int, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT m.task_id, count(*) FROM office_messages m
JOIN office_tasks t ON t.id=m.task_id
WHERE t.project_key=? AND m.status=? GROUP BY m.task_id`, projectKey(projectPath), MessageQueued)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var count int
		if err = rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	return out, rows.Err()
}

func (s *Service) writeArtifact(db *sql.DB, artifact *Artifact) error {
	raw, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO office_artifacts(task_id,path,data) VALUES(?,?,?)
ON CONFLICT(task_id,path) DO UPDATE SET data=excluded.data`, artifact.TaskID, artifact.Path, string(raw))
	return err
}

func (s *Service) loadTaskArtifacts(taskID string) ([]*Artifact, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query("SELECT data FROM office_artifacts WHERE task_id=? ORDER BY path", taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Artifact{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var artifact Artifact
		if err = json.Unmarshal([]byte(raw), &artifact); err != nil {
			return nil, err
		}
		out = append(out, &artifact)
	}
	return out, rows.Err()
}

// loadProjectArtifacts joins artifacts with their tasks so a shelf never shows
// an artifact belonging to another project.
func (s *Service) loadProjectArtifacts(path string) ([]*Artifact, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT a.data FROM office_artifacts a
JOIN office_tasks t ON t.id=a.task_id WHERE t.project_key=? ORDER BY a.task_id, a.path`, projectKey(path))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Artifact{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var artifact Artifact
		if err = json.Unmarshal([]byte(raw), &artifact); err != nil {
			return nil, err
		}
		out = append(out, &artifact)
	}
	return out, rows.Err()
}

func (s *Service) artifactByID(projectPath, id string) (*Artifact, error) {
	items, err := s.loadProjectArtifacts(projectPath)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, ErrArtifactNotFound
}
