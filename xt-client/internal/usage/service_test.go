package usage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestService_CumulativeDeduplication(t *testing.T) {
	tmp := t.TempDir()
	svc, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer svc.Close()

	req := svc.StartRequest("req-1", "trae", "test-model", "acct-1", "用户", "提问")
	p1 := int64(100)
	c1 := int64(50)
	ca1 := int64(20)
	req.OnUsage(&p1, &c1, &ca1)

	// Wait for worker
	time.Sleep(50 * time.Millisecond)

	// Same usage reported again (cumulative upstream event)
	req.OnUsage(&p1, &c1, &ca1)
	time.Sleep(50 * time.Millisecond)

	snap, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if snap.CurrentSession.PromptTokens != 100 {
		t.Errorf("expected 100 prompt tokens, got %d", snap.CurrentSession.PromptTokens)
	}
	if snap.CurrentSession.CompletionTokens != 50 {
		t.Errorf("expected 50 completion tokens, got %d", snap.CurrentSession.CompletionTokens)
	}
	if snap.CurrentSession.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", snap.CurrentSession.TotalTokens)
	}

	// Incremental usage
	p2 := int64(120) // +20
	c2 := int64(80)  // +30
	req.OnUsage(&p2, &c2, &ca1)
	time.Sleep(50 * time.Millisecond)

	snap2, _ := svc.Snapshot()
	if snap2.CurrentSession.PromptTokens != 120 {
		t.Errorf("expected 120 prompt tokens, got %d", snap2.CurrentSession.PromptTokens)
	}
	if snap2.CurrentSession.CompletionTokens != 80 {
		t.Errorf("expected 80 completion tokens, got %d", snap2.CurrentSession.CompletionTokens)
	}
	if snap2.CurrentSession.TotalTokens != 200 {
		t.Errorf("expected 200 total tokens, got %d", snap2.CurrentSession.TotalTokens)
	}

	req.OnCompleted()
	time.Sleep(50 * time.Millisecond)

	snap3, _ := svc.Snapshot()
	if snap3.Today.CompletedRequests != 1 {
		t.Errorf("expected 1 completed request, got %d", snap3.Today.CompletedRequests)
	}
	if snap3.Today.TotalRequests != 1 {
		t.Errorf("expected 1 total request, got %d", snap3.Today.TotalRequests)
	}
}

func TestService_SortingAndActiveCount(t *testing.T) {
	tmp := t.TempDir()
	svc, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer svc.Close()

	// Completed older request
	r1 := svc.StartRequest("req-1", "trae", "m1", "a1", "读", "分析代码")
	r1.OnCompleted()

	time.Sleep(10 * time.Millisecond)

	// Active request 2
	_ = svc.StartRequest("req-2", "trae", "m2", "a2", "写", "更新配置")

	time.Sleep(10 * time.Millisecond)

	// Active request 3
	r3 := svc.StartRequest("req-3", "trae", "m3", "a3", "执行", "运行测试")
	r3.OnFirstToken()

	snap, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if snap.ActiveCount != 2 {
		t.Fatalf("expected activeCount 2, got %d", snap.ActiveCount)
	}

	// Rule: "进行中优先，同组按开始时间倒序"
	// Active: req-3 (newer), req-2 (older)
	// Finished: req-1
	if len(snap.RecentRequests) != 3 {
		t.Fatalf("expected 3 recent requests, got %d", len(snap.RecentRequests))
	}
	if snap.RecentRequests[0].ID != "req-3" {
		t.Errorf("expected first to be req-3, got %s", snap.RecentRequests[0].ID)
	}
	if snap.RecentRequests[1].ID != "req-2" {
		t.Errorf("expected second to be req-2, got %s", snap.RecentRequests[1].ID)
	}
	if snap.RecentRequests[2].ID != "req-1" {
		t.Errorf("expected third to be req-1, got %s", snap.RecentRequests[2].ID)
	}
}

func TestService_ConcurrentRequests(t *testing.T) {
	tmp := t.TempDir()
	svc, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer svc.Close()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		idx := i
		go func() {
			defer wg.Done()
			id := filepath.Base(tmp) + string(rune('a'+idx))
			req := svc.StartRequest(id, "trae", "model-test", "acct", "用户", "并发测试")
			req.OnFirstToken()
			p := int64(10)
			c := int64(20)
			req.OnUsage(&p, &c, nil)
			if idx%2 == 0 {
				req.OnCompleted()
			} else {
				req.OnError("network timeout")
			}
		}()
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	snap, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if snap.CurrentSession.TotalTokens != 20*(10+20) {
		t.Errorf("expected %d total tokens, got %d", 20*30, snap.CurrentSession.TotalTokens)
	}
	if snap.Today.TotalRequests != 20 {
		t.Errorf("expected 20 total requests, got %d", snap.Today.TotalRequests)
	}
	if snap.Today.CompletedRequests != 10 {
		t.Errorf("expected 10 completed, got %d", snap.Today.CompletedRequests)
	}
	if snap.Today.FailedRequests != 10 {
		t.Errorf("expected 10 failed, got %d", snap.Today.FailedRequests)
	}
}

func TestService_CrashRecovery(t *testing.T) {
	tmp := t.TempDir()

	// Start first instance and leave a request in progress
	svc1, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	req := svc1.StartRequest("req-unfin", "trae", "m1", "a1", "用户", "未完成")
	p := int64(50)
	c := int64(25)
	req.OnUsage(&p, &c, nil)
	time.Sleep(100 * time.Millisecond)

	// Close abruptly without finishing req
	svc1.Close()

	// Restart service (new instance)
	svc2, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New restart failed: %v", err)
	}
	defer svc2.Close()

	snap, err := svc2.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	// Session tokens should be reset to 0 upon restart
	if snap.CurrentSession.TotalTokens != 0 {
		t.Errorf("expected current session total tokens 0, got %d", snap.CurrentSession.TotalTokens)
	}
	// Cumulative tokens should preserve the 75 tokens
	if snap.Cumulative.TotalTokens != 75 {
		t.Errorf("expected cumulative total tokens 75, got %d", snap.Cumulative.TotalTokens)
	}
	// The unfinalized request should now be interrupted
	if snap.Today.InterruptedRequests != 1 {
		t.Errorf("expected 1 interrupted request, got %d", snap.Today.InterruptedRequests)
	}
}

func TestService_RetentionCleanup(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "usage.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}

	// Insert a record 8 days ago
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	rec := &Record{
		RequestID: "old-req",
		BatchID:   "old-batch",
		Model:     "m",
		AccountID: "a",
		CreatedAt: oldTime,
		Status:    StatusCompleted,
	}
	_ = db.CommitRequestAndUsage(rec, DateInShanghai(oldTime), 100, 50, 0, true, StatusCompleted)

	// Insert a record 1 day ago
	recentTime := time.Now().Add(-24 * time.Hour)
	rec2 := &Record{
		RequestID: "recent-req",
		BatchID:   "rec-batch",
		Model:     "m",
		AccountID: "a",
		CreatedAt: recentTime,
		Status:    StatusCompleted,
	}
	_ = db.CommitRequestAndUsage(rec2, DateInShanghai(recentTime), 200, 100, 0, true, StatusCompleted)

	// Run cleanup older than 7 days
	deleted, err := db.CleanupExpiredRequests(time.Now().Add(-7 * 24 * time.Hour))
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Check daily usage is preserved!
	cum, err := db.LoadCumulativeStats()
	if err != nil {
		t.Fatalf("LoadCumulativeStats failed: %v", err)
	}
	if cum.TotalTokens != 450 {
		t.Errorf("expected 450 tokens preserved in daily table, got %d", cum.TotalTokens)
	}
	db.Close()
	os.RemoveAll(tmp)
}

func TestService_MidnightCrossing(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "usage.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	defer db.Close()

	// Start date: 2026-10-30
	startLoc := time.Date(2026, 10, 30, 23, 59, 50, 0, shanghaiLoc)
	rec := &Record{
		RequestID: "cross-req",
		BatchID:   "b1",
		Model:     "m1",
		AccountID: "a1",
		CreatedAt: startLoc,
		Status:    StatusCompleted,
	}

	// Upstream confirms usage on 2026-10-31
	confirmedDate := "2026-10-31"
	err = db.CommitRequestAndUsage(rec, confirmedDate, 1000, 500, 200, true, StatusCompleted)
	if err != nil {
		t.Fatalf("Commit failed: %v", err)
	}

	// Day 1 (2026-10-30) should have total_requests = 1, completed = 1, but 0 tokens
	d1, err := db.LoadDailyUsage("2026-10-30")
	if err != nil {
		t.Fatalf("load d1 failed: %v", err)
	}
	if d1.TotalRequests != 1 || d1.CompletedRequests != 1 {
		t.Errorf("d1 requests mismatch: total=%d, completed=%d", d1.TotalRequests, d1.CompletedRequests)
	}
	if d1.TotalTokens() != 0 {
		t.Errorf("d1 tokens should be 0, got %d", d1.TotalTokens())
	}

	// Day 2 (2026-10-31) should have tokens 1500, but 0 requests
	d2, err := db.LoadDailyUsage("2026-10-31")
	if err != nil {
		t.Fatalf("load d2 failed: %v", err)
	}
	if d2.TotalRequests != 0 {
		t.Errorf("d2 requests should be 0, got %d", d2.TotalRequests)
	}
	if d2.TotalTokens() != 1500 {
		t.Errorf("d2 total tokens should be 1500, got %d", d2.TotalTokens())
	}
}

func TestService_UnknownVsZeroTokens(t *testing.T) {
	tmp := t.TempDir()
	svc, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer svc.Close()

	// Request 1: has 0 tokens explicitly (e.g. prompt: 0, completion: 0)
	r1 := svc.StartRequest("req-zero", "trae", "m1", "a1", "用户", "零")
	z := int64(0)
	r1.OnUsage(&z, &z, &z)
	r1.OnCompleted()

	// Request 2: upstream provided no token usage at all (unknown)
	r2 := svc.StartRequest("req-unknown", "trae", "m2", "a2", "用户", "未知")
	r2.OnCompleted()

	time.Sleep(50 * time.Millisecond)

	snap, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	// Find summaries
	var sumZero, sumUnknown *RequestSummary
	for i := range snap.RecentRequests {
		if snap.RecentRequests[i].ID == "req-zero" {
			sumZero = &snap.RecentRequests[i]
		}
		if snap.RecentRequests[i].ID == "req-unknown" {
			sumUnknown = &snap.RecentRequests[i]
		}
	}

	if sumZero == nil || sumUnknown == nil {
		t.Fatalf("could not find both requests in recent summaries")
	}

	if sumZero.Tokens.Prompt == nil || *sumZero.Tokens.Prompt != 0 {
		t.Errorf("expected zero prompt token 0, got %v", sumZero.Tokens.Prompt)
	}
	if sumUnknown.Tokens.Prompt != nil {
		t.Errorf("expected unknown prompt token to be nil, got %v", sumUnknown.Tokens.Prompt)
	}
	if sumUnknown.Tokens.Total != nil {
		t.Errorf("expected unknown total token to be nil, got %v", sumUnknown.Tokens.Total)
	}
	if snap.Today.MissingUsageRequests != 1 {
		t.Errorf("expected 1 missing usage request, got %d", snap.Today.MissingUsageRequests)
	}
}

func TestService_RealtimeStreamingTokensAndSpeed(t *testing.T) {
	tmp := t.TempDir()
	svc, err := New(tmp, "0.1.0-test")
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer svc.Close()

	req := svc.StartRequest("req-stream", "trae", "gpt-5.1-codex", "acct-1", "读", "分析项目文件")

	// Simulate streaming deltas arriving
	req.OnStreamDelta("正在分析项目代码...")
	time.Sleep(60 * time.Millisecond)
	req.OnStreamDelta("发现3个模块需要优化配置。")

	snap, err := svc.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if len(snap.RecentRequests) != 1 {
		t.Fatalf("expected 1 recent request, got %d", len(snap.RecentRequests))
	}
	r := snap.RecentRequests[0]
	if r.ActionTag != "读" || r.ActionTitle != "分析项目文件" {
		t.Errorf("unexpected action: tag=%s, title=%s", r.ActionTag, r.ActionTitle)
	}
	if r.ChannelLabel != "Trae" {
		t.Errorf("expected channel Trae, got %s", r.ChannelLabel)
	}
	if r.Tokens.Completion == nil || *r.Tokens.Completion <= 0 {
		t.Errorf("expected positive live completion tokens during stream, got %v", r.Tokens.Completion)
	}
	if r.SpeedTokPerSec == nil || *r.SpeedTokPerSec <= 0 {
		t.Errorf("expected positive live speed during stream, got %v", r.SpeedTokPerSec)
	}
	if snap.CurrentSession.TotalTokens <= 0 {
		t.Errorf("expected current session total tokens to advance live, got %d", snap.CurrentSession.TotalTokens)
	}

	// Upstream completes and delivers official token usage
	p := int64(1500)
	c := int64(200)
	ca := int64(800)
	req.OnUsage(&p, &c, &ca)
	req.OnCompleted()
	time.Sleep(50 * time.Millisecond)

	snapFinal, _ := svc.Snapshot()
	if snapFinal.CurrentSession.TotalTokens != 1500+200 {
		t.Errorf("expected reconciled total tokens 1700, got %d", snapFinal.CurrentSession.TotalTokens)
	}
}
