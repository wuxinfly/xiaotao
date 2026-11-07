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
// The bounded tails are merged by time while preserving each table's stored order.
func (s *Service) History(id, before string) (*HistoryPage, error) {
	limits := [4]int64{math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64}
	if before != "" {
		parts := strings.Split(before, ":")
		if len(parts) != 2 && len(parts) != 4 {
			return nil, fmt.Errorf("历史游标无效")
		}
		for i := range limits {
			value := "0"
			if i < len(parts) {
				value = parts[i]
			}
			n, err := strconv.ParseInt(value, 10, 64)
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
	item.Thoughts = []ThoughtStep{}
	item.RunStats = []RunStats{}
	type record struct {
		pos      int64
		raw      []byte
		at       time.Time
		message  Message
		activity Activity
		thought  ThoughtStep
		stats    RunStats
	}
	var streams [4][]record
	for i, table := range []string{"messages", "activities", "thoughts", "run_stats"} {
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
			} else if i == 1 {
				e = json.Unmarshal(r.raw, &r.activity)
				r.at = r.activity.StartedAt
			} else if i == 2 {
				e = json.Unmarshal(r.raw, &r.thought)
				r.at = r.thought.CreatedAt
			} else {
				e = json.Unmarshal(r.raw, &r.stats)
				r.at = r.stats.StartedAt
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
	indices := [4]int{}
	bytes := 0
	for count := 0; count < historyPageSize; count++ {
		i := -1
		for stream := range streams {
			if indices[stream] == len(streams[stream]) {
				continue
			}
			if i < 0 || streams[stream][indices[stream]].at.After(streams[i][indices[i]].at) {
				i = stream
			}
		}
		if i < 0 {
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
		} else if i == 1 {
			item.Activity = append(item.Activity, r.activity)
		} else if i == 2 {
			item.Thoughts = append(item.Thoughts, r.thought)
		} else {
			item.RunStats = append(item.RunStats, r.stats)
		}
	}
	// Restore the original per-stream order for the frontend and SSE merging.
	reverse(item.Messages)
	reverse(item.Activity)
	reverse(item.Thoughts)
	reverse(item.RunStats)
	next := ""
	for i := range streams {
		if indices[i] < len(streams[i]) {
			next = fmt.Sprintf("%d:%d:%d:%d", limits[0], limits[1], limits[2], limits[3])
			break
		}
	}
	// Run descriptors are supporting metadata: a page may begin after its user
	// message and original stats position. Hydrate only runs represented here,
	// without moving the record cursors or loading the full conversation.
	runIDs := map[string]bool{}
	for _, m := range item.Messages {
		if m.RunID != "" {
			runIDs[m.RunID] = true
		}
	}
	for _, a := range item.Activity {
		if a.RunID != "" {
			runIDs[a.RunID] = true
		}
	}
	for _, thought := range item.Thoughts {
		if thought.RunID != "" {
			runIDs[thought.RunID] = true
		}
	}
	for _, stats := range item.RunStats {
		delete(runIDs, stats.RunID)
	}
	if len(runIDs) > 0 {
		args := []any{id}
		marks := []string{}
		for runID := range runIDs {
			args = append(args, runID)
			marks = append(marks, "?")
		}
		rows, e := tx.Query("SELECT data FROM run_stats WHERE session_id=? AND json_extract(data,'$.runId') IN ("+strings.Join(marks, ",")+") ORDER BY position", args...)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var raw string
			var stats RunStats
			if e = rows.Scan(&raw); e == nil {
				e = json.Unmarshal([]byte(raw), &stats)
			}
			if e != nil {
				rows.Close()
				return nil, e
			}
			item.RunStats = append(item.RunStats, stats)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	s.markDefault(item)
	return &HistoryPage{Session: item, Before: next}, tx.Commit()
}

func reverse[T any](items []T) {
	for l, r := 0, len(items)-1; l < r; l, r = l+1, r-1 {
		items[l], items[r] = items[r], items[l]
	}
}
