package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const historyPageSize = 80
const historyPageBytes = 256 * 1024

type HistoryPage struct {
	Session *Session `json:"session"`
	Before  string   `json:"before"`
}

// History reads bounded tails using indexed positions. Cursors are exclusive;
// appending new records cannot shift previously issued cursors.
// The two tails are merged by time while preserving each table's stored order.
func (s *Service) History(id, before string) (*HistoryPage, error) {
	limits := [2]int64{math.MaxInt64, math.MaxInt64}
	if before != "" {
		parts := strings.Split(before, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("历史游标无效")
		}
		for i := range limits {
			n, err := strconv.ParseInt(parts[i], 10, 64)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("历史游标无效")
			}
			limits[i] = n
		}
	}
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	// A single read transaction keeps metadata and both streams consistent.
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var meta string
	if err = tx.QueryRow("SELECT metadata FROM sessions WHERE id=?", id).Scan(&meta); err != nil {
		return nil, fmt.Errorf("读取历史失败: %w", err)
	}
	item := &Session{}
	if err = json.Unmarshal([]byte(meta), item); err != nil {
		return nil, err
	}
	item.Messages = []Message{}
	item.Activity = []Activity{}
	type record struct {
		pos      int64
		raw      []byte
		at       time.Time
		message  Message
		activity Activity
	}
	var streams [2][]record
	for i, table := range []string{"messages", "activities"} {
		rows, e := tx.Query("SELECT position,data FROM "+table+" WHERE session_id=? AND position<? ORDER BY position DESC LIMIT ?", id, limits[i], historyPageSize+1)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var r record
			if e = rows.Scan(&r.pos, &r.raw); e != nil {
				rows.Close()
				return nil, e
			}
			if i == 0 {
				e = json.Unmarshal(r.raw, &r.message)
				r.at = r.message.CreatedAt
			} else {
				e = json.Unmarshal(r.raw, &r.activity)
				r.at = r.activity.StartedAt
			}
			if e != nil {
				rows.Close()
				return nil, e
			}
			streams[i] = append(streams[i], r)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	if before == "" {
		for i := range limits {
			limits[i] = 0
			if len(streams[i]) > 0 {
				limits[i] = streams[i][0].pos + 1
			}
		}
	}
	indices := [2]int{}
	bytes := 0
	for count := 0; count < historyPageSize; count++ {
		i := 0
		if indices[0] == len(streams[0]) {
			i = 1
		} else if indices[1] < len(streams[1]) && !streams[0][indices[0]].at.After(streams[1][indices[1]].at) {
			i = 1
		}
		if indices[i] == len(streams[i]) {
			break
		}
		r := streams[i][indices[i]]
		if count > 0 && bytes+len(r.raw) > historyPageBytes {
			break
		}
		bytes += len(r.raw)
		indices[i]++
		limits[i] = r.pos
		if i == 0 {
			item.Messages = append(item.Messages, r.message)
		} else {
			item.Activity = append(item.Activity, r.activity)
		}
	}
	// Restore the original per-stream order for the frontend and SSE merging.
	for l, r := 0, len(item.Messages)-1; l < r; l, r = l+1, r-1 {
		item.Messages[l], item.Messages[r] = item.Messages[r], item.Messages[l]
	}
	for l, r := 0, len(item.Activity)-1; l < r; l, r = l+1, r-1 {
		item.Activity[l], item.Activity[r] = item.Activity[r], item.Activity[l]
	}
	next := ""
	if indices[0] < len(streams[0]) || indices[1] < len(streams[1]) {
		next = fmt.Sprintf("%d:%d", limits[0], limits[1])
	}
	s.markDefault(item)
	return &HistoryPage{Session: item, Before: next}, tx.Commit()
}
