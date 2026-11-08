package agent

import (
	"context"
	"sync"
	"xiaotao/xt-client/internal/logging"
)

type agentCtxKey struct{}

type agentCtxVal struct {
	sessionID string
	runID     string
}

func withAgentSessionRun(ctx context.Context, sessionID, runID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, agentCtxKey{}, agentCtxVal{sessionID: sessionID, runID: runID})
}

func agentSessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(agentCtxKey{}).(agentCtxVal); ok {
		return v.sessionID
	}
	return ""
}

func agentRunID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(agentCtxKey{}).(agentCtxVal); ok {
		return v.runID
	}
	return ""
}

var (
	currSessionMu sync.RWMutex
	currSessionID string
	pruneMu       sync.Mutex
	pruneStats    = map[string]*sessionPrune{}
)

func setCurrentSession(id string) {
	currSessionMu.Lock()
	currSessionID = id
	currSessionMu.Unlock()
}

func currentSession() string {
	currSessionMu.RLock()
	defer currSessionMu.RUnlock()
	return currSessionID
}

type sessionPrune struct {
	count int
	chars int
}

func recordPrune(sessionID string, charsSaved int) {
	if sessionID == "" {
		return
	}
	pruneMu.Lock()
	defer pruneMu.Unlock()
	p, ok := pruneStats[sessionID]
	if !ok {
		p = &sessionPrune{}
		pruneStats[sessionID] = p
	}
	p.count++
	p.chars += charsSaved
}

func flushPruneStats(sessionID string) {
	pruneMu.Lock()
	p, ok := pruneStats[sessionID]
	if ok {
		delete(pruneStats, sessionID)
	}
	pruneMu.Unlock()

	if ok && p.count > 0 {
		logging.Default().For(logging.ModuleAgent).Info("context tool prune summary",
			logging.F("session", sessionID),
			logging.F("prune_count", p.count),
			logging.F("pruned_chars", p.chars),
		)
	}
}

func logSessionStart(sessionID, runID, model string, historyCount int, contextWindow int64) {
	logging.Default().For(logging.ModuleAgent).Info("agent session started",
		logging.F("session", sessionID),
		logging.F("run", runID),
		logging.F("model", model),
		logging.F("history", historyCount),
		logging.F("context_window", contextWindow),
	)
}

func logSessionEnd(sessionID, runID, status string, durationMs int64, outputTokens int64, toolCalls int) {
	logging.Default().For(logging.ModuleAgent).Info("agent session finished",
		logging.F("session", sessionID),
		logging.F("run", runID),
		logging.F("status", status),
		logging.F("duration_ms", durationMs),
		logging.F("output_tokens", outputTokens),
		logging.F("tool_calls", toolCalls),
	)
	flushPruneStats(sessionID)
}

func logTurnRequestStart(sessionID, runID string, attempt int, contextTokens int64, toolsCount int) {
	logging.Default().For(logging.ModuleAgent).Info("model request started",
		logging.F("session", sessionID),
		logging.F("run", runID),
		logging.F("attempt", attempt),
		logging.F("context_tokens", contextTokens),
		logging.F("tools", toolsCount),
	)
}

func logTurnRequestEnd(sessionID, runID string, durationMs, ttftMs int64, outputTokens int64, hasToolCalls bool) {
	logging.Default().For(logging.ModuleAgent).Info("model request finished",
		logging.F("session", sessionID),
		logging.F("run", runID),
		logging.F("duration_ms", durationMs),
		logging.F("ttft_ms", ttftMs),
		logging.F("output_tokens", outputTokens),
		logging.F("has_tool_calls", hasToolCalls),
	)
}

func logToolCall(sessionID, runID, toolName, status string, durationMs int64, outputBytes int) {
	logging.Default().For(logging.ModuleAgent).Info("tool call finished",
		logging.F("session", sessionID),
		logging.F("run", runID),
		logging.F("tool", toolName),
		logging.F("status", status),
		logging.F("duration_ms", durationMs),
		logging.F("output_bytes", outputBytes),
	)
}

func logUserCancel(sessionID, runID, reason string) {
	logging.Default().For(logging.ModuleAgent).Info("agent run canceled",
		logging.F("session", sessionID),
		logging.F("run", runID),
		logging.F("reason", reason),
	)
}

func logCompactionTriggered(sessionID string, beforeTokens, threshold int64, preserveTurns int) {
	logging.Default().For(logging.ModuleAgent).Info("context compaction triggered",
		logging.F("session", sessionID),
		logging.F("before_tokens", beforeTokens),
		logging.F("threshold", threshold),
		logging.F("preserved_turns", preserveTurns),
	)
}

func logCompactionFailed(sessionID string, err error) {
	logging.Default().For(logging.ModuleAgent).Warn("context compaction failed",
		logging.F("session", sessionID),
		logging.F("error", logging.Redact(err.Error(), 200)),
	)
}
