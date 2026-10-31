package usage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var shanghaiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}()

// DateInShanghai returns YYYY-MM-DD for the given time in Asia/Shanghai.
func DateInShanghai(t time.Time) string {
	return t.In(shanghaiLoc).Format("2006-01-02")
}

type DB struct {
	db *sql.DB
	mu sync.Mutex
}

func OpenDB(dbPath string) (*DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create db dir %s: %w", dir, err)
	}

	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}

	// Ensure serialized writes on this DB handle
	db.SetMaxOpenConns(1)

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.db.Close()
}

func initSchema(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS request_records (
			request_id TEXT PRIMARY KEY,
			batch_id TEXT NOT NULL,
			channel TEXT NOT NULL DEFAULT 'trae',
			model TEXT NOT NULL,
			account_id TEXT NOT NULL,
			action_tag TEXT DEFAULT '',
			action_title TEXT DEFAULT '',
			created_at INTEGER NOT NULL,
			first_token_at INTEGER,
			ended_at INTEGER,
			status TEXT NOT NULL,
			prompt_tokens INTEGER,
			completion_tokens INTEGER,
			cached_tokens INTEGER,
			error_summary TEXT,
			is_finalized INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_records_created_at ON request_records(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_records_status ON request_records(status);`,
		`CREATE INDEX IF NOT EXISTS idx_records_batch ON request_records(batch_id);`,

		`CREATE TABLE IF NOT EXISTS daily_usage (
			date TEXT PRIMARY KEY,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			cached_tokens INTEGER NOT NULL DEFAULT 0,
			total_requests INTEGER NOT NULL DEFAULT 0,
			completed_requests INTEGER NOT NULL DEFAULT 0,
			failed_requests INTEGER NOT NULL DEFAULT 0,
			interrupted_requests INTEGER NOT NULL DEFAULT 0,
			missing_usage_requests INTEGER NOT NULL DEFAULT 0
		);`,

		`CREATE TABLE IF NOT EXISTS usage_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	}

	// Ensure migration columns exist for previously created databases
	_, _ = db.Exec(`ALTER TABLE request_records ADD COLUMN channel TEXT NOT NULL DEFAULT 'trae'`)
	_, _ = db.Exec(`ALTER TABLE request_records ADD COLUMN action_tag TEXT DEFAULT ''`)
	_, _ = db.Exec(`ALTER TABLE request_records ADD COLUMN action_title TEXT DEFAULT ''`)
	_, _ = db.Exec(`UPDATE request_records SET action_tag = '用户' WHERE action_tag = '' OR action_tag IS NULL`)
	_, _ = db.Exec(`UPDATE request_records SET action_title = '日常对话' WHERE action_title = '' OR action_title = '模型请求' OR action_title IS NULL`)
	_, _ = db.Exec(`
		UPDATE daily_usage
		SET completion_tokens = (
			SELECT COALESCE(SUM(completion_tokens), 0)
			FROM request_records
			WHERE strftime('%Y-%m-%d', created_at / 1000, 'unixepoch', '+8 hours') = daily_usage.date
		)
		WHERE completion_tokens < 0
	`)

	// Ensure tracking_started_date is saved
	today := DateInShanghai(time.Now())
	_, err := db.Exec(`INSERT OR IGNORE INTO usage_meta (key, value) VALUES ('tracking_started_date', ?)`, today)
	if err != nil {
		return fmt.Errorf("set tracking_started_date: %w", err)
	}

	return nil
}

func (d *DB) GetMeta(key string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var val string
	err := d.db.QueryRow(`SELECT value FROM usage_meta WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (d *DB) SetMeta(key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, err := d.db.Exec(`INSERT INTO usage_meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// RecoverUnfinishedRequests updates dangling requests from previous batches to interrupted.
func (d *DB) RecoverUnfinishedRequests(currentBatchID string, now time.Time) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT request_id, created_at, prompt_tokens, completion_tokens, is_finalized
		FROM request_records
		WHERE status IN ('pending', 'streaming') AND batch_id != ?
	`, currentBatchID)
	if err != nil {
		return 0, err
	}

	type unfin struct {
		id          string
		created     int64
		hasPrompt   bool
		hasComplete bool
		finalized   bool
	}
	var list []unfin
	for rows.Next() {
		var u unfin
		var p, c sql.NullInt64
		var fin int
		if err := rows.Scan(&u.id, &u.created, &p, &c, &fin); err != nil {
			rows.Close()
			return 0, err
		}
		u.hasPrompt = p.Valid
		u.hasComplete = c.Valid
		u.finalized = (fin == 1)
		list = append(list, u)
	}
	rows.Close()

	nowMs := now.UnixMilli()
	for _, u := range list {
		_, err := tx.Exec(`
			UPDATE request_records
			SET status = 'interrupted', ended_at = ?, error_summary = '服务重启中断', is_finalized = 1
			WHERE request_id = ?
		`, nowMs, u.id)
		if err != nil {
			return 0, err
		}

		if !u.finalized {
			startDate := DateInShanghai(time.UnixMilli(u.created))
			missing := 0
			if !u.hasPrompt && !u.hasComplete {
				missing = 1
			}
			_, err = tx.Exec(`
				INSERT INTO daily_usage (date, total_requests, interrupted_requests, missing_usage_requests)
				VALUES (?, 1, 1, ?)
				ON CONFLICT(date) DO UPDATE SET
					total_requests = total_requests + 1,
					interrupted_requests = interrupted_requests + 1,
					missing_usage_requests = missing_usage_requests + excluded.missing_usage_requests
			`, startDate, missing)
			if err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(list)), nil
}

// CleanupExpiredRequests removes ended requests older than the given cutoff.
func (d *DB) CleanupExpiredRequests(cutoff time.Time) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoffMs := cutoff.UnixMilli()
	res, err := d.db.Exec(`
		DELETE FROM request_records
		WHERE is_finalized = 1
		  AND status IN ('completed', 'failed', 'interrupted', 'canceled')
		  AND created_at < ?
	`, cutoffMs)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CommitRequestAndUsage updates a record and optionally applies usage and completion stats to daily_usage.
func (d *DB) CommitRequestAndUsage(
	rec *Record,
	confirmedDate string,
	deltaPrompt, deltaCompletion, deltaCached int64,
	isFirstFinal bool,
	finalStatus string,
) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var firstTokenMs, endedMs sql.NullInt64
	if rec.FirstTokenAt != nil {
		firstTokenMs = sql.NullInt64{Int64: rec.FirstTokenAt.UnixMilli(), Valid: true}
	}
	if rec.EndedAt != nil {
		endedMs = sql.NullInt64{Int64: rec.EndedAt.UnixMilli(), Valid: true}
	}

	var promptVal, completionVal, cachedVal sql.NullInt64
	if rec.PromptTokens != nil {
		promptVal = sql.NullInt64{Int64: *rec.PromptTokens, Valid: true}
	}
	if rec.CompletionTokens != nil {
		completionVal = sql.NullInt64{Int64: *rec.CompletionTokens, Valid: true}
	}
	if rec.CachedTokens != nil {
		cachedVal = sql.NullInt64{Int64: *rec.CachedTokens, Valid: true}
	}

	finalizedInt := 0
	if isFirstFinal || rec.Status == StatusCompleted || rec.Status == StatusFailed || rec.Status == StatusInterrupted || rec.Status == StatusCanceled {
		finalizedInt = 1
	}

	channelVal := rec.Channel
	if channelVal == "" {
		channelVal = "trae"
	}

	_, err = tx.Exec(`
		INSERT INTO request_records (
			request_id, batch_id, channel, model, account_id,
			action_tag, action_title, created_at,
			first_token_at, ended_at, status, prompt_tokens, completion_tokens,
			cached_tokens, error_summary, is_finalized
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(request_id) DO UPDATE SET
			channel = excluded.channel,
			action_tag = CASE WHEN excluded.action_tag != '' THEN excluded.action_tag ELSE request_records.action_tag END,
			action_title = CASE WHEN excluded.action_title != '' THEN excluded.action_title ELSE request_records.action_title END,
			first_token_at = COALESCE(excluded.first_token_at, request_records.first_token_at),
			ended_at = COALESCE(excluded.ended_at, request_records.ended_at),
			status = excluded.status,
			prompt_tokens = COALESCE(excluded.prompt_tokens, request_records.prompt_tokens),
			completion_tokens = COALESCE(excluded.completion_tokens, request_records.completion_tokens),
			cached_tokens = COALESCE(excluded.cached_tokens, request_records.cached_tokens),
			error_summary = excluded.error_summary,
			is_finalized = MAX(request_records.is_finalized, excluded.is_finalized)
	`,
		rec.RequestID, rec.BatchID, channelVal, rec.Model, rec.AccountID,
		rec.ActionTag, rec.ActionTitle, rec.CreatedAt.UnixMilli(),
		firstTokenMs, endedMs, rec.Status, promptVal, completionVal,
		cachedVal, rec.ErrorSummary, finalizedInt,
	)
	if err != nil {
		return fmt.Errorf("upsert request_record: %w", err)
	}

	// Apply token deltas to confirmedDate
	if deltaPrompt > 0 || deltaCompletion > 0 || deltaCached > 0 {
		_, err = tx.Exec(`
			INSERT INTO daily_usage (date, prompt_tokens, completion_tokens, cached_tokens)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(date) DO UPDATE SET
				prompt_tokens = prompt_tokens + excluded.prompt_tokens,
				completion_tokens = completion_tokens + excluded.completion_tokens,
				cached_tokens = cached_tokens + excluded.cached_tokens
		`, confirmedDate, deltaPrompt, deltaCompletion, deltaCached)
		if err != nil {
			return fmt.Errorf("update daily_usage tokens: %w", err)
		}
	}

	// If finalized for the first time, settle request count and terminal status on startDate
	if isFirstFinal {
		startDate := DateInShanghai(rec.CreatedAt)
		completedInc := 0
		failedInc := 0
		interruptedInc := 0
		switch finalStatus {
		case StatusCompleted:
			completedInc = 1
		case StatusFailed, StatusCanceled:
			failedInc = 1
		case StatusInterrupted:
			interruptedInc = 1
		}

		missingInc := 0
		if rec.PromptTokens == nil && rec.CompletionTokens == nil {
			missingInc = 1
		}

		_, err = tx.Exec(`
			INSERT INTO daily_usage (
				date, total_requests, completed_requests, failed_requests, interrupted_requests, missing_usage_requests
			) VALUES (?, 1, ?, ?, ?, ?)
			ON CONFLICT(date) DO UPDATE SET
				total_requests = total_requests + 1,
				completed_requests = completed_requests + excluded.completed_requests,
				failed_requests = failed_requests + excluded.failed_requests,
				interrupted_requests = interrupted_requests + excluded.interrupted_requests,
				missing_usage_requests = missing_usage_requests + excluded.missing_usage_requests
		`, startDate, completedInc, failedInc, interruptedInc, missingInc)
		if err != nil {
			return fmt.Errorf("update daily_usage requests: %w", err)
		}
	}

	return tx.Commit()
}

func (d *DB) LoadCumulativeStats() (*CumulativeStats, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var prompt, completion, cached, reqs sql.NullInt64
	err := d.db.QueryRow(`
		SELECT
			SUM(prompt_tokens),
			SUM(completion_tokens),
			SUM(cached_tokens),
			SUM(total_requests)
		FROM daily_usage
	`).Scan(&prompt, &completion, &cached, &reqs)
	if err != nil {
		return nil, err
	}

	p := prompt.Int64
	c := completion.Int64
	return &CumulativeStats{
		TotalTokens:      p + c,
		PromptTokens:     p,
		CompletionTokens: c,
		CachedTokens:     cached.Int64,
		TotalRequests:    reqs.Int64,
	}, nil
}

func (d *DB) LoadDailyUsage(date string) (*DailyUsage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	out := &DailyUsage{Date: date}
	row := d.db.QueryRow(`
		SELECT prompt_tokens, completion_tokens, cached_tokens,
		       total_requests, completed_requests, failed_requests,
		       interrupted_requests, missing_usage_requests
		FROM daily_usage
		WHERE date = ?
	`, date)
	err := row.Scan(
		&out.PromptTokens, &out.CompletionTokens, &out.CachedTokens,
		&out.TotalRequests, &out.CompletedRequests, &out.FailedRequests,
		&out.InterruptedRequests, &out.MissingUsageRequests,
	)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (d *DB) LoadHeatmapDays(startDate, endDate string) (map[string]*DailyUsage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT date, prompt_tokens, completion_tokens, cached_tokens,
		       total_requests, completed_requests, failed_requests,
		       interrupted_requests, missing_usage_requests
		FROM daily_usage
		WHERE date >= ? AND date <= ?
	`, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := make(map[string]*DailyUsage)
	for rows.Next() {
		d := &DailyUsage{}
		if err := rows.Scan(
			&d.Date, &d.PromptTokens, &d.CompletionTokens, &d.CachedTokens,
			&d.TotalRequests, &d.CompletedRequests, &d.FailedRequests,
			&d.InterruptedRequests, &d.MissingUsageRequests,
		); err != nil {
			return nil, err
		}
		res[d.Date] = d
	}
	return res, rows.Err()
}

func (d *DB) LoadRecentRecords(limit int) ([]*Record, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`
		SELECT request_id, batch_id, COALESCE(channel, 'trae'), model, account_id,
		       COALESCE(action_tag, ''), COALESCE(action_title, ''), created_at,
		       first_token_at, ended_at, status, prompt_tokens,
		       completion_tokens, cached_tokens, error_summary
		FROM request_records
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*Record
	for rows.Next() {
		var r Record
		var createdMs int64
		var firstMs, endMs, prompt, completion, cached sql.NullInt64
		var errSummary sql.NullString
		if err := rows.Scan(
			&r.RequestID, &r.BatchID, &r.Channel, &r.Model, &r.AccountID,
			&r.ActionTag, &r.ActionTitle, &createdMs,
			&firstMs, &endMs, &r.Status, &prompt,
			&completion, &cached, &errSummary,
		); err != nil {
			return nil, err
		}
		r.CreatedAt = time.UnixMilli(createdMs)
		if firstMs.Valid {
			t := time.UnixMilli(firstMs.Int64)
			r.FirstTokenAt = &t
		}
		if endMs.Valid {
			t := time.UnixMilli(endMs.Int64)
			r.EndedAt = &t
		}
		if prompt.Valid {
			v := prompt.Int64
			r.PromptTokens = &v
		}
		if completion.Valid {
			v := completion.Int64
			r.CompletionTokens = &v
		}
		if cached.Valid {
			v := cached.Int64
			r.CachedTokens = &v
		}
		r.ErrorSummary = errSummary.String
		list = append(list, &r)
	}
	return list, rows.Err()
}
