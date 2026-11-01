package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) openStore() (*sql.DB, error) {
	if err := os.MkdirAll(s.dataDir, 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(s.dataDir, "agent.db"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*sql.DB, error) { db.Close(); return nil, e }
	_, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, updated_at TEXT NOT NULL, metadata TEXT NOT NULL); CREATE TABLE IF NOT EXISTS messages (session_id TEXT NOT NULL, position INTEGER NOT NULL, data TEXT NOT NULL, PRIMARY KEY(session_id,position)); CREATE TABLE IF NOT EXISTS activities (session_id TEXT NOT NULL, position INTEGER NOT NULL, data TEXT NOT NULL, PRIMARY KEY(session_id,position)); CREATE TABLE IF NOT EXISTS migration (name TEXT PRIMARY KEY)")
	if err != nil {
		return fail(err)
	}
	var done int
	if err = db.QueryRow("SELECT count(*) FROM migration WHERE name='json-v1'").Scan(&done); err != nil {
		return fail(err)
	}
	if done == 0 {
		entries, e := os.ReadDir(filepath.Join(s.dataDir, "sessions"))
		if e != nil && !os.IsNotExist(e) {
			return fail(e)
		}
		tx, e := db.Begin()
		if e != nil {
			return fail(e)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			raw, e := os.ReadFile(filepath.Join(s.dataDir, "sessions", entry.Name()))
			if e != nil {
				tx.Rollback()
				return fail(e)
			}
			var item Session
			if e = json.Unmarshal(raw, &item); e != nil || item.ID == "" || entry.Name() != item.ID+".json" {
				tx.Rollback()
				return fail(fmt.Errorf("旧会话 %s 无法导入，请检查原文件", entry.Name()))
			}
			var exists int
			if e = tx.QueryRow("SELECT count(*) FROM sessions WHERE id=?", item.ID).Scan(&exists); e != nil {
				tx.Rollback()
				return fail(e)
			}
			if exists == 0 {
				if e = writeSession(tx, &item); e != nil {
					tx.Rollback()
					return fail(e)
				}
			}
		}
		if _, e = tx.Exec("INSERT OR IGNORE INTO migration(name) VALUES('json-v1')"); e != nil {
			tx.Rollback()
			return fail(e)
		}
		if e = tx.Commit(); e != nil {
			return fail(e)
		}
	}
	return db, nil
}
func writeSession(tx *sql.Tx, item *Session) error {
	meta := *item
	meta.Messages = nil
	meta.Activity = nil
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO sessions(id,updated_at,metadata) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at,metadata=excluded.metadata", item.ID, item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), string(raw)); err != nil {
		return err
	}
	for _, spec := range []struct {
		table string
		items any
	}{{"messages", item.Messages}, {"activities", item.Activity}} {
		var entries []json.RawMessage
		b, e := json.Marshal(spec.items)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &entries); e != nil {
			return e
		}
		for i, entry := range entries {
			if _, e = tx.Exec("INSERT INTO "+spec.table+"(session_id,position,data) VALUES(?,?,?) ON CONFLICT(session_id,position) DO UPDATE SET data=excluded.data WHERE data<>excluded.data", item.ID, i, string(entry)); e != nil {
				return e
			}
		}
		if _, e = tx.Exec("DELETE FROM "+spec.table+" WHERE session_id=? AND position>=?", item.ID, len(entries)); e != nil {
			return e
		}
	}
	return nil
}
func (s *Service) saveSession(item *Session) error {
	db, e := s.openStore()
	if e != nil {
		return e
	}
	defer db.Close()
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if e = writeSession(tx, item); e != nil {
		return e
	}
	return tx.Commit()
}
func readSession(db *sql.DB, id string) (*Session, error) {
	var raw string
	if e := db.QueryRow("SELECT metadata FROM sessions WHERE id=?", id).Scan(&raw); e != nil {
		if e == sql.ErrNoRows {
			return nil, fmt.Errorf("任务不存在")
		}
		return nil, e
	}
	var item Session
	if e := json.Unmarshal([]byte(raw), &item); e != nil {
		return nil, e
	}
	item.Messages = []Message{}
	item.Activity = []Activity{}
	for _, table := range []string{"messages", "activities"} {
		rows, e := db.Query("SELECT data FROM "+table+" WHERE session_id=? ORDER BY position", id)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var raw string
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return nil, e
			}
			if table == "messages" {
				var m Message
				e = json.Unmarshal([]byte(raw), &m)
				item.Messages = append(item.Messages, m)
			} else {
				var a Activity
				e = json.Unmarshal([]byte(raw), &a)
				item.Activity = append(item.Activity, a)
			}
			if e != nil {
				rows.Close()
				return nil, e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	return &item, nil
}
func (s *Service) loadSession(id string) (*Session, error) {
	db, e := s.openStore()
	if e != nil {
		return nil, e
	}
	defer db.Close()
	return readSession(db, id)
}
func (s *Service) listSessions() ([]*Session, error) {
	db, e := s.openStore()
	if e != nil {
		return nil, e
	}
	defer db.Close()
	rows, e := db.Query("SELECT id FROM sessions ORDER BY updated_at DESC")
	if e != nil {
		return nil, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []*Session{}
	for _, id := range ids {
		item, e := readSession(db, id)
		if e != nil {
			return nil, e
		}
		out = append(out, item)
	}
	return out, nil
}
