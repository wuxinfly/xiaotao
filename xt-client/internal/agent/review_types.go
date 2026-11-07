package agent

import "time"

const (
	ReviewStatusPending   = "pending"
	ReviewStatusRunning   = "running"
	ReviewStatusSucceeded = "succeeded"
	ReviewStatusFailed    = "failed"
	ReviewStatusCanceled  = "canceled"

	ReviewOutcomeIssuesFound          = "issues_found"
	ReviewOutcomeNoIssuesFound        = "no_issues_found"
	ReviewOutcomeInsufficientEvidence = "insufficient_evidence"

	ReviewPromptVersion = "v1"
	ReviewSchemaVersion = 1

	MaxConcurrentReviews = 2
	MaxReviewRunCount    = 10
	MaxSnapshotBytes     = 128 * 1024 // 128 KiB
	ReviewTimeout        = 5 * time.Minute
)

type ReviewScope struct {
	CutoffPositions map[string]int `json:"cutoffPositions"` // table -> cutoff position
	RunIDs          []string       `json:"runIds"`
	AvailableCount  int            `json:"availableCount"`
	IncludedCount   int            `json:"includedCount"`
	Truncation      []string       `json:"truncation,omitempty"`
}

type ReviewEvidenceItem struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // "user_message", "assistant_message", "tool_call", "tool_result", "thought_summary"
	RunID     string `json:"runId"`
	SourceID  string `json:"sourceId"` // original message ID, activity ID, etc.
	Summary   string `json:"summary"`  // sanitized preview summary
	Truncated bool   `json:"truncated,omitempty"`
}

type ReviewIssue struct {
	ID          string   `json:"id"`          // Stable issue ID inside the review (e.g. "issue_1")
	Title       string   `json:"title"`       // Short issue title
	Problem     string   `json:"problem"`     // Observed behavior fact
	Impact      string   `json:"impact"`      // Impact on collaboration or delivery
	Speculation string   `json:"speculation"` // Inferred reason (explicitly marked as speculation)
	Proposal    string   `json:"proposal"`    // Concrete suggestion for improvement
	Acceptance  string   `json:"acceptance"`  // How to verify improvement
	EvidenceIDs []string `json:"evidenceIds"` // Real IDs referencing EvidenceSnapshot
}

type ReviewGoodPractice struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	EvidenceIDs []string `json:"evidenceIds,omitempty"`
}

type ReviewResult struct {
	Outcome       string               `json:"outcome"` // issues_found | no_issues_found | insufficient_evidence
	Summary       string               `json:"summary"` // Overview of agent behavior
	Issues        []ReviewIssue        `json:"issues"`
	GoodPractices []ReviewGoodPractice `json:"goodPractices"`
	Limitations   []string             `json:"limitations"`
}

type ReviewUsage struct {
	PromptTokens     *int64 `json:"promptTokens,omitempty"`
	CompletionTokens int64  `json:"completionTokens"`
	TotalTokens      int64  `json:"totalTokens"`
}

type ReviewSuggestionLink struct {
	IssueID       string    `json:"issueId"`
	IssueKey      string    `json:"issueKey"`
	ObservationID string    `json:"observationId"`
	LinkedAt      time.Time `json:"linkedAt"`
	Status        string    `json:"status"` // "pending", etc.
	SourcePath    string    `json:"sourcePath"`
}

type BehaviorReview struct {
	ID                   string                 `json:"id"`
	SchemaVersion        int                    `json:"schemaVersion"`
	SessionID            string                 `json:"sessionId"`
	ProjectSnapshot      string                 `json:"projectSnapshot"`
	SessionTitleSnapshot string                 `json:"sessionTitleSnapshot"`
	CreatedAt            time.Time              `json:"createdAt"`
	StartedAt            *time.Time             `json:"startedAt,omitempty"`
	FinishedAt           *time.Time             `json:"finishedAt,omitempty"`
	Timezone             string                 `json:"timezone"`
	Status               string                 `json:"status"`
	RequestID            string                 `json:"requestId"`
	AttemptOf            string                 `json:"attemptOf,omitempty"`
	Model                string                 `json:"model"`
	ModelSettings        *ModelSettings         `json:"modelSettings,omitempty"`
	ClientVersion        string                 `json:"clientVersion"`
	CoreVersion          string                 `json:"coreVersion"`
	PromptVersion        string                 `json:"promptVersion"`
	Scope                ReviewScope            `json:"scope"`
	EvidenceSnapshot     []ReviewEvidenceItem   `json:"evidenceSnapshot"`
	Result               *ReviewResult          `json:"result,omitempty"`
	ErrorCode            string                 `json:"errorCode,omitempty"`
	ErrorMessage         string                 `json:"errorMessage,omitempty"`
	Usage                *ReviewUsage           `json:"usage,omitempty"`
	SuggestionLinks      []ReviewSuggestionLink `json:"suggestionLinks,omitempty"`
}

type ReviewSummary struct {
	ID                   string     `json:"id"`
	SessionID            string     `json:"sessionId"`
	SessionTitleSnapshot string     `json:"sessionTitleSnapshot"`
	ProjectSnapshot      string     `json:"projectSnapshot"`
	CreatedAt            time.Time  `json:"createdAt"`
	StartedAt            *time.Time `json:"startedAt,omitempty"`
	FinishedAt           *time.Time `json:"finishedAt,omitempty"`
	Timezone             string     `json:"timezone"`
	Status               string     `json:"status"`
	Title                string     `json:"title"` // Derived user-facing title
	Model                string     `json:"model"`
	AttemptOf            string     `json:"attemptOf,omitempty"`
	ErrorCode            string     `json:"errorCode,omitempty"`
	ErrorMessage         string     `json:"errorMessage,omitempty"`
	Outcome              string     `json:"outcome,omitempty"`
	IssuesCount          int        `json:"issuesCount"`
}

type ReviewListPage struct {
	Reviews    []ReviewSummary `json:"reviews"`
	NextCursor string          `json:"nextCursor,omitempty"`
	Total      int             `json:"total"`
}

type CreateReviewRequest struct {
	RequestID     string         `json:"requestId"`
	Model         string         `json:"model,omitempty"`
	ModelSettings *ModelSettings `json:"modelSettings,omitempty"`
}

type RetryReviewRequest struct {
	RequestID string `json:"requestId"`
}

type AddSuggestionRequest struct {
	IssueKey string `json:"issueKey,omitempty"` // If empty, a new issueKey will be generated
	Title    string `json:"title,omitempty"`
}

type ExistingSuggestionItem struct {
	IssueKey          string    `json:"issueKey"`
	Title             string    `json:"title"`
	Problem           string    `json:"problem"`
	Status            string    `json:"status"`
	ObservationsCount int       `json:"observationsCount"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
