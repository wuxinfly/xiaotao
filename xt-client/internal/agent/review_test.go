package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSanitizeEvidenceText(t *testing.T) {
	// 1. Private key
	pk := "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA0Y...\n-----END RSA PRIVATE KEY-----"
	if res := SanitizeEvidenceText(pk); !strings.Contains(res, "[REDACTED_PRIVATE_KEY]") {
		t.Errorf("expected private key redacted, got: %s", res)
	}

	// 2. Bearer token and Authorization header
	standaloneBearer := "User token is Bearer eyJhbGciOiJIUzI1NiIsIn...\n"
	sanitizedStandalone := SanitizeEvidenceText(standaloneBearer)
	if strings.Contains(sanitizedStandalone, "eyJhbGci") || !strings.Contains(sanitizedStandalone, "Bearer [REDACTED_TOKEN]") {
		t.Errorf("expected standalone bearer token redacted, got: %s", sanitizedStandalone)
	}

	authHeader := "GET /api HTTP/1.1\r\nAuthorization: Bearer eyJhbGciOiJIUzI1NiIsIn...\r\n"
	sanitizedAuth := SanitizeEvidenceText(authHeader)
	if strings.Contains(sanitizedAuth, "eyJhbGci") || !strings.Contains(sanitizedAuth, "Authorization: [REDACTED]") {
		t.Errorf("expected auth header redacted, got: %s", sanitizedAuth)
	}

	// 3. Cookie header
	cookie := "Cookie: session_id=xyz123; tracking=abc\r\n"
	if res := SanitizeEvidenceText(cookie); !strings.Contains(res, "Cookie: [REDACTED]") {
		t.Errorf("expected cookie redacted, got: %s", res)
	}

	// 4. URL credentials and query parameters
	url := "https://user:pass123@example.com/api?token=secret_token_123&api_key=key_456&normal=ok"
	sanitizedURL := SanitizeEvidenceText(url)
	if strings.Contains(sanitizedURL, "pass123") || strings.Contains(sanitizedURL, "secret_token_123") || strings.Contains(sanitizedURL, "key_456") {
		t.Errorf("expected URL credentials redacted, got: %s", sanitizedURL)
	}
	if !strings.Contains(sanitizedURL, "normal=ok") {
		t.Errorf("expected normal query param preserved, got: %s", sanitizedURL)
	}

	// 5. Nested JSON with varied casing
	rawJSON := `{
		"API_KEY": "secret-val-1",
		"password": "my-secret-pass",
		"data": {
			"refreshToken": "ref-tok-999",
			"publicInfo": "safe-value"
		}
	}`
	sanitizedJSON := SanitizeEvidenceText(rawJSON)
	if strings.Contains(sanitizedJSON, "secret-val-1") ||
		strings.Contains(sanitizedJSON, "my-secret-pass") ||
		strings.Contains(sanitizedJSON, "ref-tok-999") {
		t.Errorf("expected JSON sensitive fields redacted, got: %s", sanitizedJSON)
	}
	if !strings.Contains(sanitizedJSON, "safe-value") {
		t.Errorf("expected safe-value preserved, got: %s", sanitizedJSON)
	}
}

func TestBuildEvidenceSnapshot(t *testing.T) {
	now := time.Now()
	ended := now.Add(5 * time.Second)

	session := &Session{
		ID:        "run_test_session",
		Title:     "测试会话",
		Model:     "test-model",
		Status:    StatusCompleted,
		Messages: []Message{
			{
				ID:        "msg_1",
				Role:      "user",
				Content:   "帮我读取一下当前目录文件",
				RunID:     "run_1",
				CreatedAt: now,
			},
			{
				ID:        "msg_2",
				Role:      "assistant",
				Content:   "好的，我来帮你读取。",
				RunID:     "run_1",
				CreatedAt: now.Add(time.Second),
			},
		},
		Activity: []Activity{
			{
				ID:        "act_1",
				RunID:     "run_1",
				Kind:      "run_command",
				Title:     "执行命令",
				Tool:      "run_command",
				Input:     "dir",
				Output:    strings.Repeat("line-of-output\n", 100),
				Status:    "completed",
				StartedAt: now,
				EndedAt:   &ended,
			},
			{
				ID:        "act_2",
				RunID:     "run_1",
				Kind:      "read_file",
				Title:     "读取文件",
				Tool:      "read_file",
				Input:     "missing.txt",
				Output:    "",
				Status:    "failed",
				StartedAt: now,
				EndedAt:   &ended,
			},
		},
		RunStats: []RunStats{
			{
				RunID:     "run_1",
				StartedAt: now,
				EndedAt:   &ended,
			},
		},
	}

	snap, err := BuildEvidenceSnapshot(session, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if snap.Scope.AvailableCount != 1 || snap.Scope.IncludedCount != 1 {
		t.Errorf("expected 1 run, got available: %d, included: %d", snap.Scope.AvailableCount, snap.Scope.IncludedCount)
	}

	// Tool with empty output on failure must be marked with [未知工具结果]
	foundUnknown := false
	for _, item := range snap.EvidenceSnapshot {
		if item.SourceID == "act_2" {
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Errorf("expected act_2 in evidence snapshot")
	}
	if !strings.Contains(snap.PromptPayload, "[未知工具结果]") {
		t.Errorf("expected [未知工具结果] in prompt payload, got: %s", snap.PromptPayload)
	}

	// Tool with long output must be truncated
	if !strings.Contains(snap.PromptPayload, "截断省略") {
		t.Errorf("expected truncated tool output in prompt payload, got: %s", snap.PromptPayload)
	}
}

type mockModelClient struct {
	completeFunc func(ctx context.Context, model string, messages []map[string]any) (*completion, error)
}

func (m *mockModelClient) Complete(ctx context.Context, model string, messages []map[string]any, tools []map[string]any, onDelta func(string)) (*completion, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, model, messages)
	}
	return nil, fmt.Errorf("mock Complete not implemented")
}

func setupTestReviewService(t *testing.T) (*Service, string) {
	dir, err := os.MkdirTemp("", "review_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	svc := NewService(dir, "http://127.0.0.1:5455", func() string { return "test-token" }, nil, nil)
	svc.SetClientVersion("1.4.9")
	return svc, dir
}

func createTestSessionWithCompletedRun(svc *Service) string {
	sess, _ := svc.Create(Options{Model: "test-model"})
	now := time.Now()
	ended := now.Add(2 * time.Second)
	sess.Messages = []Message{
		{
			ID:        "m1",
			Role:      "user",
			Content:   "列出文件",
			RunID:     "r1",
			CreatedAt: now,
		},
		{
			ID:        "m2",
			Role:      "assistant",
			Content:   "已经列出",
			RunID:     "r1",
			CreatedAt: now.Add(time.Second),
		},
	}
	sess.Activity = []Activity{
		{
			ID:        "a1",
			RunID:     "r1",
			Kind:      "run_command",
			Title:     "列出文件",
			Tool:      "run_command",
			Input:     "dir",
			Output:    "file1.txt\nfile2.txt",
			Status:    "completed",
			StartedAt: now,
			EndedAt:   &ended,
		},
	}
	sess.RunStats = []RunStats{
		{
			RunID:     "r1",
			StartedAt: now,
			EndedAt:   &ended,
		},
	}
	_ = svc.saveSession(sess)
	return sess.ID
}

func TestReviewLifecycleAndValidation(t *testing.T) {
	svc, dir := setupTestReviewService(t)
	defer os.RemoveAll(dir)

	sessionID := createTestSessionWithCompletedRun(svc)

	// Case 1: Successful review with mock client
	mock := &mockModelClient{
		completeFunc: func(ctx context.Context, model string, messages []map[string]any) (*completion, error) {
			result := ReviewResult{
				Outcome: ReviewOutcomeIssuesFound,
				Summary: "整体执行良好，但存在工具重复调用问题",
				Issues: []ReviewIssue{
					{
						ID:          "issue_1",
						Title:       "工具重复调用",
						Problem:     "对同一文件连续执行了多次读取",
						Impact:      "浪费了模型上下文",
						Speculation: "未记住上一步读取的内容",
						Proposal:    "读取前检查历史内容",
						Acceptance:  "同会话同一文件仅读取一次",
						EvidenceIDs: []string{"ev_1"},
					},
				},
				GoodPractices: []ReviewGoodPractice{
					{
						ID:          "good_1",
						Title:       "及时告知结果",
						Description: "完成命令后立即回复了用户",
						EvidenceIDs: []string{"ev_1"},
					},
				},
			}
			b, _ := json.Marshal(result)
			return &completion{
				Content:      string(b),
				OutputTokens: 120,
			}, nil
		},
	}
	svc.SetModelClientForTest(mock)

	reqID := "req_test_1"
	rev, err := svc.CreateReview(sessionID, CreateReviewRequest{RequestID: reqID})
	if err != nil {
		t.Fatalf("CreateReview failed: %v", err)
	}

	// Idempotency: same request_id returns same review
	same, err := svc.CreateReview(sessionID, CreateReviewRequest{RequestID: reqID})
	if err != nil {
		t.Fatalf("CreateReview idempotent failed: %v", err)
	}
	if same.ID != rev.ID {
		t.Errorf("expected same review ID, got %s vs %s", same.ID, rev.ID)
	}

	// Wait for background review to finish
	var finished *BehaviorReview
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		r, err := svc.GetReview(rev.ID)
		if err == nil && (r.Status == ReviewStatusSucceeded || r.Status == ReviewStatusFailed) {
			finished = r
			break
		}
	}

	if finished == nil || finished.Status != ReviewStatusSucceeded {
		t.Fatalf("expected review succeeded, got: %+v", finished)
	}

	if finished.Result == nil || len(finished.Result.Issues) != 1 {
		t.Fatalf("expected 1 issue in result, got: %+v", finished.Result)
	}

	// Test Export Markdown
	filename, mdContent, err := svc.GenerateExportMarkdown(finished.ID)
	if err != nil {
		t.Fatalf("GenerateExportMarkdown failed: %v", err)
	}
	if !strings.HasSuffix(filename, ".md") || strings.Contains(filename, "/") {
		t.Errorf("invalid export filename: %s", filename)
	}
	if !strings.Contains(mdContent, "工具重复调用") || !strings.Contains(mdContent, "复盘范围") {
		t.Errorf("markdown content missing expected sections: %s", mdContent)
	}

	// Case 2: Validation failure due to unknown evidence ID
	mockInvalid := &mockModelClient{
		completeFunc: func(ctx context.Context, model string, messages []map[string]any) (*completion, error) {
			result := ReviewResult{
				Outcome: ReviewOutcomeIssuesFound,
				Summary: "模型伪造了引用",
				Issues: []ReviewIssue{
					{
						ID:          "issue_fake",
						Title:       "伪造引用",
						Problem:     "不存在的问题",
						Impact:      "无",
						Speculation: "推测",
						Proposal:    "改法",
						Acceptance:  "验收",
						EvidenceIDs: []string{"ev_99999_unknown"},
					},
				},
			}
			b, _ := json.Marshal(result)
			return &completion{
				Content:      string(b),
				OutputTokens: 50,
			}, nil
		},
	}
	svc.SetModelClientForTest(mockInvalid)

	revInvalid, err := svc.CreateReview(sessionID, CreateReviewRequest{RequestID: "req_invalid"})
	if err != nil {
		t.Fatalf("CreateReview failed: %v", err)
	}

	var finishedInvalid *BehaviorReview
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		r, err := svc.GetReview(revInvalid.ID)
		if err == nil && (r.Status == ReviewStatusSucceeded || r.Status == ReviewStatusFailed) {
			finishedInvalid = r
			break
		}
	}

	if finishedInvalid == nil || finishedInvalid.Status != ReviewStatusFailed {
		t.Fatalf("expected review to fail on unknown evidence ID, got: %+v", finishedInvalid)
	}
	if finishedInvalid.ErrorCode != "invalid_evidence_ref" {
		t.Errorf("expected error_code invalid_evidence_ref, got: %s", finishedInvalid.ErrorCode)
	}

	// Case 3: Retry review
	// Fix mock client back to valid output
	svc.SetModelClientForTest(mock)
	retried, err := svc.RetryReview(finishedInvalid.ID, RetryReviewRequest{RequestID: "req_retry_1"})
	if err != nil {
		t.Fatalf("RetryReview failed: %v", err)
	}
	if retried.AttemptOf != finishedInvalid.ID {
		t.Errorf("expected retried review to link attempt_of %s, got: %s", finishedInvalid.ID, retried.AttemptOf)
	}

	var finishedRetry *BehaviorReview
	for i := 0; i < 20; i++ {
		time.Sleep(50 * time.Millisecond)
		r, err := svc.GetReview(retried.ID)
		if err == nil && r.Status == ReviewStatusSucceeded {
			finishedRetry = r
			break
		}
	}
	if finishedRetry == nil || finishedRetry.Status != ReviewStatusSucceeded {
		t.Fatalf("expected retried review to succeed, got: %+v", finishedRetry)
	}

	// Case 4: Delete review
	if err := svc.DeleteReview(finished.ID); err != nil {
		t.Errorf("DeleteReview failed: %v", err)
	}
	if _, err := svc.GetReview(finished.ID); err == nil {
		t.Errorf("expected deleted review not found")
	}
}

func TestReviewInterruptedRecovery(t *testing.T) {
	svc, dir := setupTestReviewService(t)
	defer os.RemoveAll(dir)

	db, err := svc.openStore()
	if err != nil {
		t.Fatalf("openStore failed: %v", err)
	}

	// Manually insert a running review
	r := &BehaviorReview{
		ID:                   "rev_interrupted",
		SchemaVersion:        1,
		SessionID:            "sess_1",
		RequestID:            "req_interrupted",
		Status:               ReviewStatusRunning,
		CreatedAt:            time.Now().UTC(),
		Timezone:             "Asia/Shanghai",
		Model:                "test-model",
		ClientVersion:        "1.4.9",
		CoreVersion:          "core-1",
		PromptVersion:        "v1",
		Scope:                ReviewScope{},
		EvidenceSnapshot:     []ReviewEvidenceItem{},
		SuggestionLinks:      []ReviewSuggestionLink{},
	}
	if err := svc.insertReview(r, "prompt"); err != nil {
		t.Fatalf("insertReview failed: %v", err)
	}
	db.Close()

	// Simulate service restart
	svc2 := NewService(dir, "http://127.0.0.1:5455", nil, nil, nil)
	rev, err := svc2.GetReview("rev_interrupted")
	if err != nil {
		t.Fatalf("GetReview failed: %v", err)
	}
	if rev.Status != ReviewStatusFailed {
		t.Errorf("expected status failed, got: %s", rev.Status)
	}
	if rev.ErrorCode != "interrupted" {
		t.Errorf("expected error_code interrupted, got: %s", rev.ErrorCode)
	}
}
