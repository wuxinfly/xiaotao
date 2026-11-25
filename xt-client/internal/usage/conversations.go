package usage

import (
	"sort"
	"time"
)

type ContextParts struct {
	System     int64 `json:"system"`
	Tools      int64 `json:"tools"`
	Messages   int64 `json:"messages"`
	ToolCalls  int64 `json:"toolCalls"`
	References int64 `json:"references"`
	Images     int64 `json:"images"`
}

type ContextItem struct {
	Label  string `json:"label"`
	Tokens int64  `json:"tokens"`
}

type RequestContext struct {
	Items     map[string][]ContextItem `json:"items,omitempty"`
	Window    int64                    `json:"window"`
	Estimated int64                    `json:"estimated"`
	Parts     ContextParts             `json:"parts"`
}

type ConversationInput struct {
	TitleLookup                       func() string
	Auxiliary                         bool
	ID, Title, Source, InferenceScope string
	Context                           RequestContext
	Prefixes                          [][32]byte
}

type conversation struct {
	titleLookup                       func() string
	id, title, source, inferenceScope string
	updated                           time.Time
	fingerprint                       [32]byte
	active                            int
	totals                            TokenMetrics
	lastMain                          *Record
	finished                          []*Record
}

type ConversationSummary struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Source          string           `json:"source"`
	Active          int              `json:"active"`
	Context         RequestContext   `json:"context"`
	Totals          TokenMetrics     `json:"totals"`
	LatestRequestID string           `json:"latestRequestId"`
	Requests        []RequestSummary `json:"requests"`
}

const recentConversationLimit = 16

// Caller holds s.mu. Only the latest fingerprint is kept for inference.
func (s *Service) attachConversation(rec *Record, input ConversationInput) {
	if s.conversations == nil {
		s.conversations = make(map[string]*conversation)
	}
	id := input.ID
	if id == "" {
		id = rec.RequestID
		if match := s.matchConversation(input.InferenceScope, input.Prefixes); match != nil {
			id = match.id
			input.Source = "按历史关联"
		}
	}
	item := s.conversations[id]
	if item == nil {
		title := input.Title
		if title == "" {
			label := input.Source
			if label == "归属待确认" {
				label = "外部 API"
			}
			title = label + " " + rec.RequestID[:min(len(rec.RequestID), 6)]
		}
		item = &conversation{id: id, title: title, source: input.Source, inferenceScope: input.InferenceScope}
		s.conversations[id] = item
	}
	if input.TitleLookup != nil {
		item.titleLookup = input.TitleLookup
	}
	if input.Title != "" && (input.TitleLookup == nil || item.lastMain == nil) {
		item.title = input.Title
	}
	item.source = input.Source
	if rec.CreatedAt.After(item.updated) {
		item.updated = rec.CreatedAt
	}
	item.active++
	if len(input.Prefixes) > 0 {
		item.fingerprint = input.Prefixes[len(input.Prefixes)-1]
	}
	rec.auxiliary = input.Auxiliary
	rec.conversationID = id
	rec.requestContext = input.Context
}

func (s *Service) matchConversation(scope string, prefixes [][32]byte) *conversation {
	if len(prefixes) < 3 {
		return nil
	}
	var match *conversation
	for _, prefix := range prefixes[:len(prefixes)-1] {
		for _, item := range s.conversations {
			if item.inferenceScope != scope || item.fingerprint != prefix {
				continue
			}
			if match != nil || item.active != 0 || len(item.finished) == 0 || item.finished[0].Status != StatusCompleted {
				return nil
			}
			match = item
		}
	}
	return match
}

// Caller holds s.mu. Totals hold only confirmed usage for completed records.
func (s *Service) finishConversation(rec *Record) {
	item := s.conversations[rec.conversationID]
	if item == nil {
		return
	}
	item.active--
	if rec.EndedAt != nil && rec.EndedAt.After(item.updated) {
		item.updated = *rec.EndedAt
	}
	addTokens(&item.totals.Prompt, rec.PromptTokens)
	addTokens(&item.totals.Completion, rec.CompletionTokens)
	addTokens(&item.totals.Cached, rec.CachedTokens)
	if !rec.auxiliary && (item.lastMain == nil || rec.CreatedAt.After(item.lastMain.CreatedAt)) {
		item.lastMain = rec
	}
	item.finished = append(item.finished, rec)
	sort.Slice(item.finished, func(i, j int) bool { return item.finished[i].CreatedAt.After(item.finished[j].CreatedAt) })
	item.finished = item.finished[:min(3, len(item.finished))]
	var inactive []*conversation
	for _, entry := range s.conversations {
		if entry.active == 0 {
			inactive = append(inactive, entry)
		}
	}
	if len(inactive) > recentConversationLimit {
		sort.Slice(inactive, func(i, j int) bool { return inactive[i].updated.Before(inactive[j].updated) })
		for _, entry := range inactive[:len(inactive)-recentConversationLimit] {
			delete(s.conversations, entry.id)
		}
	}
}

func addTokens(total **int64, value *int64) {
	if value == nil {
		return
	}
	n := *value
	if *total != nil {
		n += **total
	}
	*total = &n
}

// Copy under the service lock without taking any request lock.
func (s *Service) copyConversations() []conversation {
	items := make([]conversation, 0, len(s.conversations))
	for _, item := range s.conversations {
		copyItem := *item
		copyItem.finished = append([]*Record(nil), item.finished...)
		items = append(items, copyItem)
	}
	return items
}

func summarizeConversations(items []conversation, active []*Record, now time.Time) []ConversationSummary {
	sort.Slice(items, func(i, j int) bool {
		if (items[i].active > 0) != (items[j].active > 0) {
			return items[i].active > 0
		}
		if items[i].updated.Equal(items[j].updated) {
			return items[i].id < items[j].id
		}
		return items[i].updated.After(items[j].updated)
	})
	byConversation := make(map[string][]*Record)
	for _, rec := range active {
		byConversation[rec.conversationID] = append(byConversation[rec.conversationID], rec)
	}
	summaries := make([]ConversationSummary, 0, len(items))
	for _, item := range items {
		running := byConversation[item.id]
		seen := make(map[string]bool)
		var latest *Record
		entry := ConversationSummary{ID: item.id, Title: item.title, Source: item.source, Totals: item.totals}
		records := append(running, item.finished...)
		if item.lastMain != nil {
			records = append(records, item.lastMain)
		}
		for _, rec := range records {
			if seen[rec.RequestID] {
				continue
			}
			seen[rec.RequestID] = true
			if latest == nil || (latest.auxiliary && !rec.auxiliary) || (latest.auxiliary == rec.auxiliary && rec.CreatedAt.After(latest.CreatedAt)) {
				latest = rec
			}
			entry.Requests = append(entry.Requests, toSummary(rec, now))
			if rec.Status == StatusPending || rec.Status == StatusStreaming {
				entry.Active++
			}
		}
		if latest == nil {
			continue
		}
		if item.titleLookup != nil {
			if title := item.titleLookup(); title != "" {
				entry.Title = title
			}
		}
		entry.Context = latest.requestContext
		entry.LatestRequestID = latest.RequestID
		summaries = append(summaries, entry)
	}
	return summaries
}
