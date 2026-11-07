package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// Regex for private keys
	privateKeyRegex = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9_\- ]*PRIVATE KEY-----.*?-----END [A-Z0-9_\- ]*PRIVATE KEY-----`)

	// Regex for Bearer tokens
	bearerRegex = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9\-_.~+/]+=*`)

	// Regex for sensitive headers
	headerRegex = regexp.MustCompile(`(?i)(Authorization|Proxy-Authorization|Cookie|Set-Cookie)\s*:\s*[^\r\n]+`)

	// Regex for sensitive key-value pairs (JSON, YAML, env, query-like)
	kvSensitiveRegex = regexp.MustCompile(`(?i)("?(?:token|api[_-]?key|secret|password|passwd|auth[_-]?token|credential|session[_-]?token|access[_-]?token|refresh[_-]?token)"?\s*[:=]\s*)("?[^"',\s;}{\[\]]+"?|\'[^\']*\')`)

	// Regex for URL query credentials
	urlQuerySensitiveRegex = regexp.MustCompile(`(?i)([?&](?:token|api[_-]?key|key|secret|auth|password|sig|signature|access[_-]?token)=)[^&\s#]+`)

	// Regex for Basic Auth in URLs
	urlBasicAuthRegex = regexp.MustCompile(`(https?://)([^:\s]+):([^@\s]+)@`)
)

// SanitizeEvidenceText replaces sensitive credentials and keys with safe redaction markers.
func SanitizeEvidenceText(input string) string {
	if input == "" {
		return ""
	}

	out := input
	// 1. Private keys
	out = privateKeyRegex.ReplaceAllString(out, "[REDACTED_PRIVATE_KEY]")

	// 2. Sensitive headers
	out = headerRegex.ReplaceAllStringFunc(out, func(m string) string {
		parts := strings.SplitN(m, ":", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[0]) + ": [REDACTED]"
		}
		return "[REDACTED_HEADER]"
	})

	// 3. Bearer tokens
	out = bearerRegex.ReplaceAllString(out, "Bearer [REDACTED_TOKEN]")

	// 4. URL basic auth
	out = urlBasicAuthRegex.ReplaceAllString(out, "${1}[REDACTED_USER]:[REDACTED_PASS]@")

	// 5. URL query params
	out = urlQuerySensitiveRegex.ReplaceAllString(out, "${1}[REDACTED]")

	// 6. Sensitive key-value pairs
	out = kvSensitiveRegex.ReplaceAllStringFunc(out, func(m string) string {
		sub := kvSensitiveRegex.FindStringSubmatch(m)
		if len(sub) == 3 {
			keyPrefix := sub[1]
			return keyPrefix + `"[REDACTED]"`
		}
		return m
	})

	// 7. Structured JSON deep pass if applicable
	trimmed := strings.TrimSpace(out)
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) ||
		(strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		var obj any
		if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
			if sanitizeJSONValue(obj) {
				if b, err := json.Marshal(obj); err == nil {
					out = string(b)
				}
			}
		}
	}

	return out
}

func isSensitiveKey(key string) bool {
	low := strings.ToLower(key)
	for _, term := range []string{"token", "key", "secret", "password", "passwd", "credential", "auth", "bearer", "cookie"} {
		if strings.Contains(low, term) {
			return true
		}
	}
	return false
}

func sanitizeJSONValue(v any) bool {
	modified := false
	switch val := v.(type) {
	case map[string]any:
		for k, child := range val {
			if isSensitiveKey(k) {
				val[k] = "[REDACTED]"
				modified = true
			} else {
				if sanitizeJSONValue(child) {
					modified = true
				}
			}
		}
	case []any:
		for _, child := range val {
			if sanitizeJSONValue(child) {
				modified = true
			}
		}
	}
	return modified
}

type evidenceRunData struct {
	RunID         string
	StartedAt     time.Time
	EndedAt       *time.Time
	UserPrompt    string
	UserMsgID     string
	AssistantMsgs []string
	Activities    []*Activity
	IsEnded       bool
}

type SnapshotResult struct {
	Scope            ReviewScope
	EvidenceSnapshot []ReviewEvidenceItem
	PromptPayload    string
}

func truncateText(text string, maxLen, headLen, tailLen int) (string, bool) {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text, false
	}
	if headLen+tailLen >= len(runes) {
		return text, false
	}
	omitted := len(runes) - headLen - tailLen
	truncated := string(runes[:headLen]) + fmt.Sprintf("\n... [截断省略 %d 字符] ...\n", omitted) + string(runes[len(runes)-tailLen:])
	return truncated, true
}

func BuildEvidenceSnapshot(session *Session, activeRunID string) (*SnapshotResult, error) {
	if session == nil {
		return nil, fmt.Errorf("会话不存在")
	}

	// 1. Record current stream cutoff positions
	cutoff := map[string]int{
		"messages":   len(session.Messages),
		"activities": len(session.Activity),
		"run_stats":  len(session.RunStats),
		"thoughts":   len(session.Thoughts),
	}

	// 2. Identify runs
	runOrder := []string{}
	runMap := map[string]*evidenceRunData{}

	getOrCreateRun := func(runID string) *evidenceRunData {
		if r, ok := runMap[runID]; ok {
			return r
		}
		r := &evidenceRunData{
			RunID:      runID,
			Activities: []*Activity{},
		}
		runMap[runID] = r
		runOrder = append(runOrder, runID)
		return r
	}

	// Map run stats first
	for _, stat := range session.RunStats {
		if stat.RunID == "" {
			continue
		}
		r := getOrCreateRun(stat.RunID)
		r.StartedAt = stat.StartedAt
		r.EndedAt = stat.EndedAt
		if stat.EndedAt != nil {
			r.IsEnded = true
		}
	}

	// Associate messages
	currentRunID := ""
	for _, msg := range session.Messages {
		rID := msg.RunID
		if rID == "" {
			rID = currentRunID
		}
		if msg.Role == "user" {
			if msg.RunID != "" {
				currentRunID = msg.RunID
			} else if currentRunID == "" {
				currentRunID = "run_" + msg.ID
			}
			rID = currentRunID
			r := getOrCreateRun(rID)
			r.UserPrompt = msg.Content
			r.UserMsgID = msg.ID
			if r.StartedAt.IsZero() {
				r.StartedAt = msg.CreatedAt
			}
		} else if msg.Role == "assistant" && rID != "" {
			r := getOrCreateRun(rID)
			if msg.Content != "" {
				r.AssistantMsgs = append(r.AssistantMsgs, msg.Content)
			}
		}
	}

	// Associate activities
	for i := range session.Activity {
		act := &session.Activity[i]
		rID := act.RunID
		if rID == "" {
			continue
		}
		r := getOrCreateRun(rID)
		r.Activities = append(r.Activities, act)
		if act.EndedAt != nil && !r.IsEnded {
			r.IsEnded = true
		}
	}

	// Collect ended runs
	endedRuns := []*evidenceRunData{}
	for _, rID := range runOrder {
		r := runMap[rID]
		// An active running run is not ended
		if activeRunID != "" && r.RunID == activeRunID {
			continue
		}
		// If endedAt is set or session itself is not running and has assistant reply/activity
		if r.IsEnded || (session.Status != StatusRunning && (len(r.AssistantMsgs) > 0 || len(r.Activities) > 0)) {
			r.IsEnded = true
			endedRuns = append(endedRuns, r)
		}
	}

	if len(endedRuns) == 0 {
		return nil, fmt.Errorf("会话中暂无已完成的 Agent 执行记录，无法进行复盘")
	}

	availableCount := len(endedRuns)
	// Select most recent MaxReviewRunCount runs (from newest to oldest, then sorted chronologically)
	startIndex := 0
	if availableCount > MaxReviewRunCount {
		startIndex = availableCount - MaxReviewRunCount
	}
	selectedRuns := endedRuns[startIndex:]

	truncations := []string{}
	if startIndex > 0 {
		truncations = append(truncations, fmt.Sprintf("历史 Run 数量较多（共 %d 个），已只选取最近 %d 个已结束 Run", availableCount, len(selectedRuns)))
	}

	var (
		evidenceItems []ReviewEvidenceItem
		runIDs        []string
		evIDCounter   = 1
	)

	// Format data for prompt
	type PromptRun struct {
		RunID        string   `json:"runId"`
		UserPrompt   string   `json:"userPrompt"`
		UserMsgRef   string   `json:"userMsgRef"`
		Assistant    []string `json:"assistantReplies,omitempty"`
		ToolSteps    []string `json:"toolSteps,omitempty"`
		EvidenceRefs []string `json:"evidenceRefs"`
	}

	var promptRuns []PromptRun

	for _, r := range selectedRuns {
		runIDs = append(runIDs, r.RunID)
		pr := PromptRun{
			RunID:        r.RunID,
			UserPrompt:   SanitizeEvidenceText(r.UserPrompt),
			UserMsgRef:   r.UserMsgID,
			EvidenceRefs: []string{},
		}

		// User prompt evidence
		userEvID := fmt.Sprintf("ev_%d", evIDCounter)
		evIDCounter++
		userSummary, _ := truncateText(SanitizeEvidenceText(r.UserPrompt), 200, 150, 40)
		evidenceItems = append(evidenceItems, ReviewEvidenceItem{
			ID:       userEvID,
			Kind:     "user_message",
			RunID:    r.RunID,
			SourceID: r.UserMsgID,
			Summary:  userSummary,
		})
		pr.EvidenceRefs = append(pr.EvidenceRefs, userEvID)

		// Assistant replies evidence
		for _, reply := range r.AssistantMsgs {
			sanitizedReply := SanitizeEvidenceText(reply)
			asstEvID := fmt.Sprintf("ev_%d", evIDCounter)
			evIDCounter++
			asstSummary, _ := truncateText(sanitizedReply, 250, 180, 50)
			evidenceItems = append(evidenceItems, ReviewEvidenceItem{
				ID:       asstEvID,
				Kind:     "assistant_message",
				RunID:    r.RunID,
				SourceID: r.RunID,
				Summary:  asstSummary,
			})
			pr.EvidenceRefs = append(pr.EvidenceRefs, asstEvID)
			pr.Assistant = append(pr.Assistant, sanitizedReply)
		}

		// Activities / tool steps
		for _, act := range r.Activities {
			toolEvID := fmt.Sprintf("ev_%d", evIDCounter)
			evIDCounter++

			sanitizedInput := SanitizeEvidenceText(act.Input)
			if sanitizedInput == "" {
				sanitizedInput = SanitizeEvidenceText(act.Detail)
			}
			rawOutput := act.Output
			var toolOutput string
			var truncated bool
			if rawOutput == "" && act.Status != "completed" {
				toolOutput = "[未知工具结果]"
			} else {
				sanitizedOut := SanitizeEvidenceText(rawOutput)
				toolOutput, truncated = truncateText(sanitizedOut, 800, 400, 200)
			}

			stepDesc := fmt.Sprintf("[%s] %s (工具: %s, 状态: %s) 输入: %s -> 输出: %s",
				toolEvID, act.Title, act.Tool, act.Status, sanitizedInput, toolOutput)
			pr.ToolSteps = append(pr.ToolSteps, stepDesc)
			pr.EvidenceRefs = append(pr.EvidenceRefs, toolEvID)

			summaryText := fmt.Sprintf("%s: %s (状态: %s)", act.Title, act.Tool, act.Status)
			evidenceItems = append(evidenceItems, ReviewEvidenceItem{
				ID:        toolEvID,
				Kind:      "tool_call",
				RunID:     r.RunID,
				SourceID:  act.ID,
				Summary:   summaryText,
				Truncated: truncated,
			})
		}

		promptRuns = append(promptRuns, pr)
	}

	payloadBytes, err := json.Marshal(promptRuns)
	if err != nil {
		return nil, err
	}

	// Check 128 KiB budget
	if len(payloadBytes) > MaxSnapshotBytes {
		// Cut prompt runs until under budget
		for len(payloadBytes) > MaxSnapshotBytes && len(promptRuns) > 1 {
			promptRuns = promptRuns[1:] // remove oldest
			payloadBytes, _ = json.Marshal(promptRuns)
		}
		truncations = append(truncations, fmt.Sprintf("快照序列化超过 128 KiB 预算，已裁剪最早期 Run 至 %d 字节", len(payloadBytes)))
	}

	scope := ReviewScope{
		CutoffPositions: cutoff,
		RunIDs:          runIDs,
		AvailableCount:  availableCount,
		IncludedCount:   len(selectedRuns),
		Truncation:      truncations,
	}

	return &SnapshotResult{
		Scope:            scope,
		EvidenceSnapshot: evidenceItems,
		PromptPayload:    string(payloadBytes),
	}, nil
}
