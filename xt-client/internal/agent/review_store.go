package agent

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrReviewNotFound      = errors.New("复盘记录不存在")
	ErrReviewActiveExists  = errors.New("当前会话已有正在生成的复盘任务")
	ErrReviewGlobalLimit   = errors.New("同时运行的复盘任务已达上限，请稍后重试")
	ErrReviewRunningDelete = errors.New("生成中的复盘需先取消并等待结束后才能删除")
	ErrReviewConflict      = errors.New("请求冲突")
)

func ensureReviewTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS behavior_reviews (
	id TEXT PRIMARY KEY,
	schema_version INTEGER NOT NULL,
	session_id TEXT NOT NULL,
	request_id TEXT NOT NULL UNIQUE,
	attempt_of TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	started_at TEXT NOT NULL DEFAULT '',
	finished_at TEXT NOT NULL DEFAULT '',
	timezone TEXT NOT NULL,
	model TEXT NOT NULL,
	client_version TEXT NOT NULL,
	core_version TEXT NOT NULL,
	prompt_version TEXT NOT NULL,
	project_snapshot TEXT NOT NULL DEFAULT '',
	session_title_snapshot TEXT NOT NULL DEFAULT '',
	scope TEXT NOT NULL,
	evidence_snapshot TEXT NOT NULL,
	raw_evidence_prompt TEXT NOT NULL DEFAULT '',
	result TEXT NOT NULL DEFAULT '',
	error_code TEXT NOT NULL DEFAULT '',
	error_message TEXT NOT NULL DEFAULT '',
	usage TEXT NOT NULL DEFAULT '',
	suggestion_links TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_behavior_reviews_session ON behavior_reviews(session_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_behavior_reviews_status ON behavior_reviews(status);
`)
	return err
}

func recoverInterruptedReviews(db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.Exec(`
UPDATE behavior_reviews
SET status = ?, error_code = 'interrupted', error_message = '服务退出导致复盘中断', finished_at = ?
WHERE status IN (?, ?)
`, ReviewStatusFailed, now, ReviewStatusPending, ReviewStatusRunning)
	return err
}

func parseTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		t, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return nil
		}
	}
	return &t
}

func formatTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func DeriveReviewTitle(createdAt time.Time, tzName string, result *ReviewResult) string {
	tz, err := time.LoadLocation(tzName)
	if err != nil || tz == nil {
		tz = time.FixedZone("CST", 8*3600) // Default to Asia/Shanghai UTC+8
	}
	localDate := createdAt.In(tz).Format("2006-01-02")
	if result != nil && len(result.Issues) > 0 {
		firstTitle := strings.TrimSpace(result.Issues[0].Title)
		if firstTitle != "" && len([]rune(firstTitle)) <= 20 {
			// Avoid sensitive words in title by using sanitized concise phrase
			firstTitle = strings.TrimSuffix(firstTitle, "问题")
			return fmt.Sprintf("%s %s复盘记录", localDate, firstTitle)
		}
	}
	return fmt.Sprintf("%s 小涛行为复盘记录", localDate)
}

func (s *Service) insertReview(r *BehaviorReview, rawEvidencePrompt string) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	scopeBytes, err := json.Marshal(r.Scope)
	if err != nil {
		return err
	}
	evidenceBytes, err := json.Marshal(r.EvidenceSnapshot)
	if err != nil {
		return err
	}

	resultBytes := []byte("")
	if r.Result != nil {
		resultBytes, err = json.Marshal(r.Result)
		if err != nil {
			return err
		}
	}

	usageBytes := []byte("")
	if r.Usage != nil {
		usageBytes, err = json.Marshal(r.Usage)
		if err != nil {
			return err
		}
	}

	linksBytes := []byte("[]")
	if len(r.SuggestionLinks) > 0 {
		linksBytes, err = json.Marshal(r.SuggestionLinks)
		if err != nil {
			return err
		}
	}

	_, err = db.Exec(`
INSERT INTO behavior_reviews (
	id, schema_version, session_id, request_id, attempt_of, status,
	created_at, started_at, finished_at, timezone, model,
	client_version, core_version, prompt_version, project_snapshot,
	session_title_snapshot, scope, evidence_snapshot, raw_evidence_prompt,
	result, error_code, error_message, usage, suggestion_links
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`,
		r.ID, r.SchemaVersion, r.SessionID, r.RequestID, r.AttemptOf, r.Status,
		formatTime(&r.CreatedAt), formatTime(r.StartedAt), formatTime(r.FinishedAt), r.Timezone, r.Model,
		r.ClientVersion, r.CoreVersion, r.PromptVersion, r.ProjectSnapshot,
		r.SessionTitleSnapshot, string(scopeBytes), string(evidenceBytes), rawEvidencePrompt,
		string(resultBytes), r.ErrorCode, r.ErrorMessage, string(usageBytes), string(linksBytes),
	)
	return err
}

func (s *Service) getReview(id string) (*BehaviorReview, string, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, "", err
	}
	defer db.Close()

	var (
		r                                          BehaviorReview
		createdAtRaw, startedAtRaw, finishedAtRaw  string
		scopeRaw, evidenceRaw, rawEvidencePrompt   string
		resultRaw, usageRaw, suggestionLinksRaw    string
	)

	row := db.QueryRow(`
SELECT
	id, schema_version, session_id, request_id, attempt_of, status,
	created_at, started_at, finished_at, timezone, model,
	client_version, core_version, prompt_version, project_snapshot,
	session_title_snapshot, scope, evidence_snapshot, raw_evidence_prompt,
	result, error_code, error_message, usage, suggestion_links
FROM behavior_reviews
WHERE id = ?
`, id)

	err = row.Scan(
		&r.ID, &r.SchemaVersion, &r.SessionID, &r.RequestID, &r.AttemptOf, &r.Status,
		&createdAtRaw, &startedAtRaw, &finishedAtRaw, &r.Timezone, &r.Model,
		&r.ClientVersion, &r.CoreVersion, &r.PromptVersion, &r.ProjectSnapshot,
		&r.SessionTitleSnapshot, &scopeRaw, &evidenceRaw, &rawEvidencePrompt,
		&resultRaw, &r.ErrorCode, &r.ErrorMessage, &usageRaw, &suggestionLinksRaw,
	)
	if err == sql.ErrNoRows {
		return nil, "", ErrReviewNotFound
	}
	if err != nil {
		return nil, "", err
	}

	r.CreatedAt = *parseTime(createdAtRaw)
	r.StartedAt = parseTime(startedAtRaw)
	r.FinishedAt = parseTime(finishedAtRaw)

	if err := json.Unmarshal([]byte(scopeRaw), &r.Scope); err != nil {
		return nil, "", err
	}
	if err := json.Unmarshal([]byte(evidenceRaw), &r.EvidenceSnapshot); err != nil {
		return nil, "", err
	}
	if resultRaw != "" && resultRaw != "null" {
		var res ReviewResult
		if err := json.Unmarshal([]byte(resultRaw), &res); err == nil {
			r.Result = &res
		}
	}
	if usageRaw != "" && usageRaw != "null" {
		var u ReviewUsage
		if err := json.Unmarshal([]byte(usageRaw), &u); err == nil {
			r.Usage = &u
		}
	}
	if suggestionLinksRaw != "" && suggestionLinksRaw != "null" {
		var links []ReviewSuggestionLink
		if err := json.Unmarshal([]byte(suggestionLinksRaw), &links); err == nil {
			r.SuggestionLinks = links
		}
	}

	return &r, rawEvidencePrompt, nil
}

func (s *Service) getReviewByRequestID(requestID string) (*BehaviorReview, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var id string
	err = db.QueryRow("SELECT id FROM behavior_reviews WHERE request_id = ?", requestID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, _, err := s.getReview(id)
	return r, err
}

func (s *Service) getActiveReviewForSession(sessionID string) (*BehaviorReview, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var id string
	err = db.QueryRow(`
SELECT id FROM behavior_reviews
WHERE session_id = ? AND status IN (?, ?)
ORDER BY created_at DESC LIMIT 1
`, sessionID, ReviewStatusPending, ReviewStatusRunning).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r, _, err := s.getReview(id)
	return r, err
}

func (s *Service) countActiveReviews() (int, error) {
	db, err := s.openStore()
	if err != nil {
		return 0, err
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT count(*) FROM behavior_reviews WHERE status IN (?, ?)", ReviewStatusPending, ReviewStatusRunning).Scan(&count)
	return count, err
}

func (s *Service) markReviewRunning(id string, startedAt time.Time) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	res, err := db.Exec(`
UPDATE behavior_reviews
SET status = ?, started_at = ?
WHERE id = ? AND status = ?
`, ReviewStatusRunning, formatTime(&startedAt), id, ReviewStatusPending)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("无法将状态更新为 running")
	}
	return nil
}

func (s *Service) markReviewSucceeded(id string, result *ReviewResult, usage *ReviewUsage, finishedAt time.Time) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return err
	}
	usageBytes, err := json.Marshal(usage)
	if err != nil {
		return err
	}

	// Condition: only allow transition if status is running (avoid race with cancel)
	res, err := db.Exec(`
UPDATE behavior_reviews
SET status = ?, result = ?, usage = ?, finished_at = ?
WHERE id = ? AND status = ?
`, ReviewStatusSucceeded, string(resultBytes), string(usageBytes), formatTime(&finishedAt), id, ReviewStatusRunning)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("复盘任务已被取消或状态已更改")
	}
	return nil
}

func (s *Service) markReviewFailed(id string, errCode, errMsg string, finishedAt time.Time) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
UPDATE behavior_reviews
SET status = ?, error_code = ?, error_message = ?, finished_at = ?
WHERE id = ? AND status IN (?, ?)
`, ReviewStatusFailed, errCode, errMsg, formatTime(&finishedAt), id, ReviewStatusPending, ReviewStatusRunning)
	return err
}

func (s *Service) markReviewCanceled(id string, finishedAt time.Time) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(`
UPDATE behavior_reviews
SET status = ?, finished_at = ?
WHERE id = ? AND status IN (?, ?)
`, ReviewStatusCanceled, formatTime(&finishedAt), id, ReviewStatusPending, ReviewStatusRunning)
	return err
}

func (s *Service) saveReviewSuggestionLinks(id string, links []ReviewSuggestionLink) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	linksBytes, err := json.Marshal(links)
	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE behavior_reviews SET suggestion_links = ? WHERE id = ?", string(linksBytes), id)
	return err
}

func (s *Service) deleteReviewRecord(id string) error {
	db, err := s.openStore()
	if err != nil {
		return err
	}
	defer db.Close()

	var status string
	err = db.QueryRow("SELECT status FROM behavior_reviews WHERE id = ?", id).Scan(&status)
	if err == sql.ErrNoRows {
		return ErrReviewNotFound
	}
	if err != nil {
		return err
	}

	if status == ReviewStatusPending || status == ReviewStatusRunning {
		return ErrReviewRunningDelete
	}

	_, err = db.Exec("DELETE FROM behavior_reviews WHERE id = ?", id)
	return err
}

func (s *Service) listReviews(sessionID string, cursor string, limit int) (*ReviewListPage, error) {
	db, err := s.openStore()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var total int
	if sessionID != "" {
		err = db.QueryRow("SELECT count(*) FROM behavior_reviews WHERE session_id = ?", sessionID).Scan(&total)
	} else {
		err = db.QueryRow("SELECT count(*) FROM behavior_reviews").Scan(&total)
	}
	if err != nil {
		return nil, err
	}

	var query strings.Builder
	var args []any

	query.WriteString(`
SELECT
	id, session_id, session_title_snapshot, project_snapshot,
	created_at, started_at, finished_at, timezone, status,
	model, attempt_of, error_code, error_message, result
FROM behavior_reviews
`)

	conditions := []string{}
	if sessionID != "" {
		conditions = append(conditions, "session_id = ?")
		args = append(args, sessionID)
	}
	if cursor != "" {
		conditions = append(conditions, "created_at < ?")
		args = append(args, cursor)
	}

	if len(conditions) > 0 {
		query.WriteString(" WHERE " + strings.Join(conditions, " AND "))
	}
	query.WriteString(" ORDER BY created_at DESC LIMIT ?")
	args = append(args, limit+1)

	rows, err := db.Query(query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []ReviewSummary
	for rows.Next() {
		var (
			s                                         ReviewSummary
			createdAtRaw, startedAtRaw, finishedAtRaw string
			resultRaw                                 string
		)
		err := rows.Scan(
			&s.ID, &s.SessionID, &s.SessionTitleSnapshot, &s.ProjectSnapshot,
			&createdAtRaw, &startedAtRaw, &finishedAtRaw, &s.Timezone, &s.Status,
			&s.Model, &s.AttemptOf, &s.ErrorCode, &s.ErrorMessage, &resultRaw,
		)
		if err != nil {
			return nil, err
		}
		s.CreatedAt = *parseTime(createdAtRaw)
		s.StartedAt = parseTime(startedAtRaw)
		s.FinishedAt = parseTime(finishedAtRaw)

		var res *ReviewResult
		if resultRaw != "" && resultRaw != "null" {
			var r ReviewResult
			if err := json.Unmarshal([]byte(resultRaw), &r); err == nil {
				res = &r
				s.Outcome = r.Outcome
				s.IssuesCount = len(r.Issues)
			}
		}
		s.Title = DeriveReviewTitle(s.CreatedAt, s.Timezone, res)
		reviews = append(reviews, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	nextCursor := ""
	if len(reviews) > limit {
		nextCursor = formatTime(&reviews[limit-1].CreatedAt)
		reviews = reviews[:limit]
	}

	if reviews == nil {
		reviews = []ReviewSummary{}
	}

	return &ReviewListPage{
		Reviews:    reviews,
		NextCursor: nextCursor,
		Total:      total,
	}, nil
}
