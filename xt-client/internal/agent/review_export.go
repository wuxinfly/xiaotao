package agent

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	unsafeFilenameChars = regexp.MustCompile(`[\\/:*?"<>|\s]+`)
)

func sanitizeFilenamePart(s string) string {
	cleaned := unsafeFilenameChars.ReplaceAllString(s, "_")
	cleaned = strings.Trim(cleaned, "._")
	return cleaned
}

// GenerateExportMarkdown formats a succeeded behavior review as a standalone Markdown document.
func (s *Service) GenerateExportMarkdown(id string) (filename string, content string, err error) {
	r, _, err := s.getReview(id)
	if err != nil {
		return "", "", err
	}
	if r.Status != ReviewStatusSucceeded || r.Result == nil {
		return "", "", fmt.Errorf("只能导出已成功的复盘记录")
	}

	tz, err := time.LoadLocation(r.Timezone)
	if err != nil || tz == nil {
		tz = time.FixedZone("CST", 8*3600)
	}
	localCreated := r.CreatedAt.In(tz)
	dateStr := localCreated.Format("2006-01-02")
	timeStr := localCreated.Format("150405")

	title := DeriveReviewTitle(r.CreatedAt, r.Timezone, r.Result)
	themePart := sanitizeFilenamePart(strings.TrimPrefix(title, dateStr+" "))
	if themePart == "" {
		themePart = "行为复盘记录"
	}

	shortID := r.ID
	if len(shortID) > 12 {
		shortID = shortID[len(shortID)-8:]
	}

	filename = fmt.Sprintf("%s_%s_%s_%s.md", dateStr, timeStr, themePart, shortID)
	if len(filename) > 100 {
		filename = filename[:96] + ".md"
	}

	var md strings.Builder

	// Header
	md.WriteString(fmt.Sprintf("# %s\n\n", title))

	// Section 1: 复盘范围
	md.WriteString("## 1. 复盘范围\n\n")
	md.WriteString(fmt.Sprintf("- **原会话标题**：%s\n", r.SessionTitleSnapshot))
	md.WriteString(fmt.Sprintf("- **所属项目**：%s\n", r.ProjectSnapshot))
	md.WriteString(fmt.Sprintf("- **生成时间**：%s (%s)\n", localCreated.Format("2006-01-02 15:04:05"), r.Timezone))
	md.WriteString(fmt.Sprintf("- **分析模型**：%s\n", r.Model))
	md.WriteString(fmt.Sprintf("- **客户端版本**：%s\n", r.ClientVersion))
	md.WriteString(fmt.Sprintf("- **Core 版本**：%s\n", r.CoreVersion))
	md.WriteString(fmt.Sprintf("- **提示模板版本**：%s\n", r.PromptVersion))
	md.WriteString(fmt.Sprintf("- **覆盖 Run 范围**：共可用 %d 个已结束 Run，实际纳入分析 %d 个 (Run IDs: %s)\n",
		r.Scope.AvailableCount, r.Scope.IncludedCount, strings.Join(r.Scope.RunIDs, ", ")))
	if len(r.Scope.Truncation) > 0 {
		md.WriteString(fmt.Sprintf("- **截取说明**：%s\n", strings.Join(r.Scope.Truncation, "；")))
	}
	md.WriteString("\n")

	// Section 2: 结论
	md.WriteString("## 2. 结论\n\n")
	outcomeDesc := ""
	switch r.Result.Outcome {
	case ReviewOutcomeIssuesFound:
		outcomeDesc = "发现问题"
	case ReviewOutcomeNoIssuesFound:
		outcomeDesc = "本次范围内未发现有依据的问题"
	case ReviewOutcomeInsufficientEvidence:
		outcomeDesc = "证据不足，无法充分判断"
	default:
		outcomeDesc = r.Result.Outcome
	}
	md.WriteString(fmt.Sprintf("**总评**：%s\n\n", outcomeDesc))
	if r.Result.Summary != "" {
		md.WriteString(fmt.Sprintf("%s\n\n", r.Result.Summary))
	}

	// Section 3: 发现的问题
	md.WriteString("## 3. 发现的问题\n\n")
	if len(r.Result.Issues) == 0 {
		md.WriteString("本次复盘范围内未发现有确凿依据的协作或行为问题。\n\n")
	} else {
		for i, issue := range r.Result.Issues {
			md.WriteString(fmt.Sprintf("### 3.%d %s\n\n", i+1, issue.Title))
			md.WriteString(fmt.Sprintf("- **观察到的行为事实**：%s\n", issue.Problem))
			md.WriteString(fmt.Sprintf("- **造成的影响**：%s\n", issue.Impact))
			md.WriteString(fmt.Sprintf("- **推测原因**：%s\n", issue.Speculation))
			md.WriteString(fmt.Sprintf("- **改进建议**：%s\n", issue.Proposal))
			md.WriteString(fmt.Sprintf("- **验证改善方法**：%s\n", issue.Acceptance))
			md.WriteString(fmt.Sprintf("- **证据引用**：%s\n\n", strings.Join(issue.EvidenceIDs, ", ")))
		}
	}

	// Section 4: 值得保留的做法
	md.WriteString("## 4. 值得保留的做法\n\n")
	if len(r.Result.GoodPractices) == 0 {
		md.WriteString("本次复盘未特别记录额外良好做法。\n\n")
	} else {
		for i, gp := range r.Result.GoodPractices {
			md.WriteString(fmt.Sprintf("### 4.%d %s\n\n", i+1, gp.Title))
			md.WriteString(fmt.Sprintf("%s\n\n", gp.Description))
			if len(gp.EvidenceIDs) > 0 {
				md.WriteString(fmt.Sprintf("- **证据依据**：%s\n\n", strings.Join(gp.EvidenceIDs, ", ")))
			}
		}
	}

	// Section 5: 限制
	md.WriteString("## 5. 限制说明\n\n")
	if len(r.Result.Limitations) == 0 && len(r.Scope.Truncation) == 0 {
		md.WriteString("本次分析证据完整，无显著限制。\n\n")
	} else {
		allLimits := append([]string{}, r.Result.Limitations...)
		allLimits = append(allLimits, r.Scope.Truncation...)
		for _, lim := range allLimits {
			md.WriteString(fmt.Sprintf("- %s\n", lim))
		}
		md.WriteString("\n")
	}

	// Appendix: 证据清单摘要
	md.WriteString("## 附录：证据快照索引\n\n")
	for _, ev := range r.EvidenceSnapshot {
		truncMark := ""
		if ev.Truncated {
			truncMark = " [已截断]"
		}
		md.WriteString(fmt.Sprintf("- `[%s]` (%s%s) %s\n", ev.ID, ev.Kind, truncMark, ev.Summary))
	}

	content = md.String()
	return filename, content, nil
}
