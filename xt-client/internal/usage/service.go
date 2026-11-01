package usage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Service struct {
	db        *DB
	version   string
	batchID   string
	startedAt time.Time

	mu            sync.RWMutex
	currentBatch  SessionStats
	todayCache    DailyUsage
	cumulative    CumulativeStats
	storageStatus StorageStatus
	versionSeq    int64

	activeRequests map[string]*ActiveRequest
	recentFinished []*Record

	writeCh chan *writeTask
	done    chan struct{}

	subMu sync.Mutex
	subs  map[chan *DashboardSnapshot]struct{}

	dirty atomic.Bool
}

type writeTask struct {
	rec             *Record
	confirmedDate   string
	deltaPrompt     int64
	deltaCompletion int64
	deltaCached     int64
	isFirstFinal    bool
	finalStatus     string
}

func New(dataDir, version string) (*Service, error) {
	dbPath := filepath.Join(dataDir, "usage.db")
	db, err := OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open usage db: %w", err)
	}

	batchBytes := make([]byte, 8)
	_, _ = rand.Read(batchBytes)
	batchID := hex.EncodeToString(batchBytes)
	now := time.Now()

	s := &Service{
		db:             db,
		version:        version,
		batchID:        batchID,
		startedAt:      now,
		storageStatus:  StorageStatus{Status: "healthy"},
		activeRequests: make(map[string]*ActiveRequest),
		recentFinished: make([]*Record, 0, 10),
		writeCh:        make(chan *writeTask, 2048),
		done:           make(chan struct{}),
		subs:           make(map[chan *DashboardSnapshot]struct{}),
	}

	// 1. Recover any lingering unfinalized requests from previous run
	if _, err := db.RecoverUnfinishedRequests(batchID, now); err != nil {
		s.storageStatus = StorageStatus{Status: "degraded", Message: "恢复中断请求异常: " + err.Error()}
	}

	// 2. Cleanup expired records older than 7 rolling days
	cutoff := now.Add(-7 * 24 * time.Hour)
	_, _ = db.CleanupExpiredRequests(cutoff)

	// 3. Load today & cumulative stats
	todayStr := DateInShanghai(now)
	if td, err := db.LoadDailyUsage(todayStr); err == nil {
		s.todayCache = *td
	}
	if cum, err := db.LoadCumulativeStats(); err == nil {
		s.cumulative = *cum
	}

	// 4. Load recent records from DB to seed initial display
	if recent, err := db.LoadRecentRecords(10); err == nil {
		s.recentFinished = recent
	}

	go s.writeWorker()
	go s.broadcastWorker()
	go s.dailyCleanupWorker()

	return s, nil
}

func (s *Service) Close() error {
	close(s.done)
	return s.db.Close()
}

func (s *Service) BatchID() string {
	return s.batchID
}

// ActiveRequest represents an in-flight proxy request.
type ActiveRequest struct {
	s   *Service
	rec Record
	mu  sync.Mutex

	lastReportedPrompt      int64
	lastReportedCompletion  int64
	lastReportedCached      int64
	lastCommittedPrompt     int64
	lastCommittedCompletion int64
	lastCommittedCached     int64
	finalized               bool
}

// StartRequest creates and registers a new active request record.
func (s *Service) StartRequest(id, channel, model, accountID, actionTag, actionTitle string) *ActiveRequest {
	if channel == "" {
		channel = "trae"
	}
	if actionTag == "" {
		actionTag = "用户"
	}
	if actionTitle == "" {
		actionTitle = "模型请求"
	}
	now := time.Now()
	rec := Record{
		RequestID:   id,
		BatchID:     s.batchID,
		Channel:     channel,
		Model:       model,
		AccountID:   accountID,
		ActionTag:   actionTag,
		ActionTitle: actionTitle,
		CreatedAt:   now,
		Status:      StatusPending,
	}

	ar := &ActiveRequest{
		s:   s,
		rec: rec,
	}

	s.mu.Lock()
	s.activeRequests[id] = ar
	s.mu.Unlock()

	// Enqueue initial record write
	s.enqueueWrite(&rec, DateInShanghai(now), 0, 0, 0, false, "")
	s.markDirty()

	return ar
}

func (ar *ActiveRequest) OnFirstToken() {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	if ar.rec.FirstTokenAt == nil && ar.rec.Status == StatusPending {
		now := time.Now()
		ar.rec.FirstTokenAt = &now
		ar.rec.Status = StatusStreaming

		copyRec := ar.rec
		ar.s.enqueueWrite(&copyRec, DateInShanghai(now), 0, 0, 0, false, "")
		ar.s.markDirty()
	}
}

func (ar *ActiveRequest) SetAction(tag, title string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if tag != "" {
		ar.rec.ActionTag = tag
	}
	if title != "" {
		ar.rec.ActionTitle = title
	}
	ar.s.markDirty()
}

// SetAccount records the account that ultimately served a failover request.
func (ar *ActiveRequest) SetAccount(accountID string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()
	if accountID != "" {
		ar.rec.AccountID = accountID
		ar.s.markDirty()
	}
}

// OnStreamDelta is called incrementally as text/tokens arrive from upstream during streaming.
func (ar *ActiveRequest) OnStreamDelta(deltaText string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	now := time.Now()
	if ar.rec.FirstTokenAt == nil && ar.rec.Status == StatusPending {
		ar.rec.FirstTokenAt = &now
		ar.rec.Status = StatusStreaming
	}

	est := estimateTokens(deltaText)
	if est > 0 {
		ar.lastReportedCompletion += est
		val := ar.lastReportedCompletion
		ar.rec.CompletionTokens = &val
		ar.s.applyMemoryUsage(0, est, 0, DateInShanghai(now))
		ar.s.markDirty()
	}
}

func estimateTokens(s string) int64 {
	runes := []rune(s)
	if len(runes) == 0 {
		return 0
	}
	cnt := int64(len(runes)) / 2
	if cnt < 1 {
		cnt = 1
	}
	return cnt
}

func (ar *ActiveRequest) OnUsage(prompt, completion, cached *int64) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	now := time.Now()
	confirmedDate := DateInShanghai(now)

	var pVal, cVal, caVal int64
	var memDeltaPrompt, memDeltaCompletion, memDeltaCached int64
	var dbDeltaPrompt, dbDeltaCompletion, dbDeltaCached int64

	if prompt != nil && *prompt >= 0 {
		pVal = *prompt
		ar.rec.PromptTokens = prompt
		if pVal > ar.lastReportedPrompt {
			memDeltaPrompt = pVal - ar.lastReportedPrompt
			ar.lastReportedPrompt = pVal
		}
		if pVal > ar.lastCommittedPrompt {
			dbDeltaPrompt = pVal - ar.lastCommittedPrompt
			ar.lastCommittedPrompt = pVal
		}
	}

	if completion != nil && *completion >= 0 {
		cVal = *completion
		ar.rec.CompletionTokens = completion
		// Reconcile in-memory streaming estimate with official upstream completion tokens
		memDeltaCompletion = cVal - ar.lastReportedCompletion
		ar.lastReportedCompletion = cVal

		// DB only receives confirmed positive deltas (never affected by stream estimates)
		if cVal > ar.lastCommittedCompletion {
			dbDeltaCompletion = cVal - ar.lastCommittedCompletion
			ar.lastCommittedCompletion = cVal
		}
	}

	// Keep stream estimates from hiding a later missing-usage response: a
	// usage-only chunk resets completion before storing nil.
	if completion == nil {
		ar.rec.CompletionTokens = nil
	}

	if cached != nil && *cached >= 0 {
		caVal = *cached
		ar.rec.CachedTokens = cached
		if caVal > ar.lastReportedCached {
			memDeltaCached = caVal - ar.lastReportedCached
			ar.lastReportedCached = caVal
		}
		if caVal > ar.lastCommittedCached {
			dbDeltaCached = caVal - ar.lastCommittedCached
			ar.lastCommittedCached = caVal
		}
	}

	if memDeltaPrompt != 0 || memDeltaCompletion != 0 || memDeltaCached != 0 {
		ar.s.applyMemoryUsage(memDeltaPrompt, memDeltaCompletion, memDeltaCached, confirmedDate)
	}

	copyRec := ar.rec
	ar.s.enqueueWrite(&copyRec, confirmedDate, dbDeltaPrompt, dbDeltaCompletion, dbDeltaCached, false, "")
	ar.s.markDirty()
}

func (ar *ActiveRequest) OnCompleted() {
	ar.finish(StatusCompleted, "")
}

func (ar *ActiveRequest) OnError(errSummary string) {
	ar.finish(StatusFailed, sanitizeError(errSummary))
}

func (ar *ActiveRequest) OnInterrupted(errSummary string) {
	ar.finish(StatusInterrupted, sanitizeError(errSummary))
}

func (ar *ActiveRequest) OnCanceled() {
	ar.finish(StatusCanceled, "客户端取消请求")
}

func (ar *ActiveRequest) finish(status, errSummary string) {
	ar.mu.Lock()
	defer ar.mu.Unlock()

	now := time.Now()
	ar.rec.EndedAt = &now
	ar.rec.Status = status
	if errSummary != "" {
		ar.rec.ErrorSummary = errSummary
	}

	isFirstFinal := false
	if !ar.finalized {
		ar.finalized = true
		isFirstFinal = true
	}

	copyRec := ar.rec
	ar.s.completeActiveRequest(ar.rec.RequestID, &copyRec, isFirstFinal, status)
	ar.s.enqueueWrite(&copyRec, DateInShanghai(now), 0, 0, 0, isFirstFinal, status)
	ar.s.markDirty()
}

func (s *Service) completeActiveRequest(id string, rec *Record, isFirstFinal bool, finalStatus string) {
	s.mu.Lock()
	delete(s.activeRequests, id)

	// Prepend to recentFinished and keep up to 20
	s.recentFinished = append([]*Record{rec}, s.recentFinished...)
	if len(s.recentFinished) > 20 {
		s.recentFinished = s.recentFinished[:20]
	}

	if isFirstFinal {
		// Update in-memory today & cumulative counts
		s.todayCache.TotalRequests++
		s.cumulative.TotalRequests++
		switch finalStatus {
		case StatusCompleted:
			s.todayCache.CompletedRequests++
		case StatusFailed, StatusCanceled:
			s.todayCache.FailedRequests++
		case StatusInterrupted:
			s.todayCache.InterruptedRequests++
		}
		if rec.PromptTokens == nil && rec.CompletionTokens == nil {
			s.todayCache.MissingUsageRequests++
		}
	}
	s.mu.Unlock()
}

func (s *Service) applyMemoryUsage(deltaPrompt, deltaCompletion, deltaCached int64, confirmedDate string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Current launch session usage (resets on restart)
	s.currentBatch.PromptTokens += deltaPrompt
	s.currentBatch.CompletionTokens += deltaCompletion
	s.currentBatch.CachedTokens += deltaCached
	if s.currentBatch.PromptTokens < 0 {
		s.currentBatch.PromptTokens = 0
	}
	if s.currentBatch.CompletionTokens < 0 {
		s.currentBatch.CompletionTokens = 0
	}
	if s.currentBatch.CachedTokens < 0 {
		s.currentBatch.CachedTokens = 0
	}
	s.currentBatch.TotalTokens = s.currentBatch.PromptTokens + s.currentBatch.CompletionTokens

	// Cumulative historical usage
	s.cumulative.PromptTokens += deltaPrompt
	s.cumulative.CompletionTokens += deltaCompletion
	s.cumulative.CachedTokens += deltaCached
	if s.cumulative.PromptTokens < 0 {
		s.cumulative.PromptTokens = 0
	}
	if s.cumulative.CompletionTokens < 0 {
		s.cumulative.CompletionTokens = 0
	}
	if s.cumulative.CachedTokens < 0 {
		s.cumulative.CachedTokens = 0
	}
	s.cumulative.TotalTokens = s.cumulative.PromptTokens + s.cumulative.CompletionTokens

	// Today usage cache
	todayStr := DateInShanghai(time.Now())
	if confirmedDate == todayStr {
		s.todayCache.Date = todayStr
		s.todayCache.PromptTokens += deltaPrompt
		s.todayCache.CompletionTokens += deltaCompletion
		s.todayCache.CachedTokens += deltaCached
		if s.todayCache.PromptTokens < 0 {
			s.todayCache.PromptTokens = 0
		}
		if s.todayCache.CompletionTokens < 0 {
			s.todayCache.CompletionTokens = 0
		}
		if s.todayCache.CachedTokens < 0 {
			s.todayCache.CachedTokens = 0
		}
	}
}

func (s *Service) enqueueWrite(rec *Record, confirmedDate string, deltaPrompt, deltaCompletion, deltaCached int64, isFirstFinal bool, finalStatus string) {
	task := &writeTask{
		rec:             rec,
		confirmedDate:   confirmedDate,
		deltaPrompt:     deltaPrompt,
		deltaCompletion: deltaCompletion,
		deltaCached:     deltaCached,
		isFirstFinal:    isFirstFinal,
		finalStatus:     finalStatus,
	}

	select {
	case s.writeCh <- task:
	default:
		// Buffer overflowed, record storage degraded
		s.mu.Lock()
		s.storageStatus = StorageStatus{Status: "degraded", Message: "持久化队列繁忙，写入延迟"}
		s.mu.Unlock()
	}
}

func (s *Service) writeWorker() {
	for {
		select {
		case <-s.done:
			return
		case task := <-s.writeCh:
			err := s.db.CommitRequestAndUsage(
				task.rec, task.confirmedDate,
				task.deltaPrompt, task.deltaCompletion, task.deltaCached,
				task.isFirstFinal, task.finalStatus,
			)

			s.mu.Lock()
			if err != nil {
				s.storageStatus = StorageStatus{Status: "error", Message: "数据库写入失败: " + err.Error()}
			} else if s.storageStatus.Status == "error" {
				s.storageStatus = StorageStatus{Status: "healthy"}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Service) markDirty() {
	s.dirty.Store(true)
}

func (s *Service) broadcastWorker() {
	ticker := time.NewTicker(250 * time.Millisecond) // up to 4 updates/sec
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			if s.dirty.Swap(false) {
				snap, err := s.Snapshot()
				if err == nil {
					s.subMu.Lock()
					for ch := range s.subs {
						select {
						case ch <- snap:
						default:
							// Slow consumer, skip this intermediate frame
						}
					}
					s.subMu.Unlock()
				}
			}
		}
	}
}

func (s *Service) dailyCleanupWorker() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			cutoff := time.Now().Add(-7 * 24 * time.Hour)
			_, _ = s.db.CleanupExpiredRequests(cutoff)
		}
	}
}

// Snapshot builds the full DashboardSnapshot.
func (s *Service) Snapshot() (*DashboardSnapshot, error) {
	s.mu.RLock()
	currentBatch := s.currentBatch
	today := s.todayCache
	cumulative := s.cumulative
	storage := s.storageStatus
	batchID := s.batchID
	startedAt := s.startedAt

	// Keep the service lock independent from request locks. Request updates
	// acquire a request lock before the service lock, so locking in the
	// opposite order while snapshotting can deadlock both sides.
	activeRequests := make([]*ActiveRequest, 0, len(s.activeRequests))
	for _, ar := range s.activeRequests {
		activeRequests = append(activeRequests, ar)
	}

	// Copy finished records
	finishedList := make([]*Record, len(s.recentFinished))
	copy(finishedList, s.recentFinished)
	s.mu.RUnlock()

	activeList := make([]*Record, 0, len(activeRequests))
	for _, ar := range activeRequests {
		ar.mu.Lock()
		cp := ar.rec
		activeList = append(activeList, &cp)
		ar.mu.Unlock()
	}

	now := time.Now()
	uptime := int64(now.Sub(startedAt).Seconds())

	// Active requests sorted by CreatedAt DESC
	sort.Slice(activeList, func(i, j int) bool {
		return activeList[i].CreatedAt.After(activeList[j].CreatedAt)
	})

	// Finished requests sorted by CreatedAt DESC
	sort.Slice(finishedList, func(i, j int) bool {
		return finishedList[i].CreatedAt.After(finishedList[j].CreatedAt)
	})

	// Combine: "进行中优先，同组按开始时间倒序"
	combined := make([]*Record, 0, len(activeList)+len(finishedList))
	combined = append(combined, activeList...)
	combined = append(combined, finishedList...)

	// Deduplicate by ID
	seen := make(map[string]bool)
	deduped := make([]*Record, 0, len(combined))
	for _, r := range combined {
		if !seen[r.RequestID] {
			seen[r.RequestID] = true
			deduped = append(deduped, r)
		}
	}

	// Recent top 3
	recentSummaries := make([]RequestSummary, 0, 3)
	for i := 0; i < len(deduped) && i < 3; i++ {
		recentSummaries = append(recentSummaries, toSummary(deduped[i], now))
	}

	// Active summaries
	activeSummaries := make([]RequestSummary, 0, len(activeList))
	for _, r := range activeList {
		activeSummaries = append(activeSummaries, toSummary(r, now))
	}

	// Load annual heatmap
	endDateStr := DateInShanghai(now)
	startDateStr := DateInShanghai(now.AddDate(-1, 0, 0))
	dbDays, err := s.db.LoadHeatmapDays(startDateStr, endDateStr)
	if err != nil {
		dbDays = make(map[string]*DailyUsage)
	}

	// Merge today's in-memory stats
	if today.Date == endDateStr {
		dbDays[endDateStr] = &today
	}

	trackingStart, _ := s.db.GetMeta("tracking_started_date")
	if trackingStart == "" {
		trackingStart = endDateStr
	}

	heatmapDays := make(map[string]*HeatmapDay, len(dbDays))
	for date, d := range dbDays {
		heatmapDays[date] = &HeatmapDay{
			TotalTokens:      d.TotalTokens(),
			PromptTokens:     d.PromptTokens,
			CompletionTokens: d.CompletionTokens,
			CachedTokens:     d.CachedTokens,
			TotalRequests:    d.TotalRequests,
			HasMissingUsage:  d.MissingUsageRequests > 0,
		}
	}

	seq := atomic.AddInt64(&s.versionSeq, 1)

	return &DashboardSnapshot{
		Version: s.version,
		Launch: LaunchInfo{
			StartedAt:     startedAt.Unix(),
			UptimeSeconds: uptime,
			BatchID:       batchID,
		},
		CurrentSession: currentBatch,
		Today:          today,
		Cumulative:     cumulative,
		RecentRequests: recentSummaries,
		ActiveCount:    len(activeList),
		ActiveRequests: activeSummaries,
		Heatmap: HeatmapData{
			StartDate:         startDateStr,
			EndDate:           endDateStr,
			TrackingStartDate: trackingStart,
			Days:              heatmapDays,
		},
		Storage:    storage,
		VersionSeq: seq,
	}, nil
}

func toSummary(r *Record, now time.Time) RequestSummary {
	startMs := r.CreatedAt.UnixMilli()

	var firstMs *int64
	var firstDurationMs *int64
	if r.FirstTokenAt != nil {
		f := r.FirstTokenAt.UnixMilli()
		firstMs = &f
		diff := f - startMs
		if diff >= 0 {
			firstDurationMs = &diff
		}
	}

	var endMs *int64
	var totalDurationMs int64
	if r.EndedAt != nil {
		e := r.EndedAt.UnixMilli()
		endMs = &e
		totalDurationMs = e - startMs
	} else {
		totalDurationMs = now.UnixMilli() - startMs
	}
	if totalDurationMs < 0 {
		totalDurationMs = 0
	}

	// Speed calculation: completed output tokens ÷ elapsed time since first token
	var speed *float64
	if r.CompletionTokens != nil && *r.CompletionTokens > 0 && r.FirstTokenAt != nil {
		end := now
		if r.EndedAt != nil {
			end = *r.EndedAt
		}
		sec := end.Sub(*r.FirstTokenAt).Seconds()
		if sec >= 0.05 {
			toks := float64(*r.CompletionTokens) / sec
			rounded := math.Round(toks*10) / 10
			speed = &rounded
		}
	}

	var totalTokens *int64
	if r.PromptTokens != nil || r.CompletionTokens != nil {
		var sum int64
		if r.PromptTokens != nil {
			sum += *r.PromptTokens
		}
		if r.CompletionTokens != nil {
			sum += *r.CompletionTokens
		}
		totalTokens = &sum
	}

	cleanModel := strings.TrimPrefix(r.Model, "trae/")

	channel := r.Channel
	if channel == "" {
		channel = "trae"
	}
	channelLabel := strings.ToUpper(channel)
	if strings.ToLower(channel) == "trae" {
		channelLabel = "Trae"
	}

	tag := r.ActionTag
	if tag == "" {
		tag = "用户"
	}
	title := r.ActionTitle
	if title == "" {
		title = "模型请求"
	}

	return RequestSummary{
		ID:                   r.RequestID,
		Channel:              channel,
		ChannelLabel:         channelLabel,
		Model:                cleanModel,
		AccountID:            r.AccountID,
		ActionTag:            tag,
		ActionTitle:          title,
		Status:               r.Status,
		StatusText:           formatStatusText(r.Status, r.CreatedAt, now),
		StartedAt:            startMs,
		FirstTokenAt:         firstMs,
		EndedAt:              endMs,
		DurationMs:           totalDurationMs,
		FirstTokenDurationMs: firstDurationMs,
		Tokens: TokenMetrics{
			Prompt:     r.PromptTokens,
			Completion: r.CompletionTokens,
			Cached:     r.CachedTokens,
			Total:      totalTokens,
		},
		SpeedTokPerSec: speed,
		ErrorSummary:   r.ErrorSummary,
	}
}

func formatStatusText(status string, created, now time.Time) string {
	switch status {
	case StatusPending:
		sec := int(now.Sub(created).Seconds())
		if sec <= 0 {
			return "等待响应"
		}
		return fmt.Sprintf("等待响应 · %d秒", sec)
	case StatusStreaming:
		return "正在输出"
	case StatusCompleted:
		return "已完成"
	case StatusFailed:
		return "失败"
	case StatusInterrupted:
		return "中断"
	case StatusCanceled:
		return "已取消"
	default:
		return status
	}
}

func sanitizeError(err string) string {
	if len(err) > 300 {
		err = err[:300] + "..."
	}
	low := strings.ToLower(err)
	for _, k := range []string{"token", "jwt", "key", "password", "secret", "bearer"} {
		if i := strings.Index(low, k); i >= 0 {
			err = err[:i] + k + "=<redacted>"
			break
		}
	}
	return err
}

// HandleSSE streams real-time DashboardSnapshot updates over Server-Sent Events.
func (s *Service) HandleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Initial snapshot
	snap, err := s.Snapshot()
	if err != nil {
		http.Error(w, "snapshot error", http.StatusInternalServerError)
		return
	}

	writeJSONEvent := func(eventType string, data any) error {
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if eventType != "" {
			fmt.Fprintf(w, "event: %s\n", eventType)
		}
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
		return nil
	}

	if err := writeJSONEvent("snapshot", snap); err != nil {
		return
	}

	// Register subscription
	ch := make(chan *DashboardSnapshot, 8)
	s.subMu.Lock()
	s.subs[ch] = struct{}{}
	s.subMu.Unlock()

	defer func() {
		s.subMu.Lock()
		delete(s.subs, ch)
		s.subMu.Unlock()
	}()

	pingTicker := time.NewTicker(15 * time.Second)
	defer pingTicker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-pingTicker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case update := <-ch:
			if err := writeJSONEvent("update", update); err != nil {
				return
			}
		}
	}
}
